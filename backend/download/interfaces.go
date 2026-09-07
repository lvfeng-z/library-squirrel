package download

// 插件下载执行面对外窄接口与依赖集合：接口由本模块（消费方）定义、提供方模块实现、
// app.go 装配注入。作品任务领域行仓储（本包内）与各能力提供方均经此契约进入执行面。

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/persistentStore"
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

// WorkMetaLoader 已有作品命名元数据加载接口
// 资源板块单独重下(未跑作品元数据板块)时,从已有作品获取文件名模板所需元数据(作者等),与板块选择解耦
type WorkMetaLoader interface {
	LoadWorkMeta(ctx context.Context, workId int64) (*sdkdto.WorkResponse, error)
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

// FileNameFormatProvider 文件名格式模板提供者接口
type FileNameFormatProvider interface {
	GetFileNameFormat() string
}

// SiteKeyResolver 站点 ID → 站点行批量查询（查重输入键形态统一：插件任务侧把 task.SiteID
// 反查站点键，与 share-receive/zip 导入的 manifest 域键对齐；由 site.Service 实现）
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

// WorkLivenessReader 作品活行查询接口（失败还原链守卫：作品已软删则跳过回滚）
type WorkLivenessReader interface {
	GetById(ctx context.Context, id int64) (*entity.Work, error)
}

// StoreBackupReader store 行含删读取与复活（由 persistentStore.Service 实现）
type StoreBackupReader interface {
	// ListByIdsIncludeDeleted 按 ID 集合查记录行（含已删行；行内 backup_id/file_path/deleted_at 供失败还原派生）
	ListByIdsIncludeDeleted(ctx context.Context, ids []int64) []*entity.PersistentStore
	// RestoreByIds 批量复活记录（清软删标志与 backup_id；文件还原回 store/ 后调用）
	RestoreByIds(ctx context.Context, ids []int64) error
}

// ResourceRecomputer 资源完整度重算（由 resource.Service 实现；活行 store 角色计数，
// 关联保留形态下软删行关联不计入）
type ResourceRecomputer interface {
	RecomputeResourceComplete(ctx context.Context, resourceId int64)
}

// StoreStreamer 创建存储记录并返回 StoreWriter
type StoreStreamer interface {
	StoreStream(ctx context.Context, relPath string, fileName string) (storeId int64, writer persistentStore.StoreWriter, err error)
	ResumeStream(ctx context.Context, storeId int64, offset int64) (writer persistentStore.StoreWriter, err error)
}

// StoreReader 查询 PersistentStore 记录
type StoreReader interface {
	GetById(ctx context.Context, id int64) (*entity.PersistentStore, error)
	GetAbsPath(store *entity.PersistentStore) string
}

// ResourceStoreReader resource_store 关联查询接口(多轨续传按 role 遍历 store)
type ResourceStoreReader interface {
	ListByResourceId(ctx context.Context, resourceId int64) ([]*entity.ResourceStore, error)
	// ListByResourceIds 批量查询多个 Resource 的关联行（替换链软删/失败还原派生用）
	ListByResourceIds(ctx context.Context, resourceIds []int64) ([]*entity.ResourceStore, error)
}

// ResourceStoreWriter resource_store 关联写入接口(saveResource 多 store 挂载)
type ResourceStoreWriter interface {
	CreateBatch(ctx context.Context, stores []*entity.ResourceStore) error
	// DeleteByResourceIdAndTypes 删除指定 Resource 下、store_type 属于给定集合且指向活行 store 的关联
	DeleteByResourceIdAndTypes(ctx context.Context, resourceId int64, storeTypes []string) error
	// DeleteByStoreIds 按 store ID 集合物理删除关联行（失败还原清理本次新建 store 的关联）
	DeleteByStoreIds(ctx context.Context, storeIds []int64) error
}

// Transactor 事务执行器接口
type Transactor interface {
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// PendingResourceUpdater 任务 pending_resource_id 同步更新接口（用于事务内直接写 DB）
type PendingResourceUpdater interface {
	UpdatePendingResourceID(ctx context.Context, taskId int64, resourceID sql.NullInt64) error
}

// StoreFileCleaner 事务失败时清理磁盘文件
type StoreFileCleaner interface {
	CleanupFile(relPath string)
}

// StoreDeleter 删除 PersistentStore 记录及磁盘文件（由 persistentStore.Service 实现）
// 失败还原前清理本次新建 store 时使用，backup=false 表示直接删除不产生备份
type StoreDeleter interface {
	// HardDelete 删除记录及对应文件（物理删记录）
	HardDelete(ctx context.Context, id int64, backup bool) (int64, error)
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
	WorkTasks              WorkTaskStore
	PluginExecFactory      PluginExecFactory          // 插件任务执行器获取
	WorkInfoSaver          WorkInfoSaver              // 作品完整信息保存
	WorkMetaLoader         WorkMetaLoader             // 已有作品命名元数据加载
	ResourceSaver          ResourceSaver              // 资源保存
	WorkDirProvider        WorkDirProvider            // 工作目录
	FileNameFormatProvider FileNameFormatProvider     // 文件名格式模板
	DuplicateChecker       duplicate.DuplicateChecker // 作品查重判定能力
	SiteKeyResolver        SiteKeyResolver            // 站点 ID → 站点键（查重输入键形态统一）
	ResourceReader         ResourceReader             // 已有作品资源查询
	WorkLivenessReader     WorkLivenessReader         // 作品活行查询（失败还原链守卫）
	ReplaceStoreOps        resource.ReplaceStoreOps   // 替换链能力（前置软删/失败回滚复活）
	StoreBackupReader      StoreBackupReader          // store 行含删读取与复活
	ResourceUpdater        ResourceSaver              // 替换场景更新 Resource 的 Store 字段
	StoreStreamer          StoreStreamer              // 落盘流创建/续传
	StoreReader            StoreReader                // PersistentStore 记录查询
	ResourceStoreReader    ResourceStoreReader        // resource_store 关联查询
	ResourceStoreWriter    ResourceStoreWriter        // resource_store 关联写入
	ResourceRecomputer     ResourceRecomputer         // 资源完整度重算
	Transactor             Transactor                 // 事务执行
	PendingResourceUpdater PendingResourceUpdater     // pending_resource_id 事务内直写
	StoreFileCleaner       StoreFileCleaner           // 事务失败清理磁盘文件
	StoreDeleter           StoreDeleter               // PersistentStore 记录及文件删除
	TaskCoreReader         TaskCoreReader             // 任务核心行查询（中断通知组装 TaskResParam）
}
