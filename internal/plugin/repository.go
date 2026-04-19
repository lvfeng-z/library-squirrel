package plugin

import (
	"context"
	"fmt"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm"
)

// pluginRepository 插件仓储实现
type pluginRepository struct {
	db *gorm.DB
}

// NewRepository 创建插件仓储
func NewRepository(db *gorm.DB) Repository {
	return &pluginRepository{
		db: db,
	}
}

// GORM 返回底层 GORM DB 实例
func (r *pluginRepository) GORM() *gorm.DB {
	return r.db
}

// Save 保存
func (r *pluginRepository) Save(ctx context.Context, plugin *domain.Plugin) error {
	return r.db.WithContext(ctx).Create(plugin).Error
}

// SaveBatch 批量保存
func (r *pluginRepository) SaveBatch(ctx context.Context, plugins []*domain.Plugin) error {
	return r.db.WithContext(ctx).Create(plugins).Error
}

// Update 更新
func (r *pluginRepository) Update(ctx context.Context, plugin *domain.Plugin) error {
	return r.db.WithContext(ctx).Save(plugin).Error
}

// GetById 根据ID获取
func (r *pluginRepository) GetById(ctx context.Context, id int64) (*domain.Plugin, error) {
	var plugin domain.Plugin
	err := r.db.WithContext(ctx).First(&plugin, id).Error
	if err != nil {
		return nil, err
	}
	return &plugin, nil
}

// List 查询列表
func (r *pluginRepository) List(ctx context.Context, opt *database.QueryOption) ([]*domain.Plugin, error) {
	var plugins []*domain.Plugin
	db := r.db.WithContext(ctx).Model(new(domain.Plugin))
	db = applyQueryOption(db, opt)
	err := db.Find(&plugins).Error
	if err != nil {
		return nil, err
	}
	return plugins, nil
}

// Count 统计数量
func (r *pluginRepository) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	var count int64
	db := r.db.WithContext(ctx).Model(new(domain.Plugin))
	db = applyQueryOption(db, opt)
	err := db.Count(&count).Error
	return count, err
}

// Delete 删除
func (r *pluginRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(new(domain.Plugin), id).Error
}

// Page 分页查询
func (r *pluginRepository) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Plugin, PluginQueryDTO], error) {
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

	return model.NewPage[domain.Plugin, PluginQueryDTO](list, total, page, pageSize), nil
}

// CheckInstalled 检查插件是否已安装
func (r *pluginRepository) CheckInstalled(ctx context.Context, publicId string) (bool, error) {
	var count int64
	err := r.GORM().WithContext(ctx).Model(&domain.Plugin{}).
		Where("public_id = ? AND uninstalled = 0", publicId).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetByPublicId 根据公开ID获取
func (r *pluginRepository) GetByPublicId(ctx context.Context, publicId string) (*domain.Plugin, error) {
	var plugin domain.Plugin
	err := r.GORM().WithContext(ctx).Model(&domain.Plugin{}).
		Where("public_id = ?", publicId).
		First(&plugin).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &plugin, nil
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

// 辅助函数
func buildPublicIdCondition(publicId string) string {
	return fmt.Sprintf("public_id = '%s'", publicId)
}