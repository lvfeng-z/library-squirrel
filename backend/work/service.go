package work

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/recycleBin"
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
	ListByWorkId(ctx context.Context, workId int64) ([]*sdkdto.RankedSiteAuthor, error)
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
	ListReWorkAuthor(ctx context.Context, workIds []int64) (map[int64][]*sdkdto.RankedLocalAuthor, error)
	// ListByIds 根据ID列表批量查询
	ListByIds(ctx context.Context, ids []int64) ([]*entity2.LocalAuthor, error)
}

// SiteAuthorBatchReader 站点作者批量读取接口
type SiteAuthorBatchReader interface {
	// ListSiteAuthorsByWorkIds 批量查询作品关联的站点作者，按 workId 分组
	ListSiteAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*sdkdto.RankedSiteAuthor, error)
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
	// SaveBatch 批量保存关联
	SaveBatch(ctx context.Context, rels []*entity2.ReWorkTag) error
}

// ReWorkWorkSetWriter 作品-作品集关联写入接口
type ReWorkWorkSetWriter interface {
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
	// CreateBatch 批量新建关联
	CreateBatch(ctx context.Context, rels []*entity2.ReWorkWorkSet) error
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
	// Delete 删除记录及对应文件
	// backup: 是否对已完成文件进行移动备份
	Delete(ctx context.Context, id int64, backup bool) (int64, error)
}

// ReWorkTagReader 作品-标签关联读取接口（逻辑删除快照采集）
type ReWorkTagReader interface {
	// ListByWorkId 查询作品关联的所有标签关联记录
	ListByWorkId(ctx context.Context, workId int64) ([]*entity2.ReWorkTag, error)
}

// ReWorkAuthorReader 作品-作者关联读取接口（逻辑删除快照采集）
type ReWorkAuthorReader interface {
	// ListRelationsByWorkId 查询作品关联的所有作者关联记录（含 role_name/sort_order）
	ListRelationsByWorkId(ctx context.Context, workId int64) ([]*entity2.ReWorkAuthor, error)
}

// ReWorkWorkSetReader 作品-作品集关联读取接口（逻辑删除快照采集）
type ReWorkWorkSetReader interface {
	// ListRelationsByWorkId 查询作品关联的所有作品集关联记录（含 is_cover/sort_order）
	ListRelationsByWorkId(ctx context.Context, workId int64) ([]*entity2.ReWorkWorkSet, error)
}

// StoreReader PersistentStore 读取接口（逻辑删除前采集资源文件元数据）
type StoreReader interface {
	// GetById 根据 ID 获取
	GetById(ctx context.Context, id int64) (*entity2.PersistentStore, error)
}

// RecycleItemSaver 回收站条目保存接口（逻辑删除写入快照）
type RecycleItemSaver interface {
	// Create 保存回收站条目
	Create(ctx context.Context, item *entity2.RecycleItem) error
}

// BackupMover 文件移动备份接口（逻辑删除时把资源文件移入 backup 目录）
type BackupMover interface {
	// MoveToBackup 将文件移动到 backup 目录并创建 Backup 记录，返回 Backup 记录 ID
	MoveToBackup(ctx context.Context, sourceId int64, absFilePath string, originalFilePath string, originalFileName string, originalFilenameExtension string) (int64, error)
}

// StoreRecordDeleter PersistentStore 记录删除接口（仅删 DB 记录，磁盘文件已另行处理）
type StoreRecordDeleter interface {
	// DeleteRecord 仅删除数据库记录
	DeleteRecord(ctx context.Context, id int64) error
}

// RunningTaskStopper 运行中任务停止接口（逻辑删除前停止关联任务实例，防止重建作品）
// task 记录不在删除范围，仅停止内存中的运行实例
type RunningTaskStopper interface {
	// StopRunningBySiteWork 停止指定作品关联的运行中任务实例
	StopRunningBySiteWork(ctx context.Context, siteId int64, siteWorkId string) error
}

// ResourceSaver 资源保存接口（复原时重建 resource 记录）
type ResourceSaver interface {
	// Save 保存资源记录
	Save(ctx context.Context, resource *entity2.Resource) error
}

// ResourceStoreSaver resource_store 行保存接口(复原时重建关联)
type ResourceStoreSaver interface {
	CreateBatch(ctx context.Context, stores []*entity2.ResourceStore) error
}

// SiteAuthorByIdsReader 站点作者按 ID 批量查询接口（复原时引用校验）
type SiteAuthorByIdsReader interface {
	// ListBySiteAuthorIds 根据站点作者 ID 列表批量查询
	ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) ([]*entity2.SiteAuthor, error)
}

// WorkSetByIdsReader 作品集按 ID 批量查询接口（复原时引用校验）
type WorkSetByIdsReader interface {
	// ListByIds 根据 ID 列表批量查询
	ListByIds(ctx context.Context, ids []int64) ([]*entity2.WorkSet, error)
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
	// Delete 删除
	Delete(ctx context.Context, id int64) error
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

	// 逻辑删除/复原所需的关联读取接口
	reWorkTagReader     ReWorkTagReader
	reWorkAuthorReader  ReWorkAuthorReader
	reWorkWorkSetReader ReWorkWorkSetReader
	storeReader         StoreReader

	// 逻辑删除（SoftDeleteWork）所需接口与配置
	recycleItemSaver   RecycleItemSaver
	backupMover        BackupMover
	storeRecordDeleter StoreRecordDeleter
	runningTaskStopper RunningTaskStopper // 可选，nil 时跳过任务停止
	workDirGetter      func() string

	// 复原（RestoreWorkFromSnapshot）所需接口
	resourceSaver         ResourceSaver
	resourceStoreSaver    ResourceStoreSaver
	siteAuthorByIdsReader SiteAuthorByIdsReader
	workSetByIdsReader    WorkSetByIdsReader
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
	reWorkTagReader ReWorkTagReader,
	reWorkAuthorReader ReWorkAuthorReader,
	reWorkWorkSetReader ReWorkWorkSetReader,
	storeReader StoreReader,
	recycleItemSaver RecycleItemSaver,
	backupMover BackupMover,
	storeRecordDeleter StoreRecordDeleter,
	runningTaskStopper RunningTaskStopper,
	workDirGetter func() string,
	resourceSaver ResourceSaver,
	resourceStoreSaver ResourceStoreSaver,
	siteAuthorByIdsReader SiteAuthorByIdsReader,
	workSetByIdsReader WorkSetByIdsReader,
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
		reWorkTagReader:          reWorkTagReader,
		reWorkAuthorReader:       reWorkAuthorReader,
		reWorkWorkSetReader:      reWorkWorkSetReader,
		storeReader:              storeReader,
		recycleItemSaver:         recycleItemSaver,
		backupMover:              backupMover,
		storeRecordDeleter:       storeRecordDeleter,
		runningTaskStopper:       runningTaskStopper,
		workDirGetter:            workDirGetter,
		resourceSaver:            resourceSaver,
		resourceStoreSaver:       resourceStoreSaver,
		siteAuthorByIdsReader:    siteAuthorByIdsReader,
		workSetByIdsReader:       workSetByIdsReader,
	}
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

// Delete 删除作品
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
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
		if err := s.resourceDeleter.DeleteByWorkId(txCtx, id); err != nil {
			return err
		}
		return s.repo.Delete(txCtx, id)
	})
	if err != nil {
		return err
	}

	// 事务后：删除磁盘文件（尽力而为，失败不影响业务）
	if s.storeDeleter != nil {
		for _, storeId := range storeIds {
			s.storeDeleter.Delete(ctx, storeId, false)
		}
	}
	return nil
}

// SoftDeleteWork 逻辑删除作品（移入回收站，可经回收站复原）
//
// 流程：
//  1. 收集 work 及其作者/标签/作品集关联、resource（含 store id）
//  2. 事务外：资源文件经 BackupMover 移入 backup 目录（移动文件 + 建 Backup 记录，不删 persistent_store 记录）
//  3. 构建 recycle_bin 快照（关联元数据 + resource 的 backup id 映射）
//  4. 事务内：写 recycle_bin → 删 persistent_store 记录 → 删三类关联 → 删 resource → 删 work
//  5. 停止关联的运行中任务实例（task 记录保留）
//
// 资源文件移动在事务外（文件 IO 不可回滚），事务失败时文件已安全落在 backup 目录、DB 完整回滚。
func (s *Service) SoftDeleteWork(ctx context.Context, workId int64) error {
	// 1. 校验 work 存在
	work, err := s.repo.GetById(ctx, workId)
	if err != nil {
		return err
	}
	if work == nil {
		return ErrWorkNotFound
	}

	// 2. 收集关联数据
	resources, err := s.resourceDeleter.ListByWorkId(ctx, workId)
	if err != nil {
		return fmt.Errorf("查询作品资源失败: %w", err)
	}
	authors, err := s.reWorkAuthorReader.ListRelationsByWorkId(ctx, workId)
	if err != nil {
		return fmt.Errorf("查询作品作者关联失败: %w", err)
	}
	tags, err := s.reWorkTagReader.ListByWorkId(ctx, workId)
	if err != nil {
		return fmt.Errorf("查询作品标签关联失败: %w", err)
	}
	workSets, err := s.reWorkWorkSetReader.ListRelationsByWorkId(ctx, workId)
	if err != nil {
		return fmt.Errorf("查询作品作品集关联失败: %w", err)
	}

	// 3. [事务外] 资源文件备份 + 构建 resource 快照(从 resource_store 收集 store,不读旧列)
	workDir := s.workDirGetter()
	// 批量查询 resource_store
	resourceIds := make([]int64, 0, len(resources))
	for _, res := range resources {
		resourceIds = append(resourceIds, res.GetID())
	}
	rsStoreMap, _ := s.resourceStoreBatchReader.ListStoresByResourceIds(ctx, resourceIds)

	resourceSnapshots := make([]recycleBin.ResourceSnapshot, 0, len(resources))
	type storeBackupRef struct {
		storeId  int64
		backupId int64
	}
	var storeBackups []storeBackupRef
	for _, res := range resources {
		rs := recycleBin.ResourceSnapshot{
			TaskID:           res.TaskID,
			SuggestName:      res.SuggestName,
			ResourceComplete: int(res.ResourceComplete.Int64),
		}
		// 遍历 resource_store 行备份(v1 快照格式)
		for _, rsRow := range rsStoreMap[res.GetID()] {
			backupId, err := s.backupStore(ctx, rsRow.StoreID, workDir)
			if err != nil {
				return err
			}
			rs.StoreBackups = append(rs.StoreBackups, recycleBin.StoreBackupRef{
				StoreType: rsRow.StoreType,
				BackupID:  backupId,
			})
			storeBackups = append(storeBackups, storeBackupRef{storeId: rsRow.StoreID, backupId: backupId})
		}
		resourceSnapshots = append(resourceSnapshots, rs)
	}

	// 4. 构建完整快照
	snapshot := &recycleBin.WorkRecycleSnapshot{
		Work: recycleBin.WorkSnapshot{
			SiteID:              work.SiteID,
			SiteWorkID:          work.SiteWorkID,
			SiteWorkName:        work.SiteWorkName,
			SiteAuthorID:        work.SiteAuthorID,
			SiteWorkDescription: work.SiteWorkDescription,
			SiteUploadTime:      work.SiteUploadTime,
			SiteUpdateTime:      work.SiteUpdateTime,
			NickName:            work.NickName,
			LocalAuthorID:       work.LocalAuthorID,
			LastView:            work.LastView,
		},
		Resources: resourceSnapshots,
	}
	for _, a := range authors {
		snapshot.Authors = append(snapshot.Authors, recycleBin.AuthorSnapshot{
			AuthorType:    a.AuthorType,
			LocalAuthorID: a.LocalAuthorID,
			SiteAuthorID:  a.SiteAuthorID,
			RoleName:      a.RoleName,
			SortOrder:     a.SortOrder,
		})
	}
	for _, t := range tags {
		snapshot.Tags = append(snapshot.Tags, recycleBin.TagSnapshot{
			TagType:    t.TagType,
			LocalTagID: t.LocalTagID,
			SiteTagID:  t.SiteTagID,
		})
	}
	for _, ws := range workSets {
		snapshot.WorkSets = append(snapshot.WorkSets, recycleBin.WorkSetSnapshot{
			WorkSetID: ws.WorkSetID,
			IsCover:   ws.IsCover,
			SortOrder: ws.SortOrder,
		})
	}
	snapshotJSON, err := recycleBin.MarshalSnapshot(snapshot)
	if err != nil {
		return err
	}

	// 构建 recycle_bin 条目
	recycleItem := entity2.NewRecycleItem()
	recycleItem.WorkID = sql.NullInt64{Int64: workId, Valid: true}
	recycleItem.SiteID = work.SiteID
	recycleItem.SiteWorkID = work.SiteWorkID
	recycleItem.WorkName = work.SiteWorkName
	recycleItem.DeleteTime = util.GetCurrentTimestamp()
	recycleItem.Snapshot = snapshotJSON

	// 5. [事务内] 写 recycle_bin + 删 persistent_store 记录 + 删关联 + 删 resource + 删 work
	err = s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		if err := s.recycleItemSaver.Create(txCtx, recycleItem); err != nil {
			return err
		}
		for _, sb := range storeBackups {
			if err := s.storeRecordDeleter.DeleteRecord(txCtx, sb.storeId); err != nil {
				return err
			}
		}
		if err := s.reWorkTagWriter.DeleteByWorkId(txCtx, workId); err != nil {
			return err
		}
		if err := s.reWorkWorkSetWriter.DeleteByWorkId(txCtx, workId); err != nil {
			return err
		}
		if err := s.reWorkAuthorWriter.DeleteByWorkId(txCtx, workId); err != nil {
			return err
		}
		if err := s.resourceDeleter.DeleteByWorkId(txCtx, workId); err != nil {
			return err
		}
		return s.repo.Delete(txCtx, workId)
	})
	if err != nil {
		return err
	}

	// 6. 停止关联的运行中任务实例（task 记录保留；接口由 taskManager 实现，nil 时跳过）
	if s.runningTaskStopper != nil && work.SiteID.Valid && work.SiteWorkID.Valid {
		if err := s.runningTaskStopper.StopRunningBySiteWork(ctx, work.SiteID.Int64, work.SiteWorkID.String); err != nil {
			logger.Log.Warnf("停止作品 %d 关联运行中任务失败: %v", workId, err)
		}
	}
	return nil
}

// backupStore 将单个 PersistentStore 的文件移入 backup 目录，返回 Backup 记录 ID
// 文件移动后原 persistent_store 记录保留，由调用方在事务内统一删除
func (s *Service) backupStore(ctx context.Context, storeId int64, workDir string) (int64, error) {
	store, err := s.storeReader.GetById(ctx, storeId)
	if err != nil {
		// store 记录查询失败(含脏数据 record not found:上游失败留下指向已删 store 的 resource_store 关联):
		// 不阻断,返回 BackupID=0 由调用方跳过(删作品/替换不应因脏 store 失败)
		logger.Log.Warnf("备份查询 storeId=%d 失败,跳过: %v", storeId, err)
		return 0, nil
	}
	if store == nil || !store.FilePath.Valid {
		return 0, nil
	}
	absFilePath := filepath.Join(workDir, store.FilePath.String)
	originalFileName := ""
	if store.FileName.Valid {
		originalFileName = store.FileName.String
	}
	originalExt := ""
	if store.FilenameExtension.Valid {
		originalExt = store.FilenameExtension.String
	}
	backupId, err := s.backupMover.MoveToBackup(ctx, storeId, absFilePath, store.FilePath.String, originalFileName, originalExt)
	if err != nil {
		return 0, fmt.Errorf("移动资源文件到备份失败: %w", err)
	}
	return backupId, nil
}

// HardDeleteWork 物理删除作品（数据库记录 + 资源文件）
// 仅供复原"覆盖"分支内部调用，不暴露前端
func (s *Service) HardDeleteWork(ctx context.Context, workId int64) error {
	// 停止关联运行中任务（nil 时跳过）
	if s.runningTaskStopper != nil {
		work, err := s.repo.GetById(ctx, workId)
		if err != nil {
			return err
		}
		if work != nil && work.SiteID.Valid && work.SiteWorkID.Valid {
			if err := s.runningTaskStopper.StopRunningBySiteWork(ctx, work.SiteID.Int64, work.SiteWorkID.String); err != nil {
				logger.Log.Warnf("停止作品 %d 关联运行中任务失败: %v", workId, err)
			}
		}
	}
	return s.DeleteWorkAndSurroundingData(ctx, workId)
}

// RestoreWorkFromSnapshot 从回收站快照重建作品及其关联、resource（事务由调用方通过 ctx 管理）
// storeIdByBackupId: backup 记录 ID → 还原后的新 persistent_store ID 映射
// 引用校验：snapshot 中引用了已不存在实体的关联行跳过（部分复原）
// 返回新作品 ID
func (s *Service) RestoreWorkFromSnapshot(ctx context.Context, snapshot *recycleBin.WorkRecycleSnapshot, storeIdByBackupId map[int64]int64) (int64, error) {
	// 1. 引用校验：批量查询 snapshot 引用的实体是否存在
	existingLocalAuthorIds, existingSiteAuthorIds, existingLocalTagIds, existingSiteTagIds, existingWorkSetIds, err := s.resolveExistingRefs(ctx, snapshot)
	if err != nil {
		return 0, err
	}

	// 2. 重建 work
	work := entity2.NewWork()
	work.SiteID = snapshot.Work.SiteID
	work.SiteWorkID = snapshot.Work.SiteWorkID
	work.SiteWorkName = snapshot.Work.SiteWorkName
	work.SiteAuthorID = snapshot.Work.SiteAuthorID
	work.SiteWorkDescription = snapshot.Work.SiteWorkDescription
	work.SiteUploadTime = snapshot.Work.SiteUploadTime
	work.SiteUpdateTime = snapshot.Work.SiteUpdateTime
	work.NickName = snapshot.Work.NickName
	work.LocalAuthorID = snapshot.Work.LocalAuthorID
	work.LastView = snapshot.Work.LastView
	if err := s.repo.Create(ctx, work); err != nil {
		return 0, fmt.Errorf("重建作品失败: %w", err)
	}
	workId := work.GetID()

	// 3. 重建 resource + resource_store（store_id 用 backup 映射,不写旧列）
	for _, rs := range snapshot.Resources {
		resource := entity2.NewResource()
		resource.WorkID = workId
		resource.TaskID = rs.TaskID
		resource.SuggestName = rs.SuggestName
		resource.ResourceComplete = sql.NullInt64{Int64: int64(rs.ResourceComplete), Valid: true}
		if err := s.resourceSaver.Save(ctx, resource); err != nil {
			return workId, fmt.Errorf("重建资源记录失败: %w", err)
		}
		// 从快照还原 resource_store 行(v0/v1 兼容)
		var rsStores []*entity2.ResourceStore
		for _, sb := range recycleBin.SnapshotStoreBackups(&rs) {
			if sb.BackupID <= 0 {
				continue
			}
			newStoreId, ok := storeIdByBackupId[sb.BackupID]
			if !ok || newStoreId <= 0 {
				continue
			}
			rsRow := entity2.NewResourceStore()
			rsRow.ResourceID = resource.GetID()
			rsRow.StoreType = sb.StoreType
			rsRow.Generation = entity2.GenerationDownloaded // 还原的 store 默认 downloaded
			rsRow.StoreID = newStoreId
			rsStores = append(rsStores, rsRow)
		}
		if len(rsStores) > 0 {
			if err := s.resourceStoreSaver.CreateBatch(ctx, rsStores); err != nil {
				return workId, fmt.Errorf("重建 resource_store 失败: %w", err)
			}
		}
	}

	// 4. 重建作品-作者关联（仅引用校验通过的，保留 role_name/sort_order）
	var authorLinks []*entity2.ReWorkAuthor
	for _, a := range snapshot.Authors {
		link := &entity2.ReWorkAuthor{
			BaseEntity: &model.BaseEntity{},
			AuthorType: a.AuthorType,
			WorkID:     sql.NullInt64{Int64: workId, Valid: true},
			RoleName:   a.RoleName,
			SortOrder:  a.SortOrder,
		}
		if a.AuthorType.Valid && a.AuthorType.Int64 == AuthorTypeLocal && a.LocalAuthorID.Valid {
			if existingLocalAuthorIds[a.LocalAuthorID.Int64] {
				link.LocalAuthorID = a.LocalAuthorID
				authorLinks = append(authorLinks, link)
			}
		} else if a.AuthorType.Valid && a.AuthorType.Int64 == AuthorTypeSite && a.SiteAuthorID.Valid {
			if existingSiteAuthorIds[a.SiteAuthorID.Int64] {
				link.SiteAuthorID = a.SiteAuthorID
				authorLinks = append(authorLinks, link)
			}
		}
	}
	if len(authorLinks) > 0 {
		if err := s.reWorkAuthorWriter.SaveBatch(ctx, authorLinks); err != nil {
			return workId, fmt.Errorf("重建作品作者关联失败: %w", err)
		}
	}

	// 5. 重建作品-标签关联（仅引用校验通过的）
	var tagLinks []*entity2.ReWorkTag
	for _, t := range snapshot.Tags {
		link := &entity2.ReWorkTag{
			BaseEntity: &model.BaseEntity{},
			WorkID:     sql.NullInt64{Int64: workId, Valid: true},
			TagType:    t.TagType,
		}
		if t.LocalTagID.Valid && t.LocalTagID.Int64 > 0 && existingLocalTagIds[t.LocalTagID.Int64] {
			link.LocalTagID = t.LocalTagID
			tagLinks = append(tagLinks, link)
		} else if t.SiteTagID.Valid && t.SiteTagID.Int64 > 0 && existingSiteTagIds[t.SiteTagID.Int64] {
			link.SiteTagID = t.SiteTagID
			tagLinks = append(tagLinks, link)
		}
	}
	if len(tagLinks) > 0 {
		if err := s.reWorkTagWriter.SaveBatch(ctx, tagLinks); err != nil {
			return workId, fmt.Errorf("重建作品标签关联失败: %w", err)
		}
	}

	// 6. 重建作品-作品集关联（仅引用校验通过的，保留 is_cover/sort_order）
	var workSetLinks []*entity2.ReWorkWorkSet
	for _, ws := range snapshot.WorkSets {
		if !ws.WorkSetID.Valid || !existingWorkSetIds[ws.WorkSetID.Int64] {
			continue
		}
		rel := entity2.NewReWorkWorkSet()
		rel.WorkID = sql.NullInt64{Int64: workId, Valid: true}
		rel.WorkSetID = ws.WorkSetID
		rel.IsCover = ws.IsCover
		rel.SortOrder = ws.SortOrder
		workSetLinks = append(workSetLinks, rel)
	}
	if len(workSetLinks) > 0 {
		if err := s.reWorkWorkSetWriter.CreateBatch(ctx, workSetLinks); err != nil {
			return workId, fmt.Errorf("重建作品作品集关联失败: %w", err)
		}
	}

	return workId, nil
}

// resolveExistingRefs 批量查询 snapshot 引用的实体是否存在，返回各类的存在 ID 集合（用于引用校验）
func (s *Service) resolveExistingRefs(ctx context.Context, snapshot *recycleBin.WorkRecycleSnapshot) (map[int64]bool, map[int64]bool, map[int64]bool, map[int64]bool, map[int64]bool, error) {
	localAuthorIds := make(map[int64]bool)
	siteAuthorIds := make(map[int64]bool)
	localTagIds := make(map[int64]bool)
	siteTagIds := make(map[int64]bool)
	workSetIds := make(map[int64]bool)

	// 收集 snapshot 引用的所有实体 ID
	var laIds, saIds, ltIds, stIds, wsIds []int64
	for _, a := range snapshot.Authors {
		if a.LocalAuthorID.Valid && a.LocalAuthorID.Int64 > 0 {
			laIds = append(laIds, a.LocalAuthorID.Int64)
		}
		if a.SiteAuthorID.Valid && a.SiteAuthorID.Int64 > 0 {
			saIds = append(saIds, a.SiteAuthorID.Int64)
		}
	}
	for _, t := range snapshot.Tags {
		if t.LocalTagID.Valid && t.LocalTagID.Int64 > 0 {
			ltIds = append(ltIds, t.LocalTagID.Int64)
		}
		if t.SiteTagID.Valid && t.SiteTagID.Int64 > 0 {
			stIds = append(stIds, t.SiteTagID.Int64)
		}
	}
	for _, ws := range snapshot.WorkSets {
		if ws.WorkSetID.Valid && ws.WorkSetID.Int64 > 0 {
			wsIds = append(wsIds, ws.WorkSetID.Int64)
		}
	}

	// 批量查询存在的实体 ID
	if len(laIds) > 0 {
		entities, err := s.localAuthorBatchReader.ListByIds(ctx, laIds)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("校验本地作者失败: %w", err)
		}
		for _, e := range entities {
			localAuthorIds[e.GetID()] = true
		}
	}
	if len(saIds) > 0 {
		entities, err := s.siteAuthorByIdsReader.ListBySiteAuthorIds(ctx, saIds)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("校验站点作者失败: %w", err)
		}
		for _, e := range entities {
			siteAuthorIds[e.GetID()] = true
		}
	}
	if len(ltIds) > 0 {
		entities, err := s.localTagBatchReader.ListByIds(ctx, ltIds)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("校验本地标签失败: %w", err)
		}
		for _, e := range entities {
			localTagIds[e.GetID()] = true
		}
	}
	if len(stIds) > 0 {
		entities, err := s.siteTagBatchReader.ListBySiteTagIds(ctx, stIds)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("校验站点标签失败: %w", err)
		}
		for _, e := range entities {
			siteTagIds[e.GetID()] = true
		}
	}
	if len(wsIds) > 0 {
		entities, err := s.workSetByIdsReader.ListByIds(ctx, wsIds)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("校验作品集失败: %w", err)
		}
		for _, e := range entities {
			workSetIds[e.GetID()] = true
		}
	}
	return localAuthorIds, siteAuthorIds, localTagIds, siteTagIds, workSetIds, nil
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
func (s *Service) GetFullWorkInfoByIds(ctx context.Context, ids []int64) ([]*sdkdto.WorkFullDTO, error) {
	if len(ids) == 0 {
		return []*sdkdto.WorkFullDTO{}, nil
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
	result := make([]*sdkdto.WorkFullDTO, 0, len(ids))
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
			fullDTO.SiteTags = make([]*sdkdto.SiteTagFullDTO, 0, len(tagIds))
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
			SiteAuthorID:    ptrStrValue(a.SiteAuthorID),
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
func (s *Service) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*sdkdto.RankedLocalAuthor, error) {
	if len(workIds) == 0 {
		return []*sdkdto.RankedLocalAuthor{}, nil
	}
	// 获取作品列表
	works, err := s.repo.ListByIds(ctx, workIds)
	if err != nil {
		return nil, err
	}

	// 收集所有本地作者ID
	authorMap := make(map[int64]*sdkdto.RankedLocalAuthor)
	for _, work := range works {
		if work.LocalAuthorID.Valid && work.LocalAuthorID.Int64 > 0 {
			localAuthorId := work.LocalAuthorID.Int64
			if _, exists := authorMap[localAuthorId]; !exists {
				localAuthor, err := s.localAuthorReader.GetById(ctx, localAuthorId)
				if err == nil && localAuthor != nil {
					authorMap[localAuthorId] = &sdkdto.RankedLocalAuthor{
						Author: *dto2.NewLocalAuthorDTO(localAuthor),
					}
				}
			}
		}
	}

	// 转换为列表
	result := make([]*sdkdto.RankedLocalAuthor, 0, len(authorMap))
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
	LocalAuthor *sdkdto.RankedLocalAuthor `json:"localAuthor,omitempty"`
	SiteAuthor  *sdkdto.RankedSiteAuthor  `json:"siteAuthor,omitempty"`
}

// ErrWorkIdRequired 错误定义
var (
	ErrWorkIdRequired = &pkgerr.BusinessError{Code: 400, Message: "更新作品失败，id不能为空"}
	ErrWorkNotFound   = &pkgerr.BusinessError{Code: 404, Message: "作品不存在"}
)

// AuthorType 常量（与 reWorkTag.TagType 对齐）
const (
	AuthorTypeLocal = 1
	AuthorTypeSite  = 2
)

// SaveWorkInfo 保存作品及全部周边数据，返回作品内部 DB ID
func (s *Service) SaveWorkInfo(ctx context.Context, task *entity2.Task, workResp *sdkdto.WorkResponse) (int64, error) {
	var workId int64
	err := s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		var err error
		workId, err = s.saveWorkInfoInTx(txCtx, task, workResp)
		return err
	})
	return workId, err
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
	// TODO: 当前的全量替换策略会删除该作品的全部关联（包括用户在 UI 中手动添加的本地作者/标签关联），
	//  然后仅用插件返回的数据重建。应在执行替换前提示用户确认是"替换"还是"合并"，
	//  合并模式下保留非插件来源的关联，仅更新插件管理的 Site 类型关联。
	// === Phase 3: 保存 Work + 全量替换关联 ===
	workId, err := s.saveOrUpdateWork(ctx, work)
	if err != nil {
		return 0, fmt.Errorf("保存作品失败: %w", err)
	}

	// 全量替换 work-author 关联（Site + Local）
	if err := s.reWorkAuthorWriter.DeleteByWorkId(ctx, workId); err != nil {
		return 0, fmt.Errorf("删除作品作者关联失败: %w", err)
	}
	authorLinks := buildSiteAuthorLinks(workId, siteAuthorDBIds)
	authorLinks = append(authorLinks, buildLocalAuthorLinks(workId, localAuthorDBIds)...)
	if len(authorLinks) > 0 {
		if err := s.reWorkAuthorWriter.SaveBatch(ctx, authorLinks); err != nil {
			return 0, fmt.Errorf("保存作品作者关联失败: %w", err)
		}
	}

	// 全量替换 work-tag 关联（Site + Local）
	if err := s.reWorkTagWriter.DeleteByWorkId(ctx, workId); err != nil {
		return 0, fmt.Errorf("删除作品标签关联失败: %w", err)
	}
	tagLinks := buildSiteTagLinks(workId, siteTagDBIds)
	tagLinks = append(tagLinks, buildLocalTagLinks(workId, localTagDBIds)...)
	if len(tagLinks) > 0 {
		if err := s.reWorkTagWriter.SaveBatch(ctx, tagLinks); err != nil {
			return 0, fmt.Errorf("保存作品标签关联失败: %w", err)
		}
	}

	// 全量替换 work-workset 关联
	if err := s.reWorkWorkSetWriter.DeleteByWorkId(ctx, workId); err != nil {
		return 0, fmt.Errorf("删除作品作品集关联失败: %w", err)
	}
	if len(workSetDBIds) > 0 {
		links := buildWorkSetLinks(workId, workSetDBIds)
		if err := s.reWorkWorkSetWriter.CreateBatch(ctx, links); err != nil {
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
		siteAuthorIds[i] = d.SiteAuthorID
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
		ids[i] = idMap[d.SiteAuthorID]
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
		siteTagIds[i] = d.SiteTagID
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
		ids[i] = idMap[d.SiteTagID]
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
		siteWorkSetIds[i] = d.SiteWorkSetID
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
		ids[i] = idMap[d.SiteWorkSetID]
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
		if d.ID > 0 {
			idModeIds = append(idModeIds, d.ID)
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
		if d.ID > 0 {
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
		if d.ID > 0 {
			idModeIds = append(idModeIds, d.ID)
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
		if d.ID > 0 {
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
		SiteAuthorID:    sql.NullString{String: d.SiteAuthorID, Valid: true},
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
		SiteTagID:   sql.NullString{String: d.SiteTagID, Valid: true},
		SiteTagName: sql.NullString{String: d.TagName, Valid: true},
		Description: sql.NullString{String: d.Description, Valid: d.Description != ""},
	}
}

func taskWorkSetDTOToEntity(d *sdkdto.TaskWorkSetDTO, siteId int64) *entity2.WorkSet {
	return &entity2.WorkSet{
		BaseEntity:      &model.BaseEntity{},
		SiteID:          sql.NullInt64{Int64: siteId, Valid: true},
		SiteWorkSetID:   sql.NullString{String: d.SiteWorkSetID, Valid: true},
		SiteWorkSetName: sql.NullString{String: d.WorkSetName, Valid: true},
	}
}

// ========== 关联实体构建辅助函数 ==========

func buildSiteAuthorLinks(workId int64, siteAuthorIds []int64) []*entity2.ReWorkAuthor {
	links := make([]*entity2.ReWorkAuthor, 0, len(siteAuthorIds))
	for i, authorId := range siteAuthorIds {
		links = append(links, &entity2.ReWorkAuthor{
			BaseEntity:   &model.BaseEntity{},
			AuthorType:   sql.NullInt64{Int64: AuthorTypeSite, Valid: true},
			WorkID:       sql.NullInt64{Int64: workId, Valid: true},
			SiteAuthorID: sql.NullInt64{Int64: authorId, Valid: true},
			SortOrder:    sql.NullInt64{Int64: int64(i), Valid: true},
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

func buildLocalAuthorLinks(workId int64, localAuthorIds []int64) []*entity2.ReWorkAuthor {
	links := make([]*entity2.ReWorkAuthor, 0, len(localAuthorIds))
	for i, authorId := range localAuthorIds {
		links = append(links, &entity2.ReWorkAuthor{
			BaseEntity:    &model.BaseEntity{},
			AuthorType:    sql.NullInt64{Int64: AuthorTypeLocal, Valid: true},
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
			TagType:    sql.NullInt64{Int64: 1, Valid: true}, // TagTypeLocal
			LocalTagID: sql.NullInt64{Int64: tagId, Valid: true},
		})
	}
	return links
}

func buildWorkSetLinks(workId int64, workSetIds []int64) []*entity2.ReWorkWorkSet {
	links := make([]*entity2.ReWorkWorkSet, 0, len(workSetIds))
	for i, wsId := range workSetIds {
		rel := entity2.NewReWorkWorkSet()
		rel.WorkID = sql.NullInt64{Int64: workId, Valid: true}
		rel.WorkSetID = sql.NullInt64{Int64: wsId, Valid: true}
		rel.SortOrder = sql.NullInt64{Int64: int64(i), Valid: true}
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
		if d.ID <= 0 {
			continue
		}
		if d.AuthorName != nil && *d.AuthorName != "" {
			continue
		}
		if e, ok := entityMap[d.ID]; ok && e.AuthorName.Valid {
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
		if d.ID <= 0 {
			continue
		}
		if d.LocalTagName != nil && *d.LocalTagName != "" {
			continue
		}
		if e, ok := entityMap[d.ID]; ok && e.LocalTagName.Valid {
			d.LocalTagName = &e.LocalTagName.String
		}
	}
}
