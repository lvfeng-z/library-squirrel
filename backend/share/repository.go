package share

// 分享记录仓储（share_record：无软删，历史记录的删除为物理删行）。

import (
	"context"
	"errors"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 分享记录仓储
type Repository struct {
	*database.BaseRepository[entity.ShareRecord]
}

// NewRepository 创建分享记录仓储
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{
		BaseRepository: database.NewBaseRepository[entity.ShareRecord](db),
	}
}

// dbFromCtx 事务感知取连接（MaxOpenConns=1 下事务内不得直取连接池，防死锁）
func (r *Repository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// ListAll 全部分享记录（create_time 倒序——最新分享在前）
func (r *Repository) ListAll(ctx context.Context) ([]*entity.ShareRecord, error) {
	return r.List(ctx, &database.QueryOption{
		OrderBy: []clause.Expression{
			clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "create_time"}, Desc: true}}},
		},
	})
}

// ListByState 按状态查记录（启动自动复原取 active 行）
func (r *Repository) ListByState(ctx context.Context, state string) ([]*entity.ShareRecord, error) {
	return r.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{clause.Eq{Column: "state", Value: state}},
	})
}

// GetByShareID 按业务键查记录行（无命中返回 nil, nil）
func (r *Repository) GetByShareID(ctx context.Context, shareID string) (*entity.ShareRecord, error) {
	rec, err := r.Get(ctx, &database.QueryOption{
		Conditions: []clause.Expression{clause.Eq{Column: "share_id", Value: shareID}},
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// UpdateTerminal 落终态（state/err_msg/revoked_at 显式列更新——零值 err_msg/revoked_at
// 须可写，不能用跳零值的 Updates）
func (r *Repository) UpdateTerminal(ctx context.Context, id int64, state, errMsg string, revokedAt int64) error {
	return r.dbFromCtx(ctx).WithContext(ctx).Model(&entity.ShareRecord{}).Where("id = ?", id).
		Updates(map[string]any{"state": state, "err_msg": errMsg, "revoked_at": revokedAt}).Error
}

// RefreshOnline 复原在线后刷新记录行（到期时刻按中继 WELCOME 回填、统计按重新规划值刷新；
// 会话与记录的其余参数在发布时已固定，复原不换 token/密钥/链接）
func (r *Repository) RefreshOnline(ctx context.Context, id int64, expiresAt, fileCount, totalBytes, missingFiles int64) error {
	return r.dbFromCtx(ctx).WithContext(ctx).Model(&entity.ShareRecord{}).Where("id = ?", id).
		Updates(map[string]any{
			"expires_at":    expiresAt,
			"file_count":    fileCount,
			"total_bytes":   totalBytes,
			"missing_files": missingFiles,
		}).Error
}
