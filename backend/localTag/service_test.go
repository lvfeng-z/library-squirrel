package localTag

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/reWorkTag"
	"github.com/library-squirrel/backend/siteTag"

	"gorm.io/gorm"
)

// txTransactor 真实事务适配器（事务开启后把 tx 放进 ctx，repo 方法经 DBFromContext 取事务连接——
// 与生产装配 dbTransactorAdapter 同款）
type txTransactor struct {
	db *gorm.DB
}

func (t *txTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return database.WithTransactionContext(ctx, t.db, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, database.TxKey, tx)
		return fn(txCtx)
	})
}

// newDeleteTestEnv 内存库（外键强制 + 完整迁移）+ 真实 localTag 服务与 siteTag/reWorkTag 仓储
// （删除编排的全部参与件真实落库，断言直查表）
func newDeleteTestEnv(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	svc := NewService(
		NewRepository(db),
		&txTransactor{db: db},
		siteTag.NewRepository(db),
		reWorkTag.NewRepository(db),
	)
	return svc, db
}

// insertLocalTag 插入本地标签（baseId 传 nil 为根标签），返回行 ID
func insertLocalTag(t *testing.T, db *gorm.DB, name string, baseId *int64) int64 {
	t.Helper()
	tag := domain.NewLocalTag()
	tag.LocalTagName = sql.NullString{String: name, Valid: true}
	if baseId != nil {
		tag.BaseLocalTagID = sql.NullInt64{Int64: *baseId, Valid: true}
	}
	if err := db.Create(tag).Error; err != nil {
		t.Fatalf("插 local_tag 失败: %v", err)
	}
	return tag.GetID()
}

// TestDeleteLocalTagCleansReferences 删除本地标签的编排锚定：作品关联归零、站点标签绑定置 NULL
// （行保留）、标签行消亡。外键强制库下「删除成功」本身即先清子后删父顺序的证明。
func TestDeleteLocalTagCleansReferences(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	svc, db := newDeleteTestEnv(t)

	// 外键父行种子：work（re_work_tag.work_id）+ site（site_tag.site_id）
	if err := db.Exec("INSERT INTO work (id, create_time, update_time, deleted_at) VALUES (1, 0, 0, 0)").Error; err != nil {
		t.Fatalf("建作品种子失败: %v", err)
	}
	site := domain.NewSite()
	site.SiteName = sql.NullString{String: "测试站点", Valid: true}
	if err := db.Create(site).Error; err != nil {
		t.Fatalf("插 site 失败: %v", err)
	}

	tagId := insertLocalTag(t, db, "待删标签", nil)

	siteTagRow := domain.NewSiteTag()
	siteTagRow.SiteID = sql.NullInt64{Int64: site.GetID(), Valid: true}
	siteTagRow.SiteTagID = sql.NullString{String: "site-tag-1", Valid: true}
	siteTagRow.SiteTagName = sql.NullString{String: "绑定该标签的站点标签", Valid: true}
	siteTagRow.LocalTagID = sql.NullInt64{Int64: tagId, Valid: true}
	if err := db.Create(siteTagRow).Error; err != nil {
		t.Fatalf("插 site_tag 失败: %v", err)
	}

	rel := domain.NewReWorkTag()
	rel.WorkID = sql.NullInt64{Int64: 1, Valid: true}
	rel.TagType = sql.NullInt64{Int64: constant.LOCAL, Valid: true}
	rel.LocalTagID = sql.NullInt64{Int64: tagId, Valid: true}
	if err := db.Create(rel).Error; err != nil {
		t.Fatalf("插 re_work_tag 失败: %v", err)
	}

	if err := svc.Delete(context.Background(), tagId); err != nil {
		t.Fatalf("删除本地标签失败: %v", err)
	}

	var relCount int64
	if err := db.Model(&domain.ReWorkTag{}).Where("local_tag_id = ?", tagId).Count(&relCount).Error; err != nil {
		t.Fatalf("统计 re_work_tag 失败: %v", err)
	}
	if relCount != 0 {
		t.Fatalf("删除后 re_work_tag 关联应归零，实际 %d", relCount)
	}

	var binding sql.NullInt64
	if err := db.Raw("SELECT local_tag_id FROM site_tag WHERE id = ?", siteTagRow.GetID()).Scan(&binding).Error; err != nil {
		t.Fatalf("查 site_tag 绑定失败: %v", err)
	}
	if binding.Valid {
		t.Fatalf("删除后 site_tag 行应保留且绑定置 NULL，实际绑定 %d", binding.Int64)
	}

	var tagCount int64
	if err := db.Model(&domain.LocalTag{}).Where("id = ?", tagId).Count(&tagCount).Error; err != nil {
		t.Fatalf("统计 local_tag 失败: %v", err)
	}
	if tagCount != 0 {
		t.Fatalf("删除后 local_tag 行应消亡，实际 %d", tagCount)
	}
}

// TestDeleteLocalTagReparentsChildren 子标签树指针锚定：删中间层 → 子上提挂祖父（孙变子）；
// 删根级 → 子成为根（NULL）。
func TestDeleteLocalTagReparentsChildren(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	svc, db := newDeleteTestEnv(t)

	grandfatherId := insertLocalTag(t, db, "祖父", nil)
	fatherId := insertLocalTag(t, db, "父", &grandfatherId)
	childId := insertLocalTag(t, db, "子", &fatherId)

	if err := svc.Delete(context.Background(), fatherId); err != nil {
		t.Fatalf("删除中间层标签失败: %v", err)
	}
	var childBase sql.NullInt64
	if err := db.Raw("SELECT base_local_tag_id FROM local_tag WHERE id = ?", childId).Scan(&childBase).Error; err != nil {
		t.Fatalf("查子标签父引用失败: %v", err)
	}
	if !childBase.Valid || childBase.Int64 != grandfatherId {
		t.Fatalf("删父后子标签应上提挂祖父 %d，实际 %+v", grandfatherId, childBase)
	}

	if err := svc.Delete(context.Background(), grandfatherId); err != nil {
		t.Fatalf("删除根级标签失败: %v", err)
	}
	if err := db.Raw("SELECT base_local_tag_id FROM local_tag WHERE id = ?", childId).Scan(&childBase).Error; err != nil {
		t.Fatalf("查子标签父引用失败: %v", err)
	}
	if childBase.Valid {
		t.Fatalf("删根级后子标签应成为根（NULL），实际 %+v", childBase)
	}
}
