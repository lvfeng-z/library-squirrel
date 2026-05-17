package work

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/util"
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

// ReWorkTagWriter 作品-标签关联写入接口
type ReWorkTagWriter interface {
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
	// SaveBatch 批量保存关联
	SaveBatch(ctx context.Context, rels []*entity2.ReWorkTag) error
}

// ReWorkWorkSetWriter 作品-作品集关联写入接口
type ReWorkWorkSetWriter interface {
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
	// SaveBatch 批量保存关联
	SaveBatch(ctx context.Context, rels []*entity2.ReWorkWorkSet) error
}

// SiteAuthorWriter 站点作者写入接口
type SiteAuthorWriter interface {
	// SaveOrUpdateByCompositeKey 按复合键保存或更新，返回内部 DB ID
	SaveOrUpdateByCompositeKey(ctx context.Context, author *entity2.SiteAuthor) (int64, error)
	// GetBySiteAndSiteAuthorID 根据站点ID和站点作者ID查询
	GetBySiteAndSiteAuthorID(ctx context.Context, siteId int64, siteAuthorId string) (*entity2.SiteAuthor, error)
}

// SiteTagWriter 站点标签写入接口
type SiteTagWriter interface {
	// SaveOrUpdateByCompositeKey 按复合键保存或更新，返回内部 DB ID
	SaveOrUpdateByCompositeKey(ctx context.Context, tag *entity2.SiteTag) (int64, error)
	// GetBySiteAndSiteTagID 根据站点ID和站点标签ID查询
	GetBySiteAndSiteTagID(ctx context.Context, siteId int64, siteTagId string) (*entity2.SiteTag, error)
}

// WorkSetWriter 作品集写入接口
type WorkSetWriter interface {
	// SaveOrUpdateByCompositeKey 按复合键保存或更新，返回内部 DB ID
	SaveOrUpdateByCompositeKey(ctx context.Context, ws *entity2.WorkSet) (int64, error)
	// GetBySiteAndSiteWorkSetID 根据站点ID和站点作品集ID查询
	GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*entity2.WorkSet, error)
}

// ReWorkAuthorWriter 作品-作者关联写入接口
type ReWorkAuthorWriter interface {
	// SaveBatch 批量保存关联
	SaveBatch(ctx context.Context, reWorkAuthors []*entity2.ReWorkAuthor) error
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
	localTagReader    LocalTagReader
	localAuthorReader LocalAuthorReader
	siteTagReader     SiteTagReader
	siteAuthorReader  SiteAuthorReader
	siteReader        SiteReader
	resourceReader    ResourceReader
	resourceDeleter   ResourceDeleter

	// 写入接口（用于 SaveWorkInfo）
	reWorkTagWriter     ReWorkTagWriter
	reWorkWorkSetWriter ReWorkWorkSetWriter
	siteAuthorWriter    SiteAuthorWriter
	siteTagWriter       SiteTagWriter
	workSetWriter       WorkSetWriter
	reWorkAuthorWriter  ReWorkAuthorWriter
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
	reWorkTagWriter ReWorkTagWriter,
	reWorkWorkSetWriter ReWorkWorkSetWriter,
	resourceDeleter ResourceDeleter,
	siteAuthorWriter SiteAuthorWriter,
	siteTagWriter SiteTagWriter,
	workSetWriter WorkSetWriter,
	reWorkAuthorWriter ReWorkAuthorWriter,
) *Service {
	return &Service{
		repo:                repo,
		localTagReader:      localTagReader,
		localAuthorReader:   localAuthorReader,
		siteTagReader:       siteTagReader,
		siteAuthorReader:    siteAuthorReader,
		siteReader:          siteReader,
		resourceReader:      resourceReader,
		resourceDeleter:     resourceDeleter,
		reWorkTagWriter:     reWorkTagWriter,
		reWorkWorkSetWriter: reWorkWorkSetWriter,
		siteAuthorWriter:    siteAuthorWriter,
		siteTagWriter:       siteTagWriter,
		workSetWriter:       workSetWriter,
		reWorkAuthorWriter:  reWorkAuthorWriter,
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
	if err := s.reWorkTagWriter.DeleteByWorkId(ctx, id); err != nil {
		return err
	}

	// 2. 删除作品关联的作品集关系
	if err := s.reWorkWorkSetWriter.DeleteByWorkId(ctx, id); err != nil {
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
					SiteAuthor: &dto2.SiteAuthorDTO{
						ID:                   ra.ID,
						CreateTime:           ra.CreateTime,
						UpdateTime:           ra.UpdateTime,
						SiteID:               util.Int64PtrIfValid(ra.SiteID),
						SiteAuthorID:         util.StringPtrIfValid(ra.SiteAuthorID),
						AuthorName:           util.StringPtrIfValid(ra.AuthorName),
						FixedAuthorName:      util.StringPtrIfValid(ra.FixedAuthorName),
						SiteAuthorNameBefore: util.StringPtrIfValid(ra.SiteAuthorNameBefore),
						Introduce:            util.StringPtrIfValid(ra.Introduce),
						LocalAuthorID:        util.Int64PtrIfValid(ra.LocalAuthorID),
						LastUse:              util.Int64PtrIfValid(ra.LastUse),
					},
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
	ErrWorkIdRequired = &pkgerr.BusinessError{Code: 400, Message: "更新作品失败，id不能为空"}
)

// AuthorType 常量（与 reWorkTag.TagType 对齐）
const (
	AuthorTypeLocal = 1
	AuthorTypeSite  = 2
)

// SaveWorkInfo 保存作品及全部周边数据，返回作品内部 DB ID
func (s *Service) SaveWorkInfo(ctx context.Context, task *entity2.Task, workResp *dto2.WorkResponse) (int64, error) {
	work := workResp.Work

	// 确保 SiteID 来自任务
	if task.SiteID.Valid {
		work.SiteID = task.SiteID
	}

	if !work.SiteID.Valid || work.SiteID.Int64 == 0 {
		return 0, fmt.Errorf("保存作品信息失败，siteId 不能为空，taskId: %d", task.ID)
	}
	if !work.SiteWorkID.Valid || work.SiteWorkID.String == "" {
		return 0, fmt.Errorf("保存作品信息失败，siteWorkId 不能为空，taskId: %d", task.ID)
	}
	siteId := work.SiteID.Int64

	// === Phase 1: upsert 周边主数据 ===
	siteAuthorDBIds, err := s.upsertSiteAuthors(ctx, workResp.SiteAuthors, siteId)
	if err != nil {
		return 0, fmt.Errorf("upsert 站点作者失败: %w", err)
	}

	siteTagDBIds, err := s.upsertSiteTags(ctx, workResp.SiteTags, siteId)
	if err != nil {
		return 0, fmt.Errorf("upsert 站点标签失败: %w", err)
	}

	workSetDBIds, err := s.upsertWorkSets(ctx, workResp.WorkSets, siteId)
	if err != nil {
		return 0, fmt.Errorf("upsert 作品集失败: %w", err)
	}

	// LocalAuthors/LocalTags 暂时忽略
	if len(workResp.LocalAuthors) > 0 {
		logger.Log.Warnf("[WorkService] 插件返回了 %d 个本地作者，暂未支持保存", len(workResp.LocalAuthors))
	}
	if len(workResp.LocalTags) > 0 {
		logger.Log.Warnf("[WorkService] 插件返回了 %d 个本地标签，暂未支持保存", len(workResp.LocalTags))
	}

	// === Phase 2: 回查内部 DB ID ===
	siteAuthorDBIds, err = s.querySiteAuthorDBIds(ctx, workResp.SiteAuthors, siteId)
	if err != nil {
		return 0, fmt.Errorf("回查站点作者 ID 失败: %w", err)
	}

	siteTagDBIds, err = s.querySiteTagDBIds(ctx, workResp.SiteTags, siteId)
	if err != nil {
		return 0, fmt.Errorf("回查站点标签 ID 失败: %w", err)
	}

	workSetDBIds, err = s.queryWorkSetDBIds(ctx, workResp.WorkSets, siteId)
	if err != nil {
		return 0, fmt.Errorf("回查作品集 ID 失败: %w", err)
	}

	// === Phase 3: 保存 Work + 全量替换关联 ===
	workId, err := s.saveOrUpdateWork(ctx, work)
	if err != nil {
		return 0, fmt.Errorf("保存作品失败: %w", err)
	}

	// 全量替换 work-author 关联
	if err := s.reWorkAuthorWriter.DeleteByWorkId(ctx, workId); err != nil {
		return 0, fmt.Errorf("删除作品作者关联失败: %w", err)
	}
	if len(siteAuthorDBIds) > 0 {
		links := buildSiteAuthorLinks(workId, siteAuthorDBIds)
		if err := s.reWorkAuthorWriter.SaveBatch(ctx, links); err != nil {
			return 0, fmt.Errorf("保存作品作者关联失败: %w", err)
		}
	}

	// 全量替换 work-tag 关联
	if err := s.reWorkTagWriter.DeleteByWorkId(ctx, workId); err != nil {
		return 0, fmt.Errorf("删除作品标签关联失败: %w", err)
	}
	if len(siteTagDBIds) > 0 {
		links := buildSiteTagLinks(workId, siteTagDBIds)
		if err := s.reWorkTagWriter.SaveBatch(ctx, links); err != nil {
			return 0, fmt.Errorf("保存作品标签关联失败: %w", err)
		}
	}

	// 全量替换 work-workset 关联
	if err := s.reWorkWorkSetWriter.DeleteByWorkId(ctx, workId); err != nil {
		return 0, fmt.Errorf("删除作品作品集关联失败: %w", err)
	}
	if len(workSetDBIds) > 0 {
		links := buildWorkSetLinks(workId, workSetDBIds)
		if err := s.reWorkWorkSetWriter.SaveBatch(ctx, links); err != nil {
			return 0, fmt.Errorf("保存作品作品集关联失败: %w", err)
		}
	}

	return workId, nil
}

// upsertSiteAuthors 批量 upsert 站点作者，返回 DB ID 列表
func (s *Service) upsertSiteAuthors(ctx context.Context, dtos []*dto2.TaskSiteAuthorDTO, siteId int64) ([]int64, error) {
	if len(dtos) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(dtos))
	for _, d := range dtos {
		entity := taskSiteAuthorDTOToEntity(d, siteId)
		id, err := s.siteAuthorWriter.SaveOrUpdateByCompositeKey(ctx, entity)
		if err != nil {
			return nil, fmt.Errorf("upsert 站点作者 %s 失败: %w", d.SiteAuthorID, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// upsertSiteTags 批量 upsert 站点标签
func (s *Service) upsertSiteTags(ctx context.Context, dtos []*dto2.TaskSiteTagDTO, siteId int64) ([]int64, error) {
	if len(dtos) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(dtos))
	for _, d := range dtos {
		entity := taskSiteTagDTOToEntity(d, siteId)
		id, err := s.siteTagWriter.SaveOrUpdateByCompositeKey(ctx, entity)
		if err != nil {
			return nil, fmt.Errorf("upsert 站点标签 %s 失败: %w", d.SiteTagID, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// upsertWorkSets 批量 upsert 作品集
func (s *Service) upsertWorkSets(ctx context.Context, dtos []*dto2.TaskWorkSetDTO, siteId int64) ([]int64, error) {
	if len(dtos) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(dtos))
	for _, d := range dtos {
		entity := taskWorkSetDTOToEntity(d, siteId)
		id, err := s.workSetWriter.SaveOrUpdateByCompositeKey(ctx, entity)
		if err != nil {
			return nil, fmt.Errorf("upsert 作品集 %d 失败: %w", d.WorkSetID, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// querySiteAuthorDBIds 回查站点作者内部 DB ID
func (s *Service) querySiteAuthorDBIds(ctx context.Context, dtos []*dto2.TaskSiteAuthorDTO, siteId int64) ([]int64, error) {
	if len(dtos) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(dtos))
	for _, d := range dtos {
		record, err := s.siteAuthorWriter.GetBySiteAndSiteAuthorID(ctx, siteId, d.SiteAuthorID)
		if err != nil {
			return nil, fmt.Errorf("回查站点作者 %s 失败: %w", d.SiteAuthorID, err)
		}
		ids = append(ids, record.ID)
	}
	return ids, nil
}

// querySiteTagDBIds 回查站点标签内部 DB ID
func (s *Service) querySiteTagDBIds(ctx context.Context, dtos []*dto2.TaskSiteTagDTO, siteId int64) ([]int64, error) {
	if len(dtos) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(dtos))
	for _, d := range dtos {
		record, err := s.siteTagWriter.GetBySiteAndSiteTagID(ctx, siteId, d.SiteTagID)
		if err != nil {
			return nil, fmt.Errorf("回查站点标签 %s 失败: %w", d.SiteTagID, err)
		}
		ids = append(ids, record.ID)
	}
	return ids, nil
}

// queryWorkSetDBIds 回查作品集内部 DB ID
func (s *Service) queryWorkSetDBIds(ctx context.Context, dtos []*dto2.TaskWorkSetDTO, siteId int64) ([]int64, error) {
	if len(dtos) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(dtos))
	for _, d := range dtos {
		siteWorkSetId := strconv.FormatInt(d.WorkSetID, 10)
		record, err := s.workSetWriter.GetBySiteAndSiteWorkSetID(ctx, siteId, siteWorkSetId)
		if err != nil {
			return nil, fmt.Errorf("回查作品集 %d 失败: %w", d.WorkSetID, err)
		}
		ids = append(ids, record.ID)
	}
	return ids, nil
}

// saveOrUpdateWork 按复合键保存或更新作品
func (s *Service) saveOrUpdateWork(ctx context.Context, work *entity2.Work) (int64, error) {
	existing, err := s.repo.GetBySiteAndSiteWorkID(ctx, work.SiteID.Int64, work.SiteWorkID.String)
	if err == nil && existing != nil {
		work.ID = existing.ID
		if err := s.repo.Update(ctx, work); err != nil {
			return 0, err
		}
		return existing.ID, nil
	}
	if err := s.repo.Save(ctx, work); err != nil {
		return 0, err
	}
	return work.ID, nil
}

// ========== DTO 转换辅助函数 ==========

func taskSiteAuthorDTOToEntity(d *dto2.TaskSiteAuthorDTO, siteId int64) *entity2.SiteAuthor {
	return &entity2.SiteAuthor{
		BaseEntity:    &model.BaseEntity{},
		SiteID:        sql.NullInt64{Int64: siteId, Valid: true},
		SiteAuthorID:  sql.NullString{String: d.SiteAuthorID, Valid: true},
		AuthorName:    sql.NullString{String: d.AuthorName, Valid: true},
	}
}

func taskSiteTagDTOToEntity(d *dto2.TaskSiteTagDTO, siteId int64) *entity2.SiteTag {
	return &entity2.SiteTag{
		BaseEntity:  &model.BaseEntity{},
		SiteID:      sql.NullInt64{Int64: siteId, Valid: true},
		SiteTagID:   sql.NullString{String: d.SiteTagID, Valid: true},
		SiteTagName: sql.NullString{String: d.TagName, Valid: true},
	}
}

func taskWorkSetDTOToEntity(d *dto2.TaskWorkSetDTO, siteId int64) *entity2.WorkSet {
	return &entity2.WorkSet{
		BaseEntity:         &model.BaseEntity{},
		SiteID:             sql.NullInt64{Int64: siteId, Valid: true},
		SiteWorkSetID:      sql.NullString{String: strconv.FormatInt(d.WorkSetID, 10), Valid: true},
		SiteWorkSetName:    sql.NullString{String: d.WorkSetName, Valid: true},
	}
}

// ========== 关联实体构建辅助函数 ==========

func buildSiteAuthorLinks(workId int64, siteAuthorIds []int64) []*entity2.ReWorkAuthor {
	links := make([]*entity2.ReWorkAuthor, 0, len(siteAuthorIds))
	for _, authorId := range siteAuthorIds {
		links = append(links, &entity2.ReWorkAuthor{
			BaseEntity:   &model.BaseEntity{},
			AuthorType:   sql.NullInt64{Int64: AuthorTypeSite, Valid: true},
			WorkID:       sql.NullInt64{Int64: workId, Valid: true},
			SiteAuthorID: sql.NullInt64{Int64: authorId, Valid: true},
			AuthorRank:   sql.NullInt64{Int64: 0, Valid: true},
		})
	}
	return links
}

func buildSiteTagLinks(workId int64, siteTagIds []int64) []*entity2.ReWorkTag {
	links := make([]*entity2.ReWorkTag, 0, len(siteTagIds))
	for _, tagId := range siteTagIds {
		links = append(links, &entity2.ReWorkTag{
			BaseEntity: &model.BaseEntity{},
			WorkID:     sql.NullInt64{Int64: workId, Valid: true},
			TagType:    sql.NullInt64{Int64: 2, Valid: true}, // TagTypeSite
			SiteTagID:  sql.NullInt64{Int64: tagId, Valid: true},
		})
	}
	return links
}

func buildWorkSetLinks(workId int64, workSetIds []int64) []*entity2.ReWorkWorkSet {
	links := make([]*entity2.ReWorkWorkSet, 0, len(workSetIds))
	for i, wsId := range workSetIds {
		links = append(links, &entity2.ReWorkWorkSet{
			BaseEntity: &model.BaseEntity{},
			WorkID:     sql.NullInt64{Int64: workId, Valid: true},
			WorkSetID:  sql.NullInt64{Int64: wsId, Valid: true},
			SortOrder:  sql.NullInt64{Int64: int64(i), Valid: true},
		})
	}
	return links
}
