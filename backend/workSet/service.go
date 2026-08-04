package workSet

import (
	"context"
	"database/sql"
	"errors"
	"slices"

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
	// Create 新建
	Create(ctx context.Context, workSet *entity2.WorkSet) error
	// Updates 更新
	Updates(ctx context.Context, workSet *entity2.WorkSet) error
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
	// Create 新建关联
	Create(ctx context.Context, rel *entity2.ReWorkWorkSet) error
	// CreateBatch 批量新建关联
	CreateBatch(ctx context.Context, rels []*entity2.ReWorkWorkSet) error
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
	// UpdateSiteSortOrders 批量更新原站排序顺序（写 site_sort_order，不影响本地 sort_order）
	UpdateSiteSortOrders(ctx context.Context, workSetId int64, sortOrders map[int64]int) error
	// ApplySiteOrder 把原站序拷贝到本地序（site_sort_order → sort_order，仅 site_sort_order 非空成员）
	ApplySiteOrder(ctx context.Context, workSetId int64) error
	// UpdateIsCover 更新封面标记
	UpdateIsCover(ctx context.Context, workId, workSetId int64, isCover bool) error
	// ClearOtherCovers 清除作品集的其他封面
	ClearOtherCovers(ctx context.Context, workSetId int64, exceptWorkId int64) error
	// GetCoverWorkId 获取封面作品ID
	GetCoverWorkId(ctx context.Context, workSetId int64) (int64, error)
	// ListWorkIdsByWorkSetIds 批量查询多个作品集关联的去重作品ID（传递包含原语用，消除 N+1）
	ListWorkIdsByWorkSetIds(ctx context.Context, workSetIds []int64) ([]int64, error)
	// SaveBatchOnConflict 批量保存，唯一冲突跳过该行（物理纳入复制用）
	SaveBatchOnConflict(ctx context.Context, rels []*entity2.ReWorkWorkSet) error
	// MaxSortOrderByWorkSetId 作品集下最大 sort_order（无作品返回 0）
	MaxSortOrderByWorkSetId(ctx context.Context, workSetId int64) (int64, error)
}

// ReWorkSetWorkSetRepository 作品集间父子关联（多父 DAG）仓储接口
type ReWorkSetWorkSetRepository interface {
	// SaveRelation 建立父子关系（幂等，重复建立同一关系不报错）
	SaveRelation(ctx context.Context, rel *entity2.ReWorkSetWorkSet) error
	// DeleteByParentAndChild 解除指定父子关系
	DeleteByParentAndChild(ctx context.Context, parentWorkSetId, childWorkSetId int64) error
	// CollectDescendantWorkSetIds 递归查询作品集的所有后代作品集ID（沿 parent→child 向下，不含自身）
	CollectDescendantWorkSetIds(ctx context.Context, rootWorkSetId int64) ([]int64, error)
	// CollectAncestorWorkSetIds 递归查询作品集的所有祖先作品集ID（沿 child→parent 向上，不含自身）
	CollectAncestorWorkSetIds(ctx context.Context, workSetId int64) ([]int64, error)
	// DeleteByParentWorkSetId 删除某父集的全部子集关系（父作品集删除时清理）
	DeleteByParentWorkSetId(ctx context.Context, parentWorkSetId int64) error
	// DeleteByChildWorkSetId 删除某子集的全部父集关系（子作品集删除时清理）
	DeleteByChildWorkSetId(ctx context.Context, childWorkSetId int64) error
	// ListChildWorkSetIds 查询父集的直接子作品集ID（按 sort_order 升序）
	ListChildWorkSetIds(ctx context.Context, parentWorkSetId int64) ([]int64, error)
}

// Transactor 数据库事务执行器（事务 DB 实例经 ctx 传递，仓储 dbFromCtx 自动感知）
type Transactor interface {
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// Service 作品集服务
type Service struct {
	repo                 Repository
	reWorkWorkSetRepo    ReWorkWorkSetRepository
	reWorkSetWorkSetRepo ReWorkSetWorkSetRepository
	transactor           Transactor
	fullWorkReader       FullWorkReader
	workReader           WorkReader
}

// NewService 创建作品集服务
func NewService(repo Repository, reWorkWorkSetRepo ReWorkWorkSetRepository, reWorkSetWorkSetRepo ReWorkSetWorkSetRepository, transactor Transactor, fullWorkReader FullWorkReader, workReader WorkReader) *Service {
	return &Service{
		repo:                 repo,
		reWorkWorkSetRepo:    reWorkWorkSetRepo,
		reWorkSetWorkSetRepo: reWorkSetWorkSetRepo,
		transactor:           transactor,
		fullWorkReader:       fullWorkReader,
		workReader:           workReader,
	}
}

// CollectDescendantWorkIDs 传递包含原语：返回作品集自身及其全部后代作品集所含的 work（去重、保序）
// 顺序落实 §4.4：自身作品在前（按 re_work_work_set.sort_order），其后逐个后代作品集（CollectDescendantWorkSetIds 遍历序）
// 各集内按 re_work_work_set.sort_order；同一 work 仅出现一次，首次来源为准
func (s *Service) CollectDescendantWorkIDs(ctx context.Context, workSetId int64) ([]int64, error) {
	ordered := make([]int64, 0)
	seen := make(map[int64]struct{})
	appendUnique := func(ids []int64) {
		for _, id := range ids {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				ordered = append(ordered, id)
			}
		}
	}

	// 自身作品（按 sort_order 保序）
	selfWorkIds, err := s.reWorkWorkSetRepo.ListByWorkSetId(ctx, workSetId)
	if err != nil {
		return nil, err
	}
	appendUnique(selfWorkIds)

	// 后代作品集的作品（逐集按 sort_order 保序追加，去重）
	descendantWorkSetIds, err := s.reWorkSetWorkSetRepo.CollectDescendantWorkSetIds(ctx, workSetId)
	if err != nil {
		return nil, err
	}
	for _, dwsId := range descendantWorkSetIds {
		dwsWorkIds, err := s.reWorkWorkSetRepo.ListByWorkSetId(ctx, dwsId)
		if err != nil {
			return nil, err
		}
		appendUnique(dwsWorkIds)
	}

	return ordered, nil
}

// AddChildWorkSet 建立 parent→child 父子关系（parent 将 child 纳为子集）
// 事务内环路检测：若 child 已是 parent 的祖先，再加 parent→child 会闭合环路，拒绝
func (s *Service) AddChildWorkSet(ctx context.Context, parentWorkSetId, childWorkSetId int64) error {
	if parentWorkSetId == childWorkSetId {
		return ErrWorkSetSelfParent
	}
	return s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		ancestors, err := s.reWorkSetWorkSetRepo.CollectAncestorWorkSetIds(txCtx, parentWorkSetId)
		if err != nil {
			return err
		}
		if slices.Contains(ancestors, childWorkSetId) {
			return ErrWorkSetCycleDetected
		}
		rel := entity2.NewReWorkSetWorkSet()
		rel.ParentWorkSetID = sql.NullInt64{Int64: parentWorkSetId, Valid: true}
		rel.ChildWorkSetID = sql.NullInt64{Int64: childWorkSetId, Valid: true}
		return s.reWorkSetWorkSetRepo.SaveRelation(txCtx, rel)
	})
}

// RemoveChildWorkSet 解除 parent→child 父子关系
func (s *Service) RemoveChildWorkSet(ctx context.Context, parentWorkSetId, childWorkSetId int64) error {
	return s.reWorkSetWorkSetRepo.DeleteByParentAndChild(ctx, parentWorkSetId, childWorkSetId)
}

// ListChildWorkSets 查询作品集的直接子作品集（按 sort_order 升序），层级管理 UI 展示用
func (s *Service) ListChildWorkSets(ctx context.Context, parentWorkSetId int64) ([]*entity2.WorkSet, error) {
	childIds, err := s.reWorkSetWorkSetRepo.ListChildWorkSetIds(ctx, parentWorkSetId)
	if err != nil {
		return nil, err
	}
	if len(childIds) == 0 {
		return []*entity2.WorkSet{}, nil
	}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: "id", Values: toInterfaceSlice(childIds)}},
	}
	workSets, err := s.repo.List(ctx, opt)
	if err != nil {
		return nil, err
	}
	// 按 ListChildWorkSetIds 的 sort_order 顺序重排（repo.List 不保证顺序）
	orderMap := make(map[int64]int, len(childIds))
	for i, id := range childIds {
		orderMap[id] = i
	}
	ordered := make([]*entity2.WorkSet, 0, len(workSets))
	// 用 childIds 顺序作骨架，查到的填充
	workSetMap := make(map[int64]*entity2.WorkSet, len(workSets))
	for _, ws := range workSets {
		workSetMap[ws.GetID()] = ws
	}
	for _, id := range childIds {
		if ws, ok := workSetMap[id]; ok {
			ordered = append(ordered, ws)
		}
	}
	return ordered, nil
}

// MergeWorkSetInto 物理纳入：把源作品集（及其全部后代）的 work 复制一份关联到目标作品集（静态快照）
// 复制非转移，源 B 不变；不记录来源、不可撤回；目标原有作品保序在前，纳入作品按源序追加在后；is_cover=false 维持目标自身封面
func (s *Service) MergeWorkSetInto(ctx context.Context, sourceWorkSetId, targetWorkSetId int64) error {
	if sourceWorkSetId == targetWorkSetId {
		return ErrWorkSetMergeSelf
	}
	// 源作品集及其全部后代的 work（保序、去重）
	workIds, err := s.CollectDescendantWorkIDs(ctx, sourceWorkSetId)
	if err != nil {
		return err
	}
	if len(workIds) == 0 {
		return nil
	}
	// 目标当前最大 sort_order，纳入作品追加在目标原有作品之后
	maxSort, err := s.reWorkWorkSetRepo.MaxSortOrderByWorkSetId(ctx, targetWorkSetId)
	if err != nil {
		return err
	}
	// 构造复制关联：OnConflict DoNothing 去重（单条重复不拒整批），is_cover=false 维持目标自身封面
	rels := make([]*entity2.ReWorkWorkSet, 0, len(workIds))
	for i, workId := range workIds {
		rel := entity2.NewReWorkWorkSet()
		rel.WorkID = sql.NullInt64{Int64: workId, Valid: true}
		rel.WorkSetID = sql.NullInt64{Int64: targetWorkSetId, Valid: true}
		rel.IsCover = sql.NullBool{Bool: false, Valid: true}
		rel.SortOrder = sql.NullInt64{Int64: maxSort + 1 + int64(i), Valid: true}
		rels = append(rels, rel)
	}
	return s.reWorkWorkSetRepo.SaveBatchOnConflict(ctx, rels)
}

// Save 保存作品集
func (s *Service) Save(ctx context.Context, workSet *entity2.WorkSet) error {
	return s.repo.Create(ctx, workSet)
}

// Update 更新作品集
func (s *Service) Update(ctx context.Context, workSet *entity2.WorkSet) error {
	if workSet.GetID() == 0 {
		return ErrWorkSetIdRequired
	}
	return s.repo.Updates(ctx, workSet)
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

// Delete 删除作品集（事务内清理全部关联：作品关联、父子关联[作为父集与作为子集]，再删实体）
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		// 作品-作品集关联
		if err := s.reWorkWorkSetRepo.DeleteByWorkSetId(txCtx, id); err != nil {
			return err
		}
		// 作品集间父子关联（作为父集与作为子集的行）
		if err := s.reWorkSetWorkSetRepo.DeleteByParentWorkSetId(txCtx, id); err != nil {
			return err
		}
		if err := s.reWorkSetWorkSetRepo.DeleteByChildWorkSetId(txCtx, id); err != nil {
			return err
		}
		return s.repo.Delete(txCtx, id)
	})
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
	rel := entity2.NewReWorkWorkSet()
	rel.WorkID = sql.NullInt64{Int64: workId, Valid: true}
	rel.WorkSetID = sql.NullInt64{Int64: workSetId, Valid: true}
	rel.IsCover = sql.NullBool{Bool: isCover, Valid: true}
	return s.reWorkWorkSetRepo.Create(ctx, rel)
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
		rel := entity2.NewReWorkWorkSet()
		rel.WorkID = sql.NullInt64{Int64: workId, Valid: true}
		rel.WorkSetID = sql.NullInt64{Int64: workSetId, Valid: true}
		rel.IsCover = sql.NullBool{Bool: false, Valid: true}
		rels[i] = rel
	}
	return s.reWorkWorkSetRepo.CreateBatch(ctx, rels)
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

// GetWorksByWorkSetId 获取作品集关联的作品列表（传递包含：含全部后代作品集的作品，保序去重）
func (s *Service) GetWorksByWorkSetId(ctx context.Context, workSetId int64) ([]*entity2.Work, error) {
	workIds, err := s.CollectDescendantWorkIDs(ctx, workSetId)
	if err != nil {
		return nil, err
	}
	if len(workIds) == 0 {
		return []*entity2.Work{}, nil
	}
	works, err := s.workReader.ListByIds(ctx, workIds)
	if err != nil {
		return nil, err
	}
	// 按 CollectDescendantWorkIDs 的保序顺序重排（ListByIds 不保证返回顺序）
	workMap := make(map[int64]*entity2.Work, len(works))
	for _, w := range works {
		workMap[w.GetID()] = w
	}
	ordered := make([]*entity2.Work, 0, len(workIds))
	for _, id := range workIds {
		if w, ok := workMap[id]; ok {
			ordered = append(ordered, w)
		}
	}
	return ordered, nil
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

// ApplySiteOrder 把作品集的原站序应用到本地序（site_sort_order → sort_order，仅 site_sort_order 非空成员）
func (s *Service) ApplySiteOrder(ctx context.Context, workSetId int64) error {
	return s.reWorkWorkSetRepo.ApplySiteOrder(ctx, workSetId)
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
// 传递包含：每个作品集的作品含其全部后代作品集的作品（去重、保序，§4.4），GetFullWorkInfoByIds 批量化消除 N+1
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

	// 逐作品集收集含后代的作品ID（保序、去重），汇总全部作品ID
	workSetToOrderedWorkIds := make(map[int64][]int64, len(workSets))
	allWorkIdSet := make(map[int64]struct{})
	for _, ws := range workSets {
		ids, err := s.CollectDescendantWorkIDs(ctx, ws.GetID())
		if err != nil {
			return nil, err
		}
		workSetToOrderedWorkIds[ws.GetID()] = ids
		for _, id := range ids {
			allWorkIdSet[id] = struct{}{}
		}
	}

	// 批量获取作品完整信息（消除 N+1）
	fullWorkMap := make(map[int64]*dto2.WorkFullDTO, len(allWorkIdSet))
	if len(allWorkIdSet) > 0 {
		allWorkIds := make([]int64, 0, len(allWorkIdSet))
		for id := range allWorkIdSet {
			allWorkIds = append(allWorkIds, id)
		}
		fullWorks, err := s.fullWorkReader.GetFullWorkInfoByIds(ctx, allWorkIds)
		if err != nil {
			return nil, err
		}
		for _, fw := range fullWorks {
			if fw.Work != nil {
				fullWorkMap[fw.Work.Id] = fw
			}
		}
	}

	// 按作品集组装（保留含后代的保序作品列表）
	result := make([]*dto2.WorkSetWithWorksResultDTO, 0, len(workSets))
	for _, ws := range workSets {
		dto := &dto2.WorkSetWithWorksResultDTO{
			WorkSet: dto2.NewWorkSetDTO(ws),
		}
		for _, workId := range workSetToOrderedWorkIds[ws.GetID()] {
			if fw, ok := fullWorkMap[workId]; ok {
				dto.Works = append(dto.Works, fw)
			}
		}
		result = append(result, dto)
	}

	return result, nil
}

// QueryPageWithCover 带封面的作品集分页查询
func (s *Service) QueryPageWithCover(ctx context.Context, page *model.Page[dto2.WorkSetWithCoverDTO], query WorkSetQueryDTO) (*model.Page[dto2.WorkSetWithCoverDTO], error) {
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
		return model.NewPage[dto2.WorkSetWithCoverDTO]([]*dto2.WorkSetWithCoverDTO{}, 0, page.PageNumber, page.PageSize), nil
	}

	// 构建结果
	result := make([]*dto2.WorkSetWithCoverDTO, 0, len(pageResult.Data))
	for _, ws := range pageResult.Data {
		dto := &dto2.WorkSetWithCoverDTO{
			WorkSet:   dto2.NewWorkSetDTO(ws),
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
				dto.CoverWork = dto2.NewWorkDTO(works[0])
			}
		}

		result = append(result, dto)
	}

	return model.NewPage[dto2.WorkSetWithCoverDTO](result, pageResult.DataCount, page.PageNumber, page.PageSize), nil
}

// ErrWorkSetIdRequired 错误定义
var (
	ErrWorkSetIdRequired    = errors.New("更新作品集失败，id不能为空")
	ErrWorkSetSelfParent    = errors.New("建立作品集父子关系失败，父集与子集不能相同")
	ErrWorkSetCycleDetected = errors.New("建立作品集父子关系失败，子集已是父集的祖先，将形成环路")
	ErrWorkSetMergeSelf     = errors.New("物理纳入失败，源作品集与目标作品集不能相同")
	_                       = (*pkgerr.BusinessError)(nil) // 确保 pkgerr 包被引用
)

// toInterfaceSlice converts int64 slice to interface{} slice
func toInterfaceSlice(ids []int64) []interface{} {
	result := make([]interface{}, len(ids))
	for i, id := range ids {
		result[i] = id
	}
	return result
}
