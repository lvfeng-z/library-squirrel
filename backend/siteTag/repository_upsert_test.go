package siteTag

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// newUpsertTestRepo 建内存库（全量迁移终态，含复合唯一索引）并返回仓储；种站点行
// （site_tag.site_id 外键防线，site_key NOT NULL 取注册键）
func newUpsertTestRepo(t *testing.T) *SiteTagRepository {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	if err := db.Exec("INSERT OR IGNORE INTO site (id, site_key, create_time, update_time) VALUES (1, ?, 0, 0)", identity.Local.Key).Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
	return NewRepository(db)
}

// TestUpsertConflictColumnSet 键冲突时列集语义：插件权威列（site_tag_name/description/namespace）
// 按 excluded（待插入行）刷新——namespace 插入期与冲突期同权，插件不再声明 namespace（空声明）
// 落 NULL 同为权威；用户策展列（local_tag_id 桥接/base_site_tag_id/last_use）不在更新集，保持既有值。
// BatchUpsert 与 Upsert 共用同一冲突更新列集，两法一并锚定
func TestUpsertConflictColumnSet(t *testing.T) {
	repo := newUpsertTestRepo(t)
	ctx := context.Background()

	// 首插：带 namespace 与用户态
	first := domain.NewSiteTag()
	first.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	first.SiteTagID = sql.NullString{String: "st-1", Valid: true}
	first.SiteTagName = sql.NullString{String: "名v1", Valid: true}
	first.Namespace = sql.NullString{String: "character", Valid: true}
	first.LocalTagID = sql.NullInt64{Int64: 777, Valid: true}
	first.BaseSiteTagID = sql.NullString{String: "base-1", Valid: true}
	first.LastUse = sql.NullInt64{Int64: 111222333, Valid: true}
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("首插失败: %v", err)
	}

	// 同键重声明（BatchUpsert）：名与 namespace 均变化
	second := domain.NewSiteTag()
	second.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	second.SiteTagID = sql.NullString{String: "st-1", Valid: true}
	second.SiteTagName = sql.NullString{String: "名v2", Valid: true}
	second.Namespace = sql.NullString{String: "female", Valid: true}
	if err := repo.BatchUpsert(ctx, []*domain.SiteTag{second}); err != nil {
		t.Fatalf("冲突 BatchUpsert 失败: %v", err)
	}

	var got domain.SiteTag
	if err := repo.GORM().Where("site_tag_id = ?", "st-1").First(&got).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.SiteTagName.String != "名v2" {
		t.Fatalf("站点侧名应刷新，实际 %q", got.SiteTagName.String)
	}
	if !got.Namespace.Valid || got.Namespace.String != "female" {
		t.Fatalf("namespace 属插件权威字段，冲突期应与插入期同权刷新，实际 Valid=%v value=%q", got.Namespace.Valid, got.Namespace.String)
	}
	if !got.LocalTagID.Valid || got.LocalTagID.Int64 != 777 {
		t.Fatalf("local_tag_id 不在冲突更新集，实际 Valid=%v value=%d", got.LocalTagID.Valid, got.LocalTagID.Int64)
	}
	if !got.BaseSiteTagID.Valid || got.BaseSiteTagID.String != "base-1" {
		t.Fatalf("base_site_tag_id 不在冲突更新集，实际 Valid=%v value=%q", got.BaseSiteTagID.Valid, got.BaseSiteTagID.String)
	}
	if !got.LastUse.Valid || got.LastUse.Int64 != 111222333 {
		t.Fatalf("last_use 不在冲突更新集，实际 Valid=%v value=%d", got.LastUse.Valid, got.LastUse.Int64)
	}

	// 再声明（Upsert）：不再声明 namespace（空声明）→ NULL 同为插件权威，覆盖既有值
	third := domain.NewSiteTag()
	third.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	third.SiteTagID = sql.NullString{String: "st-1", Valid: true}
	third.SiteTagName = sql.NullString{String: "名v3", Valid: true}
	if err := repo.Upsert(ctx, third); err != nil {
		t.Fatalf("冲突 Upsert 失败: %v", err)
	}

	var got2 domain.SiteTag
	if err := repo.GORM().Where("site_tag_id = ?", "st-1").First(&got2).Error; err != nil {
		t.Fatalf("二次回查失败: %v", err)
	}
	if got2.Namespace.Valid {
		t.Fatalf("空声明应把 namespace 刷新为 NULL，实际 value=%q", got2.Namespace.String)
	}
	if !got2.LocalTagID.Valid || got2.LocalTagID.Int64 != 777 || !got2.LastUse.Valid || got2.LastUse.Int64 != 111222333 {
		t.Fatalf("用户策展列应保持，实际 local_tag_id=%v/%d last_use=%v/%d",
			got2.LocalTagID.Valid, got2.LocalTagID.Int64, got2.LastUse.Valid, got2.LastUse.Int64)
	}
}
