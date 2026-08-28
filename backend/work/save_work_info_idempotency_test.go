package work

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/reWorkAuthor"
	"github.com/library-squirrel/backend/reWorkTag"
	"github.com/library-squirrel/backend/shareLock"
	"github.com/library-squirrel/backend/siteAuthor"
	"github.com/library-squirrel/backend/siteTag"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"

	"gorm.io/gorm"
)

// newSaveWorkInfoTestEnv 内存 FK 库 + saveWorkInfoInTx 焦点件（work/siteAuthor/siteTag/reWork 系真实服务）；
// 其余依赖 nil（LocalAuthors/LocalTags/WorkSets 空输入早退不触），txTransactor 复用 delete_purge_test 定义
func newSaveWorkInfoTestEnv(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	svc := NewService(
		NewRepository(db),
		&txTransactor{db: db},
		nil, // LocalTagReader
		nil, // LocalAuthorReader
		nil, // SiteTagReader
		nil, // SiteAuthorReader
		nil, // SiteReader
		nil, // ResourceReader
		reWorkTag.NewService(reWorkTag.NewRepository(db), nil), // ReWorkTagWriter（真实件）
		nil, // ReWorkWorkSetWriter
		nil, // ResourceDeleter
		siteAuthor.NewService(siteAuthor.NewRepository(db), nil, nil, nil, nil), // SiteAuthorWriter（真实件）
		siteTag.NewService(siteTag.NewRepository(db), nil, nil, nil, nil, nil),  // SiteTagWriter（真实件）
		nil, // WorkSetWriter
		reWorkAuthor.NewService(reWorkAuthor.NewRepository(db)), // ReWorkAuthorWriter（真实件）
		nil,                              // LocalTagBatchReader
		nil,                              // SiteTagBatchReader
		nil,                              // SiteBatchReader
		nil,                              // LocalAuthorBatchReader
		nil,                              // SiteAuthorBatchReader
		nil,                              // ResourceBatchReader
		nil,                              // ResourceStoreBatchReader
		nil,                              // StoreBatchReader
		nil,                              // ReWorkTagBatchReader
		nil,                              // LocalTagFindOrCreator
		nil,                              // LocalAuthorFindOrCreator
		nil,                              // StoreDeleter
		nil,                              // RunningTaskStopper
		nil,                              // ResourceStoreHardDeleter
		nil,                              // WorkSetRelationWriter
		nil,                              // CoverReferenceClearer
		shareLock.NewShareLockRegistry(), // WorkLockChecker（真实件：纯内存能力，零外部依赖）
	)
	return svc, db
}

// TestSaveWorkInfoInTxFoldsDuplicateSiteMetadata 插件元数据对同一站点作者/标签产出多条同站点 ID 的 DTO 时
// （localImport 目录两级同名分类的真实形态），SITE 关联落库折叠为一条，而非裸批量 INSERT 撞
// (work_id, site_author_id)/(work_id, site_tag_id) 唯一索引违约致任务 Failed
func TestSaveWorkInfoInTxFoldsDuplicateSiteMetadata(t *testing.T) {
	svc, db := newSaveWorkInfoTestEnv(t)

	site := entity2.NewSite()
	site.SiteName = sql.NullString{String: "dup-meta-test-site", Valid: true}
	if err := db.Create(site).Error; err != nil {
		t.Fatalf("插 site 失败: %v", err)
	}

	task := entity2.NewTask()
	task.SiteID = sql.NullInt64{Int64: site.GetID(), Valid: true}

	siteWorkId := "dup-meta-work-1"
	workResp := &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{
			SiteWorkId:   &siteWorkId,
			SiteWorkName: &siteWorkId,
		},
		// 两条同 SiteAuthorId/同 SiteTagId——upsert 落同一主数据行、关联层拿到重复 DB ID
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{
			{SiteAuthorId: "siteAuthor:dup", AuthorName: "dup作者"},
			{SiteAuthorId: "siteAuthor:dup", AuthorName: "dup作者"},
		},
		SiteTags: []*sdkdto.TaskSiteTagDTO{
			{SiteTagId: "siteTag:dup", TagName: "dup标签"},
			{SiteTagId: "siteTag:dup", TagName: "dup标签"},
		},
	}

	workId, err := svc.saveWorkInfoInTx(context.Background(), task, workResp)
	if err != nil {
		t.Fatalf("saveWorkInfoInTx 失败（重复站点元数据应折叠为单条关联）: %v", err)
	}

	var authorLinks int64
	if err := db.Model(&entity2.ReWorkAuthor{}).Where("work_id = ? AND author_type = ?", workId, constant.SITE).Count(&authorLinks).Error; err != nil {
		t.Fatalf("统计作者关联失败: %v", err)
	}
	if authorLinks != 1 {
		t.Fatalf("SITE 作者关联应恰 1 条，实际 %d", authorLinks)
	}

	var tagLinks int64
	if err := db.Model(&entity2.ReWorkTag{}).Where("work_id = ? AND tag_type = ?", workId, constant.SITE).Count(&tagLinks).Error; err != nil {
		t.Fatalf("统计标签关联失败: %v", err)
	}
	if tagLinks != 1 {
		t.Fatalf("SITE 标签关联应恰 1 条，实际 %d", tagLinks)
	}
}
