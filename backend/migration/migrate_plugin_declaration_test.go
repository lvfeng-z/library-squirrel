package migration

// 插件表声明面残留列测试：capabilities 与 resource_types 两列不在实体字段集内
// （插件声明的唯一来源是 plugin.json，不入库）。存量库带这两列启动时，迁移须照常通过、
// 存量行须经实体照常读写——AutoMigrate 只加列不删列，残留列不参与实体读写。

import (
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
)

// TestPluginLegacyDeclarationColumnsUpgrade 模拟带声明面两列的存量库启动：
// 补回两列并写入声明 JSON → 重跑迁移 → 存量行经实体读回、再落库均不受影响
func TestPluginLegacyDeclarationColumnsUpgrade(t *testing.T) {
	db, err := OpenTestDB()
	if err != nil {
		t.Skipf("测试库不可用: %v", err)
	}

	// 存量库形态：两列在场（升级前数据库带这两列）
	for _, stmt := range []string{
		`ALTER TABLE plugin ADD COLUMN capabilities TEXT`,
		`ALTER TABLE plugin ADD COLUMN resource_types TEXT`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("模拟旧库形态失败(%s): %v", stmt, err)
		}
	}

	// 存量行：走实体写入，两列值以原生 SQL 补齐（实体字段集内已无这两列）
	legacy := entity.NewPlugin()
	legacy.PublicID = sql.NullString{String: "com.example.legacy", Valid: true}
	legacy.Name = sql.NullString{String: "存量插件", Valid: true}
	legacy.Version = sql.NullString{String: "1.0.0", Valid: true}
	legacy.EntryPath = sql.NullString{String: "plugin/com.example.legacy/1.0.0/plugin.exe", Valid: true}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatalf("插存量行失败: %v", err)
	}
	if err := db.Exec(`UPDATE plugin SET capabilities = ?, resource_types = ? WHERE id = ?`,
		`["siteAuthorFetch"]`, `[{"type":"com.example.x"}]`, legacy.ID).Error; err != nil {
		t.Fatalf("回填存量声明列失败: %v", err)
	}

	// 升级启动
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("存量库迁移失败: %v", err)
	}

	// 存量行经实体读回：身份与业务字段完好（两列不在实体字段集内，不参与读写）
	var got entity.Plugin
	if err := db.First(&got, legacy.ID).Error; err != nil {
		t.Fatalf("迁移后读存量行失败: %v", err)
	}
	if got.PublicID.String != "com.example.legacy" || got.Version.String != "1.0.0" ||
		got.EntryPath.String != "plugin/com.example.legacy/1.0.0/plugin.exe" {
		t.Fatalf("存量行字段不符: publicId=%q version=%q entryPath=%q",
			got.PublicID.String, got.Version.String, got.EntryPath.String)
	}

	// 迁移后写回（Save 全字段覆盖）：不含两列的写入在带残留列的库上照常落库
	got.Name = sql.NullString{String: "存量插件(改)", Valid: true}
	if err := db.Save(&got).Error; err != nil {
		t.Fatalf("迁移后写存量行失败: %v", err)
	}
	var name sql.NullString
	if err := db.Raw(`SELECT name FROM plugin WHERE id = ?`, legacy.ID).Scan(&name).Error; err != nil {
		t.Fatalf("回读存量行 name 失败: %v", err)
	}
	if name.String != "存量插件(改)" {
		t.Fatalf("存量行 name = %q, 期望 存量插件(改)", name.String)
	}
}
