package site_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/site"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"

	"gorm.io/gorm"
)

// newSyncTestEnv 外键强制内存库 + 经生产构造函数装配的 site 服务（注册表投影同步测试环境）
func newSyncTestEnv(t *testing.T) (*site.Service, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	return site.NewService(site.NewRepository(db)), db
}

// siteRowCount 站点表总行数（测试断言用）
func siteRowCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT COUNT(*) FROM site").Scan(&n).Error; err != nil {
		t.Fatalf("统计站点行失败: %v", err)
	}
	return n
}

// TestSyncFromRegistryInsertsRegistryRows 全新库首次投影：注册表全量条目各建一行，总数与
// 注册表一致；站名/主页取注册表权威值；无主页条目（local 虚拟站点）主页落 NULL。
func TestSyncFromRegistryInsertsRegistryRows(t *testing.T) {
	svc, db := newSyncTestEnv(t)

	if err := svc.SyncFromRegistry(context.Background()); err != nil {
		t.Fatalf("首次投影同步失败: %v", err)
	}

	if got, want := siteRowCount(t, db), int64(len(identity.All())); got != want {
		t.Fatalf("投影后站点行数=%d，期望与注册表一致 %d", got, want)
	}
	for _, entry := range identity.All() {
		var row struct {
			SiteName sql.NullString
			Homepage sql.NullString
		}
		if err := db.Raw("SELECT site_name, homepage FROM site WHERE site_key = ?", entry.Key).Scan(&row).Error; err != nil {
			t.Fatalf("查询站点行 %s 失败: %v", entry.Key, err)
		}
		if !row.SiteName.Valid || row.SiteName.String != entry.Name {
			t.Fatalf("站点 %s 的站名应取注册表权威值 %q，实际 %+v", entry.Key, entry.Name, row.SiteName)
		}
		if entry.Homepage == "" {
			if row.Homepage.Valid {
				t.Fatalf("无主页条目 %s 的主页应落 NULL，实际 %+v", entry.Key, row.Homepage)
			}
		} else if !row.Homepage.Valid || row.Homepage.String != entry.Homepage {
			t.Fatalf("站点 %s 的主页应取注册表权威值 %q，实际 %+v", entry.Key, entry.Homepage, row.Homepage)
		}
	}
}

// TestSyncFromRegistryIdempotent 幂等：连续两次投影同步后行数与注册表一致，
// 既有行不被重复创建。
func TestSyncFromRegistryIdempotent(t *testing.T) {
	svc, db := newSyncTestEnv(t)
	ctx := context.Background()

	if err := svc.SyncFromRegistry(ctx); err != nil {
		t.Fatalf("首次投影同步失败: %v", err)
	}
	if err := svc.SyncFromRegistry(ctx); err != nil {
		t.Fatalf("重复投影同步应成功: %v", err)
	}

	if got, want := siteRowCount(t, db), int64(len(identity.All())); got != want {
		t.Fatalf("重复同步后站点行数=%d，期望 %d（不重复建行）", got, want)
	}
}

// TestSyncFromRegistryInsertOnly insert-only：既有行一律不动——用户改过展示名/清空主页的行
// 原样保留（编辑持久），非注册键的库内残余行同样保留（注册表只增不改，同步无删除分支），
// 仅缺失键新建。
func TestSyncFromRegistryInsertOnly(t *testing.T) {
	svc, db := newSyncTestEnv(t)

	// 预置 pixiv 行为用户编辑态（展示名改、主页清空）；预置一条非注册键行（库内异常残留）
	if err := db.Exec("INSERT INTO site (site_key, site_name, homepage, create_time, update_time) VALUES (?, '我的P站', NULL, 0, 0)", identity.Pixiv.Key).Error; err != nil {
		t.Fatalf("预置 pixiv 行失败: %v", err)
	}
	if err := db.Exec("INSERT INTO site (site_key, site_name, create_time, update_time) VALUES ('ghost-key', '幽灵站', 0, 0)").Error; err != nil {
		t.Fatalf("预置非注册键行失败: %v", err)
	}

	if err := svc.SyncFromRegistry(context.Background()); err != nil {
		t.Fatalf("投影同步失败: %v", err)
	}

	// pixiv 行保持用户编辑态，未被注册表权威值回写
	if got := queryStringForSite(t, db, "SELECT site_name FROM site WHERE site_key = ?", identity.Pixiv.Key); got != "我的P站" {
		t.Fatalf("既有行的展示名不应被投影回写，实际 %q", got)
	}
	var pixivHomepage sql.NullString
	if err := db.Raw("SELECT homepage FROM site WHERE site_key = ?", identity.Pixiv.Key).Scan(&pixivHomepage).Error; err != nil {
		t.Fatalf("查询 pixiv 主页失败: %v", err)
	}
	if pixivHomepage.Valid {
		t.Fatalf("既有行被清空的主页不应被投影回写，实际 %+v", pixivHomepage)
	}
	// 非注册键行保留（无删除分支）
	if got := queryStringForSite(t, db, "SELECT site_name FROM site WHERE site_key = ?", "ghost-key"); got != "幽灵站" {
		t.Fatalf("非注册键行应原样保留，实际 site_name=%q", got)
	}
	// 缺失键（bilibili/local）补建：总数 = 预置 2 行 + 注册表其余条目
	if got, want := siteRowCount(t, db), int64(2+len(identity.All())-1); got != want {
		t.Fatalf("投影后站点行数=%d，期望 %d（既有 2 行保留 + 缺失键补建）", got, want)
	}
}

// queryStringForSite 单值字符串查询（SQL NULL 落空串，测试断言用）
func queryStringForSite(t *testing.T, db *gorm.DB, query string, args ...any) string {
	t.Helper()
	var ns sql.NullString
	if err := db.Raw(query, args...).Scan(&ns).Error; err != nil {
		t.Fatalf("查询失败 %s: %v", query, err)
	}
	if !ns.Valid {
		return ""
	}
	return ns.String
}
