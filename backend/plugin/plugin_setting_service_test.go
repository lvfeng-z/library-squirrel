package plugin

import (
	"context"
	"fmt"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/util"
	pluginsdktransport "github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// newSettingTestService 设置服务测试装配：内存库真实插件仓储 + 内存存储仓储（与生产装配同构，
// 见 app.go 的 NewRepository → NewPluginSettingService 链），根目录与
// plantPluginWithManifestOnDisk 的落盘位置一致（应用根目录）
func newSettingTestService(t *testing.T) (*PluginSettingService, *Service) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	if err := db.AutoMigrate(&entity.Plugin{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}
	repo := NewRepository(db)
	svc := NewService(repo, nil)
	return NewPluginSettingService(repo, NewPluginStorageService(newMockStorageRepo()), util.RootPath()), svc
}

// TestGetSettingsReadsRootLevelDeclarations 根级 settings 段解析与设置服务读取：
// 声明逐字段从 plugin.json 根级可达，未存储项回落声明默认值，保存后存储值覆盖默认值
func TestGetSettingsReadsRootLevelDeclarations(t *testing.T) {
	settingSvc, svc := newSettingTestService(t)
	const publicId = "com.settings.root"
	manifest := `{"id":"` + publicId + `","name":"设置插件","version":"1.0.0",` +
		fmt.Sprintf(`"contractVersion":%d,`, pluginsdktransport.ContractVersion) +
		`"settings":[{"key":"apiToken","type":"string","title":"访问令牌","default":"anon","encrypted":true},` +
		`{"key":"pageSize","type":"integer","title":"分页大小","default":"20"}],` +
		`"extensions":{` + frontendExtensionsField + `}}`
	plantPluginWithManifestOnDisk(t, svc, publicId, manifest)

	ctx := context.Background()
	items, err := settingSvc.GetSettings(ctx, publicId)
	if err != nil {
		t.Fatalf("读取设置项失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("设置项条数 = %d, 期望 2（根级 settings 全量解析）", len(items))
	}
	if items[0].Key != "apiToken" || !items[0].Encrypted || items[0].Value != "anon" {
		t.Errorf("apiToken 项 = %+v, 期望 key=apiToken encrypted=true 默认值 anon", items[0])
	}
	if items[1].Key != "pageSize" || items[1].Value != "20" {
		t.Errorf("pageSize 项 = %+v, 期望 key=pageSize 默认值 20", items[1])
	}

	if err := settingSvc.SaveSetting(ctx, publicId, "pageSize", "50"); err != nil {
		t.Fatalf("保存设置项失败: %v", err)
	}
	items, err = settingSvc.GetSettings(ctx, publicId)
	if err != nil {
		t.Fatalf("回读设置项失败: %v", err)
	}
	if items[1].Value != "50" {
		t.Errorf("已保存设置项值 = %q, 期望 50（存储值覆盖声明默认值）", items[1].Value)
	}
}

// TestGetSettingsWithoutExtensionsSection 清单无 extensions 段时根级 settings 照常解析
// （用户设置项声明不依赖 extensions 能力包段在场）
func TestGetSettingsWithoutExtensionsSection(t *testing.T) {
	settingSvc, svc := newSettingTestService(t)
	const publicId = "com.settings.noext"
	manifest := `{"id":"` + publicId + `","name":"纯设置插件","version":"1.0.0",` +
		fmt.Sprintf(`"contractVersion":%d,`, pluginsdktransport.ContractVersion) +
		`"settings":[{"key":"locale","type":"string","title":"语言","default":"zh"}]}`
	plantPluginWithManifestOnDisk(t, svc, publicId, manifest)

	items, err := settingSvc.GetSettings(context.Background(), publicId)
	if err != nil {
		t.Fatalf("读取设置项失败: %v", err)
	}
	if len(items) != 1 || items[0].Key != "locale" || items[0].Value != "zh" {
		t.Fatalf("无 extensions 段清单的设置项 = %+v, 期望单条 locale=zh", items)
	}
}
