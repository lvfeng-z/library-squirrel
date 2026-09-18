package download

// 插件下载执行面对外窄接口与依赖集合：接口由本模块（消费方）定义、提供方模块实现、
// app.go 装配注入。作品任务领域行仓储（本包内）与各能力提供方均经此契约进入执行面。

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/resource"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// PluginExecutor 插件任务执行器（插件扩展桥实现）：经 gRPC 调用插件的任务处理 RPC
type PluginExecutor interface {
	// CreateWorkInfo 创建作品信息（核心行 + 作品任务领域行两参；插件身份在领域行）
	CreateWorkInfo(ctx context.Context, task *entity.Task, workTask *entity.WorkTask) (*sdkdto.WorkResponse, error)

	// Start 开始任务，按 storeRoles 选择性返回 StoreSpec 流集合(含 downloaded 与 derived)、WorkResponse 或错误
	// 调用方负责关闭各 StoreSpec.ReadCloser
	Start(ctx context.Context, task *entity.Task, workTask *entity.WorkTask, storeRoles []string) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error)

	// Pause 暂停任务（任务级，广播到全部 stream）
	Pause(ctx context.Context, param *sdkdto.TaskResParam) error

	// Stop 停止任务（任务级）
	Stop(ctx context.Context, param *sdkdto.TaskResParam) error

	// Resume 恢复任务:按 StreamOffsets 续传，返回新的 StoreSpec 流集合
	Resume(ctx context.Context, param *sdkdto.TaskResumeParam) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error)
}

// PluginExecFactory 插件任务执行器获取：按插件公开 ID 取该插件的下载执行器（插件扩展桥实现）
type PluginExecFactory interface {
	Executor(pluginPublicId string) (PluginExecutor, error)
}

// WorkInfoSaver 作品完整信息保存接口
type WorkInfoSaver interface {
	SaveWorkInfo(ctx context.Context, task *entity.Task, workTask *entity.WorkTask, workResp *sdkdto.WorkResponse) (int64, error)
}

// ResourceSaver 资源保存接口
type ResourceSaver interface {
	Save(ctx context.Context, resource *entity.Resource) (int64, error)
	Updates(ctx context.Context, resource *entity.Resource) error
}

// WorkDirProvider 工作目录提供者接口
type WorkDirProvider interface {
	GetWorkDir() string
}

// SiteKeyResolver 站点 ID → 站点行批量查询（由 site.Service 实现）。两个消费方：查重输入键
// 形态统一（插件任务侧把 task.SiteID 反查站点键，与 share-receive/zip 导入的 manifest 域键对齐）
// 与落盘目录派生（resolveStoreDir 取 siteKey 组装作品目录名）
type SiteKeyResolver interface {
	ListByIds(ctx context.Context, ids []int64) ([]*entity.Site, error)
}

// ResourceReader 资源查询接口（查找已有作品的资源文件）
type ResourceReader interface {
	// ListByWorkId 查询作品关联的资源
	ListByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error)
	// GetById 根据 ID 获取资源
	GetById(ctx context.Context, id int64) (*entity.Resource, error)
}

// ResourceRecomputer 资源完整度重算（由 resource.Service 实现；活行 store 角色计数，
// 关联保留形态下软删行关联不计入）
type ResourceRecomputer interface {
	RecomputeResourceComplete(ctx context.Context, resourceId int64)
}

// StoreCommitter 提交点建行能力（暂存模式下由 persistentStore.Service 实现）：
// 暂存文件 rename 到最终路径后，在提交事务内建完整 persistent_store 行（completed_at 即时
// 置位 + 宽高/头指纹/哈希双列），返回行 ID 供 resource_store 挂载
type StoreCommitter interface {
	// CommitStore 为已就位的最终路径建/复用完整行。expectedSha/actualSha 为来源声明与
	// 暂存写入流实测的 SHA256（sql.NullString，无效态=未声明/未算）
	CommitStore(ctx context.Context, relPath string, fileName string, expectedSha, actualSha sql.NullString) (int64, error)
}

// WorkLocator 续传会话定位任务所属作品（由 work.Service 实现）：暂存模式下暂停任务零 DB
// 足迹（资源行在提交点才建），恢复时按领域复合键 (site, site_work_id) 从 work_task 行回填 workId
type WorkLocator interface {
	GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*entity.Work, error)
}

// StagingPaths 暂存目录派生与暂存文件命名（task 模块暂存基建提供、装配层适配注入——
// download 与 task 双向零 import，经窄接口缝合，先例同 WorkTaskWriter/Reader）
type StagingPaths interface {
	// StagingPath 任务暂存目录绝对路径（absPath 域，仅供 os.* 调用点现场消费）
	StagingPath(workDir string, taskID int64) string
	// StagingFileName 暂存文件名（role_seq 三位零填充键，保留扩展名）
	StagingFileName(role string, storeSeq int, ext string) string
	// EnsureStagingScope 确保任务暂存作用域存在：不存在则经暂存能力包原子入口创建（目录内写
	// 自证描述），已存在（暂停/崩溃后恢复的续传场景）复用；返回目录绝对路径（absPath 域）
	EnsureStagingScope(ctx context.Context, workDir string, taskID int64) (string, error)
}

// ResourceStoreWriter resource_store 关联写入接口(saveResource 多 store 挂载)
type ResourceStoreWriter interface {
	CreateBatch(ctx context.Context, stores []*entity.ResourceStore) error
	// DeleteByResourceIdAndTypes 删除指定 Resource 下、store_type 属于给定集合且指向活行 store 的关联
	DeleteByResourceIdAndTypes(ctx context.Context, resourceId int64, storeTypes []string) error
}

// Transactor 事务执行器接口
type Transactor interface {
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// TaskCoreReader 任务核心控制行查询（由 task 模块仓储实现，装配层注入）：
// 中断通知转发插件 Pause/Stop RPC 时组装 TaskResParam 需要核心行载荷
type TaskCoreReader interface {
	GetById(ctx context.Context, id int64) (*entity.Task, error)
}

// WorkTaskStore 作品任务领域行读取（执行入口按 taskId 查行、续传判定与中断通知组装）。
// 由本包仓储实现，装配层注入
type WorkTaskStore interface {
	// GetById 按共享主键（=所属任务 id）查询领域行
	GetById(ctx context.Context, id int64) (*entity.WorkTask, error)
}

// Deps 插件下载执行面的依赖集合：作品任务领域行仓储（本包内）+ 各能力接口
// （接口由本模块定义、提供方模块实现、app.go 装配注入）
type Deps struct {
	// WorkTasks 作品任务领域行读取（执行入口按 taskId 查行/续传判定/板块模式派生源）
	WorkTasks           WorkTaskStore
	PluginExecFactory   PluginExecFactory          // 插件任务执行器获取
	WorkInfoSaver       WorkInfoSaver              // 作品完整信息保存
	WorkLocator         WorkLocator                // 续传会话按 (site, site_work_id) 定位任务所属作品
	ResourceSaver       ResourceSaver              // 资源保存
	WorkDirProvider     WorkDirProvider            // 工作目录
	DuplicateChecker    duplicate.DuplicateChecker // 作品查重判定能力
	SiteKeyResolver     SiteKeyResolver            // 站点 ID → 站点键（查重输入键形态统一 + 落盘目录派生 resolveStoreDir）
	ResourceReader      ResourceReader             // 已有作品资源查询
	ReplaceStoreOps     resource.ReplaceStoreOps   // 替换链能力（提交窗口软删/失败回滚复活）
	ResourceUpdater     ResourceSaver              // 替换场景更新 Resource 的 Store 字段
	StoreCommitter      StoreCommitter             // 提交点建行（暂存产物 rename 后建完整行）
	ResourceStoreWriter ResourceStoreWriter        // resource_store 关联写入
	ResourceRecomputer  ResourceRecomputer         // 资源完整度重算
	Transactor          Transactor                 // 事务执行
	TaskCoreReader      TaskCoreReader             // 任务核心行查询（中断通知组装 TaskResParam）
	StagingPaths        StagingPaths               // 暂存目录派生与暂存文件命名（task 基建适配）
}
