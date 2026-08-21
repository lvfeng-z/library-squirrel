package backup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/storeRegistry"
	"github.com/library-squirrel/backend/util"
)

// BackupRootDirName 备份根目录名
const BackupRootDirName = "backup"

// Repository 备份仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建备份清单行
	Create(ctx context.Context, backup *entity.Backup) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity.Backup, error)
	// Delete 删除备份清单行
	Delete(ctx context.Context, id int64) error
	// ListCreatedBefore 查询创建时间早于 beforeMs（毫秒）的清单行（治理正向无主候选扫描）
	ListCreatedBefore(ctx context.Context, beforeMs int64) ([]*entity.Backup, error)
	// ListAllIDs 全量投影清单行 ID（治理反向悬空判定的现存集）
	ListAllIDs(ctx context.Context) ([]int64, error)
}

// Service 备份服务（纯文件仓库：移入保管/取回/清理，不感知备份来源）
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

// storeFile 将源文件收入备份目录并建保管清单行；copy=true 复制保留源文件（安装包备份），false 移动（O(1) 同文件系统）
func (s *Service) storeFile(ctx context.Context, sourcePath string, copy bool) (*entity.Backup, error) {
	if !util.FileExists(sourcePath) {
		return nil, fmt.Errorf("备份失败，源文件不存在: %s", sourcePath)
	}

	workDir := s.getWorkDir()
	fileName := filepath.Base(sourcePath)

	// 源文件移出扫描目录会触发旧路径 Remove 事件（跨目录 rename 旧路径事件不被
	// fsnotify 吞掉），在文件操作点登记抑制，避免 fsmonitor 将内部移动误报为外部
	// 删除。源路径不在 workDir 内（Rel 逃逸）时不登记——监控树外路径本就无事件。
	if !copy {
		if rel, err := filepath.Rel(workDir, sourcePath); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			storeRegistry.Suppress(rel)
			defer storeRegistry.Release(rel)
		}
	}

	// 按日期构建备份目录：backup/YYYY/MM/DD/
	now := time.Now()
	// relPath 域用 path.Join（正斜杠基准，入库/做键）；absPath 拼接才用 filepath.Join
	relativeDir := path.Join(
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
			return nil, fmt.Errorf("备份失败，上下文已取消: %w", ctx.Err())
		}
		if maxRetries <= 0 {
			return nil, fmt.Errorf("备份失败，文件名冲突重试次数超限: %s", fileName)
		}
		maxRetries--
		finalFileName = addSuffix(finalFileName, fmt.Sprintf("_%d", util.GetCurrentTimestamp()))
		finalAbsolutePath = filepath.Join(absoluteDir, finalFileName)
		logger.Log.Infof("文件已存在，尝试文件名: %s", finalFileName)
	}

	if copy {
		if err := util.CopyFile(sourcePath, finalAbsolutePath); err != nil {
			return nil, fmt.Errorf("备份失败，复制文件出错: %w", err)
		}
	} else {
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
	}

	// 建保管清单行，file_path 存储相对路径
	backup := entity.NewBackup()
	backup.FileName = sql.NullString{String: finalFileName, Valid: true}
	backup.FilePath = sql.NullString{String: path.Join(relativeDir, finalFileName), Valid: true}
	backup.Workdir = sql.NullString{String: workDir, Valid: true}
	if err := s.repo.Create(ctx, backup); err != nil {
		return nil, err
	}
	return backup, nil
}

// CreateBackup 复制源文件到备份目录并建保管清单行（保留源文件；供安装包备份等需保留源的场景）
func (s *Service) CreateBackup(ctx context.Context, sourcePath string) (*entity.Backup, error) {
	return s.storeFile(ctx, sourcePath, true)
}

// MoveToBackup 移动源文件到备份目录并建保管清单行，返回清单行 ID
// （供 store 软删链：文件离开 store/ 移入 backup/，行内嵌 backup_id 引用本清单行）
func (s *Service) MoveToBackup(ctx context.Context, absFilePath string) (int64, error) {
	backup, err := s.storeFile(ctx, absFilePath, false)
	if err != nil {
		return 0, err
	}
	return backup.GetID(), nil
}

// GetById 根据ID获取备份清单行
func (s *Service) GetById(ctx context.Context, id int64) (*entity.Backup, error) {
	return s.repo.GetById(ctx, id)
}

// ListCreatedBefore 查询创建时间早于 beforeMs（毫秒）的备份清单行
// （实现 backupGovernance.BackupCatalog：正向无主候选的扫描数据源）
func (s *Service) ListCreatedBefore(ctx context.Context, beforeMs int64) ([]*entity.Backup, error) {
	return s.repo.ListCreatedBefore(ctx, beforeMs)
}

// ListAllIDs 全量投影备份清单行 ID（实现 backupGovernance.BackupCatalog：反向悬空判定的现存集）
func (s *Service) ListAllIDs(ctx context.Context) ([]int64, error) {
	return s.repo.ListAllIDs(ctx)
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

// ResolveBackupPathById 按清单行 ID 取备份文件绝对路径（无记录/查询失败返回空串）
// 供 /store/ 文件服务：软删记录行内嵌 backup_id，据此定位备份文件
func (s *Service) ResolveBackupPathById(ctx context.Context, backupId int64) string {
	if backupId <= 0 {
		return ""
	}
	backup, err := s.repo.GetById(ctx, backupId)
	if err != nil || backup == nil {
		return ""
	}
	return s.GetBackupPath(backup)
}

// DeleteBackup 删除备份的磁盘文件与清单行（文件缺失容忍——可能已被取回）
func (s *Service) DeleteBackup(ctx context.Context, id int64) error {
	backup, err := s.repo.GetById(ctx, id)
	if err != nil {
		return fmt.Errorf("查询备份 %d 失败: %w", id, err)
	}
	if backup == nil {
		return nil
	}
	if absPath := s.GetBackupPath(backup); absPath != "" {
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			logger.Log.Warnf("删除备份文件失败（将仅删除记录）: %s, %v", absPath, err)
		}
	}
	return s.repo.Delete(ctx, id)
}

// RestoreFile 从备份路径还原文件到目标路径
// targetPath 为绝对路径（文件操作）；目标落在 store/ 白名单内时的 fsmonitor 操作抑制由调用方负责
// （抑制键为 workDir 相对路径，与绝对路径不同构，调用方编排时两者皆知）
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

// addSuffix 在文件名（不含扩展名）后添加后缀，保留扩展名
func addSuffix(filename string, suffix string) string {
	ext := filepath.Ext(filename)
	name := filename[:len(filename)-len(ext)]
	return name + suffix + ext
}
