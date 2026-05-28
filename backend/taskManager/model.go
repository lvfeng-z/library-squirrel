package taskManager

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/library-squirrel/backend/backup"
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
)

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
	Resume(ctx context.Context, param *sdkdto.TaskResParam) (*sdkdto.WorkResponse, error)
}

// WorkInfoSaver 作品完整信息保存接口
type WorkInfoSaver interface {
	SaveWorkInfo(ctx context.Context, task *entity.Task, workResp *sdkdto.WorkResponse) (int64, error)
}

// ResourceSaver 资源保存接口
type ResourceSaver interface {
	Save(ctx context.Context, resource *entity.Resource) (int64, error)
}

// WorkChecker 作品查重接口
type WorkChecker interface {
	GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*entity.Work, error)
}

// ResourceReader 资源查询接口（查找已有作品的资源文件）
type ResourceReader interface {
	ListByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error)
}

// ResourceFileBackuper 资源文件备份接口
type ResourceFileBackuper interface {
	BackupFile(ctx context.Context, sourceType int, sourceId int64, fileName string, sourcePath string, workDir string) error
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
	// 资源文件备份
	resourceBackuper ResourceFileBackuper
	// 进度推送器
	pusher TaskProgressPusher
	// 跳过重复检查（替换确认后的第二次 run）
	skipDuplicateCheck bool
	// 已有作品 ID（第一次 run 检测到重复时设置，第二次 run 时用于备份）
	existingWorkId int64

	// 任务信息
	task   *entity.Task
	workId int64

	// 资源响应（插件返回）
	resourceResp *sdkdto.WorkResponse

	// 回调函数
	onStateChange func(taskId int64, oldState, newState TaskState)
	onProgress    func(taskId int64, total int64, finished int64)
}

// NewManagedTask 创建托管任务
func NewManagedTask(taskId, parentId int64, task *entity.Task, pluginExec TaskExecutor, workInfoSaver WorkInfoSaver, resourceSaver ResourceSaver, workDirProvider WorkDirProvider, fileNameFormatProvider FileNameFormatProvider, workChecker WorkChecker, resourceReader ResourceReader, resourceBackuper ResourceFileBackuper, pusher TaskProgressPusher) *ManagedTask {
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
		resourceBackuper:       resourceBackuper,
		pusher:                 pusher,
		task:                   task,
		workId:                 taskId,
	}
}

// run 核心执行逻辑
func (m *ManagedTask) run() runResult {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d panic: %v", m.taskId, r)
			m.setState(TaskStateFailed)
		}
	}()

	m.setState(TaskStateProcessing)

	// 0. 检查 workdir 是否已配置
	if m.workDirProvider.GetWorkDir() == "" {
		logger.Log.Errorf("[TaskManager] 任务 %d 失败: 未配置资源库目录", m.taskId)
		if m.pusher != nil {
			m.pusher.PushError(m.taskId, "未配置资源库目录，请先在设置中指定资源库保存位置")
		}
		m.setState(TaskStateFailed)
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
		m.backupExistingResources(m.existingWorkId)
		m.existingWorkId = 0
	}

	// 1. 调用 CreateWorkInfo 创建作品信息
	workResp, err := m.pluginExec.CreateWorkInfo(m.ctx, m.task)
	if err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d CreateWorkInfo 失败: %v", m.taskId, err)
		m.setState(TaskStateFailed)
		return runResultDone
	}

	// 2. 保存作品完整信息（Work + 周边数据）
	workId, err := m.workInfoSaver.SaveWorkInfo(m.ctx, m.task, workResp)
	if err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d 保存作品信息失败: %v", m.taskId, err)
		m.setState(TaskStateFailed)
		return runResultDone
	}
	m.workId = workId

	// 3. 获取资源读取器
	reader, startResp, err := m.pluginExec.Start(m.ctx, m.task, m.workId)
	if err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d Start 失败: %v", m.taskId, err)
		m.setState(TaskStateFailed)
		return runResultDone
	}
	defer reader.Close()

	// 4. 生成文件保存路径（将 CreateWorkInfo 的作品信息合并到 startResp）
	startResp.Work = workResp.Work
	startResp.SiteAuthors = workResp.SiteAuthors
	startResp.LocalAuthors = workResp.LocalAuthors
	localPath, relativePath, fileName := m.resolveLocalPath(startResp)

	// 4.1 确保资源目录存在
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d 创建资源目录失败: %v", m.taskId, err)
		m.setState(TaskStateFailed)
		return runResultDone
	}

	// 5. 保存 Resource 到数据库
	resource := &entity.Resource{
		BaseEntity:        &model.BaseEntity{},
		WorkID:            workId,
		TaskID:            m.task.GetID(),
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
		m.setState(TaskStateFailed)
		return runResultDone
	}

	// 6. 更新任务的 pendingResourceId
	m.task.PendingResourceID = sql.NullInt64{Int64: resourceId, Valid: true}

	file, err := os.Create(localPath)
	if err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d 创建文件失败 [%s]: %v", m.taskId, localPath, err)
		m.setState(TaskStateFailed)
		return runResultDone
	}
	defer file.Close()
	// 使用带缓冲的 io.Copy 以支持进度报告
	buf := make([]byte, 32*1024) // 32KB buffer
	var totalWritten int64

	for {
		select {
		case <-m.ctx.Done():
			// Context 取消，停止下载
			reader.Close()
			m.setState(TaskStateFailed)
			return runResultDone
		default:
			n, readErr := reader.Read(buf)
			if n > 0 {
				written, writeErr := file.Write(buf[:n])
				if written > 0 {
					totalWritten += int64(written)
				}
				if writeErr != nil {
					logger.Log.Errorf("[TaskManager] 任务 %d 写入文件失败: %v", m.taskId, writeErr)
					reader.Close()
					m.setState(TaskStateFailed)
					return runResultDone
				}
				// 报告进度
				if m.onProgress != nil {
					m.onProgress(m.taskId, startResp.Resource.Size, totalWritten)
				}
			}
			if readErr != nil {
				if readErr == io.EOF {
					reader.Close()
					file.Sync()
					// 校验下载完整性
					if startResp.Resource.Size > 0 && totalWritten < startResp.Resource.Size {
						logger.Log.Errorf("[TaskManager] 任务 %d 下载不完整: 已下载 %d / 预期 %d", m.taskId, totalWritten, startResp.Resource.Size)
						m.setState(TaskStateFailed)
						return runResultDone
					}
					m.resourceResp = startResp
					m.setState(TaskStateFinished)
					return runResultDone
				}
				logger.Log.Errorf("[TaskManager] 任务 %d 下载读取失败: %v", m.taskId, readErr)
				reader.Close()
				m.setState(TaskStateFailed)
				return runResultDone
			}
		}
	}
}

// Pause 暂停任务
func (m *ManagedTask) Pause() error {
	if m.state.Load() != int32(TaskStateProcessing) {
		return ErrTaskNotProcessing
	}
	m.setState(TaskStatePausing)

	// 调用插件 Pause
	param := &sdkdto.TaskResParam{
		Task:       dto.NewTaskDTO(m.task),
		ResourceID: m.resourceResp.Resource.ResourceID,
	}
	err := m.pluginExec.Pause(m.ctx, param)
	if err != nil {
		m.setState(TaskStateProcessing) // 暂停失败，恢复处理中
		return err
	}

	m.setState(TaskStatePaused)
	return nil
}

// Resume 恢复任务
func (m *ManagedTask) Resume() error {
	if m.state.Load() != int32(TaskStatePaused) {
		return ErrTaskNotPaused
	}

	// 调用插件 Resume
	param := &sdkdto.TaskResParam{
		Task:       dto.NewTaskDTO(m.task),
		ResourceID: m.resourceResp.Resource.ResourceID,
	}
	resp, err := m.pluginExec.Resume(m.ctx, param)
	if err != nil {
		return err
	}

	m.resourceResp = resp
	m.setState(TaskStateProcessing)
	return nil
}

// Stop 停止任务
func (m *ManagedTask) Stop() {
	m.setState(TaskStateStopping)
	m.cancel() // 触发 context 取消

	// 调用插件 Stop
	param := &sdkdto.TaskResParam{
		Task:       dto.NewTaskDTO(m.task),
		ResourceID: m.resourceResp.Resource.ResourceID,
	}
	if err := m.pluginExec.Stop(m.ctx, param); err != nil {
		logger.Log.Errorf("[TaskManager] 任务 %d Stop 失败: %v", m.taskId, err)
	}

	m.setState(TaskStateFailed)
}

// setState 设置任务状态
func (m *ManagedTask) setState(state TaskState) {
	old := TaskState(m.state.Swap(int32(state)))
	if old != state && m.onStateChange != nil {
		m.onStateChange(m.taskId, old, state)
	}
	if state == TaskStateFinished || state == TaskStateFailed {
		m.doneOnce.Do(func() { close(m.done) })
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
func (m *ManagedTask) SetOnStateChange(fn func(taskId int64, oldState, newState TaskState)) {
	m.onStateChange = fn
}

// SetOnProgress 设置进度回调
func (m *ManagedTask) SetOnProgress(fn func(taskId int64, total int64, finished int64)) {
	m.onProgress = fn
}

// backupExistingResources 备份已有作品的资源文件
func (m *ManagedTask) backupExistingResources(workId int64) {
	if m.resourceReader == nil || m.resourceBackuper == nil {
		return
	}
	resources, err := m.resourceReader.ListByWorkId(m.ctx, workId)
	if err != nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 查询已有作品资源失败（跳过备份）: %v", m.taskId, err)
		return
	}
	workDir := m.workDirProvider.GetWorkDir()
	for _, res := range resources {
		if !res.FilePath.Valid || !res.FileName.Valid {
			continue
		}
		absPath := filepath.Join(workDir, "resource", res.FilePath.String)
		if err := m.resourceBackuper.BackupFile(m.ctx, backup.SourceTypeResource, res.GetID(), res.FileName.String, absPath, workDir); err != nil {
			logger.Log.Warnf("[TaskManager] 任务 %d 备份资源文件失败 [%s]: %v", m.taskId, absPath, err)
		}
	}
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
	taskId   int64
	taskName string
	state    atomic.Int32
	children map[int64]*ManagedTask
	mu       sync.RWMutex
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
// 返回旧状态和新状态，供调用方判断是否需要持久化
func (p *ParentTask) RefreshState() (oldState, newState TaskState) {
	children := p.GetChildren()
	if len(children) == 0 {
		return p.GetState(), p.GetState()
	}

	var finishedCount, failedCount int
	var anyProcessing, anyWaiting, anyPaused bool
	for _, child := range children {
		switch child.GetState() {
		case TaskStateFinished:
			finishedCount++
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

	total := len(children)

	switch {
	case anyProcessing:
		newState = TaskStateProcessing
	case anyWaiting:
		newState = TaskStateWaiting
	case anyPaused:
		newState = TaskStatePaused
	case finishedCount == total:
		newState = TaskStateFinished
	case failedCount == total:
		newState = TaskStateFailed
	case finishedCount > 0 && finishedCount < total:
		newState = TaskStatePartlyFinished
	default:
		newState = TaskStateCreated
	}

	oldState = TaskState(p.state.Swap(int32(newState)))
	return
}

// GetState 获取父任务状态
func (p *ParentTask) GetState() TaskState {
	return TaskState(p.state.Load())
}

// AllChildrenTerminal 检查所有子任务是否都已进入终态
func (p *ParentTask) AllChildrenTerminal() bool {
	for _, child := range p.GetChildren() {
		s := child.GetState()
		if s != TaskStateFinished && s != TaskStateFailed && s != TaskStateWaitingForInput {
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
