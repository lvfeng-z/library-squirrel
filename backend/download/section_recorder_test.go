package download

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
)

// 本文件为开始入口板块写行的链路测试：任务创建落库的领域行默认不含作品信息
// （include_work_info=false），开始入口写行全量板块（store_roles=NULL、含作品信息）后，
// 执行面按行派生出的板块组合须含作品信息且资源为全量——直构领域行的测试形态掩盖
// 「创建默认值 + 开始写行 + 派生」的链路，在此以真实仓储锚定。

// fakeChildReader 任务核心行子成员查询桩：按预置 pid→子任务映射返回
type fakeChildReader struct {
	byPid map[int64][]*entity.Task
}

func (r *fakeChildReader) ListChildrenTask(ctx context.Context, pid int64) ([]*entity.Task, error) {
	return r.byPid[pid], nil
}

// TestStartRecordSectionsDerivesFullMode 开始入口写行链路：创建默认领域行（不含作品信息）→
// 按开始入口参数写行（全量+含作品信息，父任务请求展开到全部子成员）→ 重读行派生板块模式，
// 父与全部子任务均派生为「含作品信息 + 资源全量」
func TestStartRecordSectionsDerivesFullMode(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	ctx := context.Background()

	// 父任务 + 两叶子（任务树两级），各挂创建默认的作品任务领域行
	parent := entity.NewTask()
	parent.TaskType = sql.NullString{String: "plugin-download", Valid: true}
	parent.TaskName = sql.NullString{String: "合集", Valid: true}
	if err := db.Create(parent).Error; err != nil {
		t.Fatalf("建父任务失败: %v", err)
	}
	repo := NewWorkTaskRepository(db)
	leaves := make([]*entity.Task, 2)
	for i := range leaves {
		leaf := entity.NewTask()
		leaf.TaskType = sql.NullString{String: "plugin-download", Valid: true}
		leaf.Pid = sql.NullInt64{Int64: parent.GetID(), Valid: true}
		if err := db.Create(leaf).Error; err != nil {
			t.Fatalf("建叶子任务失败: %v", err)
		}
		leaves[i] = leaf
		if err := repo.CreateForTask(ctx, leaf.GetID(), entity.NewWorkTask(leaf.GetID())); err != nil {
			t.Fatalf("建叶子领域行失败: %v", err)
		}
	}
	if err := repo.CreateForTask(ctx, parent.GetID(), entity.NewWorkTask(parent.GetID())); err != nil {
		t.Fatalf("建父领域行失败: %v", err)
	}

	// 创建默认：不含作品信息、板块未记录——此默认派生不含作品信息板块（开始写行前的实态）
	defaultRow, err := repo.GetById(ctx, leaves[0].GetID())
	if err != nil {
		t.Fatalf("查创建默认领域行失败: %v", err)
	}
	if defaultRow.IncludeWorkInfo || defaultRow.StoreRoles.Valid {
		t.Fatalf("创建默认应为不含作品信息且板块未记录, 实际 workInfo=%v roles=%+v",
			defaultRow.IncludeWorkInfo, defaultRow.StoreRoles)
	}

	children := &fakeChildReader{byPid: map[int64][]*entity.Task{parent.GetID(): leaves}}
	recorder := NewSectionRecorder(repo, children)

	// 开始入口写行：全量（NULL roles）+ 含作品信息，父任务请求展开到全部子成员
	if err := recorder.RecordSections(ctx, []int64{parent.GetID()}, sql.NullString{Valid: false}, true); err != nil {
		t.Fatalf("开始板块写行失败: %v", err)
	}

	ids := []int64{parent.GetID(), leaves[0].GetID(), leaves[1].GetID()}
	for _, id := range ids {
		wt, err := repo.GetById(ctx, id)
		if err != nil {
			t.Fatalf("查任务 %d 领域行失败: %v", id, err)
		}
		if wt.StoreRoles.Valid || !wt.IncludeWorkInfo {
			t.Fatalf("任务 %d 开始写行应为全量+含作品信息, 实际 roles=%+v workInfo=%v",
				id, wt.StoreRoles, wt.IncludeWorkInfo)
		}
		mode := runModeFromTask(wt)
		if !mode.hasWorkInfo() || mode.storeScope.kind != scopeAll {
			t.Fatalf("任务 %d 派生板块应为含作品信息+资源全量, 实际 %+v", id, mode)
		}
	}
}

// TestRecordSectionsLeafRequestIsolated 单独请求叶子只写该叶子自身：同父兄弟的已记录板块
// 不被波及（运行中兄弟重纳场景的写行范围）
func TestRecordSectionsLeafRequestIsolated(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	ctx := context.Background()

	leafA, leafB := entity.NewTask(), entity.NewTask()
	for _, leaf := range []*entity.Task{leafA, leafB} {
		leaf.TaskType = sql.NullString{String: "plugin-download", Valid: true}
		if err := db.Create(leaf).Error; err != nil {
			t.Fatalf("建叶子任务失败: %v", err)
		}
	}
	repo := NewWorkTaskRepository(db)
	for _, leaf := range []*entity.Task{leafA, leafB} {
		if err := repo.CreateForTask(ctx, leaf.GetID(), entity.NewWorkTask(leaf.GetID())); err != nil {
			t.Fatalf("建领域行失败: %v", err)
		}
	}
	// 兄弟 B 已持有用户所选板块（重下载记录），请求 A 时不得被覆盖
	if err := repo.UpdateRedownloadSections(ctx, []int64{leafB.GetID()},
		sql.NullString{String: entity.StoreTypeImage, Valid: true}, false); err != nil {
		t.Fatalf("预置兄弟板块失败: %v", err)
	}

	recorder := NewSectionRecorder(repo, &fakeChildReader{byPid: map[int64][]*entity.Task{}})
	if err := recorder.RecordSections(ctx, []int64{leafA.GetID()}, sql.NullString{Valid: false}, true); err != nil {
		t.Fatalf("板块写行失败: %v", err)
	}

	gotA, err := repo.GetById(ctx, leafA.GetID())
	if err != nil {
		t.Fatalf("查 A 领域行失败: %v", err)
	}
	if gotA.StoreRoles.Valid || !gotA.IncludeWorkInfo {
		t.Fatalf("A 应被写为全量+含作品信息, 实际 roles=%+v workInfo=%v", gotA.StoreRoles, gotA.IncludeWorkInfo)
	}
	gotB, err := repo.GetById(ctx, leafB.GetID())
	if err != nil {
		t.Fatalf("查 B 领域行失败: %v", err)
	}
	if !gotB.StoreRoles.Valid || gotB.StoreRoles.String != entity.StoreTypeImage || gotB.IncludeWorkInfo {
		t.Fatalf("兄弟 B 已记录板块不应被波及, 实际 roles=%+v workInfo=%v", gotB.StoreRoles, gotB.IncludeWorkInfo)
	}
}
