package backup

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// ResourceProvider 资源操作接口（由调用方定义，resource.Service 实现）
type ResourceProvider interface {
	// GetEnabledByWorkId 查询作品关联的启用资源
	GetEnabledByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error)
	// GetById 根据 ID 获取资源
	GetById(ctx context.Context, id int64) (*entity.Resource, error)
	// Update 更新资源
	Update(ctx context.Context, resource *entity.Resource) error
}

// BackupFileProvider 备份文件操作接口（由 backup.Service 实现）
type BackupFileProvider interface {
	// MoveBackupForResource 移动资源文件到备份目录并记录原始路径
	MoveBackupForResource(ctx context.Context, sourceId int64, fileName string, sourcePath string, workDir string, originalFilePath string, originalFileName string, originalFilenameExtension string) (*entity.Backup, error)
	// GetResourceBackup 获取资源备份记录
	GetResourceBackup(ctx context.Context, resourceId int64) (*entity.Backup, error)
	// GetResourceBackups 批量获取资源备份记录
	GetResourceBackups(ctx context.Context, resourceIds []int64) ([]*entity.Backup, error)
	// RestoreFile 从备份路径还原文件到目标路径
	RestoreFile(ctx context.Context, backupPath string, targetPath string) error
	// Delete 删除备份记录
	Delete(ctx context.Context, id int64) error
	// GetBackupPath 获取备份文件的完整路径
	GetBackupPath(backup *entity.Backup) string
}

// ResourceBackupOrchestrator 资源备份编排器
// 封装资源备份/禁用/还原的完整生命周期，独立于任务执行逻辑
type ResourceBackupOrchestrator struct {
	resourceProvider ResourceProvider
	backupProvider   BackupFileProvider
}

// NewResourceBackupOrchestrator 创建资源备份编排器
func NewResourceBackupOrchestrator(resourceProvider ResourceProvider, backupProvider BackupFileProvider) *ResourceBackupOrchestrator {
	return &ResourceBackupOrchestrator{
		resourceProvider: resourceProvider,
		backupProvider:   backupProvider,
	}
}

// BackupAndDisable 备份已有作品的启用资源并标记为未启用
// 返回已备份的资源 ID 列表（用于后续还原）
func (o *ResourceBackupOrchestrator) BackupAndDisable(ctx context.Context, workId int64, workDir string) []int64 {
	resources, err := o.resourceProvider.GetEnabledByWorkId(ctx, workId)
	if err != nil {
		logger.Log.Warnf("[ResourceBackupOrchestrator] 查询作品 %d 启用资源失败（跳过备份）: %v", workId, err)
		return nil
	}

	var backedUpIds []int64
	for _, res := range resources {
		// 路径无效时跳过
		if !res.FilePath.Valid || !res.FileName.Valid {
			continue
		}

		resourceId := res.GetID()
		absPath := filepath.Join(workDir, "resource", res.FilePath.String)

		// 源文件不存在时跳过
		if !util.FileExists(absPath) {
			continue
		}

		// 已有有效备份时仅禁用，不重复移动文件
		if o.shouldSkipBackup(ctx, resourceId, absPath, workDir) {
			o.disableResource(ctx, res)
			backedUpIds = append(backedUpIds, resourceId)
			continue
		}

		// 记录原始路径用于还原
		originalFilePath := res.FilePath.String
		originalFileName := res.FileName.String
		originalFilenameExtension := ""
		if res.FilenameExtension.Valid {
			originalFilenameExtension = res.FilenameExtension.String
		}

		// 移动文件到备份目录并记录原始路径
		_, err := o.backupProvider.MoveBackupForResource(ctx, resourceId, res.FileName.String, absPath, workDir, originalFilePath, originalFileName, originalFilenameExtension)
		if err != nil {
			logger.Log.Warnf("[ResourceBackupOrchestrator] 移动备份资源文件失败 [%s]: %v", absPath, err)
			continue
		}

		o.disableResource(ctx, res)
		backedUpIds = append(backedUpIds, resourceId)
	}
	return backedUpIds
}

// Restore 还原已备份的资源
// 遍历 resourceIds，从备份记录还原文件到原始路径，重新启用资源
func (o *ResourceBackupOrchestrator) Restore(ctx context.Context, resourceIds []int64, workDir string) {
	if len(resourceIds) == 0 {
		return
	}

	logger.Log.Infof("[ResourceBackupOrchestrator] 开始还原 %d 个资源", len(resourceIds))

	backups, err := o.backupProvider.GetResourceBackups(ctx, resourceIds)
	if err != nil {
		logger.Log.Errorf("[ResourceBackupOrchestrator] 查询备份记录失败: %v", err)
		return
	}

	// 按 source_id 建立索引（取最新一条）
	backupMap := make(map[int64]*entity.Backup)
	for _, b := range backups {
		if b.SourceID.Valid {
			backupMap[b.SourceID.Int64] = b
		}
	}

	var restoredCount int
	for _, resourceId := range resourceIds {
		backup, ok := backupMap[resourceId]
		if !ok {
			logger.Log.Warnf("[ResourceBackupOrchestrator] 资源 %d 无备份记录，跳过还原", resourceId)
			continue
		}

		if err := o.restoreResource(ctx, resourceId, backup, workDir); err != nil {
			logger.Log.Errorf("[ResourceBackupOrchestrator] 还原资源 %d 失败: %v", resourceId, err)
			continue
		}
		restoredCount++
	}

	logger.Log.Infof("[ResourceBackupOrchestrator] 还原完成: %d/%d 个资源已还原", restoredCount, len(resourceIds))
}

// restoreResource 还原单个资源：移动备份文件回原始路径，重新启用资源记录
func (o *ResourceBackupOrchestrator) restoreResource(ctx context.Context, resourceId int64, backup *entity.Backup, workDir string) error {
	res, err := o.resourceProvider.GetById(ctx, resourceId)
	if err != nil || res == nil {
		return fmt.Errorf("查询资源 %d 失败: %w", resourceId, err)
	}

	// 确定还原目标路径
	if !backup.OriginalFilePath.Valid || backup.OriginalFilePath.String == "" {
		logger.Log.Warnf("[ResourceBackupOrchestrator] 资源 %d 的备份记录缺少原始路径信息，跳过文件还原", resourceId)
		return nil
	}

	targetAbsPath := filepath.Join(workDir, "resource", backup.OriginalFilePath.String)
	backupAbsPath := o.backupProvider.GetBackupPath(backup)

	// 移动备份文件回原始位置
	if err := o.backupProvider.RestoreFile(ctx, backupAbsPath, targetAbsPath); err != nil {
		return fmt.Errorf("还原文件失败: %w", err)
	}

	// 重新启用资源并恢复原始路径字段
	res.Enabled = true
	res.FilePath = backup.OriginalFilePath
	res.FileName = backup.OriginalFileName
	res.FilenameExtension = backup.OriginalFilenameExtension
	if err := o.resourceProvider.Update(ctx, res); err != nil {
		return fmt.Errorf("重新启用资源 %d 失败: %w", resourceId, err)
	}

	// 删除备份记录
	if err := o.backupProvider.Delete(ctx, backup.GetID()); err != nil {
		logger.Log.Warnf("[ResourceBackupOrchestrator] 删除备份记录 %d 失败: %v", backup.GetID(), err)
	}

	return nil
}

// shouldSkipBackup 检查资源是否已有有效备份（记录存在 + 备份文件存在 + 文件大小一致）
func (o *ResourceBackupOrchestrator) shouldSkipBackup(ctx context.Context, resourceId int64, sourcePath string, workDir string) bool {
	existing, err := o.backupProvider.GetResourceBackup(ctx, resourceId)
	if err != nil || existing == nil || !existing.FilePath.Valid {
		return false
	}
	backupAbsPath := o.backupProvider.GetBackupPath(existing)
	if !util.FileExists(backupAbsPath) || !util.FileExists(sourcePath) {
		return false
	}
	sourceSize, _ := util.GetFileSize(sourcePath)
	backupSize, _ := util.GetFileSize(backupAbsPath)
	if sourceSize > 0 && sourceSize == backupSize {
		logger.Log.Infof("[ResourceBackupOrchestrator] 资源 %d 已有有效备份，跳过（源: %d 字节, 备份: %d 字节）", resourceId, sourceSize, backupSize)
		return true
	}
	return false
}

// disableResource 将资源标记为未启用，清空文件路径字段
func (o *ResourceBackupOrchestrator) disableResource(ctx context.Context, res *entity.Resource) {
	res.Enabled = false
	res.FilePath = sql.NullString{}
	res.FileName = sql.NullString{}
	res.FilenameExtension = sql.NullString{}
	if err := o.resourceProvider.Update(ctx, res); err != nil {
		logger.Log.Warnf("[ResourceBackupOrchestrator] 标记资源 %d 为未启用失败: %v", res.GetID(), err)
	}
}
