package pluginpreference

import (
	"context"
	"errors"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PluginPreferenceRepository 插件偏好仓储实现（实现运行时面与管理面两个接口）
type PluginPreferenceRepository struct {
	*database.BaseRepository[entity.PluginPreference]
}

// NewRepository 创建插件偏好仓储
func NewRepository(db *gorm.DB) *PluginPreferenceRepository {
	return &PluginPreferenceRepository{
		BaseRepository: database.NewBaseRepository[entity.PluginPreference](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *PluginPreferenceRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *PluginPreferenceRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// GetByKey 按 (plugin_id, pref_key) 查行，未命中返回 (nil, nil)
func (r *PluginPreferenceRepository) GetByKey(ctx context.Context, pluginID int64, prefKey string) (*entity.PluginPreference, error) {
	row, err := r.Get(ctx, &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "plugin_id", Value: pluginID},
			clause.Eq{Column: "pref_key", Value: prefKey},
		},
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return row, err
}

// UpsertByKey 原子插入或整值覆写（基于 (plugin_id, pref_key) 复合唯一约束）。冲突时
// 仅重写 value 与 update_time：行 id 与 create_time 保持首记时刻，同键重写不换行
func (r *PluginPreferenceRepository) UpsertByKey(ctx context.Context, pluginID int64, prefKey, valueJSON string, now int64) error {
	row := entity.NewPluginPreference()
	row.PluginID = pluginID
	row.PrefKey = prefKey
	row.Value = valueJSON
	row.SetCreateTime(now)
	row.SetUpdateTime(now)
	return r.dbFromCtx(ctx).WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "plugin_id"}, {Name: "pref_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"value", "update_time",
		}),
	}).Create(row).Error
}

// ListByPlugin 某插件全部偏好行（pref_key 升序稳定输出；条件限定 plugin_id，跨插件行不可达）
func (r *PluginPreferenceRepository) ListByPlugin(ctx context.Context, pluginID int64) ([]*entity.PluginPreference, error) {
	return r.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{clause.Eq{Column: "plugin_id", Value: pluginID}},
		OrderBy: []clause.Expression{clause.OrderBy{Columns: []clause.OrderByColumn{
			{Column: clause.Column{Name: "pref_key"}},
		}}},
	})
}

// ListAllWithPlugin 全量偏好条目 + 归属插件显示信息（plugin_id 升序、同插件内
// update_time 降序）。JOIN plugin 表取显示名（插件行不物理删，归属名恒可查），
// 单查询组装免逐插件回查
func (r *PluginPreferenceRepository) ListAllWithPlugin(ctx context.Context) ([]EntryWithPlugin, error) {
	var entries []EntryWithPlugin
	err := r.dbFromCtx(ctx).WithContext(ctx).Table("plugin_preference").
		Select(preferencePluginSelect).
		Joins("JOIN plugin ON plugin.id = plugin_preference.plugin_id").
		Order("plugin_preference.plugin_id ASC, plugin_preference.update_time DESC").
		Scan(&entries).Error
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// ListByPublicIdWithPlugin 按插件公开 ID 查该插件偏好条目 + 显示信息（update_time 降序，
// 最近决策在前）
func (r *PluginPreferenceRepository) ListByPublicIdWithPlugin(ctx context.Context, pluginPublicId string) ([]EntryWithPlugin, error) {
	var entries []EntryWithPlugin
	err := r.dbFromCtx(ctx).WithContext(ctx).Table("plugin_preference").
		Select(preferencePluginSelect).
		Joins("JOIN plugin ON plugin.id = plugin_preference.plugin_id").
		Where("plugin.public_id = ?", pluginPublicId).
		Order("plugin_preference.update_time DESC").
		Scan(&entries).Error
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// preferencePluginSelect 管理列表 JOIN 查询的列清单（列别名对齐 EntryWithPlugin 字段的
// GORM 蛇形映射；public_id/name 可空列 COALESCE 空串，缺名不阻塞列表）
const preferencePluginSelect = `plugin_preference.id, plugin_preference.plugin_id,
plugin_preference.pref_key, plugin_preference.value, plugin_preference.update_time,
COALESCE(plugin.public_id, '') AS plugin_public_id, COALESCE(plugin.name, '') AS plugin_name`

// Delete 按条目 id 物理删除（实体无软删列；目标行不存在时幂等无效果）
func (r *PluginPreferenceRepository) Delete(ctx context.Context, id int64) error {
	return r.BaseRepository.Delete(ctx, id)
}
