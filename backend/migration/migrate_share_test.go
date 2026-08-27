package migration

// share-host 任务行退役清理迁移测试（分享方发布去任务化：存量 share-host 任务行启动一次性
// 物理删除，其余任务类型不受影响）。

import (
	"database/sql"
	"testing"

	entity2 "github.com/library-squirrel/backend/base/model/entity"

	"gorm.io/gorm"
)

// insertTaskWithType 插入指定类型的任务行
func insertTaskWithType(t *testing.T, db *gorm.DB, taskType string) {
	t.Helper()
	task := entity2.NewTask()
	task.TaskName = sql.NullString{String: "测试任务", Valid: true}
	if taskType != "" {
		task.TaskType = sql.NullString{String: taskType, Valid: true}
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("插入任务失败: %v", err)
	}
}

// TestShareHostTaskCleanup share-host 任务行物理删除：share-host 全灭、插件任务与
// share-receive 任务保留；二次迁移幂等（无命中行不再变化）
func TestShareHostTaskCleanup(t *testing.T) {
	db := newMigratedWorkSetDB(t)
	insertTaskWithType(t, db, "share-host")
	insertTaskWithType(t, db, "share-host")
	insertTaskWithType(t, db, "share-receive")
	insertTaskWithType(t, db, "")

	// 二次迁移（模拟下次启动）：清理发生在 AutoMigrate 后段
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("二次迁移失败: %v", err)
	}

	var hostLeft, receiveLeft, pluginLeft int64
	db.Raw(`SELECT COUNT(*) FROM task WHERE task_type = 'share-host'`).Scan(&hostLeft)
	db.Raw(`SELECT COUNT(*) FROM task WHERE task_type = 'share-receive'`).Scan(&receiveLeft)
	db.Raw(`SELECT COUNT(*) FROM task WHERE task_type IS NULL OR task_type = ''`).Scan(&pluginLeft)
	if hostLeft != 0 {
		t.Fatalf("share-host 任务行应被物理删除, 残留 %d 行", hostLeft)
	}
	if receiveLeft != 1 || pluginLeft != 1 {
		t.Fatalf("其余任务类型不应受影响: share-receive=%d 插件任务=%d", receiveLeft, pluginLeft)
	}
}
