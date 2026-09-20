package reWorkAuthor

// 头像展示 enrich 接线锚定：本模块的 Ranked* 产出自关联行 JOIN 作者表而来，头像路径经
// siteAuthor/localAuthor 侧解析器（SetAvatarPathResolvers 注入）后置批量补齐——未注入时
// 跳过填充不阻断查询链。

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/localAuthor"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/siteAuthor"

	"gorm.io/gorm"
)

// newAvatarEnrichEnv 内存库（外键强制 + 完整迁移）+ 两侧作者服务接好头像 store 行读取 +
// 本服务接好两侧路径解析器（生产装配同款接线）
func newAvatarEnrichEnv(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	psSvc := persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return "" })
	svc := NewService(NewRepository(db), nil, nil)
	localSvc := localAuthor.NewService(localAuthor.NewRepository(db), nil, siteAuthor.NewRepository(db), svc, nil)
	siteSvc := siteAuthor.NewService(siteAuthor.NewRepository(db), nil, nil, nil, svc)
	localSvc.SetAvatarStoreReader(psSvc)
	siteSvc.SetAvatarStoreReader(psSvc)
	svc.SetAvatarPathResolvers(siteSvc, localSvc)
	return svc, db
}

// TestListLocalAuthorsByWorkIdFillsAvatarFilePath 作品详情链（单作品作者列表）的头像填充：
// 引用活行且落盘完成的条目得到路径，引用软删行/未完成行/无引用的条目为 nil
func TestListLocalAuthorsByWorkIdFillsAvatarFilePath(t *testing.T) {
	svc, db := newAvatarEnrichEnv(t)

	workId := int64(1)
	if err := db.Exec("INSERT INTO work (id, create_time, update_time, deleted_at) VALUES (1, 0, 0, 0)").Error; err != nil {
		t.Fatalf("建作品种子失败: %v", err)
	}

	alivePath := "store/avatar/local/ab/local_1.png"
	aliveStore := plantAvatarStoreRow(t, db, alivePath, 1726800000000, 0)
	deadStore := plantAvatarStoreRow(t, db, "store/avatar/local/cd/local_2.png", 1726800000000, 1726900000000)
	incompleteStore := plantAvatarStoreRow(t, db, "store/avatar/local/ef/local_3.png", 0, 0)

	authorAlive := plantLocalAuthorOnWork(t, db, workId, "作者-完整头像", aliveStore)
	authorDead := plantLocalAuthorOnWork(t, db, workId, "作者-软删头像", deadStore)
	authorIncomplete := plantLocalAuthorOnWork(t, db, workId, "作者-未完成头像", incompleteStore)
	authorNone := plantLocalAuthorOnWork(t, db, workId, "作者-无头像", 0)

	ranked, err := svc.ListLocalAuthorsByWorkId(context.Background(), workId)
	if err != nil {
		t.Fatalf("查询作品本地作者失败: %v", err)
	}
	avatarById := make(map[int64]*string, len(ranked))
	for _, r := range ranked {
		avatarById[r.Author.Id] = r.AvatarFilePath
	}
	if len(avatarById) != 4 {
		t.Fatalf("应查出 4 个作者条目，实际 %d", len(avatarById))
	}
	if got := avatarById[authorAlive]; got == nil || *got != alivePath {
		t.Fatalf("完整头像作者应得到路径 %s，实际 %v", alivePath, got)
	}
	for _, id := range []int64{authorDead, authorIncomplete, authorNone} {
		if got := avatarById[id]; got != nil {
			t.Fatalf("作者 %d 头像应为 nil，实际 %v", id, *got)
		}
	}
}

// TestListAuthorsByWorkIdSkipsEnrichWithoutResolver 解析器未注入（装配缺失）时查询链不受影响
func TestListAuthorsByWorkIdSkipsEnrichWithoutResolver(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	svc := NewService(NewRepository(db), nil, nil)

	workId := int64(1)
	if err := db.Exec("INSERT INTO work (id, create_time, update_time, deleted_at) VALUES (1, 0, 0, 0)").Error; err != nil {
		t.Fatalf("建作品种子失败: %v", err)
	}
	plantLocalAuthorOnWork(t, db, workId, "作者-未接线", 0)

	ranked, err := svc.ListLocalAuthorsByWorkId(context.Background(), workId)
	if err != nil {
		t.Fatalf("未注入解析器时应正常查询，实际错误: %v", err)
	}
	if len(ranked) != 1 {
		t.Fatalf("应查出 1 个作者条目，实际 %d", len(ranked))
	}
}

// plantAvatarStoreRow 落一行头像 store 行并按参设定完成时刻/软删时刻
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

// plantLocalAuthorOnWork 建本地作者行并挂上作品（关联行 LOCAL 轨），返回作者行 DB id
func plantLocalAuthorOnWork(t *testing.T, db *gorm.DB, workId int64, name string, avatarStoreId int64) int64 {
	t.Helper()
	author := entity.NewLocalAuthor()
	author.AuthorName = sql.NullString{String: name, Valid: true}
	if avatarStoreId > 0 {
		author.AvatarStoreID = sql.NullInt64{Int64: avatarStoreId, Valid: true}
	}
	if err := db.Create(author).Error; err != nil {
		t.Fatalf("建本地作者 %s 失败: %v", name, err)
	}
	rel := &entity.ReWorkAuthor{
		BaseEntity:    &model.BaseEntity{},
		AuthorType:    sql.NullInt64{Int64: constant.LOCAL, Valid: true},
		WorkID:        sql.NullInt64{Int64: workId, Valid: true},
		LocalAuthorID: sql.NullInt64{Int64: author.GetID(), Valid: true},
	}
	if err := db.Create(rel).Error; err != nil {
		t.Fatalf("建作品-作者关联失败: %v", err)
	}
	return author.GetID()
}
