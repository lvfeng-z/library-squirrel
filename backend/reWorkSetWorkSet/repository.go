package reWorkSetWorkSet

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 后代/祖先递归查询的深度上限。无环 DAG 自然终止；此上限为异常数据（误成环）下的防御性兜底
const maxTraversalDepth = 100

// ReWorkSetWorkSetRepository 作品集间父子关联（多父 DAG）仓储实现
type ReWorkSetWorkSetRepository struct {
	*database.BaseRepository[domain.ReWorkSetWorkSet]
}

// NewRepository 创建关联仓储
func NewRepository(db *gorm.DB) *ReWorkSetWorkSetRepository {
	return &ReWorkSetWorkSetRepository{
		BaseRepository: database.NewBaseRepository[domain.ReWorkSetWorkSet](db),
	}
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *ReWorkSetWorkSetRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// SaveRelation 建立父子关系。OnConflict DoNothing 使其幂等——重复建立同一关系不报错
func (r *ReWorkSetWorkSetRepository) SaveRelation(ctx context.Context, rel *domain.ReWorkSetWorkSet) error {
	now := util.GetCurrentTimestamp()
	if rel.GetID() == 0 {
		rel.SetCreateTime(now)
	}
	rel.SetUpdateTime(now)
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(rel).Error
}

// DeleteByParentAndChild 解除指定父子关系
func (r *ReWorkSetWorkSetRepository) DeleteByParentAndChild(ctx context.Context, parentWorkSetId, childWorkSetId int64) error {
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("parent_work_set_id = ? AND child_work_set_id = ?", parentWorkSetId, childWorkSetId).
		Delete(new(domain.ReWorkSetWorkSet)).Error
}

// DeleteByParentWorkSetId 删除某父集的全部子集关系（父作品集删除时清理）
func (r *ReWorkSetWorkSetRepository) DeleteByParentWorkSetId(ctx context.Context, parentWorkSetId int64) error {
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("parent_work_set_id = ?", parentWorkSetId).
		Delete(new(domain.ReWorkSetWorkSet)).Error
}

// DeleteByChildWorkSetId 删除某子集的全部父集关系（子作品集删除时清理）
func (r *ReWorkSetWorkSetRepository) DeleteByChildWorkSetId(ctx context.Context, childWorkSetId int64) error {
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("child_work_set_id = ?", childWorkSetId).
		Delete(new(domain.ReWorkSetWorkSet)).Error
}

// GetByParentAndChild 根据父子作品集ID获取关联
func (r *ReWorkSetWorkSetRepository) GetByParentAndChild(ctx context.Context, parentWorkSetId, childWorkSetId int64) (*domain.ReWorkSetWorkSet, error) {
	var result domain.ReWorkSetWorkSet
	err := r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("parent_work_set_id = ? AND child_work_set_id = ?", parentWorkSetId, childWorkSetId).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListChildWorkSetIds 查询父集的直接子作品集ID（按 sort_order 升序）
func (r *ReWorkSetWorkSetRepository) ListChildWorkSetIds(ctx context.Context, parentWorkSetId int64) ([]int64, error) {
	var childIds []int64
	err := r.dbFromCtx(ctx).
		WithContext(ctx).
		Model(new(domain.ReWorkSetWorkSet)).
		Where("parent_work_set_id = ?", parentWorkSetId).
		Order("sort_order ASC").
		Pluck("child_work_set_id", &childIds).Error
	return childIds, err
}

// UpdateSiteSortOrdersForChild 批量更新一个子作品集在各父集下的原站序（写 site_sort_order，不影响本地 sort_order）
// 与 re_work_work_set.UpdateSiteSortOrders 的区别：此处 CASE 按 parent_work_set_id 匹配、限定单个 child 的父关系行
func (r *ReWorkSetWorkSetRepository) UpdateSiteSortOrdersForChild(ctx context.Context, childWorkSetId int64, parentOrders map[int64]int) error {
	if len(parentOrders) == 0 {
		return nil
	}
	caseExpr, parentIds := buildParentCaseExpression(parentOrders)
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Model(new(domain.ReWorkSetWorkSet)).
		Where("child_work_set_id = ?", childWorkSetId).
		Where("parent_work_set_id IN ?", parentIds).
		Updates(map[string]interface{}{
			"site_sort_order": gorm.Expr(caseExpr),
			"update_time":     util.GetCurrentTimestamp(),
		}).Error
}

// buildParentCaseExpression 构造按 parent_work_set_id 匹配的 CASE 表达式 + 涉及的父集 ID 列表
// 抽为纯函数便于单测（环境 CGO 不可用时无法跑内存 SQLite，CASE 串构造由此覆盖）
func buildParentCaseExpression(parentOrders map[int64]int) (expr string, parentIds []int64) {
	parentIds = make([]int64, 0, len(parentOrders))
	cases := make([]string, 0, len(parentOrders))
	for parentId, order := range parentOrders {
		parentIds = append(parentIds, parentId)
		cases = append(cases, fmt.Sprintf("WHEN %d THEN %d", parentId, order))
	}
	return "CASE parent_work_set_id " + strings.Join(cases, " ") + " END", parentIds
}

// ListParentWorkSetIds 查询子作品集的直接父作品集ID
func (r *ReWorkSetWorkSetRepository) ListParentWorkSetIds(ctx context.Context, childWorkSetId int64) ([]int64, error) {
	var parentIds []int64
	err := r.dbFromCtx(ctx).
		WithContext(ctx).
		Model(new(domain.ReWorkSetWorkSet)).
		Where("child_work_set_id = ?", childWorkSetId).
		Pluck("parent_work_set_id", &parentIds).Error
	return parentIds, err
}

// CollectDescendantWorkSetIds 递归查询作品集的所有后代作品集ID（沿 parent→child 边向下，不含 root 自身）
// 多父 DAG 用 UNION（非 UNION ALL）去重——菱形依赖（A→B、A→C、B→D、C→D）下同一后代会经多条路径到达
func (r *ReWorkSetWorkSetRepository) CollectDescendantWorkSetIds(ctx context.Context, rootWorkSetId int64) ([]int64, error) {
	query := `
		WITH RECURSIVE descendants(id, level) AS (
			SELECT child_work_set_id, 1
			FROM re_work_set_work_set
			WHERE parent_work_set_id = ?
			UNION
			SELECT child_work_set_id, descendants.level + 1
			FROM re_work_set_work_set
			JOIN descendants ON re_work_set_work_set.parent_work_set_id = descendants.id
			WHERE descendants.level < ?
		)
		SELECT id FROM descendants
	`
	var ids []int64
	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, rootWorkSetId, maxTraversalDepth).Scan(&ids).Error
	return ids, err
}

// CollectAncestorWorkSetIds 递归查询作品集的所有祖先作品集ID（沿 child→parent 边向上，不含 node 自身）
// 环路检测依据：建立 A→B 前，若 A 已是 B 的祖先（B 能沿 parent 边到达 A），再加 A→B 会闭合环路
func (r *ReWorkSetWorkSetRepository) CollectAncestorWorkSetIds(ctx context.Context, workSetId int64) ([]int64, error) {
	query := `
		WITH RECURSIVE ancestors(id, level) AS (
			SELECT parent_work_set_id, 1
			FROM re_work_set_work_set
			WHERE child_work_set_id = ?
			UNION
			SELECT parent_work_set_id, ancestors.level + 1
			FROM re_work_set_work_set
			JOIN ancestors ON re_work_set_work_set.child_work_set_id = ancestors.id
			WHERE ancestors.level < ?
		)
		SELECT id FROM ancestors
	`
	var ids []int64
	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, workSetId, maxTraversalDepth).Scan(&ids).Error
	return ids, err
}
