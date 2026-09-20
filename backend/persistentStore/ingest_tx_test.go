package persistentStore

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/settings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// newIngestTxTestService 内存库（完整迁移，含 store_ingest_journal 表）+ 真实仓储 +
// 临时库根的 Service；预建 store/ 与 staging/ 目录骨架，全局日志静音
func newIngestTxTestService(t *testing.T) (*Service, string, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	logger.Log = zap.NewNop().Sugar()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	workDir := t.TempDir()
	for _, dir := range []string{"store/work", "staging/download"} {
		if err := os.MkdirAll(filepath.Join(workDir, dir), 0o755); err != nil {
			t.Fatalf("建目录 %s 失败: %v", dir, err)
		}
	}
	svc := NewService(NewRepository(db), nil, func() string { return workDir })
	return svc, workDir, db
}

// writeFileAt 在库根下按 relPath（正斜杠）写入内容（目录按需创建）
func writeFileAt(t *testing.T, workDir, relPath, content string) {
	t.Helper()
	abs := filepath.Join(workDir, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("建目录失败(%s): %v", relPath, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("写文件失败(%s): %v", relPath, err)
	}
}

// assertFileContent 断言库根下 relPath 文件存在且内容一致
func assertFileContent(t *testing.T, workDir, relPath, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(workDir, relPath))
	if err != nil {
		t.Fatalf("文件应存在且可读(%s): %v", relPath, err)
	}
	if string(got) != want {
		t.Errorf("文件内容不符(%s): 期望 %q, 实际 %q", relPath, want, string(got))
	}
}

// assertFileGone 断言库根下 relPath 文件不存在
func assertFileGone(t *testing.T, workDir, relPath string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(workDir, relPath)); !os.IsNotExist(err) {
		t.Errorf("文件应不存在(%s): stat err=%v", relPath, err)
	}
}

// listJournals 全量列出登记行
func listJournals(t *testing.T, svc *Service) []*domain.StoreIngestJournal {
	t.Helper()
	rows, err := svc.repo.ListIngestJournals(context.Background())
	if err != nil {
		t.Fatalf("查询登记行失败: %v", err)
	}
	return rows
}

// journalByFilePath 按落位路径取登记行（不存在返回 nil）
func journalByFilePath(t *testing.T, svc *Service, filePath string) *domain.StoreIngestJournal {
	t.Helper()
	for _, row := range listJournals(t, svc) {
		if row.FilePath == filePath {
			return row
		}
	}
	return nil
}

// insertJournal 直插登记行（恢复测试构造现场用，不经服务层校验）
func insertJournal(t *testing.T, svc *Service, filePath, stagingPath, workdir string, action domain.IngestAbortAction) {
	t.Helper()
	row := domain.NewStoreIngestJournal()
	row.FilePath = filePath
	row.StagingPath = stagingPath
	row.Workdir = workdir
	row.AbortAction = action
	if err := svc.repo.CreateIngestJournals(context.Background(), []*domain.StoreIngestJournal{row}); err != nil {
		t.Fatalf("插入登记行失败: %v", err)
	}
}

// TestPrepareIngestRejectsUndeclaredAbortAction 处置声明必填无默认：空值/未定义取值/
// 混入非法项均拒绝整批（合法项也不落行）；白名单外落位路径拒绝；合法登记字段完整落库
func TestPrepareIngestRejectsUndeclaredAbortAction(t *testing.T) {
	svc, workDir, _ := newIngestTxTestService(t)
	ctx := t.Context()

	_, err := svc.PrepareIngest(ctx, []IngestItem{{
		FilePath: "store/work/a/x.bin", StagingPath: "staging/download/1/x.bin",
	}})
	if !errors.Is(err, domain.ErrIngestAbortActionUndeclared) {
		t.Errorf("未声明处置期望 ErrIngestAbortActionUndeclared，实际 %v", err)
	}

	_, err = svc.PrepareIngest(ctx, []IngestItem{{
		FilePath: "store/work/a/x.bin", StagingPath: "staging/download/1/x.bin",
		AbortAction: domain.IngestAbortAction("bogus"),
	}})
	if !errors.Is(err, domain.ErrIngestAbortActionInvalid) {
		t.Errorf("未定义处置取值期望 ErrIngestAbortActionInvalid，实际 %v", err)
	}

	// 混入未声明项：整批拒绝，合法项不落行
	_, err = svc.PrepareIngest(ctx, []IngestItem{
		{FilePath: "store/work/a/ok.bin", StagingPath: "staging/download/1/ok.bin", AbortAction: domain.AbortActionDiscard},
		{FilePath: "store/work/a/bad.bin", StagingPath: "staging/download/1/bad.bin"},
	})
	if !errors.Is(err, domain.ErrIngestAbortActionUndeclared) {
		t.Errorf("混入未声明项期望整批拒绝，实际 %v", err)
	}
	if rows := listJournals(t, svc); len(rows) != 0 {
		t.Fatalf("整批拒绝后不应存在登记行，实际 %d 条", len(rows))
	}

	// 落位路径在 store/ 白名单外：拒绝
	if _, err := svc.PrepareIngest(ctx, []IngestItem{{
		FilePath: "backup/2026/x.bin", StagingPath: "staging/download/1/x.bin",
		AbortAction: domain.AbortActionDiscard,
	}}); err == nil {
		t.Errorf("白名单外落位路径期望拒绝，实际通过")
	}

	// 合法登记：路径规范化为正斜杠落库、库根与处置随行记录，ID 数量与入参一致
	ids, err := svc.PrepareIngest(ctx, []IngestItem{
		{FilePath: `store\work\a\one.bin`, StagingPath: `staging\download\1\one.bin`, AbortAction: domain.AbortActionReturnToStaging},
		{FilePath: "store/work/a/two.bin", StagingPath: "staging/download/1/two.bin", AbortAction: domain.AbortActionDiscard},
	})
	if err != nil {
		t.Fatalf("合法登记失败: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("期望返回 2 个登记行 ID，实际 %d", len(ids))
	}
	one := journalByFilePath(t, svc, "store/work/a/one.bin")
	if one == nil {
		t.Fatalf("登记行未按正斜杠规范化落库")
	}
	if one.StagingPath != "staging/download/1/one.bin" || one.Workdir != workDir ||
		one.AbortAction != domain.AbortActionReturnToStaging {
		t.Errorf("登记行字段不符: staging=%q workdir=%q action=%q",
			one.StagingPath, one.Workdir, one.AbortAction)
	}
	if journalByFilePath(t, svc, "store/work/a/two.bin") == nil {
		t.Errorf("第二项登记行缺失")
	}
}

// TestCommitIngestSharesTransactionWithStoreRow 判据精确性：建 store 行与删登记行同事务——
// 提交成功则登记行消失、行完整落库；业务事务回滚则两者一并撤销（登记行保留，
// 下一轮恢复能收口）
func TestCommitIngestSharesTransactionWithStoreRow(t *testing.T) {
	svc, workDir, db := newIngestTxTestService(t)
	ctx := t.Context()

	// 提交成功
	ids, err := svc.PrepareIngest(ctx, []IngestItem{{
		FilePath: "store/work/a/commit.bin", StagingPath: "staging/download/2/commit.bin",
		AbortAction: domain.AbortActionReturnToStaging,
	}})
	if err != nil {
		t.Fatalf("登记失败: %v", err)
	}
	writeFileAt(t, workDir, "staging/download/2/commit.bin", "commit-content")
	if err := svc.PlaceIngest(ctx, ids); err != nil {
		t.Fatalf("落位失败: %v", err)
	}
	var storeId int64
	err = database.WithTransaction(db, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, database.TxKey, tx)
		id, err := svc.CommitIngest(txCtx, ids[0], "store/work/a/commit.bin", "commit.bin",
			sql.NullString{}, sql.NullString{})
		if err != nil {
			return err
		}
		storeId = id
		return nil
	})
	if err != nil {
		t.Fatalf("提交入库失败: %v", err)
	}
	if rows := listJournals(t, svc); len(rows) != 0 {
		t.Errorf("提交成功后登记行应消失，实际剩 %d 条", len(rows))
	}
	row, err := svc.repo.GetByFilePath(ctx, "store/work/a/commit.bin")
	if err != nil || row == nil {
		t.Fatalf("store 行应已建: err=%v row=%v", err, row)
	}
	if row.GetID() != storeId || row.CompletedAt <= 0 {
		t.Errorf("store 行不完整: id=%d(期望 %d) completedAt=%d", row.GetID(), storeId, row.CompletedAt)
	}

	// 业务事务回滚
	ids2, err := svc.PrepareIngest(ctx, []IngestItem{{
		FilePath: "store/work/a/rollback.bin", StagingPath: "staging/download/2/rollback.bin",
		AbortAction: domain.AbortActionReturnToStaging,
	}})
	if err != nil {
		t.Fatalf("登记失败: %v", err)
	}
	writeFileAt(t, workDir, "staging/download/2/rollback.bin", "rollback-content")
	if err := svc.PlaceIngest(ctx, ids2); err != nil {
		t.Fatalf("落位失败: %v", err)
	}
	sentinel := errors.New("业务事务失败")
	err = database.WithTransaction(db, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, database.TxKey, tx)
		if _, err := svc.CommitIngest(txCtx, ids2[0], "store/work/a/rollback.bin", "rollback.bin",
			sql.NullString{}, sql.NullString{}); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("业务事务应带回滚错误，实际 %v", err)
	}
	// 回滚后登记行仍在（建行与删登记同生共死），store 行未建
	kept := journalByFilePath(t, svc, "store/work/a/rollback.bin")
	if kept == nil {
		t.Fatalf("事务回滚后登记行应保留")
	}
	if row, _ := svc.repo.GetByFilePath(ctx, "store/work/a/rollback.bin"); row != nil {
		t.Errorf("事务回滚后 store 行不应存在")
	}
	// 下一轮恢复能收口：文件在最终路径 + 声明退回 → 退回暂存 + 删登记行
	recovered, err := svc.RecoverIngest(ctx)
	if err != nil || recovered != 1 {
		t.Fatalf("恢复收口期望 1 行，实际 recovered=%d err=%v", recovered, err)
	}
	assertFileContent(t, workDir, "staging/download/2/rollback.bin", "rollback-content")
	assertFileGone(t, workDir, "store/work/a/rollback.bin")
	if rows := listJournals(t, svc); len(rows) != 0 {
		t.Errorf("恢复后登记行应收口清零，实际剩 %d 条", len(rows))
	}
}

// TestCommitIngestRequiresCallerTransaction ctx 未携带事务时显式拒绝
// （建行与删登记脱离同事务会静默失去判据精确性）
func TestCommitIngestRequiresCallerTransaction(t *testing.T) {
	svc, _, _ := newIngestTxTestService(t)
	if _, err := svc.CommitIngest(t.Context(), 1, "store/work/a/x.bin", "x.bin",
		sql.NullString{}, sql.NullString{}); !errors.Is(err, ErrCommitIngestOutsideTransaction) {
		t.Errorf("期望 ErrCommitIngestOutsideTransaction，实际 %v", err)
	}
}

// TestAbortIngestFollowsDeclaredAction 撤回按登记时声明的处置执行（声明退回即退回——
// 同身份残留被覆盖；声明丢弃即丢弃）；撤回先于落位仅删登记行；重复撤回幂等无副作用
func TestAbortIngestFollowsDeclaredAction(t *testing.T) {
	svc, workDir, _ := newIngestTxTestService(t)
	ctx := t.Context()

	// 声明退回暂存：落位后撤回 → 文件回到暂存位置，目标残留被同身份覆盖
	ids, err := svc.PrepareIngest(ctx, []IngestItem{{
		FilePath: "store/work/a/one.bin", StagingPath: "staging/download/9/one.bin",
		AbortAction: domain.AbortActionReturnToStaging,
	}})
	if err != nil {
		t.Fatalf("登记失败: %v", err)
	}
	writeFileAt(t, workDir, "staging/download/9/one.bin", "one-content")
	if err := svc.PlaceIngest(ctx, ids); err != nil {
		t.Fatalf("落位失败: %v", err)
	}
	writeFileAt(t, workDir, "staging/download/9/one.bin", "stale")
	if err := svc.AbortIngest(ctx, ids); err != nil {
		t.Fatalf("撤回失败: %v", err)
	}
	assertFileContent(t, workDir, "staging/download/9/one.bin", "one-content")
	assertFileGone(t, workDir, "store/work/a/one.bin")
	if rows := listJournals(t, svc); len(rows) != 0 {
		t.Fatalf("撤回后登记行应收口，实际剩 %d 条", len(rows))
	}
	// 幂等：重复撤回无副作用
	if err := svc.AbortIngest(ctx, ids); err != nil {
		t.Fatalf("重复撤回期望无错，实际 %v", err)
	}
	assertFileContent(t, workDir, "staging/download/9/one.bin", "one-content")

	// 声明丢弃：落位后撤回 → 文件删除（暂存已随落位消费，两处皆无）
	ids2, err := svc.PrepareIngest(ctx, []IngestItem{{
		FilePath: "store/work/a/two.bin", StagingPath: "staging/download/9/two.bin",
		AbortAction: domain.AbortActionDiscard,
	}})
	if err != nil {
		t.Fatalf("登记失败: %v", err)
	}
	writeFileAt(t, workDir, "staging/download/9/two.bin", "two-content")
	if err := svc.PlaceIngest(ctx, ids2); err != nil {
		t.Fatalf("落位失败: %v", err)
	}
	if err := svc.AbortIngest(ctx, ids2); err != nil {
		t.Fatalf("撤回失败: %v", err)
	}
	assertFileGone(t, workDir, "store/work/a/two.bin")
	assertFileGone(t, workDir, "staging/download/9/two.bin")
	if rows := listJournals(t, svc); len(rows) != 0 {
		t.Fatalf("撤回后登记行应收口，实际剩 %d 条", len(rows))
	}

	// 撤回先于落位：仅删登记行，暂存文件原样保留
	ids3, err := svc.PrepareIngest(ctx, []IngestItem{{
		FilePath: "store/work/a/three.bin", StagingPath: "staging/download/9/three.bin",
		AbortAction: domain.AbortActionReturnToStaging,
	}})
	if err != nil {
		t.Fatalf("登记失败: %v", err)
	}
	writeFileAt(t, workDir, "staging/download/9/three.bin", "kept")
	if err := svc.AbortIngest(ctx, ids3); err != nil {
		t.Fatalf("撤回失败: %v", err)
	}
	assertFileContent(t, workDir, "staging/download/9/three.bin", "kept")
	if rows := listJournals(t, svc); len(rows) != 0 {
		t.Fatalf("撤回后登记行应收口，实际剩 %d 条", len(rows))
	}
}

// TestRecoverIngestBranches 恢复规则全分支：登记行 × 文件在/不在 × 处置（退回/丢弃）
// → 退回暂存、删文件、删登记行；退回不可达兜底删文件；库根不匹配行保留；
// 无登记行 + 无 store 行 + 文件在位 → 不动；重复恢复幂等
func TestRecoverIngestBranches(t *testing.T) {
	svc, workDir, _ := newIngestTxTestService(t)
	ctx := t.Context()

	// 退回暂存——文件在最终路径
	insertJournal(t, svc, "store/work/a/return.bin", "staging/download/1/return.bin",
		workDir, domain.AbortActionReturnToStaging)
	writeFileAt(t, workDir, "store/work/a/return.bin", "return-content")

	// 丢弃——文件在最终路径
	insertJournal(t, svc, "store/work/a/discard.bin", "staging/download/1/discard.bin",
		workDir, domain.AbortActionDiscard)
	writeFileAt(t, workDir, "store/work/a/discard.bin", "discard-content")

	// 退回暂存——文件未落位（仍在暂存）：恢复不动暂存文件，仅删登记行
	insertJournal(t, svc, "store/work/a/unplaced.bin", "staging/download/1/unplaced.bin",
		workDir, domain.AbortActionReturnToStaging)
	writeFileAt(t, workDir, "staging/download/1/unplaced.bin", "staged-content")

	// 丢弃——文件未落位
	insertJournal(t, svc, "store/work/a/unplaced-discard.bin", "staging/download/1/unplaced-discard.bin",
		workDir, domain.AbortActionDiscard)

	// 退回不可达——暂存目标父路径被普通文件占据（目录建不出），兜底删最终路径文件
	insertJournal(t, svc, "store/work/a/blocked.bin", "staging/blocked/x.bin",
		workDir, domain.AbortActionReturnToStaging)
	writeFileAt(t, workDir, "store/work/a/blocked.bin", "blocked-content")
	writeFileAt(t, workDir, "staging/blocked", "occupier")

	// 库根不匹配——登记行与文件都保留，待对应库根恢复
	insertJournal(t, svc, "store/work/a/mismatch.bin", "staging/download/1/mismatch.bin",
		"Z:/elsewhere", domain.AbortActionReturnToStaging)
	writeFileAt(t, workDir, "store/work/a/mismatch.bin", "mismatch-content")

	// 无登记行 + 无 store 行 + 文件在位：非我方现场，恢复不动
	writeFileAt(t, workDir, "store/work/a/foreign.bin", "foreign-content")

	recovered, err := svc.RecoverIngest(ctx)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if recovered != 5 {
		t.Errorf("期望收口 5 行（不匹配行不计），实际 %d", recovered)
	}

	assertFileContent(t, workDir, "staging/download/1/return.bin", "return-content")
	assertFileGone(t, workDir, "store/work/a/return.bin")
	assertFileGone(t, workDir, "store/work/a/discard.bin")
	assertFileContent(t, workDir, "staging/download/1/unplaced.bin", "staged-content")
	assertFileGone(t, workDir, "store/work/a/blocked.bin")
	assertFileContent(t, workDir, "store/work/a/mismatch.bin", "mismatch-content")
	assertFileContent(t, workDir, "store/work/a/foreign.bin", "foreign-content")

	rows := listJournals(t, svc)
	if len(rows) != 1 || rows[0].FilePath != "store/work/a/mismatch.bin" {
		t.Fatalf("库根不匹配行应唯一保留，实际 %d 条", len(rows))
	}

	// 幂等：重复恢复收口数为 0，保留行与在位文件无副作用
	recovered2, err := svc.RecoverIngest(ctx)
	if err != nil {
		t.Fatalf("重复恢复失败: %v", err)
	}
	if recovered2 != 0 {
		t.Errorf("重复恢复期望收口 0 行，实际 %d", recovered2)
	}
	assertFileContent(t, workDir, "store/work/a/mismatch.bin", "mismatch-content")
	if rows := listJournals(t, svc); len(rows) != 1 {
		t.Errorf("重复恢复后不匹配行应仍在，实际剩 %d 条", len(rows))
	}
}

// TestIngestEntriesShortCircuitOnUnconfiguredWorkDir 工作目录未配置（GetWorkDir 空串）：
// 启动恢复属启动期入口短路返回零值；登记/落位/撤回属请求期入口返回 ErrWorkDirNotConfigured
// （判定均先于仓储与磁盘访问，nil 仓储不可达）
func TestIngestEntriesShortCircuitOnUnconfiguredWorkDir(t *testing.T) {
	logger.Log = zap.NewNop().Sugar()
	s := NewService(nil, nil, func() string { return "" })
	ctx := t.Context()

	if n, err := s.RecoverIngest(ctx); err != nil || n != 0 {
		t.Errorf("未配置时 RecoverIngest 期望短路返回 (0,nil)，实际 (%d,%v)", n, err)
	}

	assertRefused := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, settings.ErrWorkDirNotConfigured) {
			t.Errorf("%s 期望返回 ErrWorkDirNotConfigured，实际 %v", name, err)
		}
	}
	_, err := s.PrepareIngest(ctx, []IngestItem{{
		FilePath: "store/work/a/x.bin", StagingPath: "staging/download/1/x.bin",
		AbortAction: domain.AbortActionDiscard,
	}})
	assertRefused("PrepareIngest", err)
	assertRefused("PlaceIngest", s.PlaceIngest(ctx, []int64{1}))
	assertRefused("AbortIngest", s.AbortIngest(ctx, []int64{1}))
}
