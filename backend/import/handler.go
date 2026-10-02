package importer

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/task"
)

// manifestEntryName manifest 在导出包内的条目名（与 export.Packer 写入侧一致）。
const manifestEntryName = "manifest.json"

// TaskTypeImport 导入任务类型（登记于 task.task_type；执行面策略注册归任务模块接入阶段）。
const TaskTypeImport = "import"

// importBuildTreeMaxWorks 建树作品数上限（对齐导出收集上限保护）：超限拒绝建树提示分批导出。
const importBuildTreeMaxWorks = 10000

// 错误定义。
var (
	// ErrImportTaskControlNil 未注入任务控制能力（装配缺失）
	ErrImportTaskControlNil = errors.New("导入任务控制能力未装配")
	// ErrImportTaskStoreNil 未注入导入任务领域行存取能力（装配缺失）
	ErrImportTaskStoreNil = errors.New("导入任务参数存储能力未装配")
	// ErrImportWorksOverflow manifest 作品数超建树上限
	ErrImportWorksOverflow = errors.New("作品数超上限，请分批导出")
)

// TaskControl 导入任务创建与启动能力（task.Service 与 taskManager 经 app.go 适配器组合装配；
// 二者创建晚于 ImportHandler，依赖经闭包运行期取用）。任务核心行不含领域载荷：
// 导入参数在 import_task 领域行（建树后由 Handler 补写）。
type TaskControl interface {
	// CreateBuiltinTaskParent 创建内置任务树父容器（has_child=true、pid=NULL），供先建父再建子的两段式建树
	CreateBuiltinTaskParent(ctx context.Context, taskType string, parentName string) (*entity.Task, error)
	// CreateBuiltinTaskChildren 在既有父任务下创建内置任务树子任务（pid=parentID、has_child=false），
	// 返回创建的子任务（含 ID，供调用方写各自领域行）
	CreateBuiltinTaskChildren(ctx context.Context, taskType string, parentID int64, children []task.BuiltinTaskChild) ([]*entity.Task, error)
	// StartTasks 启动任务树（传父任务 ID 即整树加载派发）
	StartTasks(ctx context.Context, taskIds []int64) error
	// DeleteTask 批量删除任务（含子任务与领域行）；建树失败回滚用
	DeleteTask(ctx context.Context, ids []int64) error
}

// ImportTaskStore 导入任务领域行存取（ImportTaskRepository 实现，接口由本模块声明）。
type ImportTaskStore interface {
	// CreateForTask 为任务 taskID 建导入任务领域行（主键覆写为 taskID）
	CreateForTask(ctx context.Context, taskID int64, it *entity.ImportTask) error
	// GetById 按共享主键（=所属任务 id）查询领域行
	GetById(ctx context.Context, id int64) (*entity.ImportTask, error)
}

// StartImportResult 建树结果摘要（成功提示引导用户去任务面板查看进度）。
type StartImportResult struct {
	ParentTaskID int64    `json:"parentTaskId"` // 父容器任务 ID
	WorkCount    int      `json:"workCount"`    // manifest 作品数
	WorkNames    []string `json:"workNames"`    // 净化后作品名（与子任务命名同源）
}

// Handler 导入 Handler（Wails Bind 方法，经 IPC 暴露给前端）。
type Handler struct {
	workDirGetter func() string    // 库根读取器（导入暂存作用域在其下创建）
	taskCtl       TaskControl      // 导入任务建树/启动/回滚能力
	taskStore     ImportTaskStore  // import_task 领域行存取
}

// NewHandler 创建导入 Handler（StartImport 建树入口；回灌 Ingest 由执行面策略调用不经
// Handler——旧同步入口已随任务化退役）。
func NewHandler(workDirGetter func() string, taskCtl TaskControl, taskStore ImportTaskStore) *Handler {
	return &Handler{workDirGetter: workDirGetter, taskCtl: taskCtl, taskStore: taskStore}
}

// StartImport 从导出 ZIP 产物建导入任务树：读包内 manifest → 建树前校验（版本锚/非空/上限/
// 站点键——提前失败，不建「注定全失败」的任务树）→ 两段式建树（父容器「导入（N 项）」→
// manifest 原字节落盘父作用域 → 子任务 + import_task 领域行）→ 整树启动。建树各步失败显式
// 删树回滚不留孤儿任务（share 收件同款失败语义）；执行进度/暂停/重试归任务面板（执行面策略）。
func (h *Handler) StartImport(ctx context.Context, zipPath string) *model.ApiResponse[*StartImportResult] {
	result, err := h.startImport(ctx, zipPath)
	if err != nil {
		return model.HandleError[*StartImportResult](err)
	}
	return model.Success(result)
}

// startImport 建树主体（错误直通，由 StartImport 统一包 ApiResponse）。
func (h *Handler) startImport(ctx context.Context, zipPath string) (*StartImportResult, error) {
	if zipPath == "" {
		return nil, errors.New("导入产物路径为空")
	}
	if h.taskCtl == nil {
		return nil, ErrImportTaskControlNil
	}
	if h.taskStore == nil {
		return nil, ErrImportTaskStoreNil
	}
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("打开导入产物失败: %w", err)
	}
	defer func() { _ = reader.Close() }()

	manifest, raw, err := readManifest(&reader.Reader)
	if err != nil {
		return nil, err
	}
	if manifest.SchemaVersion != export.SchemaVersion {
		return nil, fmt.Errorf("%w: %d（支持 %d）", ErrSchemaVersionUnsupported, manifest.SchemaVersion, export.SchemaVersion)
	}
	if len(manifest.Works) == 0 {
		return nil, errors.New("导入包中没有作品，无法导入")
	}
	if len(manifest.Works) > importBuildTreeMaxWorks {
		return nil, fmt.Errorf("%w: %d > %d", ErrImportWorksOverflow, len(manifest.Works), importBuildTreeMaxWorks)
	}
	if err := validateSiteKeys(manifest.Sites); err != nil {
		return nil, err
	}
	// 净化后作品名：子任务命名与返回 DTO workNames 的共同来源（与收件子任务命名同源规则）
	names := make([]string, 0, len(manifest.Works))
	for i := range manifest.Works {
		names = append(names, SanitizedWorkName(&manifest.Works[i]))
	}

	// 两段式建树：先建父容器拿 parentID（import_task 领域行的 ManifestRel 依赖父目录路径），
	// manifest 原字节落盘父作用域后补建子任务（核心行 + import_task 领域行）
	parent, err := h.taskCtl.CreateBuiltinTaskParent(ctx, TaskTypeImport,
		fmt.Sprintf("导入（%d 项）", len(manifest.Works)))
	if err != nil {
		return nil, err
	}
	parentID := parent.GetID()
	rollback := func() { _ = h.taskCtl.DeleteTask(ctx, []int64{parentID}) }
	// 共享 manifest 原字节落盘导入暂存根的父任务作用域（staging/import/{父任务ID}/manifest.json，
	// 与各子任务暂存作用域平级）；从 zip 条目原样拷出避免反序列化再序列化的字节漂移，
	// manifest_rel 列存此值，子任务执行面按列值直读
	if _, err := task.EnsureImportScope(ctx, h.workDirGetter(), parentID); err != nil {
		rollback()
		return nil, fmt.Errorf("创建导入暂存目录失败: %w", err)
	}
	manifestRel := task.ImportManifestRelPath(parentID)
	if err := writeRawManifestFile(h.workDirGetter(), manifestRel, raw); err != nil {
		rollback()
		return nil, fmt.Errorf("保存导入清单失败: %w", err)
	}
	children := make([]task.BuiltinTaskChild, 0, len(manifest.Works))
	for _, n := range names {
		children = append(children, task.BuiltinTaskChild{TaskName: n})
	}
	// 建子核心行（无载荷）→ 逐子任务补写 import_task 领域行；任一步失败显式删树回滚
	createdChildren, err := h.taskCtl.CreateBuiltinTaskChildren(ctx, TaskTypeImport, parentID, children)
	if err != nil {
		rollback()
		return nil, err
	}
	for i, child := range createdChildren {
		it := entity.NewImportTask(child.GetID())
		it.ZipPath = zipPath
		it.ManifestRel = manifestRel
		it.ManifestID = manifest.Works[i].ID
		if err := h.taskStore.CreateForTask(ctx, child.GetID(), it); err != nil {
			rollback()
			return nil, fmt.Errorf("写入导入任务参数失败: %w", err)
		}
	}
	// 整树启动（taskManager 按父 ID 加载整树并派发全部子任务；启动失败不删树——树已完整
	// 落库，用户可在任务面板手动启动，对齐 share 收件建树语义）
	if err := h.taskCtl.StartTasks(ctx, []int64{parentID}); err != nil {
		return nil, err
	}
	return &StartImportResult{ParentTaskID: parentID, WorkCount: len(manifest.Works), WorkNames: names}, nil
}

// readManifest 读取包内 manifest.json：返回反序列化结果与条目原字节（建树侧原样落盘共享
// manifest 用，避免再序列化字节漂移；版本锚校验由消费方承担——建树入口与 Ingest 各自前置）。
func readManifest(r *zip.Reader) (*export.Manifest, []byte, error) {
	for _, f := range r.File {
		if f.Name != manifestEntryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, nil, fmt.Errorf("读取 manifest 失败: %w", err)
		}
		data, err := io.ReadAll(rc)
		closeErr := rc.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("读取 manifest 失败: %w", err)
		}
		if closeErr != nil {
			return nil, nil, fmt.Errorf("关闭 manifest 条目失败: %w", closeErr)
		}
		manifest, err := export.Deserialize(data)
		if err != nil {
			return nil, nil, fmt.Errorf("解析 manifest 失败: %w", err)
		}
		return manifest, data, nil
	}
	return nil, nil, fmt.Errorf("导入包缺少 %s", manifestEntryName)
}

// writeRawManifestFile 共享 manifest 原字节落盘（作用域目录经确保入口已存在，直写文件）。
func writeRawManifestFile(workDir, manifestRel string, data []byte) error {
	abs := filepath.Join(workDir, filepath.FromSlash(manifestRel))
	return os.WriteFile(abs, data, 0o644)
}
