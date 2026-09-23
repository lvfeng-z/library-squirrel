package task

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/pluginTaskUrlListener"
	"github.com/library-squirrel/backend/site"
	"github.com/library-squirrel/backend/stickymemory"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// 本文件为任务 URL 创建面消歧记忆（读点命中直路由 / 候选失效重问 / 程序化入口不触达 /
// 勾选与成功门控写入 / 键编码）的回归测试。
//
// 统一契约：
//   - 交互入口多候选未显选时先查记忆：命中且记住的候选仍在候选集内 → 等价显选直路由，
//     不返回冲突载荷；未命中或候选不在场 → 照常冲突载荷。
//   - 程序化入口不查不写（即使存在本应命中的记忆，行为也与无记忆一致）。
//   - 写入仅在「显选有效 + 路由成功 + remember 勾选」三者齐备时发生；站点域求值失败不写。

// missDisambigMemory 恒未命中的消歧记忆替身：Remember 静默丢弃。维持「无记忆时冲突照常」的行为基线
type missDisambigMemory struct{}

func (missDisambigMemory) Recall(context.Context, string, string) (string, bool) { return "", false }
func (missDisambigMemory) Remember(context.Context, string, string, string) error {
	return nil
}

// disambigRecall 一次记忆读取的记录（domain + contextKey）
type disambigRecall struct {
	domain     string
	contextKey string
}

// disambigWrite 一次记忆写入的记录（domain + contextKey + value）
type disambigWrite struct {
	domain     string
	contextKey string
	value      string
}

// recordedDisambigMemory 可编程命中的消歧记忆替身：hits 按 domain+contextKey 预置命中值，
// 记录全部 Recall / Remember 调用供零调用断言
type recordedDisambigMemory struct {
	mu      sync.Mutex
	hits    map[string]string
	recalls []disambigRecall
	writes  []disambigWrite
}

func newRecordedDisambigMemory() *recordedDisambigMemory {
	return &recordedDisambigMemory{hits: make(map[string]string)}
}

func (m *recordedDisambigMemory) Recall(_ context.Context, domain, contextKey string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recalls = append(m.recalls, disambigRecall{domain: domain, contextKey: contextKey})
	value, ok := m.hits[domain+"\x1f"+contextKey]
	return value, ok
}

func (m *recordedDisambigMemory) Remember(_ context.Context, domain, contextKey, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writes = append(m.writes, disambigWrite{domain: domain, contextKey: contextKey, value: value})
	return nil
}

// newCreateByURLServiceWithMemory 同 newCreateByURLService，注入指定消歧记忆替身
func newCreateByURLServiceWithMemory(t *testing.T, mem DisambiguationMemory, getter WorkFetchProvider, listenerSvc *pluginTaskUrlListener.Service) (*Service, *fakeTaskRepo) {
	t.Helper()
	repo := newFakeTaskRepo()
	siteSvc := site.NewService(fakeSiteRepo{})
	svc := NewService(repo, fakeTransactor{}, getter, listenerSvc, siteSvc, nil, mem)
	return svc, repo
}

// siteKeyListener 带站点域声明的监听器条目（站点域求值输入）
func siteKeyListener(publicId, name, extId, siteKey string) *pluginTaskUrlListener.PluginWithExtension {
	entry := namedListener(publicId, name, extId)
	entry.SiteKey = siteKey
	return entry
}

// newSiteKeyListenerService 以生产输入形态（清单作品拉取条目声明 urlPatterns + siteKey）登记
// 监听器条目，siteKey 随条目进入派生索引供站点域求值读取
func newSiteKeyListenerService(entries ...*pluginTaskUrlListener.PluginWithExtension) *pluginTaskUrlListener.Service {
	svc := pluginTaskUrlListener.NewService(pluginTaskUrlListener.NewManager())
	for _, e := range entries {
		svc.RegisterDeclared(e.Plugin, []dto.WorkFetchDeclaration{
			{ID: e.ExtensionID, Name: e.Name.String, UrlPatterns: []string{"^http"}, SiteKey: e.SiteKey},
		})
	}
	return svc
}

// twoCandidateFixture 双候选（pub-a/ext-a 与 pub-b/ext-b，同站点域声明）的标准装配：
// 返回服务、两个处理器替身与记忆替身。fullKeyA/fullKeyB 为两个候选的全键。
func twoCandidateFixture(t *testing.T, mem DisambiguationMemory, siteKey string) (*Service, *fakePluginWorkFetcher, *fakePluginWorkFetcher, *fakeTaskRepo) {
	t.Helper()
	handlerA, handlerB := viableHandler(), viableHandler()
	svc, repo := newCreateByURLServiceWithMemory(t, mem,
		&fakeWorkFetchGetter{handlers: map[string]sdkdto.WorkFetcher{
			"pub-a/ext-a": handlerA,
			"pub-b/ext-b": handlerB,
		}},
		newSiteKeyListenerService(
			siteKeyListener("pub-a", "插件A", "ext-a", siteKey),
			siteKeyListener("pub-b", "插件B", "ext-b", siteKey),
		))
	return svc, handlerA, handlerB, repo
}

const (
	// testFullKeyA / testFullKeyB 双候选 fixture 的候选全键
	testFullKeyA = "pub-a\x00ext-a"
	testFullKeyB = "pub-b\x00ext-b"
	// testDisambigURL 双候选 fixture 的任务 URL（host 与 siteKey 声明值不同，区分两条站点域路径）
	testDisambigURL = "https://www.pixiv.net/artworks/1"
)

// TestCreateTaskByURL_MemoryHitRoutesRememberedCandidate 记忆命中：无冲突载荷，路由从记住的
// 候选起（非记住候选不被调用），且命中直路由不重写记忆。
func TestCreateTaskByURL_MemoryHitRoutesRememberedCandidate(t *testing.T) {
	mem := newRecordedDisambigMemory()
	mem.hits[entity.DomainTaskURLDisambiguation+"\x1f"+stickymemory.BuildContextKey("pixiv", []string{testFullKeyA, testFullKeyB})] = testFullKeyB
	// 字典序默认候选是 pub-a：令其一旦被调用即失败，反证路由从记住的 pub-b 起
	svc, handlerA, handlerB, repo := twoCandidateFixture(t, mem, "pixiv")
	handlerA.create = func(string) (*sdkdto.TaskCreateResult, error) {
		return nil, errors.New("非记住候选被调用")
	}

	resp, err := svc.CreateTaskByURLWithChoice(context.Background(), testDisambigURL, "", "", false)
	if err != nil {
		t.Fatalf("CreateTaskByURLWithChoice 返回错误: %v", err)
	}
	if resp.Conflict {
		t.Fatal("记忆命中不应返回冲突载荷")
	}
	if !resp.Succeed || resp.AddedQuantity != 1 {
		t.Fatalf("期望从记住候选直路由成功 Succeed=true AddedQuantity=1，得到 Succeed=%v AddedQuantity=%d Msg=%q",
			resp.Succeed, resp.AddedQuantity, resp.Msg)
	}
	if handlerB.createCalls != 1 || handlerA.createCalls != 0 || len(repo.tasks) != 1 {
		t.Fatalf("路由应从记住的 pub-b/ext-b 起（createB=%d createA=%d 落盘=%d）",
			handlerB.createCalls, handlerA.createCalls, len(repo.tasks))
	}
	if len(mem.recalls) != 1 {
		t.Fatalf("交互入口多候选未显选应查一次记忆，得到 %d 次", len(mem.recalls))
	}
	if len(mem.writes) != 0 {
		t.Fatalf("命中直路由不应重写记忆，得到 %d 次写入", len(mem.writes))
	}
}

// TestCreateTaskByURL_MemoryValueNotInCandidatesReturnsConflict 记住的候选已不在候选集
// （插件停用/卸载）：记忆视为未命中，照常返回冲突载荷交用户显选，不调用任何插件。
func TestCreateTaskByURL_MemoryValueNotInCandidatesReturnsConflict(t *testing.T) {
	mem := newRecordedDisambigMemory()
	mem.hits[entity.DomainTaskURLDisambiguation+"\x1f"+stickymemory.BuildContextKey("pixiv", []string{testFullKeyA, testFullKeyB})] = "pub-z\x00ext-z"
	svc, handlerA, handlerB, repo := twoCandidateFixture(t, mem, "pixiv")

	resp, err := svc.CreateTaskByURLWithChoice(context.Background(), testDisambigURL, "", "", false)
	if err != nil {
		t.Fatalf("CreateTaskByURLWithChoice 返回错误: %v", err)
	}
	if !resp.Conflict {
		t.Fatal("记住的候选不在场应照常返回冲突载荷")
	}
	if len(resp.ConflictCandidates) != 2 {
		t.Fatalf("期望候选清单 2 项，得到 %d 项", len(resp.ConflictCandidates))
	}
	if handlerA.createCalls != 0 || handlerB.createCalls != 0 || len(repo.tasks) != 0 {
		t.Fatalf("冲突路径不得调用任何插件（createA=%d createB=%d 落盘=%d）",
			handlerA.createCalls, handlerB.createCalls, len(repo.tasks))
	}
	if len(mem.writes) != 0 {
		t.Fatalf("冲突收口不应写记忆，得到 %d 次写入", len(mem.writes))
	}
}

// TestCreateTaskByURL_ProgrammaticEntryNeverTouchesMemory 程序化入口全程不触达记忆：
// 预置一个本应命中的记忆，行为仍与无记忆一致（按候选全键字典序路由），读取与写入均为零调用。
func TestCreateTaskByURL_ProgrammaticEntryNeverTouchesMemory(t *testing.T) {
	mem := newRecordedDisambigMemory()
	mem.hits[entity.DomainTaskURLDisambiguation+"\x1f"+stickymemory.BuildContextKey("pixiv", []string{testFullKeyA, testFullKeyB})] = testFullKeyB
	svc, handlerA, handlerB, _ := twoCandidateFixture(t, mem, "pixiv")

	resp, err := svc.CreateTaskByURL(context.Background(), testDisambigURL)
	if err != nil {
		t.Fatalf("CreateTaskByURL 返回错误: %v", err)
	}
	if !resp.Succeed || resp.Conflict {
		t.Fatalf("程序化入口应按全键字典序直路由成功，得到 Succeed=%v Conflict=%v Msg=%q",
			resp.Succeed, resp.Conflict, resp.Msg)
	}
	if handlerA.createCalls != 1 || handlerB.createCalls != 0 {
		t.Fatalf("程序化入口应恒以全键字典序命中 pub-a/ext-a，得到 createA=%d createB=%d",
			handlerA.createCalls, handlerB.createCalls)
	}
	if len(mem.recalls) != 0 || len(mem.writes) != 0 {
		t.Fatalf("程序化入口不得查写记忆（recalls=%d writes=%d）", len(mem.recalls), len(mem.writes))
	}
}

// TestCreateTaskByURL_RememberFalseWritesNothing 取消勾选（remember=false）：显选路由成功
// 也不写记忆；显选路径不查记忆。
func TestCreateTaskByURL_RememberFalseWritesNothing(t *testing.T) {
	mem := newRecordedDisambigMemory()
	svc, handlerA, handlerB, repo := twoCandidateFixture(t, mem, "pixiv")

	resp, err := svc.CreateTaskByURLWithChoice(context.Background(), testDisambigURL, "pub-b", "ext-b", false)
	if err != nil {
		t.Fatalf("CreateTaskByURLWithChoice 返回错误: %v", err)
	}
	if !resp.Succeed || handlerB.createCalls != 1 || handlerA.createCalls != 0 || len(repo.tasks) != 1 {
		t.Fatalf("显选应正常路由成功（Succeed=%v createB=%d createA=%d 落盘=%d）",
			resp.Succeed, handlerB.createCalls, handlerA.createCalls, len(repo.tasks))
	}
	if len(mem.writes) != 0 {
		t.Fatalf("remember=false 不应写记忆，得到 %d 次写入", len(mem.writes))
	}
	if len(mem.recalls) != 0 {
		t.Fatalf("显选路径不应查记忆，得到 %d 次读取", len(mem.recalls))
	}
}

// TestCreateTaskByURL_ChosenFailureWritesNoMemory 显选后路由失败（resp.Succeed=false）：
// 失败的选择不入记忆——用户下次仍会被问、可改选。
func TestCreateTaskByURL_ChosenFailureWritesNoMemory(t *testing.T) {
	mem := newRecordedDisambigMemory()
	svc, handlerA, _, _ := twoCandidateFixture(t, mem, "pixiv")
	handlerA.create = func(string) (*sdkdto.TaskCreateResult, error) {
		return nil, errors.New("connection refused")
	}

	resp, err := svc.CreateTaskByURLWithChoice(context.Background(), testDisambigURL, "pub-a", "ext-a", true)
	if err != nil {
		t.Fatalf("CreateTaskByURLWithChoice 返回错误: %v", err)
	}
	if resp.Succeed {
		t.Fatal("被选候选失败不应标记成功")
	}
	if len(mem.writes) != 0 {
		t.Fatalf("路由失败不应写记忆，得到 %d 次写入", len(mem.writes))
	}
}

// TestCreateTaskByURL_RememberSuccessWritesEncodedKey 显选 + remember=true + 路由成功：
// 写入一条记忆，键与值符合编码契约——domain 为任务 URL 消歧域；context_key 反解得站点域
// （候选未声明 siteKey 时退 URL host 兜底）与按全键字典序排序的候选组合；value 反解得显选候选。
func TestCreateTaskByURL_RememberSuccessWritesEncodedKey(t *testing.T) {
	mem := newRecordedDisambigMemory()
	// 候选不声明 siteKey（siteKeyListener 传空）：站点域走 URL host 兜底
	svc, _, handlerB, _ := twoCandidateFixture(t, mem, "")

	resp, err := svc.CreateTaskByURLWithChoice(context.Background(), testDisambigURL, "pub-b", "ext-b", true)
	if err != nil {
		t.Fatalf("CreateTaskByURLWithChoice 返回错误: %v", err)
	}
	if !resp.Succeed || handlerB.createCalls != 1 {
		t.Fatalf("显选应路由成功（Succeed=%v createB=%d Msg=%q）", resp.Succeed, handlerB.createCalls, resp.Msg)
	}
	if len(mem.writes) != 1 {
		t.Fatalf("期望恰好一次记忆写入，得到 %d 次", len(mem.writes))
	}

	w := mem.writes[0]
	if w.domain != entity.DomainTaskURLDisambiguation {
		t.Fatalf("domain 应为任务 URL 消歧域 %q，得到 %q", entity.DomainTaskURLDisambiguation, w.domain)
	}
	siteDomain, fullKeys := stickymemory.ParseContextKey(w.contextKey)
	if siteDomain != "www.pixiv.net" {
		t.Fatalf("站点域应为 URL host 兜底 www.pixiv.net，得到 %q", siteDomain)
	}
	if want := []string{testFullKeyA, testFullKeyB}; !equalStringSlices(fullKeys, want) {
		t.Fatalf("候选组合应为按全键字典序排序的双候选，得到 %v 期望 %v", fullKeys, want)
	}
	if pluginPublicId, extensionId := stickymemory.SplitCandidateFullKey(w.value); pluginPublicId != "pub-b" || extensionId != "ext-b" {
		t.Fatalf("value 应反解为显选候选 pub-b/ext-b，得到 %s/%s", pluginPublicId, extensionId)
	}
}

// equalStringSlices 逐项相等断言辅助（顺序敏感）
func equalStringSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
