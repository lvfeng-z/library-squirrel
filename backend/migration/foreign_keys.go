package migration

import (
	"fmt"
	"strings"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fkSpec 单条外键声明：引用列 → 被引用表（被引用列恒为父表主键 id）
type fkSpec struct {
	Column string
	Parent string
}

// fkTable 一张表的外键声明集合
type fkTable struct {
	Table string
	FKs   []fkSpec
}

// fkBatches 外键声明登记面（仅 NO ACTION：被引用行删除/改键时若仍有子引用即拒绝。
// 不用 CASCADE/SET NULL——级联清理是业务编排，归发起方模块，数据库不做代行）
var fkBatches = []fkTable{
	{Table: "re_work_tag", FKs: []fkSpec{
		{Column: "work_id", Parent: "work"},
		{Column: "local_tag_id", Parent: "local_tag"},
		{Column: "site_tag_id", Parent: "site_tag"},
	}},
	{Table: "re_work_author", FKs: []fkSpec{
		{Column: "work_id", Parent: "work"},
		{Column: "local_author_id", Parent: "local_author"},
		{Column: "site_author_id", Parent: "site_author"},
	}},
	{Table: "re_work_work_set", FKs: []fkSpec{
		{Column: "work_id", Parent: "work"},
		{Column: "work_set_id", Parent: "work_set"},
	}},
	{Table: "re_work_set_work_set", FKs: []fkSpec{
		{Column: "parent_work_set_id", Parent: "work_set"},
		{Column: "child_work_set_id", Parent: "work_set"},
	}},
	{Table: "work", FKs: []fkSpec{
		{Column: "site_id", Parent: "site"},
		{Column: "local_author_id", Parent: "local_author"},
	}},
	{Table: "work_set", FKs: []fkSpec{
		{Column: "site_id", Parent: "site"},
		{Column: "cover_work_id", Parent: "work"},
	}},
	{Table: "site_tag", FKs: []fkSpec{
		{Column: "site_id", Parent: "site"},
	}},
	{Table: "site_author", FKs: []fkSpec{
		{Column: "site_id", Parent: "site"},
	}},
	{Table: "task", FKs: []fkSpec{
		{Column: "pid", Parent: "task"},
		{Column: "site_id", Parent: "site"},
		{Column: "pending_resource_id", Parent: "resource"},
	}},
	{Table: "plugin", FKs: []fkSpec{
		{Column: "backup_id", Parent: "backup"},
	}},
	{Table: "resource", FKs: []fkSpec{
		{Column: "work_id", Parent: "work"},
		{Column: "task_id", Parent: "task"},
	}},
	{Table: "resource_store", FKs: []fkSpec{
		{Column: "resource_id", Parent: "resource"},
		{Column: "store_id", Parent: "persistent_store"},
	}},
	{Table: "plugin_storage", FKs: []fkSpec{
		{Column: "plugin_id", Parent: "plugin"},
	}},
	{Table: "persistent_store", FKs: []fkSpec{
		{Column: "backup_id", Parent: "backup"},
	}},
	{Table: "local_tag", FKs: []fkSpec{
		{Column: "base_local_tag_id", Parent: "local_tag"},
	}},
}

// OpenTestDB 测试库引导：内存 SQLite + 外键强制执行 + 单连接（内存库每连接独立，
// 连接池再开新连接会得到空库）+ 完整迁移（含外键声明与悬空清理）。
// 供各模块测试替换裸 gorm.Open，使测试库与生产库行为一致（外键同样强制）
func OpenTestDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(":memory:?_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err := AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("测试库迁移失败: %w", err)
	}
	return db, nil
}

// ApplyForeignKeys 为登记面的表挂外键。SQLite 无 ALTER TABLE ADD CONSTRAINT，
// 经表重建完成：以 sqlite_master 现表 DDL 为源（列序保真——实体字段序与表列序可能
// 漂移，INSERT SELECT 按位拷贝，列错位且类型兼容时是静默数据损坏），注入 FK 子句建
// 新表、拷数据、删旧表、改回原名、复原索引
func ApplyForeignKeys(db *gorm.DB) error {
	for _, t := range fkBatches {
		done, err := fkDeclared(db, t)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		if err := rebuildTableWithFK(db, t); err != nil {
			return err
		}
	}
	return nil
}

// fkDeclared 幂等判定：该表期望的（引用列, 被引用表）对是否已全部在册。
// SQLite 外键约束无独立命名（pragma_foreign_key_list 输出不含约束名），不能按名判存在
func fkDeclared(db *gorm.DB, spec fkTable) (bool, error) {
	var rows []struct {
		Table string
		From  string
	}
	if err := db.Raw(fmt.Sprintf("PRAGMA foreign_key_list('%s')", spec.Table)).Scan(&rows).Error; err != nil {
		return false, fmt.Errorf("读取 %s 外键清单失败: %w", spec.Table, err)
	}
	present := make(map[string]bool, len(rows))
	for _, r := range rows {
		present[r.From+"→"+r.Table] = true
	}
	for _, fk := range spec.FKs {
		if !present[fk.Column+"→"+fk.Parent] {
			return false, nil
		}
	}
	return true, nil
}

// rebuildTableWithFK 单表重建舞步。foreign_keys 开关切换须在事务外（事务内为无操作）；
// 重建期间关闭（否则删旧表时的隐式全行删除会触发子引用违约），结束后恢复原值
func rebuildTableWithFK(db *gorm.DB, spec fkTable) error {
	var ddl string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", spec.Table).Scan(&ddl).Error; err != nil {
		return fmt.Errorf("读取 %s 建表 DDL 失败: %w", spec.Table, err)
	}
	if ddl == "" {
		return fmt.Errorf("表 %s 不存在，无法挂外键", spec.Table)
	}

	// 现表索引清单：索引归属旧表、DROP TABLE 时一并消亡，重建后须逐一复原
	// （sql 为 NULL 的行是 SQLite 自动索引，不在 sqlite_master 存 DDL，无需复原）
	type indexDDL struct {
		Name string
		Sql  string
	}
	var idxs []indexDDL
	if err := db.Raw("SELECT name, sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND sql IS NOT NULL", spec.Table).Scan(&idxs).Error; err != nil {
		return fmt.Errorf("读取 %s 索引清单失败: %w", spec.Table, err)
	}

	newName := spec.Table + "_fk_rebuild"
	newDDL := injectForeignKeys(renameCreateTable(ddl, spec.Table, newName), spec.FKs)

	var prior int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&prior).Error; err != nil {
		return fmt.Errorf("读取 foreign_keys 状态失败: %w", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
		return fmt.Errorf("关闭 foreign_keys 失败: %w", err)
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(newDDL).Error; err != nil {
			return fmt.Errorf("建新表 %s 失败: %w", newName, err)
		}
		if err := tx.Exec(fmt.Sprintf("INSERT INTO `%s` SELECT * FROM `%s`", newName, spec.Table)).Error; err != nil {
			return fmt.Errorf("拷贝 %s 数据失败: %w", spec.Table, err)
		}
		if err := tx.Exec(fmt.Sprintf("DROP TABLE `%s`", spec.Table)).Error; err != nil {
			return fmt.Errorf("删旧表 %s 失败: %w", spec.Table, err)
		}
		// FK 关闭态下 RENAME 不联动修改他表 REFERENCES 子句；终名与原名一致，引用按名照常解析
		if err := tx.Exec(fmt.Sprintf("ALTER TABLE `%s` RENAME TO `%s`", newName, spec.Table)).Error; err != nil {
			return fmt.Errorf("改回表名 %s 失败: %w", spec.Table, err)
		}
		for _, idx := range idxs {
			if err := tx.Exec(idx.Sql).Error; err != nil {
				return fmt.Errorf("复原索引 %s 失败: %w", idx.Name, err)
			}
		}
		return nil
	})
	if restoreErr := db.Exec(fmt.Sprintf("PRAGMA foreign_keys = %d", prior)).Error; restoreErr != nil && err == nil {
		err = restoreErr
	}
	return err
}

// renameCreateTable 替换建表 DDL 中的表名（首个命中——CREATE TABLE 后的表名是首个
// 包裹该名的标识符）。sqlite_master 存储的 DDL 引号风格随来源而异：GORM 建表为反引号、
// 经 RENAME 后被 SQLite 规范化为双引号，两种风格都尝试
func renameCreateTable(ddl, oldName, newName string) string {
	if strings.Contains(ddl, "`"+oldName+"`") {
		return strings.Replace(ddl, "`"+oldName+"`", "`"+newName+"`", 1)
	}
	return strings.Replace(ddl, "\""+oldName+"\"", "\""+newName+"\"", 1)
}

// injectForeignKeys 在建表 DDL 收口括号前注入缺失的 FK 子句（NO ACTION 为 SQLite 默认
// 行为，显式写出以自释义）。已在册的 FK 不重复注入——二次重建的表 DDL 含此前批次的子句
func injectForeignKeys(ddl string, fks []fkSpec) string {
	norm := strings.NewReplacer("`", "", "\"", "").Replace(ddl)
	clauses := make([]string, 0, len(fks))
	for _, fk := range fks {
		if strings.Contains(norm, fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s", fk.Column, fk.Parent)) {
			continue
		}
		clauses = append(clauses, fmt.Sprintf("FOREIGN KEY (`%s`) REFERENCES `%s` (`id`) ON DELETE NO ACTION ON UPDATE NO ACTION", fk.Column, fk.Parent))
	}
	if len(clauses) == 0 {
		return ddl
	}
	closeIdx := strings.LastIndexByte(ddl, ')')
	body := strings.TrimRight(ddl[:closeIdx], " \t\r\n")
	return body + ",\n  " + strings.Join(clauses, ",\n  ") + "\n" + ddl[closeIdx:]
}

// cleanDanglingAssociations 清理悬空引用（历史删除链缺口遗留——指向已不存在父行的行）。
// 外键只拦新写入与被引用父行的删除、不动存量行，不清理则 foreign_key_check 永不清零。
// 两类处置：关联表行悬空（关联行自身失去意义，DELETE）；业务行引用列悬空（行是存续主体，
// 引用置 NULL 修复——对齐「记录指向不存在行时清引用」语义）
func cleanDanglingAssociations(db *gorm.DB) error {
	deletes := []string{
		"DELETE FROM re_work_tag WHERE work_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM work WHERE id = re_work_tag.work_id)",
		"DELETE FROM re_work_tag WHERE local_tag_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM local_tag WHERE id = re_work_tag.local_tag_id)",
		"DELETE FROM re_work_tag WHERE site_tag_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM site_tag WHERE id = re_work_tag.site_tag_id)",
		"DELETE FROM re_work_author WHERE work_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM work WHERE id = re_work_author.work_id)",
		"DELETE FROM re_work_author WHERE local_author_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM local_author WHERE id = re_work_author.local_author_id)",
		"DELETE FROM re_work_author WHERE site_author_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM site_author WHERE id = re_work_author.site_author_id)",
		"DELETE FROM re_work_work_set WHERE work_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM work WHERE id = re_work_work_set.work_id)",
		"DELETE FROM re_work_work_set WHERE work_set_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM work_set WHERE id = re_work_work_set.work_set_id)",
		"DELETE FROM re_work_set_work_set WHERE parent_work_set_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM work_set WHERE id = re_work_set_work_set.parent_work_set_id)",
		"DELETE FROM re_work_set_work_set WHERE child_work_set_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM work_set WHERE id = re_work_set_work_set.child_work_set_id)",
		// 恒有值族（int64 列，0=未填充）：先摘关联行再删悬空主体行，防止级联产生新悬空
		"DELETE FROM resource_store WHERE resource_id <> 0 AND NOT EXISTS (SELECT 1 FROM resource WHERE id = resource_store.resource_id)",
		"DELETE FROM resource_store WHERE store_id <> 0 AND NOT EXISTS (SELECT 1 FROM persistent_store WHERE id = resource_store.store_id)",
		"DELETE FROM resource WHERE work_id <> 0 AND NOT EXISTS (SELECT 1 FROM work WHERE id = resource.work_id)",
		"DELETE FROM plugin_storage WHERE plugin_id <> 0 AND NOT EXISTS (SELECT 1 FROM plugin WHERE id = plugin_storage.plugin_id)",
	}
	nulls := []string{
		"UPDATE work SET site_id = NULL WHERE site_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM site WHERE id = work.site_id)",
		"UPDATE work SET local_author_id = NULL WHERE local_author_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM local_author WHERE id = work.local_author_id)",
		"UPDATE work_set SET site_id = NULL WHERE site_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM site WHERE id = work_set.site_id)",
		"UPDATE work_set SET cover_work_id = NULL WHERE cover_work_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM work WHERE id = work_set.cover_work_id)",
		"UPDATE site_tag SET site_id = NULL WHERE site_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM site WHERE id = site_tag.site_id)",
		"UPDATE site_author SET site_id = NULL WHERE site_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM site WHERE id = site_author.site_id)",
		"UPDATE task SET site_id = NULL WHERE site_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM site WHERE id = task.site_id)",
		"UPDATE task SET pid = NULL WHERE pid IS NOT NULL AND NOT EXISTS (SELECT 1 FROM task WHERE id = task.pid)",
		"UPDATE task SET pending_resource_id = NULL WHERE pending_resource_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM resource WHERE id = task.pending_resource_id)",
		"UPDATE plugin SET backup_id = NULL WHERE backup_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM backup WHERE id = plugin.backup_id)",
		"UPDATE persistent_store SET backup_id = NULL WHERE backup_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM backup WHERE id = persistent_store.backup_id)",
		"UPDATE resource SET task_id = NULL WHERE task_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM task WHERE id = resource.task_id)",
		"UPDATE local_tag SET base_local_tag_id = NULL WHERE base_local_tag_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM local_tag WHERE id = local_tag.base_local_tag_id)",
	}
	// 无引用哨兵 0→NULL 归一（0 哨兵列改 NULL 语义的存量迁移；幂等）
	zeroToNull := []string{
		"UPDATE persistent_store SET backup_id = NULL WHERE backup_id = 0",
		"UPDATE resource SET task_id = NULL WHERE task_id = 0",
		"UPDATE local_tag SET base_local_tag_id = NULL WHERE base_local_tag_id = 0",
	}
	for _, stmt := range zeroToNull {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("哨兵 NULL 化失败: %w", err)
		}
	}
	for _, stmt := range deletes {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("清理关联悬空行失败: %w", err)
		}
	}
	for _, stmt := range nulls {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("修复悬空引用失败: %w", err)
		}
	}
	return nil
}
