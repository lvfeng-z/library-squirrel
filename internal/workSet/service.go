package workSet

import (
	"context"
	"database/sql"
	"errors"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm/clause"
)

// ========== 查询 DTO ==========

// WorkSetQueryDTO 作品集查询条件
type WorkSetQueryDTO struct {
	// 精确查询
	ID            *int64  `json:"-"`             // 作品集ID（程序设置，不从JSON解析）
	SiteID        *int64  `json:"siteId"`        // 站点ID
	SiteWorkSetID *string `json:"siteWorkSetId"` // 站点作品集ID
	SiteAuthorID  *string `json:"siteAuthorId"`  // 站点作者ID
	NickName      *string `json:"nickName"`      // 昵称（精确匹配）
	// 模糊查询
	SiteWorkSetNameLike *string `json:"siteWorkSetNameLike"` // 站点作品集名称（模糊匹配）
	SiteWorkSetDescLike *string `json:"siteWorkSetDescLike"` // 站点作品集描述（模糊匹配）
	NickNameLike        *string `json:"nickNameLike"`        // 昵称（模糊匹配）
	// 排序字段：create_time, update_time, site_upload_time, site_update_time, last_view, site_work_set_name
	OrderBy   string `json:"orderBy"`   // 排序字段
	OrderDesc bool   `json:"orderDesc"` // 是否降序
}

// BuildOrderBy 根据查询DTO构建排序条件
func (dto *WorkSetQueryDTO) BuildOrderBy() clause.Expression {
	column := "id"
	if dto.OrderBy != "" {
		// 支持的排序字段映射
		orderByMap := map[string]string{
			"create_time":        "create_time",
			"update_time":        "update_time",
			"site_upload_time":   "site_upload_time",
			"site_update_time":   "site_update_time",
			"last_view":          "last_view",
			"site_work_set_name": "site_work_set_name",
		}
		if v, ok := orderByMap[dto.OrderBy]; ok {
			column = v
		}
	}
	return clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: column}, Desc: dto.OrderDesc}}}
}

// ========== 外部模块接口定义（由 workSet 模块定义自己需要的接口）==========

// WorkReader 作品读取接口
type WorkReader interface {
	// ListByIds 根据ID列表批量查询
	ListByIds(ctx context.Context, ids []int64) ([]*domain.Work, error)
}

// ReWorkWorkSetRepository 作品-作品集关联仓储接口
type ReWorkWorkSetRepository interface {
	// Save 保存关联
	Save(ctx context.Context, rel *domain.ReWorkWorkSet) error
	// SaveBatch 批量保存关联
	SaveBatch(ctx context.Context, rels []*domain.ReWorkWorkSet) error
	// Delete 删除关联
	Delete(ctx context.Context, id int64) error
	// DeleteByWorkAndWorkSet 根据作品ID和作品集ID删除
	DeleteByWorkAndWorkSet(ctx context.Context, workId, workSetId int64) error
	// DeleteByWorkSetId 根据作品集ID删除所有关联
	DeleteByWorkSetId(ctx context.Context, workSetId int64) error
	// ListByWorkSetId 查询作品集关联的所有作品ID
	ListByWorkSetId(ctx context.Context, workSetId int64) ([]int64, error)
	// GetByWorkAndWorkSet 根据作品ID和作品集ID获取关联
	GetByWorkAndWorkSet(ctx context.Context, workId, workSetId int64) (*domain.ReWorkWorkSet, error)
	// UpdateSortOrders 批量更新排序顺序
	UpdateSortOrders(ctx context.Context, workSetId int64, sortOrders map[int64]int) error
	// UpdateIsCover 更新封面标记
	UpdateIsCover(ctx context.Context, workId, workSetId int64, isCover int) error
	// ClearOtherCovers 清除作品集的其他封面
	ClearOtherCovers(ctx context.Context, workSetId int64, exceptWorkId int64) error
	// GetCoverWorkId 获取封面作品ID
	GetCoverWorkId(ctx context.Context, workSetId int64) (int64, error)
}

// Service 作品集服务
type Service struct {
	repo              Repository
	reWorkWorkSetRepo ReWorkWorkSetRepository
	workReader        WorkReader
}

// NewService 创建作品集服务
func NewService(repo Repository, reWorkWorkSetRepo ReWorkWorkSetRepository, workReader WorkReader) *Service {
	return &Service{
		repo:              repo,
		reWorkWorkSetRepo: reWorkWorkSetRepo,
		workReader:        workReader,
	}
}

// Save 保存作品集
func (s *Service) Save(ctx context.Context, workSet *domain.WorkSet) error {
	return s.repo.Save(ctx, workSet)
}

// Update 更新作品集
func (s *Service) Update(ctx context.Context, workSet *domain.WorkSet) error {
	if workSet.GetID() == 0 {
		return ErrWorkSetIdRequired
	}
	return s.repo.Update(ctx, workSet)
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*domain.WorkSet, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*domain.WorkSet, error) {
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
func (s *Service) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.WorkSet], error) {
	return s.repo.Page(ctx, opt)
}

// PageByDTO 分页查询（基于 QueryDTO）
func (s *Service) PageByDTO(ctx context.Context, page, pageSize int, queryDTO WorkSetQueryDTO) (*model.Page[domain.WorkSet], error) {
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

// QueryPageWithCoverByDTO 带封面的作品集分页查询（基于 QueryDTO）
func (s *Service) QueryPageWithCoverByDTO(ctx context.Context, page, pageSize int, queryDTO WorkSetQueryDTO) (*model.Page[WorkSetWithCoverDTO], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	return s.QueryPageWithCover(ctx, page, pageSize, where, orderBy)
}

// buildConditionsFromDTO 根据查询DTO构建查询条件
func buildConditionsFromDTO(dto *WorkSetQueryDTO) []clause.Expression {
	var conditions []clause.Expression

	if dto.ID != nil {
		conditions = append(conditions, clause.Eq{Column: "id", Value: *dto.ID})
	}
	if dto.SiteID != nil {
		conditions = append(conditions, clause.Eq{Column: "site_id", Value: *dto.SiteID})
	}
	if dto.SiteWorkSetID != nil {
		conditions = append(conditions, clause.Eq{Column: "site_work_set_id", Value: *dto.SiteWorkSetID})
	}
	if dto.SiteAuthorID != nil {
		conditions = append(conditions, clause.Eq{Column: "site_author_id", Value: *dto.SiteAuthorID})
	}
	if dto.NickName != nil {
		conditions = append(conditions, clause.Eq{Column: "nick_name", Value: *dto.NickName})
	}
	if dto.SiteWorkSetNameLike != nil {
		conditions = append(conditions, clause.Like{Column: "site_work_set_name", Value: *dto.SiteWorkSetNameLike})
	}
	if dto.SiteWorkSetDescLike != nil {
		conditions = append(conditions, clause.Like{Column: "site_work_set_description", Value: *dto.SiteWorkSetDescLike})
	}
	if dto.NickNameLike != nil {
		conditions = append(conditions, clause.Like{Column: "nick_name", Value: *dto.NickNameLike})
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

// GetBySiteAndSiteWorkSetID 根据站点和站点作品集ID查询
func (s *Service) GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*domain.WorkSet, error) {
	return s.repo.GetBySiteAndSiteWorkSetID(ctx, siteId, siteWorkSetId)
}

// GetBySiteWorkSetIdAndSiteName 根据站点作品集ID和站点名称查询
func (s *Service) GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) (*domain.WorkSet, error) {
	return s.repo.GetBySiteWorkSetIdAndSiteName(ctx, siteWorkSetId, siteName)
}

// LinkWorkToWorkSet 链接作品到作品集
func (s *Service) LinkWorkToWorkSet(ctx context.Context, workId, workSetId int64, isCover int) error {
	rel := &domain.ReWorkWorkSet{
		WorkID:    sql.NullInt64{Int64: workId, Valid: true},
		WorkSetID: sql.NullInt64{Int64: workSetId, Valid: true},
		IsCover:   sql.NullInt64{Int64: int64(isCover), Valid: true},
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
	rels := make([]*domain.ReWorkWorkSet, len(workIds))
	for i, workId := range workIds {
		rels[i] = &domain.ReWorkWorkSet{
			WorkID:    sql.NullInt64{Int64: workId, Valid: true},
			WorkSetID: sql.NullInt64{Int64: workSetId, Valid: true},
			IsCover:   sql.NullInt64{Int64: 0, Valid: true},
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
func (s *Service) GetWorksByWorkSetId(ctx context.Context, workSetId int64) ([]*domain.Work, error) {
	workIds, err := s.reWorkWorkSetRepo.ListByWorkSetId(ctx, workSetId)
	if err != nil {
		return nil, err
	}
	if len(workIds) == 0 {
		return []*domain.Work{}, nil
	}
	return s.workReader.ListByIds(ctx, workIds)
}

// SetCoverWork 设置作品集的封面作品
func (s *Service) SetCoverWork(ctx context.Context, workSetId, workId int64) error {
	// 清除现有封面
	if err := s.reWorkWorkSetRepo.ClearOtherCovers(ctx, workSetId, workId); err != nil {
		return err
	}
	// 设置新封面
	return s.reWorkWorkSetRepo.UpdateIsCover(ctx, workId, workSetId, 1)
}

// UpdateSortOrders 批量更新排序顺序
func (s *Service) UpdateSortOrders(ctx context.Context, workSetId int64, sortOrders map[int64]int) error {
	return s.reWorkWorkSetRepo.UpdateSortOrders(ctx, workSetId, sortOrders)
}

// UnsetCover 取消封面设置
func (s *Service) UnsetCover(ctx context.Context, workSetId, workId int64) error {
	return s.reWorkWorkSetRepo.UpdateIsCover(ctx, workId, workSetId, 0)
}

// GetCoverWorkId 获取封面作品ID
func (s *Service) GetCoverWorkId(ctx context.Context, workSetId int64) (int64, error) {
	return s.reWorkWorkSetRepo.GetCoverWorkId(ctx, workSetId)
}

// WorkSetWithWorksDTO 作品集及其作品信息
type WorkSetWithWorksDTO struct {
	WorkSet *domain.WorkSet `json:"workSet"`
	Works   []*domain.Work  `json:"works"`
}

// ListWorkSetWithWorkByIds 根据作品集ID列表获取作品集及其作品信息
func (s *Service) ListWorkSetWithWorkByIds(ctx context.Context, workSetIds []int64) ([]*WorkSetWithWorksDTO, error) {
	if len(workSetIds) == 0 {
		return []*WorkSetWithWorksDTO{}, nil
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
	result := make([]*WorkSetWithWorksDTO, 0, len(workSets))
	for _, ws := range workSets {
		dto := &WorkSetWithWorksDTO{
			WorkSet: ws,
			Works:   []*domain.Work{},
		}

		// 获取作品集关联的作品
		workIds, err := s.reWorkWorkSetRepo.ListByWorkSetId(ctx, ws.GetID())
		if err != nil {
			return nil, err
		}
		if len(workIds) > 0 {
			works, err := s.workReader.ListByIds(ctx, workIds)
			if err != nil {
				return nil, err
			}
			dto.Works = works
		}

		result = append(result, dto)
	}

	return result, nil
}

// WorkSetWithCoverDTO 作品集及其封面作品信息
type WorkSetWithCoverDTO struct {
	WorkSet   *domain.WorkSet `json:"workSet"`
	CoverWork *domain.Work    `json:"coverWork,omitempty"`
}

// QueryPageWithCover 带封面的作品集分页查询
func (s *Service) QueryPageWithCover(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[WorkSetWithCoverDTO], error) {
	// 先查询作品集分页
	var conditions []clause.Expression
	if where != nil {
		conditions = []clause.Expression{where}
	}
	opt := &database.PageOption{
		QueryOption: database.QueryOption{
			Conditions: conditions,
			OrderBy:    []clause.Expression{order},
		},
		Page:     page,
		PageSize: pageSize,
	}
	pageResult, err := s.repo.Page(ctx, opt)
	if err != nil {
		return nil, err
	}

	if len(pageResult.Data) == 0 {
		return model.NewPage([]*WorkSetWithCoverDTO{}, 0, page, pageSize), nil
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

	return model.NewPage(result, pageResult.DataCount, page, pageSize), nil
}

// ErrWorkSetIdRequired 错误定义
var (
	ErrWorkSetIdRequired = errors.New("更新作品集失败，id不能为空")
)

// BusinessError 业务错误
type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string {
	return e.Message
}

// toInterfaceSlice converts int64 slice to interface{} slice
func toInterfaceSlice(ids []int64) []interface{} {
	result := make([]interface{}, len(ids))
	for i, id := range ids {
		result[i] = id
	}
	return result
}
