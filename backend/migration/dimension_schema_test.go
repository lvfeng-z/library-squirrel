package migration

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 关联级维度体系 schema 锚定：唯一索引含维度值（同作品同标签/作者多维度值并存、同三元组收敛）、
// 空串不因 NULL 语义漏洞产生重复行、site_tag 无 namespace 列、清单表唯一键生效。

// tableColumns 表列清单（列名 → notnull）
func tableColumns(t *testing.T, db *gorm.DB, table string) map[string]int {
	t.Helper()
	var rows []struct {
		Name    string `gorm:"column:name"`
		NotNull int    `gorm:"column:notnull"`
	}
	if err := db.Raw(`SELECT name, "notnull" AS "notnull" FROM pragma_table_info(?)`, table).Scan(&rows).Error; err != nil {
		t.Fatalf("读 %s 列清单失败: %v", table, err)
	}
	m := make(map[string]int, len(rows))
	for _, r := range rows {
		m[r.Name] = r.NotNull
	}
	return m
}

// indexColumns 索引列清单（按序号）；索引不存在返回 nil
func indexColumns(t *testing.T, db *gorm.DB, index string) []string {
	t.Helper()
	var rows []struct {
		Name string `gorm:"column:name"`
	}
	if err := db.Raw("SELECT name FROM pragma_index_info(?) ORDER BY seqno", index).Scan(&rows).Error; err != nil {
		t.Fatalf("读索引 %s 列清单失败: %v", index, err)
	}
	if len(rows) == 0 {
		return nil
	}
	cols := make([]string, 0, len(rows))
	for _, r := range rows {
		cols = append(cols, r.Name)
	}
	return cols
}

// TestDimensionUniqueIndexesContainDimensionValue 含维度值的唯一索引行为锚：
// 同 (work, tag) 异 ns 两行并存、同三元组冲突拒绝；空串维度下同 (work, tag) 不产生重复行
func TestDimensionUniqueIndexesContainDimensionValue(t *testing.T) {
	db, err := OpenTestDB()
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	seed := `
		INSERT INTO site (id, site_key, site_name, create_time, update_time) VALUES (1, 'pixiv', 'pixiv', 0, 0);
		INSERT INTO work (id, site_id, site_work_id, create_time, update_time) VALUES (10, 1, 'w-1', 0, 0);
		INSERT INTO site_tag (id, site_id, site_tag_id, create_time, update_time) VALUES (100, 1, 't-1', 0, 0);
		INSERT INTO local_tag (id, local_tag_name, create_time, update_time) VALUES (101, '本地标签', 0, 0);
		INSERT INTO site_author (id, site_id, site_author_id, create_time, update_time) VALUES (102, 1, 'a-1', 0, 0);
		INSERT INTO local_author (id, author_name, create_time, update_time) VALUES (103, '本地作者', 0, 0);
	`
	if err := db.Exec(seed).Error; err != nil {
		t.Fatalf("种子数据失败: %v", err)
	}

	// 同 (work, site_tag) 异 ns：两行并存
	if err := db.Exec(`INSERT INTO re_work_tag (work_id, tag_type, site_tag_id, namespace, source, create_time, update_time)
		VALUES (10, 1, 100, 'female', 0, 0, 0), (10, 1, 100, 'male', 0, 0, 0)`).Error; err != nil {
		t.Fatalf("同作品同标签多 ns 应并存: %v", err)
	}
	// 同三元组冲突拒绝（空串参与唯一性比较，无 NULL 不参与漏洞）
	if err := db.Exec(`INSERT INTO re_work_tag (work_id, tag_type, site_tag_id, namespace, source, create_time, update_time)
		VALUES (10, 1, 100, 'female', 0, 0, 0)`).Error; err == nil {
		t.Fatal("重复三元组 (work=10, tag=100, ns=female) 应被唯一索引拒绝")
	}
	// 空串维度值先落一行、再重复落同三元组被拒（空串不因 NULL 语义漏洞产生重复行）
	if err := db.Exec(`INSERT INTO re_work_tag (work_id, tag_type, site_tag_id, namespace, source, create_time, update_time)
		VALUES (10, 1, 100, '', 0, 0, 0)`).Error; err != nil {
		t.Fatalf("空串维度行应正常落库: %v", err)
	}
	if err := db.Exec(`INSERT INTO re_work_tag (work_id, tag_type, site_tag_id, namespace, source, create_time, update_time)
		VALUES (10, 1, 100, '', 0, 0, 0)`).Error; err == nil {
		t.Fatal("重复三元组 (work=10, tag=100, ns=空串) 应被唯一索引拒绝")
	}

	// 同 (work, site_author) 异 role：两行并存；同三元组拒绝
	if err := db.Exec(`INSERT INTO re_work_author (work_id, author_type, site_author_id, role_name, sort_order, source, create_time, update_time)
		VALUES (10, 1, 102, '原画', 0, 0, 0, 0), (10, 1, 102, '脚本', 1, 0, 0, 0)`).Error; err != nil {
		t.Fatalf("同作品同作者多 role 应并存: %v", err)
	}
	if err := db.Exec(`INSERT INTO re_work_author (work_id, author_type, site_author_id, role_name, sort_order, source, create_time, update_time)
		VALUES (10, 1, 102, '原画', 2, 0, 0, 0)`).Error; err == nil {
		t.Fatal("重复三元组 (work=10, author=102, role=原画) 应被唯一索引拒绝")
	}

	// LOCAL 轨同型（site_tag_id/local_author_id 为 NULL 的行不与 SITE 轨冲突）
	if err := db.Exec(`INSERT INTO re_work_tag (work_id, tag_type, local_tag_id, namespace, source, create_time, update_time)
		VALUES (10, 0, 101, '我的标记', 1, 0, 0), (10, 0, 101, '', 1, 0, 0)`).Error; err != nil {
		t.Fatalf("LOCAL 轨多 ns 应并存: %v", err)
	}
}

// TestDimensionSchemaShape schema 形态锚：维度列 NOT NULL、四条唯一索引含维度列、
// site_tag 无 namespace 列、清单表存在且 value 唯一
func TestDimensionSchemaShape(t *testing.T) {
	db, err := OpenTestDB()
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}

	rwtCols := tableColumns(t, db, "re_work_tag")
	if rwtCols["namespace"] != 1 {
		t.Fatalf("re_work_tag.namespace 应为 NOT NULL，实际 notnull=%d", rwtCols["namespace"])
	}
	rwaCols := tableColumns(t, db, "re_work_author")
	if rwaCols["role_name"] != 1 {
		t.Fatalf("re_work_author.role_name 应为 NOT NULL，实际 notnull=%d", rwaCols["role_name"])
	}
	if _, ok := tableColumns(t, db, "site_tag")["namespace"]; ok {
		t.Fatal("site_tag 不应存在 namespace 列")
	}

	// 四条唯一索引的列集含维度列；旧两列索引不再存在
	for idx, want := range map[string][]string{
		"idx_re_work_tag_work_local_tag_ns":         {"work_id", "local_tag_id", "namespace"},
		"idx_re_work_tag_work_site_tag_ns":          {"work_id", "site_tag_id", "namespace"},
		"idx_re_work_author_work_local_author_role": {"work_id", "local_author_id", "role_name"},
		"idx_re_work_author_work_site_author_role":  {"work_id", "site_author_id", "role_name"},
	} {
		got := indexColumns(t, db, idx)
		if len(got) != len(want) {
			t.Fatalf("索引 %s 列集 = %v，期望 %v", idx, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("索引 %s 列集 = %v，期望 %v", idx, got, want)
			}
		}
	}
	for _, old := range []string{"idx_re_work_tag_work_local_tag", "idx_re_work_tag_work_site_tag",
		"idx_re_work_author_work_local_author", "idx_re_work_author_work_site_author"} {
		if indexColumns(t, db, old) != nil {
			t.Fatalf("旧两列唯一索引 %s 应已退役", old)
		}
	}

	// 清单表存在且 value 唯一生效
	if _, ok := tableColumns(t, db, "tag_namespace")["value"]; !ok {
		t.Fatal("tag_namespace 表应存在且含 value 列")
	}
	if _, ok := tableColumns(t, db, "author_role")["value"]; !ok {
		t.Fatal("author_role 表应存在且含 value 列")
	}
	if err := db.Exec(`INSERT INTO tag_namespace (value, label, origin, last_use, create_time, update_time)
		VALUES ('character', '角色', 2, 0, 0, 0)`).Error; err != nil {
		t.Fatalf("清单行插入失败: %v", err)
	}
	if err := db.Exec(`INSERT INTO tag_namespace (value, label, origin, last_use, create_time, update_time)
		VALUES ('character', '重复', 2, 0, 0, 0)`).Error; err == nil {
		t.Fatal("tag_namespace.value 唯一约束未生效")
	}
}

// TestMigrateDimensionColumnFromLegacyShape 旧形态库（可空维度列 + 旧两列唯一索引）经 AutoMigrate
// 升级：列改型 NOT NULL DEFAULT ”、存量 NULL 归一为空串、旧索引退役、新索引落成、数据保真；
// 二次执行幂等
func TestMigrateDimensionColumnFromLegacyShape(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:?_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	// 内存库单连接：foreign_keys PRAGMA 按连接生效，连接池多连接会绕过开关
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	// 旧形态 site_tag / re_work_tag：site_tag 带 namespace 列、关联维度列可空、唯一索引不含维度值。
	// 表名与列名用反引号包裹（GORM 建表风格——表重建舞步的改名与段落剔除按该引号风格定位）
	if err := db.Exec("CREATE TABLE `site_tag` (\n" +
		"`id` integer PRIMARY KEY AUTOINCREMENT,\n" +
		"`create_time` integer, `update_time` integer,\n" +
		"`site_id` integer, `site_tag_id` text, `site_tag_name` text, `base_site_tag_id` text,\n" +
		"`description` text, `local_tag_id` integer, `namespace` text, `last_use` integer,\n" +
		"CONSTRAINT `fk_site_tag_site` FOREIGN KEY (`site_id`) REFERENCES `site` (`id`) ON DELETE NO ACTION ON UPDATE NO ACTION\n" +
		")").Error; err != nil {
		t.Fatalf("建旧形态 site_tag 失败: %v", err)
	}
	if err := db.Exec("CREATE TABLE `re_work_tag` (\n" +
		"`id` integer PRIMARY KEY AUTOINCREMENT,\n" +
		"`create_time` integer, `update_time` integer,\n" +
		"`work_id` integer, `tag_type` integer, `local_tag_id` integer, `site_tag_id` integer,\n" +
		"`namespace` text, `source` integer DEFAULT 0,\n" +
		"CONSTRAINT `fk_re_work_tag_work` FOREIGN KEY (`work_id`) REFERENCES `work` (`id`) ON DELETE NO ACTION ON UPDATE NO ACTION\n" +
		")").Error; err != nil {
		t.Fatalf("建旧形态 re_work_tag 失败: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX `idx_re_work_tag_work_site_tag` ON `re_work_tag`(`work_id`,`site_tag_id`)").Error; err != nil {
		t.Fatalf("建旧唯一索引失败: %v", err)
	}
	// work/site 种子（外键引用目标，须先于关联行存在；反引号建表风格同上，列定义取最小集——
	// 不带内联 UNIQUE 约束，避免驱动端约束猜测触发的逐轮表重建）
	if err := db.Exec("CREATE TABLE `site` (`id` integer PRIMARY KEY AUTOINCREMENT, `create_time` integer, `update_time` integer, `site_key` text NOT NULL, `site_name` text)").Error; err != nil {
		t.Fatalf("建 site 失败: %v", err)
	}
	if err := db.Exec("INSERT INTO site (id, create_time, update_time, site_key) VALUES (1, 0, 0, 'pixiv')").Error; err != nil {
		t.Fatalf("种 site 失败: %v", err)
	}
	if err := db.Exec("CREATE TABLE `work` (`id` integer PRIMARY KEY AUTOINCREMENT, `create_time` integer, `update_time` integer, `site_id` integer, `site_work_id` text)").Error; err != nil {
		t.Fatalf("建 work 失败: %v", err)
	}
	if err := db.Exec(`INSERT INTO work (id, create_time, update_time, site_id, site_work_id) VALUES (10, 0, 0, 1, 'w-1')`).Error; err != nil {
		t.Fatalf("种 work 失败: %v", err)
	}
	if err := db.Exec(`INSERT INTO re_work_tag (id, create_time, update_time, work_id, tag_type, site_tag_id, namespace, source)
		VALUES (1, 0, 0, 10, 1, 100, 'female', 0), (2, 0, 0, 10, 0, NULL, NULL, 1)`).Error; err != nil {
		t.Fatalf("种旧行失败: %v", err)
	}
	// site_tag 行（关联外键目标；携带旧 namespace 值——列删除后行本身保留）
	if err := db.Exec(`INSERT INTO site_tag (id, create_time, update_time, site_id, site_tag_id, namespace)
		VALUES (100, 0, 0, 1, 't-1', 'character')`).Error; err != nil {
		t.Fatalf("种 site_tag 行失败: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("旧形态库迁移失败: %v", err)
	}
	// 二次执行幂等
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("二次迁移应幂等: %v", err)
	}

	if tableColumns(t, db, "re_work_tag")["namespace"] != 1 {
		t.Fatal("迁移后 re_work_tag.namespace 应为 NOT NULL")
	}
	if _, ok := tableColumns(t, db, "site_tag")["namespace"]; ok {
		t.Fatal("迁移后 site_tag 不应存在 namespace 列")
	}
	if indexColumns(t, db, "idx_re_work_tag_work_site_tag") != nil {
		t.Fatal("旧唯一索引应退役")
	}
	if indexColumns(t, db, "idx_re_work_tag_work_site_tag_ns") == nil {
		t.Fatal("新唯一索引应落成")
	}

	type row struct {
		ID        int64  `gorm:"column:id"`
		Namespace string `gorm:"column:namespace"`
		Source    int64  `gorm:"column:source"`
	}
	var rows []row
	if err := db.Raw("SELECT id, namespace, source FROM re_work_tag ORDER BY id").Scan(&rows).Error; err != nil {
		t.Fatalf("回查迁移后数据失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("迁移应保真两行，实际 %d 行", len(rows))
	}
	if rows[0].Namespace != "female" || rows[0].Source != 0 {
		t.Fatalf("行1 保真失败: %+v", rows[0])
	}
	if rows[1].Namespace != "" || rows[1].Source != 1 {
		t.Fatalf("行2 存量 NULL 应归一为空串: %+v", rows[1])
	}
}
