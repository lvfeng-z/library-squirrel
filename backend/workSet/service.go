package workSet

import (
	"context"
	"database/sql"
	"errors"

	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"

	"gorm.io/gorm/clause"
)

// ========== 外部模块接口定义（由 workSet 模块定义自己需要的接口）==========

// Repository 作品集仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Save 保存
	Save(ctx context.Context, workSet *entity2.WorkSet) error
	// Update 更新
	Update(ctx context.Context, workSet *entity2.WorkSet) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity2.WorkSet, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*entity2.WorkSet, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity2.WorkSet], error)
	// GetBySiteAndSiteWorkSetID 根据站点和站点作品集ID查询
	GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*entity2.WorkSet, error)
	// GetBySiteWorkSetIdAndSiteName 根据站点作品集ID和站点名称查询
	GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) (*entity2.WorkSet, error)
	// Upsert 原子插入或更新
	Upsert(ctx context.Context, ws *entity2.WorkSet) error
}

// FullWorkReader 作品完整信息读取接口
type FullWorkReader interface {
	// GetFullWorkInfoByIds 批量获取作品完整信息（含资源、作者、标签、站点）
	GetFullWorkInfoByIds(ctx context.Context, ids []int64) ([]*dto2.WorkFullDTO, error)
}

// WorkReader 作品基础信息读取接口
type WorkReader interface {
	// ListByIds 根据ID列表批量查询
	ListByIds(ctx context.Context, ids []int64) ([]*entity2.Work, error)
}

// ReWorkWorkSetRepository 作品-作品集关联仓储接口
type ReWorkWorkSetRepository interface {
	// Save 保存关联
	Save(ctx context.Context, rel *entity2.ReWorkWorkSet) error
	// SaveBatch 批量保存关联
	SaveBatch(ctx context.Context, rels []*entity2.ReWorkWorkSet) error
	// Delete 删除关联
	Delete(ctx context.Context, id int64) error
	// DeleteByWorkAndWorkSet 根据作品ID和作品集ID删除
	DeleteByWorkAndWorkSet(ctx context.Context, workId, workSetId int64) error
	// DeleteByWorkSetId 根据作品集ID删除所有关联
	DeleteByWorkSetId(ctx context.Context, workSetId int64) error
	// ListByWorkSetId 查询作品集关联的所有作品ID
	ListByWorkSetId(ctx context.Context, workSetId int64) ([]int64, error)
	// ListByWorkId 查询作品关联的所有作品集ID
	ListByWorkId(ctx context.Context, workId int64) ([]int64, error)
	// GetByWorkAndWorkSet 根据作品ID和作品集ID获取关联
	GetByWorkAndWorkSet(ctx context.Context, workId, workSetId int64) (*entity2.ReWorkWorkSet, error)
	// UpdateSortOrders 批量更新排序顺序
	UpdateSortOrders(ctx context.Context, workSetId int64, sortOrders map[int64]int) error
	// UpdateIsCover 更新封面标记
	UpdateIsCover(ctx context.Context, workId, workSetId int64, isCover bool) error
	// ClearOtherCovers 清除作品集的其他封面
	ClearOtherCovers(ctx context.Context, workSetId int64, exceptWorkId int64) error
	// GetCoverWorkId 获取封面作品ID
	GetCoverWorkId(ctx context.Context, workSetId int64) (int64, error)
}

// Service 作品集服务
type Service struct {
	repo              Repository
	reWorkWorkSetRepo ReWorkWorkSetRepository
	fullWorkReader    FullWorkReader
	workReader        WorkReader
}

// NewService 创建作品集服务
func NewService(repo Repository, reWorkWorkSetRepo ReWorkWorkSetRepository, fullWorkReader FullWorkReader, workReader WorkReader) *Service {
	return &Service{
		repo:              repo,
		reWorkWorkSetRepo: reWorkWorkSetRepo,
		fullWorkReader:    fullWorkReader,
		workReader:        workReader,
	}
}

// Save 保存作品集
func (s *Service) Save(ctx context.Context, workSet *entity2.WorkSet) error {
	return s.repo.Save(ctx, workSet)
}

// Update 更新作品集
func (s *Service) Update(ctx context.Context, workSet *entity2.WorkSet) error {
	if workSet.GetID() == 0 {
		return ErrWorkSetIdRequired
	}
	return s.repo.Update(ctx, workSet)
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*entity2.WorkSet, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*entity2.WorkSet, error) {
	return s.repo.List(ctx, opt)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// Delete 删除作品集
func (s *Service) Delete(ctx context.Context, id int64) error {
	// 删除作品集前，先删除所有关联关系
	if err := s.reWorkWorkSetRepo.DeleteByWorkSetId(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, page *model.Page[entity2.WorkSet], query WorkSetQueryDTO) (*model.Page[entity2.WorkSet], error) {
	conv := querypkg.NewConverter(entity2.WorkSet{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
}

// GetBySiteAndSiteWorkSetID 根据站点和站点作品集ID查询
func (s *Service) GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*entity2.WorkSet, error) {
	return s.repo.GetBySiteAndSiteWorkSetID(ctx, siteId, siteWorkSetId)
}

// GetBySiteWorkSetIdAndSiteName 根据站点作品集ID和站点名称查询
func (s *Service) GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) (*entity2.WorkSet, error) {
	return s.repo.GetBySiteWorkSetIdAndSiteName(ctx, siteWorkSetId, siteName)
}

// SaveOrUpdateByCompositeKey 按 (siteId, siteWorkSetId) 原子保存或更新作品集，返回内部 DB ID
func (s *Service) SaveOrUpdateByCompositeKey(ctx context.Context, ws *entity2.WorkSet) (int64, error) {
	if err := s.repo.Upsert(ctx, ws); err != nil {
		return 0, err
	}
	if ws.ID > 0 {
		return ws.ID, nil
	}
	existing, err := s.repo.GetBySiteAndSiteWorkSetID(ctx, ws.SiteID.Int64, ws.SiteWorkSetID.String)
	if err != nil {
		return 0, err
	}
	return existing.ID, nil
}

// LinkWorkToWorkSet 链接作品到作品集
func (s *Service) LinkWorkToWorkSet(ctx context.Context, workId, workSetId int64, isCover bool) error {
	rel := &entity2.ReWorkWorkSet{
		WorkID:    sql.NullInt64{Int64: workId, Valid: true},
		WorkSetID: sql.NullInt64{Int64: workSetId, Valid: true},
		IsCover:   sql.NullBool{Bool: isCover, Valid: true},
	}
	return s.reWorkWorkSetRepo.Save(ctx, rel)
}

// UnlinkWorkFromWorkSet 从作品集移除作品
func (s *Service) UnlinkWorkFromWorkSet(ctx context.Context, workId, workSetId int64) error {
	return s.reWorkWorkSetRepo.DeleteByWorkAndWorkSet(ctx, workId, workSetId)
}

// LinkBatchToWorkSet 批量链接作品到作品集
func (s *Service) LinkBatchToWorkSet(ctx context.Context, workSetId int64, workIds []int64) error {
	if len(workIds) == 0 {
		return nil
	}
	rels := make([]*entity2.ReWorkWorkSet, len(workIds))
	for i, workId := range workIds {
		rels[i] = &entity2.ReWorkWorkSet{
			WorkID:    sql.NullInt64{Int64: workId, Valid: true},
			WorkSetID: sql.NullInt64{Int64: workSetId, Valid: true},
			IsCover:   sql.NullBool{Bool: false, Valid: true},
		}
	}
	return s.reWorkWorkSetRepo.SaveBatch(ctx, rels)
}

// RemoveBatchFromWorkSet 批量从作品集移除作品
func (s *Service) RemoveBatchFromWorkSet(ctx context.Context, workSetId int64, workIds []int64) error {
	if len(workIds) == 0 {
		return nil
	}
	for _, workId := range workIds {
		if err := s.reWorkWorkSetRepo.DeleteByWorkAndWorkSet(ctx, workId, workSetId); err != nil {
			return err
		}
	}
	return nil
}

// GetWorksByWorkSetId 获取作品集关联的作品列表
func (s *Service) GetWorksByWorkSetId(ctx context.Context, workSetId int64) ([]*entity2.Work, error) {
	workIds, err := s.reWorkWorkSetRepo.ListByWorkSetId(ctx, workSetId)
	if err != nil {
		return nil, err
	}
	if len(workIds) == 0 {
		return []*entity2.Work{}, nil
	}
	return s.workReader.ListByIds(ctx, workIds)
}

// ListWorkSetsByWorkId 获取作品关联的作品集列表
func (s *Service) ListWorkSetsByWorkId(ctx context.Context, workId int64) ([]*entity2.WorkSet, error) {
	workSetIds, err := s.reWorkWorkSetRepo.ListByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	if len(workSetIds) == 0 {
		return []*entity2.WorkSet{}, nil
	}
	result := make([]*entity2.WorkSet, 0, len(workSetIds))
	for _, id := range workSetIds {
		ws, err := s.repo.GetById(ctx, id)
		if err != nil {
			continue
		}
		result = append(result, ws)
	}
	return result, nil
}

// SetCoverWork 设置作品集的封面作品
func (s *Service) SetCoverWork(ctx context.Context, workSetId, workId int64) error {
	// 清除现有封面
	if err := s.reWorkWorkSetRepo.ClearOtherCovers(ctx, workSetId, workId); err != nil {
		return err
	}
	// 设置新封面
	return s.reWorkWorkSetRepo.UpdateIsCover(ctx, workId, workSetId, true)
}

// UpdateSortOrders 批量更新排序顺序
func (s *Service) UpdateSortOrders(ctx context.Context, workSetId int64, workIds []int64) error {
	// 将 workIds 数组转换为 map（索引即为 sortOrder）
	sortOrders := make(map[int64]int)
	for i, id := range workIds {
		sortOrders[id] = i
	}
	return s.reWorkWorkSetRepo.UpdateSortOrders(ctx, workSetId, sortOrders)
}

// UnsetCover 取消封面设置
func (s *Service) UnsetCover(ctx context.Context, workSetId, workId int64) error {
	return s.reWorkWorkSetRepo.UpdateIsCover(ctx, workId, workSetId, false)
}

// GetCoverWorkId 获取封面作品ID
func (s *Service) GetCoverWorkId(ctx context.Context, workSetId int64) (int64, error) {
	return s.reWorkWorkSetRepo.GetCoverWorkId(ctx, workSetId)
}

// ListWorkSetWithWorkByIds 根据作品集ID列表获取作品集及其作品完整信息
func (s *Service) ListWorkSetWithWorkByIds(ctx context.Context, workSetIds []int64) ([]*dto2.WorkSetWithWorksResultDTO, error) {
	if len(workSetIds) == 0 {
		return []*dto2.WorkSetWithWorksResultDTO{}, nil
	}

	// 查询作品集
	opt := &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: "id", Values: toInterfaceSlice(workSetIds)}},
	}
	workSets, err := s.repo.List(ctx, opt)
	if err != nil {
		return nil, err
	}

	// 构建结果
	result := make([]*dto2.WorkSetWithWorksResultDTO, 0, len(workSets))
	for _, ws := range workSets {
		dto := &dto2.WorkSetWithWorksResultDTO{
			WorkSet: dto2.NewWorkSetDTO(ws),
		}

		// 获取作品集关联的作品ID
		workIds, err := s.reWorkWorkSetRepo.ListByWorkSetId(ctx, ws.GetID())
		if err != nil {
			return nil, err
		}
		if len(workIds) > 0 {
			fullWorks, err := s.fullWorkReader.GetFullWorkInfoByIds(ctx, workIds)
			if err != nil {
				return nil, err
			}
			dto.Works = fullWorks
		}

		result = append(result, dto)
	}

	return result, nil
}

// WorkSetWithCoverDTO 作品集及其封面作品信息
type WorkSetWithCoverDTO struct {
	WorkSet   *entity2.WorkSet `json:"workSet"`
	CoverWork *entity2.Work    `json:"coverWork,omitempty"`
}

// QueryPageWithCover 带封面的作品集分页查询
func (s *Service) QueryPageWithCover(ctx context.Context, page *model.Page[WorkSetWithCoverDTO], query WorkSetQueryDTO) (*model.Page[WorkSetWithCoverDTO], error) {
	conv := querypkg.NewConverter(entity2.WorkSet{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	// 先查询作品集分页
	pageResult, err := s.repo.Page(ctx, opt)
	if err != nil {
		return nil, err
	}

	if len(pageResult.Data) == 0 {
		return model.NewPage[WorkSetWithCoverDTO]([]*WorkSetWithCoverDTO{}, 0, page.PageNumber, page.PageSize), nil
	}

	// 构建结果
	result := make([]*WorkSetWithCoverDTO, 0, len(pageResult.Data))
	for _, ws := range pageResult.Data {
		dto := &WorkSetWithCoverDTO{
			WorkSet:   ws,
			CoverWork: nil,
		}

		// 获取封面作品ID
		coverWorkId, err := s.reWorkWorkSetRepo.GetCoverWorkId(ctx, ws.GetID())
		if err != nil {
			return nil, err
		}
		if coverWorkId > 0 {
			works, err := s.workReader.ListByIds(ctx, []int64{coverWorkId})
			if err != nil {
				return nil, err
			}
			if len(works) > 0 {
				dto.CoverWork = works[0]
			}
		}

		result = append(result, dto)
	}

	return model.NewPage[WorkSetWithCoverDTO](result, pageResult.DataCount, page.PageNumber, page.PageSize), nil
}

// ErrWorkSetIdRequired 错误定义
var (
	ErrWorkSetIdRequired = errors.New("更新作品集失败，id不能为空")
	_                    = (*pkgerr.BusinessError)(nil) // 确保 pkgerr 包被引用
)

// toInterfaceSlice converts int64 slice to interface{} slice
func toInterfaceSlice(ids []int64) []interface{} {
	result := make([]interface{}, len(ids))
	for i, id := range ids {
		result[i] = id
	}
	return result
}
