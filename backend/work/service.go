package work

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
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

// LocalTagBatchReader 本地标签批量读取接口
type LocalTagBatchReader interface {
	// ListByIds 根据ID列表批量查询
	ListByIds(ctx context.Context, ids []int64) ([]*entity2.LocalTag, error)
}

// SiteTagBatchReader 站点标签批量读取接口
type SiteTagBatchReader interface {
	// ListBySiteTagIds 根据站点标签ID列表批量查询
	ListBySiteTagIds(ctx context.Context, siteTagIds []int64) ([]*entity2.SiteTag, error)
}

// SiteBatchReader 站点批量读取接口
type SiteBatchReader interface {
	// ListByIds 根据ID列表批量查询
	ListByIds(ctx context.Context, ids []int64) ([]*entity2.Site, error)
}

// LocalAuthorBatchReader 本地作者批量读取接口
type LocalAuthorBatchReader interface {
	// ListReWorkAuthor 批量查询作品关联的本地作者，按 workId 分组
	ListReWorkAuthor(ctx context.Context, workIds []int64) (map[int64][]*dto2.RankedLocalAuthor, error)
	// ListByIds 根据ID列表批量查询
	ListByIds(ctx context.Context, ids []int64) ([]*entity2.LocalAuthor, error)
}

// SiteAuthorBatchReader 站点作者批量读取接口
type SiteAuthorBatchReader interface {
	// ListSiteAuthorsByWorkIds 批量查询作品关联的站点作者，按 workId 分组
	ListSiteAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*dto2.RankedSiteAuthor, error)
}

// ResourceBatchReader 资源批量读取接口
type ResourceBatchReader interface {
	// ListByWorkIds 批量查询作品关联的资源，按 workId 分组
	ListByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*entity2.Resource, error)
}

// ResourceStoreBatchReader resource_store 批量读取接口(按 resourceId 分组)
type ResourceStoreBatchReader interface {
	ListStoresByResourceIds(ctx context.Context, resourceIds []int64) (map[int64][]*entity2.ResourceStore, error)
}

// StoreBatchReader PersistentStore 批量读取接口
type StoreBatchReader interface {
	// GetByIds 根据 ID 列表批量查询
	GetByIds(ctx context.Context, ids []int64) ([]*entity2.PersistentStore, error)
}

// ReWorkTagBatchReader 作品-标签关联批量读取接口
type ReWorkTagBatchReader interface {
	// ListLocalTagIdsByWorkIds 批量查询作品关联的本地标签ID，按 workId 分组
	ListLocalTagIdsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]int64, error)
	// ListSiteTagIdsByWorkIds 批量查询作品关联的站点标签ID，按 workId 分组
	ListSiteTagIdsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]int64, error)
}

// ReWorkTagWriter 作品-标签关联写入接口
type ReWorkTagWriter interface {
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
	// DeleteSiteByWorkId 删除作品的 SITE 标签关联（保留 LOCAL）
	DeleteSiteByWorkId(ctx context.Context, workId int64) error
	// SaveBatch 批量保存关联
	SaveBatch(ctx context.Context, rels []*entity2.ReWorkTag) error
	// SaveBatchOnConflict 批量保存，唯一冲突跳过（LOCAL 关联增量入库用，保留用户手动设的 namespace 等字段）
	SaveBatchOnConflict(ctx context.Context, rels []*entity2.ReWorkTag) error
}

// ReWorkWorkSetWriter 作品-作品集关联写入接口
type ReWorkWorkSetWriter interface {
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
	// CreateBatch 批量新建关联
	CreateBatch(ctx context.Context, rels []*entity2.ReWorkWorkSet) error
	// SaveBatchOnConflict 批量保存，唯一冲突跳过（增量入库，保留历史关联与 sort_order 元数据）
	SaveBatchOnConflict(ctx context.Context, rels []*entity2.ReWorkWorkSet) error
	// UpdateSiteSortOrders 批量更新原站排序（写 site_sort_order，不影响本地 sort_order）
	UpdateSiteSortOrders(ctx context.Context, workSetId int64, sortOrders map[int64]int) error
	// MaxSortOrderByWorkSetId 查询作品集下最大 sort_order（无作品返回 0），buildWorkSetLinks 续排用
	MaxSortOrderByWorkSetId(ctx context.Context, workSetId int64) (int64, error)
}

// WorkSetRelationWriter 作品集间父子关系写入接口（reWorkSetWorkSet 模块实现）
// 用于作品入库后异步拉取父集关系并建立层级 + 写 site_sort_order（对齐 ReWorkWorkSetWriter 的写入角色）
type WorkSetRelationWriter interface {
	// CollectAncestorWorkSetIds 递归查询作品集的所有祖先 ID（环路检测：建立 parent→child 前确认 child 非 parent 祖先）
	CollectAncestorWorkSetIds(ctx context.Context, workSetId int64) ([]int64, error)
	// SaveRelation 幂等建立父子关系（OnConflict DoNothing，重复建立同一关系不报错、不覆盖既有字段）
	SaveRelation(ctx context.Context, rel *entity2.ReWorkSetWorkSet) error
	// UpdateSiteSortOrdersForChild 批量更新一个子集在各父集下的原站序（写 site_sort_order，不影响本地 sort_order）
	UpdateSiteSortOrdersForChild(ctx context.Context, childWorkSetId int64, parentOrders map[int64]int) error
}

// WorkSetOrderFetcher 原站序获取能力（plugin 模块实现，按 task 的 plugin 身份路由到对应插件 proxy）
// 用于作品入库后拉取作品集内作品的原站顺序；nil（未注入）时跳过拉取
type WorkSetOrderFetcher interface {
	// QueryWorkSetOrder 按 (pluginPublicId, extensionId) 定位插件，拉取作品集内作品的原站全序；空=插件不掌握
	QueryWorkSetOrder(ctx context.Context, pluginPublicId, extensionId string, siteId int64, siteWorkSetId string) ([]*sdkdto.WorkOrderEntry, error)
}

// WorkSetRelationFetcher 作品集父集关系获取能力（plugin 模块实现，按 task 的 plugin 身份路由到对应插件 proxy）
// 用于作品入库后拉取其所属各 workSet 的父集关系 + 子集原站序；nil（未注入）时跳过拉取
type WorkSetRelationFetcher interface {
	// QueryWorkSetRelations 按 (pluginPublicId, extensionId) 定位插件，拉取本作品集的父集 + 在各父集下的原站序；空=无父/插件不掌握
	QueryWorkSetRelations(ctx context.Context, pluginPublicId, extensionId string, siteId int64, siteWorkSetId string) ([]*sdkdto.WorkSetRelationEntry, error)
}

// LocalTagFindOrCreator 本地标签查找或创建接口
type LocalTagFindOrCreator interface {
	// GetByNames 根据名称列表批量查询
	GetByNames(ctx context.Context, names []string) ([]*entity2.LocalTag, error)
	// Save 创建本地标签
	Save(ctx context.Context, tag *entity2.LocalTag) error
	// SaveBatch 批量创建本地标签
	SaveBatch(ctx context.Context, tags []*entity2.LocalTag) error
}

// LocalAuthorFindOrCreator 本地作者查找或创建接口
type LocalAuthorFindOrCreator interface {
	// GetByNames 根据名称列表批量查询
	GetByNames(ctx context.Context, names []string) ([]*entity2.LocalAuthor, error)
	// Save 创建本地作者
	Save(ctx context.Context, author *entity2.LocalAuthor) error
	// SaveBatch 批量创建本地作者
	SaveBatch(ctx context.Context, authors []*entity2.LocalAuthor) error
}

// SiteAuthorWriter 站点作者写入接口
type SiteAuthorWriter interface {
	// SaveOrUpdateByCompositeKey 按复合键保存或更新，返回内部 DB ID
	SaveOrUpdateByCompositeKey(ctx context.Context, author *entity2.SiteAuthor) (int64, error)
	// GetBySiteAndSiteAuthorID 根据站点ID和站点作者ID查询
	GetBySiteAndSiteAuthorID(ctx context.Context, siteId int64, siteAuthorId string) (*entity2.SiteAuthor, error)
	// BatchUpsert 批量插入或更新（基于 site_id + site_author_id 唯一约束）
	BatchUpsert(ctx context.Context, authors []*entity2.SiteAuthor) error
	// ListBySiteAndSiteAuthorIDs 根据站点ID和站点作者ID列表批量查询
	ListBySiteAndSiteAuthorIDs(ctx context.Context, siteId int64, siteAuthorIds []string) ([]*entity2.SiteAuthor, error)
}

// SiteTagWriter 站点标签写入接口
type SiteTagWriter interface {
	// SaveOrUpdateByCompositeKey 按复合键保存或更新，返回内部 DB ID
	SaveOrUpdateByCompositeKey(ctx context.Context, tag *entity2.SiteTag) (int64, error)
	// GetBySiteAndSiteTagID 根据站点ID和站点标签ID查询
	GetBySiteAndSiteTagID(ctx context.Context, siteId int64, siteTagId string) (*entity2.SiteTag, error)
	// BatchUpsert 批量插入或更新（基于 site_id + site_tag_id 唯一约束）
	BatchUpsert(ctx context.Context, tags []*entity2.SiteTag) error
	// ListBySiteAndSiteTagIDs 根据站点ID和站点标签ID列表批量查询
	ListBySiteAndSiteTagIDs(ctx context.Context, siteId int64, siteTagIds []string) ([]*entity2.SiteTag, error)
}

// WorkSetWriter 作品集写入接口
type WorkSetWriter interface {
	// SaveOrUpdateByCompositeKey 按复合键保存或更新，返回内部 DB ID
	SaveOrUpdateByCompositeKey(ctx context.Context, ws *entity2.WorkSet) (int64, error)
	// GetBySiteAndSiteWorkSetID 根据站点ID和站点作品集ID查询
	GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*entity2.WorkSet, error)
	// BatchUpsert 批量插入或更新（基于 site_id + site_work_set_id 唯一约束）
	BatchUpsert(ctx context.Context, workSets []*entity2.WorkSet) error
	// ListBySiteAndSiteWorkSetIDs 根据站点ID和站点作品集ID列表批量查询
	ListBySiteAndSiteWorkSetIDs(ctx context.Context, siteId int64, siteWorkSetIds []string) ([]*entity2.WorkSet, error)
}

// Transactor 数据库事务执行器
type Transactor interface {
	// ExecInTransaction 在事务中执行 fn，事务 DB 实例通过 ctx 传递
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// ReWorkAuthorWriter 作品-作者关联写入接口
type ReWorkAuthorWriter interface {
	// SaveBatch 批量保存关联
	SaveBatch(ctx context.Context, reWorkAuthors []*entity2.ReWorkAuthor) error
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
	// DeleteSiteByWorkId 删除作品的 SITE 作者关联（保留 LOCAL）
	DeleteSiteByWorkId(ctx context.Context, workId int64) error
	// SaveBatchOnConflict 批量保存，唯一冲突跳过（LOCAL 关联增量入库用，不覆盖用户手动建的关联）
	SaveBatchOnConflict(ctx context.Context, reWorkAuthors []*entity2.ReWorkAuthor) error
}

// ResourceDeleter 资源删除接口
type ResourceDeleter interface {
	// DeleteByWorkId 根据作品ID删除所有资源
	DeleteByWorkId(ctx context.Context, workId int64) error
	// ListByWorkId 查询作品关联的资源
	ListByWorkId(ctx context.Context, workId int64) ([]*entity2.Resource, error)
}

// StoreDeleter PersistentStore 删除接口
type StoreDeleter interface {
	// HardDelete 删除记录及对应文件（物理删记录）
	// backup: 是否对已完成文件进行移动备份
	HardDelete(ctx context.Context, id int64, backup bool) (int64, error)
	// DeleteUnscopedByIds 批量物理删除记录（作品彻底删除链：目标行全为已软删行，
	// HardDelete 的 GetById 受软删 scope 保护会静默跳过，故走 Unscoped 直删）
	DeleteUnscopedByIds(ctx context.Context, ids []int64) error
	// DeleteWithBackup 删除 store 文件（移入 backup 建保管清单行并写行内 backup_id，记录随软删）
	DeleteWithBackup(ctx context.Context, id int64) (int64, error)
	// ListByIdsIncludeDeleted 按 ID 集合查记录行（含已删行；行内 backup_id 与 file_path 供复原/彻底删除链使用）
	ListByIdsIncludeDeleted(ctx context.Context, ids []int64) []*entity2.PersistentStore
	// RestoreByIds 批量复活 store 记录（复原链：清软删标志与 backup_id）
	RestoreByIds(ctx context.Context, ids []int64) error
}

// ResourceStoreHardDeleter resource_store 物理删除接口（作品彻底删除链级联清理）
type ResourceStoreHardDeleter interface {
	// DeleteByResourceIds 批量物理删除 resource 关联行
	DeleteByResourceIds(ctx context.Context, resourceIds []int64) error
}

// RunningTaskStopper 运行中任务停止接口（逻辑删除前停止关联任务实例，防止重建作品）
// task 记录不在删除范围，仅停止内存中的运行实例
type RunningTaskStopper interface {
	// StopRunningBySiteWork 停止指定作品关联的运行中任务实例
	StopRunningBySiteWork(ctx context.Context, siteId int64, siteWorkId string) error
}

// Repository 作品仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建
	Create(ctx context.Context, work *entity2.Work) error
	// Updates 更新
	Updates(ctx context.Context, work *entity2.Work) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity2.Work, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*entity2.Work, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除（对 work 实体自动改写为打软删标志；物理删除走 DeleteUnscoped）
	Delete(ctx context.Context, id int64) error
	// DeleteUnscoped 物理删除（级联删除链/死入口用，绕过软删改写）
	DeleteUnscoped(ctx context.Context, id int64) error
	// GetDeletedById 按ID查询已软删行（复原链入口校验；nil = 非已删条目）
	GetDeletedById(ctx context.Context, id int64) (*entity2.Work, error)
	// ClearDeletedFlag 清软删标志（复原核心：一行 UPDATE）
	ClearDeletedFlag(ctx context.Context, id int64) error
	// ListDeletedBefore 查询软删时间早于 expireBefore（毫秒时间戳）的已删行，供 TTL 清理
	ListDeletedBefore(ctx context.Context, expireBefore int64) ([]*entity2.Work, error)
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity2.Work], error)
	// GetBySiteAndSiteWorkID 根据站点和站点作品ID查询
	GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*entity2.Work, error)
	// ListBySiteAndSiteWorkIDs 批量根据站点和站点作品ID查询
	ListBySiteAndSiteWorkIDs(ctx context.Context, siteIds []int64, siteWorkIds []string) ([]*entity2.Work, error)
	// ListByIds 根据ID列表批量查询
	ListByIds(ctx context.Context, ids []int64) ([]*entity2.Work, error)
	// UpdateLastViewBatch 批量更新最后查看时间
	UpdateLastViewBatch(ctx context.Context, ids []int64, lastView int64) error
}

// Service 作品服务
type Service struct {
	repo Repository

	// 事务执行器
	transactor Transactor

	// 外部模块依赖（通过构造函数注入）
	localTagReader    LocalTagReader
	localAuthorReader LocalAuthorReader
	siteTagReader     SiteTagReader
	siteAuthorReader  SiteAuthorReader
	siteReader        SiteReader
	resourceReader    ResourceReader
	resourceDeleter   ResourceDeleter
	storeDeleter      StoreDeleter

	// 批量读取接口（用于 GetFullWorkInfoByIds）
	localTagBatchReader      LocalTagBatchReader
	siteTagBatchReader       SiteTagBatchReader
	siteBatchReader          SiteBatchReader
	localAuthorBatchReader   LocalAuthorBatchReader
	siteAuthorBatchReader    SiteAuthorBatchReader
	resourceBatchReader      ResourceBatchReader
	resourceStoreBatchReader ResourceStoreBatchReader
	storeBatchReader         StoreBatchReader
	reWorkTagBatchReader     ReWorkTagBatchReader

	// 写入接口（用于 SaveWorkInfo）
	reWorkTagWriter          ReWorkTagWriter
	reWorkWorkSetWriter      ReWorkWorkSetWriter
	siteAuthorWriter         SiteAuthorWriter
	siteTagWriter            SiteTagWriter
	workSetWriter            WorkSetWriter
	reWorkAuthorWriter       ReWorkAuthorWriter
	localTagFindOrCreator    LocalTagFindOrCreator
	localAuthorFindOrCreator LocalAuthorFindOrCreator
	workSetRelationWriter    WorkSetRelationWriter

	// 原站序获取能力（入库后异步拉取，nil 时跳过；经 SetWorkSetOrderFetcher 延迟注入）
	// 作品集父集关系获取能力（入库后异步拉取，nil 时跳过；经 SetWorkSetRelationFetcher 延迟注入）
	workSetOrderFetcher    WorkSetOrderFetcher
	workSetRelationFetcher WorkSetRelationFetcher

	// 逻辑删除（SoftDeleteWork）所需配置
	runningTaskStopper RunningTaskStopper // 可选，nil 时跳过任务停止

	// 彻底删除（DeleteWorkAndSurroundingData）级联清理接口
	resourceStoreHardDeleter ResourceStoreHardDeleter
}

// NewService 创建作品服务
func NewService(
	repo Repository,
	transactor Transactor,
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
	localTagBatchReader LocalTagBatchReader,
	siteTagBatchReader SiteTagBatchReader,
	siteBatchReader SiteBatchReader,
	localAuthorBatchReader LocalAuthorBatchReader,
	siteAuthorBatchReader SiteAuthorBatchReader,
	resourceBatchReader ResourceBatchReader,
	resourceStoreBatchReader ResourceStoreBatchReader,
	storeBatchReader StoreBatchReader,
	reWorkTagBatchReader ReWorkTagBatchReader,
	localTagFindOrCreator LocalTagFindOrCreator,
	localAuthorFindOrCreator LocalAuthorFindOrCreator,
	storeDeleter StoreDeleter,
	runningTaskStopper RunningTaskStopper,
	resourceStoreHardDeleter ResourceStoreHardDeleter,
	workSetRelationWriter WorkSetRelationWriter,
) *Service {
	return &Service{
		repo:                     repo,
		transactor:               transactor,
		localTagReader:           localTagReader,
		localAuthorReader:        localAuthorReader,
		siteTagReader:            siteTagReader,
		siteAuthorReader:         siteAuthorReader,
		siteReader:               siteReader,
		resourceReader:           resourceReader,
		resourceDeleter:          resourceDeleter,
		localTagBatchReader:      localTagBatchReader,
		siteTagBatchReader:       siteTagBatchReader,
		siteBatchReader:          siteBatchReader,
		localAuthorBatchReader:   localAuthorBatchReader,
		siteAuthorBatchReader:    siteAuthorBatchReader,
		resourceBatchReader:      resourceBatchReader,
		resourceStoreBatchReader: resourceStoreBatchReader,
		reWorkTagBatchReader:     reWorkTagBatchReader,
		reWorkTagWriter:          reWorkTagWriter,
		reWorkWorkSetWriter:      reWorkWorkSetWriter,
		siteAuthorWriter:         siteAuthorWriter,
		siteTagWriter:            siteTagWriter,
		workSetWriter:            workSetWriter,
		reWorkAuthorWriter:       reWorkAuthorWriter,
		localTagFindOrCreator:    localTagFindOrCreator,
		localAuthorFindOrCreator: localAuthorFindOrCreator,
		storeBatchReader:         storeBatchReader,
		storeDeleter:             storeDeleter,
		runningTaskStopper:       runningTaskStopper,
		resourceStoreHardDeleter: resourceStoreHardDeleter,
		workSetRelationWriter:    workSetRelationWriter,
	}
}

// SetWorkSetOrderFetcher 延迟注入原站序获取能力
// plugin 模块在 work 之后初始化（registry 需先就绪），故 WorkSetOrderFetcher 经此 setter 注入
func (s *Service) SetWorkSetOrderFetcher(f WorkSetOrderFetcher) {
	s.workSetOrderFetcher = f
}

// SetWorkSetRelationFetcher 延迟注入作品集父集关系获取能力（同 SetWorkSetOrderFetcher 的延迟注入理由）
func (s *Service) SetWorkSetRelationFetcher(f WorkSetRelationFetcher) {
	s.workSetRelationFetcher = f
}

// SetRunningTaskStopper 延迟注入运行中任务停止器
// taskManager 在 work 之后创建（且其 TaskDeps 依赖 work），形成 work ↔ taskManager 循环，
// 故 RunningTaskStopper 经构造函数传 nil，taskManager 创建后再经此 setter 注入
func (s *Service) SetRunningTaskStopper(stopper RunningTaskStopper) {
	s.runningTaskStopper = stopper
}

// Save 保存作品
func (s *Service) Save(ctx context.Context, work *entity2.Work) error {
	return s.repo.Create(ctx, work)
}

// UpdateById 更新作品
func (s *Service) UpdateById(ctx context.Context, work *entity2.Work) error {
	if work.ID == 0 {
		return ErrWorkIdRequired
	}
	return s.repo.Updates(ctx, work)
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

// DeleteWorkAndSurroundingData 删除作品及其周围数据（级联删除）
// DB 操作在事务内原子执行，磁盘文件删除在事务成功后尽力而为
func (s *Service) DeleteWorkAndSurroundingData(ctx context.Context, id int64) error {
	// 事务前：收集需要删除的 Store ID（从 resource_store 收集,不读旧列;文件删除必须在事务后）
	var storeIds []int64
	resources, err := s.resourceDeleter.ListByWorkId(ctx, id)
	if err != nil {
		return err
	}
	resourceIds := make([]int64, 0, len(resources))
	for _, res := range resources {
		resourceIds = append(resourceIds, res.GetID())
	}
	if len(resourceIds) > 0 {
		rsStoreMap, _ := s.resourceStoreBatchReader.ListStoresByResourceIds(ctx, resourceIds)
		for _, rsList := range rsStoreMap {
			for _, rs := range rsList {
				if rs.StoreID > 0 {
					storeIds = append(storeIds, rs.StoreID)
				}
			}
		}
	}

	// 事务内：删除所有 DB 关联 + Resource + Work
	err = s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		if err := s.reWorkTagWriter.DeleteByWorkId(txCtx, id); err != nil {
			return err
		}
		if err := s.reWorkWorkSetWriter.DeleteByWorkId(txCtx, id); err != nil {
			return err
		}
		if err := s.reWorkAuthorWriter.DeleteByWorkId(txCtx, id); err != nil {
			return err
		}
		// 级联删 resource_store 关联行（历史链缺此步，遗留孤儿行——本链修复）
		if err := s.resourceStoreHardDeleter.DeleteByResourceIds(txCtx, resourceIds); err != nil {
			return err
		}
		// 物理删 persistent_store 行：本链目标（已删作品）的 store 行全为软删行，HardDelete 的
		// GetById 受软删 scope 保护会静默跳过（NotFound 提前返回），遗留离链孤儿行——故事务内 Unscoped 直删；
		// 原路径文件已移 backup/（由调用方按行内 backup_id 清理备份），链内无文件删除
		if err := s.storeDeleter.DeleteUnscopedByIds(txCtx, storeIds); err != nil {
			return err
		}
		if err := s.resourceDeleter.DeleteByWorkId(txCtx, id); err != nil {
			return err
		}
		// 物理删 work：本链仅服务彻底删除（目标为已删行，软删过滤会挡住普通 Delete）
		return s.repo.DeleteUnscoped(txCtx, id)
	})
	if err != nil {
		return err
	}
	return nil
}

// SoftDeleteWork 软删除作品（移入回收站，可经回收站复原）
//
// 流程：
//  1. 校验 work 为活行
//  2. 事务外：资源文件经 DeleteWithBackup 移入 backup 目录（建含 work_id 归属的备份记录；persistent_store 记录原地保留）
//  3. 事务内：work 软删一条 UPDATE（GORM softDelete 改写打毫秒时间戳）
//  4. 停止关联的运行中任务实例（task 记录保留）
//
// 从属行（resource/resource_store/re_work_*）原地保留——复原仅需文件还原 + 清标志。
// 资源文件移动在事务外（文件 IO 不可回滚）；事务失败中间态为文件暂离原位但备份记录含 work_id 归属，
// 作品仍为活行，可经再次删除（复用既有备份）或彻底删除清理，不产生无主文件。
func (s *Service) SoftDeleteWork(ctx context.Context, workId int64) error {
	// 1. 校验 work 存在（软删过滤下仅命中活行）
	work, err := s.repo.GetById(ctx, workId)
	if err != nil {
		return err
	}
	if work == nil {
		return ErrWorkNotFound
	}

	// 2. [事务外] 资源文件移入 backup（从 resource_store 收集 store，不读旧列）
	resources, err := s.resourceDeleter.ListByWorkId(ctx, workId)
	if err != nil {
		return fmt.Errorf("查询作品资源失败: %w", err)
	}
	resourceIds := make([]int64, 0, len(resources))
	for _, res := range resources {
		resourceIds = append(resourceIds, res.GetID())
	}
	rsStoreMap, _ := s.resourceStoreBatchReader.ListStoresByResourceIds(ctx, resourceIds)
	for _, rsList := range rsStoreMap {
		for _, rsRow := range rsList {
			if rsRow.StoreID <= 0 {
				continue
			}
			if _, err := s.storeDeleter.DeleteWithBackup(ctx, rsRow.StoreID); err != nil {
				return err
			}
		}
	}

	// 3. [事务内] work 软删一条 UPDATE
	err = s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		return s.repo.Delete(txCtx, workId)
	})
	if err != nil {
		return err
	}

	// 4. 停止关联的运行中任务实例（task 记录保留；接口由 taskManager 实现，nil 时跳过）
	if s.runningTaskStopper != nil && work.SiteID.Valid && work.SiteWorkID.Valid {
		if err := s.runningTaskStopper.StopRunningBySiteWork(ctx, work.SiteID.Int64, work.SiteWorkID.String); err != nil {
			logger.Log.Warnf("停止作品 %d 关联运行中任务失败: %v", workId, err)
		}
	}
	return nil
}

// GetDeletedWork 按ID获取已软删作品（复原链入口校验；nil = 非已删条目）
func (s *Service) GetDeletedWork(ctx context.Context, id int64) (*entity2.Work, error) {
	return s.repo.GetDeletedById(ctx, id)
}

// RestoreDeletedWork 清软删标志（复原核心：一行 UPDATE，文件还原由调用方编排）
func (s *Service) RestoreDeletedWork(ctx context.Context, id int64) error {
	return s.repo.ClearDeletedFlag(ctx, id)
}

// ListDeletedBefore 查询软删时间早于 expireBefore 的已删作品，供 TTL 清理
func (s *Service) ListDeletedBefore(ctx context.Context, expireBefore int64) ([]*entity2.Work, error) {
	return s.repo.ListDeletedBefore(ctx, expireBefore)
}

// ListWorkStoresIncludeDeleted 取作品的全部 store 记录行（含已删行；彻底删除链按行内 backup_id 定位备份）
func (s *Service) ListWorkStoresIncludeDeleted(ctx context.Context, workId int64) []*entity2.PersistentStore {
	storeIds, err := s.collectWorkStoreIds(ctx, workId)
	if err != nil {
		logger.Log.Warnf("收集作品 %d store ID 失败: %v", workId, err)
		return []*entity2.PersistentStore{}
	}
	return s.storeDeleter.ListByIdsIncludeDeleted(ctx, storeIds)
}

// mountKey 挂载键（resource_id + store_type + store_seq），多轨身份与文件名消歧的同一身份维度
type mountKey struct {
	resourceId int64
	role       string
	seq        int
}

// deriveRevivableStores 圈定作品的复活集：按挂载键取最新死代。关联保留形态下同键可有多个软删代
// （替换/merge 残留、失效前代），无差别复活会令两活行同 file_path 撞部分唯一索引、备份文件还原互相
// 覆盖；最新死代即该键最后被处置的一代（作品软删链删除的正是删除时的活代），更早代保持死态归
// 回收站文件条目
func (s *Service) deriveRevivableStores(ctx context.Context, workId int64) []*entity2.PersistentStore {
	resources, err := s.resourceDeleter.ListByWorkId(ctx, workId)
	if err != nil {
		logger.Log.Warnf("圈定作品 %d 复活集失败(查资源): %v", workId, err)
		return nil
	}
	resourceIds := make([]int64, 0, len(resources))
	for _, res := range resources {
		resourceIds = append(resourceIds, res.GetID())
	}
	rsStoreMap, _ := s.resourceStoreBatchReader.ListStoresByResourceIds(ctx, resourceIds)
	storeIdSet := make(map[int64]struct{})
	for _, rsList := range rsStoreMap {
		for _, rs := range rsList {
			if rs.StoreID > 0 {
				storeIdSet[rs.StoreID] = struct{}{}
			}
		}
	}
	storeIds := make([]int64, 0, len(storeIdSet))
	for id := range storeIdSet {
		storeIds = append(storeIds, id)
	}
	rows := s.storeDeleter.ListByIdsIncludeDeleted(ctx, storeIds)
	rowById := make(map[int64]*entity2.PersistentStore, len(rows))
	for _, row := range rows {
		rowById[row.GetID()] = row
	}
	newest := make(map[mountKey]*entity2.PersistentStore)
	for _, rsList := range rsStoreMap {
		for _, rs := range rsList {
			row := rowById[rs.StoreID]
			if row == nil || row.DeletedAt == 0 {
				continue // 活行与行缺失不进复活集
			}
			key := mountKey{resourceId: rs.ResourceID, role: rs.StoreType, seq: rs.StoreSeq}
			if cur := newest[key]; cur == nil || row.DeletedAt > cur.DeletedAt {
				newest[key] = row
			}
		}
	}
	result := make([]*entity2.PersistentStore, 0, len(newest))
	for _, row := range newest {
		result = append(result, row)
	}
	return result
}

// ListRevivableWorkStores 取作品的复活集（同键最新死代行；文件还原链按此圈定还原目标，
// 避免更早死代的备份文件还原到同路径互相覆盖）
func (s *Service) ListRevivableWorkStores(ctx context.Context, workId int64) []*entity2.PersistentStore {
	return s.deriveRevivableStores(ctx, workId)
}

// collectWorkStoreIds 收集作品的全部 store ID（resource→resource_store 链）
func (s *Service) collectWorkStoreIds(ctx context.Context, workId int64) ([]int64, error) {
	resources, err := s.resourceDeleter.ListByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	resourceIds := make([]int64, 0, len(resources))
	for _, res := range resources {
		resourceIds = append(resourceIds, res.GetID())
	}
	rsStoreMap, _ := s.resourceStoreBatchReader.ListStoresByResourceIds(ctx, resourceIds)
	storeIds := make([]int64, 0)
	for _, rsList := range rsStoreMap {
		for _, rsRow := range rsList {
			if rsRow.StoreID > 0 {
				storeIds = append(storeIds, rsRow.StoreID)
			}
		}
	}
	return storeIds, nil
}

// RestoreWorkStores 复原作品的 store 记录（复活集=同键最新死代——无差别复活会在双代同路径形态下
// 撞 file_path 部分唯一索引；文件还原由 recycleBin 编排先行，与其共用同一复活集）
func (s *Service) RestoreWorkStores(ctx context.Context, workId int64) error {
	rows := s.deriveRevivableStores(ctx, workId)
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.GetID())
	}
	return s.storeDeleter.RestoreByIds(ctx, ids)
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

// ListBySiteAndSiteWorkIDs 批量根据站点和站点作品ID查询
func (s *Service) ListBySiteAndSiteWorkIDs(ctx context.Context, siteIds []int64, siteWorkIds []string) ([]*entity2.Work, error) {
	return s.repo.ListBySiteAndSiteWorkIDs(ctx, siteIds, siteWorkIds)
}

// GetFullWorkInfoByIds 批量获取作品完整信息（含资源、作者、标签、站点）
func (s *Service) GetFullWorkInfoByIds(ctx context.Context, ids []int64) ([]*dto2.WorkFullDTO, error) {
	if len(ids) == 0 {
		return []*dto2.WorkFullDTO{}, nil
	}

	// Phase 1: 批量查询作品基础信息
	works, err := s.repo.ListByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	workMap := make(map[int64]*entity2.Work, len(works))
	for _, w := range works {
		workMap[w.GetID()] = w
	}

	// Phase 2: 批量查询本地作者（按 workId 分组）
	localAuthorMap, _ := s.localAuthorBatchReader.ListReWorkAuthor(ctx, ids)

	// Phase 3: 批量查询站点作者（按 workId 分组）
	siteAuthorMap, _ := s.siteAuthorBatchReader.ListSiteAuthorsByWorkIds(ctx, ids)

	// Phase 4: 批量查询资源（按 workId 分组）
	resourceMap, _ := s.resourceBatchReader.ListByWorkIds(ctx, ids)

	// Phase 4.5: 批量查询 resource_store 行 + PersistentStore 记录(从 resource_store 收集 storeId,不读旧列)
	var allResourceIds []int64
	for _, resources := range resourceMap {
		for _, res := range resources {
			allResourceIds = append(allResourceIds, res.GetID())
		}
	}
	resourceStoreMap, _ := s.resourceStoreBatchReader.ListStoresByResourceIds(ctx, allResourceIds)
	var allStoreIds []int64
	storeIdSet := make(map[int64]bool)
	for _, rsList := range resourceStoreMap {
		for _, rs := range rsList {
			if rs.StoreID > 0 && !storeIdSet[rs.StoreID] {
				storeIdSet[rs.StoreID] = true
				allStoreIds = append(allStoreIds, rs.StoreID)
			}
		}
	}
	storeMap := make(map[int64]*entity2.PersistentStore)
	if len(allStoreIds) > 0 {
		stores, _ := s.storeBatchReader.GetByIds(ctx, allStoreIds)
		for _, st := range stores {
			storeMap[st.GetID()] = st
		}
	}

	// Phase 5: 批量查询本地标签ID → 本地标签实体
	localTagIdMap, _ := s.reWorkTagBatchReader.ListLocalTagIdsByWorkIds(ctx, ids)
	var allLocalTagIds []int64
	for _, tagIds := range localTagIdMap {
		allLocalTagIds = append(allLocalTagIds, tagIds...)
	}
	localTagEntityMap := make(map[int64]*entity2.LocalTag)
	if len(allLocalTagIds) > 0 {
		localTagEntities, _ := s.localTagBatchReader.ListByIds(ctx, allLocalTagIds)
		for _, t := range localTagEntities {
			localTagEntityMap[t.GetID()] = t
		}
	}

	// Phase 6: 批量查询站点标签ID → 站点标签实体
	siteTagIdMap, _ := s.reWorkTagBatchReader.ListSiteTagIdsByWorkIds(ctx, ids)
	var allSiteTagIds []int64
	for _, tagIds := range siteTagIdMap {
		allSiteTagIds = append(allSiteTagIds, tagIds...)
	}
	siteTagEntityMap := make(map[int64]*entity2.SiteTag)
	if len(allSiteTagIds) > 0 {
		siteTagEntities, _ := s.siteTagBatchReader.ListBySiteTagIds(ctx, allSiteTagIds)
		for _, t := range siteTagEntities {
			siteTagEntityMap[t.GetID()] = t
		}
	}

	// Phase 6.5: 补充加载站点标签关联的本地标签
	var extraLocalTagIds []int64
	for _, st := range siteTagEntityMap {
		if st.LocalTagID.Valid && st.LocalTagID.Int64 > 0 {
			if _, exists := localTagEntityMap[st.LocalTagID.Int64]; !exists {
				extraLocalTagIds = append(extraLocalTagIds, st.LocalTagID.Int64)
			}
		}
	}
	if len(extraLocalTagIds) > 0 {
		extraLocalTags, _ := s.localTagBatchReader.ListByIds(ctx, extraLocalTagIds)
		for _, lt := range extraLocalTags {
			localTagEntityMap[lt.GetID()] = lt
		}
	}

	// Phase 7: 批量查询站点
	var siteIds []int64
	siteIdSet := make(map[int64]bool)
	for _, w := range works {
		if w.SiteID.Valid && w.SiteID.Int64 > 0 && !siteIdSet[w.SiteID.Int64] {
			siteIdSet[w.SiteID.Int64] = true
			siteIds = append(siteIds, w.SiteID.Int64)
		}
	}
	siteEntityMap := make(map[int64]*entity2.Site)
	if len(siteIds) > 0 {
		siteEntities, _ := s.siteBatchReader.ListByIds(ctx, siteIds)
		for _, site := range siteEntities {
			siteEntityMap[site.GetID()] = site
		}
	}

	// Phase 8: 组装结果
	result := make([]*dto2.WorkFullDTO, 0, len(ids))
	for _, id := range ids {
		work, ok := workMap[id]
		if !ok {
			continue
		}
		fullDTO := dto2.NewWorkFullDTO(work)

		// 本地作者
		if authors, ok := localAuthorMap[id]; ok && len(authors) > 0 {
			fullDTO.LocalAuthors = authors
		}

		// 站点作者
		if authors, ok := siteAuthorMap[id]; ok && len(authors) > 0 {
			fullDTO.SiteAuthors = authors
		}

		// 站点
		if work.SiteID.Valid && work.SiteID.Int64 > 0 {
			if site, ok := siteEntityMap[work.SiteID.Int64]; ok {
				fullDTO.Site = dto2.NewSiteDTO(site)
			}
		}

		// 本地标签
		if tagIds, ok := localTagIdMap[id]; ok && len(tagIds) > 0 {
			fullDTO.LocalTags = make([]*sdkdto.LocalTagDTO, 0, len(tagIds))
			for _, tagId := range tagIds {
				if tag, ok := localTagEntityMap[tagId]; ok {
					fullDTO.LocalTags = append(fullDTO.LocalTags, dto2.NewLocalTagDTO(tag))
				}
			}
		}

		// 站点标签
		if tagIds, ok := siteTagIdMap[id]; ok && len(tagIds) > 0 {
			fullDTO.SiteTags = make([]*dto2.SiteTagFullDTO, 0, len(tagIds))
			for _, tagId := range tagIds {
				if tag, ok := siteTagEntityMap[tagId]; ok {
					stDTO := dto2.NewSiteTagFullDTO(tag)
					if tag.SiteID.Valid && tag.SiteID.Int64 > 0 {
						if site, ok := siteEntityMap[tag.SiteID.Int64]; ok {
							stDTO.Site = dto2.NewSiteDTO(site)
						}
					}
					if tag.LocalTagID.Valid && tag.LocalTagID.Int64 > 0 {
						if localTag, ok := localTagEntityMap[tag.LocalTagID.Int64]; ok {
							stDTO.LocalTag = dto2.NewLocalTagDTO(localTag)
						}
					}
					fullDTO.SiteTags = append(fullDTO.SiteTags, stDTO)
				}
			}
		}

		// 资源(从 resource_store 组装,不读旧列)
		if resources, ok := resourceMap[id]; ok && len(resources) > 0 {
			res := resources[0]
			rsList := resourceStoreMap[res.GetID()]
			fullDTO.Resource = dto2.NewResourceFullDTO(res, rsList, storeMap)
		}

		result = append(result, fullDTO)
	}

	return result, nil
}

// LoadWorkMeta 加载作品的命名元数据(Work + 作者),构造为 WorkResponse 供文件名模板使用。
// 用于资源板块单独重下(未跑作品元数据板块)时从已有作品获取命名所需元数据,与板块选择解耦。
func (s *Service) LoadWorkMeta(ctx context.Context, workId int64) (*sdkdto.WorkResponse, error) {
	fulls, err := s.GetFullWorkInfoByIds(ctx, []int64{workId})
	if err != nil {
		return nil, err
	}
	if len(fulls) == 0 {
		return nil, nil
	}
	f := fulls[0]
	resp := &sdkdto.WorkResponse{Work: f.Work}
	for _, la := range f.LocalAuthors {
		a := la.Author
		resp.LocalAuthors = append(resp.LocalAuthors, &a)
	}
	for _, sa := range f.SiteAuthors {
		a := sa.Author
		resp.SiteAuthors = append(resp.SiteAuthors, &sdkdto.TaskSiteAuthorDTO{
			SiteAuthorId:    ptrStrValue(a.SiteAuthorID),
			AuthorName:      ptrStrValue(a.AuthorName),
			FixedAuthorName: ptrStrValue(a.FixedAuthorName),
			Introduce:       ptrStrValue(a.Introduce),
			Homepage:        ptrStrValue(a.Homepage),
		})
	}
	return resp, nil
}

// ptrStrValue 安全解引用 *string,空指针返回空串
func ptrStrValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
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
					authorMap[localAuthorId] = &dto2.RankedLocalAuthor{
						Author: *dto2.NewLocalAuthorDTO(localAuthor),
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

// ErrWorkIdRequired 错误定义
var (
	ErrWorkIdRequired = &pkgerr.BusinessError{Code: 400, Message: "更新作品失败，id不能为空"}
	ErrWorkNotFound   = &pkgerr.BusinessError{Code: 404, Message: "作品不存在"}
)

// siteOrderSyncTimeout 原站序拉取超时（网络调用，须事务外异步）
const siteOrderSyncTimeout = 30 * time.Second

// SaveWorkInfo 保存作品及全部周边数据，返回作品内部 DB ID
func (s *Service) SaveWorkInfo(ctx context.Context, task *entity2.Task, workResp *sdkdto.WorkResponse) (int64, error) {
	var workId int64
	err := s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		var err error
		workId, err = s.saveWorkInfoInTx(txCtx, task, workResp)
		return err
	})
	if err != nil {
		return workId, err
	}
	// 事务提交后异步拉取原站序写 site_sort_order：网络调用须事务外（MaxOpenConns=1 死锁），异步不阻塞入库流程
	if s.workSetOrderFetcher != nil && len(workResp.WorkSets) > 0 {
		go s.syncSiteSortOrders(task, workResp)
	}
	// 事务提交后异步拉取作品集父集关系，建立层级 + 写 site_sort_order（同上：网络调用须事务外异步）
	if s.workSetRelationFetcher != nil && len(workResp.WorkSets) > 0 {
		go s.syncWorkSetRelations(task, workResp)
	}
	return workId, nil
}

// syncSiteSortOrders 作品入库后异步拉取其所属各 workSet 的原站全序，映射并写 site_sort_order（事务外、带超时）
func (s *Service) syncSiteSortOrders(task *entity2.Task, workResp *sdkdto.WorkResponse) {
	ctx, cancel := context.WithTimeout(context.Background(), siteOrderSyncTimeout)
	defer cancel()
	siteId := task.SiteID.Int64
	for _, ws := range workResp.WorkSets {
		if ws.SiteWorkSetId == "" || !task.PluginPublicID.Valid {
			continue
		}
		entries, err := s.workSetOrderFetcher.QueryWorkSetOrder(ctx, task.PluginPublicID.String, task.PluginExtensionID.String, siteId, ws.SiteWorkSetId)
		if err != nil {
			logger.Log.Warnf("拉取作品集原站序失败 siteWorkSetId=%s: %v", ws.SiteWorkSetId, err)
			continue
		}
		if len(entries) == 0 {
			continue
		}
		if err := s.applySiteSortOrders(ctx, siteId, ws.SiteWorkSetId, entries); err != nil {
			logger.Log.Warnf("写入作品集原站序失败 siteWorkSetId=%s: %v", ws.SiteWorkSetId, err)
		}
	}
}

// applySiteSortOrders 映射 siteWorkId→work.id，写该 workSet 的 site_sort_order（仅本站已入库成员）
func (s *Service) applySiteSortOrders(ctx context.Context, siteId int64, siteWorkSetId string, entries []*sdkdto.WorkOrderEntry) error {
	workSet, err := s.workSetWriter.GetBySiteAndSiteWorkSetID(ctx, siteId, siteWorkSetId)
	if err != nil {
		return fmt.Errorf("查询作品集失败: %w", err)
	}
	// 批量映射 siteWorkId → work.id。ListBySiteAndSiteWorkIDs 是 (siteId, siteWorkId) 平行数组语义，
	// 须为每个 siteWorkId 配同一个 siteId，否则 len(siteIds)=1 只查 siteWorkIds[0]
	siteIds := make([]int64, len(entries))
	siteWorkIds := make([]string, 0, len(entries))
	for i, e := range entries {
		siteIds[i] = siteId
		siteWorkIds = append(siteWorkIds, e.SiteWorkId)
	}
	works, err := s.ListBySiteAndSiteWorkIDs(ctx, siteIds, siteWorkIds)
	if err != nil {
		return fmt.Errorf("批量查询作品失败: %w", err)
	}
	siteWorkIdToWorkId := make(map[string]int64, len(works))
	for _, w := range works {
		if w.SiteWorkID.Valid {
			siteWorkIdToWorkId[w.SiteWorkID.String] = w.GetID()
		}
	}
	sortOrders := make(map[int64]int, len(entries))
	for _, e := range entries {
		if workId, ok := siteWorkIdToWorkId[e.SiteWorkId]; ok {
			sortOrders[workId] = int(e.SortOrder)
		}
	}
	if len(sortOrders) == 0 {
		return nil
	}
	return s.reWorkWorkSetWriter.UpdateSiteSortOrders(ctx, workSet.ID, sortOrders)
}

// syncWorkSetRelations 作品入库后异步拉取其所属各 workSet 的父集关系，建立层级 + 写 site_sort_order（事务外、带超时）
// 对齐 syncSiteSortOrders 的拉取范式（主程序→插件 pull），遍历 workResp.WorkSets 逐集拉取其父集关系
func (s *Service) syncWorkSetRelations(task *entity2.Task, workResp *sdkdto.WorkResponse) {
	ctx, cancel := context.WithTimeout(context.Background(), siteOrderSyncTimeout)
	defer cancel()
	siteId := task.SiteID.Int64
	for _, ws := range workResp.WorkSets {
		if ws.SiteWorkSetId == "" || !task.PluginPublicID.Valid {
			continue
		}
		childWorkSet, err := s.workSetWriter.GetBySiteAndSiteWorkSetID(ctx, siteId, ws.SiteWorkSetId)
		if err != nil {
			logger.Log.Warnf("查询子作品集失败(跳过父集关系同步) siteWorkSetId=%s: %v", ws.SiteWorkSetId, err)
			continue
		}
		parents, err := s.workSetRelationFetcher.QueryWorkSetRelations(ctx, task.PluginPublicID.String, task.PluginExtensionID.String, siteId, ws.SiteWorkSetId)
		if err != nil {
			logger.Log.Warnf("拉取作品集父集关系失败 siteWorkSetId=%s: %v", ws.SiteWorkSetId, err)
			continue
		}
		if len(parents) == 0 {
			continue
		}
		if err := s.applyWorkSetRelations(ctx, siteId, childWorkSet.ID, parents); err != nil {
			logger.Log.Warnf("写入作品集父集关系失败 siteWorkSetId=%s: %v", ws.SiteWorkSetId, err)
		}
	}
}

// applyWorkSetRelations upsert 父集实体、建立父子关系（事务内环路检测）、写 site_sort_order
// 初始本地序 sort_order 取原站序（SaveRelation 的 OnConflict DoNothing 保证重复拉取不覆盖用户后续拖拽）
func (s *Service) applyWorkSetRelations(ctx context.Context, siteId, childWorkSetId int64, parents []*sdkdto.WorkSetRelationEntry) error {
	validParents := make([]*sdkdto.WorkSetRelationEntry, 0, len(parents))
	parentDTOs := make([]*sdkdto.TaskWorkSetDTO, 0, len(parents))
	for _, p := range parents {
		if p.ParentSiteWorkSetId == "" {
			continue
		}
		validParents = append(validParents, p)
		parentDTOs = append(parentDTOs, &sdkdto.TaskWorkSetDTO{SiteWorkSetId: p.ParentSiteWorkSetId, WorkSetName: p.ParentWorkSetName})
	}
	if len(validParents) == 0 {
		return nil
	}
	return s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		// upsert 父集实体，回查 DB ID（与 validParents 顺序对齐）
		parentIds, err := s.upsertWorkSets(txCtx, parentDTOs, siteId)
		if err != nil {
			return fmt.Errorf("upsert 父作品集失败: %w", err)
		}
		parentOrders := make(map[int64]int, len(validParents))
		for i, p := range validParents {
			parentDbId := parentIds[i]
			// 环路检测：若 child 已是 parent 的祖先，建 parent→child 会闭合环路，跳过该父
			ancestors, err := s.workSetRelationWriter.CollectAncestorWorkSetIds(txCtx, parentDbId)
			if err != nil {
				return fmt.Errorf("查询父作品集祖先失败(parent=%d): %w", parentDbId, err)
			}
			if slices.Contains(ancestors, childWorkSetId) {
				logger.Log.Warnf("跳过成环的父子关系 parent=%d child=%d", parentDbId, childWorkSetId)
				continue
			}
			// 幂等建立关系：初始 sort_order 取原站序，OnConflict DoNothing 保证重复拉取不覆盖用户后续拖拽
			rel := entity2.NewReWorkSetWorkSet()
			rel.ParentWorkSetID = sql.NullInt64{Int64: parentDbId, Valid: true}
			rel.ChildWorkSetID = sql.NullInt64{Int64: childWorkSetId, Valid: true}
			rel.SortOrder = sql.NullInt64{Int64: p.SortOrder, Valid: true}
			if err := s.workSetRelationWriter.SaveRelation(txCtx, rel); err != nil {
				return fmt.Errorf("建立父子关系失败(parent=%d child=%d): %w", parentDbId, childWorkSetId, err)
			}
			parentOrders[parentDbId] = int(p.SortOrder)
		}
		if len(parentOrders) == 0 {
			return nil
		}
		// 批量写 site_sort_order（覆盖写，支持 re-pull 刷新；不影响本地 sort_order）
		return s.workSetRelationWriter.UpdateSiteSortOrdersForChild(txCtx, childWorkSetId, parentOrders)
	})
}

// saveWorkInfoInTx 事务内执行 SaveWorkInfo 的核心逻辑
func (s *Service) saveWorkInfoInTx(ctx context.Context, task *entity2.Task, workResp *sdkdto.WorkResponse) (int64, error) {
	work := dto2.ToWorkEntity(workResp.Work)

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

	localAuthorDBIds, err := s.resolveLocalAuthors(ctx, workResp.LocalAuthors)
	if err != nil {
		return 0, fmt.Errorf("处理本地作者失败: %w", err)
	}

	localTagDBIds, err := s.resolveLocalTags(ctx, workResp.LocalTags)
	if err != nil {
		return 0, fmt.Errorf("处理本地标签失败: %w", err)
	}
	// === Phase 3: 保存 Work + 关联增量同步 ===
	// SITE 关联由插件权威管理（按 type 删后重建）；LOCAL 关联归用户管理，保留不动，插件返回的 local 增量追加（已存在跳过）；
	// workSet 增量保留（不删历史关联，避免丢失用户手动加的关联与 sort_order 元数据）。
	workId, err := s.saveOrUpdateWork(ctx, work)
	if err != nil {
		return 0, fmt.Errorf("保存作品失败: %w", err)
	}

	// 作者关联：SITE 删后重建，LOCAL 增量追加
	if err := s.reWorkAuthorWriter.DeleteSiteByWorkId(ctx, workId); err != nil {
		return 0, fmt.Errorf("删除作品 SITE 作者关联失败: %w", err)
	}
	siteAuthorLinks := buildSiteAuthorLinks(workId, siteAuthorDBIds)
	if len(siteAuthorLinks) > 0 {
		if err := s.reWorkAuthorWriter.SaveBatch(ctx, siteAuthorLinks); err != nil {
			return 0, fmt.Errorf("保存作品 SITE 作者关联失败: %w", err)
		}
	}
	localAuthorLinks := buildLocalAuthorLinks(workId, localAuthorDBIds)
	if len(localAuthorLinks) > 0 {
		if err := s.reWorkAuthorWriter.SaveBatchOnConflict(ctx, localAuthorLinks); err != nil {
			return 0, fmt.Errorf("保存作品 LOCAL 作者关联失败: %w", err)
		}
	}

	// 标签关联：SITE 删后重建，LOCAL 增量追加
	if err := s.reWorkTagWriter.DeleteSiteByWorkId(ctx, workId); err != nil {
		return 0, fmt.Errorf("删除作品 SITE 标签关联失败: %w", err)
	}
	// re_work_tag.namespace 镜像所指 site_tag.namespace（site 关联）；顺序与 siteTagDBIds 一致（upsertSiteTags 按 dtos 顺序返回）
	siteTagNamespaces := make([]string, len(workResp.SiteTags))
	for i, t := range workResp.SiteTags {
		siteTagNamespaces[i] = t.Namespace
	}
	siteTagLinks := buildSiteTagLinks(workId, siteTagDBIds, siteTagNamespaces)
	if len(siteTagLinks) > 0 {
		if err := s.reWorkTagWriter.SaveBatch(ctx, siteTagLinks); err != nil {
			return 0, fmt.Errorf("保存作品 SITE 标签关联失败: %w", err)
		}
	}
	localTagLinks := buildLocalTagLinks(workId, localTagDBIds)
	if len(localTagLinks) > 0 {
		if err := s.reWorkTagWriter.SaveBatchOnConflict(ctx, localTagLinks); err != nil {
			return 0, fmt.Errorf("保存作品 LOCAL 标签关联失败: %w", err)
		}
	}

	// 作品集关联：增量保留（已存在跳过，不删历史关联）
	if len(workSetDBIds) > 0 {
		// 各 workSet 当前最大 sort_order，供 buildWorkSetLinks 续排（该 work 排末尾，纠正维度错位不再塌 0）。
		// workSetDBIds 为该 work 声明的少数 workSet，逐个查 max 非典型 N+1。
		maxSortOrders := make(map[int64]int64, len(workSetDBIds))
		for _, wsId := range workSetDBIds {
			maxSort, err := s.reWorkWorkSetWriter.MaxSortOrderByWorkSetId(ctx, wsId)
			if err != nil {
				return 0, fmt.Errorf("查询作品集最大排序失败: %w", err)
			}
			maxSortOrders[wsId] = maxSort
		}
		links := buildWorkSetLinks(workId, workSetDBIds, maxSortOrders)
		if err := s.reWorkWorkSetWriter.SaveBatchOnConflict(ctx, links); err != nil {
			return 0, fmt.Errorf("保存作品作品集关联失败: %w", err)
		}
	}

	return workId, nil
}

// upsertSiteAuthors 批量 upsert 站点作者，返回 DB ID 列表（与 dtos 顺序一致）
func (s *Service) upsertSiteAuthors(ctx context.Context, dtos []*sdkdto.TaskSiteAuthorDTO, siteId int64) ([]int64, error) {
	if len(dtos) == 0 {
		return nil, nil
	}

	// 批量 DTO → 实体
	entities := make([]*entity2.SiteAuthor, len(dtos))
	siteAuthorIds := make([]string, len(dtos))
	for i, d := range dtos {
		entities[i] = taskSiteAuthorDTOToEntity(d, siteId)
		siteAuthorIds[i] = d.SiteAuthorId
	}

	// 一次批量 upsert
	if err := s.siteAuthorWriter.BatchUpsert(ctx, entities); err != nil {
		return nil, fmt.Errorf("批量 upsert 站点作者失败: %w", err)
	}

	// 一次批量回查获取 DB ID
	existing, err := s.siteAuthorWriter.ListBySiteAndSiteAuthorIDs(ctx, siteId, siteAuthorIds)
	if err != nil {
		return nil, fmt.Errorf("批量回查站点作者失败: %w", err)
	}

	// 构建按 siteAuthorId → DB ID 的 map
	idMap := make(map[string]int64, len(existing))
	for _, sa := range existing {
		if sa.SiteAuthorID.Valid {
			idMap[sa.SiteAuthorID.String] = sa.ID
		}
	}

	// 按 DTO 原始顺序输出
	ids := make([]int64, len(dtos))
	for i, d := range dtos {
		ids[i] = idMap[d.SiteAuthorId]
	}
	return ids, nil
}

// upsertSiteTags 批量 upsert 站点标签，返回 DB ID 列表（与 dtos 顺序一致）
func (s *Service) upsertSiteTags(ctx context.Context, dtos []*sdkdto.TaskSiteTagDTO, siteId int64) ([]int64, error) {
	if len(dtos) == 0 {
		return nil, nil
	}

	// 批量 DTO → 实体
	entities := make([]*entity2.SiteTag, len(dtos))
	siteTagIds := make([]string, len(dtos))
	for i, d := range dtos {
		entities[i] = taskSiteTagDTOToEntity(d, siteId)
		siteTagIds[i] = d.SiteTagId
	}

	// 一次批量 upsert
	if err := s.siteTagWriter.BatchUpsert(ctx, entities); err != nil {
		return nil, fmt.Errorf("批量 upsert 站点标签失败: %w", err)
	}

	// 一次批量回查获取 DB ID
	existing, err := s.siteTagWriter.ListBySiteAndSiteTagIDs(ctx, siteId, siteTagIds)
	if err != nil {
		return nil, fmt.Errorf("批量回查站点标签失败: %w", err)
	}

	// 构建按 siteTagId → DB ID 的 map
	idMap := make(map[string]int64, len(existing))
	for _, st := range existing {
		if st.SiteTagID.Valid {
			idMap[st.SiteTagID.String] = st.ID
		}
	}

	// 按 DTO 原始顺序输出
	ids := make([]int64, len(dtos))
	for i, d := range dtos {
		ids[i] = idMap[d.SiteTagId]
	}
	return ids, nil
}

// upsertWorkSets 批量 upsert 作品集，返回 DB ID 列表（与 dtos 顺序一致）
func (s *Service) upsertWorkSets(ctx context.Context, dtos []*sdkdto.TaskWorkSetDTO, siteId int64) ([]int64, error) {
	if len(dtos) == 0 {
		return nil, nil
	}

	// 批量 DTO → 实体
	entities := make([]*entity2.WorkSet, len(dtos))
	siteWorkSetIds := make([]string, len(dtos))
	for i, d := range dtos {
		entities[i] = taskWorkSetDTOToEntity(d, siteId)
		siteWorkSetIds[i] = d.SiteWorkSetId
	}

	// 一次批量 upsert
	if err := s.workSetWriter.BatchUpsert(ctx, entities); err != nil {
		return nil, fmt.Errorf("批量 upsert 作品集失败: %w", err)
	}

	// 一次批量回查获取 DB ID
	existing, err := s.workSetWriter.ListBySiteAndSiteWorkSetIDs(ctx, siteId, siteWorkSetIds)
	if err != nil {
		return nil, fmt.Errorf("批量回查作品集失败: %w", err)
	}

	// 构建按 siteWorkSetId → DB ID 的 map
	idMap := make(map[string]int64, len(existing))
	for _, ws := range existing {
		if ws.SiteWorkSetID.Valid {
			idMap[ws.SiteWorkSetID.String] = ws.ID
		}
	}

	// 按 DTO 原始顺序输出
	ids := make([]int64, len(dtos))
	for i, d := range dtos {
		ids[i] = idMap[d.SiteWorkSetId]
	}
	return ids, nil
}

// resolveLocalAuthors 处理本地作者，返回 DB ID 列表
// ID > 0 时校验存在性；ID == 0 时按名称 find-or-create
func (s *Service) resolveLocalAuthors(ctx context.Context, dtos []*sdkdto.LocalAuthorDTO) ([]int64, error) {
	if len(dtos) == 0 {
		return nil, nil
	}

	// 按 ID 模式和名称模式分组
	var idModeIds []int64
	var nameModeDtos []*sdkdto.LocalAuthorDTO
	for _, d := range dtos {
		if d.Id > 0 {
			idModeIds = append(idModeIds, d.Id)
		} else {
			nameModeDtos = append(nameModeDtos, d)
		}
	}

	// ID 模式：校验存在性并回填缺失的 AuthorName
	var idModeResults []*entity2.LocalAuthor
	if len(idModeIds) > 0 {
		var err error
		idModeResults, err = s.localAuthorBatchReader.ListByIds(ctx, idModeIds)
		if err != nil {
			return nil, fmt.Errorf("查询本地作者失败: %w", err)
		}
		if len(idModeResults) != len(idModeIds) {
			found := make(map[int64]struct{}, len(idModeResults))
			for _, r := range idModeResults {
				found[r.ID] = struct{}{}
			}
			for _, id := range idModeIds {
				if _, ok := found[id]; !ok {
					return nil, fmt.Errorf("本地作者不存在: ID=%d", id)
				}
			}
		}
		// 回填 DTO 中缺失的 AuthorName（插件可能只传了 ID）
		enrichLocalAuthorDTOs(dtos, idModeResults)
	}

	// 名称模式：收集去重名称 → 批量查询 → 创建不存在的
	nameModeIds := make([]int64, 0, len(nameModeDtos))
	if len(nameModeDtos) > 0 {
		// 校验名称有效性
		names := make([]string, 0, len(nameModeDtos))
		nameSet := make(map[string]struct{}, len(nameModeDtos))
		for _, d := range nameModeDtos {
			name := ""
			if d.AuthorName != nil {
				name = *d.AuthorName
			}
			if name == "" {
				return nil, fmt.Errorf("本地作者 ID 为 0 时 AuthorName 不能为空")
			}
			if _, ok := nameSet[name]; !ok {
				names = append(names, name)
				nameSet[name] = struct{}{}
			}
		}

		// 批量查询已有作者
		existing, err := s.localAuthorFindOrCreator.GetByNames(ctx, names)
		if err != nil {
			return nil, fmt.Errorf("查询本地作者失败: %w", err)
		}
		existingMap := make(map[string]int64, len(existing))
		for _, a := range existing {
			if a.AuthorName.Valid {
				existingMap[a.AuthorName.String] = a.ID
			}
		}

		// 创建不存在的作者（已存在的不做任何修改）
		var newAuthors []*entity2.LocalAuthor
		newAuthorNames := make(map[string]*entity2.LocalAuthor)
		for _, d := range nameModeDtos {
			name := *d.AuthorName
			if _, ok := existingMap[name]; ok {
				continue
			}
			// 同名 DTO 可能出现多次，避免重复创建
			if _, dup := newAuthorNames[name]; dup {
				continue
			}

			author := entity2.NewLocalAuthor()
			author.AuthorName = sql.NullString{String: name, Valid: true}
			if d.Introduce != nil && *d.Introduce != "" {
				author.Introduce = sql.NullString{String: *d.Introduce, Valid: true}
			}
			newAuthors = append(newAuthors, author)
			newAuthorNames[name] = author
		}
		if len(newAuthors) > 0 {
			if err := s.localAuthorFindOrCreator.SaveBatch(ctx, newAuthors); err != nil {
				return nil, fmt.Errorf("批量创建本地作者失败: %w", err)
			}
			for name, author := range newAuthorNames {
				existingMap[name] = author.ID
			}
		}

		// 按 DTO 顺序收集 ID
		for _, d := range nameModeDtos {
			if id, ok := existingMap[*d.AuthorName]; ok {
				nameModeIds = append(nameModeIds, id)
			}
		}
	}

	// 按 DTO 原始顺序合并结果
	ids := make([]int64, 0, len(dtos))
	idIdx := 0
	nameIdx := 0
	for _, d := range dtos {
		if d.Id > 0 {
			ids = append(ids, idModeIds[idIdx])
			idIdx++
		} else {
			ids = append(ids, nameModeIds[nameIdx])
			nameIdx++
		}
	}
	return ids, nil
}

// resolveLocalTags 处理本地标签，返回 DB ID 列表
// ID > 0 时校验存在性；ID == 0 时按名称 find-or-create
func (s *Service) resolveLocalTags(ctx context.Context, dtos []*sdkdto.LocalTagDTO) ([]int64, error) {
	if len(dtos) == 0 {
		return nil, nil
	}

	// 按 ID 模式和名称模式分组
	var idModeIds []int64
	var nameModeDtos []*sdkdto.LocalTagDTO
	for _, d := range dtos {
		if d.Id > 0 {
			idModeIds = append(idModeIds, d.Id)
		} else {
			nameModeDtos = append(nameModeDtos, d)
		}
	}

	// ID 模式：校验存在性并回填缺失的 LocalTagName
	var idModeResults []*entity2.LocalTag
	if len(idModeIds) > 0 {
		var err error
		idModeResults, err = s.localTagBatchReader.ListByIds(ctx, idModeIds)
		if err != nil {
			return nil, fmt.Errorf("查询本地标签失败: %w", err)
		}
		if len(idModeResults) != len(idModeIds) {
			found := make(map[int64]struct{}, len(idModeResults))
			for _, r := range idModeResults {
				found[r.ID] = struct{}{}
			}
			for _, id := range idModeIds {
				if _, ok := found[id]; !ok {
					return nil, fmt.Errorf("本地标签不存在: ID=%d", id)
				}
			}
		}
		// 回填 DTO 中缺失的 LocalTagName（插件可能只传了 ID）
		enrichLocalTagDTOs(dtos, idModeResults)
	}

	// 名称模式：收集去重名称 → 批量查询 → 创建不存在的
	nameModeIds := make([]int64, 0, len(nameModeDtos))
	if len(nameModeDtos) > 0 {
		// 校验名称有效性
		names := make([]string, 0, len(nameModeDtos))
		nameSet := make(map[string]struct{}, len(nameModeDtos))
		for _, d := range nameModeDtos {
			name := ""
			if d.LocalTagName != nil {
				name = *d.LocalTagName
			}
			if name == "" {
				return nil, fmt.Errorf("本地标签 ID 为 0 时 LocalTagName 不能为空")
			}
			if _, ok := nameSet[name]; !ok {
				names = append(names, name)
				nameSet[name] = struct{}{}
			}
		}

		// 批量查询已有标签
		existing, err := s.localTagFindOrCreator.GetByNames(ctx, names)
		if err != nil {
			return nil, fmt.Errorf("查询本地标签失败: %w", err)
		}
		existingMap := make(map[string]int64, len(existing))
		for _, t := range existing {
			if t.LocalTagName.Valid {
				existingMap[t.LocalTagName.String] = t.ID
			}
		}

		// 创建不存在的标签（已存在的不做任何修改）
		now := util.GetCurrentTimestamp()
		var newTags []*entity2.LocalTag
		newTagNames := make(map[string]*entity2.LocalTag)
		for _, name := range names {
			if _, ok := existingMap[name]; ok {
				continue
			}
			if _, dup := newTagNames[name]; dup {
				continue
			}
			tag := entity2.NewLocalTag()
			tag.LocalTagName = sql.NullString{String: name, Valid: true}
			tag.BaseLocalTagID = sql.NullInt64{Int64: 0, Valid: true}
			tag.LastUse = sql.NullInt64{Int64: now, Valid: true}
			newTags = append(newTags, tag)
			newTagNames[name] = tag
		}
		if len(newTags) > 0 {
			if err := s.localTagFindOrCreator.SaveBatch(ctx, newTags); err != nil {
				return nil, fmt.Errorf("批量创建本地标签失败: %w", err)
			}
			for name, tag := range newTagNames {
				existingMap[name] = tag.ID
			}
		}

		// 按 DTO 顺序收集 ID
		for _, d := range nameModeDtos {
			if id, ok := existingMap[*d.LocalTagName]; ok {
				nameModeIds = append(nameModeIds, id)
			}
		}
	}

	// 按 DTO 原始顺序合并结果
	ids := make([]int64, 0, len(dtos))
	idIdx := 0
	nameIdx := 0
	for _, d := range dtos {
		if d.Id > 0 {
			ids = append(ids, idModeIds[idIdx])
			idIdx++
		} else {
			ids = append(ids, nameModeIds[nameIdx])
			nameIdx++
		}
	}
	return ids, nil
}

// saveOrUpdateWork 按复合键保存或更新作品
func (s *Service) saveOrUpdateWork(ctx context.Context, work *entity2.Work) (int64, error) {
	existing, err := s.repo.GetBySiteAndSiteWorkID(ctx, work.SiteID.Int64, work.SiteWorkID.String)
	if err == nil && existing != nil {
		work.ID = existing.ID
		if err := s.repo.Updates(ctx, work); err != nil {
			return 0, err
		}
		return existing.ID, nil
	}
	if err := s.repo.Create(ctx, work); err != nil {
		return 0, err
	}
	return work.ID, nil
}

// ========== DTO 转换辅助函数 ==========

func taskSiteAuthorDTOToEntity(d *sdkdto.TaskSiteAuthorDTO, siteId int64) *entity2.SiteAuthor {
	return &entity2.SiteAuthor{
		BaseEntity:      &model.BaseEntity{},
		SiteID:          sql.NullInt64{Int64: siteId, Valid: true},
		SiteAuthorID:    sql.NullString{String: d.SiteAuthorId, Valid: true},
		AuthorName:      sql.NullString{String: d.AuthorName, Valid: true},
		FixedAuthorName: sql.NullString{String: d.FixedAuthorName, Valid: d.FixedAuthorName != ""},
		Introduce:       sql.NullString{String: d.Introduce, Valid: d.Introduce != ""},
		Homepage:        sql.NullString{String: d.Homepage, Valid: d.Homepage != ""},
	}
}

func taskSiteTagDTOToEntity(d *sdkdto.TaskSiteTagDTO, siteId int64) *entity2.SiteTag {
	return &entity2.SiteTag{
		BaseEntity:  &model.BaseEntity{},
		SiteID:      sql.NullInt64{Int64: siteId, Valid: true},
		SiteTagID:   sql.NullString{String: d.SiteTagId, Valid: true},
		SiteTagName: sql.NullString{String: d.TagName, Valid: true},
		Description: sql.NullString{String: d.Description, Valid: d.Description != ""},
		Namespace:   sql.NullString{String: d.Namespace, Valid: d.Namespace != ""},
	}
}

func taskWorkSetDTOToEntity(d *sdkdto.TaskWorkSetDTO, siteId int64) *entity2.WorkSet {
	return &entity2.WorkSet{
		BaseEntity:      &model.BaseEntity{},
		SiteID:          sql.NullInt64{Int64: siteId, Valid: true},
		SiteWorkSetID:   sql.NullString{String: d.SiteWorkSetId, Valid: true},
		SiteWorkSetName: sql.NullString{String: d.WorkSetName, Valid: true},
	}
}

// ========== 关联实体构建辅助函数 ==========

func buildSiteAuthorLinks(workId int64, siteAuthorIds []int64) []*entity2.ReWorkAuthor {
	links := make([]*entity2.ReWorkAuthor, 0, len(siteAuthorIds))
	for i, authorId := range siteAuthorIds {
		links = append(links, &entity2.ReWorkAuthor{
			BaseEntity:   &model.BaseEntity{},
			AuthorType:   sql.NullInt64{Int64: constant.SITE, Valid: true},
			WorkID:       sql.NullInt64{Int64: workId, Valid: true},
			SiteAuthorID: sql.NullInt64{Int64: authorId, Valid: true},
			SortOrder:    sql.NullInt64{Int64: int64(i), Valid: true},
		})
	}
	return links
}

func buildSiteTagLinks(workId int64, siteTagIds []int64, namespaces []string) []*entity2.ReWorkTag {
	links := make([]*entity2.ReWorkTag, 0, len(siteTagIds))
	for i, tagId := range siteTagIds {
		ns := ""
		if i < len(namespaces) {
			ns = namespaces[i]
		}
		links = append(links, &entity2.ReWorkTag{
			BaseEntity: &model.BaseEntity{},
			WorkID:     sql.NullInt64{Int64: workId, Valid: true},
			TagType:    sql.NullInt64{Int64: constant.SITE, Valid: true},
			SiteTagID:  sql.NullInt64{Int64: tagId, Valid: true},
			Namespace:  sql.NullString{String: ns, Valid: ns != ""},
		})
	}
	return links
}

func buildLocalAuthorLinks(workId int64, localAuthorIds []int64) []*entity2.ReWorkAuthor {
	links := make([]*entity2.ReWorkAuthor, 0, len(localAuthorIds))
	for i, authorId := range localAuthorIds {
		links = append(links, &entity2.ReWorkAuthor{
			BaseEntity:    &model.BaseEntity{},
			AuthorType:    sql.NullInt64{Int64: constant.LOCAL, Valid: true},
			WorkID:        sql.NullInt64{Int64: workId, Valid: true},
			LocalAuthorID: sql.NullInt64{Int64: authorId, Valid: true},
			SortOrder:     sql.NullInt64{Int64: int64(i), Valid: true},
		})
	}
	return links
}

func buildLocalTagLinks(workId int64, localTagIds []int64) []*entity2.ReWorkTag {
	links := make([]*entity2.ReWorkTag, 0, len(localTagIds))
	for _, tagId := range localTagIds {
		links = append(links, &entity2.ReWorkTag{
			BaseEntity: &model.BaseEntity{},
			WorkID:     sql.NullInt64{Int64: workId, Valid: true},
			TagType:    sql.NullInt64{Int64: constant.LOCAL, Valid: true},
			LocalTagID: sql.NullInt64{Int64: tagId, Valid: true},
		})
	}
	return links
}

// buildWorkSetLinks 构造 work→各 workSet 关联；sort_order 按各 workSet 当前 max+1 续排（该 work 排末尾），
// 纠正旧实现用「workSet 下标」当 sort_order 的维度错位（致集内全塌 0）。site_sort_order 留空待插件拉取。
func buildWorkSetLinks(workId int64, workSetIds []int64, maxSortOrders map[int64]int64) []*entity2.ReWorkWorkSet {
	links := make([]*entity2.ReWorkWorkSet, 0, len(workSetIds))
	for _, wsId := range workSetIds {
		rel := entity2.NewReWorkWorkSet()
		rel.WorkID = sql.NullInt64{Int64: workId, Valid: true}
		rel.WorkSetID = sql.NullInt64{Int64: wsId, Valid: true}
		rel.SortOrder = sql.NullInt64{Int64: maxSortOrders[wsId] + 1, Valid: true}
		links = append(links, rel)
	}
	return links
}

// enrichLocalAuthorDTOs 用数据库实体回填 DTO 中缺失的 AuthorName
func enrichLocalAuthorDTOs(dtos []*sdkdto.LocalAuthorDTO, entities []*entity2.LocalAuthor) {
	if len(dtos) == 0 || len(entities) == 0 {
		return
	}
	entityMap := make(map[int64]*entity2.LocalAuthor, len(entities))
	for _, e := range entities {
		entityMap[e.GetID()] = e
	}
	for _, d := range dtos {
		if d.Id <= 0 {
			continue
		}
		if d.AuthorName != nil && *d.AuthorName != "" {
			continue
		}
		if e, ok := entityMap[d.Id]; ok && e.AuthorName.Valid {
			d.AuthorName = &e.AuthorName.String
		}
	}
}

// enrichLocalTagDTOs 用数据库实体回填 DTO 中缺失的 LocalTagName
func enrichLocalTagDTOs(dtos []*sdkdto.LocalTagDTO, entities []*entity2.LocalTag) {
	if len(dtos) == 0 || len(entities) == 0 {
		return
	}
	entityMap := make(map[int64]*entity2.LocalTag, len(entities))
	for _, e := range entities {
		entityMap[e.GetID()] = e
	}
	for _, d := range dtos {
		if d.Id <= 0 {
			continue
		}
		if d.LocalTagName != nil && *d.LocalTagName != "" {
			continue
		}
		if e, ok := entityMap[d.Id]; ok && e.LocalTagName.Valid {
			d.LocalTagName = &e.LocalTagName.String
		}
	}
}
