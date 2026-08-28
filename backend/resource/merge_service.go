package resource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/shareLock"

	"gorm.io/gorm"
)

// 合并业务错误定义
var (
	// ErrMergeUnavailable 合并功能不可用（系统未安装 ffmpeg）。
	ErrMergeUnavailable = errors.New("合并功能不可用：系统未安装 ffmpeg")
	// ErrVideoTrackNotFound 该资源缺少视频轨。
	ErrVideoTrackNotFound = errors.New("该资源缺少视频轨，无法合并")
	// ErrAudioTrackNotFound 该资源缺少音频轨。
	ErrAudioTrackNotFound = errors.New("该资源缺少音频轨，无法合并")
	// ErrAlreadyMerged 该资源已存在合并产物（幂等守卫命中，不重复合并）。
	ErrAlreadyMerged = errors.New("该资源已存在合并产物")
	// ErrMergeInProgress 该资源已有进行中的合并（in-flight 守卫，防异步化后并发叠加）。
	ErrMergeInProgress = errors.New("该资源正在合并中")
)

// mergeTempFilePrefix 合并产物临时文件前缀：MergeResource 在 os.TempDir() 下以此前缀创建临时产物，
// 启动清理据此识别并删除崩溃残留（进程在 os.Remove 前退出时的未清理文件）。
const mergeTempFilePrefix = "ls-merge-"

// Merger 文件合并能力（由 merge.FFmpegMuxer 实现）。
// 输入输出均为文件绝对路径，不感知 store/resource；onProgress 上报合并百分比(0~100)，nil 不上报。
type Merger interface {
	MergeRemux(ctx context.Context, videoPath, audioPath, outPath string, onProgress func(percent int)) error
}

// EventEmitter Wails 事件发射能力（仅在 resource 包内本地定义，避免 resource→taskManager 包耦合；
// 阶段1 合并不进 taskManager 控制面，故独立 merge-events topic）。
type EventEmitter interface {
	Emit(eventName string, data ...any) bool
}

// MergeEventEmitter 合并进度/完成事件推送器。仅做无状态 emit，goroutine 安全（onProgress 由 ffmpeg
// 内部 copy goroutine 触发）。
type MergeEventEmitter interface {
	PushProgress(resourceId int64, percent int)
	PushComplete(resourceId int64, success bool, mergedStoreId int64, errMsg string)
}

// mergeJob 一次进行中的合并：持有脱离 IPC handler ctx 的独立 ctx，供 CancelMerge 主动中断。
type mergeJob struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// StoreOps store 落盘/查询/删除/路径原语（由 persistentStore.Service 实现）。
type StoreOps interface {
	GetById(ctx context.Context, id int64) (*domain.PersistentStore, error)
	GetAbsPath(store *domain.PersistentStore) string
	StoreFromFile(ctx context.Context, relPath, fileName, srcAbsPath string) (int64, error)
	HardDelete(ctx context.Context, id int64, backup bool) (int64, error)
	// DeleteWithBackup 删除 store 文件（移入 backup 建保管清单行并写行内 backup_id，记录随软删）——
	// overwrite 删原轨道用：轨道行入回收站文件条目，可经复原置换回滚
	DeleteWithBackup(ctx context.Context, id int64) (int64, error)
	BuildVariantPath(sourceRelPath, suffix string) string
}

// MergeSettingsReader 读合并策略（由 settings.Service 实现）。
type MergeSettingsReader interface {
	GetMergeStrategy() string
}

// MergeWorkLockChecker 作品锁查询（由 shareLock.ShareLockRegistry 实现）。overwrite 策略合并
// 收尾会软删原轨道（移走作品的活行 store 文件），作品正被分享拉取持有时在途拉取会读到
// 源文件消失，须在置换前拒绝
type MergeWorkLockChecker interface {
	IsLocked(ctx context.Context, workID int64) bool
}

// ResourceRecomputer 资源完整度重算（由 resource.Service 实现）。重算按活行 store 角色计数——
// 关联保留形态下轨道软删行关联不计入
type ResourceRecomputer interface {
	RecomputeResourceComplete(ctx context.Context, resourceId int64)
}

// Transactor 事务执行器（事务 DB 通过 ctx 传递，repository 经 dbFromCtx 感知）。
type Transactor interface {
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// ResourceAccessor 资源读+更新(合并后重算完整度用;接口隔离,避免 MergeService 持有具体 repo)。
// 实现由 ResourceRepository 满足(BaseRepository 提供 GetById/Update)。
type ResourceAccessor interface {
	GetById(ctx context.Context, id int64) (*domain.Resource, error)
	Updates(ctx context.Context, resource *domain.Resource) error
}

// MergeService 音视频合并业务编排：取 resource 的 videoTrack/audioTrack → 调合并 →
// 落产物 PersistentStore(merged) → 挂 resource_store。合并能力由 Merger 提供，
// store 落盘/路径由 StoreOps 提供，本服务只做编排。合并异步执行（不阻塞 IPC），进度与结果经 emitter 推送。
type MergeService struct {
	resourceStoreRepo *ResourceStoreRepository
	resource          ResourceAccessor
	merger            Merger
	storeOps          StoreOps
	settings          MergeSettingsReader
	tx                Transactor
	completer         ResourceRecomputer
	emitter           MergeEventEmitter
	workLock          MergeWorkLockChecker // overwrite 原轨道置换前置作品锁守卫
	jobsMu            sync.Mutex
	jobs              map[int64]*mergeJob // resourceId → 进行中合并（in-flight 守卫 + cancel 锚点）
}

// NewMergeService 创建合并业务服务。merger 为 nil 时合并功能不可用（调用返回 ErrMergeUnavailable）。
// emitter 用于异步合并的进度/完成推送（阶段1 独立 merge-events topic，不进 taskManager）。
// completer 为资源完整度重算（幂等命中与合并完成两路径共用）。
// workLock 为 overwrite 原轨道置换前置作品锁守卫（shareLock.ShareLockRegistry 实现）。
func NewMergeService(resourceStoreRepo *ResourceStoreRepository, resource ResourceAccessor, merger Merger, storeOps StoreOps, settings MergeSettingsReader, tx Transactor, completer ResourceRecomputer, emitter MergeEventEmitter, workLock MergeWorkLockChecker) *MergeService {
	return &MergeService{
		resourceStoreRepo: resourceStoreRepo,
		resource:          resource,
		merger:            merger,
		storeOps:          storeOps,
		settings:          settings,
		tx:                tx,
		completer:         completer,
		emitter:           emitter,
		workLock:          workLock,
		jobs:              make(map[int64]*mergeJob),
	}
}

// MergeResource 启动指定 Resource 的音视频合并（用户主动触发，异步执行）。
// 同步做前置校验（ffmpeg 可用 / 已存在 merged 产物幂等 / 缺轨 fail-fast / in-flight 守卫），
// 通过则注册 in-flight job 并在独立 goroutine 跑 runMerge，立即返回 nil（合并结果经 emitter 推送）。
// 不阻塞 IPC：handler 返回后合并仍在 detached ctx 上跑。
func (s *MergeService) MergeResource(ctx context.Context, resourceId int64) error {
	if s.merger == nil {
		return ErrMergeUnavailable
	}

	// 幂等守卫：已存在 merged 产物则不重复合并（避免 keep 模式累积孤儿 resource_store(merged) 关联）。
	// 历史已累积的多行 merged 不在此清理（属数据修复范畴）。GetByType 仅命中活行——轨道软删入回收站后
	// 复原置换回滚的形态下重合并不被误挡
	if existing, err := s.resourceStoreRepo.GetByType(ctx, resourceId, domain.StoreTypeVideoMain); err == nil {
		s.completer.RecomputeResourceComplete(ctx, resourceId) // 幂等命中前重算(修复历史 complete 值)
		_ = existing
		return ErrAlreadyMerged
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("查询已合并产物失败: %w", err)
	}

	// 取 videoTrack/audioTrack store（缺轨 fail-fast，返回明确中文错误）
	videoRS, err := s.resourceStoreRepo.GetByType(ctx, resourceId, domain.StoreTypeVideoTrack)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrVideoTrackNotFound
		}
		return fmt.Errorf("查询视频轨失败: %w", err)
	}
	audioRS, err := s.resourceStoreRepo.GetByType(ctx, resourceId, domain.StoreTypeAudioTrack)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAudioTrackNotFound
		}
		return fmt.Errorf("查询音频轨失败: %w", err)
	}

	// 注册 in-flight job：detached ctx（脱离 handler ctx，handler 返回后合并仍跑）+ 加锁双检防并发抢占。
	mergeCtx, cancel := context.WithCancel(context.Background())
	s.jobsMu.Lock()
	if _, running := s.jobs[resourceId]; running {
		s.jobsMu.Unlock()
		cancel()
		return ErrMergeInProgress
	}
	s.jobs[resourceId] = &mergeJob{ctx: mergeCtx, cancel: cancel}
	s.jobsMu.Unlock()

	go s.runMerge(mergeCtx, resourceId, videoRS, audioRS)
	return nil
}

// runMerge 在独立 goroutine 中执行合并全流程（ffmpeg → 落盘 → 挂 store → overwrite → 重算完整度）。
// 进度经 emitter.PushProgress 推送，终态经 emitter.PushComplete 推送；任何退出路径都从 jobs 删除（defer）。
// 取消（CancelMerge 触发 ctx 取消）仅在 MergeRemux 阶段生效：ffmpeg 运行中取消则中止合并、报"已取消"；
// ffmpeg 既已完成则改用独立 commitCtx 落盘（不随取消中断），避免 cancel 与 ffmpeg 完成的竞态误报"落盘失败"。落盘/overwrite 仅成功路径执行，故取消不会误删原轨。
func (s *MergeService) runMerge(ctx context.Context, resourceId int64, videoRS, audioRS *domain.ResourceStore) {
	defer s.removeJob(resourceId)

	// 取源文件绝对路径
	videoPS, err := s.storeOps.GetById(ctx, videoRS.StoreID)
	if err != nil {
		s.emitter.PushComplete(resourceId, false, 0, fmt.Sprintf("加载视频轨 store 失败: %v", err))
		return
	}
	audioPS, err := s.storeOps.GetById(ctx, audioRS.StoreID)
	if err != nil {
		s.emitter.PushComplete(resourceId, false, 0, fmt.Sprintf("加载音频轨 store 失败: %v", err))
		return
	}
	videoAbs := s.storeOps.GetAbsPath(videoPS)
	audioAbs := s.storeOps.GetAbsPath(audioPS)

	// ffmpeg 输出到临时文件（唯一名，跟随源视频容器扩展）
	videoExt := filepath.Ext(videoPS.FilePath.String)
	if videoExt == "" {
		videoExt = ".mp4"
	}
	tmpOut := filepath.Join(os.TempDir(), fmt.Sprintf(mergeTempFilePrefix+"%d-%d%s", resourceId, time.Now().UnixNano(), videoExt))

	// 合并（muxer 自处理超时/失败/产物残留）；onProgress 把百分比推给前端
	if err := s.merger.MergeRemux(ctx, videoAbs, audioAbs, tmpOut, func(percent int) {
		s.emitter.PushProgress(resourceId, percent)
	}); err != nil {
		msg := err.Error()
		if ctx.Err() != nil {
			msg = "已取消"
		}
		s.emitter.PushComplete(resourceId, false, 0, msg)
		return
	}

	// ffmpeg 已成功产出 tmpOut，后续落盘/挂 store/overwrite 用独立 ctx（不随 cancel 中断）：
	// 用户取消的意图是停 ffmpeg；ffmpeg 既已完成，落盘应正常完成（避免浪费已完成的合并 + 留下半成品落盘）。
	// 否则 cancel 与 ffmpeg 完成的竞态会让 StoreFromFile 撞上已取消的 ctx 报"落盘失败: context canceled"。
	commitCtx := context.Background()

	// 产物相对路径（源视频轨旁、文件名加 _merged）与展示文件名
	mergedRelPath := s.storeOps.BuildVariantPath(videoPS.FilePath.String, "_merged")
	mergedFileName := buildMergedFileName(videoPS.FileName.String, videoExt)

	// 落盘 + 建 PersistentStore；临时文件已复制为产物，随之清除
	mergedPsId, err := s.storeOps.StoreFromFile(commitCtx, mergedRelPath, mergedFileName, tmpOut)
	_ = os.Remove(tmpOut)
	if err != nil {
		s.emitter.PushComplete(resourceId, false, 0, fmt.Sprintf("落盘合并产物失败: %v", err))
		return
	}

	// 事务内挂 resource_store(merged)；失败补偿删产物 store
	if err := s.tx.ExecInTransaction(commitCtx, func(txCtx context.Context) error {
		rs := domain.NewResourceStore()
		rs.ResourceID = resourceId
		rs.StoreType = domain.StoreTypeVideoMain
		// 此处 videoMain 为合并产物(一次性派生,非流式下载),语义 derived(不可续传);本地导入路径的 videoMain 则为 downloaded。
		rs.Generation = domain.GenerationDerived
		rs.StoreID = mergedPsId
		return s.resourceStoreRepo.Create(txCtx, rs)
	}); err != nil {
		if _, derr := s.storeOps.HardDelete(commitCtx, mergedPsId, false); derr != nil {
			logger.Log.Errorf("[MergeService] 挂载合并产物失败且补偿删除产物 store 也失败: resourceId=%d 挂载错误=%v 删除错误=%v", resourceId, err, derr)
		}
		s.emitter.PushComplete(resourceId, false, 0, fmt.Sprintf("挂载合并产物失败: %v", err))
		return
	}

	// overwrite：原轨道软删（置换作品的活行 store 文件，细节见 overwriteOriginalTracks）
	if s.settings.GetMergeStrategy() == settings.MergeStrategyOverwrite {
		if err := s.overwriteOriginalTracks(commitCtx, resourceId, videoPS, audioPS); err != nil {
			s.emitter.PushComplete(resourceId, false, 0, err.Error())
			return
		}
	}

	s.completer.RecomputeResourceComplete(commitCtx, resourceId) // 合并改变 store 构成,重算(分离流下载时缺 videoMain 判 2,合并后应升为 1)
	s.emitter.PushComplete(resourceId, true, mergedPsId, "")
}

// overwriteOriginalTracks overwrite 策略收尾：软删原视频/音频轨——文件移 backup 建保管清单行，
// 行随软删入回收站文件条目（可经复原置换回滚）；轨道 resource_store 关联保留，软删行经挂载链
// 可联作品、随作品级联净化。置换前置作品锁守卫：作品正被分享拉取持有时移走原轨道文件会令
// 在途拉取读到源文件消失，命中返回 shareLock.ErrWorkLocked（合并产物已挂载保留、原轨道不动，
// 用户知情强制解锁后重试本操作）。锁为防误触软防护：资源反查异常时告警放行，不因守卫自身
// 故障阻断合并
func (s *MergeService) overwriteOriginalTracks(ctx context.Context, resourceId int64, videoPS, audioPS *domain.PersistentStore) error {
	if res, err := s.resource.GetById(ctx, resourceId); err == nil && res != nil {
		if s.workLock.IsLocked(ctx, res.WorkID) {
			logger.Log.Infof("[MergeService] 原轨道置换被作品锁拒绝: 资源 %d 所属作品 %d 正被分享拉取持有", resourceId, res.WorkID)
			return shareLock.ErrWorkLocked
		}
	} else {
		logger.Log.Warnf("[MergeService] 作品锁守卫反查资源 %d 失败（放行继续）: %v", resourceId, err)
	}
	if _, err := s.storeOps.DeleteWithBackup(ctx, videoPS.GetID()); err != nil {
		return fmt.Errorf("软删原视频轨失败: %w", err)
	}
	if _, err := s.storeOps.DeleteWithBackup(ctx, audioPS.GetID()); err != nil {
		return fmt.Errorf("软删原音频轨失败: %w", err)
	}
	return nil
}

// removeJob 从 in-flight 注册表删除指定 resource 的合并（runMerge 退出时调用）。
func (s *MergeService) removeJob(resourceId int64) {
	s.jobsMu.Lock()
	delete(s.jobs, resourceId)
	s.jobsMu.Unlock()
}

// CancelMerge 取消指定 resource 的进行中合并（无进行中合并则 no-op）。
func (s *MergeService) CancelMerge(resourceId int64) {
	s.jobsMu.Lock()
	if job, ok := s.jobs[resourceId]; ok {
		job.cancel()
	}
	s.jobsMu.Unlock()
}

// buildMergedFileName 由源文件名构造合并产物展示名（源名去扩展 + _merged + 扩展）。
func buildMergedFileName(srcFileName, ext string) string {
	base := strings.TrimSuffix(srcFileName, filepath.Ext(srcFileName))
	if base == "" {
		base = "merged"
	}
	return base + "_merged" + ext
}

// CleanupResidualTempFiles 清理合并产物临时文件残留（os.TempDir() 下 mergeTempFilePrefix 前缀文件）。
// 残留场景：MergeResource 创建临时产物后、os.Remove 之前进程崩溃（ffmpeg 执行中或落盘期间被杀）。
// 应用启动时调用，幂等——无残留时无副作用；单个删除失败不中断整体清理（残留可能被占用，下次启动再清）。
func (s *MergeService) CleanupResidualTempFiles(ctx context.Context) error {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return fmt.Errorf("读取临时目录失败: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), mergeTempFilePrefix) {
			continue
		}
		_ = os.Remove(filepath.Join(os.TempDir(), entry.Name()))
	}
	return nil
}

// wailsMergeEmitter 基于 Wails Events 的合并事件推送器，推 merge-events topic。
// 复用 Wails 事件管道与 ipcMergeEvent{type,data} 信封模式（与 taskManager 推送器同范式），
// 独立 topic、不进 taskManager 控制面。仅做无状态 emit，goroutine 安全。
type wailsMergeEmitter struct {
	emitterFn func() EventEmitter // 延迟读取：构造期 Wails emitter 可能尚未注入，emit 时再取
}

// NewWailsMergeEmitter 用"延迟返回 Wails 事件发射器"的闭包构造合并事件推送器。
// 闭包模式：合并服务在应用初始化早期构造（此时 Wails emitter 尚未经 SetEventEmitter 注入），
// 而进度事件只在用户触发合并时才 emit（彼时 emitter 已就绪），故 emit 时再读取，避免持有未就绪引用。
func NewWailsMergeEmitter(emitterFn func() EventEmitter) MergeEventEmitter {
	return &wailsMergeEmitter{emitterFn: emitterFn}
}

// emit 向 merge-events topic 推送带类型信封的事件；emitter 未就绪时静默跳过（无合并能在该阶段发生）。
func (e *wailsMergeEmitter) emit(eventType string, data any) {
	if em := e.emitterFn(); em != nil {
		em.Emit("merge-events", &ipcMergeEvent{Type: eventType, Data: data})
	}
}

func (e *wailsMergeEmitter) PushProgress(resourceId int64, percent int) {
	e.emit("progress", &mergeProgressData{ResourceId: resourceId, Percent: percent})
}

func (e *wailsMergeEmitter) PushComplete(resourceId int64, success bool, mergedStoreId int64, errMsg string) {
	e.emit("complete", &mergeCompleteData{ResourceId: resourceId, Success: success, MergedStoreId: mergedStoreId, ErrMsg: errMsg})
}

// ipcMergeEvent merge-events topic 的信封（type 区分 progress/complete），与 taskManager ipcEvent 同范式。
type ipcMergeEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// mergeProgressData 合并进度事件载荷（percent∈[0,100]）。
type mergeProgressData struct {
	ResourceId int64 `json:"resourceId"`
	Percent    int   `json:"percent"`
}

// mergeCompleteData 合并完成事件载荷（success=true 时 MergedStoreId 有效）。
type mergeCompleteData struct {
	ResourceId    int64  `json:"resourceId"`
	Success       bool   `json:"success"`
	MergedStoreId int64  `json:"mergedStoreId"`
	ErrMsg        string `json:"errMsg"`
}
