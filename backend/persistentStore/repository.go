package persistentStore

import (
	"context"
	"strings"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PersistentStoreRepository 文件持久存储仓储实现
type PersistentStoreRepository struct {
	*database.BaseRepository[domain.PersistentStore]
}

// NewRepository 创建文件持久存储仓储
func NewRepository(db *gorm.DB) *PersistentStoreRepository {
	return &PersistentStoreRepository{
		BaseRepository: database.NewBaseRepository[domain.PersistentStore](db),
	}
}

// GetByFilePath 根据文件路径获取记录
func (r *PersistentStoreRepository) GetByFilePath(ctx context.Context, filePath string) (*domain.PersistentStore, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "file_path", Value: filePath},
		},
		Limit: 1,
	}
	list, err := r.BaseRepository.List(ctx, opt)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

// ExistsByFilePath 检查文件路径是否已存在记录
func (r *PersistentStoreRepository) ExistsByFilePath(ctx context.Context, filePath string) bool {
	record, err := r.GetByFilePath(ctx, filePath)
	if err != nil {
		return false
	}
	return record != nil
}

// ResetCompleted 显式重置 completed_at=0（未完成零值是合法业务值，GORM Updates 跳零值故单列更新）
func (r *PersistentStoreRepository) ResetCompleted(ctx context.Context, id int64) error {
	return r.GORM().WithContext(ctx).
		Model(new(domain.PersistentStore)).
		Where("id = ?", id).
		Update("completed_at", 0).Error
}

// DeleteUnscopedByIds 批量物理删除记录（单条 DELETE IN）。dbFromCtx 模式：可安全用于事务内
// （作品彻底删除链在事务内调用）。目标为已软删行时 HardDelete 的 GetById 会静默跳过，故走此直删
func (r *PersistentStoreRepository) DeleteUnscopedByIds(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return database.DBFromContext(ctx, r.GORM()).WithContext(ctx).
		Unscoped().
		Where("id IN ?", ids).
		Delete(new(domain.PersistentStore)).Error
}

// RestoreByIds 批量清软删标志与备份引用（复原链：文件还原回 store/ 后记录复活，
// backup_id 指向的清单行已随还原删除故一并清零；Unscoped 逃逸 Update 的软删过滤）
func (r *PersistentStoreRepository) RestoreByIds(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.GORM().WithContext(ctx).
		Unscoped().
		Model(new(domain.PersistentStore)).
		Where("id IN ?", ids).
		Updates(map[string]interface{}{"deleted_at": 0, "backup_id": 0}).Error
}

// SoftDeleteWithBackup 软删记录并写入备份清单行引用（单条 UPDATE 同生共死：
// 文件移动入 backup/ 成功后调用，backup_id 与 deleted_at 原子落盘；backupId=0 表无备份的软删）
func (r *PersistentStoreRepository) SoftDeleteWithBackup(ctx context.Context, id int64, backupId int64) error {
	return r.GORM().WithContext(ctx).
		Unscoped().
		Model(new(domain.PersistentStore)).
		Where("id = ? AND deleted_at = 0", id).
		Updates(map[string]interface{}{
			"deleted_at": util.GetCurrentTimestamp(),
			"backup_id":  backupId,
		}).Error
}

// NormalizeFilePaths 将 file_path 中的反斜杠统一为正斜杠（符合存储规范）
// 幂等：无反斜杠时无操作。返回受影响行数
func (r *PersistentStoreRepository) NormalizeFilePaths(ctx context.Context) (int64, error) {
	result := r.GORM().WithContext(ctx).Exec(
		"UPDATE persistent_store SET file_path = REPLACE(file_path, '\\', '/') WHERE file_path LIKE '%\\%'",
	)
	return result.RowsAffected, result.Error
}

// RenameDirectoryPrefix 批量替换 file_path 的目录前缀（目录改名同步：oldPrefix/ → newPrefix/）
// 匹配 oldPrefix 开头的所有下级文件路径。返回受影响行数
func (r *PersistentStoreRepository) RenameDirectoryPrefix(ctx context.Context, oldPrefix string, newPrefix string) (int64, error) {
	// DB file_path 统一正斜杠，入参规范化防 Windows 反斜杠导致匹配失败
	oldPrefix = strings.ReplaceAll(oldPrefix, "\\", "/")
	newPrefix = strings.ReplaceAll(newPrefix, "\\", "/")
	// 用 GLOB 而非 LIKE：GLOB 区分大小写→走 file_path 索引（LIKE 默认不区分大小写→全表扫）
	result := r.GORM().WithContext(ctx).Exec(
		"UPDATE persistent_store SET file_path = REPLACE(file_path, ?, ?) WHERE file_path GLOB ?",
		oldPrefix+"/", newPrefix+"/", oldPrefix+"/*",
	)
	return result.RowsAffected, result.Error
}

// ListReferencedBackupIds 全量投影行内引用的备份清单行 ID（DISTINCT backup_id WHERE backup_id > 0）。
// Unscoped 含已删行——软删行是合法引用者（回收站待复原），GORM 默认软删 scope 排除即活备份被误判无主
func (r *PersistentStoreRepository) ListReferencedBackupIds(ctx context.Context) ([]int64, error) {
	var ids []int64
	err := r.GORM().WithContext(ctx).
		Unscoped().
		Model(new(domain.PersistentStore)).
		Where("backup_id > 0").
		Distinct().
		Pluck("backup_id", &ids).Error
	return ids, err
}

// ClearBackupRefsByBackupIds 按引用目标清 backup_id（悬空引用清列）。
// Unscoped 含已删行——已删行持有引用同样须清
func (r *PersistentStoreRepository) ClearBackupRefsByBackupIds(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.GORM().WithContext(ctx).
		Unscoped().
		Model(new(domain.PersistentStore)).
		Where("backup_id IN ?", ids).
		Update("backup_id", 0).Error
}

// ClearIllegalAliveBackupRefs 清活行（deleted_at=0）携带 backup_id>0 的非法态列，返回受影响行数。
// 构造上不可达（backup_id 与 deleted_at 单条 UPDATE 同生共死、复原双列同清），防御外部直改数据库
func (r *PersistentStoreRepository) ClearIllegalAliveBackupRefs(ctx context.Context) (int64, error) {
	result := r.GORM().WithContext(ctx).
		Unscoped().
		Model(new(domain.PersistentStore)).
		Where("deleted_at = 0 AND backup_id > 0").
		Update("backup_id", 0)
	return result.RowsAffected, result.Error
}
