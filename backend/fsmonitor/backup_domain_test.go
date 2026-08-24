package fsmonitor

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/storeRegistry"

	"go.uber.org/zap"
)

// 关联/修复路径记日志，测试进程无 logger.Init——注入 Nop 防全局 logger.Log 为 nil
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	os.Exit(m.Run())
}

// fakeBackupReader 清单行读取替身（按路径精确/前缀/全量）
type fakeBackupReader struct {
	rows []BackupRecord
}

func (f *fakeBackupReader) GetByFilePath(ctx context.Context, filePath string) (*BackupRecord, error) {
	for i := range f.rows {
		if f.rows[i].FilePath == filePath {
			rec := f.rows[i]
			return &rec, nil
		}
	}
	return nil, nil
}

func (f *fakeBackupReader) ListByPathPrefix(ctx context.Context, prefix string) ([]BackupRecord, error) {
	result := make([]BackupRecord, 0)
	for _, r := range f.rows {
		if len(r.FilePath) > len(prefix) && r.FilePath[:len(prefix)] == prefix && r.FilePath[len(prefix)] == '/' {
			result = append(result, r)
		}
	}
	return result, nil
}

func (f *fakeBackupReader) ListAllInWorkDir(ctx context.Context) ([]BackupRecord, error) {
	return f.rows, nil
}

// fakeBackupRepairer 修复替身（记录删行/改路径调用）
type fakeBackupRepairer struct {
	mu         sync.Mutex
	deletedIds []int64
	updated    map[int64]string
}

func (f *fakeBackupRepairer) DeleteRow(ctx context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletedIds = append(f.deletedIds, id)
	return nil
}

func (f *fakeBackupRepairer) UpdateFilePath(ctx context.Context, id int64, newFilePath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updated == nil {
		f.updated = make(map[int64]string)
	}
	f.updated[id] = newFilePath
	return nil
}

// fakeRefCleaner 引用清列替身（记录调用 ID 集）
type fakeRefCleaner struct {
	mu  sync.Mutex
	ids []int64
}

func (f *fakeRefCleaner) ClearBackupRefs(ctx context.Context, backupIds []int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = append(f.ids, backupIds...)
	return nil
}

// noopStoreRepairer store 域修复占位（Service 构造修复层的门槛条件，本组测试不触达其方法）
type noopStoreRepairer struct{}

func (noopStoreRepairer) UpdateFilePath(ctx context.Context, id int64, newFilePath string) error {
	return nil
}
func (noopStoreRepairer) MarkInvalid(ctx context.Context, id int64) error { return nil }
func (noopStoreRepairer) RenameDirectoryPrefix(ctx context.Context, oldPrefix, newPrefix string) (int64, error) {
	return 0, nil
}

// TestBackupWatcherFileRemove 文件 Remove：清单行命中产 backup 域 Delete；无行不报（外部无关文件）
func TestBackupWatcherFileRemove(t *testing.T) {
	reader := &fakeBackupReader{rows: []BackupRecord{{ID: 7, FilePath: "backup/2026/08/23/a.mp4"}}}
	w := newBackupWatcher(reader, func() string { return t.TempDir() })

	items := w.Process(context.Background(), FileChange{Kind: ChangeRemove, Path: "backup/2026/08/23/a.mp4", DetectedAt: 100})
	if len(items) != 1 {
		t.Fatalf("命中清单行的 Remove 应产 1 条，实际 %d", len(items))
	}
	sc := items[0]
	if sc.Domain != DomainBackup || sc.Kind != SemanticDelete || sc.BackupID != 7 || sc.FromPath != "backup/2026/08/23/a.mp4" {
		t.Fatalf("语义变更字段不符: %+v", sc)
	}

	items = w.Process(context.Background(), FileChange{Kind: ChangeRemove, Path: "backup/2026/08/23/unknown.mp4"})
	if len(items) != 0 {
		t.Fatalf("无清单行的外部文件消失不应报告，实际 %d 条", len(items))
	}
}

// TestBackupWatcherDirRemoveExpansion 目录 Remove：前缀圈行后逐行 stat 复核——
// 文件缺失的行产出 Delete，文件在位的行（目录消失事件与文件移回并发等瞬态）跳过
func TestBackupWatcherDirRemoveExpansion(t *testing.T) {
	workDir := t.TempDir()
	rows := []BackupRecord{
		{ID: 1, FilePath: "backup/2026/08/23/gone.mp4"},
		{ID: 2, FilePath: "backup/2026/08/23/still.mp4"},
	}
	// still.mp4 在磁盘（模拟瞬态：目录消失事件到达时文件实际在位）
	if err := os.MkdirAll(filepath.Join(workDir, "backup/2026/08/23"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "backup/2026/08/23/still.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := newBackupWatcher(&fakeBackupReader{rows: rows}, func() string { return workDir })

	items := w.Process(context.Background(), FileChange{Kind: ChangeRemove, Path: "backup/2026/08/23", IsDir: true, DetectedAt: 100})
	if len(items) != 1 {
		t.Fatalf("目录消失应仅产文件确实缺失的 1 条，实际 %d", len(items))
	}
	if items[0].BackupID != 1 || items[0].Kind != SemanticDelete {
		t.Fatalf("应报文件缺失行 ID=1，实际 %+v", items[0])
	}
}

// TestBackupWatcherMovePairing ChangeMove（USN 配对）：新路径在 backup 子树内=Move（sync 可行）；
// 移出 backup 子树=Delete（文件被取走，行不跟随外部路径）；无行/目录腿不报
func TestBackupWatcherMovePairing(t *testing.T) {
	reader := &fakeBackupReader{rows: []BackupRecord{{ID: 9, FilePath: "backup/2026/a.mp4"}}}
	w := newBackupWatcher(reader, func() string { return t.TempDir() })
	ctx := context.Background()

	items := w.Process(ctx, FileChange{Kind: ChangeMove, Path: "backup/2026/a.mp4", ToPath: "backup/2027/a.mp4", DetectedAt: 100})
	if len(items) != 1 || items[0].Kind != SemanticMove || items[0].ToPath != "backup/2027/a.mp4" || items[0].BackupID != 9 {
		t.Fatalf("子树内移动应产 Move(ID=9)，实际 %+v", items)
	}

	items = w.Process(ctx, FileChange{Kind: ChangeMove, Path: "backup/2026/a.mp4", ToPath: "store/resource/a.mp4", DetectedAt: 100})
	if len(items) != 1 || items[0].Kind != SemanticDelete {
		t.Fatalf("移出 backup 子树应产 Delete（保管语义不成立），实际 %+v", items)
	}

	if items := w.Process(ctx, FileChange{Kind: ChangeMove, Path: "backup/2026/none.mp4", ToPath: "backup/2026/b.mp4"}); len(items) != 0 {
		t.Fatalf("无清单行的移动不应报告，实际 %d 条", len(items))
	}
	if items := w.Process(ctx, FileChange{Kind: ChangeMove, Path: "backup/2026", ToPath: "backup/2027", IsDir: true}); len(items) != 0 {
		t.Fatalf("目录移动腿不应由 backup 域关联处理，实际 %d 条", len(items))
	}
}

// TestBackupScanSection 离线对账 backup 段：清单行文件缺失产 BackupMissing；
// 磁盘孤儿文件（无清单行）不产出；清单行全在位时零产出
func TestBackupScanSection(t *testing.T) {
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "backup/2026/08/23"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 孤儿文件（磁盘有、清单行无）
	if err := os.WriteFile(filepath.Join(workDir, "backup/2026/08/23/orphan.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 在位清单行文件
	if err := os.WriteFile(filepath.Join(workDir, "backup/2026/08/23/present.mp4"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := []BackupRecord{
		{ID: 1, FilePath: "backup/2026/08/23/missing.mp4"}, // 文件不在磁盘
		{ID: 2, FilePath: "backup/2026/08/23/present.mp4"}, // 在位
	}
	s := &scanner{backupReader: &fakeBackupReader{rows: rows}, workDirGetter: func() string { return workDir }}

	diff := DiffSet{}
	if err := s.scanBackup(context.Background(), workDir, &diff); err != nil {
		t.Fatalf("backup 段对账失败: %v", err)
	}
	if len(diff.BackupMissing) != 1 || diff.BackupMissing[0].BackupID != 1 {
		t.Fatalf("应仅报文件缺失行 ID=1，实际 %+v", diff.BackupMissing)
	}
	if len(diff.Untracked) != 0 {
		t.Fatalf("backup 段不应产出 Untracked（孤儿文件不报），实际 %+v", diff.Untracked)
	}
}

// TestBackupAckRepair 确认联动：Delete.ack=删清单行+清引用；Delete.restore 拒绝（文件已失无从复原）；
// Move.sync=行路径跟随；Move.restore=文件移回旧行路径
func TestBackupAckRepair(t *testing.T) {
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "backup/2026"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "backup/2026/moved.mp4"), []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	repairer := &fakeBackupRepairer{}
	cleaner := &fakeRefCleaner{}
	rm := NewRepairManager(noopStoreRepairer{}, repairer, cleaner, func() string { return workDir })
	ctx := context.Background()

	// Delete.ack
	id := rm.Enqueue(&SemanticChange{Domain: DomainBackup, Kind: SemanticDelete, FromPath: "backup/2026/a.mp4", BackupID: 7})
	if id == 0 {
		t.Fatal("backup 域 Delete 应入队")
	}
	if err := rm.Confirm(ctx, id, ActionAck); err != nil {
		t.Fatalf("ack 确认失败: %v", err)
	}
	if len(repairer.deletedIds) != 1 || repairer.deletedIds[0] != 7 {
		t.Fatalf("应删除清单行 7，实际 %v", repairer.deletedIds)
	}
	if len(cleaner.ids) != 1 || cleaner.ids[0] != 7 {
		t.Fatalf("应清除行 7 的引用，实际 %v", cleaner.ids)
	}

	// Delete.restore 拒绝
	id = rm.Enqueue(&SemanticChange{Domain: DomainBackup, Kind: SemanticDelete, BackupID: 8})
	if err := rm.Confirm(ctx, id, ActionRestore); err == nil {
		t.Fatal("备份缺失不支持 restore（文件已失无从复原），应报错")
	}

	// Move.sync
	id = rm.Enqueue(&SemanticChange{Domain: DomainBackup, Kind: SemanticMove, FromPath: "backup/2026/old.mp4", ToPath: "backup/2026/moved.mp4", BackupID: 9})
	if err := rm.Confirm(ctx, id, ActionSync); err != nil {
		t.Fatalf("sync 确认失败: %v", err)
	}
	if repairer.updated[9] != "backup/2026/moved.mp4" {
		t.Fatalf("清单行 9 路径应同步到新位置，实际 %v", repairer.updated)
	}

	// Move.restore：文件从新路径移回旧行路径
	id = rm.Enqueue(&SemanticChange{Domain: DomainBackup, Kind: SemanticMove, FromPath: "backup/2026/old.mp4", ToPath: "backup/2026/moved.mp4", BackupID: 9})
	if err := rm.Confirm(ctx, id, ActionRestore); err != nil {
		t.Fatalf("restore 确认失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "backup/2026/old.mp4")); err != nil {
		t.Fatalf("文件应已移回旧路径: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "backup/2026/moved.mp4")); !os.IsNotExist(err) {
		t.Fatal("新路径文件应已不存在")
	}
}

// TestBackupEventRoutingAndSuppression handleFileChange 域路由：backup 子树 Remove 进关联层产出待修复条目；
// 抑制命中的事件（主程序自写 backup/ 文件）丢弃不入队
func TestBackupEventRoutingAndSuppression(t *testing.T) {
	reader := &fakeBackupReader{rows: []BackupRecord{{ID: 3, FilePath: "backup/2026/route.mp4"}}}
	deps := &Deps{
		BackupReader:   reader,
		BackupRepairer: &fakeBackupRepairer{},
		StoreRepairer:  noopStoreRepairer{},
	}
	svc := NewService(deps, func() string { return t.TempDir() }, func() EventEmitter { return nil })
	ctx := context.Background()

	svc.handleFileChange(ctx, FileChange{Kind: ChangeRemove, Path: "backup/2026/route.mp4", DetectedAt: 100})
	pending := svc.ListPendingChanges()
	if len(pending) != 1 {
		t.Fatalf("backup 子树文件消失应入队 1 条，实际 %d", len(pending))
	}
	if pending[0].Domain != DomainBackup || pending[0].BackupID != 3 {
		t.Fatalf("待修复条目应属 backup 域（清单行 3），实际 %+v", pending[0])
	}

	// 抑制命中（主程序自写登记）：同路径另一清单行的事件被丢弃
	reader.rows = append(reader.rows, BackupRecord{ID: 4, FilePath: "backup/2026/suppressed.mp4"})
	storeRegistry.Suppress("backup/2026/suppressed.mp4")
	svc.handleFileChange(ctx, FileChange{Kind: ChangeRemove, Path: "backup/2026/suppressed.mp4"})
	if got := len(svc.ListPendingChanges()); got != 1 {
		t.Fatalf("抑制命中的 backup 事件应丢弃，待修复应仍为 1 条，实际 %d", got)
	}
}

// recordingEmitter 记录派发事件的发射器替身
type recordingEmitter struct {
	mu     sync.Mutex
	events []string
}

func (r *recordingEmitter) Emit(eventName string, data ...any) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, eventName)
	return true
}

// TestRenameOldLegRouting 改名旧名腿（fsnotify Rename Op 转发的 ChangeRemove+FromRename）路由：
// backup 域照常消费（命中清单行报缺失——同目录改名的唯一运行时信号）；store 域跳过
// （其改名检出走 Create 新名指纹配对，旧名腿进关联会与 Move 双报告）
func TestRenameOldLegRouting(t *testing.T) {
	emitter := &recordingEmitter{}
	deps := &Deps{
		Fingerprinter: stubFingerprinter{digest: "fp"},
		StoreReader: mockStoreReader{byPath: map[string]StoreRecord{
			"store/resource/A/旧.jpg": {ID: 42, FilePath: "store/resource/A/旧.jpg", ContentFingerprint: "fp"},
		}},
		StoreRepairer:  noopStoreRepairer{},
		BackupReader:   &fakeBackupReader{rows: []BackupRecord{{ID: 7, FilePath: "backup/2026/a.mp4"}}},
		BackupRepairer: &fakeBackupRepairer{},
	}
	svc := NewService(deps, func() string { return t.TempDir() }, func() EventEmitter { return emitter })
	ctx := context.Background()

	// backup 域：旧名腿消费
	svc.handleFileChange(ctx, FileChange{Kind: ChangeRemove, Path: "backup/2026/a.mp4", FromRename: true, DetectedAt: 100})
	if len(emitter.events) != 1 || len(svc.ListPendingChanges()) != 1 {
		t.Fatalf("backup 域旧名腿应报缺失并入队，emits=%d pending=%d", len(emitter.events), len(svc.ListPendingChanges()))
	}

	// store 域：旧名腿跳过（记录存在也不报）
	svc.handleFileChange(ctx, FileChange{Kind: ChangeRemove, Path: "store/resource/A/旧.jpg", FromRename: true, DetectedAt: 101})
	if len(emitter.events) != 1 {
		t.Fatalf("store 域旧名腿应跳过，emits=%d", len(emitter.events))
	}

	// store 域：普通 Remove 不受影响（真删除照报）
	svc.handleFileChange(ctx, FileChange{Kind: ChangeRemove, Path: "store/resource/A/旧.jpg", DetectedAt: 102})
	if len(emitter.events) != 2 {
		t.Fatalf("store 域普通删除应照报，emits=%d", len(emitter.events))
	}
}

// TestDedupKeyDomainDiscriminates 去重键含域维度——两域同路径同类型的变更不互撞
func TestDedupKeyDomainDiscriminates(t *testing.T) {
	storeKey := makeDedupKey(&SemanticChange{Kind: SemanticDelete, FromPath: "backup/2026/a.mp4", StoreID: 5})
	backupKey := makeDedupKey(&SemanticChange{Domain: DomainBackup, Kind: SemanticDelete, FromPath: "backup/2026/a.mp4", BackupID: 5})
	if storeKey == backupKey {
		t.Fatal("两域变更身份键应互异，否则 USN/对账段跨域去重误撞")
	}
}
