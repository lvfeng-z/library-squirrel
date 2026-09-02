package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/storeRegistry"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
)

// BackupRootDirName 备份根目录名（单一源在 storeRegistry.BackupDirPath，本常量供模块内外引用）
const BackupRootDirName = storeRegistry.BackupDirPath

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
	// PageBackups 分页查询保管清单（create_time 倒序）；includeIDs/excludeIDs 为 ID 集过滤，nil=无该向过滤
	PageBackups(ctx context.Context, pageNumber, pageSize int, includeIDs []int64, excludeIDs []int64) (*model.Page[entity.Backup], error)
	// GetByFilePathInWorkDir 按保管路径精确查指定工作目录的清单行（无命中返回 nil）
	GetByFilePathInWorkDir(ctx context.Context, workDir string, filePath string) (*entity.Backup, error)
	// ListByPathPrefixInWorkDir 按路径前缀查指定工作目录的清单行（目录前缀圈定受影响行）
	ListByPathPrefixInWorkDir(ctx context.Context, workDir string, prefix string) ([]*entity.Backup, error)
	// ListAllInWorkDir 全量查指定工作目录中保管路径有效的清单行（离线对账数据源）
	ListAllInWorkDir(ctx context.Context, workDir string) ([]*entity.Backup, error)
	// UpdateFilePath 更新清单行保管路径（移动同步）
	UpdateFilePath(ctx context.Context, id int64, filePath string) error
	// NormalizeFilePaths 规范化 file_path 分隔符为正斜杠，返回修正行数（启动迁移：历史反斜杠行修正）
	NormalizeFilePaths(ctx context.Context) (int64, error)
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

// suppressWithinWorkDir 对 workDir 内的绝对路径登记 fsmonitor 操作抑制，返回释放函数。
// 本模块对 backup/ 域的文件操作（移出还原、删除清理）触发 Remove 事件，事件命中尚存的
// 清单行会误报为外部删除，故操作点登记。路径在 workDir 外（工作目录迁移前的旧行）或
// 相对路径逃逸时不登记——监控树外本就无事件。抑制键为 workDir 相对正斜杠路径
func (s *Service) suppressWithinWorkDir(absPath string) func() {
	workDir := s.getWorkDir()
	rel, err := filepath.Rel(workDir, absPath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return func() {}
	}
	storeRegistry.Suppress(rel)
	return func() { storeRegistry.Release(rel) }
}

// storeFile 将源文件收入备份目录并建保管清单行；copy=true 复制保留源文件（安装包备份），false 移动（O(1) 同文件系统）
func (s *Service) storeFile(ctx context.Context, sourcePath string, copy bool) (*entity.Backup, error) {
	// 备份目录与清单行 Workdir 都以已配置的库根为基准，未配置即拒绝（清单行不再落空串 Workdir）
	if err := settings.RefuseIfUnconfigured(s.getWorkDir(), "backup"); err != nil {
		return nil, err
	}
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

// PageBackups 分页查询保管清单（实现 backupGovernance.BackupCatalog：备份管理面板的清单分页）。
// 引用态过滤由治理方折算成 ID 集传入，本方法只做纯 ID 集过滤与分页
func (s *Service) PageBackups(ctx context.Context, pageNumber, pageSize int, includeIDs []int64, excludeIDs []int64) (*model.Page[entity.Backup], error) {
	return s.repo.PageBackups(ctx, pageNumber, pageSize, includeIDs, excludeIDs)
}

// GetByFilePath 按保管路径精确查当前工作目录的清单行，无命中返回 nil。
// 供 fsmonitor backup 域：文件 Remove 事件按路径定位清单行。
// 工作目录过滤排除迁移前旧行（其文件不在当前监控树内，路径字符串可能撞车）
func (s *Service) GetByFilePath(ctx context.Context, filePath string) (*entity.Backup, error) {
	return s.repo.GetByFilePathInWorkDir(ctx, s.getWorkDir(), filePath)
}

// ListByPathPrefix 按路径前缀查当前工作目录的清单行（含多级下级）。
// 供 fsmonitor backup 域：目录 Remove 事件按前缀圈定受影响清单行
func (s *Service) ListByPathPrefix(ctx context.Context, prefix string) ([]*entity.Backup, error) {
	return s.repo.ListByPathPrefixInWorkDir(ctx, s.getWorkDir(), prefix)
}

// ListAllInWorkDir 全量查当前工作目录中保管路径有效的清单行。
// 供 fsmonitor backup 域离线对账：清单行 × 磁盘文件比对的数据源
func (s *Service) ListAllInWorkDir(ctx context.Context) ([]*entity.Backup, error) {
	return s.repo.ListAllInWorkDir(ctx, s.getWorkDir())
}

// UpdateFilePath 更新清单行保管路径（fsmonitor backup 域移动同步：行路径跟随文件新位置）
func (s *Service) UpdateFilePath(ctx context.Context, id int64, newFilePath string) error {
	return s.repo.UpdateFilePath(ctx, id, newFilePath)
}

// NormalizeFilePaths 规范化 file_path 分隔符为正斜杠（启动时调用一次；镜像 persistentStore 同名迁移）
func (s *Service) NormalizeFilePaths(ctx context.Context) (int64, error) {
	return s.repo.NormalizeFilePaths(ctx)
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

// DeleteBackup 删除备份的磁盘文件与清单行（文件缺失容忍；行不存在幂等成功——
// 并发删除/确认流重复条目确认时行可能已先被清）。真实删除失败（非文件缺失）
// 不删记录并返回错误：文件还在就删记录会使记录失真（RECORD_STATE_TRUTHFUL），
// 由调用方决定仅删记录（DeleteBackupRecord）或放弃。两阶段拆分见 DeleteBackupFile/DeleteBackupRecord
func (s *Service) DeleteBackup(ctx context.Context, id int64) error {
	if err := s.DeleteBackupFile(ctx, id); err != nil {
		return err
	}
	return s.DeleteBackupRecord(ctx, id)
}

// DeleteBackupFile 仅删除备份的磁盘文件（清单行不动）——文件缺失容忍，真实删除失败返回错误。
// 供删除流「先文件后记录」两阶段的 Phase A：文件删不动即中止（记录未动），由调用方决定
// 仅删记录或放弃
func (s *Service) DeleteBackupFile(ctx context.Context, id int64) error {
	backup, err := s.repo.GetById(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("查询备份 %d 失败: %w", id, err)
	}
	if backup == nil {
		return nil
	}
	if absPath := s.GetBackupPath(backup); absPath != "" {
		// 文件先删、清单行后删的窗口内 Remove 事件会命中本行，登记抑制防误报
		release := s.suppressWithinWorkDir(absPath)
		err := os.Remove(absPath)
		release()
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除备份文件失败（记录已保留）: %w", err)
		}
	}
	return nil
}

// DeleteBackupRecord 仅删除备份清单行（不动磁盘文件）——用户对文件删除失败明确选择
// 「仅删记录」的降级路径（Phase B 记录侧）：文件被占用/只读保留在磁盘，记录不再指向它
// （与 DeleteBackup 的「文件缺失容忍」同属用户知情例外，缺省路径仍是不删记录）
func (s *Service) DeleteBackupRecord(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// RestoreFile 从备份路径还原文件到目标路径
// targetPath 为绝对路径（文件操作）；目标落在 store/ 白名单内时的 fsmonitor 操作抑制由调用方负责
// （抑制键为 workDir 相对路径，与绝对路径不同构，调用方编排时两者皆知）；
// 源端（backup/ 域）的抑制在本方法内登记——文件移出备份目录即触发 backup 域 Remove 事件，
// 此时清单行尚未被调用方删除，命中即误报为外部删除
func (s *Service) RestoreFile(ctx context.Context, backupPath string, targetPath string) error {
	if err := settings.RefuseIfUnconfigured(s.getWorkDir(), "backup"); err != nil {
		return err
	}
	if !util.FileExists(backupPath) {
		return fmt.Errorf("还原失败，备份文件不存在: %s", backupPath)
	}
	releaseSource := s.suppressWithinWorkDir(backupPath)
	defer releaseSource()
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
