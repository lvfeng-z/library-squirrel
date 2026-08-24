package migration

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// openFKTestDB 外键测试库：内存 SQLite + 强制执行 + 完整迁移（含 FK 重建）
func openFKTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := gorm.Open(sqlite.Open(":memory:?_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

// TestRenameCreateTableBothQuoteStyles 表名替换兼容两种引号风格：GORM 建表的反引号、
// 经 RENAME 后 SQLite 规范化存储的双引号（二次重建的表 DDL 为后者）
func TestRenameCreateTableBothQuoteStyles(t *testing.T) {
	backtick := "CREATE TABLE `resource` (`id` integer PRIMARY KEY AUTOINCREMENT)"
	if got := renameCreateTable(backtick, "resource", "resource_fk_rebuild"); !strings.Contains(got, "`resource_fk_rebuild`") || strings.Contains(got, "CREATE TABLE `resource`") {
		t.Fatalf("反引号风格改名失败: %s", got)
	}
	doubleQuoted := "CREATE TABLE \"resource\" (\"id\" integer PRIMARY KEY AUTOINCREMENT)"
	if got := renameCreateTable(doubleQuoted, "resource", "resource_fk_rebuild"); !strings.Contains(got, "\"resource_fk_rebuild\"") || strings.Contains(got, "CREATE TABLE \"resource\"") {
		t.Fatalf("双引号风格改名失败: %s", got)
	}
}

// TestInjectForeignKeysSkipsExisting 已在册的 FK 子句不重复注入（二次重建的表 DDL
// 含此前批次子句，重复注入产生冗余约束）
func TestInjectForeignKeysSkipsExisting(t *testing.T) {
	ddl := "CREATE TABLE `resource` (`id` integer, `work_id` integer, FOREIGN KEY (`work_id`) REFERENCES `work` (`id`))"
	out := injectForeignKeys(ddl, []fkSpec{{Column: "work_id", Parent: "work"}, {Column: "task_id", Parent: "task"}})
	norm := strings.NewReplacer("`", "", "\"", "").Replace(out)
	if n := strings.Count(norm, "FOREIGN KEY (work_id) REFERENCES work"); n != 1 {
		t.Fatalf("已存在的 work_id 外键应保持 1 份，实际 %d 份: %s", n, out)
	}
	if n := strings.Count(norm, "FOREIGN KEY (task_id) REFERENCES task"); n != 1 {
		t.Fatalf("缺失的 task_id 外键应注入 1 份，实际 %d 份: %s", n, out)
	}
}

// TestForeignKeyEnforcedOnConnection DSN 参数生效验证：连接上 foreign_keys 处于开启态
func TestForeignKeyEnforcedOnConnection(t *testing.T) {
	db := openFKTestDB(t)
	var on int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&on).Error; err != nil {
		t.Fatalf("读取 foreign_keys 状态失败: %v", err)
	}
	if on != 1 {
		t.Fatalf("foreign_keys 应为 1，实际 %d", on)
	}
}

// TestForeignKeysDeclared 声明面断言：登记表期望的（引用列, 被引用表）对外键全部在册
func TestForeignKeysDeclared(t *testing.T) {
	db := openFKTestDB(t)
	for _, spec := range fkBatches {
		done, err := fkDeclared(db, spec)
		if err != nil {
			t.Fatalf("读取 %s 外键清单失败: %v", spec.Table, err)
		}
		if !done {
			t.Fatalf("表 %s 的外键声明不完整", spec.Table)
		}
	}
}

// TestForeignKeyWriteGuard 悬空写入防线：指向不存在作品的关联行被拒绝，合法引用放行
func TestForeignKeyWriteGuard(t *testing.T) {
	db := openFKTestDB(t)
	w := entity.NewWork()
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("建作品行失败: %v", err)
	}

	bad := entity.NewReWorkTag()
	bad.WorkID = sql.NullInt64{Int64: 999999, Valid: true}
	if err := db.Create(bad).Error; err == nil {
		t.Fatal("指向不存在作品的关联行应被外键拒绝")
	}

	good := entity.NewReWorkTag()
	good.WorkID = sql.NullInt64{Int64: w.GetID(), Valid: true}
	if err := db.Create(good).Error; err != nil {
		t.Fatalf("合法关联行不应被拒: %v", err)
	}
}

// TestForeignKeyDeleteGuard 删除防线：被关联行引用的父行删除被拒绝，摘除关联后放行
// （后者即删除编排链的正常路径：先清子引用再删父行）
func TestForeignKeyDeleteGuard(t *testing.T) {
	db := openFKTestDB(t)
	w := entity.NewWork()
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("建作品行失败: %v", err)
	}
	rt := entity.NewReWorkTag()
	rt.WorkID = sql.NullInt64{Int64: w.GetID(), Valid: true}
	if err := db.Create(rt).Error; err != nil {
		t.Fatalf("建关联行失败: %v", err)
	}

	if err := db.Exec("DELETE FROM work WHERE id = ?", w.GetID()).Error; err == nil {
		t.Fatal("被关联行引用的作品删除应被外键拒绝")
	}

	if err := db.Exec("DELETE FROM re_work_tag WHERE id = ?", rt.GetID()).Error; err != nil {
		t.Fatalf("摘除关联行失败: %v", err)
	}
	if err := db.Exec("DELETE FROM work WHERE id = ?", w.GetID()).Error; err != nil {
		t.Fatalf("摘除关联后删除不应被拒: %v", err)
	}
}

// TestForeignKeyCheckClean 迁移后全库无外键违约存量（含悬空清理效果）
func TestForeignKeyCheckClean(t *testing.T) {
	db := openFKTestDB(t)
	var rows []struct {
		Table string
		RowId int64
	}
	if err := db.Raw("PRAGMA foreign_key_check").Scan(&rows).Error; err != nil {
		t.Fatalf("foreign_key_check 失败: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("foreign_key_check 应为空，实际 %d 行: %+v", len(rows), rows)
	}
}

// TestForeignKeysIdempotentSecondRun 二次迁移幂等：FK 已在册走跳过路径，数据不损
func TestForeignKeysIdempotentSecondRun(t *testing.T) {
	db := openFKTestDB(t)
	w := entity.NewWork()
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("建作品行失败: %v", err)
	}
	rt := entity.NewReWorkTag()
	rt.WorkID = sql.NullInt64{Int64: w.GetID(), Valid: true}
	if err := db.Create(rt).Error; err != nil {
		t.Fatalf("建关联行失败: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("二次迁移失败: %v", err)
	}

	var n int64
	if err := db.Raw("SELECT COUNT(*) FROM re_work_tag").Scan(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("二次迁移后关联行应保留，实际 %d 行", n)
	}
	done, err := fkDeclared(db, fkBatches[0])
	if err != nil || !done {
		t.Fatalf("二次迁移后外键应仍在册: done=%v err=%v", done, err)
	}
}

// TestRebuildPreservesIndexes 重建保留索引：唯一索引随舞步复原（以 re_work_tag 的
// 组合唯一索引行数与唯一性为证）
func TestRebuildPreservesIndexes(t *testing.T) {
	db := openFKTestDB(t)
	var idxCount int64
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND tbl_name = 're_work_tag' AND sql IS NOT NULL").Scan(&idxCount).Error; err != nil {
		t.Fatalf("计数索引失败: %v", err)
	}
	if idxCount < 2 {
		t.Fatalf("re_work_tag 应保有至少 2 个显式索引（两组组合唯一索引），实际 %d", idxCount)
	}

	// 唯一性仍生效：同 (work_id, local_tag_id) 二次插入被唯一索引拒绝
	w := entity.NewWork()
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("建作品行失败: %v", err)
	}
	lt := entity.NewLocalTag()
	if err := db.Create(lt).Error; err != nil {
		t.Fatalf("建本地标签行失败: %v", err)
	}
	first := entity.NewReWorkTag()
	first.WorkID = sql.NullInt64{Int64: w.GetID(), Valid: true}
	first.LocalTagID = sql.NullInt64{Int64: lt.GetID(), Valid: true}
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("建关联行失败: %v", err)
	}
	dup := entity.NewReWorkTag()
	dup.WorkID = sql.NullInt64{Int64: w.GetID(), Valid: true}
	dup.LocalTagID = sql.NullInt64{Int64: lt.GetID(), Valid: true}
	if err := db.Create(dup).Error; err == nil {
		t.Fatal("同键关联行应被唯一索引拒绝（索引随重建复原的锚定）")
	}
}
