package plugin

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/library-squirrel/wails/internal/config"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/logger"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm/clause"
)

const (
	// PluginRoot 插件根目录
	PluginRoot = "plugin"
	// PluginPackageRoot 插件包目录
	PluginPackageRoot = "plugin/package"
	// UninstalledTrue 已卸载
	UninstalledTrue = 1
	// UninstalledFalse 未卸载
	UninstalledFalse = 0
)

// ========== 查询 DTO ==========

// PluginQueryDTO 插件查询条件
type PluginQueryDTO struct {
	// 精确查询
	ID             *int64  `json:"-"`              // 插件ID（程序设置，不从JSON解析）
	PublicID       *string `json:"publicId"`       // 公开ID（精确匹配）
	Name           *string `json:"name"`           // 插件名称（精确匹配）
	Author         *string `json:"author"`         // 作者（精确匹配）
	Version        *string `json:"version"`        // 版本号（精确匹配）
	ActivationType *string `json:"activationType"` // 激活类型（精确匹配）
	Uninstalled    *int    `json:"uninstalled"`    // 是否已卸载（0=未卸载，1=已卸载）
	// 模糊查询
	NameLike   *string `json:"nameLike"`   // 插件名称（模糊匹配）
	AuthorLike *string `json:"authorLike"` // 作者（模糊匹配）
	// 排序字段：create_time, update_time, name, author, sort_num
	OrderBy   string `json:"orderBy"`   // 排序字段
	OrderDesc bool   `json:"orderDesc"` // 是否降序
}

// BuildOrderBy 根据查询DTO构建排序条件
func (dto *PluginQueryDTO) BuildOrderBy() clause.Expression {
	column := "sort_num"
	if dto.OrderBy != "" {
		// 支持的排序字段映射
		orderByMap := map[string]string{
			"create_time": "create_time",
			"update_time": "update_time",
			"name":        "name",
			"author":      "author",
			"sort_num":    "sort_num",
		}
		if v, ok := orderByMap[dto.OrderBy]; ok {
			column = v
		}
	}
	return clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: column}, Desc: dto.OrderDesc}}}
}

// 错误定义
var (
	ErrPluginNotFound      = errors.New("plugin not found")
	ErrPluginAlreadyExists = errors.New("plugin already exists")
	ErrInvalidPackage      = errors.New("invalid plugin package")
	ErrInvalidManifest     = errors.New("invalid plugin manifest")
	ErrBackupNotFound      = errors.New("backup not found")
)

// buildConditionsFromDTO 根据查询DTO构建查询条件
func buildConditionsFromDTO(dto *PluginQueryDTO) []clause.Expression {
	var conditions []clause.Expression

	if dto.ID != nil {
		conditions = append(conditions, clause.Eq{Column: "id", Value: *dto.ID})
	}
	if dto.PublicID != nil {
		conditions = append(conditions, clause.Eq{Column: "public_id", Value: *dto.PublicID})
	}
	if dto.Name != nil {
		conditions = append(conditions, clause.Eq{Column: "name", Value: *dto.Name})
	}
	if dto.Author != nil {
		conditions = append(conditions, clause.Eq{Column: "author", Value: *dto.Author})
	}
	if dto.Version != nil {
		conditions = append(conditions, clause.Eq{Column: "version", Value: *dto.Version})
	}
	if dto.ActivationType != nil {
		conditions = append(conditions, clause.Eq{Column: "activation_type", Value: *dto.ActivationType})
	}
	if dto.Uninstalled != nil {
		conditions = append(conditions, clause.Eq{Column: "uninstalled", Value: *dto.Uninstalled})
	}
	if dto.NameLike != nil {
		conditions = append(conditions, clause.Like{Column: "name", Value: *dto.NameLike})
	}
	if dto.AuthorLike != nil {
		conditions = append(conditions, clause.Like{Column: "author", Value: *dto.AuthorLike})
	}

	return conditions
}

// combineConditions 将多个条件组合成单个表达式
func combineConditions(conditions []clause.Expression) clause.Expression {
	if len(conditions) == 0 {
		return nil
	}
	if len(conditions) == 1 {
		return conditions[0]
	}
	result := clause.AndConditions{}
	for _, cond := range conditions {
		result.Exprs = append(result.Exprs, cond)
	}
	return result
}

// Repository 插件仓储接口（由 service 定义需要的数据库操作方法）
// 注意：只定义 service 真正需要的方法，遵循最小依赖原则
type Repository interface {
	// Save 保存
	Save(ctx context.Context, plugin *domain.Plugin) error
	// SaveBatch 批量保存
	SaveBatch(ctx context.Context, plugins []*domain.Plugin) error
	// Update 更新
	Update(ctx context.Context, plugin *domain.Plugin) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.Plugin, error)
	// List 查询列表
	List(ctx context.Context, conditions []clause.Expression, orderBy clause.Expression, limit, offset int) ([]*domain.Plugin, error)
	// Count 统计数量
	Count(ctx context.Context, conditions []clause.Expression) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, page, pageSize int, conditions []clause.Expression, orderBy clause.Expression) (*model.Page[domain.Plugin], error)
	// CheckInstalled 检查插件是否已安装
	CheckInstalled(ctx context.Context, publicId string) (bool, error)
	// GetByPublicId 根据公开ID获取
	GetByPublicId(ctx context.Context, publicId string) (*domain.Plugin, error)
}

// BackupProvider 备份提供者接口（由 plugin service 定义需要的备份能力）
type BackupProvider interface {
	// CreatePluginBackup 创建插件备份
	CreatePluginBackup(ctx context.Context, sourceId int64, fileName string, sourcePath string, workDir string) (*domain.Backup, error)
	// GetPluginBackup 获取插件备份
	GetPluginBackup(ctx context.Context, sourceId int64) (*domain.Backup, error)
}

// Service 插件服务
type Service struct {
	repo           Repository
	backupProvider BackupProvider
	loader         *Loader
}

// NewService 创建插件服务
func NewService(repo Repository, backupProvider BackupProvider, loader *Loader) *Service {
	return &Service{
		repo:           repo,
		backupProvider: backupProvider,
		loader:         loader,
	}
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*domain.Plugin, error) {
	return s.repo.GetById(ctx, id)
}

// Save 保存插件
func (s *Service) Save(ctx context.Context, plugin *domain.Plugin) error {
	return s.repo.Save(ctx, plugin)
}

// Update 更新插件
func (s *Service) Update(ctx context.Context, plugin *domain.Plugin) error {
	return s.repo.Update(ctx, plugin)
}

// Delete 删除插件
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// Page 分页查询
// Page 分页查询（基于 QueryDTO）
func (s *Service) Page(ctx context.Context, page, pageSize int, queryDTO PluginQueryDTO) (*model.Page[domain.Plugin], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.Page(ctx, page, pageSize, conditions, orderBy)
}

// List 查询列表
func (s *Service) List(ctx context.Context, where clause.Expression, order clause.Expression, limit, offset int) ([]*domain.Plugin, error) {
	var conditions []clause.Expression
	if where != nil {
		conditions = []clause.Expression{where}
	}
	return s.repo.List(ctx, conditions, order, limit, offset)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, where clause.Expression) (int64, error) {
	var conditions []clause.Expression
	if where != nil {
		conditions = []clause.Expression{where}
	}
	return s.repo.Count(ctx, conditions)
}

// CheckInstalled 检查插件是否已安装
func (s *Service) CheckInstalled(ctx context.Context, publicId string) (bool, error) {
	return s.repo.CheckInstalled(ctx, publicId)
}

// GetByPublicId 根据公开ID获取插件
func (s *Service) GetByPublicId(ctx context.Context, publicId string) (*domain.Plugin, error) {
	return s.repo.GetByPublicId(ctx, publicId)
}

// InstallFromPath 从插件包路径安装插件
func (s *Service) InstallFromPath(ctx context.Context, packagePath string, installType domain.InstallType) (*domain.Plugin, error) {
	// 验证文件存在
	if !util.FileExists(packagePath) {
		return nil, fmt.Errorf("plugin package not found: %s", packagePath)
	}

	// 加载插件包
	installDTO, err := s.loadPluginPackage(packagePath)
	if err != nil {
		return nil, err
	}

	return s.install(ctx, installDTO, installType)
}

// loadPluginPackage 加载插件安装包
func (s *Service) loadPluginPackage(packagePath string) (*domain.PluginInstallDTO, error) {
	// 打开 ZIP 文件
	reader, err := zip.OpenReader(packagePath)
	if err != nil {
		return nil, ErrInvalidPackage
	}
	defer reader.Close()

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

	// 验证必要字段
	if manifest.ID == "" || manifest.Name == "" || manifest.Version == "" || manifest.Author == "" {
		return nil, ErrInvalidManifest
	}
	if len(manifest.Contributes) == 0 {
		return nil, ErrInvalidManifest
	}
	if manifest.Activation.Type == 0 && manifest.EntryFile == "" {
		return nil, ErrInvalidManifest
	}

	// 构建安装 DTO
	installDTO := manifest.ToPluginInstallDTO(packagePath)
	return installDTO, nil
}

// install 安装插件核心逻辑
func (s *Service) install(ctx context.Context, installDTO *domain.PluginInstallDTO, installType domain.InstallType) (*domain.Plugin, error) {
	// 检查是否已安装
	existing, err := s.repo.GetByPublicId(ctx, installDTO.PublicID)
	if err != nil {
		return nil, err
	}

	var uninstalledPlugin *domain.Plugin
	if existing != nil {
		if existing.Uninstalled.Valid && existing.Uninstalled.Int64 == UninstalledFalse {
			return nil, ErrPluginAlreadyExists
		}
		uninstalledPlugin = existing
	}

	// 获取工作目录
	workDir := s.getWorkDir()
	if workDir == "" {
		workDir = "."
	}

	// 创建备份
	backup, err := s.backupProvider.CreatePluginBackup(ctx, 0, filepath.Base(installDTO.PackagePath), installDTO.PackagePath, workDir)
	if err != nil {
		return nil, err
	}

	// 构建安装路径
	pathRelative := filepath.Join(installDTO.PublicID, installDTO.Version)
	installPath := filepath.Join(workDir, PluginPackageRoot, pathRelative)

	// 创建目录并解压
	if err := util.CreateDirIfNotExists(installPath); err != nil {
		return nil, err
	}
	if err := util.ExtractZip(installDTO.PackagePath, installPath); err != nil {
		return nil, err
	}

	// 构建插件记录
	plugin := domain.NewPlugin()
	plugin.PublicID = sql.NullString{String: installDTO.PublicID, Valid: true}
	plugin.Author = sql.NullString{String: installDTO.Author, Valid: true}
	plugin.Name = sql.NullString{String: installDTO.Name, Valid: true}
	plugin.Version = sql.NullString{String: installDTO.Version, Valid: true}
	plugin.EntryPath = sql.NullString{String: filepath.Join(PluginPackageRoot, pathRelative, installDTO.EntryFile), Valid: true}
	plugin.RootPath = sql.NullString{String: filepath.Join(PluginPackageRoot, pathRelative), Valid: true}
	plugin.BackupID = sql.NullInt64{Int64: backup.ID, Valid: true}
	plugin.ActivationType = sql.NullString{String: string(rune(installDTO.Activation.Type + '0')), Valid: true}

	if uninstalledPlugin != nil {
		// 更新已卸载的插件
		plugin.ID = uninstalledPlugin.ID
		plugin.PluginData = uninstalledPlugin.PluginData
		plugin.Uninstalled = sql.NullInt64{Int64: UninstalledFalse, Valid: true}
		if err := s.repo.Update(ctx, plugin); err != nil {
			return nil, err
		}
	} else {
		// 保存新插件
		if err := s.repo.Save(ctx, plugin); err != nil {
			return nil, err
		}
	}

	logger.Log.Infof("Plugin installed: %s/%s-%s", installDTO.Author, installDTO.Name, installDTO.Version)
	return plugin, nil
}

// Reinstall 重新安装插件
func (s *Service) Reinstall(ctx context.Context, pluginPublicId string, installType domain.InstallType) (*domain.Plugin, error) {
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

	// 获取备份
	backup, err := s.backupProvider.GetPluginBackup(ctx, plugin.ID)
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

	return s.ReinstallFromPath(ctx, pluginPublicId, packagePath, installType)
}

// ReinstallFromPath 从指定路径重新安装插件
func (s *Service) ReinstallFromPath(ctx context.Context, pluginPublicId string, packagePath string, installType domain.InstallType) (*domain.Plugin, error) {
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

	// 卸载旧插件
	if err := s.uninstall(ctx, pluginPublicId); err != nil {
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

	// 重新安装
	return s.install(ctx, installDTO, installType)
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

// uninstall 卸载插件核心逻辑
func (s *Service) uninstall(ctx context.Context, pluginPublicId string) error {
	// 获取插件
	plugin, err := s.repo.GetByPublicId(ctx, pluginPublicId)
	if err != nil {
		return err
	}
	if plugin == nil {
		return ErrPluginNotFound
	}

	// 删除插件目录
	workDir := s.getWorkDir()
	if workDir == "" {
		workDir = "."
	}
	rootPath := ""
	if plugin.RootPath.Valid {
		rootPath = plugin.RootPath.String
	}
	pluginPath := filepath.Join(workDir, rootPath)
	if err := util.RemoveDir(pluginPath); err != nil {
		logger.Log.Warnf("Failed to remove plugin directory: %v", err)
	}

	// 设置为已卸载状态
	plugin.Uninstalled = sql.NullInt64{Int64: UninstalledTrue, Valid: true}
	if err := s.repo.Update(ctx, plugin); err != nil {
		return err
	}

	logger.Log.Infof("Plugin uninstalled: %s", pluginPublicId)
	return nil
}

// SetUninstalled 设置插件为已卸载状态
func (s *Service) SetUninstalled(ctx context.Context, pluginId int64) error {
	plugin, err := s.repo.GetById(ctx, pluginId)
	if err != nil {
		return err
	}
	if plugin == nil {
		return ErrPluginNotFound
	}

	plugin.Uninstalled = sql.NullInt64{Int64: UninstalledTrue, Valid: true}
	return s.repo.Update(ctx, plugin)
}

// getWorkDir 获取工作目录
func (s *Service) getWorkDir() string {
	cfg := config.Get()
	if cfg != nil && cfg.App.DataPath != "" {
		return cfg.App.DataPath
	}
	return "."
}

// GetPluginRoot 获取插件根目录
func (s *Service) GetPluginRoot() string {
	return PluginPackageRoot
}

// ReadVueFile 读取插件的 Vue 文件内容
func (s *Service) ReadVueFile(pluginPublicId string, filePath string) (string, error) {
	// 获取插件信息
	plugin, err := s.repo.GetByPublicId(context.Background(), pluginPublicId)
	if err != nil {
		return "", fmt.Errorf("failed to get plugin: %w", err)
	}
	if plugin == nil {
		return "", ErrPluginNotFound
	}

	// 构建文件完整路径
	workDir := s.getWorkDir()
	rootPath := ""
	if plugin.RootPath.Valid {
		rootPath = plugin.RootPath.String
	}
	fullPath := filepath.Join(workDir, rootPath, filePath)

	// 读取文件
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}
