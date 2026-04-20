package work

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// workRepository 作品仓储实现
type workRepository struct {
	db *gorm.DB
}

// NewRepository 创建作品仓储
func NewRepository(db *gorm.DB) Repository {
	return &workRepository{
		db: db,
	}
}

// GORM 返回底层 GORM DB 实例
func (r *workRepository) GORM() *gorm.DB {
	return r.db
}

// Save 保存
func (r *workRepository) Save(ctx context.Context, work *domain.Work) error {
	return r.db.WithContext(ctx).Create(work).Error
}

// Update 更新
func (r *workRepository) Update(ctx context.Context, work *domain.Work) error {
	return r.db.WithContext(ctx).Save(work).Error
}

// GetById 根据ID获取
func (r *workRepository) GetById(ctx context.Context, id int64) (*domain.Work, error) {
	var work domain.Work
	err := r.db.WithContext(ctx).First(&work, id).Error
	if err != nil {
		return nil, err
	}
	return &work, nil
}

// List 查询列表
func (r *workRepository) List(ctx context.Context, opt *database.QueryOption) ([]*domain.Work, error) {
	var works []*domain.Work
	db := r.db.WithContext(ctx).Model(new(domain.Work))
	db = applyQueryOption(db, opt)
	err := db.Find(&works).Error
	if err != nil {
		return nil, err
	}
	return works, nil
}

// Count 统计数量
func (r *workRepository) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	var count int64
	db := r.db.WithContext(ctx).Model(new(domain.Work))
	db = applyQueryOption(db, opt)
	err := db.Count(&count).Error
	return count, err
}

// Delete 删除
func (r *workRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(new(domain.Work), id).Error
}

// Page 分页查询
func (r *workRepository) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Work, WorkQueryDTO], error) {
	page := opt.Page
	pageSize := opt.PageSize

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	// 构建查询选项（设置 Limit 和 Offset）
	queryOpt := opt.QueryOption
	queryOpt.Limit = pageSize
	queryOpt.Offset = offset

	// 查询列表
	list, err := r.List(ctx, &queryOpt)
	if err != nil {
		return nil, err
	}

	// 统计总数（不需要 Limit 和 Offset）
	countOpt := opt.QueryOption
	countOpt.Limit = 0
	countOpt.Offset = 0
	total, err := r.Count(ctx, &countOpt)
	if err != nil {
		return nil, err
	}

	return model.NewPage[domain.Work, WorkQueryDTO](list, total, page, pageSize), nil
}

// GetBySiteAndSiteWorkID 根据站点和站点作品ID查询
func (r *workRepository) GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*domain.Work, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.And(
				clause.Eq{Column: "site_id", Value: siteId},
				clause.Eq{Column: "site_work_id", Value: siteWorkId},
			),
		},
	}
	var work domain.Work
	db := r.db.WithContext(ctx).Model(new(domain.Work))
	db = applyQueryOption(db, opt)
	err := db.First(&work).Error
	if err != nil {
		return nil, err
	}
	return &work, nil
}

// ListByIds 根据ID列表批量查询
func (r *workRepository) ListByIds(ctx context.Context, ids []int64) ([]*domain.Work, error) {
	if len(ids) == 0 {
		return []*domain.Work{}, nil
	}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: "id", Values: toInterfaceSlice(ids)}},
	}
	var works []*domain.Work
	db := r.db.WithContext(ctx).Model(new(domain.Work))
	db = applyQueryOption(db, opt)
	err := db.Find(&works).Error
	if err != nil {
		return nil, err
	}
	return works, nil
}

// UpdateLastViewBatch 批量更新最后查看时间
func (r *workRepository) UpdateLastViewBatch(ctx context.Context, ids []int64, lastView int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.GORM().
		WithContext(ctx).
		Model(new(domain.Work)).
		Where("id IN ?", ids).
		Update("last_view", lastView).Error
}

// applyQueryOption 将 QueryOption 应用到 db 实例
func applyQueryOption(db *gorm.DB, opt *database.QueryOption) *gorm.DB {
	// 1. Select（覆盖型）
	if opt.Select != nil {
		db = db.Select(opt.Select)
	}

	// 2. Joins（叠加型）
	for _, join := range opt.Joins {
		db = db.Clauses(join)
	}

	// 3. Conditions（叠加型）
	for _, cond := range opt.Conditions {
		db = db.Where(cond)
	}

	// 4. OrderBy（叠加型）
	if len(opt.OrderBy) > 0 {
		db = db.Order(opt.OrderBy)
	}

	// 5. GroupBy（覆盖型）
	if opt.GroupBy != nil {
		db = db.Clauses(opt.GroupBy)
	}

	// 6. Having（覆盖型）
	if opt.Having != nil {
		db = db.Having(opt.Having)
	}

	// 7. Limit & Offset（覆盖型）
	if opt.Limit > 0 {
		db = db.Limit(opt.Limit)
	}
	if opt.Offset > 0 {
		db = db.Offset(opt.Offset)
	}

	return db
}

// toInterfaceSlice converts int64 slice to interface{} slice
func toInterfaceSlice(ids []int64) []interface{} {
	result := make([]interface{}, len(ids))
	for i, id := range ids {
		result[i] = id
	}
	return result
}
