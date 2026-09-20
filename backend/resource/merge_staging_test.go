package resource

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/staging"
	"github.com/library-squirrel/backend/storeRegistry"

	"gorm.io/gorm"

	"github.com/lvfeng-z/library-squirrel-sdk/storepath"
)

// ==== 合并产物暂存作用域与提交点入库事务测试 ====
// ==== （产物写 staging/merge/{铸造键}/，提交点走登记→落位→事务内建行+删登记行）====

const mergeStageSiteKey = "bilibili"

const mergeStageSiteWorkId = "BV1xx411c7mD_4538792"

// mergeStageMerger 合并能力桩：校验产物落在带自证描述的暂存作用域内，写标记内容并记录输出路径。
// scope.json 缺失时返回错误，令合并以失败收口暴露断言结果
type mergeStageMerger struct{ outPath string }

func (m *mergeStageMerger) MergeRemux(ctx context.Context, videoPath, audioPath, outPath string, onProgress func(percent int)) error {
	m.outPath = outPath
	if _, err := os.Stat(filepath.Join(filepath.Dir(outPath), "scope.json")); err != nil {
		return fmt.Errorf("产物不在带自证描述的暂存作用域内: %w", err)
	}
	return os.WriteFile(outPath, []byte("merged-product"), 0o644)
}

// mergeStageStoreOps store 行查询/路径/软删桩：返回预置行与工作目录下的轨道路径
type mergeStageStoreOps struct {
	workDir string
}

func (o *mergeStageStoreOps) GetById(ctx context.Context, id int64) (*domain.PersistentStore, error) {
	ps := domain.NewPersistentStore()
	ps.SetID(id)
	ps.FilePath = sql.NullString{String: "store/work/track/videoTrack_000.mp4", Valid: true}
	return ps, nil
}
func (o *mergeStageStoreOps) GetAbsPath(store *domain.PersistentStore) string {
	return filepath.Join(o.workDir, "store/work/track/videoTrack_000.mp4")
}
func (o *mergeStageStoreOps) DeleteWithBackup(ctx context.Context, id int64) (int64, error) {
	return 0, nil
}

// mergeStageJournal 入库登记行的桩内形态（登记内容 + 撤回处置声明）
type mergeStageJournal struct {
	filePath    string
	stagingPath string
	action      domain.IngestAbortAction
}

// mergeStageCommit 建行记录（含建行时 ctx 是否携带事务）
type mergeStageCommit struct {
	relPath  string
	fileName string
	inTx     bool
}

// mergeStageAbort 撤回时点记录：登记声明与产物是否已在最终路径。记录存在本身即证明撤回
// 入口时该登记行仍在（事务回滚已恢复其删除）
type mergeStageAbort struct {
	action      domain.IngestAbortAction
	fileAtFinal bool
}

// mergeStageIngestSnapshot 事务失败回滚快照（登记行与建行记录）
type mergeStageIngestSnapshot struct {
	nextJournalId int64
	journals      map[int64]mergeStageJournal
	commits       []mergeStageCommit
}

// mergeStageIngestor 提交点入库事务桩：在真实文件系统上按与生产一致的语义实现四调用
// （登记行内存持有；落位=暂存同卷 rename，操作抑制先于 rename 登记并随方法尾宽限释放；
// 提交=校验产物已就位与 ctx 事务标记、记录建行并删登记行；撤回=按登记声明退回暂存/丢弃
// 文件并收口登记行）。四调用序列、登记内容、建行/撤回记录供断言；storeId 为建行返回的
// 行 ID（可指向未播种的行，供挂载外键失败注入事务失败）
type mergeStageIngestor struct {
	workDir       string
	storeId       int64
	seq           []string // 四调用序列（prepare/place/commit/abort），断言提交点编排顺序
	nextJournalId int64
	journals      map[int64]mergeStageJournal
	preps         []persistentStore.IngestItem
	commits       []mergeStageCommit
	aborts        []mergeStageAbort
}

func newMergeStageIngestor(workDir string, storeId int64) *mergeStageIngestor {
	return &mergeStageIngestor{workDir: workDir, storeId: storeId, journals: map[int64]mergeStageJournal{}}
}

func (s *mergeStageIngestor) PrepareIngest(ctx context.Context, items []persistentStore.IngestItem) ([]int64, error) {
	s.seq = append(s.seq, "prepare")
	ids := make([]int64, 0, len(items))
	for _, it := range items {
		s.nextJournalId++
		s.journals[s.nextJournalId] = mergeStageJournal{
			filePath:    it.FilePath,
			stagingPath: it.StagingPath,
			action:      it.AbortAction,
		}
		s.preps = append(s.preps, it)
		ids = append(ids, s.nextJournalId)
	}
	return ids, nil
}

func (s *mergeStageIngestor) PlaceIngest(ctx context.Context, ids []int64) error {
	s.seq = append(s.seq, "place")
	suppressed := make([]string, 0, len(ids))
	defer func() {
		for _, key := range suppressed {
			storeRegistry.Release(key)
		}
	}()
	for _, id := range ids {
		j, ok := s.journals[id]
		if !ok {
			continue
		}
		// 抑制先于 rename（与生产落位同步语义）
		storeRegistry.Suppress(j.filePath)
		suppressed = append(suppressed, j.filePath)
		finalAbs := filepath.Join(s.workDir, j.filePath)
		if err := os.MkdirAll(filepath.Dir(finalAbs), 0o755); err != nil {
			return err
		}
		if err := os.Rename(filepath.Join(s.workDir, j.stagingPath), finalAbs); err != nil {
			return err
		}
	}
	return nil
}

func (s *mergeStageIngestor) CommitIngest(ctx context.Context, intentId int64, relPath string, fileName string, expectedSha, actualSha sql.NullString) (int64, error) {
	s.seq = append(s.seq, "commit")
	// 行在文件之后建：建行时产物须已 rename 就位
	if _, serr := os.Stat(filepath.Join(s.workDir, relPath)); serr != nil {
		return 0, fmt.Errorf("建行时产物未就位: %w", serr)
	}
	_, inTx := ctx.Value(database.TxKey).(*gorm.DB)
	s.commits = append(s.commits, mergeStageCommit{relPath: relPath, fileName: fileName, inTx: inTx})
	delete(s.journals, intentId)
	return s.storeId, nil
}

func (s *mergeStageIngestor) AbortIngest(ctx context.Context, ids []int64) error {
	s.seq = append(s.seq, "abort")
	suppressed := make([]string, 0, len(ids))
	defer func() {
		for _, key := range suppressed {
			storeRegistry.Release(key)
		}
	}()
	for _, id := range ids {
		j, ok := s.journals[id]
		if !ok {
			continue
		}
		finalAbs := filepath.Join(s.workDir, j.filePath)
		_, statErr := os.Stat(finalAbs)
		s.aborts = append(s.aborts, mergeStageAbort{action: j.action, fileAtFinal: statErr == nil})
		if statErr != nil {
			// 未落位（撤回先于落位）：仅收口登记行
			delete(s.journals, id)
			continue
		}
		storeRegistry.Suppress(j.filePath)
		suppressed = append(suppressed, j.filePath)
		switch j.action {
		case domain.AbortActionDiscard:
			if err := os.Remove(finalAbs); err != nil {
				return err
			}
		case domain.AbortActionReturnToStaging:
			stagingAbs := filepath.Join(s.workDir, j.stagingPath)
			if err := os.MkdirAll(filepath.Dir(stagingAbs), 0o755); err != nil {
				return err
			}
			if err := os.Remove(stagingAbs); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.Rename(finalAbs, stagingAbs); err != nil {
				return err
			}
		}
		delete(s.journals, id)
	}
	return nil
}

func (s *mergeStageIngestor) snapshot() mergeStageIngestSnapshot {
	journals := make(map[int64]mergeStageJournal, len(s.journals))
	for k, v := range s.journals {
		journals[k] = v
	}
	commits := make([]mergeStageCommit, len(s.commits))
	copy(commits, s.commits)
	return mergeStageIngestSnapshot{nextJournalId: s.nextJournalId, journals: journals, commits: commits}
}

func (s *mergeStageIngestor) restore(snap mergeStageIngestSnapshot) {
	s.nextJournalId = snap.nextJournalId
	s.journals = snap.journals
	s.commits = snap.commits
}

// mergeStageTransactor 事务桩：同步执行并向 ctx 注入事务标记（database.TxKey，模拟生产事务
// 把连接放进 ctx 供 CommitIngest 感知）；携带入库桩时失败回滚恢复其登记行与建行记录
// （模拟真实事务的原子性撤销——建行与删登记行同生共死，回滚后两者一并复原）
type mergeStageTransactor struct {
	db       *gorm.DB
	ingestor *mergeStageIngestor
}

func (t mergeStageTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if t.db != nil {
		ctx = context.WithValue(ctx, database.TxKey, t.db)
	}
	if t.ingestor == nil {
		return fn(ctx)
	}
	snap := t.ingestor.snapshot()
	if err := fn(ctx); err != nil {
		t.ingestor.restore(snap)
		return err
	}
	return nil
}

// mergeStageEmitter 终态记账桩
type mergeStageEmitter struct {
	success    bool
	mergedPsId int64
	errMsg     string
}

func (e *mergeStageEmitter) PushProgress(resourceId int64, percent int) {}
func (e *mergeStageEmitter) PushComplete(resourceId int64, success bool, mergedStoreId int64, errMsg string) {
	e.success = success
	e.mergedPsId = mergedStoreId
	e.errMsg = errMsg
}

// mergeStageSettings 固定 keep 策略（不触达原轨道置换链）
type mergeStageSettings struct{}

func (mergeStageSettings) GetMergeStrategy() string { return settings.MergeStrategyKeep }

// snapshotMergeTempResidue 系统临时目录内合并产物前缀残留快照（合并不应再写系统临时目录）
func snapshotMergeTempResidue(t *testing.T) map[string]struct{} {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return map[string]struct{}{}
	}
	set := make(map[string]struct{})
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "ls-merge-") {
			set[e.Name()] = struct{}{}
		}
	}
	return set
}

// mergeStageFinalPath 期望的产物最终落位相对路径（与下载侧同口径派生）
func mergeStageFinalPath(t *testing.T) string {
	t.Helper()
	dirName, err := storepath.WorkDirName(mergeStageSiteKey, mergeStageSiteWorkId)
	if err != nil {
		t.Fatalf("派生作品目录名失败: %v", err)
	}
	return path.Join("store", "work",
		storepath.BucketSegment(mergeStageSiteKey, mergeStageSiteWorkId),
		dirName,
		"videoMain_000.mp4")
}

// seedMergeStageBase DB 种子：作品 500（resource.work_id 外键防线）+ 资源 700（挂载目标）
func seedMergeStageBase(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec("INSERT INTO work (id, create_time, update_time, deleted_at) VALUES (500, 0, 0, 0)").Error; err != nil {
		t.Fatalf("建作品种子失败: %v", err)
	}
	res := domain.NewResource()
	res.ID = 700
	res.WorkID = 500
	res.ResourceType = "video"
	if err := db.Create(res).Error; err != nil {
		t.Fatalf("插 resource 失败: %v", err)
	}
}

// TestRunMergeIngestsThroughTxAPI 合并提交点走入库事务 API 的全序列断言（锚6 成功轨）：
// 产物写进 {workDir}/staging/merge/{铸造键}/ 作用域（目录内含自证 scope.json）、系统临时目录
// 零写入；提交序列为登记（处置声明恒丢弃、暂存路径指向作用域内产物）→ 落位（建行时文件已
// 就位）→ 事务内建行（ctx 携带事务标记）+ 挂 resource_store(videoMain, derived)；提交成功后
// 登记行收口，流程退出作用域显式回收
func TestRunMergeIngestsThroughTxAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	ctx := context.Background()
	workDir := t.TempDir()

	// DB 种子：作品/资源 + persistent_store 行（resource_store.store_id 外键防线，
	// 行 ID 由建行桩返回）
	seedMergeStageBase(t, db)
	psRow := domain.NewPersistentStore()
	psRow.FilePath = sql.NullString{String: "store/work/track/videoTrack_000.mp4", Valid: true}
	psRow.CompletedAt = 1
	if err := db.Create(psRow).Error; err != nil {
		t.Fatalf("插 persistent_store 失败: %v", err)
	}

	merger := &mergeStageMerger{}
	ops := &mergeStageStoreOps{workDir: workDir}
	ingestor := newMergeStageIngestor(workDir, psRow.GetID())
	emitter := &mergeStageEmitter{}
	svc := NewMergeService(
		NewResourceStoreRepository(db),
		mergeNamingResource{},
		mergeNamingWork{
			siteId:     sql.NullInt64{Int64: 11, Valid: true},
			siteWorkId: sql.NullString{String: mergeStageSiteWorkId, Valid: true},
		},
		mergeNamingSite{siteKey: mergeStageSiteKey},
		merger,
		ops,
		ingestor,
		mergeStageSettings{},
		replaceWorkDir{workDir: workDir},
		mergeStageTransactor{db: db, ingestor: ingestor},
		noopReplaceRecompute{},
		emitter,
		nil, // workLock：keep 策略不触达置换链
	)

	videoRS := domain.NewResourceStore()
	videoRS.StoreID = 901
	audioRS := domain.NewResourceStore()
	audioRS.StoreID = 902
	tempBefore := snapshotMergeTempResidue(t)
	svc.runMerge(ctx, 700, videoRS, audioRS)

	if !emitter.success {
		t.Fatalf("合并应成功，实际失败: %s", emitter.errMsg)
	}
	if emitter.mergedPsId != psRow.GetID() {
		t.Errorf("完成事件 storeId = %d, want %d", emitter.mergedPsId, psRow.GetID())
	}

	// 提交序列：登记 → 落位 → 事务内建行（无撤回）
	if got, want := ingestor.seq, []string{"prepare", "place", "commit"}; !reflect.DeepEqual(got, want) {
		t.Errorf("提交点调用序列 = %v, want %v", got, want)
	}

	// 登记内容：单文件、最终路径与下载侧同口径、暂存路径指向 merge 作用域内产物、处置声明丢弃
	if len(ingestor.preps) != 1 {
		t.Fatalf("登记项数 = %d, want 1", len(ingestor.preps))
	}
	prep := ingestor.preps[0]
	if prep.FilePath != mergeStageFinalPath(t) {
		t.Errorf("登记最终路径 = %q, want %q", prep.FilePath, mergeStageFinalPath(t))
	}
	if !strings.HasPrefix(prep.StagingPath, "staging/merge/") || path.Base(prep.StagingPath) != "videoMain_000.mp4" {
		t.Errorf("登记暂存路径 = %q, want staging/merge/ 作用域内 videoMain_000.mp4", prep.StagingPath)
	}
	if prep.AbortAction != domain.AbortActionDiscard {
		t.Errorf("登记撤回处置 = %q, want %q（derived 一次性产物恒丢弃）", prep.AbortAction, domain.AbortActionDiscard)
	}

	// 建行：路径/文件名与登记一致、在事务内调用（ctx 携带事务标记）、建行时产物已就位
	if len(ingestor.commits) != 1 {
		t.Fatalf("建行次数 = %d, want 1", len(ingestor.commits))
	}
	commit := ingestor.commits[0]
	if commit.relPath != mergeStageFinalPath(t) || commit.fileName != "videoMain_000.mp4" {
		t.Errorf("建行 relPath=%q fileName=%q, want %q / videoMain_000.mp4",
			commit.relPath, commit.fileName, mergeStageFinalPath(t))
	}
	if !commit.inTx {
		t.Errorf("建行应发生在事务内（ctx 携带 database.TxKey）")
	}

	// 登记行收口：提交成功后无未收口登记行
	if len(ingestor.journals) != 0 {
		t.Errorf("提交成功后登记行应收口，剩余 %d 行", len(ingestor.journals))
	}

	// 暂存位置：产物输出路径位于 {workDir}/staging/merge/{作用域键}/ 下
	mergeRoot := filepath.Join(workDir, staging.RootName, string(staging.OwnerMerge))
	if got := filepath.Dir(filepath.Dir(merger.outPath)); got != mergeRoot {
		t.Errorf("产物暂存父目录 = %q, want %q", got, mergeRoot)
	}

	// 落位：产物文件在最终 store 路径且内容完整
	finalAbs := filepath.Join(workDir, mergeStageFinalPath(t))
	data, rerr := os.ReadFile(finalAbs)
	if rerr != nil {
		t.Fatalf("产物应落位到最终路径 %s: %v", finalAbs, rerr)
	}
	if string(data) != "merged-product" {
		t.Errorf("产物内容 = %q, want %q", string(data), "merged-product")
	}

	// 挂载成行：resource_store(videoMain, derived) 指向建行返回的 store
	rsRepo := NewResourceStoreRepository(db)
	mounted, merr := rsRepo.GetByType(ctx, 700, domain.StoreTypeVideoMain)
	if merr != nil {
		t.Fatalf("查询挂载关联失败: %v", merr)
	}
	if mounted.StoreID != psRow.GetID() || mounted.Generation != domain.GenerationDerived {
		t.Errorf("挂载关联 storeId=%d generation=%s, want storeId=%d generation=%s",
			mounted.StoreID, mounted.Generation, psRow.GetID(), domain.GenerationDerived)
	}

	// 收尾：merge 属主根下无残留作用域
	entries, eerr := os.ReadDir(mergeRoot)
	if eerr != nil {
		t.Fatalf("读取 merge 属主根失败: %v", eerr)
	}
	if len(entries) != 0 {
		t.Errorf("流程退出后 merge 属主根应无作用域残留，实际 %d 项", len(entries))
	}

	// 系统临时目录零写入
	tempAfter := snapshotMergeTempResidue(t)
	for name := range tempAfter {
		if _, ok := tempBefore[name]; !ok {
			t.Errorf("合并不应写系统临时目录，发现新残留: %s", name)
		}
	}
}

// TestRunMergeTxFailureRollsBackJournalAndDiscardsProduct 提交事务失败补偿（锚6 失败轨）：
// 建行成功而挂载失败（resource_store 外键命中未播种的 store 行）→ 事务回滚撤销建行与登记行
// 删除（登记行保留）→ 撤回按登记的丢弃声明清除已落位产物并收口登记行；暂存作用域同步回收，
// 不留 videoMain 挂载
func TestRunMergeTxFailureRollsBackJournalAndDiscardsProduct(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	ctx := context.Background()
	workDir := t.TempDir()

	seedMergeStageBase(t, db)
	// 建行返回未播种的行 ID：CommitIngest 成功、挂载 Create 命中 resource_store.store_id
	// 外键防线失败，注入"建行成功而事务失败"的现场
	ingestor := newMergeStageIngestor(workDir, 4242)
	merger := &mergeStageMerger{}
	emitter := &mergeStageEmitter{}
	svc := NewMergeService(
		NewResourceStoreRepository(db),
		mergeNamingResource{},
		mergeNamingWork{
			siteId:     sql.NullInt64{Int64: 11, Valid: true},
			siteWorkId: sql.NullString{String: mergeStageSiteWorkId, Valid: true},
		},
		mergeNamingSite{siteKey: mergeStageSiteKey},
		merger,
		&mergeStageStoreOps{workDir: workDir},
		ingestor,
		mergeStageSettings{},
		replaceWorkDir{workDir: workDir},
		mergeStageTransactor{db: db, ingestor: ingestor},
		noopReplaceRecompute{},
		emitter,
		nil,
	)

	videoRS := domain.NewResourceStore()
	videoRS.StoreID = 901
	audioRS := domain.NewResourceStore()
	audioRS.StoreID = 902
	svc.runMerge(ctx, 700, videoRS, audioRS)

	if emitter.success {
		t.Fatalf("挂载失败应以失败收口，实际成功")
	}

	// 提交序列：登记 → 落位 → 事务内建行 → 撤回
	if got, want := ingestor.seq, []string{"prepare", "place", "commit", "abort"}; !reflect.DeepEqual(got, want) {
		t.Errorf("提交点调用序列 = %v, want %v", got, want)
	}

	// 撤回记录存在即证明撤回入口时登记行仍在——事务回滚已恢复 CommitIngest 在桩内删除的
	// 登记行（真实语义=建行与删登记行同事务、一并回滚）；撤回按丢弃声明在最终路径上清走产物
	if len(ingestor.aborts) != 1 {
		t.Fatalf("撤回应处理 1 条登记行（回滚保留证明），实际 %d", len(ingestor.aborts))
	}
	if ingestor.aborts[0].action != domain.AbortActionDiscard {
		t.Errorf("撤回应按登记的丢弃声明执行，实际 %q", ingestor.aborts[0].action)
	}
	if !ingestor.aborts[0].fileAtFinal {
		t.Errorf("撤回时产物应已在最终路径（丢弃轨清走对象）")
	}
	if len(ingestor.journals) != 0 {
		t.Errorf("撤回后登记行应收口，剩余 %d 行", len(ingestor.journals))
	}

	// 产物文件不滞留最终路径；无 videoMain 挂载
	if _, serr := os.Stat(filepath.Join(workDir, mergeStageFinalPath(t))); !os.IsNotExist(serr) {
		t.Errorf("失败收口应清除已落位产物文件，err=%v", serr)
	}
	if _, merr := NewResourceStoreRepository(db).GetByType(ctx, 700, domain.StoreTypeVideoMain); !errors.Is(merr, gorm.ErrRecordNotFound) {
		t.Errorf("失败收口不应留下 videoMain 挂载，查询错误=%v", merr)
	}

	// 收尾：merge 属主根下无残留作用域
	entries, eerr := os.ReadDir(filepath.Join(workDir, staging.RootName, string(staging.OwnerMerge)))
	if eerr != nil {
		t.Fatalf("读取 merge 属主根失败: %v", eerr)
	}
	if len(entries) != 0 {
		t.Errorf("失败收口后 merge 属主根应无作用域残留，实际 %d 项", len(entries))
	}
}

// TestMergeResourceRefusesUnconfiguredWorkDir 工作目录未配置的请求期拒绝：
// MergeResource 同步返回哨兵错误，不进入合并流程
func TestMergeResourceRefusesUnconfiguredWorkDir(t *testing.T) {
	svc := NewMergeService(
		nil, nil, nil, nil,
		&mergeStageMerger{}, // ffmpeg 可用（非 nil），排除功能不可用分支
		&mergeStageStoreOps{},
		nil,
		mergeStageSettings{},
		replaceWorkDir{workDir: ""},
		nil, nil, &mergeStageEmitter{}, nil,
	)
	if err := svc.MergeResource(context.Background(), 700); !errors.Is(err, settings.ErrWorkDirNotConfigured) {
		t.Fatalf("未配置工作目录应返回 settings.ErrWorkDirNotConfigured，实际 %v", err)
	}
}
