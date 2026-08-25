package migration

import (
	"testing"

	"gorm.io/gorm"
)

// TestCleanDanglingSelfReferencePreservesLiveParent 自引用外键（task.pid / local_tag.base_local_tag_id）
// 的悬空清理不得误伤在库父行：清理语句子查询的内层 FROM 与外层表同名时会遮蔽外层表引用，
// 条件退化为「内层行自身 id=引用列」恒空、NOT EXISTS 恒真，全表引用列被清（存量事故实锚——
// 每次启动幂等执行，子任务 pid 全部丢失致任务页树状塌平）。锚定：在库父子关系保留、悬空引用清 NULL。
func TestCleanDanglingSelfReferencePreservesLiveParent(t *testing.T) {
	db, err := OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}

	// task：父(has_child=1) + 子(pid=父id) 正常写入；悬空 pid 行关 PRAGMA 种植（FK 强制下不可正常产生）
	if err := db.Exec(`INSERT INTO task (id, task_name, has_child, pid, status, create_time, update_time)
		VALUES (1, 'parent', 1, NULL, 0, 0, 0), (2, 'child', 0, 1, 0, 0, 0)`).Error; err != nil {
		t.Fatalf("种植 task 父子行失败: %v", err)
	}
	plantSelfRefDangling(t, db, `INSERT INTO task (id, task_name, has_child, pid, status, create_time, update_time)
		VALUES (3, 'orphan', 0, 99999, 0, 0, 0)`)

	// local_tag 同款：父 + 子(非 0 引用) + 悬空
	if err := db.Exec(`INSERT INTO local_tag (id, local_tag_name, base_local_tag_id, create_time, update_time)
		VALUES (1, 'root', NULL, 0, 0), (2, 'sub', 1, 0, 0)`).Error; err != nil {
		t.Fatalf("种植 local_tag 父子行失败: %v", err)
	}
	plantSelfRefDangling(t, db, `INSERT INTO local_tag (id, local_tag_name, base_local_tag_id, create_time, update_time)
		VALUES (3, 'dangling', 88888, 0, 0)`)

	// 全量迁移幂等重跑（模拟下次启动，cleanDanglingAssociations 随之执行）
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("二次迁移失败: %v", err)
	}

	pidOf := func(table, id string) *int64 {
		var pid *int64
		if err := db.Raw("SELECT " + selfRefCol(table) + " FROM " + table + " WHERE id = " + id).Scan(&pid).Error; err != nil {
			t.Fatalf("查 %s 行 %s 失败: %v", table, id, err)
		}
		return pid
	}
	if pid := pidOf("task", "2"); pid == nil || *pid != 1 {
		t.Fatalf("子任务 pid 应保留父 ID 1，实际 %+v", pid)
	}
	if pid := pidOf("task", "3"); pid != nil {
		t.Fatalf("悬空 pid 行应清 NULL，实际 %d", *pid)
	}
	if pid := pidOf("local_tag", "2"); pid == nil || *pid != 1 {
		t.Fatalf("子标签 base_local_tag_id 应保留父 ID 1，实际 %+v", pid)
	}
	if pid := pidOf("local_tag", "3"); pid != nil {
		t.Fatalf("悬空 base_local_tag_id 行应清 NULL，实际 %d", *pid)
	}
}

// selfRefCol 自引用清理锚定的引用列名
func selfRefCol(table string) string {
	if table == "task" {
		return "pid"
	}
	return "base_local_tag_id"
}

// plantSelfRefDangling 外键强制下悬空引用不可经正常写入产生，短暂关闭外键种植存量遗留形态
func plantSelfRefDangling(t *testing.T, db *gorm.DB, insert string) {
	t.Helper()
	if err := db.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
		t.Fatalf("关闭外键失败: %v", err)
	}
	if err := db.Exec(insert).Error; err != nil {
		t.Fatalf("种植悬空行失败: %v", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("恢复外键失败: %v", err)
	}
}
