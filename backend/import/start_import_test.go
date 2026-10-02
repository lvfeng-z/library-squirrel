package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/task"

	"github.com/stretchr/testify/require"
)

// 本文件为 StartImport 建树入口测试：fake 任务控制桩断言建树形态（父容器命名/子任务净化名/
// import_task 领域行载荷/manifest 原字节落盘/整树启动）、失败删树回滚、建树前校验拒绝
// （上限/版本锚/缺 manifest）。对齐 share 收件建树测试（backend/share/receive_test.go）形态。

// fakeImportTaskControl StartImport 建树流程的任务控制桩：模拟 task.Service 两段式建树行为，
// 记录启动/删除调用与建树结果供断言。createChildrenErr 注入建子失败路径。
type fakeImportTaskControl struct {
	mu                sync.Mutex
	nextTaskID        int64
	parent            *entity.Task
	children          []*entity.Task
	startedIDs        []int64
	deletedIDs        []int64
	createChildrenErr error
}

func (f *fakeImportTaskControl) CreateBuiltinTaskParent(ctx context.Context, taskType string, parentName string) (*entity.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextTaskID++
	parent := entity.NewTask()
	parent.ID = f.nextTaskID
	parent.TaskName = sql.NullString{String: parentName, Valid: true}
	parent.TaskType = sql.NullString{String: taskType, Valid: true}
	parent.HasChild = sql.NullBool{Bool: true, Valid: true}
	f.parent = parent
	return parent, nil
}

func (f *fakeImportTaskControl) CreateBuiltinTaskChildren(ctx context.Context, taskType string, parentID int64, children []task.BuiltinTaskChild) ([]*entity.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createChildrenErr != nil {
		return nil, f.createChildrenErr
	}
	created := make([]*entity.Task, 0, len(children))
	for _, c := range children {
		f.nextTaskID++
		child := entity.NewTask()
		child.ID = f.nextTaskID
		child.Pid = sql.NullInt64{Int64: parentID, Valid: true}
		child.TaskName = sql.NullString{String: c.TaskName, Valid: true}
		child.TaskType = sql.NullString{String: taskType, Valid: true}
		child.HasChild = sql.NullBool{Bool: false, Valid: true}
		f.children = append(f.children, child)
		created = append(created, child)
	}
	return created, nil
}

func (f *fakeImportTaskControl) StartTasks(ctx context.Context, taskIds []int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startedIDs = append(f.startedIDs, taskIds...)
	return nil
}

func (f *fakeImportTaskControl) DeleteTask(ctx context.Context, ids []int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletedIDs = append(f.deletedIDs, ids...)
	return nil
}

// fakeImportTaskStore 导入任务领域行存取桩：记录写入行供断言。createErr 注入写行失败路径。
type fakeImportTaskStore struct {
	mu        sync.Mutex
	rows      map[int64]*entity.ImportTask
	createErr error
}

func (f *fakeImportTaskStore) CreateForTask(ctx context.Context, taskID int64, it *entity.ImportTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return f.createErr
	}
	if f.rows == nil {
		f.rows = map[int64]*entity.ImportTask{}
	}
	f.rows[taskID] = it
	return nil
}

func (f *fakeImportTaskStore) GetById(ctx context.Context, id int64) (*entity.ImportTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if it, ok := f.rows[id]; ok {
		return it, nil
	}
	return nil, fmt.Errorf("导入任务领域行不存在: %d", id)
}

// buildStartImportZip 构建含 manifest.json 的导出包 fixture，返回包路径与 manifest 原字节
// （建树只读 manifest 条目，文件条目无需塞入）。
func buildStartImportZip(t *testing.T, manifest *export.Manifest) (string, []byte) {
	t.Helper()
	data, err := manifest.Serialize()
	require.NoError(t, err)
	return writeZip(t, map[string][]byte{"manifest.json": data}), data
}

// TestStartImportBuildsTaskTree 建树主线：父容器「导入（N 项）」＋每作品一子任务（净化名）＋
// import_task 领域行载荷三元组（zip_path/manifest_rel/manifest_id）＋manifest 原字节落盘
// 父作用域＋整树按父 ID 启动＋返回建树摘要。
func TestStartImportBuildsTaskTree(t *testing.T) {
	workDir := t.TempDir()
	manifest, _ := buildFixture()
	zipPath, manifestData := buildStartImportZip(t, manifest)
	ctl := &fakeImportTaskControl{nextTaskID: 500}
	store := &fakeImportTaskStore{}
	handler := NewHandler(func() string { return workDir }, ctl, store)

	resp := handler.StartImport(context.Background(), zipPath)
	require.True(t, resp.Success, "建树失败: %s", resp.Msg)

	n := len(manifest.Works)
	parent := ctl.parent
	require.NotNil(t, parent, "应创建父容器")
	require.Equal(t, fmt.Sprintf("导入（%d 项）", n), parent.TaskName.String)
	require.Equal(t, TaskTypeImport, parent.TaskType.String)
	require.True(t, parent.HasChild.Bool)
	require.Len(t, ctl.children, n)
	require.Equal(t, []int64{parent.GetID()}, ctl.startedIDs, "整树应按父 ID 启动")
	require.Empty(t, ctl.deletedIDs, "成功路径不应删树")

	// 子任务命名＝净化后作品名；领域行载荷三元组齐备且主键共享
	for i, child := range ctl.children {
		require.Equal(t, SanitizedWorkName(&manifest.Works[i]), child.TaskName.String)
		require.Equal(t, parent.GetID(), child.Pid.Int64)
		require.False(t, child.HasChild.Bool)
		it, ok := store.rows[child.GetID()]
		require.True(t, ok, "子任务 %d 缺 import_task 领域行", child.GetID())
		require.Equal(t, child.GetID(), it.GetID(), "领域行主键应与任务行共享")
		require.Equal(t, zipPath, it.ZipPath)
		require.Equal(t, task.ImportManifestRelPath(parent.GetID()), it.ManifestRel)
		require.Equal(t, manifest.Works[i].ID, it.ManifestID)
	}

	// 共享 manifest 原字节落盘父作用域（无再序列化字节漂移）；作用域自证描述存在
	rel := task.ImportManifestRelPath(parent.GetID())
	raw, err := os.ReadFile(filepath.Join(workDir, filepath.FromSlash(rel)))
	require.NoError(t, err, "共享 manifest 应落盘父作用域")
	require.Equal(t, manifestData, raw, "落盘 manifest 应为包内原字节")
	require.FileExists(t, filepath.Join(task.ImportStagingPath(workDir, parent.GetID()), "scope.json"),
		"父作用域应经确保入口创建")

	// 返回建树摘要（引导去任务面板）
	require.Equal(t, parent.GetID(), resp.Data.ParentTaskID)
	require.Equal(t, n, resp.Data.WorkCount)
	require.Len(t, resp.Data.WorkNames, n)
}

// TestStartImportRollbackOnFailure 建树中途失败显式删树回滚（不留孤儿任务）：建子失败与
// 领域行写入失败两路径均删父树、不启动、不残留领域行。
func TestStartImportRollbackOnFailure(t *testing.T) {
	manifest, _ := buildFixture()
	zipPath, _ := buildStartImportZip(t, manifest)

	t.Run("建子失败删树回滚", func(t *testing.T) {
		workDir := t.TempDir()
		ctl := &fakeImportTaskControl{createChildrenErr: errors.New("建子失败")}
		store := &fakeImportTaskStore{}
		resp := NewHandler(func() string { return workDir }, ctl, store).StartImport(context.Background(), zipPath)
		require.False(t, resp.Success)
		require.NotNil(t, ctl.parent)
		require.Equal(t, []int64{ctl.parent.GetID()}, ctl.deletedIDs, "失败应显式删树回滚")
		require.Empty(t, store.rows, "回滚后不应残留领域行")
		require.Empty(t, ctl.startedIDs, "失败树不应启动")
	})

	t.Run("领域行写入失败删树回滚", func(t *testing.T) {
		workDir := t.TempDir()
		ctl := &fakeImportTaskControl{}
		store := &fakeImportTaskStore{createErr: errors.New("写行失败")}
		resp := NewHandler(func() string { return workDir }, ctl, store).StartImport(context.Background(), zipPath)
		require.False(t, resp.Success)
		require.Equal(t, []int64{ctl.parent.GetID()}, ctl.deletedIDs, "失败应显式删树回滚")
		require.Empty(t, ctl.startedIDs)
	})
}

// TestStartImportRejectsInvalidPackage 建树前校验拒绝：缺 manifest、作品数超上限、版本锚
// 不匹配——均不建树（不建「注定全失败」的任务树）。
func TestStartImportRejectsInvalidPackage(t *testing.T) {
	workDir := t.TempDir()

	t.Run("缺 manifest 的包报错", func(t *testing.T) {
		ctl := &fakeImportTaskControl{}
		zipPath := writeZip(t, map[string][]byte{"other.txt": []byte("x")})
		resp := NewHandler(func() string { return workDir }, ctl, &fakeImportTaskStore{}).StartImport(context.Background(), zipPath)
		require.False(t, resp.Success)
		require.Nil(t, ctl.parent, "校验失败不应建树")
	})

	t.Run("作品数超上限拒绝建树", func(t *testing.T) {
		ctl := &fakeImportTaskControl{}
		manifest, _ := buildFixture()
		seed := manifest.Works[0]
		for i := 0; i < importBuildTreeMaxWorks; i++ {
			manifest.Works = append(manifest.Works, seed)
		}
		zipPath, _ := buildStartImportZip(t, manifest)
		resp := NewHandler(func() string { return workDir }, ctl, &fakeImportTaskStore{}).StartImport(context.Background(), zipPath)
		require.False(t, resp.Success)
		require.Contains(t, resp.Msg, "作品数超上限")
		require.Nil(t, ctl.parent, "超限不应建树")
	})

	t.Run("版本锚不匹配拒绝", func(t *testing.T) {
		ctl := &fakeImportTaskControl{}
		manifest, _ := buildFixture()
		manifest.SchemaVersion = export.SchemaVersion + 1
		zipPath, _ := buildStartImportZip(t, manifest)
		resp := NewHandler(func() string { return workDir }, ctl, &fakeImportTaskStore{}).StartImport(context.Background(), zipPath)
		require.False(t, resp.Success)
		require.Nil(t, ctl.parent, "版本拒绝不应建树")
	})
}
