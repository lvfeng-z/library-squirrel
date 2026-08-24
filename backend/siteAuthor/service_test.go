package siteAuthor

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/reWorkAuthor"

	"gorm.io/gorm"
)

// txTransactor 真实事务适配器（事务开启后把 tx 放进 ctx，repo 方法经 DBFromContext 取事务连接——
// 与生产装配 dbTransactorAdapter 同款，验证删除编排各步的事务内通路）
type txTransactor struct {
	db *gorm.DB
}

func (t *txTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return database.WithTransactionContext(ctx, t.db, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, database.TxKey, tx)
		return fn(txCtx)
	})
}

// newDeleteTestEnv 内存库（外键强制 + 完整迁移）+ 删除编排的全部参与件真实落库（siteAuthor 仓储、
// reWorkAuthor 服务、事务执行器）；localAuthor/site 查询操作依赖不被删除路径触及，传 nil
func newDeleteTestEnv(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	svc := NewService(
		NewRepository(db),
		nil, // LocalAuthorOperator（删除编排不触及）
		nil, // SiteOperator（删除编排不触及）
		&txTransactor{db: db},
		reWorkAuthor.NewService(reWorkAuthor.NewRepository(db)),
	)
	return svc, db
}

// TestDeleteSiteAuthorCleansReferences 删除站点作者的编排锚定：作品-作者关联归零、作者行消亡。
// 外键强制库下「删除成功」本身即先清子后删父顺序的证明（未清关联即删行会被外键直接拒绝）。
func TestDeleteSiteAuthorCleansReferences(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	svc, db := newDeleteTestEnv(t)

	// 外键父行种子：work（re_work_author.work_id）+ site（site_author.site_id）
	if err := db.Exec("INSERT INTO work (id, create_time, update_time, deleted_at) VALUES (1, 0, 0, 0)").Error; err != nil {
		t.Fatalf("建作品种子失败: %v", err)
	}
	if err := db.Exec("INSERT INTO site (id, create_time, update_time) VALUES (1, 0, 0)").Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}

	siteAu := domain.NewSiteAuthor()
	siteAu.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	siteAu.SiteAuthorID = sql.NullString{String: "sa-1", Valid: true}
	siteAu.AuthorName = sql.NullString{String: "待删站点作者", Valid: true}
	if err := db.Create(siteAu).Error; err != nil {
		t.Fatalf("插 site_author 失败: %v", err)
	}

	// re_work_author 实体无工厂方法，生产代码同用字面量构建
	rel := &domain.ReWorkAuthor{
		BaseEntity:   &model.BaseEntity{},
		AuthorType:   sql.NullInt64{Int64: constant.SITE, Valid: true},
		WorkID:       sql.NullInt64{Int64: 1, Valid: true},
		SiteAuthorID: sql.NullInt64{Int64: siteAu.GetID(), Valid: true},
	}
	if err := db.Create(rel).Error; err != nil {
		t.Fatalf("插 re_work_author 失败: %v", err)
	}

	if err := svc.Delete(context.Background(), siteAu.GetID()); err != nil {
		t.Fatalf("删除站点作者失败: %v", err)
	}

	var relCount int64
	if err := db.Model(&domain.ReWorkAuthor{}).Where("site_author_id = ?", siteAu.GetID()).Count(&relCount).Error; err != nil {
		t.Fatalf("统计 re_work_author 失败: %v", err)
	}
	if relCount != 0 {
		t.Fatalf("删除后 re_work_author 关联应归零，实际 %d", relCount)
	}

	var authorCount int64
	if err := db.Model(&domain.SiteAuthor{}).Where("id = ?", siteAu.GetID()).Count(&authorCount).Error; err != nil {
		t.Fatalf("统计 site_author 失败: %v", err)
	}
	if authorCount != 0 {
		t.Fatalf("删除后 site_author 行应消亡，实际 %d", authorCount)
	}
}
