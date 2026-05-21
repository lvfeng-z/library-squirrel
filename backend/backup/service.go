package backup

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

const (
	// SourceTypePlugin 插件备份
	SourceTypePlugin = 1
	// BackupRootDirName 备份根目录名
	BackupRootDirName = "backup"
)

// Repository 备份仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Save 保存备份
	Save(ctx context.Context, backup *entity.Backup) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity.Backup, error)
	// GetBySourceTypeAndSourceId 根据来源类型和来源ID获取
	GetBySourceTypeAndSourceId(ctx context.Context, sourceType int, sourceId int64) (*entity.Backup, error)
}

// Service 备份服务
type Service struct {
	repo Repository
}

// NewService 创建备份服务
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateBackup 创建备份，将源文件复制到 workdir/backup/YYYY/MM/DD/ 下
func (s *Service) CreateBackup(ctx context.Context, sourceType int, sourceId int64, fileName string, sourcePath string, workDir string) (*entity.Backup, error) {
	if !util.FileExists(sourcePath) {
		return nil, fmt.Errorf("创建备份失败，源文件不存在: %s", sourcePath)
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
	for util.FileExists(finalAbsolutePath) {
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
	if err := s.repo.Save(ctx, backup); err != nil {
		return nil, err
	}
	return backup, nil
}

// CreatePluginBackup 创建插件备份
func (s *Service) CreatePluginBackup(ctx context.Context, sourceId int64, fileName string, sourcePath string, workDir string) (*entity.Backup, error) {
	return s.CreateBackup(ctx, SourceTypePlugin, sourceId, fileName, sourcePath, workDir)
}

// GetById 根据ID获取备份
func (s *Service) GetById(ctx context.Context, id int64) (*entity.Backup, error) {
	return s.repo.GetById(ctx, id)
}

// GetPluginBackup 获取插件备份
func (s *Service) GetPluginBackup(ctx context.Context, sourceId int64) (*entity.Backup, error) {
	return s.repo.GetBySourceTypeAndSourceId(ctx, SourceTypePlugin, sourceId)
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
