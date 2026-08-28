package work

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/shareLock"
	"github.com/library-squirrel/backend/workSet"

	"gorm.io/gorm"
)

// txTransactor 真实事务适配器（事务开启后把 tx 放进 ctx，repo 方法经 DBFromContext 取事务连接——
// 与生产装配 dbTransactorAdapter 同款，验证 DeleteUnscopedByIds 的事务内通路）
type txTransactor struct {
	db *gorm.DB
}

func (t *txTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return database.WithTransactionContext(ctx, t.db, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, database.TxKey, tx)
		return fn(txCtx)
	})
}

// purgeResourceDeleter 预置作品的 resource 集合（ListByWorkId 返回预置；DeleteByWorkId 记录调用
// 并真实删行——外键强制下 purge 链须先清 resource 行才可删 work）
type purgeResourceDeleter struct {
	db        *gorm.DB
	resources []*domain.Resource
	deleted   []int64
}

func (f *purgeResourceDeleter) ListByWorkId(ctx context.Context, workId int64) ([]*domain.Resource, error) {
	result := make([]*domain.Resource, 0, len(f.resources))
	for _, r := range f.resources {
		if r.WorkID == workId {
			result = append(result, r)
		}
	}
	return result, nil
}

func (f *purgeResourceDeleter) DeleteByWorkId(ctx context.Context, workId int64) error {
	f.deleted = append(f.deleted, workId)
	// 事务内调用：经 ctx 取事务连接（单连接下用根连接会死锁）
	return database.DBFromContext(ctx, f.db).WithContext(ctx).Exec("DELETE FROM resource WHERE work_id = ?", workId).Error
}

// purgeRsBatchReader 预置 resource→resource_store 映射（ListStoresByResourceIds 返回预置）
type purgeRsBatchReader struct {
	rsMap map[int64][]*domain.ResourceStore
}

func (f *purgeRsBatchReader) ListStoresByResourceIds(ctx context.Context, resourceIds []int64) (map[int64][]*domain.ResourceStore, error) {
	result := make(map[int64][]*domain.ResourceStore, len(resourceIds))
	for _, rid := range resourceIds {
		if list, ok := f.rsMap[rid]; ok {
			result[rid] = list
		}
	}
	return result, nil
}

// purgeReWorkTagWriter / purgeReWorkAuthorWriter / purgeReWorkWorkSetWriter 关联删除的空操作替身
// （purge 链焦点在 store 行物理消亡，关联删除不在焦点；接口方法集各自对齐）
type purgeReWorkTagWriter struct{}

func (purgeReWorkTagWriter) DeleteByWorkId(ctx context.Context, workId int64) error     { return nil }
func (purgeReWorkTagWriter) DeleteSiteByWorkId(ctx context.Context, workId int64) error { return nil }
func (purgeReWorkTagWriter) SaveBatchOnConflict(ctx context.Context, rels []*domain.ReWorkTag) error {
	return nil
}

type purgeReWorkAuthorWriter struct{}

func (purgeReWorkAuthorWriter) DeleteByWorkId(ctx context.Context, workId int64) error { return nil }
func (purgeReWorkAuthorWriter) DeleteSiteByWorkId(ctx context.Context, workId int64) error {
	return nil
}
func (purgeReWorkAuthorWriter) SaveBatchOnConflict(ctx context.Context, rels []*domain.ReWorkAuthor) error {
	return nil
}

type purgeReWorkWorkSetWriter struct{}

func (purgeReWorkWorkSetWriter) DeleteByWorkId(ctx context.Context, workId int64) error { return nil }
func (purgeReWorkWorkSetWriter) CreateBatch(ctx context.Context, rels []*domain.ReWorkWorkSet) error {
	return nil
}
func (purgeReWorkWorkSetWriter) SaveBatchOnConflict(ctx context.Context, rels []*domain.ReWorkWorkSet) error {
	return nil
}
func (purgeReWorkWorkSetWriter) UpdateSiteSortOrders(ctx context.Context, workSetId int64, sortOrders map[int64]int) error {
	return nil
}
func (purgeReWorkWorkSetWriter) MaxSortOrderByWorkSetId(ctx context.Context, workSetId int64) (int64, error) {
	return 0, nil
}

// purgeRsHardDeleter resource_store 级联删除替身（记录调用并真实删行——外键强制下 purge 链
// 须先摘关联才可删 persistent_store 行；purge 焦点在 persistent_store 行）
type purgeRsHardDeleter struct {
	db                 *gorm.DB
	deletedResourceIds []int64
}

func (p *purgeRsHardDeleter) DeleteByResourceIds(ctx context.Context, resourceIds []int64) error {
	p.deletedResourceIds = append(p.deletedResourceIds, resourceIds...)
	if len(resourceIds) == 0 {
		return nil
	}
	// 事务内调用：经 ctx 取事务连接（单连接下用根连接会死锁）
	return database.DBFromContext(ctx, p.db).WithContext(ctx).Exec("DELETE FROM resource_store WHERE resource_id IN ?", resourceIds).Error
}

// newPurgeTestEnv 内存库（work/resource/resource_store/persistent_store 四表）+ 真实
// work/persistentStore 服务（purge 链焦点件），其余依赖空操作替身
func newPurgeTestEnv(t *testing.T) (*Service, *persistentStore.Service, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	// 作品行种子（resource.work_id 外键防线，fixture 统一 WorkID=1）
	if err := db.Exec("INSERT INTO work (id, create_time, update_time, deleted_at) VALUES (1, 0, 0, 0)").Error; err != nil {
		t.Fatalf("建作品种子失败: %v", err)
	}
	if err := db.AutoMigrate(&domain.Work{}, &domain.Resource{}, &domain.ResourceStore{}, &domain.PersistentStore{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}
	workRepo := NewRepository(db)
	psSvc := persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return t.TempDir() })
	svc := NewService(
		workRepo,
		&txTransactor{db: db},
		nil,                              // LocalTagReader
		nil,                              // LocalAuthorReader
		nil,                              // SiteTagReader
		nil,                              // SiteAuthorReader
		nil,                              // SiteReader
		nil,                              // ResourceReader
		purgeReWorkTagWriter{},           // ReWorkTagWriter
		purgeReWorkWorkSetWriter{},       // ReWorkWorkSetWriter
		nil,                              // ResourceDeleter（测试内经 fixture 覆盖私有字段注入预置）
		nil,                              // SiteAuthorWriter
		nil,                              // SiteTagWriter
		nil,                              // WorkSetWriter
		purgeReWorkAuthorWriter{},        // ReWorkAuthorWriter
		nil,                              // LocalTagBatchReader
		nil,                              // SiteTagBatchReader
		nil,                              // SiteBatchReader
		nil,                              // LocalAuthorBatchReader
		nil,                              // SiteAuthorBatchReader
		nil,                              // ResourceBatchReader
		nil,                              // ResourceStoreBatchReader（同 ResourceDeleter，测试内注入）
		nil,                              // StoreBatchReader
		nil,                              // ReWorkTagBatchReader
		nil,                              // LocalTagFindOrCreator
		nil,                              // LocalAuthorFindOrCreator
		psSvc,                            // StoreDeleter（真实件：purge 链 store 行物理删除的提供方）
		nil,                              // RunningTaskStopper
		&purgeRsHardDeleter{db: db},      // ResourceStoreHardDeleter
		nil,                              // WorkSetRelationWriter
		workSet.NewRepository(db),        // CoverReferenceClearer（真实件：purge 链首步清封面引用）
		shareLock.NewShareLockRegistry(), // WorkLockChecker（真实件：纯内存能力，零外部依赖）
	)
	return svc, psSvc, db
}

// 插软删 work 行 + 软删 store 行（挂载链经 fixture 预置返回，不落 resource/resource_store 表）
func insertDeletedWorkWithStore(t *testing.T, db *gorm.DB, psSvc *persistentStore.Service) (workId, storeId int64) {
	t.Helper()
	work := domain.NewWork()
	if err := db.Create(work).Error; err != nil {
		t.Fatalf("插 work 失败: %v", err)
	}
	workId = work.GetID()
	if err := db.Exec("UPDATE work SET deleted_at = ? WHERE id = ?", 1000, workId).Error; err != nil {
		t.Fatalf("软删 work 失败: %v", err)
	}
	store := domain.NewPersistentStore()
	store.FilePath.Valid = true
	store.FilePath.String = "store/resource/作者/purge_test.mp4"
	store.CompletedAt = 1
	if err := db.Create(store).Error; err != nil {
		t.Fatalf("插 store 失败: %v", err)
	}
	storeId = store.GetID()
	if err := persistentStore.NewRepository(db).SoftDeleteWithBackup(context.Background(), storeId, 0); err != nil {
		t.Fatalf("软删 store 失败: %v", err)
	}
	return workId, storeId
}

// countStoreRows 含已删行统计 persistent_store 行数（物理消亡断言用，Unscoped）
func countStoreRows(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Unscoped().Model(&domain.PersistentStore{}).Count(&n).Error; err != nil {
		t.Fatalf("统计 store 行失败: %v", err)
	}
	return n
}

// TestDeleteWorkAndSurroundingDataClearsCoverReferences 彻底删除链首步清封面引用（外键删除防线
// 前置义务）：活集与软删集的 cover_work_id 指向被 purge 作品时，引用随链清空，work 物理删除
// 不被外键拦停（软删集行不分行态，同样须清）
func TestDeleteWorkAndSurroundingDataClearsCoverReferences(t *testing.T) {
	svc, psSvc, db := newPurgeTestEnv(t)
	workId, _ := insertDeletedWorkWithStore(t, db, psSvc)

	mkSet := func(softDeleted bool) int64 {
		t.Helper()
		ws := domain.NewWorkSet()
		ws.CoverWorkID.Valid = true
		ws.CoverWorkID.Int64 = workId
		if err := db.Create(ws).Error; err != nil {
			t.Fatalf("插 work_set 失败: %v", err)
		}
		if softDeleted {
			if err := db.Exec("UPDATE work_set SET deleted_at = 2000 WHERE id = ?", ws.GetID()).Error; err != nil {
				t.Fatalf("软删 work_set 失败: %v", err)
			}
		}
		return ws.GetID()
	}
	liveSet, deadSet := mkSet(false), mkSet(true)

	svc.resourceDeleter = &purgeResourceDeleter{db: db}
	svc.resourceStoreBatchReader = &purgeRsBatchReader{}
	if err := svc.DeleteWorkAndSurroundingData(context.Background(), workId); err != nil {
		t.Fatalf("彻底删除失败（封面引用应已前置清空）: %v", err)
	}
	for _, id := range []int64{liveSet, deadSet} {
		var cover sql.NullInt64
		if err := db.Raw("SELECT cover_work_id FROM work_set WHERE id = ?", id).Scan(&cover).Error; err != nil {
			t.Fatalf("查封面引用失败: %v", err)
		}
		if cover.Valid {
			t.Fatalf("作品集 %d 的封面引用应已清空，实际 %d", id, cover.Int64)
		}
	}
}

// TestHardDeleteSkipsSoftDeletedRow 对照锚定：HardDelete 对已软删行静默跳过（内部 GetById 受软删
// scope 保护，NotFound 提前返回）——这正是历史 purge 链残留产道的根因，故彻底删除链改走
// DeleteUnscopedByIds 直删（见下一用例）
func TestHardDeleteSkipsSoftDeletedRow(t *testing.T) {
	_, psSvc, db := newPurgeTestEnv(t)
	_, storeId := insertDeletedWorkWithStore(t, db, psSvc)
	if _, err := psSvc.HardDelete(context.Background(), storeId, false); err != nil {
		t.Fatalf("HardDelete 不应报错: %v", err)
	}
	if n := countStoreRows(t, db); n != 1 {
		t.Fatalf("对照锚定失效：HardDelete 对软删行应静默跳过（行保留），实际行数 %d", n)
	}
}

// TestDeleteWorkAndSurroundingDataPurgesStoreRows 彻底删除链物理删 store 行：
// 已软删 store 行随 work 级联删除物理消亡（回归锚定：曾因事务后走 HardDelete 被软删 scope
// 静默跳过，每次 Purge 遗留离链孤儿行）
func TestDeleteWorkAndSurroundingDataPurgesStoreRows(t *testing.T) {
	svc, psSvc, db := newPurgeTestEnv(t)
	workId, storeId := insertDeletedWorkWithStore(t, db, psSvc)

	// 预置挂载链（resource + resource_store 映射经 fixture 返回，模拟级联删除的输入）
	resource := domain.NewResource()
	resource.WorkID = workId
	resource.ResourceType = "video"
	if err := db.Create(resource).Error; err != nil {
		t.Fatalf("插 resource 失败: %v", err)
	}
	rs := domain.NewResourceStore()
	rs.ResourceID = resource.GetID()
	rs.StoreType = domain.StoreTypeVideoMain
	rs.Generation = domain.GenerationDownloaded
	rs.StoreID = storeId
	rs.StoreSeq = 0
	if err := db.Create(rs).Error; err != nil {
		t.Fatalf("插 resource_store 失败: %v", err)
	}
	svc.resourceDeleter = &purgeResourceDeleter{db: db, resources: []*domain.Resource{resource}}
	svc.resourceStoreBatchReader = &purgeRsBatchReader{rsMap: map[int64][]*domain.ResourceStore{resource.GetID(): {rs}}}

	if err := svc.DeleteWorkAndSurroundingData(context.Background(), workId); err != nil {
		t.Fatalf("彻底删除失败: %v", err)
	}
	if n := countStoreRows(t, db); n != 0 {
		t.Fatalf("purge 后 persistent_store 应物理消亡（含已删行），实际残留 %d 行", n)
	}
	var workCount int64
	if err := db.Unscoped().Model(&domain.Work{}).Where("id = ?", workId).Count(&workCount).Error; err != nil {
		t.Fatalf("统计 work 行失败: %v", err)
	}
	if workCount != 0 {
		t.Fatalf("purge 后 work 行应物理消亡，实际残留 %d 行", workCount)
	}
}

// TestDeleteUnscopedByIdsRemovesSoftDeletedRows 批量物理删行对软删行生效（单条 DELETE IN 直删通路）
func TestDeleteUnscopedByIdsRemovesSoftDeletedRows(t *testing.T) {
	_, psSvc, db := newPurgeTestEnv(t)
	_, storeId := insertDeletedWorkWithStore(t, db, psSvc)
	if err := psSvc.DeleteUnscopedByIds(context.Background(), []int64{storeId}); err != nil {
		t.Fatalf("批量物理删失败: %v", err)
	}
	if n := countStoreRows(t, db); n != 0 {
		t.Fatalf("软删行应被物理删除，实际残留 %d 行", n)
	}
}
