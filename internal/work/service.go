package work

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model"
	dto2 "github.com/library-squirrel/wails/pkg/model/dto"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
	querypkg "github.com/library-squirrel/wails/pkg/query"
)

// ========== 外部模块接口定义（由 work 模块定义自己需要的接口）==========

// LocalTagReader 本地标签读取接口
type LocalTagReader interface {
	// ListByWorkId 查询作品关联的本地标签
	ListByWorkId(ctx context.Context, workId int64) ([]*entity2.LocalTag, error)
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity2.LocalTag, error)
}

// LocalAuthorReader 本地作者读取接口
type LocalAuthorReader interface {
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity2.LocalAuthor, error)
}

// SiteTagReader 站点标签读取接口
type SiteTagReader interface {
	// ListByWorkId 查询作品关联的站点标签
	ListByWorkId(ctx context.Context, workId int64) ([]*entity2.SiteTag, error)
}

// SiteAuthorReader 站点作者读取接口
type SiteAuthorReader interface {
	// ListByWorkId 查询作品关联的站点作者
	ListByWorkId(ctx context.Context, workId int64) ([]*dto2.RankedSiteAuthor, error)
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity2.SiteAuthor, error)
}

// SiteReader 站点读取接口
type SiteReader interface {
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity2.Site, error)
}

// ResourceReader 资源读取接口
type ResourceReader interface {
	// ListByWorkId 查询作品关联的资源
	ListByWorkId(ctx context.Context, workId int64) ([]*entity2.Resource, error)
}

// ReWorkTagDeleter 作品-标签关联删除接口
type ReWorkTagDeleter interface {
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
}

// ReWorkWorkSetDeleter 作品-作品集关联删除接口
type ReWorkWorkSetDeleter interface {
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
}

// ResourceDeleter 资源删除接口
type ResourceDeleter interface {
	// DeleteByWorkId 根据作品ID删除所有资源
	DeleteByWorkId(ctx context.Context, workId int64) error
}

// Repository 作品仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Save 保存
	Save(ctx context.Context, work *entity2.Work) error
	// Update 更新
	Update(ctx context.Context, work *entity2.Work) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity2.Work, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*entity2.Work, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity2.Work], error)
	// GetBySiteAndSiteWorkID 根据站点和站点作品ID查询
	GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*entity2.Work, error)
	// ListByIds 根据ID列表批量查询
	ListByIds(ctx context.Context, ids []int64) ([]*entity2.Work, error)
	// UpdateLastViewBatch 批量更新最后查看时间
	UpdateLastViewBatch(ctx context.Context, ids []int64, lastView int64) error
}

// Service 作品服务
type Service struct {
	repo Repository

	// 外部模块依赖（通过构造函数注入）
	localTagReader       LocalTagReader
	localAuthorReader    LocalAuthorReader
	siteTagReader        SiteTagReader
	siteAuthorReader     SiteAuthorReader
	siteReader           SiteReader
	resourceReader       ResourceReader
	reWorkTagDeleter     ReWorkTagDeleter
	reWorkWorkSetDeleter ReWorkWorkSetDeleter
	resourceDeleter      ResourceDeleter
}

// NewService 创建作品服务
func NewService(
	repo Repository,
	localTagReader LocalTagReader,
	localAuthorReader LocalAuthorReader,
	siteTagReader SiteTagReader,
	siteAuthorReader SiteAuthorReader,
	siteReader SiteReader,
	resourceReader ResourceReader,
	reWorkTagDeleter ReWorkTagDeleter,
	reWorkWorkSetDeleter ReWorkWorkSetDeleter,
	resourceDeleter ResourceDeleter,
) *Service {
	return &Service{
		repo:                 repo,
		localTagReader:       localTagReader,
		localAuthorReader:    localAuthorReader,
		siteTagReader:        siteTagReader,
		siteAuthorReader:     siteAuthorReader,
		siteReader:           siteReader,
		resourceReader:       resourceReader,
		reWorkTagDeleter:     reWorkTagDeleter,
		reWorkWorkSetDeleter: reWorkWorkSetDeleter,
		resourceDeleter:      resourceDeleter,
	}
}

// Save 保存作品
func (s *Service) Save(ctx context.Context, work *entity2.Work) error {
	return s.repo.Save(ctx, work)
}

// UpdateById 更新作品
func (s *Service) UpdateById(ctx context.Context, work *entity2.Work) error {
	if work.ID == 0 {
		return ErrWorkIdRequired
	}
	return s.repo.Update(ctx, work)
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*entity2.Work, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*entity2.Work, error) {
	return s.repo.List(ctx, opt)
}

// ListByIds 根据ID列表批量查询
func (s *Service) ListByIds(ctx context.Context, ids []int64) ([]*entity2.Work, error) {
	return s.repo.ListByIds(ctx, ids)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// Delete 删除作品
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// DeleteWorkAndSurroundingData 删除作品及其周围数据（级联删除）
func (s *Service) DeleteWorkAndSurroundingData(ctx context.Context, id int64) error {
	// 1. 删除作品关联的标签
	if err := s.reWorkTagDeleter.DeleteByWorkId(ctx, id); err != nil {
		return err
	}

	// 2. 删除作品关联的作品集关系
	if err := s.reWorkWorkSetDeleter.DeleteByWorkId(ctx, id); err != nil {
		return err
	}

	// 3. 删除作品关联的资源
	if err := s.resourceDeleter.DeleteByWorkId(ctx, id); err != nil {
		return err
	}

	// 4. 删除作品本身
	return s.repo.Delete(ctx, id)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, page *model.Page[entity2.Work], query WorkQueryDTO) (*model.Page[entity2.Work], error) {
	conv := querypkg.NewConverter(entity2.Work{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
}

// GetBySiteAndSiteWorkID 根据站点和站点作品ID查询
func (s *Service) GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*entity2.Work, error) {
	return s.repo.GetBySiteAndSiteWorkID(ctx, siteId, siteWorkId)
}

// GetFullWorkInfoById 获取作品完整信息
func (s *Service) GetFullWorkInfoById(ctx context.Context, id int64) (*dto2.WorkFullDTO, error) {
	// 获取作品基本信息
	work, err := s.repo.GetById(ctx, id)
	if err != nil {
		return nil, err
	}

	// 构建 WorkFullDTO
	fullDTO := dto2.NewWorkFullDTO(work)

	// 获取本地作者信息
	if work.LocalAuthorID.Valid && work.LocalAuthorID.Int64 > 0 {
		localAuthor, err := s.localAuthorReader.GetById(ctx, work.LocalAuthorID.Int64)
		if err == nil && localAuthor != nil {
			fullDTO.LocalAuthors = []*dto2.LocalAuthorDTO{dto2.NewLocalAuthorDTO(localAuthor)}
		}
	}

	// 获取站点作者信息
	if work.SiteAuthorID.Valid && work.SiteAuthorID.String != "" {
		rankedAuthors, err := s.siteAuthorReader.ListByWorkId(ctx, id)
		if err == nil && len(rankedAuthors) > 0 {
			fullDTO.SiteAuthors = make([]*dto2.SiteAuthorFullDTO, 0, len(rankedAuthors))
			for _, ra := range rankedAuthors {
				if ra == nil {
					continue
				}
				fullDTO.SiteAuthors = append(fullDTO.SiteAuthors, &dto2.SiteAuthorFullDTO{
					ID:                   ra.ID,
					CreateTime:           ra.CreateTime,
					UpdateTime:           ra.UpdateTime,
					SiteID:               ra.SiteID,
					SiteAuthorID:         ra.SiteAuthorID,
					AuthorName:           ra.AuthorName,
					FixedAuthorName:      ra.FixedAuthorName,
					SiteAuthorNameBefore: ra.SiteAuthorNameBefore,
					Introduce:            ra.Introduce,
					LocalAuthorID:        ra.LocalAuthorID,
					LastUse:              ra.LastUse,
				})
			}
		}
	}

	// 获取站点信息
	if work.SiteID.Valid && work.SiteID.Int64 > 0 {
		site, err := s.siteReader.GetById(ctx, work.SiteID.Int64)
		if err == nil && site != nil {
			fullDTO.Site = dto2.NewSiteDTO(site)
		}
	}

	// 获取本地标签信息
	localTags, err := s.localTagReader.ListByWorkId(ctx, id)
	if err == nil && len(localTags) > 0 {
		fullDTO.LocalTags = make([]*dto2.LocalTagDTO, len(localTags))
		for i, tag := range localTags {
			fullDTO.LocalTags[i] = dto2.NewLocalTagDTO(tag)
		}
	}

	// 获取站点标签信息
	siteTags, err := s.siteTagReader.ListByWorkId(ctx, id)
	if err == nil && len(siteTags) > 0 {
		fullDTO.SiteTags = make([]*dto2.SiteTagFullDTO, len(siteTags))
		for i, tag := range siteTags {
			fullDTO.SiteTags[i] = dto2.NewSiteTagFullDTO(tag)
		}
	}

	// 获取资源信息
	resources, err := s.resourceReader.ListByWorkId(ctx, id)
	if err == nil && len(resources) > 0 {
		fullDTO.Resources = make([]*dto2.ResourceDTO, len(resources))
		for i, res := range resources {
			fullDTO.Resources[i] = dto2.NewResourceDTO(res)
		}
	}

	return fullDTO, nil
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 根据作品ID列表获取带排名的本地作者
func (s *Service) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto2.RankedLocalAuthor, error) {
	if len(workIds) == 0 {
		return []*dto2.RankedLocalAuthor{}, nil
	}
	// 获取作品列表
	works, err := s.repo.ListByIds(ctx, workIds)
	if err != nil {
		return nil, err
	}

	// 收集所有本地作者ID
	authorMap := make(map[int64]*dto2.RankedLocalAuthor)
	for _, work := range works {
		if work.LocalAuthorID.Valid && work.LocalAuthorID.Int64 > 0 {
			localAuthorId := work.LocalAuthorID.Int64
			if _, exists := authorMap[localAuthorId]; !exists {
				localAuthor, err := s.localAuthorReader.GetById(ctx, localAuthorId)
				if err == nil && localAuthor != nil {
					authorName := ""
					if localAuthor.AuthorName.Valid {
						authorName = localAuthor.AuthorName.String
					}
					introduce := ""
					if localAuthor.Introduce.Valid {
						introduce = localAuthor.Introduce.String
					}
					lastUse := int64(0)
					if localAuthor.LastUse.Valid {
						lastUse = localAuthor.LastUse.Int64
					}
					authorMap[localAuthorId] = &dto2.RankedLocalAuthor{
						ID:         localAuthor.ID,
						AuthorName: authorName,
						Introduce:  introduce,
						LastUse:    lastUse,
						CreateTime: localAuthor.CreateTime,
						UpdateTime: localAuthor.UpdateTime,
					}
				}
			}
		}
	}

	// 转换为列表
	result := make([]*dto2.RankedLocalAuthor, 0, len(authorMap))
	for _, author := range authorMap {
		result = append(result, author)
	}
	return result, nil
}

// UpdateLastView 批量更新作品最后使用时间
func (s *Service) UpdateLastView(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return s.repo.UpdateLastViewBatch(ctx, ids, util.GetCurrentTimestamp())
}

// WorkAuthorDTO 作品作者信息
type WorkAuthorDTO struct {
	LocalAuthor *dto2.RankedLocalAuthor `json:"localAuthor,omitempty"`
	SiteAuthor  *dto2.RankedSiteAuthor  `json:"siteAuthor,omitempty"`
}

// ErrWorkIdRequired 错误定义
var (
	ErrWorkIdRequired = &BusinessError{Code: 400, Message: "更新作品失败，id不能为空"}
)

// BusinessError 业务错误
type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string {
	return e.Message
}
