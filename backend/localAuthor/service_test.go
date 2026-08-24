package localAuthor

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
	"github.com/library-squirrel/backend/siteAuthor"
	"github.com/library-squirrel/backend/work"

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

// newDeleteTestEnv 外键强制内存库 + 真实提供方装配（siteAuthor/work 仓储与 reWorkAuthor 服务）
func newDeleteTestEnv(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	svc := NewService(
		NewRepository(db),
		&txTransactor{db: db},
		siteAuthor.NewRepository(db),
		reWorkAuthor.NewService(reWorkAuthor.NewRepository(db)),
		work.NewRepository(db),
	)
	return svc, db
}

// TestDeleteLocalAuthorCleansReferences 删除本地作者在单事务内清三类指向引用后删作者行：
// site_author 绑定列置 NULL（行保留）、re_work_author 关联行归零、work 镜像列置 NULL——
// 含软删 work 行（外键拦截不分行态，软删行引用不清则删作者即被拒）。删除成功本身即
// 「先清子引用后删父行」顺序的证明（任一步漏做，外键强制库直接违约报错）
func TestDeleteLocalAuthorCleansReferences(t *testing.T) {
	svc, db := newDeleteTestEnv(t)

	// 站点种子（work.site_id / site_author.site_id 两外键的父行）
	if err := db.Exec("INSERT INTO site (id, create_time, update_time) VALUES (1, 0, 0)").Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
	author := domain.NewLocalAuthor()
	author.AuthorName = sql.NullString{String: "作者甲", Valid: true}
	if err := db.Create(author).Error; err != nil {
		t.Fatalf("建本地作者失败: %v", err)
	}

	// 站点作者绑定该本地作者（绑定列）
	siteAu := domain.NewSiteAuthor()
	siteAu.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	siteAu.SiteAuthorID = sql.NullString{String: "sa-1", Valid: true}
	siteAu.LocalAuthorID = sql.NullInt64{Int64: author.ID, Valid: true}
	if err := db.Create(siteAu).Error; err != nil {
		t.Fatalf("建站点作者失败: %v", err)
	}

	// 两个作品持镜像列：一活行一软删行
	mkWork := func(siteWorkId string) *domain.Work {
		t.Helper()
		w := domain.NewWork()
		w.SiteID = sql.NullInt64{Int64: 1, Valid: true}
		w.SiteWorkID = sql.NullString{String: siteWorkId, Valid: true}
		w.LocalAuthorID = sql.NullInt64{Int64: author.ID, Valid: true}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("建作品 %s 失败: %v", siteWorkId, err)
		}
		return w
	}
	liveWork, deadWork := mkWork("w-live"), mkWork("w-dead")
	if err := db.Exec("UPDATE work SET deleted_at = 1000 WHERE id = ?", deadWork.ID).Error; err != nil {
		t.Fatalf("软删作品失败: %v", err)
	}

	// 两作品各挂一条本地作者关联（re_work_author；该实体无工厂方法，生产代码同用字面量构建）
	mkRel := func(workId int64) {
		t.Helper()
		rel := &domain.ReWorkAuthor{
			BaseEntity:    &model.BaseEntity{},
			AuthorType:    sql.NullInt64{Int64: constant.LOCAL, Valid: true},
			WorkID:        sql.NullInt64{Int64: workId, Valid: true},
			LocalAuthorID: sql.NullInt64{Int64: author.ID, Valid: true},
		}
		if err := db.Create(rel).Error; err != nil {
			t.Fatalf("建作品-作者关联失败: %v", err)
		}
	}
	mkRel(liveWork.ID)
	mkRel(deadWork.ID)

	if err := svc.Delete(context.Background(), author.ID); err != nil {
		t.Fatalf("删除本地作者失败（三类引用清理应已先行完成）: %v", err)
	}

	// 作者行归零
	var authorCount int64
	if err := db.Raw("SELECT COUNT(*) FROM local_author WHERE id = ?", author.ID).Scan(&authorCount).Error; err != nil {
		t.Fatalf("统计作者行失败: %v", err)
	}
	if authorCount != 0 {
		t.Fatalf("删除后 local_author 行应归零，实际 %d", authorCount)
	}

	// re_work_author 关联行归零
	var relCount int64
	if err := db.Raw("SELECT COUNT(*) FROM re_work_author WHERE local_author_id = ?", author.ID).Scan(&relCount).Error; err != nil {
		t.Fatalf("统计关联行失败: %v", err)
	}
	if relCount != 0 {
		t.Fatalf("删除后 re_work_author 行应归零，实际 %d", relCount)
	}

	// site_author 行保留且绑定列为 NULL
	var saCount int64
	if err := db.Raw("SELECT COUNT(*) FROM site_author WHERE id = ?", siteAu.ID).Scan(&saCount).Error; err != nil {
		t.Fatalf("统计站点作者行失败: %v", err)
	}
	if saCount != 1 {
		t.Fatalf("site_author 行应保留，实际 %d 行", saCount)
	}
	var binding sql.NullInt64
	if err := db.Raw("SELECT local_author_id FROM site_author WHERE id = ?", siteAu.ID).Scan(&binding).Error; err != nil {
		t.Fatalf("查站点绑定列失败: %v", err)
	}
	if binding.Valid {
		t.Fatalf("site_author 绑定列应置 NULL，实际 %d", binding.Int64)
	}

	// work 镜像列置 NULL——软删行镜像列同样置 NULL（原生 SQL 直查，不受软删 scope 影响）
	for _, workId := range []int64{liveWork.ID, deadWork.ID} {
		var mirror sql.NullInt64
		if err := db.Raw("SELECT local_author_id FROM work WHERE id = ?", workId).Scan(&mirror).Error; err != nil {
			t.Fatalf("查作品 %d 镜像列失败: %v", workId, err)
		}
		if mirror.Valid {
			t.Fatalf("作品 %d 的镜像列应置 NULL，实际 %d", workId, mirror.Int64)
		}
	}
}
