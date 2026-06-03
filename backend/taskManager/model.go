package taskManager

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util/filename"
	sdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
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
	Start(ctx context.Context, task *entity.Task, workId int64) (io.ReadCloser, *sdkdto.WorkResponse, error)

	// Pause 暂停任务
	// 返回是否真正暂停成功（插件可能不支持暂停）
	Pause(ctx context.Context, param *sdkdto.TaskResParam) error

	// Stop 停止任务
	Stop(ctx context.Context, param *sdkdto.TaskResParam) error

	// Resume 恢复任务
	Resume(ctx context.Context, param *sdkdto.TaskResParam) (io.ReadCloser, *sdkdto.WorkResponse, error)
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

// ResourceBackupOrchestrator 资源备份编排器接口
// 封装资源备份/禁用/还原的完整生命周期，由 backup 包的 ResourceBackupOrchestrator 实现
type ResourceBackupOrchestrator interface {
	// BackupAndDisable 备份已有作品的启用资源并标记为未启用，返回已备份的资源 ID 列表
	BackupAndDisable(ctx context.Context, workId int64, workDir string) []int64
	// Restore 还原已备份的资源
	Restore(ctx context.Context, resourceIds []int64, workDir string)
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

	// 作品信息保存器
	workInfoSaver WorkInfoSaver
	// 资源保存器
	resourceSaver ResourceSaver
	// 工作目录提供者（实时读取，不缓存）
	workDirProvider WorkDirProvider
	// 文件名格式模板提供者（实时读取，不缓存）
	fileNameFormatProvider FileNameFormatProvider

	// 作品查重
	workChecker WorkChecker
	// 资源查询（查找已有作品的资源文件）
	resourceReader ResourceReader
	// 资源备份编排器
	backupOrchestrator ResourceBackupOrchestrator
	// 已备份的资源 ID 列表（用于任务失败时还原）
	backedUpResourceIds []int64
	// 进度推送器
	pusher TaskProgressPusher
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
	drainDone chan struct{} // downloadLoop drain 完成后通知 Pause()

	// 下载状态（跨暂停/恢复周期存活）
	currentReader io.ReadCloser // 当前数据流 reader
	currentFile   *os.File      // 当前写入文件句柄
	localPath     string        // 本地文件绝对路径
	totalWritten  int64         // 已写入字节数

	// 回调函数
	onStateChange      func(taskId int64, oldState, newState TaskState, errMsg string)
	onProgress         func(taskId int64, total int64, finished int64)
	onResourceIDUpdate func(taskId int64, resourceID sql.NullInt64)
}

// NewManagedTask 创建托管任务
func NewManagedTask(taskId, parentId int64, task *entity.Task, pluginExec TaskExecutor, workInfoSaver WorkInfoSaver, resourceSaver ResourceSaver, workDirProvider WorkDirProvider, fileNameFormatProvider FileNameFormatProvider, workChecker WorkChecker, resourceReader ResourceReader, backupOrchestrator ResourceBackupOrchestrator, pusher TaskProgressPusher) *ManagedTask {
	ctx, cancel := context.WithCancel(context.Background())
	return &ManagedTask{
		taskId:                 taskId,
		parentId:               parentId,
		state:                  atomic.Int32{},
		ctx:                    ctx,
		cancel:                 cancel,
		done:                   make(chan struct{}),
		pluginExec:             pluginExec,
		workInfoSaver:          workInfoSaver,
		resourceSaver:          resourceSaver,
		workDirProvider:        workDirProvider,
		fileNameFormatProvider: fileNameFormatProvider,
		workChecker:            workChecker,
		resourceReader:         resourceReader,
		backupOrchestrator:     backupOrchestrator,
		pusher:                 pusher,
		task:                   task,
		workId:                 taskId,
		pauseCh:                make(chan struct{}, 1),
	}
}

// run 核心执行逻辑
func (m *ManagedTask) run() runResult {
	// 最先注册，最后执行：任务失败时还原已备份的资源
	defer func() {
		if m.GetState() == TaskStateFailed && len(m.backedUpResourceIds) > 0 {
			workDir := m.workDirProvider.GetWorkDir()
			logger.Log.Infof("[TaskManager] 任务 %d 失败，开始还原 %d 个已备份资源", m.taskId, len(m.backedUpResourceIds))
			m.backupOrchestrator.Restore(m.ctx, m.backedUpResourceIds, workDir)
			m.backedUpResourceIds = nil
		}
	}()
	// panic recovery：后注册，先执行
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d panic: %v", m.taskId, r)
			m.setFailed(fmt.Sprintf("任务执行 panic: %v", r))
		}
	}()

	m.setState(TaskStateProcessing)
	logger.Log.Infof("[TaskManager] run() 入口: taskId=%d, PendingResourceID={Valid:%v, Int64:%d}, continuable=%v", m.taskId, m.task.PendingResourceID.Valid, m.task.PendingResourceID.Int64, m.task.Continuable.Valid)

	// 0. 检查 workdir 是否已配置
	if m.workDirProvider.GetWorkDir() == "" {
		logger.Log.Errorf("[TaskManager] 任务 %d 失败: 未配置资源库目录", m.taskId)
		if m.pusher != nil {
			m.pusher.PushError(m.taskId, "未配置资源库目录，请先在设置中指定资源库保存位置")
		}
		if m.abortedByPause() {
			return runResultPaused
		}
		m.setFailed("未配置资源库目录，请先在设置中指定资源库保存位置")
		return runResultDone
	}

	// 0.0 检查作品是否已存在（仅首次执行时检查）
	if !m.skipDuplicateCheck && m.workChecker != nil && m.task.SiteID.Valid && m.task.SiteWorkID.Valid && m.task.SiteWorkID.String != "" {
		existing, err := m.workChecker.GetBySiteAndSiteWorkID(m.ctx, m.task.SiteID.Int64, m.task.SiteWorkID.String)
		if err == nil && existing != nil {
			existingWorkName := ""
			if existing.SiteWorkName.Valid {
				existingWorkName = existing.SiteWorkName.String
			}
			m.existingWorkId = existing.GetID()
			if m.pusher != nil {
				m.pusher.PushDuplicateDetected(m.taskId, m.task.TaskName.String, existing.GetID(), existingWorkName)
			}
			m.setState(TaskStateWaitingForInput)
			return runResultNeedConfirm
		}
	}

	// 0.1 替换确认后备份已有资源文件（第二次 run 时执行）
	if m.existingWorkId > 0 {
		workDir := m.workDirProvider.GetWorkDir()
		m.backedUpResourceIds = m.backupOrchestrator.BackupAndDisable(m.ctx, m.existingWorkId, workDir)
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
	workId, err := m.workInfoSaver.SaveWorkInfo(m.ctx, m.task, workResp)
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
	reader, startResp, err := m.pluginExec.Start(m.ctx, m.task, m.workId)
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
	localPath, relativePath, fileName := m.resolveLocalPath(startResp)

	// 4.1 确保资源目录存在
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d 创建资源目录失败: %v", m.taskId, err)
		if m.abortedByPause() {
			return runResultPaused
		}
		m.setFailed(fmt.Sprintf("创建资源目录失败: %v", err))
		return runResultDone
	}

	// 5. 保存 Resource 到数据库
	resource := &entity.Resource{
		BaseEntity:        &model.BaseEntity{},
		WorkID:            workId,
		TaskID:            m.task.GetID(),
		Enabled:           true,
		FilePath:          sql.NullString{String: relativePath, Valid: true},
		FileName:          sql.NullString{String: fileName, Valid: true},
		FilenameExtension: sql.NullString{String: startResp.Resource.Format, Valid: true},
		SuggestName:       sql.NullString{String: startResp.Resource.SuggestName, Valid: startResp.Resource.SuggestName != ""},
		ResourceSize:      sql.NullInt64{Int64: startResp.Resource.Size, Valid: true},
		Workdir:           sql.NullString{String: m.workDirProvider.GetWorkDir(), Valid: true},
		ResourceComplete:  startResp.Resource.Completeness,
	}
	resourceId, err := m.resourceSaver.Save(m.ctx, resource)
	if err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d 保存资源失败: %v", m.taskId, err)
		if m.abortedByPause() {
			return runResultPaused
		}
		m.setFailed(fmt.Sprintf("保存资源到数据库失败: %v", err))
		return runResultDone
	}

	// 6. 更新任务的 pendingResourceId 并持久化到数据库
	m.task.PendingResourceID = sql.NullInt64{Int64: resourceId, Valid: true}
	if m.onResourceIDUpdate != nil {
		m.onResourceIDUpdate(m.taskId, m.task.PendingResourceID)
	}

	// 7. 创建文件并进入下载循环
	file, err := os.Create(localPath)
	if err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d 创建文件失败 [%s]: %v", m.taskId, localPath, err)
		if m.abortedByPause() {
			return runResultPaused
		}
		m.setFailed(fmt.Sprintf("创建本地文件失败: %v", err))
		return runResultDone
	}

	// 保存下载状态到 ManagedTask 字段（downloadLoop 和 resumeFromPersistedState 使用）
	m.currentReader = reader
	m.currentFile = file
	m.localPath = localPath
	m.totalWritten = 0

	return m.downloadLoop()
}

// downloadLoop 可暂停的下载循环
// 正常下载 → 收到 pauseCh 进入 drain → 返回 runResultPaused 退出 goroutine
func (m *ManagedTask) downloadLoop() runResult {
	buf := make([]byte, 32*1024)
	defer func() {
		m.currentReader.Close()
		m.currentFile.Close()
	}()

	for {
		select {
		case <-m.pauseCh:
			// 收到暂停信号，drain 所有已发送数据直到上游关闭
		logger.Log.Infof("[TaskManager] downloadLoop 收到暂停信号: taskId=%d, totalWritten=%d", m.taskId, m.totalWritten)
			m.drainReader(buf)
			m.currentFile.Sync()
			close(m.drainDone)
		logger.Log.Infof("[TaskManager] downloadLoop drain 完成: taskId=%d, totalWritten=%d", m.taskId, m.totalWritten)

			m.setState(TaskStatePaused)
			// 暂停后直接退出 goroutine，释放信号量，恢复时统一重新调度
			return runResultPaused
		case <-m.ctx.Done():
			m.setFailed("任务被取消")
			return runResultDone
		default:
		}

		n, readErr := m.currentReader.Read(buf)
		if n > 0 {
			written, writeErr := m.currentFile.Write(buf[:n])
			if written > 0 {
				m.totalWritten += int64(written)
			}
			if writeErr != nil {
				logger.Log.Errorf("[TaskManager] 任务 %d 写入文件失败: %v", m.taskId, writeErr)
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
				m.currentFile.Sync()
				// 校验下载完整性
				if m.resourceResp.Resource.Size > 0 && m.totalWritten < m.resourceResp.Resource.Size {
					logger.Log.Errorf("[TaskManager] 任务 %d 下载不完整: 已下载 %d / 预期 %d", m.taskId, m.totalWritten, m.resourceResp.Resource.Size)
					m.setFailed(fmt.Sprintf("下载不完整: 已下载 %d / 预期 %d", m.totalWritten, m.resourceResp.Resource.Size))
					return runResultDone
				}
				// 下载完成，清除 pending_resource_id
				m.clearPendingResourceID()
				m.setState(TaskStateFinished)
				return runResultDone
			}
			// 暂停导致的读取失败：drain 已缓冲数据，走暂停流程
			// download 阶段不取消 context，直接检查状态
			s := m.GetState()
			if s == TaskStatePausing || s == TaskStatePaused {
				m.drainReader(buf)
				m.currentFile.Sync()
				close(m.drainDone)
				m.setState(TaskStatePaused)
				return runResultPaused
			}
			logger.Log.Errorf("[TaskManager] 任务 %d 下载读取失败: %v", m.taskId, readErr)
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
			if written, writeErr := m.currentFile.Write(buf[:n]); writeErr == nil {
				m.totalWritten += int64(written)
			}
		}
		if err != nil {
			return
		}
	}
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

	m.setState(TaskStateProcessing)

	// 1. 通过 pending_resource_id 加载 Resource 实体
	if !m.task.PendingResourceID.Valid {
		logger.Log.Warnf("[TaskManager] 任务 %d 无有效的 pending_resource_id，降级为完整重新执行", m.taskId)
		return m.run()
	}
	resource, err := m.resourceReader.GetById(m.ctx, m.task.PendingResourceID.Int64)
	if err != nil || resource == nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 加载 Resource(id=%d) 失败: %v，降级为完整重新执行", m.taskId, m.task.PendingResourceID.Int64, err)
		return m.run()
	}

	// 2. 计算本地文件绝对路径和已下载字节数
	workDir := m.workDirProvider.GetWorkDir()
	if workDir == "" {
		logger.Log.Errorf("[TaskManager] 任务 %d 失败: 未配置资源库目录", m.taskId)
		m.setFailed("未配置资源库目录，请先在设置中指定资源库保存位置")
		return runResultDone
	}
	localPath := filepath.Join(workDir, "resource", resource.FilePath.String)
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 本地文件不存在 [%s]，降级为完整重新执行: %v", m.taskId, localPath, err)
		return m.run()
	}
	downloadedBytes := fileInfo.Size()

	logger.Log.Infof("[TaskManager] 任务 %d 跨重启续传: resourceID=%d, localPath=%s, downloadedBytes=%d", m.taskId, resource.GetID(), localPath, downloadedBytes)

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
	m.localPath = localPath

	// 4.1 初始化 resourceResp（应用重启后 resourceResp 为 nil，从数据库 Resource 实体恢复 Size/Format）
	if m.resourceResp == nil || m.resourceResp.Resource == nil {
		m.resourceResp = &sdkdto.WorkResponse{
			Resource: &sdkdto.TaskResourceDTO{
				Size:   resource.ResourceSize.Int64,
				Format: resource.FilenameExtension.String,
			},
		}
	}

	// 5. 根据插件响应的 Continuable 决定续传策略
	continuable := newResp != nil && newResp.Resource != nil &&
		newResp.Resource.Continuable != nil && *newResp.Resource.Continuable
		logger.Log.Infof("[TaskManager] resumeFromPersistedState Continuable 判定: taskId=%d, newResp=%v, continuable=%v, downloadedBytes=%d", m.taskId, newResp != nil, continuable, downloadedBytes)

	var file *os.File
	if continuable {
		// 可续传：以追加模式打开文件，从已下载位置继续
		m.totalWritten = downloadedBytes
		file, err = os.OpenFile(localPath, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d 追加打开文件失败 [%s]: %v", m.taskId, localPath, err)
			m.setFailed(fmt.Sprintf("续传追加打开文件失败: %v", err))
			return runResultDone
		}
	} else {
		// 不可续传：截断文件从头开始下载
		m.totalWritten = 0
		file, err = os.OpenFile(localPath, os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d 截断打开文件失败 [%s]: %v", m.taskId, localPath, err)
			m.setFailed(fmt.Sprintf("续传截断打开文件失败: %v", err))
			return runResultDone
		}
	}
	m.currentFile = file
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

	// 准备 drain 完成信号
	m.drainDone = make(chan struct{})

	// ① 先通知 downloadLoop 进入 drain 模式
	m.pauseCh <- struct{}{}

	// ② 再通知插件暂停（插件关闭上游，导致 reader 返回 EOF，drain 完成）
	param := &sdkdto.TaskResParam{
		Task:       dto.NewTaskDTO(m.task),
		ResourceID: m.resourceResp.Resource.ResourceID,
	}
	if err := m.pluginExec.Pause(m.ctx, param); err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d 插件 Pause 失败: %v", m.taskId, err)
	}

	// ③ 等待 drain 完成
	<-m.drainDone

	return nil
}

// prepareForResume 重置任务的运行时状态，准备重新调度
// 由 ResumeTaskTree 在 tryDispatch 前调用
func (m *ManagedTask) prepareForResume() {
	m.cancel()
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.pauseCh = make(chan struct{}, 1)
	m.currentReader = nil
	m.currentFile = nil
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

	if m.resourceResp != nil {
		param := &sdkdto.TaskResParam{
			Task:       dto.NewTaskDTO(m.task),
			ResourceID: m.resourceResp.Resource.ResourceID,
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

// isRunning 检查任务是否在处理中
func (m *ManagedTask) isRunning() bool {
	return m.state.Load() == int32(TaskStateProcessing)
}

// isPaused 检查任务是否已暂停
func (m *ManagedTask) isPaused() bool {
	return m.state.Load() == int32(TaskStatePaused)
}

// isStopped 检查任务是否已停止
func (m *ManagedTask) isStopped() bool {
	return m.state.Load() == int32(TaskStateFailed)
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
// 返回值: absSavePath 绝对保存路径, relativePath 相对于 workdir/resource/ 的相对路径, fileName 文件名
func (m *ManagedTask) resolveLocalPath(startResp *sdkdto.WorkResponse) (absSavePath, relativePath, fileName string) {
	res := startResp.Resource
	workDir := m.workDirProvider.GetWorkDir()
	tpl := m.fileNameFormatProvider.GetFileNameFormat()

	// 模板为空时使用插件建议的文件名
	if tpl == "" {
		fileName = m.buildSuggestedFileName(res)
		relativePath = fileName
		absSavePath = filepath.Join(workDir, "resource", fileName)
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

	relativePath = filepath.Join(authorDir, fileName)
	absSavePath = filepath.Join(workDir, "resource", authorDir, fileName)
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
		name = res.RemotePath
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

// RemoveChild 移除子任务
func (p *ParentTask) RemoveChild(taskId int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.children, taskId)
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
func (p *ParentTask) AllChildrenTerminal() bool {
	for _, child := range p.GetChildren() {
		s := child.GetState()
		if s != TaskStateFinished && s != TaskStateFailed {
			return false
		}
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
