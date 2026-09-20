package authorInfo

// 作者个人信息编排：site 侧拉取主链（能力广播定位插件 → RPC 流式首块 meta 回写元数据（插件
// 权威域白名单列）→ 判定需落字节（来源 URL 变化或行无引用）→ 换头像先删旧 → 头像字节写暂存
// （尺寸上限）→ 四调用入库 → 业务事务内建行 + 同事务写 site_author.avatar_store_id → 作用域
// 回收。两触发面（作品入库后自动带开关、手动单条/批量）共用同一拉取序列与在途去重集合）；
// local 侧头像手动导入/移除；siteAuthor/localAuthor 删除联动的头像行清理（AvatarFileCleaner，
// 行面入调用方删除事务、文件面在事务提交后清理）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/staging"
	"github.com/library-squirrel/backend/storeRegistry"
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"

	pkgerr "github.com/library-squirrel/backend/error"
	"gorm.io/gorm"
)

// avatarFormatWhitelist 头像格式声明白名单（扩展段直接取自插件声明，契约输入不信任——
// 白名单外取值中止头像落盘记日志）
var avatarFormatWhitelist = map[string]bool{
	"jpg": true, "jpeg": true, "png": true, "gif": true, "webp": true, "bmp": true,
}

const (
	// maxAvatarBytes 头像字节尺寸上限（超限中止，头像无合法超 10MB 形态）
	maxAvatarBytes = 10 * 1024 * 1024
	// authorFetchTimeout 单作者拉取超时（网络调用：广播定位 + 元数据 + 头像字节整程）
	authorFetchTimeout = 60 * time.Second
	// siteAvatarScopeContentShape site 侧拉取暂存作用域的内容形态标签（作用域内单头像文件平铺）
	siteAvatarScopeContentShape = "site-author-avatar"
	// localAvatarScopeContentShape local 侧导入暂存作用域的内容形态标签（作用域内单头像文件平铺）
	localAvatarScopeContentShape = "local-author-avatar"
	// localAvatarScopeKeyPrefix local 侧导入暂存作用域键前缀：siteAuthor/local_author 两表 DB id
	// 独立编号，裸数字键会同值跨表撞作用域目录，前缀消歧
	localAvatarScopeKeyPrefix = "local-"
)

// Service 作者个人信息编排服务（site 侧拉取主链 + local 侧头像导入/移除 + 删除联动头像清理）
type Service struct {
	siteAuthors  SiteAuthorStore
	localAuthors LocalAuthorStore
	ingestor     StoreIngestor
	storeOps     AvatarStoreOps
	fetchCfg     AuthorFetchSettings
	workDir      WorkDirProvider
	transactor   Transactor

	// fetcher 站点作者信息拉取能力桥（plugin 模块实现）。plugin 模块在本服务之后初始化
	// （能力桥依赖插件加载器就绪），经 SetSiteAuthorFetcher 延迟注入；nil 时两触发面均跳过
	fetcher SiteAuthorFetcher

	// 在途作者拉取去重集合（两触发面共用）
	mu       sync.Mutex
	inFlight map[int64]struct{}
}

// NewService 创建作者信息编排服务
func NewService(
	siteAuthors SiteAuthorStore,
	localAuthors LocalAuthorStore,
	ingestor StoreIngestor,
	storeOps AvatarStoreOps,
	fetchCfg AuthorFetchSettings,
	workDir WorkDirProvider,
	transactor Transactor,
) *Service {
	return &Service{
		siteAuthors:  siteAuthors,
		localAuthors: localAuthors,
		ingestor:     ingestor,
		storeOps:     storeOps,
		fetchCfg:     fetchCfg,
		workDir:      workDir,
		transactor:   transactor,
		inFlight:     make(map[int64]struct{}),
	}
}

// SetSiteAuthorFetcher 注入站点作者信息拉取能力桥（plugin 模块在插件装配段接线）
func (s *Service) SetSiteAuthorFetcher(f SiteAuthorFetcher) {
	s.fetcher = f
}

// ===== 触发面一：作品入库后自动（带开关，work 经 SiteAuthorRefreshScheduler 窄接口调用） =====

// OnSiteAuthorsUpserted 站点作者已 upsert 完成通知：开关（authorSettings.autoFetchInfo）只控制
// 本触发面。契约：非阻塞立即返回——逐作者串行拉取在内部 goroutine 进行，在途作者静默跳过，
// 单次触发单次尝试不重试，失败仅记日志。入参只传作者 DB ID 集合（含跨站引用作者，站点键
// 由实现侧按行 JOIN site 反查）
func (s *Service) OnSiteAuthorsUpserted(siteAuthorIds []int64) {
	if s.fetcher == nil || len(siteAuthorIds) == 0 || !s.fetchCfg.AuthorAutoFetchInfoEnabled() {
		return
	}
	go func() {
		ctx := context.Background()
		targets, err := s.siteAuthors.ListFetchTargetsByIds(ctx, siteAuthorIds)
		if err != nil {
			logger.Log.Warnf("[authorInfo] 作品入库后自动拉取：反查作者目标行失败: %v", err)
			return
		}
		for _, target := range targets {
			if !s.acquire(target.ID) {
				continue
			}
			s.fetchTargetWithTimeout(ctx, target)
			s.release(target.ID)
		}
	}()
}

// ===== 触发面二：手动拉取（单条/批量，不受开关限制） =====

// FetchSiteAuthorInfoById 手动拉取单个站点作者信息：按行取站点键后广播路由。命中在途拉取
// 直接拒绝（用户可稍后重试）。失败上抛（前端行操作 loading 态由调用侧承载）
func (s *Service) FetchSiteAuthorInfoById(ctx context.Context, siteAuthorId int64) error {
	if s.fetcher == nil {
		return pkgerr.NewBusinessError(500, "作者信息拉取能力未就绪")
	}
	target, err := s.resolveTarget(ctx, siteAuthorId)
	if err != nil {
		return err
	}
	if !s.acquire(target.ID) {
		return pkgerr.NewBusinessError(409, "该作者的信息正在拉取中，请稍后再试")
	}
	defer s.release(target.ID)
	if err := s.fetchTargetWithTimeout(ctx, target); err != nil {
		logger.Log.Warnf("[authorInfo] 手动拉取站点作者 %d 信息失败: %v", siteAuthorId, err)
		return err
	}
	return nil
}

// SiteAuthorFetchItemResult 批量拉取的逐条结果（前端逐条反馈成功/失败与原因）
type SiteAuthorFetchItemResult struct {
	SiteAuthorId int64  `json:"siteAuthorId"`
	Success      bool   `json:"success"`
	Message      string `json:"message"`
}

// FetchSiteAuthorsInfoByIds 手动批量拉取：逐作者串行走同一拉取序列、共用在途去重；单作者
// 失败不阻断其余，返回与入参顺序一致的逐条结果清单
func (s *Service) FetchSiteAuthorsInfoByIds(ctx context.Context, siteAuthorIds []int64) ([]*SiteAuthorFetchItemResult, error) {
	if s.fetcher == nil {
		return nil, pkgerr.NewBusinessError(500, "作者信息拉取能力未就绪")
	}
	results := make([]*SiteAuthorFetchItemResult, 0, len(siteAuthorIds))
	if len(siteAuthorIds) == 0 {
		return results, nil
	}
	targets, err := s.siteAuthors.ListFetchTargetsByIds(ctx, siteAuthorIds)
	if err != nil {
		return nil, fmt.Errorf("反查作者目标行失败: %w", err)
	}
	targetById := make(map[int64]*dto.SiteAuthorFetchTarget, len(targets))
	for _, t := range targets {
		targetById[t.ID] = t
	}
	for _, id := range siteAuthorIds {
		result := &SiteAuthorFetchItemResult{SiteAuthorId: id}
		target, ok := targetById[id]
		switch {
		case !ok:
			result.Message = "站点作者不存在"
		case !s.acquire(id):
			result.Message = "正在拉取中，已跳过"
		default:
			if err := s.fetchTargetWithTimeout(ctx, target); err != nil {
				logger.Log.Warnf("[authorInfo] 批量拉取站点作者 %d 信息失败: %v", id, err)
				result.Message = err.Error()
			} else {
				result.Success = true
			}
			s.release(id)
		}
		results = append(results, result)
	}
	return results, nil
}

// fetchTargetWithTimeout 带整程超时的单作者拉取（在途去重由调用方持有）
func (s *Service) fetchTargetWithTimeout(ctx context.Context, target *dto.SiteAuthorFetchTarget) error {
	ctx, cancel := context.WithTimeout(ctx, authorFetchTimeout)
	defer cancel()
	return s.fetchSiteAuthor(ctx, target)
}

// resolveTarget 按单 ID 反查拉取目标行
func (s *Service) resolveTarget(ctx context.Context, siteAuthorId int64) (*dto.SiteAuthorFetchTarget, error) {
	targets, err := s.siteAuthors.ListFetchTargetsByIds(ctx, []int64{siteAuthorId})
	if err != nil {
		return nil, fmt.Errorf("反查作者目标行失败: %w", err)
	}
	if len(targets) == 0 {
		return nil, pkgerr.NewBusinessError(404, "站点作者不存在")
	}
	return targets[0], nil
}

// ===== 单作者拉取序列 =====

// fetchSiteAuthor 单作者拉取序列（调用方须已持有在途去重）：广播定位插件 → 流式首块 meta
// 回写元数据 → 资源轨道（按需：换头像删旧 → 暂存字节 → 四调用入库 → 同事务写引用列）。
// 失败上抛不留半成品——元数据回写与资源落库各自独立成功，头像缺省是合法态
func (s *Service) fetchSiteAuthor(ctx context.Context, target *dto.SiteAuthorFetchTarget) error {
	if err := settings.RefuseIfUnconfigured(s.workDir.GetWorkDir(), "authorInfo"); err != nil {
		return err
	}
	if target.SiteKey == "" {
		return fmt.Errorf("作者 %d 的站点行缺失 site_key，无法路由拉取", target.ID)
	}
	if target.SiteAuthorID == "" {
		return fmt.Errorf("作者 %d 缺少站点侧作者 id", target.ID)
	}
	session := &avatarFetchSession{svc: s, target: target, ctx: ctx}
	if err := s.fetcher.FetchSiteAuthorInfo(ctx, target.SiteKey, target.SiteAuthorID, session.onMeta, session.onData); err != nil {
		session.cleanupScope()
		return err
	}
	// 字节已在暂存（拉取阶段完成），提交点不再受拉取超时约束——超时竞态不应作废已下载字节
	return session.commitIngest(context.Background())
}

// avatarFetchSession 单作者一次拉取的资源轨道状态：meta 到达后按需建暂存作用域并逐块落
// 暂存文件（尺寸上限），流正常收尾后由 commitIngest 走四调用入库提交
type avatarFetchSession struct {
	svc    *Service
	target *dto.SiteAuthorFetchTarget
	// ctx 拉取流 ctx（onMeta 内的库写与作用域创建随流取消）；提交/回收阶段刻意不使用
	ctx context.Context

	file           *os.File // 暂存文件句柄（absPath 域，落盘调用点现场消费）
	written        int64    // 已写字节数（上限判定）
	bytesReceived  bool     // 是否收到过字节块（提交前判空——声明头像而零字节属插件协议违规）
	scopeKey       string   // 已建暂存作用域键（空=未建资源轨道）
	stagingFileRel string   // 暂存文件 relPath（workDir 相对、正斜杠）
	finalRelPath   string   // 最终落位 relPath（store/avatar/site/...）
	finalFileName  string   // 最终文件名
}

// onMeta 首块元数据消费：回写元数据 → 判定需落字节（站点声明头像且（来源 URL 变化或行无
// 引用））→ 建资源轨道（换头像先删旧、派生路径、建作用域）。返回 false 表示无需头像字节
func (sess *avatarFetchSession) onMeta(meta *pluginsdkdto.AuthorInfoMeta) (bool, error) {
	if err := sess.svc.writeBackMeta(sess.ctx, sess.target, meta); err != nil {
		return false, err
	}
	avatarUrl := meta.GetAvatarUrl()
	if avatarUrl == "" {
		return false, nil // 站点侧无头像，合法态（仅刷新元数据）
	}
	urlUnchanged := sess.target.AvatarSourceURL.Valid && sess.target.AvatarSourceURL.String == avatarUrl
	if urlUnchanged && sess.target.AvatarStoreID.Valid {
		return false, nil // 头像未变更且引用在，跳过资源落盘
	}
	format := strings.ToLower(strings.TrimSpace(meta.GetAvatarFormat()))
	if !avatarFormatWhitelist[format] {
		return false, fmt.Errorf("插件声明的头像格式 %q 不在白名单内，中止头像落盘", meta.GetAvatarFormat())
	}
	// 换头像：来源 URL 已变化 → 先删旧（崩溃窗口收敛为「无头像」，下次触发重拉）
	if sess.target.AvatarStoreID.Valid {
		siteAuthorId := sess.target.ID
		if err := sess.svc.deleteAvatarByReference(sess.ctx, sess.target.AvatarStoreID.Int64, func(txCtx context.Context) error {
			return sess.svc.siteAuthors.UpdateAvatarStoreId(txCtx, siteAuthorId, sql.NullInt64{})
		}); err != nil {
			return false, err
		}
	}
	finalRel, err := SiteAvatarRelPath(sess.target.SiteKey, sess.target.SiteAuthorID, format)
	if err != nil {
		return false, fmt.Errorf("派生头像落盘路径失败: %w", err)
	}
	scopeKey := strconv.FormatInt(sess.target.ID, 10)
	if _, err := staging.CreateScope(sess.ctx, sess.svc.workDir.GetWorkDir(),
		staging.OwnerAuthorInfo, scopeKey, siteAvatarScopeContentShape); err != nil {
		return false, fmt.Errorf("创建拉取暂存作用域失败: %w", err)
	}
	sess.scopeKey = scopeKey
	sess.finalRelPath = finalRel
	sess.finalFileName = path.Base(finalRel)
	sess.stagingFileRel = path.Join(staging.RootName, string(staging.OwnerAuthorInfo), scopeKey, sess.finalFileName)
	return true, nil
}

// onData 头像字节块落暂存（上限 10MB 超限中止）
func (sess *avatarFetchSession) onData(data []byte) error {
	if sess.scopeKey == "" {
		return errors.New("资源块先于资源轨道建立（onMeta 未声明需要头像）")
	}
	sess.bytesReceived = true
	if sess.file == nil {
		abs := filepath.Join(sess.svc.workDir.GetWorkDir(), sess.stagingFileRel)
		f, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("打开头像暂存文件失败: %w", err)
		}
		sess.file = f
	}
	if sess.written+int64(len(data)) > maxAvatarBytes {
		return fmt.Errorf("头像字节超过上限 %dMB，中止", maxAvatarBytes/(1024*1024))
	}
	n, err := sess.file.Write(data)
	if err != nil {
		return fmt.Errorf("写头像暂存文件失败: %w", err)
	}
	sess.written += int64(n)
	return nil
}

// commitIngest 流正常收尾后的提交点：暂存字节走四调用入库（撤回处置恒丢弃——头像可重拉
// 无续传价值），业务事务内建 store 行 + 同事务写 site_author.avatar_store_id（引用列与建行
// 同生共死，「行建而引用未写」的中间态不存在）。未建资源轨道的会话为纯元数据刷新，空操作
func (sess *avatarFetchSession) commitIngest(ctx context.Context) error {
	defer sess.cleanupScope()
	if sess.scopeKey == "" {
		return nil
	}
	if !sess.bytesReceived {
		// meta 声明头像（URL/格式非空）却零字节块：按协议违规收口，不落空文件
		return errors.New("插件声明头像但未发送任何字节块")
	}
	if err := sess.closeFile(); err != nil {
		return err
	}
	intentIds, err := sess.svc.ingestor.PrepareIngest(ctx, []persistentStore.IngestItem{{
		FilePath:    sess.finalRelPath,
		StagingPath: sess.stagingFileRel,
		AbortAction: entity.AbortActionDiscard,
	}})
	if err != nil {
		return fmt.Errorf("登记头像入库意图失败: %w", err)
	}
	if err := sess.svc.ingestor.PlaceIngest(ctx, intentIds); err != nil {
		sess.abortIngested(intentIds)
		return fmt.Errorf("头像落位失败: %w", err)
	}
	if err := sess.svc.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		storeId, cerr := sess.svc.ingestor.CommitIngest(txCtx, intentIds[0], sess.finalRelPath, sess.finalFileName,
			sql.NullString{}, sql.NullString{})
		if cerr != nil {
			return fmt.Errorf("建头像 store 行失败: %w", cerr)
		}
		return sess.svc.siteAuthors.UpdateAvatarStoreId(txCtx, sess.target.ID, sql.NullInt64{Int64: storeId, Valid: true})
	}); err != nil {
		sess.abortIngested(intentIds)
		return fmt.Errorf("提交头像入库失败: %w", err)
	}
	return nil
}

// abortIngested 提交失败撤回：按丢弃处置清走已落位文件并收口登记行（幂等）；撤回自身失败
// 仅记日志——登记行保留由启动恢复收口
func (sess *avatarFetchSession) abortIngested(intentIds []int64) {
	if err := sess.svc.ingestor.AbortIngest(context.Background(), intentIds); err != nil {
		logger.Log.Warnf("[authorInfo] 撤回头像入库失败（登记行保留，启动恢复收口）: %v", err)
	}
}

// cleanupScope 回收暂存作用域（正常收尾与失败路径统一收口；失败仅记日志留启动清扫兜底）
func (sess *avatarFetchSession) cleanupScope() {
	_ = sess.closeFile()
	if sess.scopeKey == "" {
		return
	}
	if err := staging.RemoveScope(context.Background(), sess.svc.workDir.GetWorkDir(),
		staging.OwnerAuthorInfo, sess.scopeKey); err != nil {
		logger.Log.Warnf("[authorInfo] 回收头像拉取暂存作用域失败（留待启动清扫）: %v", err)
	}
	sess.scopeKey = ""
}

// closeFile 关闭暂存文件句柄（幂等）
func (sess *avatarFetchSession) closeFile() error {
	if sess.file == nil {
		return nil
	}
	err := sess.file.Close()
	sess.file = nil
	if err != nil {
		return fmt.Errorf("关闭头像暂存文件失败: %w", err)
	}
	return nil
}

// writeBackMeta 拉回元数据回写 site_author：经 siteAuthor upsert 复用任务链重拉的同一套
// 覆盖语义（DoUpdates 白名单=author_name/fixed_author_name/site_author_name_before/introduce/
// homepage/avatar_source_url）；local_author_id/last_use/avatar_store_id 用户域/引用域按行保留。
// fixed_author_name/site_author_name_before 无拉取契约来源，与任务链不声明时同款落 NULL
func (s *Service) writeBackMeta(ctx context.Context, target *dto.SiteAuthorFetchTarget, meta *pluginsdkdto.AuthorInfoMeta) error {
	row := entity.NewSiteAuthor()
	row.SiteID = sql.NullInt64{Int64: target.SiteID, Valid: true}
	row.SiteAuthorID = sql.NullString{String: target.SiteAuthorID, Valid: true}
	row.AuthorName = sql.NullString{String: meta.GetAuthorName(), Valid: true}
	row.Introduce = sql.NullString{String: meta.GetIntroduce(), Valid: meta.GetIntroduce() != ""}
	row.Homepage = sql.NullString{String: meta.GetHomepage(), Valid: meta.GetHomepage() != ""}
	row.AvatarSourceURL = sql.NullString{String: meta.GetAvatarUrl(), Valid: meta.GetAvatarUrl() != ""}
	if err := s.siteAuthors.Upsert(ctx, row); err != nil {
		return fmt.Errorf("回写作者元数据失败: %w", err)
	}
	return nil
}

// deleteAvatarByReference 删除指定引用的头像（site 换头像删旧 / local 移除头像与换头像共用）：
// 事务内先清引用列（FK NO ACTION 要求释放引用后方可删 store 行）再物理删 store 行，事务提交后
// 按行内 file_path 物理删文件（含操作抑制登记——文件离开 store/ 白名单子树会触发 fsmonitor 外部
// 变更裁决）。clearRef 为引用列清理动作（调用方闭包携带作者行身份与置 NULL 写入）。
// 引用指向的行若为软删行（外部裁决失效态），GetById 经软删 scope 不可见，仅清引用列
func (s *Service) deleteAvatarByReference(ctx context.Context, storeId int64, clearRef func(ctx context.Context) error) error {
	var relForFile string
	store, err := s.storeOps.GetById(ctx, storeId)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询旧头像 store 行失败: %w", err)
		}
		store = nil
	}
	if store != nil && store.FilePath.Valid {
		relForFile = store.FilePath.String
	}
	if err := s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		if err := clearRef(txCtx); err != nil {
			return fmt.Errorf("清除头像引用列失败: %w", err)
		}
		return s.storeOps.DeleteUnscopedByIds(txCtx, []int64{storeId})
	}); err != nil {
		return err
	}
	if relForFile != "" {
		s.removeAvatarFile(s.workDir.GetWorkDir(), relForFile)
	}
	return nil
}

// removeAvatarFile 物理删单个头像文件（absPath 域现场拼接；含 fsmonitor 操作抑制登记——文件
// 离开 store/ 白名单子树会触发外部变更裁决）。尽力而为：文件不存在视为已删，其他失败记日志
// 留 fsmonitor 对账裁决；工作目录未配置态无库内文件可删，直接跳过
func (s *Service) removeAvatarFile(workDir, relPath string) {
	if workDir == "" {
		return
	}
	storeRegistry.Suppress(relPath)
	defer storeRegistry.Release(relPath)
	if err := os.Remove(filepath.Join(workDir, relPath)); err != nil && !os.IsNotExist(err) {
		logger.Log.Warnf("[authorInfo] 删除头像文件失败: %v", err)
	}
}

// ===== local 侧头像手动导入（Handler 经 SetLocalAuthorAvatar / RemoveLocalAuthorAvatar 消费） =====

// SetLocalAuthorAvatar 为本地作者设置头像：源文件为用户经前端文件对话框选取的任意盘绝对路径，
// 校验图片格式白名单后拷入暂存作用域（跨卷不可 rename，恒 copy；尺寸上限与 site 侧一致）→
// 旧头像按换头像形态先行删除（同扩展名时新旧路径相同，先删旧保证不做同路径覆写、亦不撞
// file_path 活行唯一索引）→ 四调用入库（撤回处置=丢弃，重导入即重产）→ 业务事务内建 store 行 +
// 同事务写 local_author.avatar_store_id（引用列与建行同生共死）。失败不留半成品，旧头像在拷贝
// 成功后才删——源文件不可读/超限的失败不伤及现有头像
func (s *Service) SetLocalAuthorAvatar(ctx context.Context, localAuthorId int64, sourceAbsPath string) error {
	workDir := s.workDir.GetWorkDir()
	if err := settings.RefuseIfUnconfigured(workDir, "authorInfo"); err != nil {
		return err
	}
	author, err := s.localAuthors.GetById(ctx, localAuthorId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return pkgerr.NewBusinessError(404, "本地作者不存在")
		}
		return fmt.Errorf("查询本地作者失败: %w", err)
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(sourceAbsPath), "."))
	if !avatarFormatWhitelist[ext] {
		return pkgerr.NewBusinessError(400, "不支持的头像图片格式（支持 jpg/jpeg/png/gif/webp/bmp）")
	}
	finalRel, err := LocalAvatarRelPath(localAuthorId, ext)
	if err != nil {
		return fmt.Errorf("派生本地作者头像落盘路径失败: %w", err)
	}
	scopeKey := localAvatarScopeKeyPrefix + strconv.FormatInt(localAuthorId, 10)
	// 作用域先清后建：本作者私有暂存目录自愈残留（上次会话未回收的作用域内容已无消费价值）
	if err := staging.RemoveScope(ctx, workDir, staging.OwnerAuthorInfo, scopeKey); err != nil {
		return fmt.Errorf("清理头像导入暂存作用域失败: %w", err)
	}
	if _, err := staging.CreateScope(ctx, workDir, staging.OwnerAuthorInfo, scopeKey, localAvatarScopeContentShape); err != nil {
		return fmt.Errorf("创建头像导入暂存作用域失败: %w", err)
	}
	defer func() {
		if err := staging.RemoveScope(context.Background(), workDir, staging.OwnerAuthorInfo, scopeKey); err != nil {
			logger.Log.Warnf("[authorInfo] 回收头像导入暂存作用域失败（留待启动清扫）: %v", err)
		}
	}()
	stagingFileRel := path.Join(staging.RootName, string(staging.OwnerAuthorInfo), scopeKey, path.Base(finalRel))
	if err := copyCappedFile(sourceAbsPath, filepath.Join(workDir, filepath.FromSlash(stagingFileRel)), maxAvatarBytes); err != nil {
		return err
	}
	// 换头像先删旧：拷贝已成功，此后的失败路径崩溃窗口收敛为「无头像」，重新导入即可
	if author.AvatarStoreID.Valid {
		if err := s.deleteAvatarByReference(ctx, author.AvatarStoreID.Int64, func(txCtx context.Context) error {
			return s.localAuthors.UpdateAvatarStoreId(txCtx, localAuthorId, sql.NullInt64{})
		}); err != nil {
			return err
		}
	}
	intentIds, err := s.ingestor.PrepareIngest(ctx, []persistentStore.IngestItem{{
		FilePath:    finalRel,
		StagingPath: stagingFileRel,
		AbortAction: entity.AbortActionDiscard,
	}})
	if err != nil {
		return fmt.Errorf("登记头像入库意图失败: %w", err)
	}
	abort := func() {
		if err := s.ingestor.AbortIngest(context.Background(), intentIds); err != nil {
			logger.Log.Warnf("[authorInfo] 撤回头像入库失败（登记行保留，启动恢复收口）: %v", err)
		}
	}
	if err := s.ingestor.PlaceIngest(ctx, intentIds); err != nil {
		abort()
		return fmt.Errorf("头像落位失败: %w", err)
	}
	if err := s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		storeId, cerr := s.ingestor.CommitIngest(txCtx, intentIds[0], finalRel, path.Base(finalRel),
			sql.NullString{}, sql.NullString{})
		if cerr != nil {
			return fmt.Errorf("建头像 store 行失败: %w", cerr)
		}
		return s.localAuthors.UpdateAvatarStoreId(txCtx, localAuthorId, sql.NullInt64{Int64: storeId, Valid: true})
	}); err != nil {
		abort()
		return fmt.Errorf("提交头像入库失败: %w", err)
	}
	return nil
}

// RemoveLocalAuthorAvatar 移除本地作者头像：置 NULL + 删 store 行（同一事务）+ 事务提交后删文件。
// 无头像（引用列空）为幂等成功
func (s *Service) RemoveLocalAuthorAvatar(ctx context.Context, localAuthorId int64) error {
	if err := settings.RefuseIfUnconfigured(s.workDir.GetWorkDir(), "authorInfo"); err != nil {
		return err
	}
	author, err := s.localAuthors.GetById(ctx, localAuthorId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return pkgerr.NewBusinessError(404, "本地作者不存在")
		}
		return fmt.Errorf("查询本地作者失败: %w", err)
	}
	if !author.AvatarStoreID.Valid {
		return nil
	}
	return s.deleteAvatarByReference(ctx, author.AvatarStoreID.Int64, func(txCtx context.Context) error {
		return s.localAuthors.UpdateAvatarStoreId(txCtx, localAuthorId, sql.NullInt64{})
	})
}

// copyCappedFile 拷贝文件到目标路径（目标目录须已存在；上限 maxBytes 超限报错——读取侧
// 截断到 max+1 字节判定）。源头像可在任意盘，跨卷 rename 不可行故恒 copy
func copyCappedFile(sourceAbsPath, dstAbsPath string, maxBytes int64) error {
	src, err := os.Open(sourceAbsPath)
	if err != nil {
		return pkgerr.NewBusinessError(400, fmt.Sprintf("打开头像源文件失败: %v", err))
	}
	defer src.Close()
	dst, err := os.OpenFile(dstAbsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("打开头像暂存文件失败: %w", err)
	}
	defer dst.Close()
	n, err := io.Copy(dst, io.LimitReader(src, maxBytes+1))
	if err != nil {
		return fmt.Errorf("拷贝头像文件失败: %w", err)
	}
	if n > maxBytes {
		return pkgerr.NewBusinessError(400, fmt.Sprintf("头像文件超过上限 %dMB", maxBytes/(1024*1024)))
	}
	return nil
}

// ===== 删除联动头像清理（siteAuthor/localAuthor 的 AvatarFileCleaner 实现） =====

// DeleteAvatarStoreRows 删除联动的行面：在调用方删除事务内执行（ctx 携带事务连接）——读被删
// 引用的头像行（含软删失效行——作者行消亡后其头像行即无主死行，一并物理删清不留回收站孤儿）
// 取文件路径，再物理删行。调用契约：须在作者行删除之后调用，FK NO ACTION 下引用未释放时
// 删行被外键拒绝。返回被删行的文件相对路径清单（供事务提交后删文件；行缺失返回空清单）
func (s *Service) DeleteAvatarStoreRows(ctx context.Context, storeIds []int64) ([]string, error) {
	if len(storeIds) == 0 {
		return nil, nil
	}
	relPaths := make([]string, 0, len(storeIds))
	for _, row := range s.storeOps.ListByIdsIncludeDeleted(ctx, storeIds) {
		if row.FilePath.Valid && row.FilePath.String != "" {
			relPaths = append(relPaths, row.FilePath.String)
		}
	}
	if err := s.storeOps.DeleteUnscopedByIds(ctx, storeIds); err != nil {
		return nil, fmt.Errorf("物理删头像 store 行失败: %w", err)
	}
	return relPaths, nil
}

// RemoveAvatarFiles 删除联动的文件面：事务提交后按 relPath 物理删头像文件（含 fsmonitor 操作
// 抑制登记）。尽力而为——失败记日志留对账裁决，不阻断调用方删除链收尾
func (s *Service) RemoveAvatarFiles(relPaths []string) {
	if len(relPaths) == 0 {
		return
	}
	workDir := s.workDir.GetWorkDir()
	for _, rel := range relPaths {
		s.removeAvatarFile(workDir, rel)
	}
}

// acquire 在途去重登记：命中（该作者已在拉取中）返回 false。两触发面共用
func (s *Service) acquire(siteAuthorId int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.inFlight[siteAuthorId]; ok {
		return false
	}
	s.inFlight[siteAuthorId] = struct{}{}
	return true
}

// release 释放在途登记
func (s *Service) release(siteAuthorId int64) {
	s.mu.Lock()
	delete(s.inFlight, siteAuthorId)
	s.mu.Unlock()
}
