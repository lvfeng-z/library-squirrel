package journal

import (
	"context"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// JournalRepository 记账账表仓储实现
type JournalRepository struct {
	*database.BaseRepository[entity.JournalEntry]
}

// NewRepository 创建记账账表仓储
func NewRepository(db *gorm.DB) *JournalRepository {
	return &JournalRepository{
		BaseRepository: database.NewBaseRepository[entity.JournalEntry](db),
	}
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *JournalRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// ListSinceSnapshot 同一读事务内双读（最旧存活游标边界 + cursor 后行集）：轮转裁剪与
// 游标读取并发时两读共享同一快照，过期判定不会落在裁剪前、行集落在裁剪后的不一致状态
func (r *JournalRepository) ListSinceSnapshot(ctx context.Context, cursor int64, limit int) (*ListSnapshot, error) {
	snap := &ListSnapshot{}
	err := r.dbFromCtx(ctx).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var boundary struct {
			Total int64
			MinID int64
		}
		if err := tx.Raw("SELECT COUNT(*) AS total, COALESCE(MIN(id), 0) AS min_id FROM journal_entry").Scan(&boundary).Error; err != nil {
			return err
		}
		snap.AliveExists = boundary.Total > 0
		snap.OldestAliveID = boundary.MinID
		return tx.Where(clause.Gt{Column: "id", Value: cursor}).
			Order("id ASC").
			Limit(limit).
			Find(&snap.Entries).Error
	})
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// RotateOldest 轮转裁剪（单语句，两条件 OR 连接）：删除创建时刻早于 expiredBefore（毫秒
// 时间戳）的超龄行，以及 id 落在最新 keepRows 行之外的行（容量轨裁最旧）。行数不超容量
// 且无超龄行时零命中（幂等）；容量子查询在表行数不足 keepRows 时返回空集，条件不命中
func (r *JournalRepository) RotateOldest(ctx context.Context, expiredBefore int64, keepRows int) (int64, error) {
	res := r.dbFromCtx(ctx).WithContext(ctx).Exec(`
		DELETE FROM journal_entry
		WHERE create_time < ?
		   OR id < (SELECT MIN(id) FROM (SELECT id FROM journal_entry ORDER BY id DESC LIMIT 1 OFFSET ?))
	`, expiredBefore, keepRows-1)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}
