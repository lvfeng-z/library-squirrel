package resource

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/staging"

	"github.com/lvfeng-z/library-squirrel-sdk/storepath"
)

// ==== 合并产物暂存作用域测试（产物写 staging/merge/{铸造键}/，同卷 rename 落位）====

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

// mergeStageStoreOps 提交点建行桩：返回预置行 ID；建行时校验产物文件已 rename 就位（行在文件之后建）
type mergeStageStoreOps struct {
	workDir          string
	storeId          int64
	committedRelPath string
	failCommit       bool
}

func (o *mergeStageStoreOps) GetById(ctx context.Context, id int64) (*domain.PersistentStore, error) {
	ps := domain.NewPersistentStore()
	ps.SetID(id)
	ps.FilePath = sql.NullString{String: "store/resource/track/videoTrack_000.mp4", Valid: true}
	return ps, nil
}
func (o *mergeStageStoreOps) GetAbsPath(store *domain.PersistentStore) string {
	return filepath.Join(o.workDir, "store/resource/track/videoTrack_000.mp4")
}
func (o *mergeStageStoreOps) CommitStore(ctx context.Context, relPath string, fileName string, expectedSha, actualSha sql.NullString) (int64, error) {
	if o.failCommit {
		return 0, errors.New("建行桩失败")
	}
	o.committedRelPath = relPath
	if _, serr := os.Stat(filepath.Join(o.workDir, relPath)); serr != nil {
		return 0, fmt.Errorf("建行时产物未就位: %w", serr)
	}
	return o.storeId, nil
}
func (o *mergeStageStoreOps) DeleteWithBackup(ctx context.Context, id int64) (int64, error) {
	return 0, nil
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

// mergeStageTransactor 事务桩：直接同步执行
type mergeStageTransactor struct{}

func (mergeStageTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
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
	return path.Join("store", "resource",
		storepath.BucketSegment(mergeStageSiteKey, mergeStageSiteWorkId),
		dirName,
		"videoMain_000.mp4")
}

// TestRunMergeStagesProductUnderMergeScope 合并产物暂存位置与落位搬运：产物写进
// {workDir}/staging/merge/{铸造键}/ 作用域（目录内含自证 scope.json）、系统临时目录零写入；
// ffmpeg 成功后产物经同卷 rename 落位到 store/ 最终路径（建行时文件已就位），流程退出作用域
// 显式回收；resource_store(videoMain, derived) 挂载成行
func TestRunMergeStagesProductUnderMergeScope(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	ctx := context.Background()
	workDir := t.TempDir()

	// DB 种子：作品 500（resource.work_id 外键防线）/ 资源 700 / persistent_store 行
	// （resource_store.store_id 外键防线，行 ID 由建行桩返回）
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
	psRow := domain.NewPersistentStore()
	psRow.FilePath = sql.NullString{String: "store/resource/track/videoTrack_000.mp4", Valid: true}
	psRow.CompletedAt = 1
	if err := db.Create(psRow).Error; err != nil {
		t.Fatalf("插 persistent_store 失败: %v", err)
	}

	merger := &mergeStageMerger{}
	ops := &mergeStageStoreOps{workDir: workDir, storeId: psRow.GetID()}
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
		mergeStageSettings{},
		replaceWorkDir{workDir: workDir},
		mergeStageTransactor{},
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

	// 暂存位置：产物输出路径位于 {workDir}/staging/merge/{作用域键}/ 下
	mergeRoot := filepath.Join(workDir, staging.RootName, string(staging.OwnerMerge))
	if got := filepath.Dir(filepath.Dir(merger.outPath)); got != mergeRoot {
		t.Errorf("产物暂存父目录 = %q, want %q", got, mergeRoot)
	}

	// 落位：产物文件在最终 store 路径且内容完整；建行收到同一相对路径
	finalAbs := filepath.Join(workDir, mergeStageFinalPath(t))
	data, rerr := os.ReadFile(finalAbs)
	if rerr != nil {
		t.Fatalf("产物应落位到最终路径 %s: %v", finalAbs, rerr)
	}
	if string(data) != "merged-product" {
		t.Errorf("产物内容 = %q, want %q", string(data), "merged-product")
	}
	if ops.committedRelPath != mergeStageFinalPath(t) {
		t.Errorf("建行路径 = %q, want %q", ops.committedRelPath, mergeStageFinalPath(t))
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

// TestRunMergeCommitFailureCleansProductFile 提交事务失败补偿：建行失败时行未落库，
// 已 rename 落位的产物文件随之清除、暂存作用域同步回收
func TestRunMergeCommitFailureCleansProductFile(t *testing.T) {
	workDir := t.TempDir()
	merger := &mergeStageMerger{}
	ops := &mergeStageStoreOps{workDir: workDir, failCommit: true}
	emitter := &mergeStageEmitter{}
	svc := NewMergeService(
		nil, // 提交在建行一步失败，关联仓储不触达
		mergeNamingResource{},
		mergeNamingWork{
			siteId:     sql.NullInt64{Int64: 11, Valid: true},
			siteWorkId: sql.NullString{String: mergeStageSiteWorkId, Valid: true},
		},
		mergeNamingSite{siteKey: mergeStageSiteKey},
		merger,
		ops,
		mergeStageSettings{},
		replaceWorkDir{workDir: workDir},
		mergeStageTransactor{},
		noopReplaceRecompute{},
		emitter,
		nil,
	)

	videoRS := domain.NewResourceStore()
	videoRS.StoreID = 901
	audioRS := domain.NewResourceStore()
	audioRS.StoreID = 902
	svc.runMerge(context.Background(), 700, videoRS, audioRS)

	if emitter.success {
		t.Fatalf("建行失败应以失败收口，实际成功")
	}
	if _, err := os.Stat(filepath.Join(workDir, mergeStageFinalPath(t))); !os.IsNotExist(err) {
		t.Errorf("失败收口应清除已落位产物文件，err=%v", err)
	}
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
		mergeStageSettings{},
		replaceWorkDir{workDir: ""},
		nil, nil, &mergeStageEmitter{}, nil,
	)
	if err := svc.MergeResource(context.Background(), 700); !errors.Is(err, settings.ErrWorkDirNotConfigured) {
		t.Fatalf("未配置工作目录应返回 settings.ErrWorkDirNotConfigured，实际 %v", err)
	}
}
