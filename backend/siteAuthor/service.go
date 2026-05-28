package siteAuthor

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
	"gorm.io/gorm/clause"
)

// Repository 站点作者仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Save 保存
	Save(ctx context.Context, author *entity.SiteAuthor) error
	// SaveBatch 批量保存
	SaveBatch(ctx context.Context, authors []*entity.SiteAuthor) error
	// Update 更新
	Update(ctx context.Context, author *entity.SiteAuthor) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity.SiteAuthor, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*entity.SiteAuthor, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity.SiteAuthor], error)
	// ListByWorkId 查询作品的站点作者
	ListByWorkId(ctx context.Context, workId int64) ([]*sdkdto.RankedSiteAuthor, error)
	// ListBySiteAuthorIds 根据站点作者ID列表查询
	ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) ([]*entity.SiteAuthor, error)
	// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表
	ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*sdkdto.RankedSiteAuthorWithWorkId, error)
	// UpdateBindLocalAuthor 绑定本地作者
	UpdateBindLocalAuthor(ctx context.Context, localAuthorId *int64, siteAuthorIds []int64) (int64, error)
	// UpdateLastUseByIds 批量更新最后使用时间
	UpdateLastUseByIds(ctx context.Context, ids []int64, lastUse int64) error
	// GetBySiteAndSiteAuthorID 根据站点ID和站点作者ID查询
	GetBySiteAndSiteAuthorID(ctx context.Context, siteId int64, siteAuthorId string) (*entity.SiteAuthor, error)
	// Upsert 原子插入或更新
	Upsert(ctx context.Context, author *entity.SiteAuthor) error
}

// LocalAuthorOperator 本地作者接口
type LocalAuthorOperator interface {
	ListByIds(ctx context.Context, ids []int64) ([]*entity.LocalAuthor, error)
	GetByName(ctx context.Context, name string) (*entity.LocalAuthor, error)
	GetByNames(ctx context.Context, names []string) ([]*entity.LocalAuthor, error)
	Save(ctx context.Context, author *entity.LocalAuthor) error
}

// SiteOperator 站点接口
type SiteOperator interface {
	ListByIds(ctx context.Context, ids []int64) ([]*entity.Site, error)
}

// Service 站点作者服务
type Service struct {
	repo          Repository
	localAuthorOp LocalAuthorOperator
	siteOp        SiteOperator
}

// NewService 创建站点作者服务
func NewService(repo Repository, localAuthorQueryOp LocalAuthorOperator, siteOp SiteOperator) *Service {
	return &Service{
		repo:          repo,
		localAuthorOp: localAuthorQueryOp,
		siteOp:        siteOp,
	}
}

// Save 保存站点作者
func (s *Service) Save(ctx context.Context, author *entity.SiteAuthor) error {
	return s.repo.Save(ctx, author)
}

// SaveBatch 批量保存站点作者
func (s *Service) SaveBatch(ctx context.Context, authors []*entity.SiteAuthor) error {
	return s.repo.SaveBatch(ctx, authors)
}

// UpdateById 更新站点作者
func (s *Service) UpdateById(ctx context.Context, author *entity.SiteAuthor) error {
	if author.ID == 0 {
		return ErrAuthorIdRequired
	}
	return s.repo.Update(ctx, author)
}

// UpdateLastUse 批量更新最后使用时间
func (s *Service) UpdateLastUse(ctx context.Context, ids []int64) error {
	now := util.GetCurrentTimestamp()
	return s.repo.UpdateLastUseByIds(ctx, ids, now)
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*entity.SiteAuthor, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*entity.SiteAuthor, error) {
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
func (s *Service) Page(ctx context.Context, page *model.Page[entity.SiteAuthor], query SiteAuthorQueryDTO) (*model.Page[entity.SiteAuthor], error) {
	conv := querypkg.NewConverter(entity.SiteAuthor{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
}

// QueryBoundOrUnboundToLocalAuthorPage 查询绑定或未绑定到本地作者的站点作者分页
func (s *Service) QueryBoundOrUnboundToLocalAuthorPage(ctx context.Context, page *model.Page[sdkdto.SiteAuthorLocalRelateDTO], query SiteAuthorQueryDTO) (*model.Page[sdkdto.SiteAuthorLocalRelateDTO], error) {
	conv := querypkg.NewConverter(entity.SiteAuthor{})

	var boundOnLocalAuthorId *bool
	if query.BoundOnLocalAuthorId.Value != nil {
		boundOnLocalAuthorId = query.BoundOnLocalAuthorId.Value
	}
	var localAuthorId *int64
	if query.LocalAuthorID.Value != nil {
		localAuthorId = query.LocalAuthorID.Value
		// 避免ToPageOption生成LocalAuthorID的默认条件
		query.LocalAuthorID.Value = nil
	}

	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}

	// 根据 boundOnLocalAuthorId 添加 localAuthorId 的过滤条件
	if localAuthorId != nil {
		if boundOnLocalAuthorId != nil && *boundOnLocalAuthorId {
			// 绑定到指定本地作者
			opt.Conditions = append(opt.Conditions, clause.Eq{Column: "local_author_id", Value: *localAuthorId})
		} else if boundOnLocalAuthorId != nil && !*boundOnLocalAuthorId {
			// 未绑定到指定本地作者（包括绑定到其他本地作者或从未绑定过本地作者的）
			opt.Conditions = append(opt.Conditions, clause.Expr{SQL: "(local_author_id != ? OR local_author_id IS NULL)", Vars: []any{*localAuthorId}})
		}
	}

	rawPage, err := s.repo.Page(ctx, opt)
	if err != nil {
		return nil, err
	}

	// 填充关联数据（含 HasSameNameLocalAuthor）
	return s.enrichLocalRelateDTO(ctx, rawPage)
}

// QueryLocalRelateDTOPage 查询站点作者与本地作者关联DTO分页
func (s *Service) QueryLocalRelateDTOPage(ctx context.Context, page *model.Page[sdkdto.SiteAuthorLocalRelateDTO], query SiteAuthorQueryDTO) (*model.Page[sdkdto.SiteAuthorLocalRelateDTO], error) {
	conv := querypkg.NewConverter(entity.SiteAuthor{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}

	rawPage, err := s.repo.Page(ctx, opt)
	if err != nil {
		return nil, err
	}

	return s.enrichLocalRelateDTO(ctx, rawPage)
}

// enrichLocalRelateDTO 批量填充站点作者关联DTO的关联数据
func (s *Service) enrichLocalRelateDTO(ctx context.Context, rawPage *model.Page[entity.SiteAuthor]) (*model.Page[sdkdto.SiteAuthorLocalRelateDTO], error) {
	siteAuthors := rawPage.Data
	if len(siteAuthors) == 0 {
		return model.NewPage[sdkdto.SiteAuthorLocalRelateDTO](nil, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
	}

	// 收集关联 ID 和作者名称
	localAuthorIds := make([]int64, 0)
	siteIds := make([]int64, 0)
	authorNames := make([]string, 0)
	for _, author := range siteAuthors {
		if author.LocalAuthorID.Valid && author.LocalAuthorID.Int64 > 0 {
			localAuthorIds = append(localAuthorIds, author.LocalAuthorID.Int64)
		}
		if author.SiteID.Valid && author.SiteID.Int64 > 0 {
			siteIds = append(siteIds, author.SiteID.Int64)
		}
		if author.AuthorName.Valid && author.AuthorName.String != "" {
			authorNames = append(authorNames, author.AuthorName.String)
		}
	}

	// 批量查询 LocalAuthor by IDs
	localAuthorMap := make(map[int64]*sdkdto.LocalAuthorDTO)
	if len(localAuthorIds) > 0 {
		localAuthors, err := s.localAuthorOp.ListByIds(ctx, localAuthorIds)
		if err != nil {
			return nil, err
		}
		for _, lt := range localAuthors {
			localAuthorMap[lt.ID] = &sdkdto.LocalAuthorDTO{
				ID:         lt.GetID(),
				AuthorName: util.NullStringToPointer(lt.AuthorName),
				Introduce:  util.NullStringToPointer(lt.Introduce),
				CreateTime: lt.GetCreateTime(),
				UpdateTime: lt.GetUpdateTime(),
			}
		}
	}

	// 批量查询 Site
	siteMap := make(map[int64]*sdkdto.SiteDTO)
	if len(siteIds) > 0 {
		sites, err := s.siteOp.ListByIds(ctx, util.UniqueInt64(siteIds))
		if err != nil {
			return nil, err
		}
		for _, st := range sites {
			siteMap[st.ID] = dto.NewSiteDTO(st)
		}
	}

	// 批量检查 HasSameNameLocalAuthor
	sameNameMap := make(map[string]bool)
	if len(authorNames) > 0 {
		uniqueNames := util.UniqueString(authorNames)
		localAuthors, err := s.localAuthorOp.GetByNames(ctx, uniqueNames)
		if err != nil {
			return nil, err
		}
		for _, la := range localAuthors {
			if la.AuthorName.Valid {
				sameNameMap[la.AuthorName.String] = true
			}
		}
	}

	// 组装结果
	results := make([]*sdkdto.SiteAuthorLocalRelateDTO, 0, len(siteAuthors))
	for _, author := range siteAuthors {
		relateDTO := dto.NewSiteAuthorLocalRelateDTO(author)
		if author.LocalAuthorID.Valid && author.LocalAuthorID.Int64 > 0 {
			relateDTO.LocalAuthor = localAuthorMap[author.LocalAuthorID.Int64]
		}
		if author.SiteID.Valid && author.SiteID.Int64 > 0 {
			relateDTO.Site = siteMap[author.SiteID.Int64]
		}
		if author.AuthorName.Valid {
			relateDTO.HasSameNameLocalAuthor = sameNameMap[author.AuthorName.String]
		}
		results = append(results, relateDTO)
	}

	return model.NewPage[sdkdto.SiteAuthorLocalRelateDTO](results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}

// ListByWorkId 查询作品的站点作者
func (s *Service) ListByWorkId(ctx context.Context, workId int64) ([]*sdkdto.RankedSiteAuthor, error) {
	return s.repo.ListByWorkId(ctx, workId)
}

// ListBySiteAuthorIds 根据站点作者ID列表查询
func (s *Service) ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) ([]*entity.SiteAuthor, error) {
	return s.repo.ListBySiteAuthorIds(ctx, siteAuthorIds)
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表
func (s *Service) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*sdkdto.RankedSiteAuthorWithWorkId, error) {
	return s.repo.ListRankedSiteAuthorWithWorkIdByWorkIds(ctx, workIds)
}

// UpdateBindLocalAuthor 绑定或解除本地作者绑定
func (s *Service) UpdateBindLocalAuthor(ctx context.Context, localAuthorId *int64, siteAuthorIds []int64) (bool, error) {
	if len(siteAuthorIds) == 0 {
		return true, nil
	}
	affected, err := s.repo.UpdateBindLocalAuthor(ctx, localAuthorId, siteAuthorIds)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// CreateSameNameLocalAuthor 创建或获取同名的本地作者
func (s *Service) CreateSameNameLocalAuthor(ctx context.Context, siteAuthor *entity.SiteAuthor) (int64, error) {
	if !siteAuthor.AuthorName.Valid {
		return 0, nil
	}
	// 查询是否已有同名作者（通过依赖注入的 LocalAuthorOperator）
	existing, err := s.localAuthorOp.GetByName(ctx, siteAuthor.AuthorName.String)
	if err == nil && existing != nil {
		return existing.ID, nil
	}

	// 新增同名作者（通过依赖注入的 LocalAuthorOperator）
	newLocalAuthor := entity.NewLocalAuthor()
	newLocalAuthor.AuthorName = siteAuthor.AuthorName
	newLocalAuthor.Introduce = siteAuthor.Introduce

	if err := s.localAuthorOp.Save(ctx, newLocalAuthor); err != nil {
		return 0, err
	}
	return newLocalAuthor.ID, nil
}

// CreateAndBindSameNameLocalAuthor 创建并绑定同名的本地作者
func (s *Service) CreateAndBindSameNameLocalAuthor(ctx context.Context, siteAuthor *entity.SiteAuthor) (bool, error) {
	if siteAuthor.ID == 0 {
		return false, pkgerr.NewBusinessError(400, "创建同名本地作者失败，作者ID不能为空")
	}
	if !siteAuthor.AuthorName.Valid || siteAuthor.AuthorName.String == "" {
		return false, pkgerr.NewBusinessError(400, "创建同名本地作者失败，作者名称不能为空")
	}

	localAuthorId, err := s.CreateSameNameLocalAuthor(ctx, siteAuthor)
	if err != nil {
		return false, err
	}

	return s.UpdateBindLocalAuthor(ctx, &localAuthorId, []int64{siteAuthor.ID})
}

// GetBySiteAndSiteAuthorID 根据站点ID和站点作者ID查询
func (s *Service) GetBySiteAndSiteAuthorID(ctx context.Context, siteId int64, siteAuthorId string) (*entity.SiteAuthor, error) {
	return s.repo.GetBySiteAndSiteAuthorID(ctx, siteId, siteAuthorId)
}

// SaveOrUpdateByCompositeKey 按 (siteId, siteAuthorId) 原子保存或更新站点作者，返回内部 DB ID
func (s *Service) SaveOrUpdateByCompositeKey(ctx context.Context, author *entity.SiteAuthor) (int64, error) {
	if err := s.repo.Upsert(ctx, author); err != nil {
		return 0, err
	}
	if author.ID > 0 {
		return author.ID, nil
	}
	// OnConflict 更新场景下 ID 可能未回填，查询获取
	existing, err := s.repo.GetBySiteAndSiteAuthorID(ctx, author.SiteID.Int64, author.SiteAuthorID.String)
	if err != nil {
		return 0, err
	}
	return existing.ID, nil
}

// 错误定义
var (
	ErrAuthorIdRequired = &pkgerr.BusinessError{Code: 400, Message: "更新站点作者失败，id不能为空"}
)
