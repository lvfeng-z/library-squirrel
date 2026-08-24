package siteTag

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/reWorkTag"

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

// newDeleteTestEnv 内存库（外键强制 + 完整迁移）+ 删除编排的全部参与件真实落库（siteTag 仓储、
// reWorkTag 仓储、事务执行器）；localTag/site 查询操作依赖不被删除路径触及，传 nil
func newDeleteTestEnv(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	svc := NewService(
		NewRepository(db),
		nil, // LocalTagOperator（删除编排不触及）
		nil, // LocalTagQueryOperator（删除编排不触及）
		nil, // SiteQueryOperator（删除编排不触及）
		&txTransactor{db: db},
		reWorkTag.NewRepository(db),
	)
	return svc, db
}

// TestDeleteSiteTagCleansReferences 删除站点标签的编排锚定：作品-标签关联归零、标签行消亡。
// 外键强制库下「删除成功」本身即先清子后删父顺序的证明（未清关联即删行会被外键直接拒绝）。
func TestDeleteSiteTagCleansReferences(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	svc, db := newDeleteTestEnv(t)

	// 外键父行种子：work（re_work_tag.work_id）+ site（site_tag.site_id）
	if err := db.Exec("INSERT INTO work (id, create_time, update_time, deleted_at) VALUES (1, 0, 0, 0)").Error; err != nil {
		t.Fatalf("建作品种子失败: %v", err)
	}
	site := domain.NewSite()
	site.SiteName = sql.NullString{String: "测试站点", Valid: true}
	if err := db.Create(site).Error; err != nil {
		t.Fatalf("插 site 失败: %v", err)
	}

	siteTagRow := domain.NewSiteTag()
	siteTagRow.SiteID = sql.NullInt64{Int64: site.GetID(), Valid: true}
	siteTagRow.SiteTagID = sql.NullString{String: "site-tag-1", Valid: true}
	siteTagRow.SiteTagName = sql.NullString{String: "待删站点标签", Valid: true}
	if err := db.Create(siteTagRow).Error; err != nil {
		t.Fatalf("插 site_tag 失败: %v", err)
	}

	rel := domain.NewReWorkTag()
	rel.WorkID = sql.NullInt64{Int64: 1, Valid: true}
	rel.TagType = sql.NullInt64{Int64: constant.SITE, Valid: true}
	rel.SiteTagID = sql.NullInt64{Int64: siteTagRow.GetID(), Valid: true}
	if err := db.Create(rel).Error; err != nil {
		t.Fatalf("插 re_work_tag 失败: %v", err)
	}

	if err := svc.Delete(context.Background(), siteTagRow.GetID()); err != nil {
		t.Fatalf("删除站点标签失败: %v", err)
	}

	var relCount int64
	if err := db.Model(&domain.ReWorkTag{}).Where("site_tag_id = ?", siteTagRow.GetID()).Count(&relCount).Error; err != nil {
		t.Fatalf("统计 re_work_tag 失败: %v", err)
	}
	if relCount != 0 {
		t.Fatalf("删除后 re_work_tag 关联应归零，实际 %d", relCount)
	}

	var tagCount int64
	if err := db.Model(&domain.SiteTag{}).Where("id = ?", siteTagRow.GetID()).Count(&tagCount).Error; err != nil {
		t.Fatalf("统计 site_tag 失败: %v", err)
	}
	if tagCount != 0 {
		t.Fatalf("删除后 site_tag 行应消亡，实际 %d", tagCount)
	}
}
