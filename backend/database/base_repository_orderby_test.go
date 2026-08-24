package database

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"gorm.io/gorm/clause"
)

// TestQueryOptionOrderByNotSilentlyDropped 验证 QueryOption.OrderBy 真实生效：
// GORM Order() 按元素具体类型分派，整切片传入不进任何 case 而被静默忽略——
// 该缺陷曾致 ResolveFileState 同路径多代行取错（无排序时按索引序命中最老已删行，
// /store/ 状态路由误走 backup 反查返回 404）。回归口径：按 deleted_at 升序 + Limit 1
// 必须命中活行（deleted_at=0），而非插入序更早的已删行。
func TestQueryOptionOrderByNotSilentlyDropped(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	if err := db.AutoMigrate(&entity.PersistentStore{}); err != nil {
		t.Fatalf("迁移测试实体失败: %v", err)
	}

	// 先插已删行（id 更小）、后插活行——无排序时 SQLite 按索引序返回先插的已删行
	deleted := entity.NewPersistentStore()
	deleted.FilePath = sql.NullString{String: "store/resource/a.mp4", Valid: true}
	if err := db.Create(deleted).Error; err != nil {
		t.Fatalf("插入已删行失败: %v", err)
	}
	if err := db.Exec("UPDATE persistent_store SET deleted_at = 1787057774285 WHERE id = ?", deleted.GetID()).Error; err != nil {
		t.Fatalf("标记已删失败: %v", err)
	}
	active := entity.NewPersistentStore()
	active.FilePath = sql.NullString{String: "store/resource/a.mp4", Valid: true}
	if err := db.Create(active).Error; err != nil {
		t.Fatalf("插入活行失败: %v", err)
	}

	repo := NewBaseRepository[entity.PersistentStore](db)
	opt := &QueryOption{
		Conditions:     []clause.Expression{clause.Eq{Column: "file_path", Value: "store/resource/a.mp4"}},
		OrderBy:        []clause.Expression{clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "deleted_at"}}}}},
		IncludeDeleted: true,
		Limit:          1,
	}
	records, err := repo.List(context.Background(), opt)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("期望命中 1 行，实际 %d 行", len(records))
	}
	if records[0].GetID() != active.GetID() {
		t.Fatalf("排序失效：Limit 1 命中已删行 id=%d（期望活行 id=%d）——OrderBy 被 GORM 静默丢弃",
			records[0].GetID(), active.GetID())
	}
}
