package localAuthor

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm/clause"
)

// ========== 查询 DTO ==========

// LocalAuthorQueryDTO 本地作者查询条件
type LocalAuthorQueryDTO struct {
	// 精确查询
	ID         *int64  `json:"-"`          // 本地作者ID（程序设置，不从JSON解析）
	AuthorName *string `json:"authorName"` // 作者名称（精确匹配）
	// 模糊查询
	AuthorNameLike *string `json:"authorNameLike"` // 作者名称（模糊匹配）
	IntroduceLike  *string `json:"introduceLike"`  // 介绍（模糊匹配）
	// 排序字段：create_time, update_time, author_name, last_use
	OrderBy   string `json:"orderBy"`   // 排序字段
	OrderDesc bool   `json:"orderDesc"` // 是否降序
}

// BuildOrderBy 根据查询DTO构建排序条件
func (dto *LocalAuthorQueryDTO) BuildOrderBy() clause.Expression {
	column := "id"
	if dto.OrderBy != "" {
		// 支持的排序字段映射
		orderByMap := map[string]string{
			"create_time": "create_time",
			"update_time": "update_time",
			"author_name": "author_name",
			"last_use":    "last_use",
		}
		if v, ok := orderByMap[dto.OrderBy]; ok {
			column = v
		}
	}
	return clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: column}, Desc: dto.OrderDesc}}}
}

// Repository 本地作者仓储接口（由 service 定义需要的数据库操作方法）
// 注意：只定义 service 真正需要的方法，遵循最小依赖原则
type Repository interface {
	// Save 保存
	Save(ctx context.Context, author *domain.LocalAuthor) error
	// Update 更新
	Update(ctx context.Context, author *domain.LocalAuthor) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.LocalAuthor, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.LocalAuthor, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.LocalAuthor], error)
	// ListReWorkAuthor 批量获取作品与作者的关联
	ListReWorkAuthor(ctx context.Context, workIds []int64) (map[int64][]*model.RankedLocalAuthor, error)
	// ListByWorkId 查询作品的本地作者
	ListByWorkId(ctx context.Context, workId int64) ([]*model.RankedLocalAuthor, error)
	// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表
	ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedLocalAuthorWithWorkId, error)
	// ListSelectItems 查询选择项列表
	ListSelectItems(ctx context.Context, where clause.Expression, order clause.Expression) ([]*domain.SelectItem, error)
	// QuerySelectItemPage 分页查询选择项
	QuerySelectItemPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.SelectItem], error)
}

// Service 本地作者服务
type Service struct {
	repo Repository
}

// NewService 创建本地作者服务
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
	}
}

// Save 保存作者
func (s *Service) Save(ctx context.Context, author *domain.LocalAuthor) error {
	return s.repo.Save(ctx, author)
}

// UpdateById 更新作者
func (s *Service) UpdateById(ctx context.Context, author *domain.LocalAuthor) error {
	if author.ID == 0 {
		return ErrAuthorIdRequired
	}
	return s.repo.Update(ctx, author)
}

// UpdateLastUse 批量更新最后使用时间
func (s *Service) UpdateLastUse(ctx context.Context, ids []int64) error {
	now := util.GetCurrentTimestamp()
	for _, id := range ids {
		author, err := s.repo.GetById(ctx, id)
		if err != nil {
			continue
		}
		author.LastUse = sql.NullInt64{Int64: now, Valid: true}
		if err := s.repo.Update(ctx, author); err != nil {
			return err
		}
	}
	return nil
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*domain.LocalAuthor, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*domain.LocalAuthor, error) {
	return s.repo.List(ctx, opt)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// Delete 删除作者
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.LocalAuthor], error) {
	return s.repo.Page(ctx, opt)
}

// PageByDTO 分页查询（基于 QueryDTO）
func (s *Service) PageByDTO(ctx context.Context, page, pageSize int, queryDTO LocalAuthorQueryDTO) (*model.Page[domain.LocalAuthor], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	orderBy := queryDTO.BuildOrderBy()
	opt := &database.PageOption{
		QueryOption: database.QueryOption{
			Conditions: conditions,
			OrderBy:    []clause.Expression{orderBy},
		},
		Page:     page,
		PageSize: pageSize,
	}
	return s.repo.Page(ctx, opt)
}

// ListSelectItemsByDTO 查询选择项列表（基于 QueryDTO）
func (s *Service) ListSelectItemsByDTO(ctx context.Context, queryDTO LocalAuthorQueryDTO) ([]*domain.SelectItem, error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.ListSelectItems(ctx, where, orderBy)
}

// QuerySelectItemPageByDTO 分页查询选择项（基于 QueryDTO）
func (s *Service) QuerySelectItemPageByDTO(ctx context.Context, page, pageSize int, queryDTO LocalAuthorQueryDTO) (*model.Page[domain.SelectItem], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.QuerySelectItemPage(ctx, page, pageSize, where, orderBy)
}

// buildConditionsFromDTO 根据查询DTO构建查询条件
func buildConditionsFromDTO(dto *LocalAuthorQueryDTO) []clause.Expression {
	var conditions []clause.Expression

	if dto.ID != nil {
		conditions = append(conditions, clause.Eq{Column: "id", Value: *dto.ID})
	}
	if dto.AuthorName != nil {
		conditions = append(conditions, clause.Eq{Column: "author_name", Value: *dto.AuthorName})
	}
	if dto.AuthorNameLike != nil {
		conditions = append(conditions, clause.Like{Column: "author_name", Value: *dto.AuthorNameLike})
	}
	if dto.IntroduceLike != nil {
		conditions = append(conditions, clause.Like{Column: "introduce", Value: *dto.IntroduceLike})
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

// ListReWorkAuthor 批量获取作品与作者的关联
func (s *Service) ListReWorkAuthor(ctx context.Context, workIds []int64) (map[int64][]*model.RankedLocalAuthor, error) {
	return s.repo.ListReWorkAuthor(ctx, workIds)
}

// ListByWorkId 查询作品的本地作者
func (s *Service) ListByWorkId(ctx context.Context, workId int64) ([]*model.RankedLocalAuthor, error) {
	return s.repo.ListByWorkId(ctx, workId)
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表
func (s *Service) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedLocalAuthorWithWorkId, error) {
	return s.repo.ListRankedLocalAuthorWithWorkIdByWorkIds(ctx, workIds)
}

// ListSelectItems 查询选择项列表
func (s *Service) ListSelectItems(ctx context.Context, where clause.Expression, order clause.Expression) ([]*domain.SelectItem, error) {
	return s.repo.ListSelectItems(ctx, where, order)
}

// QuerySelectItemPage 分页查询选择项
func (s *Service) QuerySelectItemPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.SelectItem], error) {
	return s.repo.QuerySelectItemPage(ctx, page, pageSize, where, order)
}

// 错误定义
var (
	ErrAuthorIdRequired = &BusinessError{Code: 400, Message: "更新本地作者失败，id不能为空"}
)

// BusinessError 业务错误
type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string {
	return e.Message
}
