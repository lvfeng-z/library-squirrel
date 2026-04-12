package localTag

import (
	"context"
	"database/sql"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm/clause"
)

// 根标签ID
const RootLocalTagID = 0

// ========== 查询 DTO ==========

// LocalTagQueryDTO 本地标签查询条件
type LocalTagQueryDTO struct {
	// 精确查询
	ID             *int64  `json:"-"`              // 本地标签ID（程序设置，不从JSON解析）
	BaseLocalTagID *int64  `json:"baseLocalTagId"` // 基础本地标签ID
	LocalTagName   *string `json:"localTagName"`   // 本地标签名称（精确匹配）
	// 模糊查询
	LocalTagNameLike *string `json:"localTagNameLike"` // 本地标签名称（模糊匹配）
	// 排序字段：create_time, update_time, local_tag_name, last_use
	OrderBy   string `json:"orderBy"`   // 排序字段
	OrderDesc bool   `json:"orderDesc"` // 是否降序
}

// BuildOrderBy 根据查询DTO构建排序条件
func (dto *LocalTagQueryDTO) BuildOrderBy() clause.Expression {
	column := "id"
	if dto.OrderBy != "" {
		// 支持的排序字段映射
		orderByMap := map[string]string{
			"create_time":    "create_time",
			"update_time":    "update_time",
			"local_tag_name": "local_tag_name",
			"last_use":       "last_use",
		}
		if v, ok := orderByMap[dto.OrderBy]; ok {
			column = v
		}
	}
	return clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: column}, Desc: dto.OrderDesc}}}
}

// Repository 本地标签仓储接口（由 service 定义需要的数据库操作方法）
// 注意：只定义 service 真正需要的方法，遵循最小依赖原则
type Repository interface {
	// Save 保存
	Save(ctx context.Context, tag *domain.LocalTag) error
	// Update 更新
	Update(ctx context.Context, tag *domain.LocalTag) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.LocalTag, error)
	// GetByName 根据名称获取
	GetByName(ctx context.Context, name string) (*domain.LocalTag, error)
	// List 查询列表
	List(ctx context.Context, conditions []clause.Expression, orderBy clause.Expression, limit, offset int) ([]*domain.LocalTag, error)
	// Count 统计数量
	Count(ctx context.Context, conditions []clause.Expression) (int64, error)
	// Page 分页查询
	Page(ctx context.Context, page, pageSize int, conditions []clause.Expression, orderBy clause.Expression) (*model.Page[domain.LocalTag], error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// SelectTreeNode 递归查询子标签
	SelectTreeNode(ctx context.Context, rootId int64, depth int) ([]*domain.LocalTag, error)
	// SelectParentNode 递归查询上级标签
	SelectParentNode(ctx context.Context, nodeId int64) ([]*domain.LocalTag, error)
	// ListByWorkId 查询作品关联的本地标签
	ListByWorkId(ctx context.Context, workId int64) ([]*domain.LocalTag, error)
	// QueryDTOPage DTO分页查询
	QueryDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.LocalTag], error)
	// ListSelectItems 查询选择项列表
	ListSelectItems(ctx context.Context, where clause.Expression, order clause.Expression) ([]*domain.SelectItem, error)
	// QuerySelectItemPage 分页查询选择项
	QuerySelectItemPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, secondaryLabel string) (*model.Page[domain.SelectItem], error)
	// QueryPageByWorkId 根据作品ID分页查询
	QueryPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64) (*model.Page[domain.LocalTag], error)
}

// Service 本地标签服务
type Service struct {
	repo Repository
}

// NewService 创建本地标签服务
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
	}
}

// Save 保存本地标签
func (s *Service) Save(ctx context.Context, tag *domain.LocalTag) error {
	if !tag.BaseLocalTagID.Valid || tag.BaseLocalTagID.Int64 == 0 {
		tag.BaseLocalTagID = sql.NullInt64{Int64: 0, Valid: true} // 表示根标签
	}
	err := s.repo.Save(ctx, tag)
	if err != nil {
		return err
	}
	return nil
}

// UpdateById 更新本地标签
func (s *Service) UpdateById(ctx context.Context, tag *domain.LocalTag) error {
	if tag.ID == 0 {
		return ErrTagIdRequired
	}

	// 不能将自己设为上级标签
	if tag.BaseLocalTagID.Valid && tag.BaseLocalTagID.Int64 == tag.ID {
		return ErrCannotBeBaseOfSelf
	}

	if !tag.BaseLocalTagID.Valid || tag.BaseLocalTagID.Int64 == 0 {
		tag.BaseLocalTagID = sql.NullInt64{Int64: 0, Valid: true}
	}

	// 查询新上级节点的所有上级节点，如果新上级是本节点的下级，则需要调整
	if tag.BaseLocalTagID.Valid && tag.BaseLocalTagID.Int64 != 0 {
		parentTags, err := s.repo.SelectParentNode(ctx, tag.BaseLocalTagID.Int64)
		if err != nil {
			return err
		}
		parentTagIds := make([]int64, len(parentTags))
		for i, pt := range parentTags {
			parentTagIds[i] = pt.ID
		}

		// 如果新的上级节点是原本的下级节点
		if contains(parentTagIds, tag.ID) {
			// 查询本节点原来的数据
			old, err := s.repo.GetById(ctx, tag.ID)
			if err != nil {
				return ErrOriginalTagNotFound
			}

			// 将新上级节点移动到本节点的原上级节点之下
			newBaseTag := domain.NewLocalTag()
			if tag.BaseLocalTagID.Valid {
				newBaseTag.SetID(tag.BaseLocalTagID.Int64)
			}
			newBaseTag.BaseLocalTagID = old.BaseLocalTagID
			if err := s.repo.Update(ctx, newBaseTag); err != nil {
				return err
			}
		}
	}

	return s.repo.Update(ctx, tag)
}

// UpdateLastUse 更新最后使用时间
func (s *Service) UpdateLastUse(ctx context.Context, ids []int64) error {
	now := util.GetCurrentTimestamp()
	for _, id := range ids {
		tag, err := s.repo.GetById(ctx, id)
		if err != nil {
			continue
		}
		tag.LastUse = sql.NullInt64{Int64: now, Valid: true}
		if err := s.repo.Update(ctx, tag); err != nil {
			return err
		}
	}
	return nil
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*domain.LocalTag, error) {
	return s.repo.GetById(ctx, id)
}

// GetByName 根据名称获取
func (s *Service) GetByName(ctx context.Context, name string) (*domain.LocalTag, error) {
	return s.repo.GetByName(ctx, name)
}

// List 查询列表
func (s *Service) List(ctx context.Context, where clause.Expression, order clause.Expression, limit, offset int) ([]*domain.LocalTag, error) {
	var conditions []clause.Expression
	if where != nil {
		conditions = []clause.Expression{where}
	}
	return s.repo.List(ctx, conditions, order, limit, offset)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, where clause.Expression) (int64, error) {
	var conditions []clause.Expression
	if where != nil {
		conditions = []clause.Expression{where}
	}
	return s.repo.Count(ctx, conditions)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.LocalTag], error) {
	var conditions []clause.Expression
	if where != nil {
		conditions = []clause.Expression{where}
	}
	return s.repo.Page(ctx, page, pageSize, conditions, order)
}

// PageByDTO 分页查询（基于 QueryDTO）
func (s *Service) PageByDTO(ctx context.Context, page, pageSize int, queryDTO LocalTagQueryDTO) (*model.Page[domain.LocalTag], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.Page(ctx, page, pageSize, conditions, orderBy)
}

// Delete 删除标签
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// GetTree 获取标签树形结构
func (s *Service) GetTree(ctx context.Context, rootId int64, depth int) ([]*domain.LocalTag, error) {
	if rootId == 0 {
		rootId = RootLocalTagID // 默认根标签
	}
	if depth <= 0 {
		depth = 1
	}
	return s.repo.SelectTreeNode(ctx, rootId, depth)
}

// SelectParentNode 查询上级标签
func (s *Service) SelectParentNode(ctx context.Context, nodeId int64) ([]*domain.LocalTag, error) {
	return s.repo.SelectParentNode(ctx, nodeId)
}

// ListByWorkId 查询作品关联的标签
func (s *Service) ListByWorkId(ctx context.Context, workId int64) ([]*domain.LocalTag, error) {
	return s.repo.ListByWorkId(ctx, workId)
}

// QueryDTOPage DTO分页查询
func (s *Service) QueryDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.LocalTag], error) {
	return s.repo.QueryDTOPage(ctx, page, pageSize, where, order)
}

// ListSelectItems 查询选择项列表
func (s *Service) ListSelectItems(ctx context.Context, where clause.Expression, order clause.Expression) ([]*domain.SelectItem, error) {
	return s.repo.ListSelectItems(ctx, where, order)
}

// QuerySelectItemPage 分页查询选择项
func (s *Service) QuerySelectItemPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, secondaryLabel string) (*model.Page[domain.SelectItem], error) {
	return s.repo.QuerySelectItemPage(ctx, page, pageSize, where, order, secondaryLabel)
}

// QuerySelectItemPageByDTO 分页查询选择项（基于 QueryDTO）
func (s *Service) QuerySelectItemPageByDTO(ctx context.Context, page, pageSize int, queryDTO LocalTagQueryDTO, secondaryLabel string) (*model.Page[domain.SelectItem], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.QuerySelectItemPage(ctx, page, pageSize, where, orderBy, secondaryLabel)
}

// ListSelectItemsByDTO 查询选择项列表（基于 QueryDTO）
func (s *Service) ListSelectItemsByDTO(ctx context.Context, queryDTO LocalTagQueryDTO) ([]*domain.SelectItem, error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.ListSelectItems(ctx, where, orderBy)
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (s *Service) QuerySelectItemPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64) (*model.Page[domain.SelectItem], error) {
	pageResult, err := s.repo.QueryPageByWorkId(ctx, page, pageSize, where, order, workId)
	if err != nil {
		return nil, err
	}

	// 转换为 SelectItem
	items := make([]*domain.SelectItem, len(pageResult.Data))
	for i, tag := range pageResult.Data {
		label := ""
		if tag.LocalTagName.Valid {
			label = tag.LocalTagName.String
		}
		lastUse := int64(0)
		if tag.LastUse.Valid {
			lastUse = tag.LastUse.Int64
		}
		items[i] = &domain.SelectItem{
			Value:   tag.ID,
			Label:   label,
			LastUse: lastUse,
		}
	}
	return model.NewPage(items, pageResult.DataCount, page, pageSize), nil
}

// QuerySelectItemPageByWorkIdByDTO 根据作品ID分页查询选择项（基于 QueryDTO）
func (s *Service) QuerySelectItemPageByWorkIdByDTO(ctx context.Context, page, pageSize int, queryDTO LocalTagQueryDTO, workId int64) (*model.Page[domain.SelectItem], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	pageResult, err := s.repo.QueryPageByWorkId(ctx, page, pageSize, where, orderBy, workId)
	if err != nil {
		return nil, err
	}

	// 转换为 SelectItem
	items := make([]*domain.SelectItem, len(pageResult.Data))
	for i, tag := range pageResult.Data {
		label := ""
		if tag.LocalTagName.Valid {
			label = tag.LocalTagName.String
		}
		lastUse := int64(0)
		if tag.LastUse.Valid {
			lastUse = tag.LastUse.Int64
		}
		items[i] = &domain.SelectItem{
			Value:   tag.ID,
			Label:   label,
			LastUse: lastUse,
		}
	}
	return model.NewPage(items, pageResult.DataCount, page, pageSize), nil
}

// buildConditionsFromDTO 根据查询DTO构建查询条件
func buildConditionsFromDTO(dto *LocalTagQueryDTO) []clause.Expression {
	var conditions []clause.Expression

	if dto.ID != nil {
		conditions = append(conditions, clause.Eq{Column: "id", Value: *dto.ID})
	}
	if dto.BaseLocalTagID != nil {
		conditions = append(conditions, clause.Eq{Column: "base_local_tag_id", Value: *dto.BaseLocalTagID})
	}
	if dto.LocalTagName != nil {
		conditions = append(conditions, clause.Eq{Column: "local_tag_name", Value: *dto.LocalTagName})
	}
	if dto.LocalTagNameLike != nil {
		conditions = append(conditions, clause.Like{Column: "local_tag_name", Value: *dto.LocalTagNameLike})
	}

	return conditions
}

// combineConditions 将多个条件组合成单个表达式
func combineConditions(conditions []clause.Expression) clause.Expression {
	if len(conditions) == 0 {
		return nil
	}
	if len(conditions) == 1 {
		return conditions[0]
	}
	result := clause.AndConditions{}
	for _, cond := range conditions {
		result.Exprs = append(result.Exprs, cond)
	}
	return result
}

// 辅助函数
func contains(slice []int64, item int64) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

// 错误定义
var (
	ErrTagIdRequired       = &BusinessError{Code: 400, Message: "更新本地标签失败，id不能为空"}
	ErrCannotBeBaseOfSelf  = &BusinessError{Code: 400, Message: "基础标签不能为自身"}
	ErrOriginalTagNotFound = &BusinessError{Code: 404, Message: "修改本地标签失败，原标签信息不能为空"}
)

// BusinessError 业务错误
type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string {
	return e.Message
}
