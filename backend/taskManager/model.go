package taskManager

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/library-squirrel/backend/backup"
	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/util/filename"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// TaskState 任务状态
type TaskState int32

const (
	TaskStateCreated         TaskState = iota // 0: 已创建（未启动）
	TaskStateWaiting                          // 1: 等待中（排队中）
	TaskStateProcessing                       // 2: 处理中
	TaskStatePausing                          // 3: 暂停中
	TaskStatePaused                           // 4: 已暂停
	TaskStateStopping                         // 5: 停止中
	TaskStateFinished                         // 6: 已完成
	TaskStateFailed                           // 7: 失败
	TaskStatePartlyFinished                   // 8: 部分完成（父任务专用）
	TaskStateWaitingForInput                  // 9: 等待用户确认（瞬态，不持久化）
)

// runResult run() 方法的返回值类型
type runResult int

const (
	runResultDone        runResult = iota // 正常完成（Finished/Failed）
	runResultNeedConfirm                  // 检测到重复，需要用户确认
	runResultPaused                       // Setup 阶段暂停，goroutine 退出，等待恢复后重新调度
)

// runMode 板块执行选择:workInfo 为作品元数据独立板块,storeRoles 为所选资源 store_type 集合
type runMode struct {
	workInfo   bool     // 作品元数据板块
	storeRoles []string // 资源板块:所选 store_type 子集(main/thumbnail/videoTrack/...)
}

// allStoreRoles 全集资源角色;随资源类型扩展追加
var allStoreRoles = []string{
	entity.StoreTypeMain,
	entity.StoreTypeThumbnail,
}

// runModeFull 全量执行(全部元数据 + 全部资源角色)
var runModeFull = runMode{workInfo: true, storeRoles: allStoreRoles}

func (m runMode) hasWorkInfo() bool { return m.workInfo }

// hasStore 是否选择了指定 store_type
func (m runMode) hasStore(storeType string) bool {
	return slices.Contains(m.storeRoles, storeType)
}

// hasAnyStore 是否选择了任意资源板块(决定是否产生任务终态)
func (m runMode) hasAnyStore() bool { return len(m.storeRoles) > 0 }

// runModeFromTask 从 task 持久化字段派生 runMode
// StoreRoles 未设置(NULL)= 首次执行,返回全量;已设置(Redownload)= 按字段还原
func runModeFromTask(t *entity.Task) runMode {
	if !t.StoreRoles.Valid {
		return runModeFull
	}
	return runMode{
		workInfo:   t.IncludeWorkInfo,
		storeRoles: parseStoreRoles(t.StoreRoles),
	}
}

// parseStoreRoles 解析逗号分隔的 store_type 字符串为切片
func parseStoreRoles(s sql.NullString) []string {
	if !s.Valid || s.String == "" {
		return nil
	}
	parts := strings.Split(s.String, ",")
	roles := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			roles = append(roles, p)
		}
	}
	return roles
}

// taskStateName 任务状态名称映射
func taskStateName(s TaskState) string {
	switch s {
	case TaskStateCreated:
		return "Created"
	case TaskStateWaiting:
		return "Waiting"
	case TaskStateProcessing:
		return "Processing"
	case TaskStatePausing:
		return "Pausing"
	case TaskStatePaused:
		return "Paused"
	case TaskStateStopping:
		return "Stopping"
	case TaskStateFinished:
		return "Finished"
	case TaskStateFailed:
		return "Failed"
	case TaskStatePartlyFinished:
		return "PartlyFinished"
	case TaskStateWaitingForInput:
		return "WaitingForInput"
	default:
		return "Unknown"
	}
}

// isStableState 判断任务状态是否为稳定状态（需要持久化到数据库）
func isStableState(state TaskState) bool {
	switch state {
	case TaskStatePaused, TaskStateFinished, TaskStateFailed, TaskStatePartlyFinished:
		return true
	default:
		return false
	}
}

// TaskExecutor 任务执行器接口
// 由 TaskManager 定义，Plugin 模块实现
type TaskExecutor interface {
	// CreateWorkInfo 创建作品信息
	CreateWorkInfo(ctx context.Context, task *entity.Task) (*sdkdto.WorkResponse, error)

	// Start 开始任务,按 storeRoles 选择性返回 StoreSpec 流集合(含 downloaded 与 derived)、WorkResponse 或错误
	// 调用方负责关闭各 StoreSpec.ReadCloser
	Start(ctx context.Context, task *entity.Task, storeRoles []string) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error)

	// Pause 暂停任务（任务级，广播到全部 stream）
	Pause(ctx context.Context, param *sdkdto.TaskResParam) error

	// Stop 停止任务（任务级）
	Stop(ctx context.Context, param *sdkdto.TaskResParam) error

	// Resume 恢复任务:按 StreamOffsets 续传,返回新的 StoreSpec 流集合
	Resume(ctx context.Context, param *sdkdto.TaskResumeParam) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error)
}

// WorkInfoSaver 作品完整信息保存接口
type WorkInfoSaver interface {
	SaveWorkInfo(ctx context.Context, task *entity.Task, workResp *sdkdto.WorkResponse) (int64, error)
}

// WorkMetaLoader 已有作品命名元数据加载接口
// 资源板块单独重下(未跑作品元数据板块)时,从已有作品获取文件名模板所需元数据(作者等),与板块选择解耦
type WorkMetaLoader interface {
	LoadWorkMeta(ctx context.Context, workId int64) (*sdkdto.WorkResponse, error)
}

// ResourceSaver 资源保存接口
type ResourceSaver interface {
	Save(ctx context.Context, resource *entity.Resource) (int64, error)
	Update(ctx context.Context, resource *entity.Resource) error
}

// WorkChecker 作品查重接口
type WorkChecker interface {
	GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*entity.Work, error)
	// 批量查重：siteIds[i] 与 siteWorkIds[i] 一一对应
	ListBySiteAndSiteWorkIDs(ctx context.Context, siteIds []int64, siteWorkIds []string) ([]*entity.Work, error)
}

// ResourceReader 资源查询接口（查找已有作品的资源文件）
type ResourceReader interface {
	// ListByWorkId 查询作品关联的资源
	ListByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error)
	// GetEnabledByWorkId 查询作品关联的启用资源
	GetEnabledByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error)
	// GetById 根据 ID 获取资源
	GetById(ctx context.Context, id int64) (*entity.Resource, error)
}

// StoreBackupItem 单个 Store 的备份条目（由 backup 包定义具体实现，此处仅引用类型）
type StoreBackupItem = backup.StoreBackupItem

// StoreBackupOrchestrator 资源存储备份编排器接口
// 封装替换场景下作品 Resource 指定类型 PersistentStore 的备份和还原（板块隔离：按需只备份单一类型）
// 当前业务中一个 Work 恰好对应一条 Resource，接口以 workId 为入参
type StoreBackupOrchestrator interface {
	// BackupStores 备份作品 Resource 指定类型的 Store，返回备份清单
	BackupStores(ctx context.Context, workId int64, types ...backup.StoreType) []*StoreBackupItem
	// RestoreAllStores 从备份清单还原所有 Store 并更新对应 Resource
	// 仅还原 BackupID > 0 的条目；BackupID == 0 的条目跳过（对应 Resource 字段保持 null）
	RestoreAllStores(ctx context.Context, items []*StoreBackupItem)
}

// StoreStreamer 创建存储记录并返回 StoreWriter
type StoreStreamer interface {
	StoreStream(ctx context.Context, relPath string, fileName string) (storeId int64, writer persistentStore.StoreWriter, err error)
	ResumeStream(ctx context.Context, storeId int64, offset int64) (writer persistentStore.StoreWriter, err error)
}

// StoreFileCleaner 事务失败时清理磁盘文件
type StoreFileCleaner interface {
	CleanupFile(relPath string)
}

// Transactor 事务执行器接口
type Transactor interface {
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// PendingResourceUpdater 任务 pending_resource_id 同步更新接口（用于事务内直接写 DB）
type PendingResourceUpdater interface {
	UpdatePendingResourceID(ctx context.Context, taskId int64, resourceID sql.NullInt64) error
}

// StoreReader 查询 PersistentStore 记录
type StoreReader interface {
	GetById(ctx context.Context, id int64) (*entity.PersistentStore, error)
	GetAbsPath(store *entity.PersistentStore) string
}

// ResourceStoreReader resource_store 关联查询接口(多轨续传按 role 遍历 store)
type ResourceStoreReader interface {
	ListByResourceId(ctx context.Context, resourceId int64) ([]*entity.ResourceStore, error)
}

// ResourceStoreWriter resource_store 关联写入接口(saveResource 多 store 挂载)
type ResourceStoreWriter interface {
	SaveBatch(ctx context.Context, stores []*entity.ResourceStore) error
	DeleteByResourceIdAndTypes(ctx context.Context, resourceId int64, storeTypes []string) error
}

// TaskDeps ManagedTask 的共享依赖集合
// 将 NewManager 和 NewManagedTask 的大量参数收敛为一个结构体，新增依赖只需改此处
type TaskDeps struct {
	WorkInfoSaver           WorkInfoSaver
	WorkMetaLoader          WorkMetaLoader
	ResourceSaver           ResourceSaver
	WorkDirProvider         WorkDirProvider
	FileNameFormatProvider  FileNameFormatProvider
	WorkChecker             WorkChecker
	ResourceReader          ResourceReader
	StoreBackupOrchestrator StoreBackupOrchestrator
	ResourceUpdater         ResourceSaver // 替换场景更新 Resource 的 Store 字段
	Pusher                  TaskProgressPusher
	StoreStreamer           StoreStreamer
	StoreReader             StoreReader
	ResourceStoreReader     ResourceStoreReader
	ResourceStoreWriter     ResourceStoreWriter
	Transactor              Transactor
	PendingResourceUpdater  PendingResourceUpdater
	StoreFileCleaner        StoreFileCleaner
}

// ==== 多流控制器 ====

// streamState 单条流的执行状态
type streamState int32

const (
	streamDownloading streamState = iota
	streamCompleted
	streamPaused
	streamFailed
	streamCanceled
)

// streamResultKind 单条流 copyLoop 的结果分类
type streamResultKind int

const (
	resultOK streamResultKind = iota // 完成
	resultPaused
	resultCanceled
	resultFailed
)

// streamResult 单条流 copyLoop 的结果
type streamResult struct {
	kind   streamResultKind
	errMsg string
}

// streamController 管理单个 store 的传输(downloaded/derived 通用:reader→store 拷贝)
type streamController struct {
	role        string                        // store_type(main/thumbnail/videoTrack/...)
	generation  string                        // downloaded | derived
	format      string                        // 文件扩展名
	size        int64                         // 远程大小;-1 未知
	suggestName string                        // 插件建议文件名
	continuable bool                          // 是否支持续传(derived 恒为 false)

	reader      io.ReadCloser                  // 资源数据流(由调用方关闭)
	storeWriter persistentStore.StoreWriter    // 当前写入的 StoreWriter
	storeId     int64                          // PersistentStore 记录 ID
	relPath     string                         // StoreStream 的相对路径(事务回滚/清理用)
	written     int64                          // 已写入字节数(mu 保护)
	state       atomic.Int32                   // streamState
	mu          sync.Mutex                     // 保护 written 与 drain 期间的 reader/storeWriter 访问
}

// newStreamController 构建单流控制器
func newStreamController(spec *sdkdto.StoreSpec, storeId int64, writer persistentStore.StoreWriter, relPath string) *streamController {
	sc := &streamController{
		role:        spec.Role,
		generation:  spec.Generation,
		format:      spec.Format,
		size:        spec.Size,
		suggestName: spec.SuggestName,
		storeWriter: writer,
		storeId:     storeId,
		relPath:     relPath,
		reader:      spec.ReadCloser,
	}
	if spec.Continuable != nil {
		sc.continuable = *spec.Continuable
	}
	return sc
}

// ManagedTask 任务运行控制结构体
type ManagedTask struct {
	taskId   int64
	parentId int64 // 父任务ID，0表示无父任务
	state    atomic.Int32
	ctx      context.Context
	cancel   context.CancelFunc

	// 任务完成信号
	done     chan struct{}
	doneOnce sync.Once

	// executeTask goroutine 生命周期信号:goroutine 启动时创建(开),退出时关闭。
	// prepareForResume 据此等待旧 goroutine 退出后再改写共享状态(ctx/streams/pauseCh),避免竞态。
	runExited chan struct{}
	runMu     sync.Mutex

	// 任务执行器（通过接口调用）
	pluginExec TaskExecutor

	// 执行模式（Full/ResourceOnly/WorkInfo/Thumbnail）
	runMode runMode

	// 共享依赖
	deps *TaskDeps

	// 备份清单（用于任务失败时还原）
	storeBackupItems []*StoreBackupItem
	isReplace        bool
	// 跳过重复检查（替换确认后的第二次 run）
	skipDuplicateCheck bool
	// 跨重启续传标记（从数据库恢复的暂停任务）
	resumeFromDB bool
	// 已有作品 ID（第一次 run 检测到重复时设置，第二次 run 时用于备份）
	existingWorkId int64

	// 当前错误信息（仅 TaskStateFailed 时有效，通过 onStateChange 回调传递到 Manager）
	errorMessage string

	// 任务信息
	task   *entity.Task
	workId int64

	// 作品信息响应(Start/Resume 返回;供文件名模板 token data;不再承载资源细节)
	workResp *sdkdto.WorkResponse

	// 多流控制器集合(按本次所选 storeRoles 过滤后的 spec 构建)
	streams []*streamController

	// 暂停/恢复协调通道(close 广播到全部 stream goroutine;prepareForResume 重建)
	pauseCh  chan struct{}
	pauseMu  sync.Mutex

	// atomic 进度快照字段，供 BuildSnapshot 并发安全读取
	progressTotal    atomic.Int64 // 资源总大小
	progressFinished atomic.Int64 // 已下载字节数


	// 回调函数
	onStateChange      func(taskId int64, oldState, newState TaskState, errMsg string)
	onProgress         func(taskId int64, total int64, finished int64)
	onResourceIDUpdate func(taskId int64, resourceID sql.NullInt64)
}

// NewManagedTask 创建托管任务
func NewManagedTask(taskId, parentId int64, task *entity.Task, pluginExec TaskExecutor, deps *TaskDeps) *ManagedTask {
	ctx, cancel := context.WithCancel(context.Background())
	return &ManagedTask{
		taskId:     taskId,
		parentId:   parentId,
		state:      atomic.Int32{},
		ctx:        ctx,
		cancel:     cancel,
		done:       make(chan struct{}),
		pluginExec: pluginExec,
		deps:       deps,
		task:       task,
		workId:     taskId,
		pauseCh:    make(chan struct{}),
	}
}

// run 核心执行逻辑入口，按 runMode 分流到对应板块
func (m *ManagedTask) run() runResult {
	// 最先注册，最后执行：任务失败时还原已备份的 Store
	// 使用 context.Background() 而非 m.ctx，确保任务被停止（context 已取消）后还原操作仍能正常执行
	defer func() {
		if m.GetState() == TaskStateFailed && len(m.storeBackupItems) > 0 {
			logger.Log.Infof("[TaskManager] 任务 %d 失败，开始还原 %d 个已备份 Store", m.taskId, len(m.storeBackupItems))
			m.deps.StoreBackupOrchestrator.RestoreAllStores(context.Background(), m.storeBackupItems)
			m.storeBackupItems = nil
		}
	}()
	// panic recovery：后注册，先执行
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d panic: %v", m.taskId, r)
			m.setFailed(fmt.Sprintf("任务执行 panic: %v", r))
		}
	}()

	// 防止 goroutine 覆盖已被 Pause() 设置的暂停状态
	// 场景：executeTask 的 ctx.Done 检查通过后、本行执行前，Pause() 刚好 cancel + setPaused
	if m.ctx.Err() != nil {
		if s := TaskState(m.state.Load()); s == TaskStatePausing || s == TaskStatePaused {
			return runResultPaused
		}
	}
	m.setState(TaskStateProcessing)
	logger.Log.Infof("[TaskManager] run() 入口: taskId=%d, runMode={workInfo:%v, stores:%v}, PendingResourceID={Valid:%v, Int64:%d}, continuable=%v", m.taskId, m.runMode.hasWorkInfo(), m.runMode.storeRoles, m.task.PendingResourceID.Valid, m.task.PendingResourceID.Int64, m.task.Continuable.Valid)

	// 统一走板块组合执行（全集等价完整下载含查重；真子集按所选板块）
	return m.runSectionCombo()
}

// runSectionCombo 板块组合执行:严格按 runMode 所选板块执行
// 含任一资源板块(含全集)时走查重(ConfirmReplace 两段式)并产生终态;仅 workInfo 为非终态(保持执行前状态、不持久化)
func (m *ManagedTask) runSectionCombo() runResult {
	// workdir 检查（资源板块需要，置于查重前避免无效确认）
	if m.runMode.hasAnyStore() && m.deps.WorkDirProvider.GetWorkDir() == "" {
		logger.Log.Errorf("[TaskManager] 任务 %d 失败: 未配置资源库目录", m.taskId)
		return m.comboFail("未配置资源库目录，请先在设置中指定资源库保存位置")
	}

	// 含 main：查重（fallback；主路径在 Manager.batchCheckDuplicates）
	if m.runMode.hasStore(entity.StoreTypeMain) && !m.skipDuplicateCheck && m.deps.WorkChecker != nil &&
		m.task.SiteID.Valid && m.task.SiteWorkID.Valid && m.task.SiteWorkID.String != "" {
		existing, err := m.deps.WorkChecker.GetBySiteAndSiteWorkID(m.ctx, m.task.SiteID.Int64, m.task.SiteWorkID.String)
		if err == nil && existing != nil {
			m.existingWorkId = existing.GetID()
			existingWorkName := ""
			if existing.SiteWorkName.Valid {
				existingWorkName = existing.SiteWorkName.String
			}
			if m.deps.Pusher != nil {
				m.deps.Pusher.PushDuplicateDetected(m.taskId, m.task.TaskName.String, existing.GetID(), existingWorkName)
			}
			m.setState(TaskStateWaitingForInput)
			return runResultNeedConfirm
		}
	}

	// workId 定位 + 替换判定
	// 任何资源板块(main/thumbnail/...)在已有作品上重执行都视为替换,需备份旧 store;不限于 main
	if m.runMode.hasStore(entity.StoreTypeMain) {
		// 查重命中(existingWorkId>0，确认后重入或 batchCheckDuplicates 设置)
		if m.existingWorkId > 0 {
			m.workId = m.existingWorkId
			m.existingWorkId = 0
			m.isReplace = true
		} else if !m.runMode.hasWorkInfo() {
			// 含 main 的重执行必须定位到已有作品(否则无处挂载主资源)
			logger.Log.Errorf("[TaskManager] 任务 %d 资源重执行未定位到作品", m.taskId)
			return m.comboFail("未找到任务对应的作品，无法重新下载资源")
		}
		// 含 workInfo 且查重未命中：workInfo 板块的 SaveWorkInfo 会提供 workId(新作品,非替换)
	} else if !m.runMode.hasWorkInfo() && m.runMode.hasAnyStore() {
		// 非 main 资源板块(纯缩略图等):由任务记录定位已有作品,标记为替换
		workId, err := m.resolveWorkIdByTask()
		if err != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d 定位作品失败: %v", m.taskId, err)
			return m.comboFail(fmt.Sprintf("定位作品失败: %v", err))
		}
		m.workId = workId
		m.isReplace = true
	}

	// 替换场景:备份所选板块对应的旧 store。
	// 任一被重执行的板块,只要该类型 store 在已有作品上存在就备份(备份→生成→失败还原,统一替换语义)。
	if m.isReplace && m.runMode.hasAnyStore() {
		m.storeBackupItems = m.deps.StoreBackupOrchestrator.BackupStores(m.ctx, m.workId, toBackupStoreTypes(m.runMode.storeRoles)...)
	}

	// 板块 A：作品信息（CreateWorkInfo + SaveWorkInfo，提供 workId 与文件名模板数据）
	var workResp *sdkdto.WorkResponse
	if m.runMode.hasWorkInfo() {
		var err error
		workResp, err = m.pluginExec.CreateWorkInfo(m.ctx, m.task)
		if err != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d CreateWorkInfo 失败: %v", m.taskId, err)
			return m.comboFail(fmt.Sprintf("创建作品信息失败: %v", err))
		}
		savedWorkId, err := m.deps.WorkInfoSaver.SaveWorkInfo(m.ctx, m.task, workResp)
		if err != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d 保存作品信息失败: %v", m.taskId, err)
			return m.comboFail(fmt.Sprintf("保存作品信息失败: %v", err))
		}
		m.workId = savedWorkId
	}

	// 资源板块:有任意 store 时,Start 按所选 storeRoles 选择性产出
	if m.runMode.hasAnyStore() {
		specs, startResp, err := m.pluginExec.Start(m.ctx, m.task, m.runMode.storeRoles)
		if err != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d Start 失败: %v", m.taskId, err)
			return m.comboFail(fmt.Sprintf("获取资源流集合失败: %v", err))
		}
		// 合并作品元数据到 startResp(供文件名模板 resolveMainPath 使用):
		// 本次跑了作品元数据板块(A)则用其结果;否则(资源板块单独重下)从已有作品加载命名元数据
		m.mergeWorkMetaForNaming(startResp, workResp)
		selected := m.filterSpecsByRoles(specs)
		// 防御性排空未选资源角色的 reader:正常情况下插件已按 storeRoles 只产出所选 role(selected==specs,无操作);
		// 若插件多产出了未选 role,其 io.Pipe 无消费者会永久阻塞 demux(多流复用一条 gRPC stream),此处兜底排空
		m.drainUnselectedReaders(specs, selected)
		if len(selected) == 0 {
			logger.Log.Errorf("[TaskManager] 任务 %d 插件未产出所选资源角色: %v", m.taskId, m.runMode.storeRoles)
			return m.comboFail("插件未产出所选资源类型")
		}
		return m.startDownload(selected, startResp)
	}

	// 无资源板块(纯 workInfo):非终态成功,保持执行前状态、不持久化
	m.finishNonTerminalSection("")
	return runResultDone
}

// comboFail 组合执行失败处理
// 暂停则返回 Paused；含资源板块为终态失败（PushError + setFailed + run() defer 还原备份），无资源板块为非终态（finishNonTerminalSection 内部 PushError + 恢复执行前状态）
func (m *ManagedTask) comboFail(errMsg string) runResult {
	if m.abortedByPause() {
		return runResultPaused
	}
	if m.runMode.hasAnyStore() {
		if m.deps.Pusher != nil {
			m.deps.Pusher.PushError(m.taskId, errMsg)
		}
		m.setFailed(errMsg)
	} else {
		m.finishNonTerminalSection(errMsg)
	}
	return runResultDone
}

// startDownload 为每个 spec 建存储、挂 resource_store、进入多流下载循环
func (m *ManagedTask) startDownload(specs []*sdkdto.StoreSpec, workResp *sdkdto.WorkResponse) runResult {
	m.workResp = workResp

	// 解析主资源路径(供主轨与缩略图派生路径)
	mainSpec := findSpec(specs, entity.StoreTypeMain)
	if mainSpec == nil {
		// 无主轨(如纯缩略图重下):以首个 spec 占位解析主路径
		if len(specs) > 0 {
			mainSpec = specs[0]
		}
	}
	var mainRelPath, mainFileName string
	if mainSpec != nil {
		mainRelPath, mainFileName = m.resolveMainPath(mainSpec, workResp)
	}

	// 事务:为每个 spec 建 StoreStream + 挂 resource_store + Resource Save + PendingResourceID 更新
	streams := make([]*streamController, 0, len(specs))
	txErr := m.deps.Transactor.ExecInTransaction(context.Background(), func(txCtx context.Context) error {
		mounts := make([]pendingMount, 0, len(specs))
		for _, spec := range specs {
			relPath, fileName := m.resolveStorePath(spec, mainRelPath, mainFileName)
			storeId, writer, storeErr := m.deps.StoreStreamer.StoreStream(txCtx, relPath, fileName)
			if storeErr != nil {
				return storeErr
			}
			streams = append(streams, newStreamController(spec, storeId, writer, relPath))
			mounts = append(mounts, pendingMount{role: spec.Role, generation: spec.Generation, storeId: storeId})
		}

		// 保存 Resource(替换场景更新 / 新建场景创建) + 挂 resource_store
		resourceId, resourceErr := m.saveResource(txCtx, m.workId, mounts)
		if resourceErr != nil {
			return resourceErr
		}

		// 同步更新 pending_resource_id（事务内直接写 DB）
		m.task.PendingResourceID = sql.NullInt64{Int64: resourceId, Valid: true}
		return m.deps.PendingResourceUpdater.UpdatePendingResourceID(txCtx, m.taskId, m.task.PendingResourceID)
	})
	if txErr != nil {
		// 事务回滚：DB 记录已全部回滚，需显式关闭句柄并清理文件
		for _, s := range streams {
			if s.storeWriter != nil {
				s.storeWriter.Close()
			}
			m.deps.StoreFileCleaner.CleanupFile(s.relPath)
		}
		streams = nil
		logger.Log.Errorf("[TaskManager] 任务 %d 创建资源事务失败: %v", m.taskId, txErr)
		if m.abortedByPause() {
			return runResultPaused
		}
		m.setFailed(fmt.Sprintf("创建资源失败: %v", txErr))
		return runResultDone
	}

	// setup 阶段暂停在事务窗口内命中:Pause 此时 len(streams)==0 走 cancel 路径,
	// 事务用 context.Background 不受影响仍提交;此处清理已建句柄并返回暂停,避免带着已取消的 ctx 进入 downloadLoop
	if m.abortedByPause() {
		for _, s := range streams {
			if s.storeWriter != nil {
				s.storeWriter.Sync()
				s.storeWriter.Close()
			}
			if s.reader != nil {
				s.reader.Close()
			}
		}
		m.streams = nil
		return runResultPaused
	}

	m.streams = streams
	return m.downloadLoop()
}

// resolveWorkIdByTask 由任务记录的 (site_id, site_work_id) 定位作品 ID
func (m *ManagedTask) resolveWorkIdByTask() (int64, error) {
	if m.deps.WorkChecker == nil || !m.task.SiteID.Valid || !m.task.SiteWorkID.Valid || m.task.SiteWorkID.String == "" {
		return 0, fmt.Errorf("任务缺少 site_id/site_work_id，无法定位作品")
	}
	work, err := m.deps.WorkChecker.GetBySiteAndSiteWorkID(m.ctx, m.task.SiteID.Int64, m.task.SiteWorkID.String)
	if err != nil {
		return 0, fmt.Errorf("查询作品失败: %w", err)
	}
	if work == nil {
		return 0, fmt.Errorf("未找到任务对应的作品（siteWorkId=%s）", m.task.SiteWorkID.String)
	}
	return work.GetID(), nil
}

// isNonTerminalMode 是否为非终态板块组合（无资源板块）：执行不产生任务终态、不持久化任务状态
func (m *ManagedTask) isNonTerminalMode() bool {
	return !m.runMode.hasAnyStore()
}

// finishNonTerminalSection 结束 A 非终态板块：恢复执行前状态（任务的 DB status），不持久化
// errMsg 非空表示失败，推送失败通知；空表示成功
func (m *ManagedTask) finishNonTerminalSection(errMsg string) {
	if errMsg != "" {
		m.errorMessage = errMsg
		if m.deps.Pusher != nil {
			m.deps.Pusher.PushError(m.taskId, errMsg)
		}
	}
	// 回到执行前状态（任务的 DB status），onStateChange 按 runMode 跳过持久化
	m.setState(TaskState(m.task.Status))
}

// pendingMount saveResource 挂载单个 store 的中间结构
type pendingMount struct {
	role       string
	generation string
	storeId    int64
}

// saveResource 保存 Resource(事务内调用)并挂 resource_store 行
// 替换场景:查询已有 Resource 并更新;新建场景:创建新 Resource
// 双写:既回填旧列(WorkStoreID/ThumbnailStoreID,供未迁移的 backup/recycleBin/work/search 消费),也写 resource_store 行(新模型,供多轨续传与阶段6)
func (m *ManagedTask) saveResource(ctx context.Context, workId int64, mounts []pendingMount) (int64, error) {
	var resourceId int64

	if m.isReplace {
		existing := m.findReplaceResource(ctx, workId)
		if existing != nil {
			applyMountsToResource(existing, mounts)
			existing.ResourceComplete = 0
			if err := m.deps.ResourceUpdater.Update(ctx, existing); err != nil {
				return 0, fmt.Errorf("更新 Resource 失败: %w", err)
			}
			resourceId = existing.GetID()
		}
	}

	if resourceId == 0 {
		// 非替换场景或替换未定位到已有 Resource:创建新 Resource
		resource := entity.NewResource()
		resource.WorkID = workId
		resource.TaskID = m.task.GetID()
		resource.Enabled = true
		applyMountsToResource(resource, mounts)
		resource.ResourceComplete = 0 // 下载未完成

		var err error
		resourceId, err = m.deps.ResourceSaver.Save(ctx, resource)
		if err != nil {
			return 0, fmt.Errorf("保存资源到数据库失败: %w", err)
		}
	}

	// 挂 resource_store 行(新模型):先清同 role 旧行,再插入本次产出
	if err := m.mountResourceStores(ctx, resourceId, mounts); err != nil {
		return 0, fmt.Errorf("挂载 resource_store 失败: %w", err)
	}

	return resourceId, nil
}

// findReplaceResource 替换场景定位已有 Resource(优先备份清单,回退 workId 查询)
func (m *ManagedTask) findReplaceResource(ctx context.Context, workId int64) *entity.Resource {
	for _, item := range m.storeBackupItems {
		if item.StoreType == backup.StoreTypeWork {
			existing, err := m.deps.ResourceReader.GetById(ctx, item.ResourceID)
			if err != nil || existing == nil {
				logger.Log.Warnf("[TaskManager] 查询已有 Resource(id=%d) 失败: %v", item.ResourceID, err)
				continue
			}
			return existing
		}
	}
	resources, queryErr := m.deps.ResourceReader.GetEnabledByWorkId(ctx, workId)
	if queryErr != nil {
		logger.Log.Warnf("[TaskManager] 查询作品 %d 资源失败: %v", workId, queryErr)
		return nil
	}
	if len(resources) > 0 {
		return resources[0]
	}
	return nil
}

// applyMountsToResource 回填 Resource 旧列(main→WorkStoreID,thumbnail→ThumbnailStoreID),向后兼容未迁移消费端
func applyMountsToResource(r *entity.Resource, mounts []pendingMount) {
	for _, mt := range mounts {
		switch mt.role {
		case entity.StoreTypeMain:
			r.WorkStoreID = sql.NullInt64{Int64: mt.storeId, Valid: true}
		case entity.StoreTypeThumbnail:
			r.ThumbnailStoreID = sql.NullInt64{Int64: mt.storeId, Valid: true}
		}
	}
}

// mountResourceStores 写入 resource_store 行(替换本次产出 role 的旧关联后再插入)
func (m *ManagedTask) mountResourceStores(ctx context.Context, resourceId int64, mounts []pendingMount) error {
	if m.deps.ResourceStoreWriter == nil {
		return nil
	}
	roles := uniqueRoles(mounts)
	if err := m.deps.ResourceStoreWriter.DeleteByResourceIdAndTypes(ctx, resourceId, roles); err != nil {
		return err
	}
	if len(mounts) == 0 {
		return nil
	}
	stores := make([]*entity.ResourceStore, 0, len(mounts))
	for i, mt := range mounts {
		s := entity.NewResourceStore()
		s.ResourceID = resourceId
		s.StoreType = mt.role
		s.Generation = mt.generation
		s.StoreID = mt.storeId
		s.OrderIdx = i
		stores = append(stores, s)
	}
	return m.deps.ResourceStoreWriter.SaveBatch(ctx, stores)
}

// downloadLoop 多流并发下载循环:每条 spec 一个 goroutine 跑 read→write→累计
// 全部 completed → Finished;任一 failed → Failed(保留已完成轨的 store);暂停 → Paused;取消 → Stop 已处理
func (m *ManagedTask) downloadLoop() runResult {
	// 关闭 reader(各 spec reader 由本方法负责)
	defer m.closeStreamReaders()

	downloadStart := time.Now()
	totalSize := m.totalStreamSize()

	var wg sync.WaitGroup
	var failedMsg atomic.Value // string
	var hasFailed atomic.Bool
	var hasCanceled atomic.Bool

	for _, s := range m.streams {
		wg.Add(1)
		go func(s *streamController) {
			defer wg.Done()
			res := s.copyLoop(m)
			switch res.kind {
			case resultOK:
				// 正常完成
			case resultPaused:
				// 由 downloadLoop 统一判定
			case resultCanceled:
				hasCanceled.Store(true)
			case resultFailed:
				hasFailed.Store(true)
				if res.errMsg != "" {
					failedMsg.Store(res.errMsg)
				}
			}
		}(s)
	}
	wg.Wait()

	logger.Log.Infof("[TaskManager] downloadLoop 结束: taskId=%d, totalSize=%d, elapsed=%v", m.taskId, totalSize, time.Since(downloadStart))

	// 暂停优先(stream 任一进入 paused 即任务级暂停)
	if m.anyStreamPaused() {
		m.setState(TaskStatePaused)
		return runResultPaused
	}
	if hasCanceled.Load() {
		// setup 阶段 pause 取消了 ctx(Pause len(streams)==0 → cancel):视为暂停,不 cleanup
		// 否则是 Stop 取消(已 setFailed)
		if m.abortedByPause() {
			return runResultPaused
		}
		return runResultDone
	}
	if hasFailed.Load() {
		msg := "下载失败"
		if v := failedMsg.Load(); v != nil {
			if s, ok := v.(string); ok && s != "" {
				msg = s
			}
		}
		m.setFailed(msg)
		return runResultDone
	}
	// 全部完成
	m.clearPendingResourceID()
	m.setState(TaskStateFinished)
	return runResultDone
}

// copyLoop 单流读取循环:read→write→累计,响应暂停/取消/EOF
func (s *streamController) copyLoop(m *ManagedTask) streamResult {
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-m.pauseSignal():
			return s.handlePause(buf)
		case <-m.ctx.Done():
			s.abort()
			s.state.Store(int32(streamCanceled))
			return streamResult{kind: resultCanceled}
		default:
		}

		n, readErr := s.reader.Read(buf)
		if n > 0 {
			written, writeErr := s.storeWriter.Write(buf[:n])
			if written > 0 {
				s.mu.Lock()
				s.written += int64(written)
				s.mu.Unlock()
			}
			if writeErr != nil {
				logger.Log.Errorf("[TaskManager] 任务 %d 写入文件失败(role=%s): %v", m.taskId, s.role, writeErr)
				s.abort()
				s.state.Store(int32(streamFailed))
				return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("写入文件失败: %v", writeErr)}
			}
			m.reportProgress()
		}
		if readErr != nil {
			if readErr == io.EOF {
				return s.handleEOF(m)
			}
			// 非 EOF 读取错误:可能是暂停/停止导致或真正的读取失败
			if m.isPausing() {
				return s.handlePause(buf)
			}
			if m.isStopping() {
				s.abort()
				s.state.Store(int32(streamCanceled))
				return streamResult{kind: resultCanceled}
			}
			logger.Log.Errorf("[TaskManager] 任务 %d 下载读取失败(role=%s): %v", m.taskId, s.role, readErr)
			s.abort()
			s.state.Store(int32(streamFailed))
			return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("下载读取失败: %v", readErr)}
		}
	}
}

// handleEOF 处理 reader EOF:暂停/停止导致的 EOF 走对应路径,否则校验完整性并完成
func (s *streamController) handleEOF(m *ManagedTask) streamResult {
	// 暂停导致上游关闭产生的 EOF:drain 后置 paused
	if m.isPausing() {
		return s.handlePause(nil)
	}
	// 停止导致上游关闭产生的 EOF:不完成 store,置 canceled(Stop 已 abort + setFailed)
	if m.isStopping() {
		s.abort()
		s.state.Store(int32(streamCanceled))
		return streamResult{kind: resultCanceled}
	}
	// 完整性校验:downloaded 轨已写量不足视为不完整
	s.mu.Lock()
	written := s.written
	s.mu.Unlock()
	if s.generation == entity.GenerationDownloaded && s.size > 0 && written < s.size {
		logger.Log.Errorf("[TaskManager] 任务 %d 下载不完整(role=%s): 已下载 %d / 预期 %d", m.taskId, s.role, written, s.size)
		s.abort()
		s.state.Store(int32(streamFailed))
		return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("%s 下载不完整: 已下载 %d / 预期 %d", s.role, written, s.size)}
	}
	if err := s.storeWriter.Complete(); err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d Complete 失败(role=%s): %v", m.taskId, s.role, err)
		s.state.Store(int32(streamFailed))
		return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("完成存储失败: %v", err)}
	}
	s.state.Store(int32(streamCompleted))
	return streamResult{kind: resultOK}
}

// handlePause 排空缓冲区、同步并关闭写入器、置 paused
func (s *streamController) handlePause(buf []byte) streamResult {
	if buf != nil {
		s.drain(buf)
	}
	s.storeWriter.Sync()
	s.storeWriter.Close()
	s.state.Store(int32(streamPaused))
	return streamResult{kind: resultPaused}
}

// drain 排空 reader 中所有已发送数据并写入文件,直到 reader 返回错误或 EOF
func (s *streamController) drain(buf []byte) {
	for {
		n, err := s.reader.Read(buf)
		if n > 0 {
			if written, writeErr := s.storeWriter.Write(buf[:n]); writeErr == nil && written > 0 {
				s.mu.Lock()
				s.written += int64(written)
				s.mu.Unlock()
			}
		}
		if err != nil {
			return
		}
	}
}

// abort 放弃写入:关闭句柄 + 删除文件 + 删 DB 记录
func (s *streamController) abort() {
	if s.storeWriter != nil {
		s.storeWriter.Abort()
	}
}

// resumeFromPersistedState 从数据库恢复的跨重启续传(多轨)
// 任务在之前的运行中已暂停,pending_resource_id 已持久化。本方法跳过 CreateWorkInfo/SaveWorkInfo/Start,
// 按 resource_store 各轨 PersistentStore 状态计算续传偏移,调用插件 Resume 取新流集合,续接/重建 store 后进入下载循环
func (m *ManagedTask) resumeFromPersistedState() runResult {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d resumeFromPersistedState panic: %v", m.taskId, r)
			m.setFailed(fmt.Sprintf("跨重启续传 panic: %v", r))
		}
	}()

	// 同 run() 的防护
	if m.ctx.Err() != nil {
		if s := TaskState(m.state.Load()); s == TaskStatePausing || s == TaskStatePaused {
			return runResultPaused
		}
	}
	m.setState(TaskStateProcessing)

	// 1. 通过 pending_resource_id 加载 Resource 实体
	if !m.task.PendingResourceID.Valid {
		logger.Log.Warnf("[TaskManager] 任务 %d 无有效的 pending_resource_id，降级为完整重新执行", m.taskId)
		return m.run()
	}
	resource, err := m.deps.ResourceReader.GetById(m.ctx, m.task.PendingResourceID.Int64)
	if err != nil || resource == nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 加载 Resource(id=%d) 失败: %v，降级为完整重新执行", m.taskId, m.task.PendingResourceID.Int64, err)
		return m.run()
	}

	workDir := m.deps.WorkDirProvider.GetWorkDir()
	if workDir == "" {
		logger.Log.Errorf("[TaskManager] 任务 %d 失败: 未配置资源库目录", m.taskId)
		m.setFailed("未配置资源库目录，请先在设置中指定资源库保存位置")
		return runResultDone
	}

	// 2. 读 resource_store 各轨 store 关联
	if m.deps.ResourceStoreReader == nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 未配置 ResourceStoreReader，降级为完整重新执行", m.taskId)
		return m.run()
	}
	storeRows, err := m.deps.ResourceStoreReader.ListByResourceId(m.ctx, resource.GetID())
	if err != nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 查询 resource_store 失败: %v，降级为完整重新执行", m.taskId, err)
		return m.run()
	}
	if len(storeRows) == 0 {
		logger.Log.Warnf("[TaskManager] 任务 %d Resource 无 resource_store 关联，降级为完整重新执行", m.taskId)
		return m.run()
	}

	m.workId = resource.WorkID

	// 3. 计算各 downloaded 轨续传偏移 + 收集未完成 derived 轨(整轨重产)
	// 已完成(状态 Complete 且文件存在)的轨道跳过;downloaded 未完成按文件大小算偏移;derived 未完成收集到 incompleteDerivedRoles
	streamOffsets := map[string]int64{}
	completedRoles := map[string]struct{}{}
	var incompleteDerivedRoles []string
	for _, row := range storeRows {
		store, storeErr := m.deps.StoreReader.GetById(m.ctx, row.StoreID)
		if storeErr != nil || store == nil {
			// store 记录丢失:downloaded 整轨重下(offset=0),derived 整轨重产
			if row.Generation == entity.GenerationDerived {
				incompleteDerivedRoles = append(incompleteDerivedRoles, row.StoreType)
			} else {
				streamOffsets[row.StoreType] = 0
			}
			continue
		}
		absPath := m.deps.StoreReader.GetAbsPath(store)
		info, statErr := os.Stat(absPath)
		if store.Status == entity.StoreStatusComplete && statErr == nil {
			// 该轨已完成:不进入 Resume/重产
			completedRoles[row.StoreType] = struct{}{}
			continue
		}
		// 未完成:downloaded 按偏移续传;derived 整轨重产
		if row.Generation == entity.GenerationDerived {
			incompleteDerivedRoles = append(incompleteDerivedRoles, row.StoreType)
		} else if statErr != nil {
			streamOffsets[row.StoreType] = 0 // 文件缺失:整轨重下
		} else {
			streamOffsets[row.StoreType] = info.Size()
		}
	}

	logger.Log.Infof("[TaskManager] 任务 %d 跨重启续传: resourceID=%d, offsets=%v, completed=%v, regenDerived=%v", m.taskId, resource.GetID(), streamOffsets, completedRoles, incompleteDerivedRoles)

	// 4. 调用插件 Resume(按 StreamOffsets 续传未完成 downloaded 轨)
	param := &sdkdto.TaskResumeParam{
		Task:          dto.NewTaskDTO(m.task),
		StreamOffsets: streamOffsets,
	}
	specs, newResp, err := m.pluginExec.Resume(m.ctx, param)
	if err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d 跨重启 Resume 失败: %v", m.taskId, err)
		m.setFailed(fmt.Sprintf("跨重启续传失败: %v", err))
		return runResultDone
	}
	if newResp == nil {
		newResp = &sdkdto.WorkResponse{}
	}
	// 缺陷2: resume 也加载作品命名元数据(与 runSectionCombo 一致),避免重建路径落 unknownAuthor
	m.mergeWorkMetaForNaming(newResp, nil)
	m.workResp = newResp

	// 缺陷3: 未完成的 derived 轨由 Start 重新生成(Resume 只续传 downloaded;derived 一次性产物未完成须整轨重产)
	if len(incompleteDerivedRoles) > 0 {
		derivedSpecs, _, startErr := m.pluginExec.Start(m.ctx, m.task, incompleteDerivedRoles)
		if startErr != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d 重产 derived 轨 %v 失败: %v", m.taskId, incompleteDerivedRoles, startErr)
			m.setFailed(fmt.Sprintf("重产资源失败: %v", startErr))
			return runResultDone
		}
		specs = append(specs, derivedSpecs...)
	}

	if len(specs) == 0 {
		// 无未完成轨道需续传/重产:任务直接完成
		logger.Log.Infof("[TaskManager][dispatch] taskId=%d resumeFromPersistedState 无未完成轨道,直接 Finished(streamOffsets=%v regenDerived=%v)", m.taskId, streamOffsets, incompleteDerivedRoles)
		m.clearPendingResourceID()
		m.setState(TaskStateFinished)
		return runResultDone
	}

	// 5. 为每个返回的 spec 续接(continuable downloaded)或重建 store,构建 streamController
	mainSpec := findSpec(specs, entity.StoreTypeMain)
	if mainSpec == nil && len(specs) > 0 {
		mainSpec = specs[0]
	}
	var mainRelPath, mainFileName string
	if mainSpec != nil {
		mainRelPath, mainFileName = m.resolveMainPath(mainSpec, newResp)
	}

	streams := make([]*streamController, 0, len(specs))
	txErr := m.deps.Transactor.ExecInTransaction(context.Background(), func(txCtx context.Context) error {
		mounts := make([]pendingMount, 0, len(specs))
		for _, spec := range specs {
			relPath, fileName := m.resolveStorePath(spec, mainRelPath, mainFileName)
			existingRow := findStoreRow(storeRows, spec.Role)
			// continuable 的 downloaded 轨且有正偏移:用已有 storeId + ResumeStream 续传
			// 写入偏移:插件指定(spec.ResumeWriteOffset)优先,否则用主程序 stat 的 streamOffsets
			if spec.Generation == entity.GenerationDownloaded && existingRow != nil && streamOffsets[spec.Role] > 0 {
				writeOffset := streamOffsets[spec.Role]
				if spec.ResumeWriteOffset != nil {
					writeOffset = *spec.ResumeWriteOffset
				}
				writer, resumeErr := m.deps.StoreStreamer.ResumeStream(txCtx, existingRow.StoreID, writeOffset)
				if resumeErr != nil {
					return resumeErr
				}
				sc := newStreamController(spec, existingRow.StoreID, writer, relPath)
				sc.written = writeOffset
				streams = append(streams, sc)
				mounts = append(mounts, pendingMount{role: spec.Role, generation: spec.Generation, storeId: existingRow.StoreID})
			} else {
				// derived 或 offset=0 的 downloaded:StoreStream 重建
				storeId, writer, storeErr := m.deps.StoreStreamer.StoreStream(txCtx, relPath, fileName)
				if storeErr != nil {
					return storeErr
				}
				streams = append(streams, newStreamController(spec, storeId, writer, relPath))
				mounts = append(mounts, pendingMount{role: spec.Role, generation: spec.Generation, storeId: storeId})
			}
		}
		// 更新 resource_store(新模型,替换本次产出 role 的关联)
		if err := m.mountResourceStores(txCtx, resource.GetID(), mounts); err != nil {
			return err
		}
		// 缺陷1: 同步回填 Resource 旧列(WorkStoreID/ThumbnailStoreID),保持双写一致。
		// 阶段6 前前端/backup/recycleBin 仍读旧列;不回填会导致 resume 重建后旧列指向被遗弃的旧 store → 前端看到 0 字节
		applyMountsToResource(resource, mounts)
		return m.deps.ResourceUpdater.Update(txCtx, resource)
	})
	if txErr != nil {
		for _, s := range streams {
			if s.storeWriter != nil {
				s.storeWriter.Close()
			}
			m.deps.StoreFileCleaner.CleanupFile(s.relPath)
		}
		streams = nil
		logger.Log.Errorf("[TaskManager] 任务 %d 跨重启续传事务失败: %v", m.taskId, txErr)
		if m.abortedByPause() {
			return runResultPaused
		}
		m.setFailed(fmt.Sprintf("跨重启续传创建存储失败: %v", txErr))
		return runResultDone
	}

	m.streams = streams
	return m.downloadLoop()
}

// Pause 暂停任务(任务级,广播到全部活跃 stream)
func (m *ManagedTask) Pause() error {
	if m.state.Load() != int32(TaskStateProcessing) {
		return ErrTaskNotProcessing
	}
	m.setState(TaskStatePausing)

	// 下载尚未开始的场景（setup 阶段），直接取消 context
	if len(m.streams) == 0 {
		logger.Log.Infof("[TaskManager] Pause: taskId=%d 在 setup 阶段暂停，直接取消", m.taskId)
		m.cancel()
		m.setState(TaskStatePaused)
		return nil
	}

	logger.Log.Infof("[TaskManager] Pause: taskId=%d 在 download 阶段暂停, PendingResourceID={Valid:%v, Int64:%d}",
		m.taskId, m.task.PendingResourceID.Valid, m.task.PendingResourceID.Int64)

	// ① 广播暂停信号(close 让全部 stream goroutine 进入 drain)
	m.closePauseCh()

	// ② 通知插件暂停(插件关闭上游 → reader EOF → stream drain 完成)
	param := &sdkdto.TaskResParam{
		Task:       dto.NewTaskDTO(m.task),
		ResourceID: m.task.PendingResourceID.Int64,
	}
	if err := m.pluginExec.Pause(m.ctx, param); err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d 插件 Pause 失败: %v", m.taskId, err)
	}

	return nil
}

// prepareForResume 重置任务的运行时状态，准备重新调度
// 由 ResumeTaskTree 在 tryDispatch 前调用
func (m *ManagedTask) prepareForResume() {
	// 等待上一次 executeTask goroutine 退出,避免与旧 goroutine 竞争共享状态(ctx/streams/pauseCh)。
	// 频繁启停下若不等待,旧 goroutine 的 downloadLoop defer(closeStreamReaders)可能破坏新 goroutine 的 streams,
	// 或旧 goroutine 卡住持有信号量槽 → 并行度下降。
	m.runMu.Lock()
	exited := m.runExited
	m.runMu.Unlock()
	if exited != nil {
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			logger.Log.Warnf("[TaskManager] 任务 %d prepareForResume 等待旧 goroutine 退出超时(可能卡住,继续 resume)", m.taskId)
		}
	}

	m.cancel()
	m.ctx, m.cancel = context.WithCancel(context.Background())
	// 关闭旧 reader + 重建暂停通道
	m.closeStreamReaders()
	m.streams = nil
	m.pauseMu.Lock()
	m.pauseCh = make(chan struct{})
	m.pauseMu.Unlock()
	// 有 PendingResourceID 走 resumeFromPersistedState（内部按各轨 store 状态续传/重产）
	// 无 PendingResourceID 走 run()（从头执行）
	m.resumeFromDB = m.task.PendingResourceID.Valid
	logger.Log.Infof("[TaskManager] prepareForResume: taskId=%d, PendingResourceID={Valid:%v, Int64:%d}, resumeFromDB=%v",
		m.taskId, m.task.PendingResourceID.Valid, m.task.PendingResourceID.Int64, m.resumeFromDB)
}

// Stop 停止任务(任务级,广播取消全部 stream)
func (m *ManagedTask) Stop() {
	m.setState(TaskStateStopping)
	m.cancel() // 触发 context 取消（中断各 stream 的 downloadLoop）

	for _, s := range m.streams {
		s.abort()
	}

	param := &sdkdto.TaskResParam{
		Task:       dto.NewTaskDTO(m.task),
		ResourceID: m.task.PendingResourceID.Int64,
	}
	if err := m.pluginExec.Stop(m.ctx, param); err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d Stop 失败: %v", m.taskId, err)
	}

	m.setFailed("任务被用户停止")
}

// clearPendingResourceID 清除任务的 pending_resource_id（下载完成时调用）
func (m *ManagedTask) clearPendingResourceID() {
	m.task.PendingResourceID = sql.NullInt64{Valid: false}
	if m.onResourceIDUpdate != nil {
		m.onResourceIDUpdate(m.taskId, m.task.PendingResourceID)
	}
}

// setState 设置任务状态
func (m *ManagedTask) setState(state TaskState) {
	old := TaskState(m.state.Swap(int32(state)))
	if old == state {
		return
	}
	// 非 Failed 状态清除错误信息（重试后不应保留旧错误）
	if state != TaskStateFailed {
		m.errorMessage = ""
	}
	taskName := ""
	if m.task != nil && m.task.TaskName.Valid {
		taskName = m.task.TaskName.String
	}
	logger.Log.Infof("[TaskManager] 任务状态变更 [%s](%d): %s → %s", taskName, m.taskId, taskStateName(old), taskStateName(state))
	if m.onStateChange != nil {
		m.onStateChange(m.taskId, old, state, m.errorMessage)
	}
	if state == TaskStateFinished || state == TaskStateFailed {
		m.doneOnce.Do(func() { close(m.done) })
	}
}

// setFailed 设置任务为失败状态，并记录错误信息
// 终态清空 pending_resource_id：失败的任务不再续传，避免残留的 pending_resource_id
// 在 work/resource 被外部操作（删除/还原）后指向失效的 resource/store
func (m *ManagedTask) setFailed(errMsg string) {
	m.errorMessage = errMsg
	m.setState(TaskStateFailed)
	if m.task.PendingResourceID.Valid {
		m.clearPendingResourceID()
	}
}

// Done 返回任务完成信号 channel，任务终态（Finished/Failed）时关闭
func (m *ManagedTask) Done() <-chan struct{} {
	return m.done
}

// GetState 获取当前状态
func (m *ManagedTask) GetState() TaskState {
	return TaskState(m.state.Load())
}

// SetOnStateChange 设置状态变化回调
func (m *ManagedTask) SetOnStateChange(fn func(taskId int64, oldState, newState TaskState, errMsg string)) {
	m.onStateChange = fn
}

// SetOnProgress 设置进度回调
func (m *ManagedTask) SetOnProgress(fn func(taskId int64, total int64, finished int64)) {
	m.onProgress = fn
}

// abortedByPause 检查是否因暂停导致 context 被取消
// Pause 在 setup 阶段（streams 为空）会直接取消 context
// run() 的 setup 代码遇到此类情况应直接退出，不覆盖 Pause 已设置的 Paused 状态
func (m *ManagedTask) abortedByPause() bool {
	if m.ctx.Err() == nil {
		return false
	}
	s := m.state.Load()
	return s == int32(TaskStatePausing) || s == int32(TaskStatePaused)
}

// isPausing 任务是否处于暂停中/已暂停(setup 阶段暂停靠 ctx 取消,download 阶段靠状态判定)
func (m *ManagedTask) isPausing() bool {
	s := m.state.Load()
	return s == int32(TaskStatePausing) || s == int32(TaskStatePaused)
}

// isStopping 任务是否处于停止中(Stop 已发起,阻塞在 Read 的 stream 收到 EOF 时据此避免误完成)
func (m *ManagedTask) isStopping() bool {
	return m.state.Load() == int32(TaskStateStopping)
}

// pauseSignal 返回暂停广播通道(关闭即广播)
func (m *ManagedTask) pauseSignal() <-chan struct{} {
	m.pauseMu.Lock()
	defer m.pauseMu.Unlock()
	return m.pauseCh
}

// closePauseCh 关闭暂停通道(广播到全部 stream goroutine),幂等
func (m *ManagedTask) closePauseCh() {
	m.pauseMu.Lock()
	defer m.pauseMu.Unlock()
	select {
	case <-m.pauseCh:
		// 已关闭
	default:
		close(m.pauseCh)
	}
}

// closeStreamReaders 关闭全部 stream 的 reader
func (m *ManagedTask) closeStreamReaders() {
	for _, s := range m.streams {
		if s.reader != nil {
			s.reader.Close()
		}
	}
}

// anyStreamPaused 是否有任意 stream 进入 paused 状态
func (m *ManagedTask) anyStreamPaused() bool {
	for _, s := range m.streams {
		if streamState(s.state.Load()) == streamPaused {
			return true
		}
	}
	return false
}

// totalStreamSize 全部 stream 的远程大小之和(进度总量)
func (m *ManagedTask) totalStreamSize() int64 {
	var total int64
	for _, s := range m.streams {
		if s.size > 0 {
			total += s.size
		}
	}
	return total
}

// reportProgress 汇总全部 stream 进度并推送
func (m *ManagedTask) reportProgress() {
	if m.onProgress == nil {
		return
	}
	var total, finished int64
	for _, s := range m.streams {
		if s.size > 0 {
			total += s.size
		}
		s.mu.Lock()
		finished += s.written
		s.mu.Unlock()
	}
	m.onProgress(m.taskId, total, finished)
}

// filterSpecsByRoles 按 runMode.storeRoles 过滤 spec(空集合=全量)
func (m *ManagedTask) filterSpecsByRoles(specs []*sdkdto.StoreSpec) []*sdkdto.StoreSpec {
	if len(m.runMode.storeRoles) == 0 {
		out := make([]*sdkdto.StoreSpec, len(specs))
		copy(out, specs)
		return out
	}
	roleSet := make(map[string]struct{}, len(m.runMode.storeRoles))
	for _, r := range m.runMode.storeRoles {
		roleSet[r] = struct{}{}
	}
	var out []*sdkdto.StoreSpec
	for _, s := range specs {
		if s == nil {
			continue
		}
		if _, ok := roleSet[s.Role]; ok {
			out = append(out, s)
		}
	}
	return out
}

// drainUnselectedReaders 排空 all 中未被 selected 选中的 spec reader(读到 EOF 后关闭)
// 用于过滤后丢弃的 role:多流 gRPC demux 会向其 io.Pipe 写数据,无人消费会永久阻塞 demux
func (m *ManagedTask) drainUnselectedReaders(all, selected []*sdkdto.StoreSpec) {
	if len(all) == len(selected) {
		return
	}
	selectedSet := make(map[*sdkdto.StoreSpec]struct{}, len(selected))
	for _, s := range selected {
		selectedSet[s] = struct{}{}
	}
	for _, sp := range all {
		if sp == nil || sp.ReadCloser == nil {
			continue
		}
		if _, ok := selectedSet[sp]; ok {
			continue
		}
		reader := sp.ReadCloser
		go func() {
			_, _ = io.Copy(io.Discard, reader)
			_ = reader.Close()
		}()
	}
}

// findSpec 在 spec 集合中查找指定 role
func findSpec(specs []*sdkdto.StoreSpec, role string) *sdkdto.StoreSpec {
	for _, s := range specs {
		if s != nil && s.Role == role {
			return s
		}
	}
	return nil
}

// findStoreRow 在 resource_store 行中查找指定 role
func findStoreRow(rows []*entity.ResourceStore, role string) *entity.ResourceStore {
	for _, r := range rows {
		if r != nil && r.StoreType == role {
			return r
		}
	}
	return nil
}

// uniqueRoles 提取 mounts 中去重后的 role 列表
func uniqueRoles(mounts []pendingMount) []string {
	seen := make(map[string]struct{}, len(mounts))
	roles := make([]string, 0, len(mounts))
	for _, mt := range mounts {
		if _, ok := seen[mt.role]; ok {
			continue
		}
		seen[mt.role] = struct{}{}
		roles = append(roles, mt.role)
	}
	return roles
}

// toBackupStoreTypes 将 store_type 字符串映射为 backup.StoreType(未映射类型如 videoTrack 暂不备份,待阶段6扩展)
func toBackupStoreTypes(roles []string) []backup.StoreType {
	seen := make(map[backup.StoreType]struct{})
	var out []backup.StoreType
	for _, r := range roles {
		var t backup.StoreType
		switch r {
		case entity.StoreTypeMain:
			t = backup.StoreTypeWork
		case entity.StoreTypeThumbnail:
			t = backup.StoreTypeThumbnail
		default:
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// mergeWorkInfo 将 from(作品信息板块 A 的响应)合并到 to(Start/Resume 的响应),供文件名模板使用
func mergeWorkInfo(to, from *sdkdto.WorkResponse) {
	if to == nil || from == nil {
		return
	}
	if from.Work != nil {
		to.Work = from.Work
	}
	to.SiteAuthors = from.SiteAuthors
	to.LocalAuthors = from.LocalAuthors
	to.SiteTags = from.SiteTags
	to.LocalTags = from.LocalTags
}

// mergeWorkMetaForNaming 把作品命名元数据合并到 Start/Resume 的响应,供文件名模板使用。
// 本次执行跑了作品元数据板块(A)则用其结果;否则(资源板块单独重下)从已有作品加载命名元数据(作者等),
// 避免 ${author} 等占位符因元数据缺失回落到 unknownAuthor。
func (m *ManagedTask) mergeWorkMetaForNaming(startResp, workResp *sdkdto.WorkResponse) {
	if startResp == nil {
		return
	}
	if workResp != nil {
		mergeWorkInfo(startResp, workResp)
		return
	}
	if m.workId <= 0 || m.deps.WorkMetaLoader == nil {
		return
	}
	meta, err := m.deps.WorkMetaLoader.LoadWorkMeta(m.ctx, m.workId)
	if err != nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 加载已有作品 %d 命名元数据失败: %v", m.taskId, m.workId, err)
		return
	}
	if meta != nil {
		mergeWorkInfo(startResp, meta)
	}
}

// resolveMainPath 根据资源信息和文件名模板生成主资源(或主下载轨)的本地保存路径
// 返回 relativePath（相对于 workDir）和 fileName
func (m *ManagedTask) resolveMainPath(spec *sdkdto.StoreSpec, workResp *sdkdto.WorkResponse) (relativePath, fileName string) {
	tpl := m.deps.FileNameFormatProvider.GetFileNameFormat()

	// 模板为空时使用插件建议的文件名
	if tpl == "" {
		fileName = m.buildSuggestedFileName(spec)
		relativePath = filepath.Join("store", "resource", fileName)
		return
	}

	// 模板模式：提取占位符数据 → 格式化 → 净化 → 拼接扩展名 → 按作者分目录
	tokenData := filename.ExtractTokenData(workResp)
	formatted := filename.FormatFileName(tpl, tokenData)
	sanitizedFileName := filename.SanitizeFileName(formatted)

	ext := normalizeExt(spec.Format)
	fileName = sanitizedFileName + ext

	authorDir := filename.SanitizeFileName(tokenData.Author)

	relativePath = filepath.Join("store", "resource", authorDir, fileName)
	return
}

// resolveStorePath 按 role 解析各 store 路径:thumbnail 派生自主路径,其余(含 main/未来轨道)用主路径逻辑
func (m *ManagedTask) resolveStorePath(spec *sdkdto.StoreSpec, mainRelPath, mainFileName string) (relativePath, fileName string) {
	if spec.Role == entity.StoreTypeThumbnail {
		relativePath = buildThumbnailRelPath(mainRelPath, spec.Format)
		fileName = buildThumbnailFileName(mainFileName, spec.Format)
		return
	}
	return m.resolveMainPath(spec, m.workResp)
}

// normalizeExt 规范化扩展名(确保以 "." 开头)
func normalizeExt(format string) string {
	ext := format
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}

// buildSuggestedFileName 根据插件建议文件名构建最终文件名（含扩展名）
// 优先使用插件 SuggestName，仅保留纯文件名部分并进行清洗，扩展名由 Format 字段控制
func (m *ManagedTask) buildSuggestedFileName(spec *sdkdto.StoreSpec) string {
	name := spec.SuggestName
	if name != "" {
		// 只保留纯文件名，丢弃任何路径部分
		name = filepath.Base(name)
		name = filename.SanitizeFileName(name)
	}
	if name == "" {
		name = "task"
		if m.task.TaskName.Valid {
			name = m.task.TaskName.String
		}
	}
	return name + normalizeExt(spec.Format)
}

// ParentTask 父任务运行结构体
type ParentTask struct {
	taskId    int64
	taskName  string
	state     atomic.Int32
	refreshMu sync.Mutex // 保护 RefreshState 的读取和写入原子性，防止并发 goroutine 推送过时状态
	children  map[int64]*ManagedTask
	mu        sync.RWMutex
}

// NewParentTask 创建父任务
func NewParentTask(taskId int64, taskName string) *ParentTask {
	return &ParentTask{
		taskId:   taskId,
		taskName: taskName,
		state:    atomic.Int32{},
		children: make(map[int64]*ManagedTask),
	}
}

// AddChild 添加子任务
func (p *ParentTask) AddChild(child *ManagedTask) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.children[child.taskId] = child
}

// GetChildren 获取所有子任务
func (p *ParentTask) GetChildren() []*ManagedTask {
	p.mu.RLock()
	defer p.mu.RUnlock()
	children := make([]*ManagedTask, 0, len(p.children))
	for _, child := range p.children {
		children = append(children, child)
	}
	return children
}

// GetChild 获取指定子任务
func (p *ParentTask) GetChild(taskId int64) (*ManagedTask, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	child, ok := p.children[taskId]
	return child, ok
}

// RefreshState 根据子任务状态刷新父任务状态
// 返回旧状态、新状态、已完成子任务数和子任务总数，供调用方判断是否需要持久化和推送进度
func (p *ParentTask) RefreshState() (oldState, newState TaskState, finishedCount, total int) {
	children := p.GetChildren()
	if len(children) == 0 {
		return p.GetState(), p.GetState(), 0, 0
	}

	var fc, failedCount int
	var anyProcessing, anyWaiting, anyPaused bool
	for _, child := range children {
		switch child.GetState() {
		case TaskStateFinished:
			fc++
		case TaskStateFailed:
			failedCount++
		case TaskStateProcessing, TaskStatePausing, TaskStateStopping, TaskStateWaitingForInput:
			anyProcessing = true
		case TaskStatePaused:
			anyPaused = true
		case TaskStateWaiting:
			anyWaiting = true
		}
	}

	t := len(children)

	switch {
	case anyProcessing:
		newState = TaskStateProcessing
	case anyWaiting:
		newState = TaskStateWaiting
	case anyPaused:
		newState = TaskStatePaused
	case fc == t:
		newState = TaskStateFinished
	case failedCount == t:
		newState = TaskStateFailed
	case fc > 0 && fc < t:
		newState = TaskStatePartlyFinished
	default:
		newState = TaskStateCreated
	}

	oldState = TaskState(p.state.Swap(int32(newState)))
	return oldState, newState, fc, t
}

// GetState 获取父任务状态
func (p *ParentTask) GetState() TaskState {
	return TaskState(p.state.Load())
}

// AllChildrenTerminal 检查所有子任务是否都已进入终态
// 包含 Created 状态：任务被跳过时回退到 Created，此时任务已无需继续执行，应视为终态
func (p *ParentTask) AllChildrenTerminal() bool {
	for _, child := range p.GetChildren() {
		s := child.GetState()
		if s != TaskStateFinished && s != TaskStateFailed && s != TaskStateCreated {
			return false
		}
	}
	return true
}

// buildThumbnailRelPath 构建缩略图相对路径
// 去除 WorkStore FilePath 中的 "store/resource/" 前缀，
// 将缩略图直接放在 "store/thumbnail/作者/" 下，避免 "store/thumbnail/resource/作者/" 的冗余层级。
func buildThumbnailRelPath(resourceRelPath string, thumbFormat string) string {
	// 统一为正斜杠，兼容 Windows 下 filepath.Join 产出的反斜杠路径
	relPath := filepath.ToSlash(resourceRelPath)
	// 去除 "store/resource/" 前缀（含尾斜杠，避免残留前导斜杠）
	relPath = strings.TrimPrefix(relPath, "store/resource/")
	ext := filepath.Ext(relPath)
	base := strings.TrimSuffix(relPath, ext)
	return "store/thumbnail/" + base + "_thumbnail." + thumbFormat
}

// buildThumbnailFileName 构建缩略图文件名
// "video.mp4", "jpg" → "video_thumbnail.jpg"
func buildThumbnailFileName(resourceFileName string, thumbFormat string) string {
	ext := filepath.Ext(resourceFileName)
	base := strings.TrimSuffix(resourceFileName, ext)
	return base + "_thumbnail." + thumbFormat
}

// 任务管理错误定义
var (
	ErrTaskNotProcessing = &TaskManagerError{message: "task is not in processing state"}
	ErrTaskNotPaused     = &TaskManagerError{message: "task is not in paused state"}
	ErrTaskTreeNotFound  = &TaskManagerError{message: "task tree not found"}
)

// TaskManagerError 任务管理器错误
type TaskManagerError struct {
	message string
}

func (e *TaskManagerError) Error() string {
	return e.message
}
