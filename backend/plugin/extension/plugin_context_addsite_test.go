package extension

import (
	"context"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/site"
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
	"gorm.io/gorm"
)

// newAddSiteContext 经生产构造函数组装 PluginContext，站点查询/保存接真实 site 服务（OpenTestDB）。
// 返回 site 服务（按键查询断言用）与底层 DB（行计数断言用）。
func newAddSiteContext(t *testing.T) (*site.Service, *gorm.DB, pluginsdkdto.PluginContext) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	siteSvc := site.NewService(site.NewRepository(db), nil, nil, nil, nil, nil, nil)
	pc := NewPluginContext(PluginContextDeps{
		PluginInfo: &PluginInfo{ID: 1, PublicID: "pub-test", Name: "测试插件"},
		SiteSave:   siteSvc,
		SiteQuery:  siteSvc,
	})
	return siteSvc, db, pc
}

// TestAddSiteUnregisteredKeyRejected 站点键不在 identity 注册表（含空键、笔误键）→ 报错拒绝，
// 报错文案即注册渠道指引（向 SDK identity 包提 PR 注册）。
func TestAddSiteUnregisteredKeyRejected(t *testing.T) {
	_, db, pc := newAddSiteContext(t)

	cases := []struct {
		name string
		dto  *pluginsdkdto.SiteDTO
	}{
		{"空键", &pluginsdkdto.SiteDTO{}},
		{"未注册键", &pluginsdkdto.SiteDTO{SiteKey: "s000000000000"}},
	}
	for _, c := range cases {
		err := pc.AddSite([]*pluginsdkdto.SiteDTO{c.dto})
		if err == nil {
			t.Fatalf("%s：未注册站点键应报错拒绝", c.name)
		}
		for _, want := range []string{"identity", "PR"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s：报错文案应含注册渠道指引（%q），得到: %v", c.name, want, err)
			}
		}
	}

	// 未注册键不落任何站点行
	var count int64
	if err := db.Model(&entity.Site{}).Count(&count).Error; err != nil {
		t.Fatalf("统计站点行失败: %v", err)
	}
	if count != 0 {
		t.Errorf("未注册键不应落站点行，得到 %d 行", count)
	}
}

// TestAddSiteByKeyDedupAndCanonicalName 已注册键：按键查重（同键再次注册跳过，不报错不重复建行）；
// 新建行的名称/主页取注册表权威值，插件自报名称与主页不落库；本地虚拟站点无主页落 NULL。
func TestAddSiteByKeyDedupAndCanonicalName(t *testing.T) {
	siteSvc, _, pc := newAddSiteContext(t)
	ctx := context.Background()

	// 插件自报的名称/主页与注册表权威值不同（权威性由注册表保证）
	misleadingName := "我的皮西维"
	misleadingHomepage := "https://fake.example.com"
	if err := pc.AddSite([]*pluginsdkdto.SiteDTO{
		{SiteKey: identity.Pixiv.Key, SiteName: &misleadingName, Homepage: &misleadingHomepage},
	}); err != nil {
		t.Fatalf("已注册键 AddSite 应成功: %v", err)
	}

	created, err := siteSvc.GetByKey(ctx, identity.Pixiv.Key)
	if err != nil || created == nil {
		t.Fatalf("按键查不到新建站点行: %v", err)
	}
	if !created.SiteName.Valid || created.SiteName.String != identity.Pixiv.Name {
		t.Errorf("站点名应取注册表权威值 %q，得到 %+v", identity.Pixiv.Name, created.SiteName)
	}
	if !created.Homepage.Valid || created.Homepage.String != identity.Pixiv.Homepage {
		t.Errorf("主页应取注册表权威值 %q，得到 %+v", identity.Pixiv.Homepage, created.Homepage)
	}

	// 同键重复注册：跳过，不报错、不重复建行
	if err := pc.AddSite([]*pluginsdkdto.SiteDTO{
		{SiteKey: identity.Pixiv.Key, SiteName: &misleadingName},
	}); err != nil {
		t.Fatalf("同键重复 AddSite 应跳过而非报错: %v", err)
	}
	again, _ := siteSvc.GetByKey(ctx, identity.Pixiv.Key)
	if again == nil || again.GetID() != created.GetID() {
		t.Fatalf("同键重复注册不应新建行（应维持原行 %d）", created.GetID())
	}

	// 本地虚拟站点：无主页落 NULL，名称取注册表权威值
	if err := pc.AddSite([]*pluginsdkdto.SiteDTO{{SiteKey: identity.Local.Key}}); err != nil {
		t.Fatalf("注册 local 虚拟站点应成功: %v", err)
	}
	local, err := siteSvc.GetByKey(ctx, identity.Local.Key)
	if err != nil || local == nil {
		t.Fatalf("按键查不到 local 站点行: %v", err)
	}
	if !local.SiteName.Valid || local.SiteName.String != identity.Local.Name {
		t.Errorf("local 站点名应取注册表权威值 %q，得到 %+v", identity.Local.Name, local.SiteName)
	}
	if local.Homepage.Valid {
		t.Errorf("local 虚拟站点无主页，Homepage 应为 NULL，得到 %+v", local.Homepage)
	}
}
