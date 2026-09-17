package reWorkTag

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/tagNamespace"

	"gorm.io/gorm"
)

// 手动挂联链的关联级 ns 维度行为锚定（DB 真实链：真仓储 + 真事务 + 真 ns 清单服务）。
// schema 层唯一索引行为（原始 SQL 直插）由 migration/dimension_schema_test 锚定，此处锚服务链。

// gormTransactor 测试事务执行器（与 app.go dbTransactorAdapter 同构：事务连接经 ctx 传递）
type gormTransactor struct{ db *gorm.DB }

func (t *gormTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, database.TxKey, tx))
	})
}

// newDimensionTestEnv 内存 FK 库 + 真实服务链
func newDimensionTestEnv(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	return NewService(NewRepository(db), &gormTransactor{db: db}, tagNamespace.NewService(tagNamespace.NewRepository(db))), db
}

func seedDimWork(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	w := entity.NewWork()
	w.SiteWorkName = sql.NullString{String: "维度测试作品", Valid: true}
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("插作品失败: %v", err)
	}
	return w.GetID()
}

func seedDimSiteTag(t *testing.T, db *gorm.DB) *entity.SiteTag {
	t.Helper()
	st := entity.NewSiteTag()
	st.SiteTagID = sql.NullString{String: "st-1", Valid: true}
	st.SiteTagName = sql.NullString{String: "站点标签", Valid: true}
	if err := db.Create(st).Error; err != nil {
		t.Fatalf("插站点标签失败: %v", err)
	}
	return st
}

func seedDimLocalTag(t *testing.T, db *gorm.DB) *entity.LocalTag {
	t.Helper()
	lt := entity.NewLocalTag()
	lt.LocalTagName = sql.NullString{String: "本地标签", Valid: true}
	if err := db.Create(lt).Error; err != nil {
		t.Fatalf("插本地标签失败: %v", err)
	}
	return lt
}

// TestManualLinkMultiNamespaceSiteAndLocalTracks 同作品同标签多 ns 并存（SITE 轨与 LOCAL 轨各一）：
// 两条不同 ns 关联落两行互不冲突；重复写同 (work, tag, ns) 收敛为一行（upsert 不重复建行）；
// 空串 ns 与具体 ns 并存、空串不产生第二行
func TestManualLinkMultiNamespaceSiteAndLocalTracks(t *testing.T) {
	svc, db := newDimensionTestEnv(t)
	workId := seedDimWork(t, db)
	siteTag := seedDimSiteTag(t, db)

	ctx := context.Background()
	// SITE 轨：同标签挂 female + male 两条 ns
	if err := svc.LinkBatchToWork(ctx, workId, constant.SITE, []int64{siteTag.GetID()}, []string{"female"}); err != nil {
		t.Fatalf("挂 female 失败: %v", err)
	}
	if err := svc.LinkBatchToWork(ctx, workId, constant.SITE, []int64{siteTag.GetID()}, []string{"male"}); err != nil {
		t.Fatalf("挂 male 失败: %v", err)
	}
	if n := countDimRows(t, db, "work_id = ? AND site_tag_id = ?", workId, siteTag.GetID()); n != 2 {
		t.Fatalf("SITE 轨同标签多 ns 应并存 2 行，实际 %d", n)
	}
	// 重复写同 (work, tag, ns) 收敛
	if err := svc.LinkBatchToWork(ctx, workId, constant.SITE, []int64{siteTag.GetID()}, []string{"female"}); err != nil {
		t.Fatalf("重复挂 female 失败: %v", err)
	}
	if n := countDimRows(t, db, "work_id = ? AND site_tag_id = ? AND namespace = ?", workId, siteTag.GetID(), "female"); n != 1 {
		t.Fatalf("重复 (work, tag, ns=female) 应收敛 1 行，实际 %d", n)
	}
	// 空串 ns 行独立存在、不重复
	if err := svc.LinkBatchToWork(ctx, workId, constant.SITE, []int64{siteTag.GetID()}, []string{""}); err != nil {
		t.Fatalf("挂空 ns 失败: %v", err)
	}
	if err := svc.LinkBatchToWork(ctx, workId, constant.SITE, []int64{siteTag.GetID()}, nil); err != nil {
		t.Fatalf("nil ns 数组再挂失败: %v", err)
	}
	if n := countDimRows(t, db, "work_id = ? AND site_tag_id = ? AND namespace = ''", workId, siteTag.GetID()); n != 1 {
		t.Fatalf("空串 ns 应恰 1 行，实际 %d", n)
	}

	// LOCAL 轨同型
	localTag := seedDimLocalTag(t, db)
	if err := svc.LinkBatchToWork(ctx, workId, constant.LOCAL, []int64{localTag.GetID()}, []string{"我的标记"}); err != nil {
		t.Fatalf("挂本地标记失败: %v", err)
	}
	if err := svc.LinkBatchToWork(ctx, workId, constant.LOCAL, []int64{localTag.GetID()}, []string{"另一标记"}); err != nil {
		t.Fatalf("挂另一本地标记失败: %v", err)
	}
	if n := countDimRows(t, db, "work_id = ? AND local_tag_id = ?", workId, localTag.GetID()); n != 2 {
		t.Fatalf("LOCAL 轨同标签多 ns 应并存 2 行，实际 %d", n)
	}
}

// TestManualLinkNormalizesNamespace 维度值归一化：Female / female␣ / female 归一为同一关联值与
// 同一清单行（不分裂为多行多候选）
func TestManualLinkNormalizesNamespace(t *testing.T) {
	svc, db := newDimensionTestEnv(t)
	workId := seedDimWork(t, db)
	siteTag := seedDimSiteTag(t, db)

	ctx := context.Background()
	for _, variant := range []string{"Female", "female ", "female"} {
		if err := svc.LinkBatchToWork(ctx, workId, constant.SITE, []int64{siteTag.GetID()}, []string{variant}); err != nil {
			t.Fatalf("挂 %q 失败: %v", variant, err)
		}
	}
	if n := countDimRows(t, db, "work_id = ? AND site_tag_id = ?", workId, siteTag.GetID()); n != 1 {
		t.Fatalf("大小写/空白变体应归一收敛 1 行，实际 %d", n)
	}
	var rel entity.ReWorkTag
	if err := db.Where("work_id = ? AND site_tag_id = ?", workId, siteTag.GetID()).First(&rel).Error; err != nil {
		t.Fatalf("回查关联失败: %v", err)
	}
	if rel.Namespace != "female" {
		t.Fatalf("关联 ns 应为归一化 female，实际 %q", rel.Namespace)
	}
	// 清单行同样归一收敛：female 恰一行
	var invRows []entity.TagNamespace
	if err := db.Where("value = ?", "female").Find(&invRows).Error; err != nil {
		t.Fatalf("回查清单失败: %v", err)
	}
	if len(invRows) != 1 {
		t.Fatalf("清单 female 应恰 1 行，实际 %d", len(invRows))
	}
}

// TestManualLinkRegistersUserInventory 手动挂联登记 ns 清单：自定义值 find-or-create（origin=user）、
// label 空（显示回落 value）；内置值行（origin=builtin）不被降级、label 不改写、last_use 刷新
func TestManualLinkRegistersUserInventory(t *testing.T) {
	svc, db := newDimensionTestEnv(t)
	workId := seedDimWork(t, db)
	siteTag := seedDimSiteTag(t, db)

	// 预置内置行（模拟启动投影）并注入较早 last_use
	builtin := entity.NewTagNamespace()
	builtin.Value = "character"
	builtin.Label = "角色"
	builtin.Origin = constant.ORIGIN_BUILTIN
	builtin.LastUse = 1000
	if err := db.Create(builtin).Error; err != nil {
		t.Fatalf("预置内置清单行失败: %v", err)
	}

	ctx := context.Background()
	if err := svc.LinkBatchToWork(ctx, workId, constant.SITE,
		[]int64{siteTag.GetID(), siteTag.GetID()}, []string{"我的分类", "character"}); err != nil {
		t.Fatalf("手动挂联失败: %v", err)
	}

	var custom entity.TagNamespace
	if err := db.Where("value = ?", "我的分类").First(&custom).Error; err != nil {
		t.Fatalf("自定义 ns 应 find-or-create 清单行: %v", err)
	}
	if custom.Origin != constant.ORIGIN_USER {
		t.Fatalf("手动链登记的清单行 origin 应为 user，实际 %d", custom.Origin)
	}
	if custom.Label != "" {
		t.Fatalf("find-or-create 行 label 应为空（显示回落 value），实际 %q", custom.Label)
	}
	if custom.LastUse == 0 {
		t.Fatal("清单行 last_use 应被刷新")
	}

	var builtinAfter entity.TagNamespace
	if err := db.Where("value = ?", "character").First(&builtinAfter).Error; err != nil {
		t.Fatalf("回查内置行失败: %v", err)
	}
	if builtinAfter.Origin != constant.ORIGIN_BUILTIN {
		t.Fatalf("内置行 origin 不应被改写，实际 %d", builtinAfter.Origin)
	}
	if builtinAfter.Label != "角色" {
		t.Fatalf("内置行 label 不应被改写，实际 %q", builtinAfter.Label)
	}
	if builtinAfter.LastUse <= 1000 {
		t.Fatalf("内置行 last_use 应照常刷新，实际 %d", builtinAfter.LastUse)
	}
}

// TestRemoveDimensionFromWorkPreciseUnlink 精确维度摘除：只删命中 (work, tag, ns) 行，不波及
// 同标签其他 ns 行（改 ns 的旧值行删除语义）；粗粒度 RemoveBatchFromWork 删该标签全部 ns 行
func TestRemoveDimensionFromWorkPreciseUnlink(t *testing.T) {
	svc, db := newDimensionTestEnv(t)
	workId := seedDimWork(t, db)
	siteTag := seedDimSiteTag(t, db)

	ctx := context.Background()
	if err := svc.LinkBatchToWork(ctx, workId, constant.SITE,
		[]int64{siteTag.GetID(), siteTag.GetID(), siteTag.GetID()},
		[]string{"female", "male", ""}); err != nil {
		t.Fatalf("挂三条 ns 关联失败: %v", err)
	}

	// 精确摘除 female：male 与空串行保留
	if err := svc.RemoveDimensionFromWork(ctx, workId, constant.SITE, []int64{siteTag.GetID()}, []string{"Female"}); err != nil {
		t.Fatalf("精确摘除失败: %v", err)
	}
	if n := countDimRows(t, db, "work_id = ? AND site_tag_id = ?", workId, siteTag.GetID()); n != 2 {
		t.Fatalf("精确摘除后应剩 2 行（male + 空串），实际 %d", n)
	}
	if n := countDimRows(t, db, "work_id = ? AND site_tag_id = ? AND namespace = ?", workId, siteTag.GetID(), "female"); n != 0 {
		t.Fatalf("female 行应被摘除，实际 %d", n)
	}

	// 粗粒度移除：全部 ns 行删除
	if err := svc.RemoveBatchFromWork(ctx, workId, constant.SITE, []int64{siteTag.GetID()}); err != nil {
		t.Fatalf("粗粒度移除失败: %v", err)
	}
	if n := countDimRows(t, db, "work_id = ? AND site_tag_id = ?", workId, siteTag.GetID()); n != 0 {
		t.Fatalf("粗粒度移除后应 0 行，实际 %d", n)
	}
}

// countDimRows 条件计数（re_work_tag）
func countDimRows(t *testing.T, db *gorm.DB, cond string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&entity.ReWorkTag{}).Where(cond, args...).Count(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	return n
}
