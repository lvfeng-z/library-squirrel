package siteTag

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"gorm.io/gorm/clause"
)

// ========== 外部模块接口定义（由 siteTag 模块定义自己需要的接口）==========

// LocalTagOperator 本地标签操作接口
type LocalTagOperator interface {
	// Save 保存本地标签
	Save(ctx context.Context, tag *entity2.LocalTag) error
	// GetByName 根据名称获取本地标签
	GetByName(ctx context.Context, name string) (*entity2.LocalTag, error)
}

// LocalTagQueryOperator 本地标签查询接口（用于siteTag模块查询关联数据）
type LocalTagQueryOperator interface {
	ListByIds(ctx context.Context, ids []int64) ([]*entity2.LocalTag, error)
}

// SiteQueryOperator 站点查询接口（用于siteTag模块查询关联数据）
type SiteQueryOperator interface {
	ListByIds(ctx context.Context, ids []int64) ([]*entity2.Site, error)
}

// Repository 站点标签仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建
	Create(ctx context.Context, tag *entity2.SiteTag) error
	// CreateBatch 批量新建
	CreateBatch(ctx context.Context, tags []*entity2.SiteTag) error
	// Updates 更新
	Updates(ctx context.Context, tag *entity2.SiteTag) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity2.SiteTag, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*entity2.SiteTag, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity2.SiteTag], error)
	// ListByWorkId 查询作品的站点标签
	ListByWorkId(ctx context.Context, workId int64) ([]*entity2.SiteTag, error)
	// ListBySiteTagIds 根据站点标签ID列表查询
	ListBySiteTagIds(ctx context.Context, siteTagIds []int64) ([]*entity2.SiteTag, error)
	// UpdateBindLocalTag 绑定本地标签
	UpdateBindLocalTag(ctx context.Context, localTagId *int64, siteTagIds []int64) (int64, error)
	// GetBySiteAndSiteTagID 根据站点ID和站点标签ID查询
	GetBySiteAndSiteTagID(ctx context.Context, siteId int64, siteTagId string) (*entity2.SiteTag, error)
	// QueryPageByWorkId 根据作品ID分页查询站点标签
	QueryPageByWorkId(ctx context.Context, opt *database.PageOption, workId int64, boundOnWorkId *bool) (*model.Page[sdkdto.SiteTagFullDTO], error)
	// QueryLocalRelateDTOPage 查询站点标签与本地标签关联DTO分页
	QueryLocalRelateDTOPage(ctx context.Context, opt *database.PageOption, workId int64, boundOnWorkId *bool) (*model.Page[sdkdto.SiteTagLocalRelateDTO], error)
	// QuerySelectItemPageByWorkId 根据作品ID分页查询站点标签选择项
	QuerySelectItemPageByWorkId(ctx context.Context, opt *database.PageOption, workId int64, boundOnWorkId *bool) (*model.Page[sdkdto.SelectItem], error)
	// Upsert 原子插入或更新
	Upsert(ctx context.Context, tag *entity2.SiteTag) error
	// BatchUpsert 批量插入或更新
	BatchUpsert(ctx context.Context, tags []*entity2.SiteTag) error
	// ListBySiteAndSiteTagIDs 根据站点ID和站点标签ID列表批量查询
	ListBySiteAndSiteTagIDs(ctx context.Context, siteId int64, siteTagIds []string) ([]*entity2.SiteTag, error)
}

// Service 站点标签服务
type Service struct {
	repo             Repository
	localTagOperator LocalTagOperator
	localTagQueryOp  LocalTagQueryOperator
	siteQueryOp      SiteQueryOperator
}

// NewService 创建站点标签服务
func NewService(repo Repository, localTagOperator LocalTagOperator, localTagQueryOp LocalTagQueryOperator, siteQueryOp SiteQueryOperator) *Service {
	return &Service{
		repo:             repo,
		localTagOperator: localTagOperator,
		localTagQueryOp:  localTagQueryOp,
		siteQueryOp:      siteQueryOp,
	}
}

// Save 保存站点标签
func (s *Service) Save(ctx context.Context, tag *entity2.SiteTag) error {
	return s.repo.Create(ctx, tag)
}

// SaveBatch 批量保存站点标签
func (s *Service) SaveBatch(ctx context.Context, tags []*entity2.SiteTag) error {
	return s.repo.CreateBatch(ctx, tags)
}

// UpdateById 更新站点标签
func (s *Service) UpdateById(ctx context.Context, tag *entity2.SiteTag) error {
	if tag.ID == 0 {
		return ErrTagIdRequired
	}
	return s.repo.Updates(ctx, tag)
}

// GetBySiteAndSiteTagID 根据站点ID和站点标签ID查询
func (s *Service) GetBySiteAndSiteTagID(ctx context.Context, siteId int64, siteTagId string) (*entity2.SiteTag, error) {
	return s.repo.GetBySiteAndSiteTagID(ctx, siteId, siteTagId)
}

// SaveOrUpdateByCompositeKey 按 (siteId, siteTagId) 原子保存或更新站点标签，返回内部 DB ID
func (s *Service) SaveOrUpdateByCompositeKey(ctx context.Context, tag *entity2.SiteTag) (int64, error) {
	if err := s.repo.Upsert(ctx, tag); err != nil {
		return 0, err
	}
	if tag.ID > 0 {
		return tag.ID, nil
	}
	existing, err := s.repo.GetBySiteAndSiteTagID(ctx, tag.SiteID.Int64, tag.SiteTagID.String)
	if err != nil {
		return 0, err
	}
	return existing.ID, nil
}

// BatchUpsert 批量插入或更新站点标签
func (s *Service) BatchUpsert(ctx context.Context, tags []*entity2.SiteTag) error {
	return s.repo.BatchUpsert(ctx, tags)
}

// ListBySiteAndSiteTagIDs 根据站点ID和站点标签ID列表批量查询
func (s *Service) ListBySiteAndSiteTagIDs(ctx context.Context, siteId int64, siteTagIds []string) ([]*entity2.SiteTag, error) {
	return s.repo.ListBySiteAndSiteTagIDs(ctx, siteId, siteTagIds)
}

// UpdateLastUse 批量更新最后使用时间
func (s *Service) UpdateLastUse(ctx context.Context, ids []int64) error {
	now := util.GetCurrentTimestamp()
	for _, id := range ids {
		tag, err := s.repo.GetById(ctx, id)
		if err != nil {
			continue
		}
		tag.LastUse = sql.NullInt64{Int64: now, Valid: true}
		if err := s.repo.Updates(ctx, tag); err != nil {
			return err
		}
	}
	return nil
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*entity2.SiteTag, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*entity2.SiteTag, error) {
	return s.repo.List(ctx, opt)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// Delete 删除标签
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, page *model.Page[entity2.SiteTag], query SiteTagQueryDTO) (*model.Page[entity2.SiteTag], error) {
	conv := querypkg.NewConverter(entity2.SiteTag{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
}

// QueryBoundOrUnboundToLocalTagPage 查询绑定或未绑定到本地标签的站点标签分页
func (s *Service) QueryBoundOrUnboundToLocalTagPage(ctx context.Context, page *model.Page[sdkdto.SiteTagFullDTO], query SiteTagQueryDTO) (*model.Page[sdkdto.SiteTagFullDTO], error) {
	conv := querypkg.NewConverter(entity2.SiteTag{})

	var boundOnLocalTagId *bool
	if query.BoundOnLocalTagId.Value != nil {
		boundOnLocalTagId = query.BoundOnLocalTagId.Value
	}
	var localTagId *int64
	if query.LocalTagID.Value != nil {
		localTagId = query.LocalTagID.Value
		// 避免ToPageOption生成LocalTagID的默认条件
		query.LocalTagID.Value = nil
	}

	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}

	// 根据 boundOnLocalTagId 添加 localTagId 的过滤条件
	if localTagId != nil {
		if boundOnLocalTagId != nil && *boundOnLocalTagId {
			// 绑定到指定本地标签
			opt.Conditions = append(opt.Conditions, clause.Eq{Column: "local_tag_id", Value: *localTagId})
		} else if boundOnLocalTagId != nil && !*boundOnLocalTagId {
			// 未绑定到指定本地标签（包括绑定到其他本地标签或从未绑定过本地标签的）
			opt.Conditions = append(opt.Conditions, clause.Expr{SQL: "(local_tag_id != ? OR local_tag_id IS NULL)", Vars: []any{*localTagId}})
		}
	}

	rawPage, err := s.repo.Page(ctx, opt)
	if err != nil {
		return nil, err
	}

	// 填充关联数据
	return s.enrichSiteTagsWithRelations(ctx, rawPage)
}

// enrichSiteTagsWithRelations 批量填充站点标签的关联数据（本地标签和站点）
func (s *Service) enrichSiteTagsWithRelations(ctx context.Context, rawPage *model.Page[entity2.SiteTag]) (*model.Page[sdkdto.SiteTagFullDTO], error) {
	siteTags := rawPage.Data
	if len(siteTags) == 0 {
		return model.NewPage[sdkdto.SiteTagFullDTO](nil, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
	}

	// 收集需要查询的 LocalTagID 和 SiteID
	localTagIds := make([]int64, 0)
	siteIds := make([]int64, 0)
	for _, tag := range siteTags {
		if tag.LocalTagID.Valid && tag.LocalTagID.Int64 > 0 {
			localTagIds = append(localTagIds, tag.LocalTagID.Int64)
		}
		if tag.SiteID.Valid && tag.SiteID.Int64 > 0 {
			siteIds = append(siteIds, tag.SiteID.Int64)
		}
	}

	// 批量查询 LocalTag
	localTagMap := make(map[int64]*sdkdto.LocalTagDTO)
	if len(localTagIds) > 0 {
		localTags, err := s.localTagQueryOp.ListByIds(ctx, localTagIds)
		if err != nil {
			return nil, err
		}
		for _, lt := range localTags {
			localTagMap[lt.ID] = &sdkdto.LocalTagDTO{
				ID:             lt.GetID(),
				LocalTagName:   util.NullStringToPointer(lt.LocalTagName),
				BaseLocalTagID: util.NullInt64ToPointer(lt.BaseLocalTagID),
				Description:    util.NullStringToPointer(lt.Description),
				CreateTime:     lt.GetCreateTime(),
				UpdateTime:     lt.GetUpdateTime(),
			}
		}
	}

	// 批量查询 Site
	siteMap := make(map[int64]*sdkdto.SiteDTO)
	if len(siteIds) > 0 {
		sites, err := s.siteQueryOp.ListByIds(ctx, util.UniqueInt64(siteIds))
		if err != nil {
			return nil, err
		}
		for _, st := range sites {
			siteMap[st.ID] = dto2.NewSiteDTO(st)
		}
	}

	// 组装结果
	results := make([]*sdkdto.SiteTagFullDTO, 0, len(siteTags))
	for _, tag := range siteTags {
		dto := dto2.NewSiteTagFullDTO(tag)
		if tag.LocalTagID.Valid && tag.LocalTagID.Int64 > 0 {
			dto.LocalTag = localTagMap[tag.LocalTagID.Int64]
		}
		if tag.SiteID.Valid && tag.SiteID.Int64 > 0 {
			dto.Site = siteMap[tag.SiteID.Int64]
		}
		results = append(results, dto)
	}

	return model.NewPage[sdkdto.SiteTagFullDTO](results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}

// QueryPageByWorkId 根据作品ID分页查询站点标签
func (s *Service) QueryPageByWorkId(ctx context.Context, page *model.Page[sdkdto.SiteTagFullDTO], query SiteTagQueryDTO, workId int64, boundOnWorkId *bool) (*model.Page[sdkdto.SiteTagFullDTO], error) {
	conv := querypkg.NewConverter(entity2.SiteTag{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.QueryPageByWorkId(ctx, opt, workId, boundOnWorkId)
}

// QueryLocalRelateDTOPage 查询站点标签与本地标签关联DTO分页
func (s *Service) QueryLocalRelateDTOPage(ctx context.Context, page *model.Page[sdkdto.SiteTagLocalRelateDTO], query SiteTagQueryDTO, workId int64, boundOnWorkId *bool) (*model.Page[sdkdto.SiteTagLocalRelateDTO], error) {
	conv := querypkg.NewConverter(entity2.SiteTag{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.QueryLocalRelateDTOPage(ctx, opt, workId, boundOnWorkId)
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询站点标签选择项
func (s *Service) QuerySelectItemPageByWorkId(ctx context.Context, page *model.Page[sdkdto.SelectItem], query SiteTagQueryDTO, workId int64, boundOnWorkId *bool) (*model.Page[sdkdto.SelectItem], error) {
	conv := querypkg.NewConverter(entity2.SiteTag{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.QuerySelectItemPageByWorkId(ctx, opt, workId, boundOnWorkId)
}

// ListByWorkId 查询作品的站点标签
func (s *Service) ListByWorkId(ctx context.Context, workId int64) ([]*entity2.SiteTag, error) {
	return s.repo.ListByWorkId(ctx, workId)
}

// ListBySiteTagIds 根据站点标签ID列表查询
func (s *Service) ListBySiteTagIds(ctx context.Context, siteTagIds []int64) ([]*entity2.SiteTag, error) {
	return s.repo.ListBySiteTagIds(ctx, siteTagIds)
}

// UpdateBindLocalTag 绑定或解除本地标签绑定
func (s *Service) UpdateBindLocalTag(ctx context.Context, localTagId *int64, siteTagIds []int64) (bool, error) {
	if len(siteTagIds) == 0 {
		return true, nil
	}
	affected, err := s.repo.UpdateBindLocalTag(ctx, localTagId, siteTagIds)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// CreateAndBindSameNameLocalTag 创建并绑定同名本地标签
// 查找或创建同名本地标签，并绑定到站点标签
func (s *Service) CreateAndBindSameNameLocalTag(ctx context.Context, siteTag *entity2.SiteTag) (*entity2.LocalTag, error) {
	// 1. 查找同名本地标签
	localTag, err := s.localTagOperator.GetByName(ctx, siteTag.SiteTagName.String)
	if err != nil {
		return nil, err
	}

	// 2. 如果不存在，则创建
	if localTag == nil {
		localTag = entity2.NewLocalTag()
		localTag.LocalTagName = siteTag.SiteTagName
		localTag.BaseLocalTagID = sql.NullInt64{Int64: 0, Valid: true}
		localTag.LastUse = sql.NullInt64{Int64: util.GetCurrentTimestamp(), Valid: true}
		if err := s.localTagOperator.Save(ctx, localTag); err != nil {
			return nil, err
		}
	}

	// 3. 绑定站点标签到本地标签
	if _, err := s.repo.UpdateBindLocalTag(ctx, &localTag.ID, []int64{siteTag.ID}); err != nil {
		return nil, err
	}

	return localTag, nil
}

// 错误定义
var (
	ErrTagIdRequired = &pkgerr.BusinessError{Code: 400, Message: "更新站点标签失败，id不能为空"}
)
