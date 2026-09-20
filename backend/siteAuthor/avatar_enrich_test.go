package siteAuthor

// 头像展示 enrich 锚定：管理页关联 DTO（SiteAuthorLocalRelateDTO）与批量路径解析
// （AvatarFilePathsByAuthorIds）按「活行且落盘完成」口径产出头像路径——软删/未完成的
// 被引用行不产出路径，作者行降级为占位图语义（nil）。

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/localAuthor"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/reWorkAuthor"

	"gorm.io/gorm"
)

// newAvatarEnrichEnv 内存库（外键强制 + 完整迁移）+ siteAuthor 服务接好头像 store 行读取
// （真实 persistentStore 服务，软删行经 GORM 软删 scope 排除）。作者名种子会触发同名本地作者
// 检查，localAuthor 操作方以真实服务参与
func newAvatarEnrichEnv(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	reWorkAuthorSvc := reWorkAuthor.NewService(reWorkAuthor.NewRepository(db), nil, nil)
	localSvc := localAuthor.NewService(localAuthor.NewRepository(db), nil, NewRepository(db), reWorkAuthorSvc, nil)
	svc := NewService(
		NewRepository(db),
		localSvc, // LocalAuthorOperator（同名本地作者检查）
		nil,      // SiteOperator（种子无 site 行即不触及）
		&txTransactor{db: db},
		reWorkAuthorSvc,
	)
	psSvc := persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return "" })
	svc.SetAvatarStoreReader(psSvc)
	return svc, db
}

// plantAvatarStoreRow 落一行头像 store 行并按参设定完成时刻/软删时刻（create_time/update_time
// 由 GORM 自动填充，completed_at/deleted_at 经显式 UPDATE 控制零值语义）
func plantAvatarStoreRow(t *testing.T, db *gorm.DB, relPath string, completedAt, deletedAt int64) int64 {
	t.Helper()
	row := entity.NewPersistentStore()
	row.FilePath = sql.NullString{String: relPath, Valid: true}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("建头像 store 行失败: %v", err)
	}
	if completedAt > 0 {
		if err := db.Exec("UPDATE persistent_store SET completed_at = ? WHERE id = ?", completedAt, row.GetID()).Error; err != nil {
			t.Fatalf("标记 store 行完成失败: %v", err)
		}
	}
	if deletedAt > 0 {
		if err := db.Exec("UPDATE persistent_store SET deleted_at = ? WHERE id = ?", deletedAt, row.GetID()).Error; err != nil {
			t.Fatalf("软删 store 行失败: %v", err)
		}
	}
	return row.GetID()
}

func plantSiteAuthorWithAvatar(t *testing.T, db *gorm.DB, name string, avatarStoreId int64) *entity.SiteAuthor {
	t.Helper()
	author := entity.NewSiteAuthor()
	author.AuthorName = sql.NullString{String: name, Valid: true}
	if avatarStoreId > 0 {
		author.AvatarStoreID = sql.NullInt64{Int64: avatarStoreId, Valid: true}
	}
	if err := db.Create(author).Error; err != nil {
		t.Fatalf("建站点作者 %s 失败: %v", name, err)
	}
	return author
}

// TestQueryLocalRelateDTOPageFillsAvatarFilePath 管理页关联 DTO 批量组装：头像引用指向活行且
// 落盘完成的作者行得到头像路径；引用软删行、未完成行与无引用的作者行为 nil
func TestQueryLocalRelateDTOPageFillsAvatarFilePath(t *testing.T) {
	svc, db := newAvatarEnrichEnv(t)

	alivePath := "store/avatar/site/ab/pixiv_100.jpg"
	aliveStore := plantAvatarStoreRow(t, db, alivePath, 1726800000000, 0)
	deadStore := plantAvatarStoreRow(t, db, "store/avatar/site/cd/pixiv_200.jpg", 1726800000000, 1726900000000)
	incompleteStore := plantAvatarStoreRow(t, db, "store/avatar/site/ef/pixiv_300.jpg", 0, 0)

	authorAlive := plantSiteAuthorWithAvatar(t, db, "作者-完整头像", aliveStore)
	authorDead := plantSiteAuthorWithAvatar(t, db, "作者-软删头像", deadStore)
	authorIncomplete := plantSiteAuthorWithAvatar(t, db, "作者-未完成头像", incompleteStore)
	authorNone := plantSiteAuthorWithAvatar(t, db, "作者-无头像", 0)

	page, err := svc.QueryLocalRelateDTOPage(context.Background(),
		&model.Page[dto.SiteAuthorLocalRelateDTO]{PageNumber: 1, PageSize: 10}, SiteAuthorQueryDTO{})
	if err != nil {
		t.Fatalf("查询管理页关联 DTO 失败: %v", err)
	}

	byId := make(map[int64]*dto.SiteAuthorLocalRelateDTO, len(page.Data))
	for _, row := range page.Data {
		byId[row.SiteAuthor.ID] = row
	}
	if len(byId) != 4 {
		t.Fatalf("应查出 4 个站点作者，实际 %d", len(byId))
	}
	if got := byId[authorAlive.GetID()].AvatarFilePath; got == nil || *got != alivePath {
		t.Fatalf("完整头像作者应得到路径 %s，实际 %v", alivePath, got)
	}
	if got := byId[authorDead.GetID()].AvatarFilePath; got != nil {
		t.Fatalf("软删头像引用应产出 nil，实际 %v", *got)
	}
	if got := byId[authorIncomplete.GetID()].AvatarFilePath; got != nil {
		t.Fatalf("未完成头像引用应产出 nil，实际 %v", *got)
	}
	if got := byId[authorNone.GetID()].AvatarFilePath; got != nil {
		t.Fatalf("无头像引用应产出 nil，实际 %v", *got)
	}
}

// TestAvatarFilePathsByAuthorIds 批量路径解析：一次调用解析全部作者 id，仅可展示头像的 id 进入
// 返回 map；解析器未注入（装配缺失）时静默返回空，不阻断查询链
func TestAvatarFilePathsByAuthorIds(t *testing.T) {
	svc, db := newAvatarEnrichEnv(t)

	alivePath := "store/avatar/site/ab/pixiv_111.jpg"
	aliveStore := plantAvatarStoreRow(t, db, alivePath, 1726800000000, 0)
	deadStore := plantAvatarStoreRow(t, db, "store/avatar/site/cd/pixiv_222.jpg", 1726800000000, 1726900000000)

	authorAlive := plantSiteAuthorWithAvatar(t, db, "批量-完整", aliveStore)
	authorDead := plantSiteAuthorWithAvatar(t, db, "批量-软删", deadStore)
	authorNone := plantSiteAuthorWithAvatar(t, db, "批量-无头像", 0)

	paths, err := svc.AvatarFilePathsByAuthorIds(context.Background(),
		[]int64{authorAlive.GetID(), authorDead.GetID(), authorNone.GetID(), authorAlive.GetID()})
	if err != nil {
		t.Fatalf("批量解析头像路径失败: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("仅完整头像作者应进入返回 map，实际 %d 条: %v", len(paths), paths)
	}
	if got, ok := paths[authorAlive.GetID()]; !ok || got == nil || *got != alivePath {
		t.Fatalf("完整头像作者应解析出 %s，实际 %v", alivePath, got)
	}

	bareSvc := NewService(NewRepository(db), nil, nil, &txTransactor{db: db}, nil)
	paths, err = bareSvc.AvatarFilePathsByAuthorIds(context.Background(), []int64{authorAlive.GetID()})
	if err != nil {
		t.Fatalf("未注入读取器时应静默成功，实际错误: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("未注入读取器应返回空 map，实际 %v", paths)
	}
}
