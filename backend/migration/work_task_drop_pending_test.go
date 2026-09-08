package migration

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestWorkTaskPendingColumnDroppedOnLegacyDB 旧形态库（work_task 携 pending_resource_id 列
// 且外键子句在册）跑迁移管线后：列删除、行数据保真、其余外键保留、二次运行幂等。
// 旧形态以 RENAME 规范化后的双引号 DDL 表达（历次外键表重建舞步的落库形态）
func TestWorkTaskPendingColumnDroppedOnLegacyDB(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	// 旧形态两表（先于全部迁移存在；父表 task 同步预建，供行数据过悬空清理）
	if err := db.Exec(`CREATE TABLE "task" (
		"id" integer PRIMARY KEY AUTOINCREMENT,
		"create_time" integer,
		"update_time" integer,
		"has_child" numeric,
		"pid" integer,
		"task_name" text,
		"status" integer,
		"error_message" text,
		"task_type" text,
		FOREIGN KEY ("pid") REFERENCES "task" ("id") ON DELETE NO ACTION ON UPDATE NO ACTION
	)`).Error; err != nil {
		t.Fatalf("预建旧形态 task 表失败: %v", err)
	}
	if err := db.Exec(`CREATE TABLE "work_task" (
		"id" integer PRIMARY KEY AUTOINCREMENT,
		"create_time" integer,
		"update_time" integer,
		"site_id" integer,
		"site_work_id" text,
		"url" text,
		"pending_resource_id" integer,
		"continuable" numeric,
		"plugin_public_id" text,
		"plugin_extension_id" text,
		"plugin_data" text,
		"store_roles" text,
		"involved_roles" text,
		"resource_type" text,
		"include_work_info" numeric,
		FOREIGN KEY ("id") REFERENCES "task" ("id") ON DELETE NO ACTION ON UPDATE NO ACTION,
		FOREIGN KEY ("site_id") REFERENCES "site" ("id") ON DELETE NO ACTION ON UPDATE NO ACTION,
		FOREIGN KEY ("pending_resource_id") REFERENCES "resource" ("id") ON DELETE NO ACTION ON UPDATE NO ACTION
	)`).Error; err != nil {
		t.Fatalf("预建旧形态 work_task 表失败: %v", err)
	}
	if err := db.Exec(`INSERT INTO task (id, create_time, update_time, status) VALUES (101, 1, 1, 0)`).Error; err != nil {
		t.Fatalf("预插 task 行失败: %v", err)
	}
	if err := db.Exec(`INSERT INTO work_task (id, create_time, update_time, site_work_id, url, pending_resource_id) VALUES (101, 1, 1, 'w-1', 'http://x/1', 55)`).Error; err != nil {
		t.Fatalf("预插 work_task 行失败: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("旧形态库迁移失败: %v", err)
	}

	// 列已删除
	var pendingCol int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('work_task') WHERE name = 'pending_resource_id'").Scan(&pendingCol).Error; err != nil {
		t.Fatalf("读取 work_task 列清单失败: %v", err)
	}
	if pendingCol != 0 {
		t.Fatal("pending_resource_id 列应已删除")
	}

	// 行数据保真（共享主键行未被重建/清理误伤）
	var siteWorkId string
	var url string
	if err := db.Raw("SELECT site_work_id, url FROM work_task WHERE id = 101").Row().Scan(&siteWorkId, &url); err != nil {
		t.Fatalf("读取迁移后 work_task 行失败: %v", err)
	}
	if siteWorkId != "w-1" || url != "http://x/1" {
		t.Fatalf("迁移后行数据漂移: siteWorkId=%q url=%q", siteWorkId, url)
	}

	// 其余外键保留、pending 外键不在册
	var refs []struct {
		Table string
		From  string
	}
	if err := db.Raw("PRAGMA foreign_key_list('work_task')").Scan(&refs).Error; err != nil {
		t.Fatalf("读取 work_task 外键清单失败: %v", err)
	}
	present := make(map[string]bool, len(refs))
	for _, r := range refs {
		present[r.From+"→"+r.Table] = true
	}
	if !present["id→task"] || !present["site_id→site"] {
		t.Fatalf("共享主键与站点外键应保留, 实际 %v", present)
	}
	if present["pending_resource_id→resource"] {
		t.Fatal("pending_resource_id 外键应随列一并移除")
	}

	// 二次运行幂等（列缺失即跳过删列重建）
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("二次迁移应幂等, 实际失败: %v", err)
	}
	var rowCount int
	if err := db.Raw("SELECT COUNT(*) FROM work_task WHERE id = 101").Scan(&rowCount).Error; err != nil {
		t.Fatalf("二次迁移后读取 work_task 行失败: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("二次迁移后行应原样保留, 实际 %d 行", rowCount)
	}
}
