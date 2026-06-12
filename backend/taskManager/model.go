package taskManager

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

	// Start 开始任务
	// 返回资源读取器（io.ReadCloser）、WorkResponse 或错误
	// 调用方负责关闭返回的 ReadCloser
	Start(ctx context.Context, task *entity.Task) (io.ReadCloser, *sdkdto.WorkResponse, error)

	// Pause 暂停任务
	// 返回是否真正暂停成功（插件可能不支持暂停）
	Pause(ctx context.Context, param *sdkdto.TaskResParam) error

	// Stop 停止任务
	Stop(ctx context.Context, param *sdkdto.TaskResParam) error

	// Resume 恢复任务
	Resume(ctx context.Context, param *sdkdto.TaskResParam) (io.ReadCloser, *sdkdto.WorkResponse, error)

	// GetThumbnail 获取缩略图
	GetThumbnail(ctx context.Context, task *entity.Task) (*sdkdto.ThumbnailResponse, error)
}

// WorkInfoSaver 作品完整信息保存接口
type WorkInfoSaver interface {
	SaveWorkInfo(ctx context.Context, task *entity.Task, workResp *sdkdto.WorkResponse) (int64, error)
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
// 封装替换场景下作品 Resource 全部 PersistentStore 的一站式备份和还原
// 当前业务中一个 Work 恰好对应一条 Resource，接口以 workId 为入参
type StoreBackupOrchestrator interface {
	// BackupAllStores 备份作品 Resource 的全部 Store，返回备份清单
	// Resource 的每个 Store 字段（WorkStoreID、ThumbnailStoreID 等）都会产生一条 StoreBackupItem
	BackupAllStores(ctx context.Context, workId int64) []*StoreBackupItem
	// RestoreAllStores 从备份清单还原所有 Store 并更新对应 Resource
	// 仅还原 BackupID > 0 的条目；BackupID == 0 的条目跳过（对应 Resource 字段保持 null）
	RestoreAllStores(ctx context.Context, items []*StoreBackupItem)
}

// StoreStreamer 创建存储记录并返回 StoreWriter
type StoreStreamer interface {
	StoreStream(ctx context.Context, relPath string, fileName string) (storeId int64, writer persistentStore.StoreWriter, err error)
	ResumeStream(ctx context.Context, storeId int64) (writer persistentStore.StoreWriter, err error)
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

// ThumbnailStoreWriter 缩略图存储接口，接受 io.Reader 一步完成存入
type ThumbnailStoreWriter interface {
	Store(ctx context.Context, relPath string, fileName string, reader io.Reader) (int64, error)
}

// TaskDeps ManagedTask 的共享依赖集合
// 将 NewManager 和 NewManagedTask 的大量参数收敛为一个结构体，新增依赖只需改此处
type TaskDeps struct {
	WorkInfoSaver           WorkInfoSaver
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
	ThumbnailStoreWriter    ThumbnailStoreWriter
	Transactor              Transactor
	PendingResourceUpdater  PendingResourceUpdater
	StoreFileCleaner        StoreFileCleaner
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

	// 任务执行器（通过接口调用）
	pluginExec TaskExecutor

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

	// 资源响应（插件返回）
	resourceResp *sdkdto.WorkResponse

	// 暂停/恢复协调通道
	pauseCh   chan struct{} // Pause() 发信号，downloadLoop 消费

	// 下载状态（跨暂停/恢复周期存活）
	currentReader io.ReadCloser               // 当前数据流 reader
	storeWriter   persistentStore.StoreWriter // 当前写入的 StoreWriter
	workStoreId   int64                       // StoreStream 返回的作品资源 store ID
	totalWritten  int64                       // 已写入字节数

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
		pauseCh:    make(chan struct{}, 1),
	}
}

// run 核心执行逻辑
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
	logger.Log.Infof("[TaskManager] run() 入口: taskId=%d, PendingResourceID={Valid:%v, Int64:%d}, continuable=%v", m.taskId, m.task.PendingResourceID.Valid, m.task.PendingResourceID.Int64, m.task.Continuable.Valid)

	// 0. 检查 workdir 是否已配置
	if m.deps.WorkDirProvider.GetWorkDir() == "" {
		logger.Log.Errorf("[TaskManager] 任务 %d 失败: 未配置资源库目录", m.taskId)
		if m.deps.Pusher != nil {
			m.deps.Pusher.PushError(m.taskId, "未配置资源库目录，请先在设置中指定资源库保存位置")
		}
		if m.abortedByPause() {
			return runResultPaused
		}
		m.setFailed("未配置资源库目录，请先在设置中指定资源库保存位置")
		return runResultDone
	}

	// 0.0 检查作品是否已存在（仅首次执行时检查）
	if !m.skipDuplicateCheck && m.deps.WorkChecker != nil && m.task.SiteID.Valid && m.task.SiteWorkID.Valid && m.task.SiteWorkID.String != "" {
		existing, err := m.deps.WorkChecker.GetBySiteAndSiteWorkID(m.ctx, m.task.SiteID.Int64, m.task.SiteWorkID.String)
		if err == nil && existing != nil {
			existingWorkName := ""
			if existing.SiteWorkName.Valid {
				existingWorkName = existing.SiteWorkName.String
			}
			m.existingWorkId = existing.GetID()
			if m.deps.Pusher != nil {
				m.deps.Pusher.PushDuplicateDetected(m.taskId, m.task.TaskName.String, existing.GetID(), existingWorkName)
			}
			m.setState(TaskStateWaitingForInput)
			return runResultNeedConfirm
		}
	}

	// 0.1 替换确认后备份已有资源文件（第二次 run 时执行）
	if m.existingWorkId > 0 {
		m.isReplace = true
		m.storeBackupItems = m.deps.StoreBackupOrchestrator.BackupAllStores(m.ctx, m.existingWorkId)
		m.existingWorkId = 0
	}

	// 1. 调用 CreateWorkInfo 创建作品信息
	workResp, err := m.pluginExec.CreateWorkInfo(m.ctx, m.task)
	if err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d CreateWorkInfo 失败: %v", m.taskId, err)
		if m.abortedByPause() {
			return runResultPaused
		}
		m.setFailed(fmt.Sprintf("创建作品信息失败: %v", err))
		return runResultDone
	}

	// 2. 保存作品完整信息（Work + 周边数据）
	workId, err := m.deps.WorkInfoSaver.SaveWorkInfo(m.ctx, m.task, workResp)
	if err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d 保存作品信息失败: %v", m.taskId, err)
		if m.abortedByPause() {
			return runResultPaused
		}
		m.setFailed(fmt.Sprintf("保存作品信息失败: %v", err))
		return runResultDone
	}
	m.workId = workId

	// 3. 获取资源读取器
	reader, startResp, err := m.pluginExec.Start(m.ctx, m.task)
	if err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d Start 失败: %v", m.taskId, err)
		if m.abortedByPause() {
			return runResultPaused
		}
		m.setFailed(fmt.Sprintf("获取资源读取器失败: %v", err))
		return runResultDone
	}

	// 4. 生成文件保存路径（将 CreateWorkInfo 的作品信息合并到 startResp）
	startResp.Work = workResp.Work
	startResp.SiteAuthors = workResp.SiteAuthors
	startResp.LocalAuthors = workResp.LocalAuthors
	startResp.SiteTags = workResp.SiteTags
	startResp.LocalTags = workResp.LocalTags
	m.resourceResp = startResp
	relativePath, fileName := m.resolveLocalPath(startResp)

	// 4-6. 事务 2：StoreStream(DB记录) + Resource Save + PendingResourceID 更新
	var writer persistentStore.StoreWriter
	var storeId int64
	var resourceId int64

	txErr := m.deps.Transactor.ExecInTransaction(context.Background(), func(txCtx context.Context) error {
		// 4.1 StoreStream：创建文件 + PersistentStore DB 记录（DB 参与事务）
		var storeErr error
		storeId, writer, storeErr = m.deps.StoreStreamer.StoreStream(txCtx, relativePath, fileName)
		if storeErr != nil {
			return storeErr
		}

		// 5. 保存 Resource（替换场景更新 / 新建场景创建）
		var resourceErr error
		resourceId, resourceErr = m.saveResource(txCtx, workId, storeId, startResp)
		if resourceErr != nil {
			return resourceErr
		}

		// 6. 同步更新 pending_resource_id（事务内直接写 DB）
		m.task.PendingResourceID = sql.NullInt64{Int64: resourceId, Valid: true}
		return m.deps.PendingResourceUpdater.UpdatePendingResourceID(txCtx, m.taskId, m.task.PendingResourceID)
	})
	if txErr != nil {
		// 事务回滚：DB 记录已全部回滚，需显式清理文件
		if writer != nil {
			writer.Close()
		}
		m.deps.StoreFileCleaner.CleanupFile(relativePath)
		logger.Log.Errorf("[TaskManager] 任务 %d 创建资源事务失败: %v", m.taskId, txErr)
		if m.abortedByPause() {
			return runResultPaused
		}
		m.setFailed(fmt.Sprintf("创建资源失败: %v", txErr))
		return runResultDone
	}

	// 7. 保存下载状态并进入下载循环
	m.currentReader = reader
	m.storeWriter = writer
	m.workStoreId = storeId
	m.totalWritten = 0

	return m.downloadLoop()
}

// saveResource 保存 Resource（事务内调用）
// 替换场景：查询已有 Resource 并更新 WorkStoreID
// 新建场景：创建新 Resource
func (m *ManagedTask) saveResource(ctx context.Context, workId int64, storeId int64, startResp *sdkdto.WorkResponse) (int64, error) {
	var resourceId int64

	if m.isReplace {
		// 替换场景：查询已有 Resource 并更新
		// 优先从备份清单获取 ResourceID（有 Store 被备份的情况）
		// 备份清单为空时（旧数据无 Store），通过 workId 查询已有 Resource
		var existingResource *entity.Resource
		for _, item := range m.storeBackupItems {
			if item.StoreType == backup.StoreTypeWork {
				existing, err := m.deps.ResourceReader.GetById(ctx, item.ResourceID)
				if err != nil || existing == nil {
					return 0, fmt.Errorf("查询已有 Resource(id=%d) 失败: %w", item.ResourceID, err)
				}
				existingResource = existing
				break
			}
		}
		if existingResource == nil {
			// 备份清单中无 WorkStore 条目，通过 workId 查询已有 Resource
			resources, queryErr := m.deps.ResourceReader.GetEnabledByWorkId(ctx, workId)
			if queryErr != nil {
				return 0, fmt.Errorf("查询作品 %d 资源失败: %w", workId, queryErr)
			}
			if len(resources) > 0 {
				existingResource = resources[0]
			}
			// len(resources) == 0：上一次执行创建了 Work 但未创建 Resource，降级为新建
		}
		if existingResource != nil {
			// 更新已有 Resource
			existingResource.WorkStoreID = sql.NullInt64{Int64: storeId, Valid: true}
			existingResource.ThumbnailStoreID = sql.NullInt64{Valid: false} // 清除旧缩略图，由 saveThumbnail 重新生成
			existingResource.SuggestName = sql.NullString{String: startResp.Resource.SuggestName, Valid: startResp.Resource.SuggestName != ""}
			existingResource.ResourceComplete = 0 // 下载未完成
			if err := m.deps.ResourceUpdater.Update(ctx, existingResource); err != nil {
				return 0, fmt.Errorf("更新 Resource 失败: %w", err)
			}
			resourceId = existingResource.GetID()
		}
	}

	if resourceId == 0 {
		// 非替换场景：创建新 Resource
		resource := entity.NewResource()
		resource.WorkID = workId
		resource.TaskID = m.task.GetID()
		resource.Enabled = true
		resource.SuggestName = sql.NullString{String: startResp.Resource.SuggestName, Valid: startResp.Resource.SuggestName != ""}
		resource.ResourceComplete = 0 // 下载未完成
		resource.WorkStoreID = sql.NullInt64{Int64: storeId, Valid: true}

		var err error
		resourceId, err = m.deps.ResourceSaver.Save(ctx, resource)
		if err != nil {
			return 0, fmt.Errorf("保存资源到数据库失败: %w", err)
		}
	}

	return resourceId, nil
}

// downloadLoop 可暂停的下载循环
// 正常下载 → 收到 pauseCh 进入 drain → 返回 runResultPaused 退出 goroutine
func (m *ManagedTask) downloadLoop() runResult {
	buf := make([]byte, 32*1024)
	defer func() {
		m.currentReader.Close()
	}()
	downloadStart := time.Now()
	totalSize := int64(0)
	if m.resourceResp != nil && m.resourceResp.Resource != nil {
		totalSize = m.resourceResp.Resource.Size
	}

	for {
		select {
		case <-m.pauseCh:
			// 收到暂停信号，drain 所有已发送数据直到上游关闭
			logger.Log.Infof("[TaskManager] downloadLoop 收到暂停信号: taskId=%d, totalWritten=%d/%d, elapsed=%v", m.taskId, m.totalWritten, totalSize, time.Since(downloadStart))
			m.drainAndPause(buf)
			return runResultPaused
		case <-m.ctx.Done():
			m.storeWriter.Abort() // 清理文件和 DB 记录
			m.setFailed("任务被取消")
			return runResultDone
		default:
		}

		n, readErr := m.currentReader.Read(buf)
		if n > 0 {
			written, writeErr := m.storeWriter.Write(buf[:n])
			if written > 0 {
				m.totalWritten += int64(written)
			}
			if writeErr != nil {
				logger.Log.Errorf("[TaskManager] 任务 %d 写入文件失败: %v", m.taskId, writeErr)
				m.storeWriter.Abort()
				m.setFailed(fmt.Sprintf("写入文件失败: %v", writeErr))
				return runResultDone
			}
			// 报告进度
			if m.onProgress != nil {
				m.onProgress(m.taskId, m.resourceResp.Resource.Size, m.totalWritten)
			}
		}
			if readErr != nil {
				if readErr == io.EOF {
					if err := m.storeWriter.Complete(); err != nil {
						logger.Log.Errorf("[TaskManager] 任务 %d Complete 失败: %v", m.taskId, err)
						m.setFailed(fmt.Sprintf("完成存储失败: %v", err))
						return runResultDone
					}
					// 校验下载完整性
					if m.resourceResp.Resource.Size > 0 && m.totalWritten < m.resourceResp.Resource.Size {
						logger.Log.Errorf("[TaskManager] 任务 %d 下载不完整: 已下载 %d / 预期 %d", m.taskId, m.totalWritten, m.resourceResp.Resource.Size)
						m.setFailed(fmt.Sprintf("下载不完整: 已下载 %d / 预期 %d", m.totalWritten, m.resourceResp.Resource.Size))
						return runResultDone
					}
					// 下载完成，向插件请求缩略图并保存（在清除 PendingResourceID 之前，saveThumbnail 需要读取 Resource 实体）
					m.saveThumbnail()
					// 清除 pending_resource_id
					m.clearPendingResourceID()
					m.setState(TaskStateFinished)
					logger.Log.Infof("[TaskManager] downloadLoop 正常完成: taskId=%d, totalWritten=%d/%d, elapsed=%v", m.taskId, m.totalWritten, totalSize, time.Since(downloadStart))
					return runResultDone
				}
				// 非 EOF 读取错误：可能是暂停导致或真正的读取失败
				s := m.GetState()
				if s == TaskStatePausing || s == TaskStatePaused {
					m.drainAndPause(buf)
					return runResultPaused
				}
				logger.Log.Errorf("[TaskManager] 任务 %d 下载读取失败: %v", m.taskId, readErr)
				m.storeWriter.Abort()
				m.setFailed(fmt.Sprintf("下载读取失败: %v", readErr))
				return runResultDone
			}
		}
	}

	// drainReader 排空 reader 中所有已发送数据并写入文件
	// 循环读取直到 reader 返回错误或 EOF（插件关闭上游时触发）
	func (m *ManagedTask) drainReader(buf []byte) {
		for {
			n, err := m.currentReader.Read(buf)
			if n > 0 {
				if written, writeErr := m.storeWriter.Write(buf[:n]); writeErr == nil {
					m.totalWritten += int64(written)
				}
			}
			if err != nil {
				return
			}
		}
	}

	// drainAndPause 排空缓冲区、同步并关闭写入器、设置暂停状态
	func (m *ManagedTask) drainAndPause(buf []byte) {
		m.drainReader(buf)
		m.storeWriter.Sync()
		m.storeWriter.Close()
		m.setState(TaskStatePaused)
	}

// resumeFromPersistedState 从数据库恢复的跨重启续传
// 任务在之前的运行中已暂停，pending_resource_id 已持久化到数据库
// 本方法跳过 CreateWorkInfo/SaveWorkInfo/Start 流程，直接调用插件 Resume 续传
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

	// 2. 通过 WorkStoreID 获取 PersistentStore 记录，计算已下载字节数
	if !resource.WorkStoreID.Valid {
		logger.Log.Warnf("[TaskManager] 任务 %d Resource 无有效的 WorkStoreID，降级为完整重新执行", m.taskId)
		return m.run()
	}

	workDir := m.deps.WorkDirProvider.GetWorkDir()
	if workDir == "" {
		logger.Log.Errorf("[TaskManager] 任务 %d 失败: 未配置资源库目录", m.taskId)
		m.setFailed("未配置资源库目录，请先在设置中指定资源库保存位置")
		return runResultDone
	}

	store, err := m.deps.StoreReader.GetById(m.ctx, resource.WorkStoreID.Int64)
	if err != nil || store == nil {
		logger.Log.Warnf("[TaskManager] 任务 %d PersistentStore(id=%d) 不存在，降级为完整重新执行", m.taskId, resource.WorkStoreID.Int64)
		return m.run()
	}

	absPath := m.deps.StoreReader.GetAbsPath(store)
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 本地文件不存在 [%s]，降级为完整重新执行: %v", m.taskId, absPath, err)
		return m.run()
	}
	downloadedBytes := fileInfo.Size()

	logger.Log.Infof("[TaskManager] 任务 %d 跨重启续传: resourceID=%d, storeId=%d, downloadedBytes=%d", m.taskId, resource.GetID(), store.GetID(), downloadedBytes)

	// 3. 调用插件 Resume
	param := &sdkdto.TaskResParam{
		Task:            dto.NewTaskDTO(m.task),
		ResourceID:      m.task.PendingResourceID.Int64,
		DownloadedBytes: downloadedBytes,
	}
	newReader, newResp, err := m.pluginExec.Resume(m.ctx, param)
	if err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d 跨重启 Resume 失败: %v", m.taskId, err)
		m.setFailed(fmt.Sprintf("跨重启续传失败: %v", err))
		return runResultDone
	}

	// 4. 设置运行状态
	m.workId = resource.WorkID
	m.workStoreId = store.GetID()

	// 4.1 初始化 resourceResp（应用重启后 resourceResp 为 nil，从数据库 Resource 实体恢复 Size/Format）
	if m.resourceResp == nil || m.resourceResp.Resource == nil {
		m.resourceResp = &sdkdto.WorkResponse{
			Resource: &sdkdto.TaskResourceDTO{
				Size:   downloadedBytes,
				Format: store.FilenameExtension.String,
			},
		}
	}

	// 5. 根据插件响应的 Continuable 决定续传策略
	continuable := newResp != nil && newResp.Resource != nil &&
		newResp.Resource.Continuable != nil && *newResp.Resource.Continuable
	logger.Log.Infof("[TaskManager] resumeFromPersistedState Continuable 判定: taskId=%d, newResp=%v, continuable=%v, downloadedBytes=%d", m.taskId, newResp != nil, continuable, downloadedBytes)

	if continuable {
		// 可续传：通过 ResumeStream 以 append 模式打开文件
		m.totalWritten = downloadedBytes
		resumeWriter, err := m.deps.StoreStreamer.ResumeStream(m.ctx, store.GetID())
		if err != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d ResumeStream 失败: %v", m.taskId, err)
			m.setFailed(fmt.Sprintf("续传打开存储失败: %v", err))
			return runResultDone
		}
		m.storeWriter = resumeWriter
	} else {
		// 不可续传：通过 StoreStream 重新创建（覆盖旧记录）
		// 先 Abort 旧的未完成记录
		// 注意：这里不能直接 Abort 旧记录因为 ResumeStream 可能失败，但 StoreStream 会处理已有 relPath 的情况
		m.totalWritten = 0
		relPath := ""
		fileName := ""
		if store.FilePath.Valid {
			relPath = store.FilePath.String
		}
		if store.FileName.Valid {
			fileName = store.FileName.String
		}
		newStoreId, newWriter, err := m.deps.StoreStreamer.StoreStream(m.ctx, relPath, fileName)
		if err != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d StoreStream 失败: %v", m.taskId, err)
			m.setFailed(fmt.Sprintf("续传重新创建存储失败: %v", err))
			return runResultDone
		}
		m.workStoreId = newStoreId
		m.storeWriter = newWriter
	}
	m.currentReader = newReader

	return m.downloadLoop()
}

// Pause 暂停任务
func (m *ManagedTask) Pause() error {
	if m.state.Load() != int32(TaskStateProcessing) {
		return ErrTaskNotProcessing
	}
	m.setState(TaskStatePausing)

	// 下载尚未开始的场景（setup 阶段），直接取消 context
	if m.currentReader == nil {
		logger.Log.Infof("[TaskManager] Pause: taskId=%d 在 setup 阶段暂停，直接取消", m.taskId)
		m.cancel()
		m.setState(TaskStatePaused)
		return nil
	}

	logger.Log.Infof("[TaskManager] Pause: taskId=%d 在 download 阶段暂停, PendingResourceID={Valid:%v, Int64:%d}, totalWritten=%d",
		m.taskId, m.task.PendingResourceID.Valid, m.task.PendingResourceID.Int64, m.totalWritten)

	// ① 先通知 downloadLoop 进入 drain 模式
	m.pauseCh <- struct{}{}

	// ② 再通知插件暂停（插件关闭上游，导致 reader 返回 EOF，drain 完成）
	param := &sdkdto.TaskResParam{
		Task:       dto.NewTaskDTO(m.task),
		ResourceID: m.task.PendingResourceID.Int64,
	}
	if err := m.pluginExec.Pause(m.ctx, param); err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d 插件 Pause 失败: %v", m.taskId, err)
	}

		//    - 正常暂停：drainAndPause → Paused
		//    - 下载完成：EOF → Finished（数据已全部写入）
		//    两种情况都使任务进入稳定状态，不会阻塞 PauseTaskTree

	return nil
}

// prepareForResume 重置任务的运行时状态，准备重新调度
// 由 ResumeTaskTree 在 tryDispatch 前调用
func (m *ManagedTask) prepareForResume() {
	m.cancel()
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.pauseCh = make(chan struct{}, 1)
	m.currentReader = nil
	m.storeWriter = nil
	m.totalWritten = 0
	// 有 PendingResourceID 走 resumeFromPersistedState（内部根据插件响应 Continuable 决定续传/重新下载）
	// 无 PendingResourceID 走 run()（从头执行）
	m.resumeFromDB = m.task.PendingResourceID.Valid
	logger.Log.Infof("[TaskManager] prepareForResume: taskId=%d, PendingResourceID={Valid:%v, Int64:%d}, resumeFromDB=%v",
		m.taskId, m.task.PendingResourceID.Valid, m.task.PendingResourceID.Int64, m.resumeFromDB)
}

// Stop 停止任务
func (m *ManagedTask) Stop() {
	m.setState(TaskStateStopping)
	m.cancel() // 触发 context 取消（中断 downloadLoop 或暂停等待）

	if m.storeWriter != nil {
		m.storeWriter.Abort()
	}

	if m.resourceResp != nil {
		param := &sdkdto.TaskResParam{
			Task:       dto.NewTaskDTO(m.task),
			ResourceID: m.task.PendingResourceID.Int64,
		}
		if err := m.pluginExec.Stop(m.ctx, param); err != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d Stop 失败: %v", m.taskId, err)
		}
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
func (m *ManagedTask) setFailed(errMsg string) {
	m.errorMessage = errMsg
	m.setState(TaskStateFailed)
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
// Pause 在 setup 阶段（currentReader == nil）会直接取消 context
// run() 的 setup 代码遇到此类情况应直接退出，不覆盖 Pause 已设置的 Paused 状态
func (m *ManagedTask) abortedByPause() bool {
	if m.ctx.Err() == nil {
		return false
	}
	s := TaskState(m.state.Load())
	return s == TaskStatePausing || s == TaskStatePaused
}

// resolveLocalPath 根据资源信息和文件名模板生成本地文件保存路径
// 返回 relativePath（相对于 workDir）和 fileName
func (m *ManagedTask) resolveLocalPath(startResp *sdkdto.WorkResponse) (relativePath, fileName string) {
	res := startResp.Resource
	tpl := m.deps.FileNameFormatProvider.GetFileNameFormat()

	// 模板为空时使用插件建议的文件名
	if tpl == "" {
		fileName = m.buildSuggestedFileName(res)
		relativePath = filepath.Join("store", "resource", fileName)
		return
	}

	// 模板模式：提取占位符数据 → 格式化 → 净化 → 拼接扩展名 → 按作者分目录
	tokenData := filename.ExtractTokenData(startResp)
	formatted := filename.FormatFileName(tpl, tokenData)
	sanitizedFileName := filename.SanitizeFileName(formatted)

	ext := res.Format
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	fileName = sanitizedFileName + ext

	authorDir := filename.SanitizeFileName(tokenData.Author)

	relativePath = filepath.Join("store", "resource", authorDir, fileName)
	return
}

// buildSuggestedFileName 根据插件建议文件名构建最终文件名（含扩展名）
// 优先使用插件 SuggestName，仅保留纯文件名部分并进行清洗，扩展名由 Format 字段控制
func (m *ManagedTask) buildSuggestedFileName(res *sdkdto.TaskResourceDTO) string {
	name := res.SuggestName
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
	ext := res.Format
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return name + ext
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

// saveThumbnail 向插件请求缩略图并保存
func (m *ManagedTask) saveThumbnail() {
	// 前置检查：无插件数据时跳过
	if !m.task.PluginData.Valid || m.task.PluginData.String == "" {
		return
	}

	// 获取当前 Resource 实体
	resource, err := m.deps.ResourceReader.GetById(m.ctx, m.task.PendingResourceID.Int64)
	if err != nil || resource == nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 缩略图生成跳过: 获取 Resource 失败: %v", m.taskId, err)
		return
	}
	// 已有缩略图时跳过
	if resource.ThumbnailStoreID.Valid {
		return
	}

	// 调用插件获取缩略图
	thumbResp, err := m.pluginExec.GetThumbnail(m.ctx, m.task)
	if err != nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 缩略图获取失败: %v", m.taskId, err)
		return
	}
	if thumbResp == nil || len(thumbResp.Data) == 0 {
		return
	}

	// 确定格式
	thumbFormat := thumbResp.Format
	if thumbFormat == "" {
		thumbFormat = "jpg"
	}

	// 获取资源文件信息，构建缩略图相对路径和文件名
	store, err := m.deps.StoreReader.GetById(m.ctx, m.workStoreId)
	if err != nil || store == nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 缩略图生成跳过: 获取 store 记录失败: %v", m.taskId, err)
		return
	}
	thumbRelPath := buildThumbnailRelPath(store.FilePath.String, thumbFormat)
	thumbFileName := buildThumbnailFileName(store.FileName.String, thumbFormat)

	// 通过 Store 一步完成写入
	storeID, err := m.deps.ThumbnailStoreWriter.Store(
		m.ctx, thumbRelPath, thumbFileName, bytes.NewReader(thumbResp.Data),
	)
	if err != nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 缩略图存储失败: %v", m.taskId, err)
		return
	}

	// 更新 Resource 记录
	resource.ThumbnailStoreID = sql.NullInt64{Int64: storeID, Valid: true}
	m.deps.ResourceUpdater.Update(m.ctx, resource)
}

// buildThumbnailRelPath 构建缩略图相对路径
// 去除 WorkStore FilePath 中的 "store/resource/" 前缀，
// 将缩略图直接放在 "store/thumbnail/作者/" 下，避免 "store/thumbnail/resource/作者/" 的冗余层级。
func buildThumbnailRelPath(resourceRelPath string, thumbFormat string) string {
	// 去除 "store/resource/" 前缀
	relPath := strings.TrimPrefix(resourceRelPath, "store/resource")
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
