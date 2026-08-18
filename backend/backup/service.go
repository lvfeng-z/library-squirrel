package backup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/storeRegistry"
	"github.com/library-squirrel/backend/util"
)

const (
	// SourceTypePlugin 插件备份
	SourceTypePlugin = 1
	// SourceTypeResource 资源备份
	SourceTypeResource = 2
	// SourceTypePersistentStore PersistentStore 备份
	SourceTypePersistentStore = 3
	// BackupRootDirName 备份根目录名
	BackupRootDirName = "backup"
)

// Repository 备份仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建备份
	Create(ctx context.Context, backup *entity.Backup) error
	// Updates 更新备份
	Updates(ctx context.Context, backup *entity.Backup) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity.Backup, error)
	// GetBySourceTypeAndSourceId 根据来源类型和来源ID获取
	GetBySourceTypeAndSourceId(ctx context.Context, sourceType int, sourceId int64) (*entity.Backup, error)
	// GetBySourceTypeAndSourceIds 批量获取备份记录
	GetBySourceTypeAndSourceIds(ctx context.Context, sourceType int, sourceIds []int64) ([]*entity.Backup, error)
	// Delete 删除备份记录
	Delete(ctx context.Context, id int64) error
}

// Service 备份服务
type Service struct {
	repo          Repository
	workDirGetter func() string // 每次调用获取最新的 workDir（从设置管理器读取）
}

// NewService 创建备份服务
func NewService(repo Repository, workDirGetter func() string) *Service {
	return &Service{repo: repo, workDirGetter: workDirGetter}
}

// getWorkDir 获取当前 workDir（每次从设置管理器读取最新值）
func (s *Service) getWorkDir() string {
	return s.workDirGetter()
}

// CreateBackup 创建备份，将源文件复制到 workdir/backup/YYYY/MM/DD/ 下
func (s *Service) CreateBackup(ctx context.Context, sourceType int, sourceId int64, sourcePath string) (*entity.Backup, error) {
	if !util.FileExists(sourcePath) {
		return nil, fmt.Errorf("创建备份失败，源文件不存在: %s", sourcePath)
	}

	workDir := s.getWorkDir()
	fileName := filepath.Base(sourcePath)

	// 按日期构建备份目录：backup/YYYY/MM/DD/
	now := time.Now()
	relativeDir := filepath.Join(
		BackupRootDirName,
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", now.Month()),
		fmt.Sprintf("%02d", now.Day()),
	)
	absoluteDir := filepath.Join(workDir, relativeDir)
	if err := util.CreateDirIfNotExists(absoluteDir); err != nil {
		return nil, err
	}

	// 处理文件名冲突
	finalFileName := fileName
	finalAbsolutePath := filepath.Join(absoluteDir, finalFileName)
	maxRetries := 50
	for util.FileExists(finalAbsolutePath) {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("创建备份失败，上下文已取消: %w", ctx.Err())
		}
		if maxRetries <= 0 {
			return nil, fmt.Errorf("创建备份失败，文件名冲突重试次数超限: %s", fileName)
		}
		maxRetries--
		finalFileName = addSuffix(finalFileName, fmt.Sprintf("_%d", util.GetCurrentTimestamp()))
		finalAbsolutePath = filepath.Join(absoluteDir, finalFileName)
		logger.Log.Infof("文件已存在，尝试文件名: %s", finalFileName)
	}

	// 复制源文件到备份目录
	if err := util.CopyFile(sourcePath, finalAbsolutePath); err != nil {
		return nil, fmt.Errorf("创建备份失败，复制文件出错: %w", err)
	}

	// 保存备份记录，file_path 存储相对路径
	backup := entity.NewBackup()
	backup.SourceType = sql.NullInt64{Int64: int64(sourceType), Valid: true}
	backup.SourceID = sql.NullInt64{Int64: sourceId, Valid: true}
	backup.FileName = sql.NullString{String: finalFileName, Valid: true}
	backup.FilePath = sql.NullString{String: filepath.Join(relativeDir, finalFileName), Valid: true}
	backup.Workdir = sql.NullString{String: workDir, Valid: true}
	if err := s.repo.Create(ctx, backup); err != nil {
		return nil, err
	}
	return backup, nil
}

// MoveBackup 移动备份，将源文件移动到 workdir/backup/YYYY/MM/DD/ 下（O(1) 同文件系统）
// 当调用方不需要保留源文件时使用，比 CreateBackup（复制）更高效
func (s *Service) MoveBackup(ctx context.Context, sourceType int, sourceId int64, sourcePath string) (*entity.Backup, error) {
	if !util.FileExists(sourcePath) {
		return nil, fmt.Errorf("移动备份失败，源文件不存在: %s", sourcePath)
	}

	workDir := s.getWorkDir()
	fileName := filepath.Base(sourcePath)

	// 源文件移出扫描目录会触发旧路径 Remove 事件（跨目录 rename 旧路径事件不被
	// fsnotify 吞掉），在文件操作点登记抑制，避免 fsmonitor 将内部移动误报为外部
	// 删除。源路径不在 workDir 内（Rel 逃逸）时不登记——监控树外路径本就无事件。
	if rel, err := filepath.Rel(workDir, sourcePath); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		storeRegistry.Suppress(rel)
		defer storeRegistry.Release(rel)
	}

	// 按日期构建备份目录：backup/YYYY/MM/DD/
	now := time.Now()
	relativeDir := filepath.Join(
		BackupRootDirName,
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", now.Month()),
		fmt.Sprintf("%02d", now.Day()),
	)
	absoluteDir := filepath.Join(workDir, relativeDir)
	if err := util.CreateDirIfNotExists(absoluteDir); err != nil {
		return nil, err
	}

	// 处理文件名冲突
	finalFileName := fileName
	finalAbsolutePath := filepath.Join(absoluteDir, finalFileName)
	maxRetries := 50
	for util.FileExists(finalAbsolutePath) {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("移动备份失败，上下文已取消: %w", ctx.Err())
		}
		if maxRetries <= 0 {
			return nil, fmt.Errorf("移动备份失败，文件名冲突重试次数超限: %s", fileName)
		}
		maxRetries--
		finalFileName = addSuffix(finalFileName, fmt.Sprintf("_%d", util.GetCurrentTimestamp()))
		finalAbsolutePath = filepath.Join(absoluteDir, finalFileName)
		logger.Log.Infof("文件已存在，尝试文件名: %s", finalFileName)
	}

	// 移动源文件到备份目录（同文件系统下 O(1)）
	if err := os.Rename(sourcePath, finalAbsolutePath); err != nil {
		// 跨文件系统时回退为复制
		logger.Log.Warnf("移动备份失败（回退为复制）: %v", err)
		if copyErr := util.CopyFile(sourcePath, finalAbsolutePath); copyErr != nil {
			return nil, fmt.Errorf("移动备份失败，回退复制也失败: %w（原始移动错误: %v）", copyErr, err)
		}
		// 复制成功后删除源文件
		_ = os.Remove(sourcePath)
	}

	// 保存备份记录，file_path 存储相对路径
	backup := entity.NewBackup()
	backup.SourceType = sql.NullInt64{Int64: int64(sourceType), Valid: true}
	backup.SourceID = sql.NullInt64{Int64: sourceId, Valid: true}
	backup.FileName = sql.NullString{String: finalFileName, Valid: true}
	backup.FilePath = sql.NullString{String: filepath.Join(relativeDir, finalFileName), Valid: true}
	backup.Workdir = sql.NullString{String: workDir, Valid: true}
	if err := s.repo.Create(ctx, backup); err != nil {
		return nil, err
	}
	return backup, nil
}

// CreatePluginBackup 创建插件备份
func (s *Service) CreatePluginBackup(ctx context.Context, sourceId int64, sourcePath string) (*entity.Backup, error) {
	return s.CreateBackup(ctx, SourceTypePlugin, sourceId, sourcePath)
}

// GetById 根据ID获取备份
func (s *Service) GetById(ctx context.Context, id int64) (*entity.Backup, error) {
	return s.repo.GetById(ctx, id)
}

// GetPluginBackup 获取插件备份
func (s *Service) GetPluginBackup(ctx context.Context, sourceId int64) (*entity.Backup, error) {
	return s.repo.GetBySourceTypeAndSourceId(ctx, SourceTypePlugin, sourceId)
}

// GetResourceBackup 获取资源备份
func (s *Service) GetResourceBackup(ctx context.Context, resourceId int64) (*entity.Backup, error) {
	return s.repo.GetBySourceTypeAndSourceId(ctx, SourceTypeResource, resourceId)
}

// GetBackupPath 获取备份文件的完整路径
func (s *Service) GetBackupPath(backup *entity.Backup) string {
	var workdir, filePath string
	if backup.Workdir.Valid {
		workdir = backup.Workdir.String
	}
	if backup.FilePath.Valid {
		filePath = backup.FilePath.String
	}
	return filepath.Join(workdir, filePath)
}

// addSuffix 在文件名（不含扩展名）后添加后缀，保留扩展名
func addSuffix(filename string, suffix string) string {
	ext := filepath.Ext(filename)
	name := filename[:len(filename)-len(ext)]
	return name + suffix + ext
}

// MoveBackupForResource 移动资源文件到备份目录，并记录原始路径信息用于后续还原
func (s *Service) MoveBackupForResource(ctx context.Context, sourceId int64, sourcePath string, originalFilePath string, originalFileName string, originalFilenameExtension string) (*entity.Backup, error) {
	backup, err := s.MoveBackup(ctx, SourceTypeResource, sourceId, sourcePath)
	if err != nil {
		return nil, err
	}
	// 补充原始资源路径信息
	backup.OriginalFilePath = sql.NullString{String: originalFilePath, Valid: originalFilePath != ""}
	backup.OriginalFileName = sql.NullString{String: originalFileName, Valid: originalFileName != ""}
	backup.OriginalFilenameExtension = sql.NullString{String: originalFilenameExtension, Valid: originalFilenameExtension != ""}
	if err := s.repo.Updates(ctx, backup); err != nil {
		return nil, fmt.Errorf("更新备份原始路径失败: %w", err)
	}
	return backup, nil
}

// MoveToBackup 将文件移动到备份目录并创建备份记录，供 PersistentStore 删除时调用
// sourceId: PersistentStore 记录 ID
// absFilePath: 源文件绝对路径
// originalFilePath: PersistentStore 中的相对路径（用于还原时确定目标位置）
// originalFileName: 原始文件名
// originalFilenameExtension: 原始扩展名
// 返回备份记录 ID
func (s *Service) MoveToBackup(ctx context.Context, sourceId int64, absFilePath string, originalFilePath string, originalFileName string, originalFilenameExtension string) (int64, error) {
	backup, err := s.MoveBackup(ctx, SourceTypePersistentStore, sourceId, absFilePath)
	if err != nil {
		return 0, err
	}
	// 记录 PersistentStore 的原始路径信息，用于还原
	backup.OriginalFilePath = sql.NullString{String: originalFilePath, Valid: originalFilePath != ""}
	backup.OriginalFileName = sql.NullString{String: originalFileName, Valid: originalFileName != ""}
	backup.OriginalFilenameExtension = sql.NullString{String: originalFilenameExtension, Valid: originalFilenameExtension != ""}
	if err := s.repo.Updates(ctx, backup); err != nil {
		return 0, fmt.Errorf("更新备份原始路径失败: %w", err)
	}
	return backup.GetID(), nil
}

// RestoreFile 从备份路径还原文件到目标路径
//
// TODO(suppression): 当前无调用方（活跃还原走 persistentStore.StoreFromExternal）。
// 若未来接入且 targetPath 落在 store/ 白名单内，须在 os.Rename/os.Remove 前后
// storeRegistry.Suppress/Release(targetPath)，避免还原写入被 fsmonitor 误报为外部变更。
func (s *Service) RestoreFile(ctx context.Context, backupPath string, targetPath string) error {
	if !util.FileExists(backupPath) {
		return fmt.Errorf("还原失败，备份文件不存在: %s", backupPath)
	}
	// 确保目标目录存在
	if err := util.CreateDirIfNotExists(filepath.Dir(targetPath)); err != nil {
		return fmt.Errorf("还原失败，创建目标目录出错: %w", err)
	}
	// 目标文件已存在时（如新下载的部分文件），先删除
	if util.FileExists(targetPath) {
		if err := os.Remove(targetPath); err != nil {
			return fmt.Errorf("还原失败，无法删除已存在的目标文件: %w", err)
		}
	}
	// 移动文件（同文件系统 O(1)）
	if err := os.Rename(backupPath, targetPath); err != nil {
		// 跨文件系统回退为复制
		logger.Log.Warnf("还原文件移动失败（回退为复制）: %v", err)
		if copyErr := util.CopyFile(backupPath, targetPath); copyErr != nil {
			return fmt.Errorf("还原失败，回退复制也失败: %w（原始移动错误: %v）", copyErr, err)
		}
		_ = os.Remove(backupPath)
	}
	return nil
}

// GetResourceBackups 批量获取资源备份记录
func (s *Service) GetResourceBackups(ctx context.Context, resourceIds []int64) ([]*entity.Backup, error) {
	return s.repo.GetBySourceTypeAndSourceIds(ctx, SourceTypeResource, resourceIds)
}

// Delete 删除备份记录
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
