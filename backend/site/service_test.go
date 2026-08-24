package site_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/site"
	"github.com/library-squirrel/backend/siteAuthor"
	"github.com/library-squirrel/backend/siteTag"
	"github.com/library-squirrel/backend/task"
	"github.com/library-squirrel/backend/work"
	"github.com/library-squirrel/backend/workSet"

	"gorm.io/gorm"
)

// txTransactor 真实事务适配器（事务开启后把 tx 放进 ctx，repo 方法经 DBFromContext 取事务连接——
// 与生产装配 dbTransactorAdapter 同款，验证守卫计数与删除的事务内通路）
type txTransactor struct {
	db *gorm.DB
}

func (t *txTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return database.WithTransactionContext(ctx, t.db, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, database.TxKey, tx)
		return fn(txCtx)
	})
}

// newGuardTestEnv 外键强制内存库 + 五类真实计数仓储装配（site 删除守卫全链）
func newGuardTestEnv(t *testing.T) (*site.Service, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	svc := site.NewService(
		site.NewRepository(db),
		&txTransactor{db: db},
		work.NewRepository(db),
		task.NewRepository(db),
		workSet.NewRepository(db),
		siteTag.NewRepository(db),
		siteAuthor.NewRepository(db),
	)
	return svc, db
}

// seedSite 建站点行（id=1；work/task/work_set/site_tag/site_author 五面 site_id 外键的父行）
func seedSite(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec("INSERT INTO site (id, create_time, update_time) VALUES (1, 0, 0)").Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
}

// siteRowExists 站点行是否仍在
func siteRowExists(t *testing.T, db *gorm.DB) bool {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT COUNT(*) FROM site WHERE id = 1").Scan(&n).Error; err != nil {
		t.Fatalf("统计站点行失败: %v", err)
	}
	return n == 1
}

// assertRejected 断言删除被守卫拒绝：errors.Is 判别成立、消息含各预期片段与清理指引、站点行保留
func assertRejected(t *testing.T, svc *site.Service, db *gorm.DB, wantMsgParts ...string) {
	t.Helper()
	err := svc.Delete(context.Background(), 1)
	if err == nil {
		t.Fatal("站点删除应被守卫拒绝，实际成功")
	}
	if !errors.Is(err, site.ErrSiteHasReferences) {
		t.Fatalf("应返回 ErrSiteHasReferences（errors.Is 判别），实际 %v", err)
	}
	if !strings.Contains(err.Error(), "无法删除站点") {
		t.Fatalf("错误消息应以无法删除站点开头，实际 %q", err.Error())
	}
	for _, part := range wantMsgParts {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("错误消息应含 %q，实际 %q", part, err.Error())
		}
	}
	if !siteRowExists(t, db) {
		t.Fatal("守卫拒绝后站点行应保留")
	}
}

// mkWork 建挂在站点 1 下的作品行
func mkWork(t *testing.T, db *gorm.DB, siteWorkId string) {
	t.Helper()
	w := entity.NewWork()
	w.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	w.SiteWorkID = sql.NullString{String: siteWorkId, Valid: true}
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("建作品 %s 失败: %v", siteWorkId, err)
	}
}

// softDeleteWorkById 打软删标志（毫秒时间戳非零=已删）
func softDeleteWorkById(t *testing.T, db *gorm.DB, siteWorkId string) {
	t.Helper()
	if err := db.Exec("UPDATE work SET deleted_at = 1000 WHERE site_work_id = ?", siteWorkId).Error; err != nil {
		t.Fatalf("软删作品 %s 失败: %v", siteWorkId, err)
	}
}

// mkWorkSet 建挂在站点 1 下的作品集行
func mkWorkSet(t *testing.T, db *gorm.DB, siteWorkSetId string) {
	t.Helper()
	ws := entity.NewWorkSet()
	ws.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	ws.SiteWorkSetID = sql.NullString{String: siteWorkSetId, Valid: true}
	if err := db.Create(ws).Error; err != nil {
		t.Fatalf("建作品集 %s 失败: %v", siteWorkSetId, err)
	}
}

// softDeleteWorkSetById 打软删标志（毫秒时间戳非零=已删）
func softDeleteWorkSetById(t *testing.T, db *gorm.DB, siteWorkSetId string) {
	t.Helper()
	if err := db.Exec("UPDATE work_set SET deleted_at = 1000 WHERE site_work_set_id = ?", siteWorkSetId).Error; err != nil {
		t.Fatalf("软删作品集 %s 失败: %v", siteWorkSetId, err)
	}
}

// TestDeleteSiteGuard 站点删除纯守卫：五类引用（作品活/软删行、任务、作品集活/软删行、站点标签、
// 站点作者）任一存在即拒绝——errors.Is(ErrSiteHasReferences) 成立、消息含对应计数与清理指引、
// 站点行保留；全部为零 → 删除成功。预置子行均以站点行种子为父（外键强制）
func TestDeleteSiteGuard(t *testing.T) {
	t.Run("WorkAliveRow", func(t *testing.T) {
		svc, db := newGuardTestEnv(t)
		seedSite(t, db)
		mkWork(t, db, "w-1")
		assertRejected(t, svc, db, "作品 1", "在作品页删除相关作品")
	})

	t.Run("WorkDeletedRow", func(t *testing.T) {
		svc, db := newGuardTestEnv(t)
		seedSite(t, db)
		mkWork(t, db, "w-1")
		softDeleteWorkById(t, db, "w-1")
		assertRejected(t, svc, db, "回收站中作品 1", "在回收站彻底删除相关作品")
	})

	t.Run("WorkAliveAndDeletedAggregated", func(t *testing.T) {
		svc, db := newGuardTestEnv(t)
		seedSite(t, db)
		mkWork(t, db, "w-1")
		mkWork(t, db, "w-2")
		mkWork(t, db, "w-3")
		softDeleteWorkById(t, db, "w-3")
		assertRejected(t, svc, db,
			"作品 3（含回收站 1）",
			"在作品页删除相关作品", "在回收站彻底删除相关作品")
	})

	t.Run("TaskRow", func(t *testing.T) {
		svc, db := newGuardTestEnv(t)
		seedSite(t, db)
		taskRow := entity.NewTask()
		taskRow.SiteID = sql.NullInt64{Int64: 1, Valid: true}
		taskRow.TaskName = sql.NullString{String: "任务一", Valid: true}
		if err := db.Create(taskRow).Error; err != nil {
			t.Fatalf("建任务失败: %v", err)
		}
		assertRejected(t, svc, db, "任务 1", "在任务列表删除相关任务")
	})

	t.Run("WorkSetAliveRow", func(t *testing.T) {
		svc, db := newGuardTestEnv(t)
		seedSite(t, db)
		mkWorkSet(t, db, "ws-1")
		assertRejected(t, svc, db, "作品集 1", "在作品集页删除相关作品集")
	})

	t.Run("WorkSetDeletedRow", func(t *testing.T) {
		svc, db := newGuardTestEnv(t)
		seedSite(t, db)
		mkWorkSet(t, db, "ws-1")
		softDeleteWorkSetById(t, db, "ws-1")
		assertRejected(t, svc, db, "回收站中作品集 1", "在回收站彻底删除相关作品集")
	})

	t.Run("SiteTagRow", func(t *testing.T) {
		svc, db := newGuardTestEnv(t)
		seedSite(t, db)
		tag := entity.NewSiteTag()
		tag.SiteID = sql.NullInt64{Int64: 1, Valid: true}
		tag.SiteTagID = sql.NullString{String: "st-1", Valid: true}
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("建站点标签失败: %v", err)
		}
		assertRejected(t, svc, db, "站点标签 1", "在站点标签页删除相关标签")
	})

	t.Run("SiteAuthorRow", func(t *testing.T) {
		svc, db := newGuardTestEnv(t)
		seedSite(t, db)
		author := entity.NewSiteAuthor()
		author.SiteID = sql.NullInt64{Int64: 1, Valid: true}
		author.SiteAuthorID = sql.NullString{String: "sa-1", Valid: true}
		if err := db.Create(author).Error; err != nil {
			t.Fatalf("建站点作者失败: %v", err)
		}
		assertRejected(t, svc, db, "站点作者 1", "在站点作者页删除相关作者")
	})

	t.Run("AllCategoriesTogether", func(t *testing.T) {
		svc, db := newGuardTestEnv(t)
		seedSite(t, db)
		mkWork(t, db, "w-1")
		taskRow := entity.NewTask()
		taskRow.SiteID = sql.NullInt64{Int64: 1, Valid: true}
		if err := db.Create(taskRow).Error; err != nil {
			t.Fatalf("建任务失败: %v", err)
		}
		tag := entity.NewSiteTag()
		tag.SiteID = sql.NullInt64{Int64: 1, Valid: true}
		tag.SiteTagID = sql.NullString{String: "st-1", Valid: true}
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("建站点标签失败: %v", err)
		}
		assertRejected(t, svc, db,
			"作品 1", "任务 1", "站点标签 1",
			"在作品页删除相关作品", "在任务列表删除相关任务", "在站点标签页删除相关标签")
	})

	t.Run("NoReferencesDeletesSite", func(t *testing.T) {
		svc, db := newGuardTestEnv(t)
		seedSite(t, db)
		if err := svc.Delete(context.Background(), 1); err != nil {
			t.Fatalf("五类引用全空时删除站点应成功，实际失败: %v", err)
		}
		if siteRowExists(t, db) {
			t.Fatal("删除成功后站点行应不存在")
		}
	})
}
