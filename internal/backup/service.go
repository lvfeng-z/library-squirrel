package backup

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/internal/util"
)

const (
	// SourceTypePlugin 插件备份
	SourceTypePlugin = 1
)

// Service 备份服务
type Service struct {
	repo Repository
}

// NewService 创建备份服务
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateBackup 创建备份
func (s *Service) CreateBackup(ctx context.Context, sourceType int, sourceId int64, fileName string, sourcePath string, workDir string) (*model.Backup, error) {
	backup := model.NewBackup()
	backup.SourceType = sql.NullInt64{Int64: int64(sourceType), Valid: true}
	backup.SourceID = sql.NullInt64{Int64: sourceId, Valid: true}
	backup.FileName = sql.NullString{String: fileName, Valid: true}
	backup.FilePath = sql.NullString{String: sourcePath, Valid: true}
	backup.Workdir = sql.NullString{String: workDir, Valid: true}
	if err := s.repo.Save(ctx, backup); err != nil {
		return nil, err
	}
	return backup, nil
}

// CreatePluginBackup 创建插件备份
func (s *Service) CreatePluginBackup(ctx context.Context, sourceId int64, fileName string, sourcePath string, workDir string) (*model.Backup, error) {
	return s.CreateBackup(ctx, SourceTypePlugin, sourceId, fileName, sourcePath, workDir)
}

// GetById 根据ID获取备份
func (s *Service) GetById(ctx context.Context, id int64) (*model.Backup, error) {
	return s.repo.GetById(ctx, id)
}

// GetPluginBackup 获取插件备份
func (s *Service) GetPluginBackup(ctx context.Context, sourceId int64) (*model.Backup, error) {
	return s.repo.GetBySourceTypeAndSourceId(ctx, SourceTypePlugin, sourceId)
}

// GetBackupPath 获取备份文件的完整路径
func (s *Service) GetBackupPath(backup *model.Backup) string {
	var workdir, filePath string
	if backup.Workdir.Valid {
		workdir = backup.Workdir.String
	}
	if backup.FilePath.Valid {
		filePath = backup.FilePath.String
	}
	return filepath.Join(workdir, filePath)
}

// GenBackupFilePath 生成本次备份的文件路径
func GenBackupFilePath(sourceType int, fileName string) string {
	now := util.GetCurrentTimestamp()
	return fmt.Sprintf("backup/%d/%d_%s", sourceType, now, fileName)
}
