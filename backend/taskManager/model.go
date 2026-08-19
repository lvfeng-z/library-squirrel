package taskManager

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path"
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
	workInfo    bool     // 作品元数据板块
	storeRoles  []string // 资源板块:所选 store_type 子集(main/thumbnail/videoTrack/...);空可能为"默认插件下全量"或"仅作品信息",由 fetchStores 区分
	fetchStores bool     // 本次是否拉取资源:仅作品信息=false(跳过 Start);首跑/重下资源=true。空 universe 的默认插件首跑亦为 true(Start 传空→插件下全量)
}

// runModeFull 首跑执行模式:含作品元数据板块;资源板块留空(由 runModeFromTask 据 universe 派生,空=插件自决全量)
var runModeFull = runMode{workInfo: true}

func (m runMode) hasWorkInfo() bool { return m.workInfo }

// hasStore 是否选择了指定 store_type
func (m runMode) hasStore(storeType string) bool {
	return slices.Contains(m.storeRoles, storeType)
}

// hasAnyStore 是否选择了任意资源板块(决定是否产生任务终态)
func (m runMode) hasAnyStore() bool { return len(m.storeRoles) > 0 }

// runModeFromTask 从 task 持久化字段派生 runMode
// StoreRoles NULL(Start/首次执行,含默认插件)→拉取资源,storeRoles 取 universe(空 universe→Start 传空,插件下全量)
// StoreRoles Valid(Redownload 已记录)→空 selection=仅作品信息(不拉资源),非空=所选子集(拉资源)
// workInfo 统一取 IncludeWorkInfo 字段(首跑由 StartTaskTree 记录为 true)
func runModeFromTask(t *entity.Task) runMode {
	if !t.StoreRoles.Valid {
		return runMode{workInfo: t.IncludeWorkInfo, storeRoles: parseStoreRoles(t.InvolvedRoles), fetchStores: true}
	}
	sel := parseStoreRoles(t.StoreRoles)
	return runMode{workInfo: t.IncludeWorkInfo, storeRoles: sel, fetchStores: len(sel) > 0}
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
	Updates(ctx context.Context, resource *entity.Resource) error
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
	// GetById 根据 ID 获取资源
	GetById(ctx context.Context, id int64) (*entity.Resource, error)
}

// StoreBackupItem 单个 Store 的备份条目（由 backup 包定义具体实现，此处仅引用类型）
type StoreBackupItem = backup.StoreBackupItem

// StoreBackupOrchestrator 资源存储备份编排器接口
// 封装替换场景下作品 Resource 指定 store_type 的备份和还原（板块隔离：按需只备份指定类型）
type StoreBackupOrchestrator interface {
	// BackupStores 备份作品 Resource 指定 store_type 的 Store，返回备份清单
	BackupStores(ctx context.Context, workId int64, storeTypes ...string) []*StoreBackupItem
	// RestoreAllStores 从备份清单还原所有 Store，返回 (restored, skipped)
	RestoreAllStores(ctx context.Context, items []*StoreBackupItem) (restored, skipped int)
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

// StoreDeleter 删除 PersistentStore 记录及磁盘文件（由 persistentStore.Service 实现）
// 失败还原前清理本次新建 store 时使用，backup=false 表示直接删除不产生备份
type StoreDeleter interface {
	// HardDelete 删除记录及对应文件（物理删记录）
	HardDelete(ctx context.Context, id int64, backup bool) (int64, error)
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
	CreateBatch(ctx context.Context, stores []*entity.ResourceStore) error
	DeleteByResourceIdAndTypes(ctx context.Context, resourceId int64, storeTypes []string) error
}

// WorkStoreRoleChecker 已有作品 store 行角色查询接口(覆盖确认行级判定用:任务板块选择与已有行求交)
type WorkStoreRoleChecker interface {
	// ListStoreTypeSetsByWorkIds 批量查询多个作品的 {store_type 集合};无 resource 行的作品不出键,零 store 行的作品出空集合
	ListStoreTypeSetsByWorkIds(ctx context.Context, workIds []int64) (map[int64]map[string]struct{}, error)
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
	WorkStoreRoleChecker    WorkStoreRoleChecker
	Transactor              Transactor
	PendingResourceUpdater  PendingResourceUpdater
	StoreFileCleaner        StoreFileCleaner
	StoreDeleter            StoreDeleter
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
	role        string // store_type(main/thumbnail/videoTrack/...)
	generation  string // downloaded | derived
	format      string // 文件扩展名
	size        int64  // 远程大小;-1 未知
	suggestName string // 插件建议文件名
	continuable bool   // 是否支持续传(derived 恒为 false)

	reader        io.ReadCloser               // 资源数据流(由调用方关闭)
	storeWriter   persistentStore.StoreWriter // 当前写入的 StoreWriter
	storeId       int64                       // PersistentStore 记录 ID
	relPath       string                      // StoreStream 的相对路径(事务回滚/清理用)
	written       int64                       // 已写入字节数(mu 保护)
	initialOffset int64                       // 续传初始偏移(恢复时 = writeOffset;新建轨为 0),进度分母补全完整大小
	state         atomic.Int32                // streamState
	mu            sync.Mutex                  // 保护 written 与 drain 期间的 reader/storeWriter 访问
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

	// actor 通信与生命周期:一条常驻 goroutine 串行处理命令,任务级可变状态只在其内修改
	// (reader 等跨 RPC 对象的访问除外,详见 actorLoop 注释)
	cmdCh        chan taskCmd    // 命令通道(外部→actor,postCmd 非阻塞投递)
	actorDone    chan struct{}   // actor goroutine 退出信号(终态关闭)
	actorStarted atomic.Bool     // 是否已首次 dispatch:dispatch 的 CAS(false→true) 守卫,保证一任务只投一次 cmdStart,重复调用幂等返回 false(取代 dispatchState 三态)
	manager      *Manager        // back-reference:actor 入队/清理需访问 Manager
	semaphore    chan struct{}   // Manager 信号量(取/释槽位)
	slotHeld     bool            // 当前是否持信号量槽位(仅 actor goroutine 访问)
	runCtx       context.Context // 当前 start/resume 命令的子 ctx(中断在途长任务)
	runCancel    context.CancelFunc
	inDownload   atomic.Bool // 是否已进入 downloadLoop:cmdWatcher 据此区分暂停阶段(downloadLoop 走 softPause 排空在途,setup 走立即 runCancel)
	softPause    atomic.Bool // 软暂停标志:downloadLoop 阶段 cmdWatcher 置位,copyLoop 完成当前在途读取并落盘后据此退出,使磁盘 stat 对齐真实中断点
	drainTimer   *time.Timer // drain 超时兜底定时器:在途数据迟迟不落盘(插件卡死/网络黑洞)时强制 runCancel,退化为有损立即暂停
	pendingCmds  []taskCmd   // 长任务执行期间 watcher 累积的命令(主循环稍后处理)

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
	// 用户在 ConfirmReplace 选择跳过：本会话视为终态参与父聚合（AllChildrenTerminal），不持久化、崩溃重启当作未跳过；仅对本次 Start/Retry 执行有效，Resume 不读
	skipped bool
	// 跨重启续传标记（从数据库恢复的暂停任务）
	resumeFromDB bool
	// 已有作品 ID（第一次 run 检测到重复时设置，第二次 run 时用于备份）
	existingWorkId int64

	// 当前错误信息（仅 TaskStateFailed 时有效，通过 onStateChange 回调传递到 Manager）
	errorMessage string

	// 任务信息
	task   *entity.Task
	workId int64
	// 本次执行 saveResource 产出的 Resource ID（替换场景失败后，还原旧 store 完重挂 resource_store 用）
	currentResourceId int64

	// 作品信息响应(Start/Resume 返回;供文件名模板 token data;不再承载资源细节)
	workResp *sdkdto.WorkResponse

	// 多流控制器集合(按本次所选 storeRoles 过滤后的 spec 构建)
	streams []*streamController

	// atomic 进度快照字段，供 BuildSnapshot 并发安全读取
	progressTotal    atomic.Int64 // 资源总大小
	progressFinished atomic.Int64 // 已下载字节数

	// 回调函数
	onStateChange      func(taskId int64, oldState, newState TaskState, errMsg string)
	onProgress         func(taskId int64, total int64, finished int64)
	onResourceIDUpdate func(taskId int64, resourceID sql.NullInt64)
}

// NewManagedTask 创建托管任务并启动 actor goroutine(一生一灭,任务级可变状态只在其内修改)
func NewManagedTask(taskId, parentId int64, task *entity.Task, pluginExec TaskExecutor, deps *TaskDeps, manager *Manager, semaphore chan struct{}) *ManagedTask {
	ctx, cancel := context.WithCancel(context.Background())
	m := &ManagedTask{
		taskId:     taskId,
		parentId:   parentId,
		state:      atomic.Int32{},
		ctx:        ctx,
		cancel:     cancel,
		done:       make(chan struct{}),
		cmdCh:      make(chan taskCmd, 8),
		actorDone:  make(chan struct{}),
		manager:    manager,
		semaphore:  semaphore,
		pluginExec: pluginExec,
		deps:       deps,
		task:       task,
		workId:     taskId,
	}
	// actorStarted 保持零值 false:它是 dispatch 的 CAS(false→true) 首派守卫,首次 dispatch 据此投 cmdStart。
	// 创建期赋 true 会使守卫永远失败、cmdStart 不投递,新任务卡在 Created 永不执行。
	go m.actorLoop()
	return m
}

// ==== per-task actor:命令通道 + 主循环 ====

// cmdKind actor 命令种类
type cmdKind int

const (
	cmdStart          cmdKind = iota // 初始执行 / Retry / Redownload
	cmdResume                        // 从 Paused/Pausing 恢复(含跨重启续传)
	cmdPause                         // 暂停(仅 Processing/Pausing/Paused 有效)
	cmdStop                          // 停止(终态 Failed)
	cmdConfirmReplace                // WaitingForInput 确认后(skip=true 跳过 / skipDup=true 替换)
)

// taskCmd actor 命令
type taskCmd struct {
	kind    cmdKind
	skipDup bool       // cmdConfirmReplace: 替换时跳过查重
	skip    bool       // cmdConfirmReplace: 跳过该任务
	ack     chan error // 可选应答(Pause/Stop 等待 actor 处理;nil=fire-and-forget)
}

// postCmd 非阻塞投递命令到 actor。绝不阻塞投递方(投递路径不持 Manager.mu,防死锁)。
func (m *ManagedTask) postCmd(cmd taskCmd) {
	select {
	case m.cmdCh <- cmd:
	default:
		logger.Log.Warnf("[TaskManager] 任务 %d cmdCh 满,丢弃命令 %d", m.taskId, cmd.kind)
	}
}

// actorLoop actor 主循环:串行处理命令,任务级可变状态只在此 goroutine 内修改。
// 对象创建时启动,终态(或主 ctx 取消)退出;退出前启动 cmdCh drain 防投递方阻塞。
// 作用域:串行覆盖限主程序调度层(命令按序、状态机一致、槽位独占);不覆盖插件 transport 层
// 的 serveSpecsPull goroutine(per-RPC,受 gRPC stream 控制)。reader 等跨 RPC 对象的访问
// 串行性由每次 Start/Resume 新建 reader 结构性保证,不依赖此 actor(见 plugin-dev-guide.md「ctx 与 reader 契约」)。
func (m *ManagedTask) actorLoop() {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("[TaskManager] actor panic taskId=%d: %v", m.taskId, r)
			m.setFailed(fmt.Sprintf("actor panic: %v", r))
		}
		// 启动 drain 防止 actor 退出后投递方阻塞在 cmdCh
		go func() {
			for range m.cmdCh {
			}
		}()
		close(m.actorDone)
	}()
	for {
		select {
		case <-m.ctx.Done():
			return
		case cmd, ok := <-m.cmdCh:
			if !ok {
				return
			}
			switch cmd.kind {
			case cmdStart, cmdResume, cmdConfirmReplace:
				m.handleRunCmd(cmd)
			case cmdPause:
				m.handlePauseCmd(cmd)
			case cmdStop:
				m.handleStopCmd(cmd)
			}
			if isTerminalState(m.GetState()) {
				return
			}
		}
	}
}

// isTerminalState 是否终态(Finished/Failed/PartlyFinished)
func isTerminalState(s TaskState) bool {
	return s == TaskStateFinished || s == TaskStateFailed || s == TaskStatePartlyFinished
}

// handleRunCmd 处理 start/resume/confirmReplace:取槽位 → 派生 runCtx → watcher 监听中断命令 → 执行长任务 → 处理 result + pendingCmds
func (m *ManagedTask) handleRunCmd(cmd taskCmd) {
	// cmdConfirmReplace skip:不取槽位,直接清理 + cancel actor 主 ctx(actorLoop 退出)
	if cmd.kind == cmdConfirmReplace && cmd.skip {
		// 先置 skipped(供随后 cleanupFinishedTask→AllChildrenTerminal 视为终态),再回执行前状态与清理
		m.skipped = true
		m.setState(TaskState(m.task.Status))
		m.manager.cleanupFinishedTask(m)
		m.cancel()
		return
	}

	// cmdConfirmReplace replace:在取槽位前设 skipDuplicateCheck。
	// 若取不到槽位 enqueueSelf,后续被 dispatchFromQueue 以 cmdResume 唤醒时 skipDuplicateCheck 仍生效,
	// run() 跳过查重,避免"批量替换确认后,等待槽位的任务被唤醒时再次弹窗"。
	if cmd.kind == cmdConfirmReplace {
		m.skipDuplicateCheck = cmd.skipDup
	}

	// 取信号量槽位(首次进入);取到则出队,取不到入 waitingQueue 等唤醒
	if !m.slotHeld {
		select {
		case m.semaphore <- struct{}{}:
			m.slotHeld = true
			m.dequeueSelf()
		default:
			m.enqueueSelf()
			return
		}
	}

	// cmdResume 重置执行期可变状态(cmdConfirmReplace 的 skipDuplicateCheck 已在取槽位前设置)
	if cmd.kind == cmdResume {
		m.prepareForResume()
	}

	// 重置优雅暂停状态:cmdStart/cmdConfirmReplace 不经 prepareForResume,在此统一重置覆盖所有 run 命令
	m.softPause.Store(false)
	m.inDownload.Store(false)
	if m.drainTimer != nil {
		m.drainTimer.Stop()
		m.drainTimer = nil
	}
	// 派生 runCtx(每条 run 命令新建,中断在途长任务用)
	m.runCtx, m.runCancel = context.WithCancel(m.ctx)
	stopWatcher := make(chan struct{})
	go m.cmdWatcher(stopWatcher)

	// 执行长任务(阻塞在此,可被 watcher 的 runCancel 中断)
	result := m.runOnce()

	close(stopWatcher)
	if m.drainTimer != nil {
		// drain 正常完成则停定时器;已触发(超时)则 Stop 返回 false,无副作用
		m.drainTimer.Stop()
		m.drainTimer = nil
	}
	if m.runCancel != nil {
		// 此时无在途(drain 已完成或超时已强制取消),取消 stream 作清理
		m.runCancel()
		m.runCancel = nil
	}

	// 处理 result + 释放槽位
	switch result {
	case runResultDone: // 终态 Finished/Failed
		m.slotHeld = false
		<-m.semaphore
		m.manager.dispatchFromQueue()
		m.manager.cleanupFinishedTask(m)
	case runResultNeedConfirm: // 查重命中,等确认
		m.slotHeld = false
		<-m.semaphore
		m.manager.dispatchFromQueue()
		m.manager.enqueueWaitingForInput(m)
	case runResultPaused: // 暂停
		m.slotHeld = false
		<-m.semaphore
		m.manager.dispatchFromQueue()
	}

	// 处理 watcher 累积的命令(按序)
	pending := m.pendingCmds
	m.pendingCmds = nil
	for _, c := range pending {
		switch c.kind {
		case cmdStart, cmdResume, cmdConfirmReplace:
			m.handleRunCmd(c)
		case cmdPause:
			m.handlePauseCmd(c)
		case cmdStop:
			m.handleStopCmd(c)
		}
		if isTerminalState(m.GetState()) {
			return
		}
	}
}

// runOnce 执行单次任务主体(run 或跨重启续传)
func (m *ManagedTask) runOnce() runResult {
	if m.resumeFromDB {
		logger.Log.Infof("[TaskManager] executeTask: taskId=%d, resumeFromDB=%v", m.taskId, m.resumeFromDB)
		return m.resumeFromPersistedState()
	}
	return m.run()
}

// drainTimeout 优雅暂停的 drain 超时阈值:暂停后等待在途数据落盘的最长时间,超时则强制取消 runCtx,退化为有损立即暂停。
// 单个 chunk 往返(主程序发起读取 → 插件返回数据 → 主程序落盘)通常 <1s,2s 足够覆盖常态往返,且暂停延迟用户无感。
const drainTimeout = 2 * time.Second

// cmdWatcher 长任务执行期间监听 cmdCh:暂停走优雅暂停(置 softPause + 启动 drainTimer,不立即取消,在途数据照常落盘);
// 停止仍立即 runCancel(放弃语义,无需保留在途);所有命令暂存 pendingCmds 由主循环稍后处理
func (m *ManagedTask) cmdWatcher(stop <-chan struct{}) {
	for {
		select {
		case c := <-m.cmdCh:
			if c.kind == cmdPause {
				if m.inDownload.Load() {
					// downloadLoop 阶段:优雅暂停——不取消 runCtx,通知 copyLoop 完成当前在途往返(读取→落盘)后退出
					m.softPause.Store(true)
					// drain 超时兜底:在途迟迟不落盘则强制取消,退化为有损立即暂停
					m.drainTimer = time.AfterFunc(drainTimeout, func() {
						if m.softPause.Load() && m.runCancel != nil {
							logger.Log.Warnf("[TaskManager] 任务 %d drain 超时,强制取消(退化为有损立即暂停)", m.taskId)
							m.runCancel()
						}
					})
				} else {
					// setup 阶段:无在途 chunk 需排空,立即取消 runCtx 中断插件 RPC(快速暂停)
					if m.runCancel != nil {
						m.runCancel()
					}
				}
				m.pendingCmds = append(m.pendingCmds, c)
				return
			}
			if c.kind == cmdStop {
				// 停止是放弃,立即取消(不走 drain)
				if m.runCancel != nil {
					m.runCancel()
				}
				m.pendingCmds = append(m.pendingCmds, c)
				return
			}
			m.pendingCmds = append(m.pendingCmds, c)
		case <-stop:
			return
		}
	}
}

// enqueueSelf 取不到信号量槽位时入 Manager.waitingQueue(按 taskId 去重)并置 Waiting
func (m *ManagedTask) enqueueSelf() {
	m.manager.mu.Lock()
	for _, t := range m.manager.waitingQueue {
		if t.taskId == m.taskId {
			m.manager.mu.Unlock()
			return // 已在队
		}
	}
	m.manager.waitingQueue = append(m.manager.waitingQueue, m)
	m.manager.mu.Unlock()
	m.setState(TaskStateWaiting)
}

// dequeueSelf 取到信号量槽位后从 Manager.waitingQueue 移除自己(若在内)
func (m *ManagedTask) dequeueSelf() {
	m.manager.mu.Lock()
	kept := make([]*ManagedTask, 0, len(m.manager.waitingQueue))
	for _, t := range m.manager.waitingQueue {
		if t.taskId != m.taskId {
			kept = append(kept, t)
		}
	}
	m.manager.waitingQueue = kept
	m.manager.mu.Unlock()
}

// handlePauseCmd 处理 pause 命令:非终态任务 → Paused(命令队列保证 PauseTaskTree 的 pause 覆盖陈旧 resume)。
// Processing 状态额外中断在途长任务(runCancel,watcher 在长任务期间已触发时此处幂等);其余状态仅通知插件 + 落 Paused。
func (m *ManagedTask) handlePauseCmd(cmd taskCmd) {
	s := m.GetState()
	if isTerminalState(s) {
		if cmd.ack != nil {
			cmd.ack <- ErrTaskNotProcessing
		}
		return
	}
	if s == TaskStateProcessing {
		m.setState(TaskStatePausing)
		if m.runCancel != nil {
			m.runCancel()
		}
	}
	// 通知插件暂停(有上游时关上游 HTTP 保留 validBytes 供 Resume Range 续传;无上游时插件幂等处理)
	param := &sdkdto.TaskResParam{
		Task:       dto.NewTaskDTO(m.task),
		ResourceId: m.task.PendingResourceID.Int64,
	}
	if err := m.pluginExec.Pause(m.ctx, param); err != nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 插件 Pause 失败: %v", m.taskId, err)
	}
	m.setState(TaskStatePaused)
	if cmd.ack != nil {
		cmd.ack <- nil
	}
}

// handleStopCmd 处理 stop 命令:终态 Failed
func (m *ManagedTask) handleStopCmd(cmd taskCmd) {
	if isTerminalState(m.GetState()) {
		if cmd.ack != nil {
			cmd.ack <- nil
		}
		return
	}
	m.setState(TaskStateStopping)
	if m.runCancel != nil {
		m.runCancel()
	}
	for _, s := range m.streams {
		s.abort()
	}
	param := &sdkdto.TaskResParam{
		Task:       dto.NewTaskDTO(m.task),
		ResourceId: m.task.PendingResourceID.Int64,
	}
	if err := m.pluginExec.Stop(m.ctx, param); err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d Stop 失败: %v", m.taskId, err)
	}
	m.setFailed("任务被用户停止")
	if cmd.ack != nil {
		cmd.ack <- nil
	}
}

// Pause 暂停任务(投递 cmdPause + 等 actor 处理;仅 Processing 时返回 nil,否则 ErrTaskNotProcessing)
func (m *ManagedTask) Pause() error {
	ack := make(chan error, 1)
	m.postCmd(taskCmd{kind: cmdPause, ack: ack})
	return <-ack
}

// Stop 停止任务(投递 cmdStop + 等 actor 处理)
func (m *ManagedTask) Stop() {
	ack := make(chan error, 1)
	m.postCmd(taskCmd{kind: cmdStop, ack: ack})
	<-ack
}

// run 核心执行逻辑入口，按 runMode 分流到对应板块
func (m *ManagedTask) run() runResult {
	// 最先注册，最后执行：任务失败时还原已备份的 Store
	// 使用 context.Background() 而非 m.ctx，确保任务被停止（context 已取消）后还原操作仍能正常执行
	defer func() {
		if m.GetState() == TaskStateFailed && len(m.storeBackupItems) > 0 {
			logger.Log.Infof("[TaskManager] 任务 %d 失败，开始还原 %d 个已备份 Store", m.taskId, len(m.storeBackupItems))
			// 先清理本次 startDownload 新建的 store：回滚本次下载产物到备份点，
			// 释放其占用的 file_path，否则还原旧 store 时 INSERT 会触发 UNIQUE 冲突
			m.cleanupCreatedStores(context.Background())
			_, skipped := m.deps.StoreBackupOrchestrator.RestoreAllStores(context.Background(), m.storeBackupItems)
			// 还原后重挂 resource_store 到还原的 store（RestoreAllStores 仅还原文件、不感知 resource_store）
			m.remountRestoredStores(context.Background())
			if skipped > 0 {
				logger.Log.Warnf("[TaskManager] 任务 %d 有 %d 个旧资源因备份缺失无法还原", m.taskId, skipped)
			}
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
	if m.runCtx.Err() != nil {
		// runCtx 取消(Pause/Stop 经 watcher runCancel):视为中断,不进执行。
		// 不依赖 state——watcher 是独立 goroutine,只 runCancel + 暂存命令,setState 由后续 handlePauseCmd/handleStopCmd 处理。
		return runResultPaused
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
	if m.runMode.fetchStores && m.deps.WorkDirProvider.GetWorkDir() == "" {
		logger.Log.Errorf("[TaskManager] 任务 %d 失败: 未配置资源库目录", m.taskId)
		return m.comboFail("未配置资源库目录，请先在设置中指定资源库保存位置")
	}

	// 含资源板块：查重（fallback；主路径在 Manager.batchCheckDuplicates）
	if m.runMode.fetchStores && !m.skipDuplicateCheck && m.deps.WorkChecker != nil &&
		m.task.SiteID.Valid && m.task.SiteWorkID.Valid && m.task.SiteWorkID.String != "" {
		existing, err := m.deps.WorkChecker.GetBySiteAndSiteWorkID(m.runCtx, m.task.SiteID.Int64, m.task.SiteWorkID.String)
		if err == nil && existing != nil {
			// 行级覆盖判定:所选板块与已有作品 store 行求交,空交集不弹窗(保留 existingWorkId 供替换定位)
			// 板块为空(插件自决全量)时已有任意行即冲突,冲突载荷取已有行角色;行级信息不可得时保守弹窗
			var conflictRoles []string
			hitStoreRows := true
			if m.deps.WorkStoreRoleChecker != nil {
				sets, serr := m.deps.WorkStoreRoleChecker.ListStoreTypeSetsByWorkIds(m.runCtx, []int64{existing.GetID()})
				if serr != nil {
					logger.Log.Errorf("[TaskManager] 任务 %d 行级覆盖判定查询失败: %v，退回弹窗", m.taskId, serr)
				} else if existingTypes := sets[existing.GetID()]; len(existingTypes) == 0 {
					hitStoreRows = false
				} else if len(m.runMode.storeRoles) == 0 {
					conflictRoles = sortedStoreRoles(existingTypes)
				} else {
					conflictRoles = intersectRoles(m.runMode.storeRoles, existingTypes)
					if len(conflictRoles) == 0 {
						hitStoreRows = false
					}
				}
			}
			if hitStoreRows {
				m.existingWorkId = existing.GetID()
				existingWorkName := ""
				if existing.SiteWorkName.Valid {
					existingWorkName = existing.SiteWorkName.String
				}
				if m.deps.Pusher != nil {
					m.deps.Pusher.PushDuplicateDetected(m.taskId, m.task.TaskName.String, existing.GetID(), existingWorkName, conflictRoles)
				}
				m.setState(TaskStateWaitingForInput)
				return runResultNeedConfirm
			}
			// 零交集/零行:无覆盖对象,不弹窗,保留 existingWorkId 供替换定位
			m.existingWorkId = existing.GetID()
		}
	}

	// workId 定位 + 替换判定
	// 含资源板块在已有作品上重执行都视为替换,需备份旧 store
	if m.runMode.fetchStores {
		// 查重命中(existingWorkId>0，确认后重入或 batchCheckDuplicates 设置)
		if m.existingWorkId > 0 {
			m.workId = m.existingWorkId
			m.existingWorkId = 0
			m.isReplace = true
		} else if !m.runMode.hasWorkInfo() {
			// 含资源板块的重执行必须定位到已有作品(否则无处挂载资源)
			logger.Log.Errorf("[TaskManager] 任务 %d 资源重执行未定位到作品", m.taskId)
			return m.comboFail("未找到任务对应的作品，无法重新下载资源")
		}
		// 含 workInfo 且查重未命中：workInfo 板块的 SaveWorkInfo 会提供 workId(新作品,非替换)
	}

	// 替换场景:备份所选板块对应的旧 store。
	// 任一被重执行的板块,只要该类型 store 在已有作品上存在就备份(备份→生成→失败还原,统一替换语义)。
	if m.isReplace && m.runMode.hasAnyStore() {
		m.storeBackupItems = m.deps.StoreBackupOrchestrator.BackupStores(m.runCtx, m.workId, m.runMode.storeRoles...)
	}

	// 板块 A：作品信息（CreateWorkInfo + SaveWorkInfo，提供 workId 与文件名模板数据）
	var workResp *sdkdto.WorkResponse
	if m.runMode.hasWorkInfo() {
		var err error
		workResp, err = m.pluginExec.CreateWorkInfo(m.runCtx, m.task)
		if err != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d CreateWorkInfo 失败: %v", m.taskId, err)
			return m.comboFail(fmt.Sprintf("创建作品信息失败: %v", err))
		}
		savedWorkId, err := m.deps.WorkInfoSaver.SaveWorkInfo(m.runCtx, m.task, workResp)
		if err != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d 保存作品信息失败: %v", m.taskId, err)
			return m.comboFail(fmt.Sprintf("保存作品信息失败: %v", err))
		}
		m.workId = savedWorkId
	}

	// 资源板块:fetchStores 时 Start 按所选 storeRoles 选择性产出(空 storeRoles=默认插件下全量)
	if m.runMode.fetchStores {
		specs, startResp, err := m.pluginExec.Start(m.runCtx, m.task, m.runMode.storeRoles)
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
	if m.runMode.fetchStores {
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

	// 解析 bas 基准名与目录(所有 store 文件名共用;bas 由模板+作品元数据生成,不依赖具体 spec)
	baseRelPath, bas := m.resolveBaseName(workResp)
	roleCounters := make(map[string]int, len(specs))
	// 多 store 判定(资源级):资源 store 总数>1 则全部带 role+seq;单 store 用 <bas>.<ext>
	multiStore := len(specs) > 1

	// 事务:为每个 spec 建 StoreStream + 挂 resource_store + Resource Save + PendingResourceID 更新
	streams := make([]*streamController, 0, len(specs))
	txErr := m.deps.Transactor.ExecInTransaction(context.Background(), func(txCtx context.Context) error {
		mounts := make([]pendingMount, 0, len(specs))
		for _, spec := range specs {
			sameRoleSeq := roleCounters[spec.Role]
			roleCounters[spec.Role]++
			relPath, fileName := m.resolveStorePath(spec, baseRelPath, bas, sameRoleSeq, multiStore)
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
		m.currentResourceId = resourceId

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

	// 时序不变量:downloadLoop 须在上方 startDownload 事务提交后执行。插件 pull chunk 时可能经
	// GetStoreRelPath 查询 resource_store 路径(如 document 引用兄弟 image 文件名),该查询走独立
	// DB 连接,仅事务提交后 resource_store 行与任务 PendingResourceID 才对其可见。事务回滚/暂停路径上方已提前 return。
	m.streams = streams
	return m.downloadLoop()
}

// isNonTerminalMode 是否为非终态板块组合（无资源板块）：执行不产生任务终态、不持久化任务状态
func (m *ManagedTask) isNonTerminalMode() bool {
	return !m.runMode.fetchStores
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

// saveResource 保存 Resource(事务内调用)并挂 resource_store 行。
// saveResource 保存 Resource(事务内调用)并挂 resource_store 行。
// 始终按 workId 查找已有 Resource:找到则更新(避免频繁启停 setup 阶段暂停后恢复导致重复创建),
// 未找到则创建新 Resource。isReplace 标志仅用于 runSectionCombo 的备份决策,不影响此处。
// store 关联只写 resource_store 行(阶段6 已完成消费方迁移,旧固定列已废弃)。
func (m *ManagedTask) saveResource(ctx context.Context, workId int64, mounts []pendingMount) (int64, error) {
	var resourceId int64

	// 始终查找已有 Resource(不依赖 isReplace),防止重复创建
	existing := m.findReplaceResource(ctx, workId)
	if existing != nil {
		existing.ResourceComplete = sql.NullInt64{Int64: 0, Valid: true}
		if err := m.deps.ResourceUpdater.Updates(ctx, existing); err != nil {
			return 0, fmt.Errorf("更新 Resource 失败: %w", err)
		}
		resourceId = existing.GetID()
	}

	if resourceId == 0 {
		// 无已有 Resource:创建新 Resource
		resource := entity.NewResource()
		resource.WorkID = workId
		resource.TaskID = m.task.GetID()
		resource.ResourceComplete = sql.NullInt64{Int64: 0, Valid: true} // 下载未完成
		// 创建期声明的资源类型;严格识别——空值或非预定义值在写入前抛错,不兜底
		resourceType := m.task.ResourceType.String
		if err := entity.ValidateResourceType(resourceType); err != nil {
			return 0, fmt.Errorf("资源类型声明无效: %w", err)
		}
		resource.ResourceType = resourceType

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

// markResourceComplete 计算资源完整度并持久化(下载完成时调用)。
// 三态(决策4):0=未校验(未知/未声明 resource_type 或 reader 缺失,不校验)、1=完整(结构校验通过)、
// 2=不完整(缺角色或超量,前端徽标提示但不阻断打开)。
// 读路径不抛错;查询异常降级为保持未校验,不阻断任务完成。
func (m *ManagedTask) markResourceComplete(ctx context.Context, resourceId int64) {
	if resourceId == 0 {
		return
	}
	resource, err := m.deps.ResourceReader.GetById(ctx, resourceId)
	if err != nil || resource == nil {
		logger.Log.Warnf("[TaskManager] 计算资源完整度失败: resourceId=%d err=%v", resourceId, err)
		return
	}
	complete := 0 // 默认未校验
	if entity.LookupResourceTypeSpec(resource.ResourceType) != nil && m.deps.ResourceStoreReader != nil {
		storeRows, sErr := m.deps.ResourceStoreReader.ListByResourceId(ctx, resourceId)
		if sErr != nil {
			logger.Log.Warnf("[TaskManager] 查询 resource_store 计数失败: resourceId=%d err=%v", resourceId, sErr)
		} else {
			counts := make(map[string]int, len(storeRows))
			for _, s := range storeRows {
				counts[s.StoreType]++
			}
			c, missing, excess := entity.ComputeResourceComplete(resource.ResourceType, counts)
			complete = c
			if c == 2 {
				logger.Log.Infof("[TaskManager] 资源结构不完整: resourceId=%d type=%s missing=%v excess=%v", resourceId, resource.ResourceType, missing, excess)
			}
		}
	}
	if resource.ResourceComplete.Valid && resource.ResourceComplete.Int64 == int64(complete) {
		return // 值未变,跳过写库
	}
	resource.ResourceComplete = sql.NullInt64{Int64: int64(complete), Valid: true}
	if err := m.deps.ResourceUpdater.Updates(ctx, resource); err != nil {
		logger.Log.Warnf("[TaskManager] 更新 ResourceComplete 失败: resourceId=%d err=%v", resourceId, err)
	}
}

// findReplaceResource 替换场景定位已有 Resource(优先备份清单第一个条目,回退 workId 查询)
func (m *ManagedTask) findReplaceResource(ctx context.Context, workId int64) *entity.Resource {
	if len(m.storeBackupItems) > 0 {
		item := m.storeBackupItems[0]
		existing, err := m.deps.ResourceReader.GetById(ctx, item.ResourceID)
		if err != nil || existing == nil {
			logger.Log.Warnf("[TaskManager] 查询已有 Resource(id=%d) 失败: %v", item.ResourceID, err)
		} else {
			return existing
		}
	}
	resources, queryErr := m.deps.ResourceReader.ListByWorkId(ctx, workId)
	if queryErr != nil {
		logger.Log.Warnf("[TaskManager] 查询作品 %d 资源失败: %v", workId, queryErr)
		return nil
	}
	if len(resources) > 0 {
		return resources[0]
	}
	return nil
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
	roleSeq := make(map[string]int, len(mounts)) // 同 role 内序号:store 稳定身份(与 resume 身份匹配、文件名消歧统一)
	for _, mt := range mounts {
		// 严格识别 store_type:非六预定义角色抛错,不兜底
		if err := entity.ValidateStoreType(mt.role); err != nil {
			return fmt.Errorf("store_type 非法(%s): %w", mt.role, err)
		}
		s := entity.NewResourceStore()
		s.ResourceID = resourceId
		s.StoreType = mt.role
		s.Generation = mt.generation
		s.StoreID = mt.storeId
		s.StoreSeq = roleSeq[mt.role]
		roleSeq[mt.role]++
		stores = append(stores, s)
	}
	return m.deps.ResourceStoreWriter.CreateBatch(ctx, stores)
}

// cleanupCreatedStores 清理本次 startDownload 新建的 PersistentStore（记录 + 磁盘文件）
// 替换场景任务失败、还原旧 store 前调用：释放本次新建 store 占用的 file_path，
// 否则 RestoreAllStores 还原旧 store 时 INSERT 会触发 UNIQUE 冲突。
// 仅事务提交成功后 m.streams 有值时生效；事务回滚时 m.streams 已为 nil。
func (m *ManagedTask) cleanupCreatedStores(ctx context.Context) {
	if len(m.streams) == 0 {
		return
	}
	// 先关闭所有 storeWriter 释放文件句柄（Windows 下文件锁会阻碍删除）
	for _, s := range m.streams {
		if s.storeWriter != nil {
			s.storeWriter.Close()
		}
	}
	for _, s := range m.streams {
		if _, err := m.deps.StoreDeleter.HardDelete(ctx, s.storeId, false); err != nil {
			logger.Log.Warnf("[TaskManager] 清理本次新建 store(id=%d, type=%s) 失败: %v", s.storeId, s.role, err)
		}
	}
	m.streams = nil
}

// remountRestoredStores 还原旧 store 后，将 resource_store 重挂到还原的 store
// RestoreAllStores 已在 storeBackupItems 各项回填 NewStoreID；据此重建 resource_store 关联，
// 否则 resource_store 仍指向已清理的本次新建 store（断裂孤儿）。
// 复用 mountResourceStores 语义（先删同 role 旧关联再插），resourceId 取本次 saveResource 的产物。
func (m *ManagedTask) remountRestoredStores(ctx context.Context) {
	if m.currentResourceId == 0 || len(m.storeBackupItems) == 0 {
		return
	}
	mounts := make([]pendingMount, 0, len(m.storeBackupItems))
	for _, item := range m.storeBackupItems {
		if item.NewStoreID > 0 {
			mounts = append(mounts, pendingMount{role: item.StoreType, generation: item.Generation, storeId: item.NewStoreID})
		}
	}
	if len(mounts) == 0 {
		return
	}
	if err := m.mountResourceStores(ctx, m.currentResourceId, mounts); err != nil {
		logger.Log.Warnf("[TaskManager] 还原后重挂 resource_store 失败: %v", err)
	}
}

// downloadLoop 多流并发下载循环:每条 spec 一个 goroutine 跑 read→write→累计
// 全部 completed → Finished;任一 failed → Failed(保留已完成轨的 store);暂停 → Paused;取消 → Stop 已处理
func (m *ManagedTask) downloadLoop() runResult {
	// 标记进入下载阶段:cmdWatcher 据此让 cmdPause 走 softPause 排空在途(setup 阶段则立即 runCancel)
	m.inDownload.Store(true)
	defer m.inDownload.Store(false)
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
	// 全部完成:先计算并持久化资源完整度(此刻所有 store 已 Complete),再清 pending + 转 Finished
	m.markResourceComplete(m.runCtx, m.currentResourceId)
	m.clearPendingResourceID()
	m.setState(TaskStateFinished)
	return runResultDone
}

// copyLoop 单流读取循环:read→write→累计,响应暂停/取消/EOF
func (s *streamController) copyLoop(m *ManagedTask) streamResult {
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-m.runCtx.Done():
			// runCtx 取消(Pause/Stop 经 watcher runCancel 中断在途 reader):统一保留文件。
			// 此刻 state 尚未由 handlePauseCmd/handleStopCmd 设置(它们在长任务返回后处理);
			// Stop 的文件删除由后续 handleStopCmd 的 streams abort 处理。
			return s.handlePause(buf)
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
		// 优雅暂停:本轮 Read 的数据已落盘,退出收尾。传 nil 跳过 handlePause 的 drain——
		// pull 模型下 drain 会再 Read(即发起新 PullRequest)拉取新数据,违背"暂停只阻止新数据发起"。
		// runCtx.Err()==nil 守卫区分正常退出与超时兜底:兜底已 runCancel 时由下方 readErr 分支走有损路径
		if m.softPause.Load() && m.runCtx.Err() == nil {
			return s.handlePause(nil)
		}
		if readErr != nil {
			if readErr == io.EOF {
				return s.handleEOF(m)
			}
			// runCtx 取消导致的读取错误(gRPC stream cancel):视为中断,保留文件,不 Failed
			if m.runCtx.Err() != nil {
				return s.handlePause(buf)
			}
			// 非 EOF 非 runCtx 取消:真正的读取失败
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
	// runCtx 取消(Pause/Stop 经 watcher)导致上游关闭产生 EOF:视为中断,保留文件
	if m.runCtx.Err() != nil {
		return s.handlePause(nil)
	}
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
	// 完整性校验:downloaded 轨
	s.mu.Lock()
	written := s.written
	s.mu.Unlock()
	if s.generation == entity.GenerationDownloaded {
		switch {
		case s.size > 0 && written < s.size:
			// 已知预期大小且未下完
			logger.Log.Errorf("[TaskManager] 任务 %d 下载不完整(role=%s): 已下载 %d / 预期 %d", m.taskId, s.role, written, s.size)
			s.abort()
			s.state.Store(int32(streamFailed))
			return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("%s 下载不完整: 已下载 %d / 预期 %d", s.role, written, s.size)}
		case written == 0:
			// 预期大小未知(spec.Size<=0)但一字节未写:空产物,判定不完整
			logger.Log.Errorf("[TaskManager] 任务 %d 下载为空(role=%s): written=0", m.taskId, s.role)
			s.abort()
			s.state.Store(int32(streamFailed))
			return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("%s 下载为空(written=0)", s.role)}
		}
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
	if m.runCtx.Err() != nil {
		// runCtx 取消(Pause/Stop 经 watcher runCancel):视为中断,不进执行。
		// 不依赖 state——watcher 是独立 goroutine,只 runCancel + 暂存命令,setState 由后续 handlePauseCmd/handleStopCmd 处理。
		return runResultPaused
	}
	m.setState(TaskStateProcessing)

	// 1. 通过 pending_resource_id 加载 Resource 实体
	if !m.task.PendingResourceID.Valid {
		logger.Log.Warnf("[TaskManager] 任务 %d 无有效的 pending_resource_id，降级为完整重新执行", m.taskId)
		return m.run()
	}
	resource, err := m.deps.ResourceReader.GetById(m.runCtx, m.task.PendingResourceID.Int64)
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
	storeRows, err := m.deps.ResourceStoreReader.ListByResourceId(m.runCtx, resource.GetID())
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
	streamOffsets := make([]*sdkdto.StoreResumeOffset, 0, len(storeRows))
	completedSet := make(map[storeIdentity]struct{}, len(storeRows))
	var incompleteDerivedRoles []string
	for _, row := range storeRows {
		ident := storeIdentity{role: row.StoreType, seq: row.StoreSeq}
		store, storeErr := m.deps.StoreReader.GetById(m.runCtx, row.StoreID)
		if storeErr != nil || store == nil {
			// store 记录丢失:downloaded 整轨重下(offset=0),derived 整轨重产
			if row.Generation == entity.GenerationDerived {
				incompleteDerivedRoles = append(incompleteDerivedRoles, row.StoreType)
			} else {
				streamOffsets = append(streamOffsets, &sdkdto.StoreResumeOffset{Role: row.StoreType, StoreSeq: int32(row.StoreSeq), Offset: 0})
			}
			continue
		}
		absPath := m.deps.StoreReader.GetAbsPath(store)
		info, statErr := os.Stat(absPath)
		if store.CompletedAt > 0 && statErr == nil {
			// 该 store 已完成:按身份记录,不进入 Resume/重产(同 role 多 store 各自独立判定)
			completedSet[ident] = struct{}{}
			continue
		}
		// 未完成:downloaded 按偏移续传,derived 整轨重产
		if row.Generation == entity.GenerationDerived {
			incompleteDerivedRoles = append(incompleteDerivedRoles, row.StoreType)
		} else {
			var offset int64
			if statErr == nil {
				offset = info.Size()
			}
			streamOffsets = append(streamOffsets, &sdkdto.StoreResumeOffset{Role: row.StoreType, StoreSeq: int32(row.StoreSeq), Offset: offset})
		}
	}

	logger.Log.Infof("[TaskManager] 任务 %d 跨重启续传: resourceID=%d, offsets=%v, completed=%v, regenDerived=%v", m.taskId, resource.GetID(), streamOffsets, completedSet, incompleteDerivedRoles)

	// 4. 调用插件 Resume(按 StreamOffsets 续传未完成 downloaded store,身份化 role+store_seq)
	param := &sdkdto.TaskResumeParam{
		Task:          dto.NewTaskDTO(m.task),
		StreamOffsets: streamOffsets,
	}
	specs, newResp, err := m.pluginExec.Resume(m.runCtx, param)
	if err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d 跨重启 Resume 失败: %v", m.taskId, err)
		// Pause 在 Resume 进行中取消 ctx(stream ctx 继承任务 ctx):视为暂停,不置失败
		if m.abortedByPause() {
			return runResultPaused
		}
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
		derivedSpecs, _, startErr := m.pluginExec.Start(m.runCtx, m.task, incompleteDerivedRoles)
		if startErr != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d 重产 derived 轨 %v 失败: %v", m.taskId, incompleteDerivedRoles, startErr)
			// Pause 在 derived 重产进行中取消 ctx:视为暂停,不置失败
			if m.abortedByPause() {
				return runResultPaused
			}
			m.setFailed(fmt.Sprintf("重产资源失败: %v", startErr))
			return runResultDone
		}
		specs = append(specs, derivedSpecs...)
	}

	if len(specs) == 0 {
		// 无未完成轨道需续传/重产:任务直接完成
		logger.Log.Infof("[TaskManager][dispatch] taskId=%d resumeFromPersistedState 无未完成轨道,直接 Finished(streamOffsets=%v regenDerived=%v)", m.taskId, streamOffsets, incompleteDerivedRoles)
		m.markResourceComplete(m.runCtx, resource.GetID())
		m.clearPendingResourceID()
		m.setState(TaskStateFinished)
		return runResultDone
	}

	// 5. 为每个返回的 spec 续接(continuable downloaded)或重建 store,构建 streamController
	// 解析 bas 基准名与目录(与 startDownload 一致)
	baseRelPath, bas := m.resolveBaseName(newResp)
	// 多 store 判定基于资源全局 store 总数:resume 的 specs 是未完成子集(已完成 store 不在其中),
	// 不能用 len(specs)——否则部分完成时判定翻转→文件名漂移→续传/重建到错误路径
	multiStore := len(storeRows) > 1
	// 解析每个 spec 的全局 store_seq(specs 是未完成子集,同 role 部分完成时 specs 内重计会与全局 store_seq
	// 错位 → findStoreRowByIdentity 匹配已完成行 → 续传覆盖;须按 streamOffsets/storeRows 取全局 seq)
	specSeq := resumeSpecSeq(specs, streamOffsets, storeRows, completedSet)

	streams := make([]*streamController, 0, len(specs))
	txErr := m.deps.Transactor.ExecInTransaction(context.Background(), func(txCtx context.Context) error {
		// 未完成 store 的续传/重建:按 spec 处理,记录 (role,seq)→storeId 供全量重挂组装
		storeIdByIdentity := make(map[storeIdentity]int64, len(specs))
		for _, spec := range specs {
			sameRoleSeq := specSeq[spec]
			relPath, fileName := m.resolveStorePath(spec, baseRelPath, bas, sameRoleSeq, multiStore)
			// 身份匹配:同 role 内按 store_seq(sameRoleSeq)精确定位已有行(替代 role 首匹配,支持 N-同 role)
			existingRow := findStoreRowByIdentity(storeRows, spec.Role, sameRoleSeq)
			offset, hasOffset := findResumeOffset(streamOffsets, spec.Role, sameRoleSeq)
			// continuable 的 downloaded store 且有正偏移:用已有 storeId + ResumeStream 续传
			// 写入偏移:插件指定(spec.ResumeWriteOffset)优先,否则用主程序 stat 的 offset
			if spec.Generation == entity.GenerationDownloaded && existingRow != nil && hasOffset && offset > 0 {
				writeOffset := offset
				if spec.ResumeWriteOffset != nil {
					writeOffset = *spec.ResumeWriteOffset
				}
				writer, resumeErr := m.deps.StoreStreamer.ResumeStream(txCtx, existingRow.StoreID, writeOffset)
				if resumeErr != nil {
					return resumeErr
				}
				logger.Log.Infof("[ResumeMount] taskId=%d role=%s seq=%d mode=ResumeStream storeId=%d writeOffset=%d streamOffset=%d",
					m.taskId, spec.Role, sameRoleSeq, existingRow.StoreID, writeOffset, offset)
				sc := newStreamController(spec, existingRow.StoreID, writer, relPath)
				sc.written = writeOffset
				sc.initialOffset = writeOffset
				streams = append(streams, sc)
				storeIdByIdentity[storeIdentity{spec.Role, sameRoleSeq}] = existingRow.StoreID
			} else {
				// derived 或 offset=0 的 downloaded:StoreStream 重建
				storeId, writer, storeErr := m.deps.StoreStreamer.StoreStream(txCtx, relPath, fileName)
				if storeErr != nil {
					return storeErr
				}
				logger.Log.Infof("[ResumeMount] taskId=%d role=%s seq=%d mode=StoreStream storeId=%d writeOffset=0 streamOffset=%d",
					m.taskId, spec.Role, sameRoleSeq, storeId, offset)
				streams = append(streams, newStreamController(spec, storeId, writer, relPath))
				storeIdByIdentity[storeIdentity{spec.Role, sameRoleSeq}] = storeId
			}
		}
		// 全量重挂:按 storeRows 顺序(保持 store_seq 稳定)组装已完成 + 本次续传/重建的 store。
		// 已完成用原 storeId(不重下),未完成用 storeIdByIdentity;避免 mountResourceStores 批删丢已完成同 role 关联。
		mounts := make([]pendingMount, 0, len(storeRows))
		for _, row := range storeRows {
			storeId := row.StoreID
			if newId, ok := storeIdByIdentity[storeIdentity{row.StoreType, row.StoreSeq}]; ok {
				storeId = newId
			}
			mounts = append(mounts, pendingMount{role: row.StoreType, generation: row.Generation, storeId: storeId})
		}
		if err := m.mountResourceStores(txCtx, resource.GetID(), mounts); err != nil {
			return err
		}
		return nil
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

	// 时序不变量:downloadLoop 须在上方 resume 落盘事务提交后执行。插件 pull chunk 时可能经
	// GetStoreRelPath 查询 resource_store 路径(如 document 引用兄弟 image 文件名),该查询走独立
	// DB 连接,仅事务提交后 resource_store 行与任务 PendingResourceID 才对其可见。事务回滚/暂停路径上方已提前 return。
	m.streams = streams
	return m.downloadLoop()
}

// prepareForResume 重置任务的运行时可变状态(关闭旧 reader、streams=nil、按 PendingResourceID 设 resumeFromDB)。
// actor 模型下 m.ctx 是 actor 主 ctx(一生一灭,不重建),runCtx 由 handleRunCmd 派生;此处只重置执行期可变字段。
// 仅 actor goroutine 在处理 cmdResume 时调用,无并发访问。
func (m *ManagedTask) prepareForResume() {
	// 清除上一轮暂停标志(防御性:handleRunCmd 派生 runCtx 前已统一重置,此处显式表达 resume 重置语义)
	m.softPause.Store(false)
	if m.drainTimer != nil {
		m.drainTimer.Stop()
		m.drainTimer = nil
	}
	m.closeStreamReaders()
	m.streams = nil
	// 有 PendingResourceID 走 resumeFromPersistedState（内部按各轨 store 状态续传/重产）
	// 无 PendingResourceID 走 run()（从头执行）
	m.resumeFromDB = m.task.PendingResourceID.Valid
	logger.Log.Infof("[TaskManager] prepareForResume: taskId=%d, PendingResourceID={Valid:%v, Int64:%d}, resumeFromDB=%v",
		m.taskId, m.task.PendingResourceID.Valid, m.task.PendingResourceID.Int64, m.resumeFromDB)
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
	// runCtx 取消即视为中断(Pause/Stop 经 watcher runCancel);不依赖 state——
	// watcher 收到 pause/stop 时只 runCancel + 暂存命令,setState 由后续 handlePauseCmd/handleStopCmd 处理。
	return m.runCtx.Err() != nil
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
		// size<=0 的轨(如 document lazy 产物,生成前大小未知)不参与进度核算:
		// 既不计入 total 也不计入 finished,避免 finished 超 total 导致进度 >100%
		if s.size <= 0 {
			continue
		}
		total += s.size // spec.Size 为完整大小(retry_reader 206 据 Content-Range 还原)
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

// storeIdentity resource_store 行的身份键:同 role 内 store_seq 唯一定位一个 store(N-同 role 多 store 支持)
type storeIdentity struct {
	role string
	seq  int
}

// findStoreRowByIdentity 按 (role, store_seq) 身份在 resource_store 行中精确匹配(替代 role 首匹配,避免同 role 歧义)
func findStoreRowByIdentity(rows []*entity.ResourceStore, role string, storeSeq int) *entity.ResourceStore {
	for _, r := range rows {
		if r != nil && r.StoreType == role && r.StoreSeq == storeSeq {
			return r
		}
	}
	return nil
}

// findResumeOffset 在续传偏移列表中按 (role, store_seq) 查找;未命中返回 found=false
func findResumeOffset(offsets []*sdkdto.StoreResumeOffset, role string, storeSeq int) (offset int64, found bool) {
	for _, o := range offsets {
		if o != nil && o.Role == role && int(o.StoreSeq) == storeSeq {
			return o.Offset, true
		}
	}
	return 0, false
}

// resumeSpecSeq 解析 resume 返回的每个 spec 对应的全局 store_seq。
// specs 是未完成子集(已完成 store 不在其中),若按 specs 内 roleCounters 重计 seq,同 role 部分完成时会与
// 全局 store_seq 错位 → findStoreRowByIdentity/findResumeOffset 匹配到已完成行 → 续传覆盖已完成 store。
// 配对:downloaded specs 按 Resume 返回顺序与 streamOffsets 配对(streamOffsets 由主程序按 storeRows 未完成
// downloaded 顺序构造,携带全局 StoreSeq);derived specs 按 role 从 storeRows 未完成 derived 行查(同 role 单例)。
// 依赖插件 Resume/Start 按传入顺序返回 specs 的契约
func resumeSpecSeq(specs []*sdkdto.StoreSpec, streamOffsets []*sdkdto.StoreResumeOffset, storeRows []*entity.ResourceStore, completed map[storeIdentity]struct{}) map[*sdkdto.StoreSpec]int {
	out := make(map[*sdkdto.StoreSpec]int, len(specs))
	dlIdx := 0
	derivedSeq := make(map[string]int)
	for _, spec := range specs {
		if spec == nil {
			continue
		}
		if spec.Generation == entity.GenerationDownloaded {
			if dlIdx < len(streamOffsets) {
				out[spec] = int(streamOffsets[dlIdx].StoreSeq)
				dlIdx++
			}
			continue
		}
		if seq, ok := derivedSeq[spec.Role]; ok {
			out[spec] = seq
			continue
		}
		for _, row := range storeRows {
			if row == nil || row.StoreType != spec.Role || row.Generation != entity.GenerationDerived {
				continue
			}
			if _, complete := completed[storeIdentity{row.StoreType, row.StoreSeq}]; complete {
				continue
			}
			derivedSeq[spec.Role] = row.StoreSeq
			out[spec] = row.StoreSeq
			break
		}
	}
	return out
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
	meta, err := m.deps.WorkMetaLoader.LoadWorkMeta(m.runCtx, m.workId)
	if err != nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 加载已有作品 %d 命名元数据失败: %v", m.taskId, m.workId, err)
		return
	}
	if meta != nil {
		mergeWorkInfo(startResp, meta)
	}
}

// resolveBaseName 算 bas(基准名,不含 ext)与目录相对路径(store/resource/<作者>)。
// bas = FileNameFormat 模板经占位符替换+净化生成(D2 方案 B 保证模板非空);不依赖具体 spec,
// 模板仅消费作品元数据。多 store 文件名的 role/seq/desc 段由 resolveStorePath 按 spec 拼接
func (m *ManagedTask) resolveBaseName(workResp *sdkdto.WorkResponse) (relativePath, bas string) {
	tpl := m.deps.FileNameFormatProvider.GetFileNameFormat()
	tokenData := filename.ExtractTokenData(workResp)
	formatted := filename.FormatFileName(tpl, tokenData)
	bas = filename.SanitizeFileName(formatted)
	authorDir := filename.SanitizeFileName(tokenData.Author)
	// relPath 域用 path.Join（正斜杠），落库/查重基准一致
	relativePath = path.Join("store", "resource", authorDir)
	return
}

// resolveStorePath 按资源级判定拼 store 文件名与路径(thumbnail 普通 role,无特例):
//   - 单 store 资源(multiStore=false):<bas>.<ext>
//   - 多 store 资源(multiStore=true):<bas>_<role>_<seq>[_<描述>].<ext>
//
// seq 为同 role 内 0-based 序号(= store_seq,resume 身份键);描述取自 spec.Description,净化后为空则省略。
// StoreStream 据 relPath 创建文件,relPath 末段须与 fileName 一致,否则多 store 落盘同一 relPath 互相覆盖
func (m *ManagedTask) resolveStorePath(spec *sdkdto.StoreSpec, baseRelPath, bas string, sameRoleSeq int, multiStore bool) (relativePath, fileName string) {
	ext := normalizeExt(spec.Format)
	if !multiStore {
		fileName = bas + ext
	} else {
		name := fmt.Sprintf("%s_%s_%03d", bas, spec.Role, sameRoleSeq)
		if spec.Description != "" {
			if desc := filename.SanitizeFileName(spec.Description); desc != "" {
				name += "_" + desc
			}
		}
		fileName = name + ext
	}
	// filepath.ToSlash 统一正斜杠入库(跨平台规范,与 persistentStore.BuildVariantPath 一致;
	// 避免 Windows 下 filepath.Join 产生反斜杠致 DB 路径分隔符不一致)
	relativePath = filepath.ToSlash(filepath.Join(baseRelPath, fileName))
	return
}

// normalizeExt 规范化扩展名(确保以 "." 开头)
func normalizeExt(format string) string {
	ext := format
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
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

// AllChildrenTerminal 检查所有子任务是否都已进入终态（含显式跳过）
// 终态 = Finished/Failed/PartlyFinished（isTerminalState）或被用户跳过（skipped 标志）。
// 不再把 Created 当终态：未启动的 Created 兄弟需保留父任务运行态，跳过改由 skipped 显式表达。
func (p *ParentTask) AllChildrenTerminal() bool {
	for _, child := range p.GetChildren() {
		s := child.GetState()
		if isTerminalState(s) || child.skipped {
			continue
		}
		return false
	}
	return true
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
