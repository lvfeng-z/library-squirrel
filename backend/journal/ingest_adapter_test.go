package journal

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/export"
	importer "github.com/library-squirrel/backend/import"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// fakeManifestIngestor 原导入器测试替身：固定返回结果/错误，计数调用（并发安全）
type fakeManifestIngestor struct {
	mu     sync.Mutex
	calls  int
	result *importer.ImportResult
	err    error
}

func (f *fakeManifestIngestor) Ingest(ctx context.Context, manifest *export.Manifest, fileSource importer.FileSource, staging importer.IngestStaging, opts *importer.IngestOptions) (*importer.ImportResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.result, f.err
}

func (f *fakeManifestIngestor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeWorkTitleReader 标题批量读取替身（并发安全）
type fakeWorkTitleReader struct {
	mu    sync.Mutex
	calls int
	fulls []*dto.WorkFullDTO
	err   error
}

func (f *fakeWorkTitleReader) GetFullWorkInfoByIds(ctx context.Context, ids []int64) ([]*dto.WorkFullDTO, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.fulls, f.err
}

func (f *fakeWorkTitleReader) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeWorkInfoSaver 原保存器替身（并发安全）
type fakeWorkInfoSaver struct {
	mu     sync.Mutex
	calls  int
	workID int64
	err    error
}

func (f *fakeWorkInfoSaver) SaveWorkInfo(ctx context.Context, task *entity.Task, workTask *entity.WorkTask, workResp *sdkdto.WorkResponse) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.workID, f.err
}

func (f *fakeWorkInfoSaver) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// failingJournalRepo 写账必败仓储（Service 依赖面 Repository 的替身）：Create 恒错，
// 驱动「写账失败不影响业务返回」路径（轮转失败仅告警不阻断，见 Service.Append）
type failingJournalRepo struct{}

func (failingJournalRepo) Create(ctx context.Context, entry *entity.JournalEntry) error {
	return errors.New("写账失败")
}

func (failingJournalRepo) ListSinceSnapshot(ctx context.Context, cursor int64, limit int) (*ListSnapshot, error) {
	return &ListSnapshot{}, nil
}

func (failingJournalRepo) RotateOldest(ctx context.Context, expiredBefore int64, keepRows int) (int64, error) {
	return 0, nil
}

// decodeIngestPayload 解析入库账行载荷（断言辅助）
func decodeIngestPayload(t *testing.T, e *entity.JournalEntry) workIngestedPayload {
	t.Helper()
	var payload workIngestedPayload
	if err := json.Unmarshal([]byte(e.Payload), &payload); err != nil {
		t.Fatalf("账行载荷非 JSON 摘要: %v（payload=%q）", err, e.Payload)
	}
	return payload
}

// TestManifestIngestDecoratorJournalsCreatedWorks 导入路径（分享收件 / zip 导入共用装饰器）：
// Ingest 成功且建行清单非空时逐作品落账（域/事件类型/主体/载荷逐项断言），业务结果原样
// 透传、原导入器恰好调用一次；标题读取失败降级空标题不阻断落账；失败或零建行零落账
func TestManifestIngestDecoratorJournalsCreatedWorks(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()

	strPtr := func(s string) *string { return &s }
	inner := &fakeManifestIngestor{result: &importer.ImportResult{
		CreatedWorks:   2,
		CreatedWorkIDs: []int64{101, 102},
	}}
	titles := &fakeWorkTitleReader{fulls: []*dto.WorkFullDTO{
		{Work: &sdkdto.WorkDTO{Id: 101, SiteWorkName: strPtr("导入作品一")}},
		{Work: &sdkdto.WorkDTO{Id: 102}}, // 标题缺省（指针 nil）：载荷降级空串
	}}
	dec := NewManifestIngestJournalDecorator(inner, NewWorkIngestRecorder(svc), titles)

	got, err := dec.Ingest(ctx, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("导入透传不应报错: %v", err)
	}
	if got != inner.result {
		t.Fatalf("业务结果应原样透传（同一结果实例）")
	}
	if inner.callCount() != 1 || titles.callCount() != 1 {
		t.Fatalf("原导入器与标题读取应各恰好调用一次，实际 %d/%d", inner.callCount(), titles.callCount())
	}

	entries := listTerminalEntries(t, repo)
	if len(entries) != 2 {
		t.Fatalf("两建行作品应各落一行账，实际 %d 行", len(entries))
	}
	wantTitles := map[int64]string{101: "导入作品一", 102: ""}
	for i, wantID := range []int64{101, 102} {
		e := entries[i]
		if e.Domain != entity.JournalDomainIngest || e.EventType != EventTypeWorkIngested {
			t.Fatalf("账行域/事件类型不符: domain=%s eventType=%s", e.Domain, e.EventType)
		}
		if e.SubjectID != wantID {
			t.Fatalf("账行主体引用应为作品 ID %d，实际 %d", wantID, e.SubjectID)
		}
		payload := decodeIngestPayload(t, e)
		if payload.WorkID != wantID || payload.Title != wantTitles[wantID] || payload.Source != IngestSourceManifestImport {
			t.Fatalf("建行作品 %d 载荷不符: %+v", wantID, payload)
		}
	}

	// 标题读取失败：落账不阻断，载荷降级空标题（主体=作品 ID 已足够消费方回查）
	before := countRows(t, repo)
	failingTitles := &fakeWorkTitleReader{err: errors.New("标题读取失败")}
	decNoTitle := NewManifestIngestJournalDecorator(
		&fakeManifestIngestor{result: &importer.ImportResult{CreatedWorks: 1, CreatedWorkIDs: []int64{103}}},
		NewWorkIngestRecorder(svc), failingTitles)
	if _, err := decNoTitle.Ingest(ctx, nil, nil, nil, nil); err != nil {
		t.Fatalf("标题读取失败不应影响导入返回: %v", err)
	}
	entries = listTerminalEntries(t, repo)
	if len(entries) != int(before)+1 {
		t.Fatalf("标题读取失败仍应落账，期望 %d 行实际 %d 行", before+1, len(entries))
	}
	last := entries[len(entries)-1]
	if last.SubjectID != 103 {
		t.Fatalf("降级落账主体引用应为 103，实际 %d", last.SubjectID)
	}
	if payload := decodeIngestPayload(t, last); payload.Title != "" || payload.Source != IngestSourceManifestImport {
		t.Fatalf("降级载荷不符: %+v", payload)
	}

	// 失败透传零落账（错误语义不变）
	ingestErr := errors.New("导入失败")
	decErr := NewManifestIngestJournalDecorator(&fakeManifestIngestor{err: ingestErr}, NewWorkIngestRecorder(svc), titles)
	if _, err := decErr.Ingest(ctx, nil, nil, nil, nil); !errors.Is(err, ingestErr) {
		t.Fatalf("导入失败应原样透传: got %v", err)
	}

	// 零建行（全跳过 / 仅替换增补——不新建作品行）零落账
	decSkip := NewManifestIngestJournalDecorator(
		&fakeManifestIngestor{result: &importer.ImportResult{ReplacedConfirmed: 1}}, NewWorkIngestRecorder(svc), titles)
	if _, err := decSkip.Ingest(ctx, nil, nil, nil, nil); err != nil {
		t.Fatalf("零建行导入不应报错: %v", err)
	}
	if got, want := countRows(t, repo), before+1; got != want {
		t.Fatalf("失败/零建行应零新增账，期望 %d 行实际 %d 行", want, got)
	}
}

// TestSaveWorkInfoDecoratorJournalsCreatedWork 插件下载路径：保存成功后按返回作品 ID 落一行账
// （载荷标题取 WorkResponse、来源=plugin_download）；WorkResponse 缺省时标题降级空串；
// 保存失败原样透传零落账
func TestSaveWorkInfoDecoratorJournalsCreatedWork(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()

	title := "下载作品标题"
	inner := &fakeWorkInfoSaver{workID: 501}
	dec := NewSaveWorkInfoIngestDecorator(inner, NewWorkIngestRecorder(svc))

	workResp := &sdkdto.WorkResponse{Work: &sdkdto.WorkDTO{SiteWorkName: &title}}
	gotID, err := dec.SaveWorkInfo(ctx, nil, nil, workResp)
	if err != nil {
		t.Fatalf("保存透传不应报错: %v", err)
	}
	if gotID != 501 || inner.callCount() != 1 {
		t.Fatalf("业务返回应原样透传: id=%d calls=%d", gotID, inner.callCount())
	}

	entries := listTerminalEntries(t, repo)
	if len(entries) != 1 {
		t.Fatalf("保存成功应落 1 行账，实际 %d 行", len(entries))
	}
	e := entries[0]
	if e.Domain != entity.JournalDomainIngest || e.EventType != EventTypeWorkIngested || e.SubjectID != 501 {
		t.Fatalf("账行域/事件类型/主体不符: %+v", e)
	}
	payload := decodeIngestPayload(t, e)
	if payload.WorkID != 501 || payload.Title != title || payload.Source != IngestSourcePluginDownload {
		t.Fatalf("下载路径载荷不符: %+v", payload)
	}

	// WorkResponse 缺省（nil / Work 缺失）：标题降级空串仍落账
	innerNil := &fakeWorkInfoSaver{workID: 502}
	decNil := NewSaveWorkInfoIngestDecorator(innerNil, NewWorkIngestRecorder(svc))
	if _, err := decNil.SaveWorkInfo(ctx, nil, nil, nil); err != nil {
		t.Fatalf("WorkResponse 缺省不应报错: %v", err)
	}
	entries = listTerminalEntries(t, repo)
	if len(entries) != 2 {
		t.Fatalf("缺省标题仍应落账，期望 2 行实际 %d 行", len(entries))
	}
	if payload := decodeIngestPayload(t, entries[1]); entries[1].SubjectID != 502 || payload.Title != "" {
		t.Fatalf("缺省标题账行不符: subject=%d payload=%+v", entries[1].SubjectID, payload)
	}

	// 保存失败：错误原样透传、零新增账
	saveErr := errors.New("保存作品信息失败")
	decErr := NewSaveWorkInfoIngestDecorator(&fakeWorkInfoSaver{err: saveErr}, NewWorkIngestRecorder(svc))
	if _, err := decErr.SaveWorkInfo(ctx, nil, nil, nil); !errors.Is(err, saveErr) {
		t.Fatalf("保存失败应原样透传: got %v", err)
	}
	if got := countRows(t, repo); got != 2 {
		t.Fatalf("保存失败应零新增账，实际 %d 行", got)
	}
}

// TestIngestJournalWriteFailureKeepsBusinessReturn 写账失败不影响业务返回：仓储恒错的记账
// 服务下，两路径装饰器的返回值与错误语义不变、不 panic；未装配记账（nil recorder / nil svc）
// 静默跳过
func TestIngestJournalWriteFailureKeepsBusinessReturn(t *testing.T) {
	ctx := context.Background()
	failingSvc := NewService(failingJournalRepo{})

	// 导入路径：结果实例与错误原样返回
	inner := &fakeManifestIngestor{result: &importer.ImportResult{CreatedWorks: 1, CreatedWorkIDs: []int64{7}}}
	dec := NewManifestIngestJournalDecorator(inner, NewWorkIngestRecorder(failingSvc), &fakeWorkTitleReader{})
	got, err := dec.Ingest(ctx, nil, nil, nil, nil)
	if err != nil || got != inner.result {
		t.Fatalf("写账失败不应影响导入返回: got=%v err=%v", got, err)
	}

	// 下载路径：作品 ID 与错误原样返回
	saver := &fakeWorkInfoSaver{workID: 9}
	decSave := NewSaveWorkInfoIngestDecorator(saver, NewWorkIngestRecorder(failingSvc))
	id, err := decSave.SaveWorkInfo(ctx, nil, nil, nil)
	if err != nil || id != 9 {
		t.Fatalf("写账失败不应影响保存返回: id=%d err=%v", id, err)
	}

	// 未装配记账（nil 适配器 / 适配器持 nil 服务）：静默跳过、业务照常
	var nilRecorder *WorkIngestRecorder
	nilRecorder.RecordWorkIngest(1, "t", IngestSourcePluginDownload) // 不 panic 即通过
	decUnwired := NewManifestIngestJournalDecorator(
		&fakeManifestIngestor{result: &importer.ImportResult{CreatedWorkIDs: []int64{1}}},
		NewWorkIngestRecorder(nil), nil) // svc=nil + titles=nil 双缺省
	if got, err := decUnwired.Ingest(ctx, nil, nil, nil, nil); err != nil || got == nil {
		t.Fatalf("未装配记账不应影响导入返回: got=%v err=%v", got, err)
	}
}

// TestIngestJournalConcurrentBothPaths 两路径并发入库记账（-race）：多 goroutine 并发经两装饰器
// 落账——行数守恒（导入每轮 2 行 + 下载每轮 1 行）、账行 id 严格升序无重复
func TestIngestJournalConcurrentBothPaths(t *testing.T) {
	svc, repo := newTestService(t)

	ingestDec := NewManifestIngestJournalDecorator(
		&fakeManifestIngestor{result: &importer.ImportResult{CreatedWorks: 2, CreatedWorkIDs: []int64{201, 202}}},
		NewWorkIngestRecorder(svc), &fakeWorkTitleReader{})
	saveDec := NewSaveWorkInfoIngestDecorator(&fakeWorkInfoSaver{workID: 301}, NewWorkIngestRecorder(svc))

	const workers = 8
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			if _, err := ingestDec.Ingest(context.Background(), nil, nil, nil, nil); err != nil {
				t.Errorf("并发导入透传失败: %v", err)
				return
			}
			if _, err := saveDec.SaveWorkInfo(context.Background(), nil, nil, nil); err != nil {
				t.Errorf("并发保存透传失败: %v", err)
			}
		})
	}
	wg.Wait()

	entries := listTerminalEntries(t, repo)
	if got, want := len(entries), workers*3; got != want {
		t.Fatalf("并发两路径应共落 %d 行账，实际 %d 行", want, got)
	}
	prev := int64(0)
	seen := make(map[int64]struct{}, len(entries))
	for _, e := range entries {
		if e.Domain != entity.JournalDomainIngest || e.EventType != EventTypeWorkIngested {
			t.Fatalf("账行域/事件类型不符: %+v", e)
		}
		if id := e.GetID(); id <= prev {
			t.Fatalf("账行 id 应严格升序: prev=%d id=%d", prev, id)
		} else {
			prev = id
			seen[id] = struct{}{}
		}
	}
	if len(seen) != workers*3 {
		t.Fatalf("账行 id 不应重复: 去重后 %d", len(seen))
	}
}
