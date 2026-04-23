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
	"github.com/library-squirrel/wails/internal/database"
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/logger"
	"github.com/library-squirrel/wails/pkg/model"
	domain "github.com/library-squirrel/wails/pkg/model/dto"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
	"github.com/library-squirrel/wails/pkg/query"
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

// 错误定义
var (
	ErrPluginNotFound      = errors.New("plugin not found")
	ErrPluginAlreadyExists = errors.New("plugin already exists")
	ErrInvalidPackage      = errors.New("invalid plugin package")
	ErrInvalidManifest     = errors.New("invalid plugin manifest")
	ErrBackupNotFound      = errors.New("backup not found")
)

// Repository 插件仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Save 保存
	Save(ctx context.Context, plugin *entity2.Plugin) error
	// SaveBatch 批量保存
	SaveBatch(ctx context.Context, plugins []*entity2.Plugin) error
	// Update 更新
	Update(ctx context.Context, plugin *entity2.Plugin) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity2.Plugin, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*entity2.Plugin, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity2.Plugin, any], error)
	// CheckInstalled 检查插件是否已安装
	CheckInstalled(ctx context.Context, publicId string) (bool, error)
	// GetByPublicId 根据公开ID获取
	GetByPublicId(ctx context.Context, publicId string) (*entity2.Plugin, error)
}

// BackupProvider 备份提供者接口（由 plugin service 定义需要的备份能力）
type BackupProvider interface {
	// CreatePluginBackup 创建插件备份
	CreatePluginBackup(ctx context.Context, sourceId int64, fileName string, sourcePath string, workDir string) (*entity2.Backup, error)
	// GetPluginBackup 获取插件备份
	GetPluginBackup(ctx context.Context, sourceId int64) (*entity2.Backup, error)
}

// Service 插件服务
type Service struct {
	repo           Repository
	backupProvider BackupProvider
}

// NewService 创建插件服务
func NewService(repo Repository, backupProvider BackupProvider) *Service {
	return &Service{
		repo:           repo,
		backupProvider: backupProvider,
	}
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*entity2.Plugin, error) {
	return s.repo.GetById(ctx, id)
}

// Save 保存插件
func (s *Service) Save(ctx context.Context, plugin *entity2.Plugin) error {
	return s.repo.Save(ctx, plugin)
}

// Update 更新插件
func (s *Service) Update(ctx context.Context, plugin *entity2.Plugin) error {
	return s.repo.Update(ctx, plugin)
}

// Delete 删除插件
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// Page 分页查询
// Page 分页查询（基于 QueryDTO）
func (s *Service) Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity2.Plugin, any], error) {
	return s.repo.Page(ctx, opt)
}

// PageByDTO 分页查询（基于 QueryDTO）
func (s *Service) PageByDTO(ctx context.Context, page, pageSize int, queryDTO PluginQueryDTO) (*model.Page[entity2.Plugin, any], error) {
	conv := query.NewConverter(entity2.Plugin{})
	opt, err := conv.ToPageOption(queryDTO, page, pageSize, nil)
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

// InstallFromPath 从插件包路径安装插件
func (s *Service) InstallFromPath(ctx context.Context, packagePath string, installType domain.InstallType) (*entity2.Plugin, error) {
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
func (s *Service) install(ctx context.Context, installDTO *domain.PluginInstallDTO, installType domain.InstallType) (*entity2.Plugin, error) {
	// 检查是否已安装
	existing, err := s.repo.GetByPublicId(ctx, installDTO.PublicID)
	if err != nil {
		return nil, err
	}

	var uninstalledPlugin *entity2.Plugin
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
	plugin := entity2.NewPlugin()
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
func (s *Service) Reinstall(ctx context.Context, pluginPublicId string, installType domain.InstallType) (*entity2.Plugin, error) {
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
func (s *Service) ReinstallFromPath(ctx context.Context, pluginPublicId string, packagePath string, installType domain.InstallType) (*entity2.Plugin, error) {
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
