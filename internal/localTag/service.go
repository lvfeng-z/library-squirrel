package localTag

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model"
	"github.com/library-squirrel/wails/pkg/query"

	"gorm.io/gorm/clause"
)

// 根标签ID
const RootLocalTagID = 0

// ========== 查询 DTO ==========

// LocalTagQueryDTO 本地标签查询条件
type LocalTagQueryDTO struct {
	ID              query.QueryAttribute[int64]  `json:"-" query:"id"`                             // 本地标签ID（程序设置，不从JSON解析）
	BaseLocalTagID  query.QueryAttribute[int64]  `json:"baseLocalTagId" query:"base_local_tag_id"` // 基础本地标签ID
	LocalTagName    query.QueryAttribute[string] `json:"localTagName" query:"local_tag_name"`      // 本地标签名称（精确匹配）
	LocalTagNameStr query.QueryAttribute[string] `json:"localTagNameStr" query:"local_tag_name"`   // 本地标签名称（模糊匹配）
	UpdateTime      query.QueryAttribute[int64]  `json:"updateTime" query:"update_time"`           // 更新时间（可用于排序）
	CreateTime      query.QueryAttribute[int64]  `json:"createTime" query:"create_time"`           // 创建时间（可用于排序）
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
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.LocalTag, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.LocalTag, any], error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// SelectTreeNode 递归查询子标签
	SelectTreeNode(ctx context.Context, rootId int64, depth int) ([]*domain.LocalTag, error)
	// SelectParentNode 递归查询上级标签
	SelectParentNode(ctx context.Context, nodeId int64) ([]*domain.LocalTag, error)
	// ListByWorkId 查询作品关联的本地标签
	ListByWorkId(ctx context.Context, workId int64) ([]*domain.LocalTag, error)
	// QueryDTOPage DTO分页查询
	QueryDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.LocalTag, LocalTagQueryDTO], error)
	// ListSelectItems 查询选择项列表
	ListSelectItems(ctx context.Context, where clause.Expression, order clause.Expression) ([]*domain.SelectItem, error)
	// QuerySelectItemPage 分页查询选择项
	QuerySelectItemPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, secondaryLabel string) (*model.Page[domain.SelectItem, LocalTagQueryDTO], error)
	// QueryPageByWorkId 根据作品ID分页查询
	QueryPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64) (*model.Page[domain.LocalTag, LocalTagQueryDTO], error)
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
func (s *Service) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.LocalTag, any], error) {
	return s.repo.Page(ctx, opt)
}

// PageByDTO 分页查询（基于 QueryDTO）
func (s *Service) PageByDTO(ctx context.Context, page, pageSize int, queryDTO LocalTagQueryDTO) (*model.Page[domain.LocalTag, any], error) {
	conv := query.NewConverter(domain.LocalTag{})
	opt, err := conv.ToPageOption(queryDTO, page, pageSize)
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

// QueryDTOPage DTO分页查询
func (s *Service) QueryDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.LocalTag, LocalTagQueryDTO], error) {
	return s.repo.QueryDTOPage(ctx, page, pageSize, where, order)
}

// ListSelectItems 查询选择项列表
func (s *Service) ListSelectItems(ctx context.Context, where clause.Expression, order clause.Expression) ([]*domain.SelectItem, error) {
	return s.repo.ListSelectItems(ctx, where, order)
}

// QuerySelectItemPage 分页查询选择项
func (s *Service) QuerySelectItemPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, secondaryLabel string) (*model.Page[domain.SelectItem, LocalTagQueryDTO], error) {
	return s.repo.QuerySelectItemPage(ctx, page, pageSize, where, order, secondaryLabel)
}

// QuerySelectItemPageByDTO 分页查询选择项（基于 QueryDTO）
func (s *Service) QuerySelectItemPageByDTO(ctx context.Context, page, pageSize int, queryDTO LocalTagQueryDTO, secondaryLabel string) (*model.Page[domain.SelectItem, LocalTagQueryDTO], error) {
	conv := query.NewConverter(domain.LocalTag{})
	queryOpt, err := conv.ToQueryOption(queryDTO)
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
	return s.repo.QuerySelectItemPage(ctx, page, pageSize, where, order, secondaryLabel)
}

// ListSelectItemsByDTO 查询选择项列表（基于 QueryDTO）
func (s *Service) ListSelectItemsByDTO(ctx context.Context, queryDTO LocalTagQueryDTO) ([]*domain.SelectItem, error) {
	conv := query.NewConverter(domain.LocalTag{})
	queryOpt, err := conv.ToQueryOption(queryDTO)
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

// QuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (s *Service) QuerySelectItemPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64) (*model.Page[domain.SelectItem, LocalTagQueryDTO], error) {
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
		items[i] = &domain.SelectItem{
			Value: tag.ID,
			Label: label,
		}
	}
	return model.NewPage[domain.SelectItem, LocalTagQueryDTO](items, pageResult.DataCount, page, pageSize), nil
}

// QuerySelectItemPageByWorkIdByDTO 根据作品ID分页查询选择项（基于 QueryDTO）
func (s *Service) QuerySelectItemPageByWorkIdByDTO(ctx context.Context, page, pageSize int, queryDTO LocalTagQueryDTO, workId int64) (*model.Page[domain.SelectItem, LocalTagQueryDTO], error) {
	conv := query.NewConverter(domain.LocalTag{})
	queryOpt, err := conv.ToQueryOption(queryDTO)
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
		items[i] = &domain.SelectItem{
			Value: tag.ID,
			Label: label,
		}
	}
	return model.NewPage[domain.SelectItem, LocalTagQueryDTO](items, pageResult.DataCount, page, pageSize), nil
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
