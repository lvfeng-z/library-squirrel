package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	domain "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/plugin/extension"
	"github.com/library-squirrel/backend/util"
)

const (
	// PluginRoot 插件根目录
	PluginRoot = "plugin"
	// PluginPackageRoot 插件包目录
	PluginPackageRoot = "plugin/package"

	// 插件来源枚举（plugin.Source 字段取值，由主程序按安装入口判定，不由插件声明）
	SourceBundled     = "bundled"
	SourceLocal       = "local"
	SourceURL         = "url"
	SourceMarketplace = "marketplace"
)

// 错误定义
var (
	ErrPluginNotFound      = errors.New("plugin not found")
	ErrPluginAlreadyExists = errors.New("plugin already exists")
	ErrInvalidPackage      = errors.New("invalid plugin package")
	ErrInvalidManifest     = errors.New("invalid plugin manifest")
	ErrBackupNotFound      = errors.New("backup not found")
	// ErrPluginStateChanged 并发守卫命中：读取行后行内备份引用已被其他流程（重装/换版）改写，
	// 本次操作中止，由调用方引导重试
	ErrPluginStateChanged = errors.New("plugin state changed since read")
)

// installContext 安装上下文：来源与信任（由安装入口判定，服务端权威；与承载 manifest 数据的 installDTO 正交）
type installContext struct {
	Source  string // 来源枚举 SourceBundled/SourceLocal/SourceURL/SourceMarketplace
	Trusted bool   // 信任标记；bundled=true，第三方经用户确认后 true，绕过 UI 的异常安装为 false
}

// resolveReinstallContext 据原插件来源与调用方透传的 trusted 解析重装上下文：
// 来源沿用原插件记录（NULL 兜底 local），bundled 强制信任（不论透传值），第三方沿用透传 trusted。
func resolveReinstallContext(source sql.NullString, trusted bool) installContext {
	resolved := SourceLocal
	if source.Valid {
		resolved = source.String
	}
	resolvedTrusted := trusted
	if resolved == SourceBundled {
		resolvedTrusted = true
	}
	return installContext{Source: resolved, Trusted: resolvedTrusted}
}

// Repository 插件仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建
	Create(ctx context.Context, plugin *entity2.Plugin) error
	// CreateBatch 批量新建
	CreateBatch(ctx context.Context, plugins []*entity2.Plugin) error
	// Updates 更新（部分更新，仅非零字段）
	Updates(ctx context.Context, plugin *entity2.Plugin) error
	// Save 完整替换（GORM Save，含零值；重新安装等场景使用）
	Save(ctx context.Context, plugin *entity2.Plugin) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity2.Plugin, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*entity2.Plugin, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity2.Plugin], error)
	// CheckInstalled 检查插件是否已安装
	CheckInstalled(ctx context.Context, publicId string) (bool, error)
	// GetByPublicId 根据公开ID获取
	GetByPublicId(ctx context.Context, publicId string) (*entity2.Plugin, error)
	// ListReferencedBackupIds 全量投影行内 BackupID（供备份治理引用集对账）
	ListReferencedBackupIds(ctx context.Context) ([]int64, error)
	// ClearBackupRefsByBackupIds 按引用目标清列（悬空引用清列，BackupID 置 NULL）
	ClearBackupRefsByBackupIds(ctx context.Context, ids []int64) error
	// MarkUninstalledAndClearBackup 标记已卸载并清空备份引用（expectedBackupId 为并发守卫——
	// 行内引用与捕获值不符时 0 行受影响，返回 ErrPluginStateChanged 由调用方引导重试）
	MarkUninstalledAndClearBackup(ctx context.Context, publicId string, expectedBackupId int64) error
}

// BackupProvider 备份提供者接口（由 plugin service 定义需要的备份能力）
type BackupProvider interface {
	// CreateBackup 复制源文件到备份目录并建保管清单行（插件安装包备份，保留源文件）
	CreateBackup(ctx context.Context, sourcePath string) (*entity2.Backup, error)
	// GetById 按清单行 ID 获取备份（行内 BackupID 引用直查）
	GetById(ctx context.Context, id int64) (*entity2.Backup, error)
	// DeleteBackup 删除备份的磁盘文件与清单行（文件缺失容忍；卸载/换版直清旧备份）
	DeleteBackup(ctx context.Context, id int64) error
}

// PluginActivator 插件激活器接口，由应用层实现，负责读取 manifest、注册静态资源、前端扩展和启动子进程
type PluginActivator interface {
	// Activate 激活已安装的插件
	Activate(plugin *entity2.Plugin) error
}

// Service 插件服务
type Service struct {
	repo           Repository
	backupProvider BackupProvider
	activator      PluginActivator
	participants   []LifecycleParticipant
	runtimeStopper func(pluginPublicId string) error

	runtimeStatusProvider RuntimeStatusProvider
	extensionListProvider ExtensionListProvider
	urlListenerProvider   UrlListenerProvider

	pendingMu       sync.Mutex                      // 检查更新待办列表互斥锁（含执行中守卫）
	pendingUpgrades map[string]*pendingUpgradeEntry // 启动期检测出的更新待办（内存态，重启重检）
}

// NewService 创建插件服务
func NewService(repo Repository, backupProvider BackupProvider) *Service {
	return &Service{
		repo:            repo,
		backupProvider:  backupProvider,
		pendingUpgrades: make(map[string]*pendingUpgradeEntry),
	}
}

// SetActivator 设置插件激活器
func (s *Service) SetActivator(activator PluginActivator) {
	s.activator = activator
}

// RuntimeStatusProvider 运行时状态提供者接口
type RuntimeStatusProvider interface {
	GetPluginRuntimeStatus(pluginPublicId string) *RuntimeStatus
}

// RuntimeStatus 运行时状态（plugin 包内部类型，供 Provider 返回）
type RuntimeStatus struct {
	IsRunning   bool
	PID         int
	ActivatedAt int64 // Unix 毫秒
}

// ExtensionListProvider 扩展点列表提供者接口
type ExtensionListProvider interface {
	GetTaskHandlersByPlugin(pluginPublicId string) []ExtensionMeta
	GetSiteBrowsersByPlugin(pluginPublicId string) []ExtensionMeta
	GetFrontendExtensionsByPlugin(pluginPublicId string) []FrontendExtensionMeta
}

// UrlListenerProvider URL 监听规则提供者接口
type UrlListenerProvider interface {
	ListPatternsByPlugin(pluginPublicId string) []string
}

// ExtensionMeta 扩展点元数据
type ExtensionMeta struct {
	ID          string
	Name        string
	Description string
}

// FrontendExtensionMeta 前端扩展元数据
type FrontendExtensionMeta struct {
	ID   string
	Name string
	Kind string
}

// SetRuntimeStatusProvider 设置运行时状态提供者
func (s *Service) SetRuntimeStatusProvider(provider RuntimeStatusProvider) {
	s.runtimeStatusProvider = provider
}

// SetExtensionListProvider 设置扩展点列表提供者
func (s *Service) SetExtensionListProvider(provider ExtensionListProvider) {
	s.extensionListProvider = provider
}

// SetUrlListenerProvider 设置 URL 监听规则提供者
func (s *Service) SetUrlListenerProvider(provider UrlListenerProvider) {
	s.urlListenerProvider = provider
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*entity2.Plugin, error) {
	return s.repo.GetById(ctx, id)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, page *model.Page[entity2.Plugin], query PluginQueryDTO) (*model.Page[entity2.Plugin], error) {
	// 默认过滤已卸载的插件
	if query.Uninstalled.IsEmpty() {
		query.Uninstalled.Value = new(bool)
	}
	conv := querypkg.NewConverter(entity2.Plugin{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*entity2.Plugin, error) {
	return s.repo.List(ctx, opt)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// CheckInstalled 检查插件是否已安装
func (s *Service) CheckInstalled(ctx context.Context, publicId string) (bool, error) {
	return s.repo.CheckInstalled(ctx, publicId)
}

// GetByPublicId 根据公开ID获取插件
func (s *Service) GetByPublicId(ctx context.Context, publicId string) (*entity2.Plugin, error) {
	return s.repo.GetByPublicId(ctx, publicId)
}

// InstallFromPath 从本地插件包路径安装插件。来源固定 local；trusted 由调用方透传用户的知情同意结果，
// 缺省/绕过 UI 的异常安装为 false（运行门控：trusted=false 的插件不激活，需用户在管理页显式信任）。
func (s *Service) InstallFromPath(ctx context.Context, packagePath string, trusted bool) (*entity2.Plugin, error) {
	// 验证文件存在
	if !util.FileExists(packagePath) {
		return nil, fmt.Errorf("plugin package not found: %s", packagePath)
	}

	// 加载插件包
	installDTO, err := s.loadPluginPackage(packagePath)
	if err != nil {
		return nil, err
	}

	return s.install(ctx, installDTO, false, installContext{Source: SourceLocal, Trusted: trusted})
}

// loadPluginPackage 加载插件安装包
func (s *Service) loadPluginPackage(packagePath string) (*domain.PluginInstallDTO, error) {
	// 读取包字节
	pkgBytes, err := os.ReadFile(packagePath)
	if err != nil {
		return nil, ErrInvalidPackage
	}

	// 打开 ZIP
	reader, err := zip.NewReader(bytes.NewReader(pkgBytes), int64(len(pkgBytes)))
	if err != nil {
		return nil, ErrInvalidPackage
	}

	// 查找 plugin.json
	var manifestBytes []byte
	for _, file := range reader.File {
		if file.Name == "plugin.json" {
			rc, err := file.Open()
			if err != nil {
				return nil, ErrInvalidPackage
			}
			manifestBytes, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, ErrInvalidPackage
			}
			break
		}
	}

	if len(manifestBytes) == 0 {
		return nil, ErrInvalidManifest
	}

	// 解析 manifest
	var manifest domain.PluginManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, ErrInvalidManifest
	}

	logger.Log.Infof("解析插件包: id=%s name=%s version=%s author=%s",
		manifest.ID, manifest.Name, manifest.Version, manifest.Author)

	// 验证必要字段
	if manifest.ID == "" || manifest.Name == "" || manifest.Version == "" || manifest.Author == "" {
		return nil, ErrInvalidManifest
	}
	// 身份键校验（id 即 publicId）：反向域名格式，且拒绝旧身份 UUID 后缀残留（防新旧格式混存产生双身份记录）
	if err := domain.ValidatePluginID(manifest.ID); err != nil {
		return nil, err
	}
	if manifest.Extensions == nil {
		return nil, ErrInvalidManifest
	}
	ext := manifest.Extensions
	hasExtensions := len(ext.TaskHandlers) > 0 || len(ext.SiteBrowsers) > 0 || len(ext.FrontendExtensions) > 0
	if !hasExtensions {
		return nil, ErrInvalidManifest
	}
	// EntryFile 仅在有运行时扩展点时必填
	hasRuntime := len(ext.TaskHandlers) > 0 || len(ext.SiteBrowsers) > 0
	if hasRuntime && manifest.EntryFile == "" {
		return nil, ErrInvalidManifest
	}

	// 契约版本兼容校验（安装期预检；过新/过旧均拒绝安装，与加载期 LoadPluginProcess 终检互为兜底）
	if err := extension.ValidateContractVersion(manifest.ContractVersion); err != nil {
		return nil, err
	}

	// 构建安装 DTO
	installDTO := manifest.ToPluginInstallDTO(packagePath)
	return installDTO, nil
}

// install 安装插件（包含激活）。reinstall=true 为重装/升级（复用同 publicId 原记录覆盖），false 为全新安装（已存在且未卸载则报错）
func (s *Service) install(ctx context.Context, installDTO *domain.PluginInstallDTO, reinstall bool, ictx installContext) (*entity2.Plugin, error) {
	plugin, err := s.installCore(ctx, installDTO, reinstall, ictx)
	if err != nil {
		return nil, err
	}

	// 激活插件
	if s.activator != nil {
		if err := s.activator.Activate(plugin); err != nil {
			logger.Log.Warnf("插件激活失败: %s, %v", installDTO.PublicID, err)
		}
	}

	return plugin, nil
}

// InstallBundled 安装捆绑插件（检查更新流的检测入口）。仅 pre-Run 由启动扫描调用：
// 运行期换版必须走 ApplyPendingUpgrade（含参与者否决），勿复用本方法——强制分支的直装会绕过否决。
// 分支优先级（已装且未卸载、判变成立时）：契约不兼容强制 > 拒绝标记短路 > 未打标记默升级 > 记 available 待办。
// 仅 bundled 来源记录参与判变（尊重用户手动安装的版本）；非 bundled 静默跳过。
// 与 InstallFromPath 的区别：不调用 activator.Activate()，已安装时不报错
func (s *Service) InstallBundled(ctx context.Context, packagePath string) (*entity2.Plugin, error) {
	// 加载插件包：失败（含契约不兼容装不进）记 error 待办供管理页告知，原错误继续上抛（调用方记日志）
	installDTO, err := s.loadPluginPackage(packagePath)
	if err != nil {
		s.recordPending(&pendingUpgradeEntry{
			PluginName:  filepath.Base(packagePath),
			Kind:        PendingKindError,
			Source:      SourceBundled,
			PackagePath: packagePath,
			Message:     err.Error(),
		})
		return nil, err
	}

	// 检查是否已安装且未卸载
	existing, err := s.repo.GetByPublicId(ctx, installDTO.PublicID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Uninstalled.Valid && !existing.Uninstalled.Bool {
		if needBundledUpgrade(existing, installDTO) {
			// 已装契约不兼容：旧版无法在当前主程序加载，保留无意义——强制直装换版（pre-Run 无进程可停，
			// 无视拒绝标记），记 forced 待办供管理页告知
			if forceErr := extension.ValidateContractVersion(contractVersionOf(existing)); forceErr != nil {
				logger.Log.Warnf("已装插件契约不兼容，强制升级重装: %s, %v", installDTO.PublicID, forceErr)
				plugin, installErr := s.installCore(ctx, installDTO, true, resolveReinstallContext(existing.Source, true))
				if installErr != nil {
					return nil, installErr
				}
				s.recordPending(&pendingUpgradeEntry{
					PublicID:         installDTO.PublicID,
					PluginName:       installDTO.Name,
					InstalledVersion: versionString(existing.Version),
					TargetVersion:    installDTO.Version,
					TargetBuildID:    installDTO.BuildID,
					Direction:        upgradeDirection(versionString(existing.Version), installDTO.Version),
					Kind:             PendingKindForced,
					Source:           SourceBundled,
					PackagePath:      packagePath,
					Message:          forceErr.Error(),
				})
				return plugin, nil
			}
			// 拒绝标记等值短路：用户已跳过此构建，不再记待办（新 buildId 到来自动失效）
			if existing.UpgradeDeclinedBuildID.Valid && existing.UpgradeDeclinedBuildID.String == installDTO.BuildID {
				logger.Log.Infof("捆绑插件升级已被用户跳过(buildId=%s): %s", installDTO.BuildID, installDTO.PublicID)
				return nil, nil
			}
			// 未打标包：判据回落 version，维持静默升级（不进检查更新流；打标校验由构建管线兜底）
			if installDTO.BuildID == "" {
				logger.Log.Infof("捆绑插件包未打标，按 version 静默升级重装: %s", installDTO.PublicID)
				return s.installCore(ctx, installDTO, true, resolveReinstallContext(existing.Source, true))
			}
			// 可升级：记 available 待办，保留旧版继续运行（红点提醒，答复走 ApplyPendingUpgrade 当次生效）
			logger.Log.Infof("捆绑插件有新构建(buildId=%s)，记入更新待办: %s", installDTO.BuildID, installDTO.PublicID)
			s.recordPending(&pendingUpgradeEntry{
				PublicID:         installDTO.PublicID,
				PluginName:       installDTO.Name,
				InstalledVersion: versionString(existing.Version),
				TargetVersion:    installDTO.Version,
				TargetBuildID:    installDTO.BuildID,
				Direction:        upgradeDirection(versionString(existing.Version), installDTO.Version),
				Kind:             PendingKindAvailable,
				Source:           SourceBundled,
				PackagePath:      packagePath,
			})
			return nil, nil
		}
		logger.Log.Infof("捆绑插件已安装，跳过: %s", installDTO.PublicID)
		// 存量官方身份证实（启动扫描顺带）：历史手动安装的官方包在此按已装目录内容纠正 Official 标记
		installedDir := ""
		if existing.RootPath.Valid && existing.RootPath.String != "" {
			installedDir = filepath.Join(s.getAppRoot(), existing.RootPath.String)
		}
		s.verifyExistingOfficial(ctx, officialRoster(), existing, installedDir)
		return nil, nil
	}

	return s.installCore(ctx, installDTO, false, installContext{Source: SourceBundled, Trusted: true})
}

// needBundledUpgrade 判断已安装记录是否需随捆绑包升级重装：
// 仅 bundled 来源的记录受此约束。判据优先 buildId（构建身份，同源码状态永远同值）——
// 不一致或已装缺失（历史记录）即重装；zip 未打标时回落 version 比较（同版本跳过，
// 该盲区由 build:plugins 的打标校验兜底，仅绕过管线的疑似手工包会落入）
func needBundledUpgrade(existing *entity2.Plugin, installDTO *domain.PluginInstallDTO) bool {
	if !existing.Source.Valid || existing.Source.String != SourceBundled {
		return false
	}
	if installDTO.BuildID != "" {
		return !existing.BuildID.Valid || existing.BuildID.String != installDTO.BuildID
	}
	return !existing.Version.Valid || existing.Version.String != installDTO.Version
}

// installCore 安装插件核心逻辑（解压 + 写 DB + 备份），不含激活。
// reinstall=true 为重装/升级：复用同 publicId 原记录（保留 ID 与 CreateTime）覆盖，不论其卸载状态；
// reinstall=false 为全新安装：同 publicId 且未卸载的记录视为重复安装，返回 ErrPluginAlreadyExists。
func (s *Service) installCore(ctx context.Context, installDTO *domain.PluginInstallDTO, reinstall bool, ictx installContext) (*entity2.Plugin, error) {
	// 检查是否已安装
	existing, err := s.repo.GetByPublicId(ctx, installDTO.PublicID)
	if err != nil {
		return nil, err
	}

	// reusePlugin 非 nil 时复用该记录（保留 ID 与 CreateTime）覆盖写入；nil 时新建
	var reusePlugin *entity2.Plugin
	if existing != nil {
		if reinstall {
			// 重装/升级：复用原记录覆盖
			reusePlugin = existing
		} else {
			// 全新安装：同 publicId 且未卸载的记录视为重复
			if existing.Uninstalled.Valid && !existing.Uninstalled.Bool {
				return nil, ErrPluginAlreadyExists
			}
			reusePlugin = existing
		}
	}

	// 构建安装路径（插件安装到应用根目录）
	appRoot := s.getAppRoot()
	pathRelative := filepath.Join(installDTO.PublicID, installDTO.Version)
	installPath := filepath.Join(appRoot, PluginPackageRoot, pathRelative)

	// 创建目录并解压
	if err := util.CreateDirIfNotExists(installPath); err != nil {
		return nil, err
	}
	logger.Log.Infof("解压插件包到: %s", installPath)
	if err := util.ExtractZip(installDTO.PackagePath, installPath); err != nil {
		return nil, err
	}

	// 构建插件记录
	plugin := entity2.NewPlugin()
	plugin.PublicID = sql.NullString{String: installDTO.PublicID, Valid: true}
	plugin.Author = sql.NullString{String: installDTO.Author, Valid: true}
	plugin.Name = sql.NullString{String: installDTO.Name, Valid: true}
	plugin.Version = sql.NullString{String: installDTO.Version, Valid: true}
	plugin.ContractVersion = sql.NullInt64{Int64: int64(installDTO.ContractVersion), Valid: installDTO.ContractVersion > 0}
	plugin.ConfigSchemaVersion = sql.NullInt64{Int64: int64(installDTO.ConfigSchemaVersion), Valid: true} // 0=legacy/未管理，总是写入
	if len(installDTO.Capabilities) > 0 {
		if capsJSON, err := json.Marshal(installDTO.Capabilities); err == nil {
			plugin.Capabilities = sql.NullString{String: string(capsJSON), Valid: true}
		}
	}
	if len(installDTO.ResourceTypes) > 0 {
		if rtJSON, err := json.Marshal(installDTO.ResourceTypes); err == nil {
			plugin.ResourceTypes = sql.NullString{String: string(rtJSON), Valid: true}
		}
	}
	plugin.EntryPath = sql.NullString{String: filepath.Join(PluginPackageRoot, pathRelative, installDTO.EntryFile), Valid: true}
	plugin.RootPath = sql.NullString{String: filepath.Join(PluginPackageRoot, pathRelative), Valid: true}
	plugin.ActivationType = sql.NullString{String: string(rune(installDTO.Activation.Type + '0')), Valid: true}
	plugin.Uninstalled = sql.NullBool{Bool: false, Valid: true}
	plugin.Source = sql.NullString{String: ictx.Source, Valid: ictx.Source != ""}
	plugin.SourceDetail = sql.NullString{String: installDTO.PackagePath, Valid: installDTO.PackagePath != ""}
	plugin.BuildID = sql.NullString{String: installDTO.BuildID, Valid: installDTO.BuildID != ""}
	plugin.Trusted = sql.NullBool{Bool: ictx.Trusted, Valid: true}
	// 官方身份统一查名单推导（单一推导点，重装链换内容后随此重算）：与渠道/信任均正交
	plugin.Official = s.matchOfficial(installDTO)

	// 捕获重装前的旧备份引用（换版直清目标；Save 全字段覆盖后行内旧值即失）
	prevBackupID := int64(0)
	if reusePlugin != nil && reusePlugin.BackupID.Valid {
		prevBackupID = reusePlugin.BackupID.Int64
	}

	if reusePlugin != nil {
		// 复用原记录（重装/升级，或重装已卸载插件）：完整替换该记录的所有字段，保留原始 ID 与 CreateTime
		plugin.ID = reusePlugin.ID
		plugin.SetCreateTime(reusePlugin.GetCreateTime())
		if err := s.repo.Save(ctx, plugin); err != nil {
			return nil, err
		}
	} else {
		// 新建插件
		if err := s.repo.Create(ctx, plugin); err != nil {
			return nil, err
		}
	}

	// 创建备份（安装包复制入 backup/，清单行 ID 内嵌 plugin.BackupID 引用）
	backup, err := s.backupProvider.CreateBackup(ctx, installDTO.PackagePath)
	if err != nil {
		return nil, err
	}
	plugin.BackupID = sql.NullInt64{Int64: backup.ID, Valid: true}
	if err := s.repo.Updates(ctx, plugin); err != nil {
		return nil, err
	}

	// 换版直清旧备份：备份仅服务当前安装版本的重装修复，行内引用已指向新备份后旧备份无用；
	// 失败留无主备份由治理保留期兜底
	if prevBackupID > 0 && prevBackupID != backup.ID {
		if err := s.backupProvider.DeleteBackup(ctx, prevBackupID); err != nil {
			logger.Log.Warnf("换版后清理旧安装包备份 %d 失败（留待治理清理）: %v", prevBackupID, err)
		}
	}

	// 重装时清理旧安装目录（解压与落库均已成功后执行，失败最多留垃圾目录、不产生半损态）：
	// publicId 改名或版本变更都会使安装路径变化，旧目录不再被任何记录引用
	if reusePlugin != nil && reusePlugin.RootPath.Valid && reusePlugin.RootPath.String != "" &&
		reusePlugin.RootPath.String != plugin.RootPath.String {
		oldInstallPath := filepath.Join(appRoot, reusePlugin.RootPath.String)
		if err := util.RemoveDir(oldInstallPath); err != nil {
			logger.Log.Warnf("清理旧插件目录失败: %s, %v", oldInstallPath, err)
		} else {
			// 逐级回收空父目录（如身份键简化前残留的 author 层；os.Remove 仅空目录成功，非空静默失败）
			_ = os.Remove(filepath.Dir(oldInstallPath))
			_ = os.Remove(filepath.Dir(filepath.Dir(oldInstallPath)))
		}
	}

	logger.Log.Infof("插件已安装: %s/%s-%s", installDTO.Author, installDTO.Name, installDTO.Version)
	return plugin, nil
}

// Name 引用方展示名（实现 backupGovernance.BackupReferencer：监视哨统计分组与备份管理面板用）
func (s *Service) Name() string {
	return "插件"
}

// ListReferencedBackupIDs 全量行内 BackupID 引用的清单行 ID（实现 backupGovernance.BackupReferencer）。
// 卸载链清空 backup_id，已卸载行不再持有备份引用——投影集即当前已安装版本的现役引用
func (s *Service) ListReferencedBackupIDs(ctx context.Context) ([]int64, error) {
	return s.repo.ListReferencedBackupIds(ctx)
}

// ClearBackupRefsByBackupIDs 按引用目标清列（实现 backupGovernance.BackupReferencer：治理方算出
// 悬空 ID 后调用，BackupID 置 NULL）
func (s *Service) ClearBackupRefsByBackupIDs(ctx context.Context, ids []int64) error {
	return s.repo.ClearBackupRefsByBackupIds(ctx, ids)
}

// Reinstall 重新安装插件（trusted 由调用方透传；source 沿用原插件来源）
func (s *Service) Reinstall(ctx context.Context, pluginPublicId string, trusted bool) (*entity2.Plugin, error) {
	// 获取插件
	plugin, err := s.repo.GetByPublicId(ctx, pluginPublicId)
	if err != nil {
		return nil, err
	}
	if plugin == nil {
		return nil, ErrPluginNotFound
	}
	if !plugin.BackupID.Valid || plugin.BackupID.Int64 == 0 {
		return nil, ErrBackupNotFound
	}

	// 获取备份（行内 BackupID 直查保管清单）
	backup, err := s.backupProvider.GetById(ctx, plugin.BackupID.Int64)
	if err != nil {
		return nil, err
	}
	if backup == nil {
		return nil, ErrBackupNotFound
	}

	// 构建备份文件路径
	workdir := ""
	if backup.Workdir.Valid {
		workdir = backup.Workdir.String
	}
	filePath := ""
	if backup.FilePath.Valid {
		filePath = backup.FilePath.String
	}
	packagePath := filepath.Join(workdir, filePath)

	return s.ReinstallFromPath(ctx, pluginPublicId, packagePath, trusted)
}

// ReinstallFromPath 从指定路径重新安装插件。source 沿用原插件来源；bundled 强制 trusted=true，第三方用调用方透传的 trusted
func (s *Service) ReinstallFromPath(ctx context.Context, pluginPublicId string, packagePath string, trusted bool) (*entity2.Plugin, error) {
	if packagePath == "" {
		return nil, fmt.Errorf("package path is required")
	}

	// 获取原插件信息
	plugin, err := s.repo.GetByPublicId(ctx, pluginPublicId)
	if err != nil {
		return nil, err
	}
	if plugin == nil {
		return nil, ErrPluginNotFound
	}

	// 停止旧插件运行时并删除旧文件（不修改数据库状态）
	if err := s.deactivate(ctx, pluginPublicId, PluginStopOpUpdate); err != nil {
		return nil, err
	}

	// 加载新插件包
	installDTO, err := s.loadPluginPackage(packagePath)
	if err != nil {
		return nil, err
	}

	// 验证 publicId 一致
	if installDTO.PublicID != pluginPublicId {
		return nil, fmt.Errorf("plugin publicId mismatch")
	}

	// 重新安装（来源沿用原插件、bundled 强制信任、第三方沿用透传 trusted）
	return s.install(ctx, installDTO, true, resolveReinstallContext(plugin.Source, trusted))
}

// Uninstall 卸载插件
func (s *Service) Uninstall(ctx context.Context, pluginPublicId string) error {
	// 获取插件
	plugin, err := s.repo.GetByPublicId(ctx, pluginPublicId)
	if err != nil {
		return err
	}
	if plugin == nil {
		return ErrPluginNotFound
	}

	return s.uninstall(ctx, pluginPublicId)
}

// SetTrusted 设置插件信任状态（手动信任/取消信任入口）。
// trusted=true 落标记后立即激活；trusted=false 即时停用运行时（停进程+清痕迹，不删文件不卸载标记），
// 停用成功才落标记——参与者否决（如运行中任务拦截）时插件保持运行、标记不变；
// force=true 跳过否决检查（前端确认对话框明示代价后传入）
func (s *Service) SetTrusted(ctx context.Context, pluginPublicId string, trusted bool, force bool) (*entity2.Plugin, error) {
	plugin, err := s.repo.GetByPublicId(ctx, pluginPublicId)
	if err != nil {
		return nil, err
	}
	if plugin == nil {
		return nil, ErrPluginNotFound
	}

	if !trusted {
		if err := s.stopRuntime(ctx, pluginPublicId, PluginStopOpUntrust, force); err != nil {
			return nil, err
		}
	}

	plugin.Trusted = sql.NullBool{Bool: trusted, Valid: true}
	if err := s.repo.Save(ctx, plugin); err != nil {
		return nil, err
	}

	if trusted && s.activator != nil {
		if err := s.activator.Activate(plugin); err != nil {
			logger.Log.Warnf("信任后激活插件失败: %s, %v", pluginPublicId, err)
		}
	}

	logger.Log.Infof("插件信任状态已更新: %s trusted=%v", pluginPublicId, trusted)
	return plugin, nil
}

// uninstall 卸载插件核心逻辑
func (s *Service) uninstall(ctx context.Context, pluginPublicId string) error {
	// 停止运行时并删除文件
	if err := s.deactivate(ctx, pluginPublicId, PluginStopOpUninstall); err != nil {
		return err
	}

	// 获取插件记录，捕获卸载前的备份引用（直清目标）
	plugin, err := s.repo.GetByPublicId(ctx, pluginPublicId)
	if err != nil {
		return err
	}
	if plugin == nil {
		return ErrPluginNotFound
	}
	oldBackupID := int64(0)
	if plugin.BackupID.Valid {
		oldBackupID = plugin.BackupID.Int64
	}

	// 标记已卸载并清空备份引用（单条 UPDATE，map 形态零值安全——结构体 Updates 跳过 NULL 列）。
	// backup_id 条件为并发守卫：读取后引用被重装/换版并发改写时条件不中，中止并引导重试
	if err := s.repo.MarkUninstalledAndClearBackup(ctx, pluginPublicId, oldBackupID); err != nil {
		return err
	}

	// 直清安装包备份（备份仅服务已安装版本的重装修复，卸载后无用；失败留无主备份由治理保留期兜底）
	if oldBackupID > 0 {
		if err := s.backupProvider.DeleteBackup(ctx, oldBackupID); err != nil {
			logger.Log.Warnf("卸载后清理安装包备份 %d 失败（留待治理清理）: %v", oldBackupID, err)
		}
	}

	// 清理该插件的更新待办（防卸载后残留可操作项）
	s.removePending(pluginPublicId)

	logger.Log.Infof("插件已卸载: %s", pluginPublicId)
	return nil
}

// deactivate 停止插件运行时并删除插件文件，不修改数据库记录（卸载/重装共用）。
// op 透传给参与者否决检查（卸载/重装/取消信任的拦截标准不同）
func (s *Service) deactivate(ctx context.Context, pluginPublicId string, op PluginStopOp) error {
	plugin, err := s.repo.GetByPublicId(ctx, pluginPublicId)
	if err != nil {
		return err
	}
	if plugin == nil {
		return ErrPluginNotFound
	}

	logger.Log.Infof("正在停用插件: %s (op=%s)", pluginPublicId, op)

	if err := s.stopRuntime(ctx, pluginPublicId, op, false); err != nil {
		return err
	}

	s.removeFiles(plugin)
	return nil
}

// getAppRoot 获取应用根目录，用于插件安装、卸载等程序文件操作
func (s *Service) getAppRoot() string {
	return util.RootPath()
}

// GetPluginRoot 获取插件根目录
func (s *Service) GetPluginRoot() string {
	return PluginPackageRoot
}

// GetPluginStatus 获取插件状态
func (s *Service) GetPluginStatus(ctx context.Context, pluginPublicId string) (*PluginStatusDTO, error) {
	plugin, err := s.repo.GetByPublicId(ctx, pluginPublicId)
	if err != nil {
		return nil, err
	}
	if plugin == nil {
		return nil, ErrPluginNotFound
	}

	status := &PluginStatusDTO{}

	// 运行时状态
	if s.runtimeStatusProvider != nil {
		rt := s.runtimeStatusProvider.GetPluginRuntimeStatus(pluginPublicId)
		status.IsRunning = rt.IsRunning
		status.PID = rt.PID
		if rt.ActivatedAt > 0 {
			status.ActivatedAt = rt.ActivatedAt
		}
	}

	// 扩展点列表
	if s.extensionListProvider != nil {
		for _, ext := range s.extensionListProvider.GetTaskHandlersByPlugin(pluginPublicId) {
			status.TaskHandlers = append(status.TaskHandlers, ExtensionInfo{ID: ext.ID, Name: ext.Name, Description: ext.Description})
		}
		for _, ext := range s.extensionListProvider.GetSiteBrowsersByPlugin(pluginPublicId) {
			status.SiteBrowsers = append(status.SiteBrowsers, ExtensionInfo{ID: ext.ID, Name: ext.Name, Description: ext.Description})
		}
		for _, item := range s.extensionListProvider.GetFrontendExtensionsByPlugin(pluginPublicId) {
			status.FrontendExtensions = append(status.FrontendExtensions, FrontendExtensionInfo{ID: item.ID, Name: item.Name, Kind: item.Kind})
		}
	}

	// URL 监听规则
	if s.urlListenerProvider != nil {
		status.UrlPatterns = s.urlListenerProvider.ListPatternsByPlugin(pluginPublicId)
	}

	return status, nil
}
