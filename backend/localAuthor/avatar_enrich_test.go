package localAuthor

// 头像展示 enrich 锚定：作品作者链（ListByWorkId/ListReWorkAuthor 的 RankedLocalAuthor）按
// 「活行且落盘完成」口径批量补头像路径——软删/未完成的被引用行不产出路径，作者条目降级为
// 占位图语义（nil）。

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/reWorkAuthor"
	"github.com/library-squirrel/backend/siteAuthor"
	"github.com/library-squirrel/backend/work"

	"gorm.io/gorm"
)

// newAvatarEnrichEnv 内存库（外键强制 + 完整迁移）+ localAuthor 服务接好头像 store 行读取
// （真实 persistentStore 服务，软删行经 GORM 软删 scope 排除）
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
	svc := NewService(
		NewRepository(db),
		&txTransactor{db: db},
		siteAuthor.NewRepository(db),
		reWorkAuthorSvc,
		work.NewRepository(db),
	)
	psSvc := persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return "" })
	svc.SetAvatarStoreReader(psSvc)
	return svc, db
}

// plantAvatarStoreRow 落一行头像 store 行并按参设定完成时刻/软删时刻
func plantAvatarStoreRow(t *testing.T, db *gorm.DB, relPath string, completedAt, deletedAt int64) int64 {
	t.Helper()
	row := domain.NewPersistentStore()
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

// plantLocalAuthorWithAvatar 建本地作者行并挂上作品（关联行 LOCAL 轨），返回作者行
func plantLocalAuthorWithAvatar(t *testing.T, db *gorm.DB, workId int64, name string, avatarStoreId int64) *domain.LocalAuthor {
	t.Helper()
	author := plantLocalAuthor(t, db, name, avatarStoreId)
	rel := &domain.ReWorkAuthor{
		BaseEntity:    &model.BaseEntity{},
		AuthorType:    sql.NullInt64{Int64: constant.LOCAL, Valid: true},
		WorkID:        sql.NullInt64{Int64: workId, Valid: true},
		LocalAuthorID: sql.NullInt64{Int64: author.GetID(), Valid: true},
	}
	if err := db.Create(rel).Error; err != nil {
		t.Fatalf("建作品-作者关联失败: %v", err)
	}
	return author
}

// TestListByWorkIdFillsAvatarFilePath 作品作者链批量组装：头像引用指向活行且落盘完成的条目得到
// 头像路径；引用软删行、未完成行与无引用的条目为 nil。ListByWorkId 与 ListReWorkAuthor 共用
// 同一填充路径，本用例同时锚定两条入口
func TestListByWorkIdFillsAvatarFilePath(t *testing.T) {
	svc, db := newAvatarEnrichEnv(t)

	workId := int64(1)
	if err := db.Exec("INSERT INTO work (id, create_time, update_time, deleted_at) VALUES (1, 0, 0, 0)").Error; err != nil {
		t.Fatalf("建作品种子失败: %v", err)
	}

	alivePath := "store/avatar/local/ab/local_1.png"
	aliveStore := plantAvatarStoreRow(t, db, alivePath, 1726800000000, 0)
	deadStore := plantAvatarStoreRow(t, db, "store/avatar/local/cd/local_2.png", 1726800000000, 1726900000000)
	incompleteStore := plantAvatarStoreRow(t, db, "store/avatar/local/ef/local_3.png", 0, 0)

	authorAlive := plantLocalAuthorWithAvatar(t, db, workId, "作者-完整头像", aliveStore)
	authorDead := plantLocalAuthorWithAvatar(t, db, workId, "作者-软删头像", deadStore)
	authorIncomplete := plantLocalAuthorWithAvatar(t, db, workId, "作者-未完成头像", incompleteStore)
	authorNone := plantLocalAuthorWithAvatar(t, db, workId, "作者-无头像", 0)

	ranked, err := svc.ListByWorkId(context.Background(), workId)
	if err != nil {
		t.Fatalf("查询作品本地作者失败: %v", err)
	}
	assertRankedAvatars(t, ranked, map[int64]*string{
		authorAlive.GetID():      &alivePath,
		authorDead.GetID():       nil,
		authorIncomplete.GetID(): nil,
		authorNone.GetID():       nil,
	})

	rankedMap, err := svc.ListReWorkAuthor(context.Background(), []int64{workId})
	if err != nil {
		t.Fatalf("批量查询作品本地作者失败: %v", err)
	}
	assertRankedAvatars(t, rankedMap[workId], map[int64]*string{
		authorAlive.GetID():      &alivePath,
		authorDead.GetID():       nil,
		authorIncomplete.GetID(): nil,
		authorNone.GetID():       nil,
	})
}

// TestQueryFullPageFillsAvatarFilePath 管理页列表/详情两端点的包装 DTO 组装：头像引用指向活行且
// 落盘完成的条目得到头像路径；引用软删行、未完成行与无引用的条目为 nil。QueryFullPage 与
// GetFullById 共用同一批量解析器，本用例锚定两条入口
func TestQueryFullPageFillsAvatarFilePath(t *testing.T) {
	svc, db := newAvatarEnrichEnv(t)

	alivePath := "store/avatar/local/ab/local_1.png"
	aliveStore := plantAvatarStoreRow(t, db, alivePath, 1726800000000, 0)
	deadStore := plantAvatarStoreRow(t, db, "store/avatar/local/cd/local_2.png", 1726800000000, 1726900000000)
	incompleteStore := plantAvatarStoreRow(t, db, "store/avatar/local/ef/local_3.png", 0, 0)

	authorAlive := plantLocalAuthor(t, db, "作者-完整头像", aliveStore)
	authorDead := plantLocalAuthor(t, db, "作者-软删头像", deadStore)
	authorIncomplete := plantLocalAuthor(t, db, "作者-未完成头像", incompleteStore)
	authorNone := plantLocalAuthor(t, db, "作者-无头像", 0)

	expected := map[int64]*string{
		authorAlive.GetID():      &alivePath,
		authorDead.GetID():       nil,
		authorIncomplete.GetID(): nil,
		authorNone.GetID():       nil,
	}

	fullPage, err := svc.QueryFullPage(context.Background(), &model.Page[dto.LocalAuthorFullDTO]{PageNumber: 1, PageSize: 10}, LocalAuthorQueryDTO{})
	if err != nil {
		t.Fatalf("分页查询宿主侧展示 DTO 失败: %v", err)
	}
	if len(fullPage.Data) != len(expected) {
		t.Fatalf("应查出 %d 个作者条目，实际 %d", len(expected), len(fullPage.Data))
	}
	for _, full := range fullPage.Data {
		assertFullAvatar(t, full, expected[full.Author.Id])
	}

	single, err := svc.GetFullById(context.Background(), authorAlive.GetID())
	if err != nil {
		t.Fatalf("按 ID 查询宿主侧展示 DTO 失败: %v", err)
	}
	assertFullAvatar(t, single, expected[authorAlive.GetID()])
}

// plantLocalAuthor 建本地作者行（不挂作品关联），返回作者行
func plantLocalAuthor(t *testing.T, db *gorm.DB, name string, avatarStoreId int64) *domain.LocalAuthor {
	t.Helper()
	author := domain.NewLocalAuthor()
	author.AuthorName = sql.NullString{String: name, Valid: true}
	if avatarStoreId > 0 {
		author.AvatarStoreID = sql.NullInt64{Int64: avatarStoreId, Valid: true}
	}
	if err := db.Create(author).Error; err != nil {
		t.Fatalf("建本地作者 %s 失败: %v", name, err)
	}
	return author
}

// assertFullAvatar 断言包装 DTO 的头像路径填充结果（nil=期望无头像）
func assertFullAvatar(t *testing.T, full *dto.LocalAuthorFullDTO, want *string) {
	t.Helper()
	got := full.AvatarFilePath
	switch {
	case want == nil && got != nil:
		t.Fatalf("作者 %d 头像应为 nil，实际 %v", full.Author.Id, *got)
	case want != nil && (got == nil || *got != *want):
		t.Fatalf("作者 %d 头像应为 %s，实际 %v", full.Author.Id, *want, got)
	}
}

// assertRankedAvatars 按「作者 id → 期望路径（nil=期望无头像）」逐条断言填充结果
func assertRankedAvatars(t *testing.T, ranked []*dto.RankedLocalAuthor, expected map[int64]*string) {
	t.Helper()
	byId := make(map[int64]*dto.RankedLocalAuthor, len(ranked))
	for _, r := range ranked {
		byId[r.Author.Id] = r
	}
	if len(byId) != len(expected) {
		t.Fatalf("应查出 %d 个作者条目，实际 %d", len(expected), len(byId))
	}
	for authorId, want := range expected {
		got := byId[authorId].AvatarFilePath
		switch {
		case want == nil && got != nil:
			t.Fatalf("作者 %d 头像应为 nil，实际 %v", authorId, *got)
		case want != nil && (got == nil || *got != *want):
			t.Fatalf("作者 %d 头像应为 %s，实际 %v", authorId, *want, got)
		}
	}
}
