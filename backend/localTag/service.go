package localTag

import (
	"context"
	"database/sql"
	"errors"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm/clause"
)

// 根标签ID
const RootLocalTagID = 0

// Repository 本地标签仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建
	Create(ctx context.Context, tag *domain.LocalTag) error
	// CreateBatch 批量新建
	CreateBatch(ctx context.Context, tags []*domain.LocalTag) error
	// Updates 更新
	Updates(ctx context.Context, tag *domain.LocalTag) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.LocalTag, error)
	// GetByName 根据名称获取
	GetByName(ctx context.Context, name string) (*domain.LocalTag, error)
	// GetByNames 根据名称列表批量查询本地标签
	GetByNames(ctx context.Context, names []string) ([]*domain.LocalTag, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.LocalTag, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.LocalTag], error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// SelectTreeNode 递归查询子标签
	SelectTreeNode(ctx context.Context, rootId int64, depth int) ([]*domain.LocalTag, error)
	// SelectParentNode 递归查询上级标签
	SelectParentNode(ctx context.Context, nodeId int64) ([]*domain.LocalTag, error)
	// ListByWorkId 查询作品关联的本地标签
	ListByWorkId(ctx context.Context, workId int64) ([]*domain.LocalTag, error)
	// ListSelectItems 查询选择项列表
	ListSelectItems(ctx context.Context, where clause.Expression, order clause.Expression) ([]*dto.SelectItem, error)
	// QuerySelectItemPage 分页查询选择项
	QuerySelectItemPage(ctx context.Context, opt *database.PageOption, secondaryLabel string) (*model.Page[dto.SelectItem], error)
	// QueryPageByWorkId 根据作品ID分页查询
	QueryPageByWorkId(ctx context.Context, opt *database.PageOption, workId int64, boundOnWorkId *bool) (*model.Page[domain.LocalTag], error)
	// QuerySelectItemPageByWorkId 根据作品ID分页查询选择项
	QuerySelectItemPageByWorkId(ctx context.Context, opt *database.PageOption, workId int64, boundOnWorkId *bool) (*model.Page[dto.SelectItem], error)
	// QueryWithBaseTagPage 分页查询包含基础标签信息的本地标签
	QueryWithBaseTagPage(ctx context.Context, opt *database.PageOption) (*model.Page[dto.LocalTagWithBaseTagDTO], error)
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
	err := s.repo.Create(ctx, tag)
	if err != nil {
		return err
	}
	return nil
}

// SaveBatch 批量保存本地标签
func (s *Service) SaveBatch(ctx context.Context, tags []*domain.LocalTag) error {
	for _, tag := range tags {
		if !tag.BaseLocalTagID.Valid || tag.BaseLocalTagID.Int64 == 0 {
			tag.BaseLocalTagID = sql.NullInt64{Int64: 0, Valid: true}
		}
	}
	return s.repo.CreateBatch(ctx, tags)
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
			if err := s.repo.Updates(ctx, newBaseTag); err != nil {
				return err
			}
		}
	}

	return s.repo.Updates(ctx, tag)
}

// UpdateLastUse 更新最后使用时间
func (s *Service) UpdateLastUse(ctx context.Context, ids []int64) error {
	now := util.GetCurrentTimestamp()
	for _, id := range ids {
		// Updates 仅写非零字段，建只含 ID+LastUse 的对象即可，无需读回完整记录
		tag := domain.NewLocalTag()
		tag.SetID(id)
		tag.LastUse = sql.NullInt64{Int64: now, Valid: true}
		if err := s.repo.Updates(ctx, tag); err != nil {
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

// GetByNames 根据名称列表批量查询本地标签
func (s *Service) GetByNames(ctx context.Context, names []string) ([]*domain.LocalTag, error) {
	if len(names) == 0 {
		return make([]*domain.LocalTag, 0), nil
	}
	return s.repo.GetByNames(ctx, names)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*domain.LocalTag, error) {
	return s.repo.List(ctx, opt)
}

// ListByIds 根据ID列表批量查询
func (s *Service) ListByIds(ctx context.Context, ids []int64) ([]*domain.LocalTag, error) {
	if len(ids) == 0 {
		return make([]*domain.LocalTag, 0), nil
	}
	return s.repo.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: "id", Values: util.ToAnySlice(ids)}},
	})
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, page *model.Page[domain.LocalTag], query LocalTagQueryDTO) (*model.Page[domain.LocalTag], error) {
	conv := querypkg.NewConverter(domain.LocalTag{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
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

// ListSelectItems 查询选择项列表（基于 QueryDTO）
func (s *Service) ListSelectItems(ctx context.Context, queryDTO LocalTagQueryDTO) ([]*dto.SelectItem, error) {
	conv := querypkg.NewConverter(domain.LocalTag{})
	queryOpt, err := conv.ToQueryOption(queryDTO, nil)
	if err != nil {
		return nil, err
	}
	var where clause.Expression
	if len(queryOpt.Conditions) > 0 {
		where = queryOpt.Conditions[0]
	}
	var order clause.Expression
	if len(queryOpt.OrderBy) > 0 {
		order = queryOpt.OrderBy[0]
	}
	return s.repo.ListSelectItems(ctx, where, order)
}

// QuerySelectItemPage 分页查询选择项
func (s *Service) QuerySelectItemPage(ctx context.Context, page *model.Page[dto.SelectItem], query LocalTagQueryDTO, secondaryLabel string) (*model.Page[dto.SelectItem], error) {
	conv := querypkg.NewConverter(domain.LocalTag{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.QuerySelectItemPage(ctx, opt, secondaryLabel)
}

// QueryPageByWorkId 根据作品ID分页查询
func (s *Service) QueryPageByWorkId(ctx context.Context, opt *database.PageOption, workId int64, boundOnWorkId *bool) (*model.Page[domain.LocalTag], error) {
	return s.repo.QueryPageByWorkId(ctx, opt, workId, boundOnWorkId)
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (s *Service) QuerySelectItemPageByWorkId(ctx context.Context, page *model.Page[dto.SelectItem], query LocalTagQueryDTO, boundOnWorkId *bool) (*model.Page[dto.SelectItem], error) {
	if query.WorkId.Value == nil {
		return nil, errors.New("workId is required")
	}
	workId := *query.WorkId.Value
	opt := &database.PageOption{
		QueryOption: database.QueryOption{},
		Page:        page.PageNumber,
		PageSize:    page.PageSize,
	}
	return s.repo.QuerySelectItemPageByWorkId(ctx, opt, workId, boundOnWorkId)
}

// QueryWithBaseTagPage 分页查询包含基础标签信息的本地标签
func (s *Service) QueryWithBaseTagPage(ctx context.Context, page *model.Page[dto.LocalTagWithBaseTagDTO], query LocalTagQueryDTO) (*model.Page[dto.LocalTagWithBaseTagDTO], error) {
	conv := querypkg.NewConverter(domain.LocalTag{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, new("local_tag"))
	if err != nil {
		return nil, err
	}
	return s.repo.QueryWithBaseTagPage(ctx, opt)
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
	ErrTagIdRequired       = &pkgerr.BusinessError{Code: 400, Message: "更新本地标签失败，id不能为空"}
	ErrCannotBeBaseOfSelf  = &pkgerr.BusinessError{Code: 400, Message: "基础标签不能为自身"}
	ErrOriginalTagNotFound = &pkgerr.BusinessError{Code: 404, Message: "修改本地标签失败，原标签信息不能为空"}
)
