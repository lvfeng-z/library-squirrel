package siteAuthor

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
)

// TestListFetchTargetsByIdsJoinsSiteKey 拉取目标行反查：JOIN site 逐行反查 site_key（多站点
// 混合集各行携带各自键——含跨站引用作者的消费前提）；站点行缺失的作者行照常返回且 site_key
// 为空（LEFT JOIN 容忍，由调用方按不可路由跳过）；行缺失返回交集不报错
func TestListFetchTargetsByIdsJoinsSiteKey(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	repo := NewRepository(db)

	if err := db.Exec("INSERT INTO site (id, site_key, site_name, create_time, update_time) VALUES (1, 'pixiv', 'pixiv', 0, 0)").Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
	if err := db.Exec("INSERT INTO site (id, site_key, site_name, create_time, update_time) VALUES (2, 'bilibili', 'bilibili', 0, 0)").Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}

	mkAuthor := func(siteId int64, siteAuthorId string) *domain.SiteAuthor {
		row := domain.NewSiteAuthor()
		row.SiteID = sql.NullInt64{Int64: siteId, Valid: true}
		row.SiteAuthorID = sql.NullString{String: siteAuthorId, Valid: true}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("建作者种子失败: %v", err)
		}
		return row
	}
	px := mkAuthor(1, "px-1")
	bl := mkAuthor(2, "bl-1")
	// 站点行缺失的作者行（异常态遗留形态）——FK 强制下不可经正常写入产生，关 PRAGMA 种植
	// （plantDanglingPluginRef 先例）
	if err := db.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
		t.Fatalf("关闭外键失败: %v", err)
	}
	dangling := mkAuthor(999, "dangling-1")
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("恢复外键失败: %v", err)
	}

	targets, err := repo.ListFetchTargetsByIds(context.Background(), []int64{px.GetID(), bl.GetID(), dangling.GetID(), 424242})
	if err != nil {
		t.Fatalf("反查失败: %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("应返回 3 行（缺失 id 不报错返回交集）, 实际 %d", len(targets))
	}
	keyById := make(map[int64]string, len(targets))
	for _, tg := range targets {
		keyById[tg.ID] = tg.SiteKey
	}
	if keyById[px.GetID()] != "pixiv" {
		t.Fatalf("pixiv 作者应解析 site_key=pixiv, 实际 %q", keyById[px.GetID()])
	}
	if keyById[bl.GetID()] != "bilibili" {
		t.Fatalf("bilibili 作者应解析 site_key=bilibili, 实际 %q", keyById[bl.GetID()])
	}
	if keyById[dangling.GetID()] != "" {
		t.Fatalf("站点行缺失的作者 site_key 应为空, 实际 %q", keyById[dangling.GetID()])
	}
}
