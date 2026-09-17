package work

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/reWorkAuthor"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"

	"gorm.io/gorm"
)

// 作者关联用户手动挂联链路锚定：LinkBatchToWork 建行 source=MANUAL + role_name 落库、
// 重拉窄域重建不动用户来源关联、插件再声明用户已手动挂的作者不翻转来源不重复建行、
// Unlink 摘除用户来源行。

// newManualAuthorLinker 用户手动挂联的真实 reWorkAuthor 服务
func newManualAuthorLinker(db *gorm.DB) *reWorkAuthor.Service {
	return reWorkAuthor.NewService(reWorkAuthor.NewRepository(db))
}

// seedSiteAuthorRow 直插站点作者行（不走入库链——手动挂联的目标行须先于插件声明存在）
func seedSiteAuthorRow(t *testing.T, db *gorm.DB, siteId int64, siteAuthorId string) *entity2.SiteAuthor {
	t.Helper()
	sa := entity2.NewSiteAuthor()
	sa.SiteID = sql.NullInt64{Int64: siteId, Valid: true}
	sa.SiteAuthorID = sql.NullString{String: siteAuthorId, Valid: true}
	sa.AuthorName = sql.NullString{String: siteAuthorId + "-名", Valid: true}
	if err := db.Create(sa).Error; err != nil {
		t.Fatalf("插站点作者行失败 %s: %v", siteAuthorId, err)
	}
	return sa
}

// seedLocalAuthorRow 直插本地作者行（手动挂联的目标行）
func seedLocalAuthorRow(t *testing.T, db *gorm.DB, name string) *entity2.LocalAuthor {
	t.Helper()
	la := entity2.NewLocalAuthor()
	la.AuthorName = sql.NullString{String: name, Valid: true}
	if err := db.Create(la).Error; err != nil {
		t.Fatalf("插本地作者行失败 %s: %v", name, err)
	}
	return la
}

// TestManualAuthorLinkCreatesManualRowWithRole 手动挂联建行：SITE 关联携带 roleNames 落 role_name、
// LOCAL 关联空 roleNames 落 NULL、单条挂联同语义；roleNames 与 authorIds 长度不匹配报错不落盘。
func TestManualAuthorLinkCreatesManualRowWithRole(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	siteWorkId := "manual-author-link-work"
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
	})
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	siteAuthor := seedSiteAuthorRow(t, db, pixiv.GetID(), "px-a1")
	localAuthor := seedLocalAuthorRow(t, db, "本地作者甲")

	linker := newManualAuthorLinker(db)
	if err := linker.LinkBatchToWork(context.Background(), workId, constant.SITE, []int64{siteAuthor.GetID()}, []string{"原作"}); err != nil {
		t.Fatalf("手动挂 SITE 作者失败: %v", err)
	}
	if err := linker.LinkBatchToWork(context.Background(), workId, constant.LOCAL, []int64{localAuthor.GetID()}, nil); err != nil {
		t.Fatalf("手动挂 LOCAL 作者失败: %v", err)
	}

	var siteRel entity2.ReWorkAuthor
	if err := db.Where("work_id = ? AND site_author_id = ?", workId, siteAuthor.GetID()).First(&siteRel).Error; err != nil {
		t.Fatalf("回查 SITE 作者关联失败: %v", err)
	}
	if siteRel.Source != constant.MANUAL {
		t.Fatalf("手动挂的 SITE 作者关联 source 应为 MANUAL，实际 %d", siteRel.Source)
	}
	if !siteRel.RoleName.Valid || siteRel.RoleName.String != "原作" {
		t.Fatalf("SITE 作者关联 role_name 应为 原作，实际 Valid=%v value=%q", siteRel.RoleName.Valid, siteRel.RoleName.String)
	}
	var localRel entity2.ReWorkAuthor
	if err := db.Where("work_id = ? AND local_author_id = ?", workId, localAuthor.GetID()).First(&localRel).Error; err != nil {
		t.Fatalf("回查 LOCAL 作者关联失败: %v", err)
	}
	if localRel.Source != constant.MANUAL {
		t.Fatalf("手动挂的 LOCAL 作者关联 source 应为 MANUAL，实际 %d", localRel.Source)
	}
	if localRel.RoleName.Valid {
		t.Fatalf("空 roleNames 挂联的 LOCAL 关联 role_name 应为 NULL，实际 %q", localRel.RoleName.String)
	}

	// 长度不匹配：报错且不落盘
	another := seedSiteAuthorRow(t, db, pixiv.GetID(), "px-a2")
	if err := linker.LinkBatchToWork(context.Background(), workId, constant.SITE, []int64{siteAuthor.GetID(), another.GetID()}, []string{"仅一个"}); err != reWorkAuthor.ErrRoleNameCountMismatch {
		t.Fatalf("期望 ErrRoleNameCountMismatch，得到 %v", err)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND site_author_id = ?", workId, another.GetID()); n != 0 {
		t.Fatalf("长度不匹配不应落盘，实际 %d 条", n)
	}
}

// TestRepullKeepsManualAuthorLinks 重拉窄域重建（作者轨）：插件未声明的 SITE 作者关联清理、
// 声明的重建 source=PLUGIN、用户手动挂的关联不动。
func TestRepullKeepsManualAuthorLinks(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	manualAuthor := seedSiteAuthorRow(t, db, pixiv.GetID(), "px-a3")
	siteWorkId := "repull-manual-author-work"
	fullPull := &sdkdto.WorkResponse{
		Work:        &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{{SiteAuthorId: "px-a1", AuthorName: "作者1"}, {SiteAuthorId: "px-a2", AuthorName: "作者2"}},
	}
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, fullPull)
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	linker := newManualAuthorLinker(db)
	if err := linker.LinkBatchToWork(context.Background(), workId, constant.SITE, []int64{manualAuthor.GetID()}, []string{"赞助"}); err != nil {
		t.Fatalf("手动挂 SITE 作者失败: %v", err)
	}

	// 重拉：作者 px-a2 不再声明
	partialPull := &sdkdto.WorkResponse{
		Work:        &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{{SiteAuthorId: "px-a1", AuthorName: "作者1"}},
	}
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, partialPull); err != nil {
		t.Fatalf("重拉（部分声明）失败: %v", err)
	}

	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND author_type = ?", workId, constant.SITE); n != 2 {
		t.Fatalf("SITE 作者关联应剩 2 条（声明 px-a1 + 手动 px-a3），实际 %d", n)
	}
	a1 := seededAuthorDbIdBySiteKey(t, db, pixiv.GetID(), "px-a1")
	a2 := seededAuthorDbIdBySiteKey(t, db, pixiv.GetID(), "px-a2")
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND site_author_id = ?", workId, a2); n != 0 {
		t.Fatalf("未声明的 px-a2 关联应被清理，实际残留 %d 条", n)
	}
	var pluginRel entity2.ReWorkAuthor
	if err := db.Where("work_id = ? AND site_author_id = ?", workId, a1).First(&pluginRel).Error; err != nil {
		t.Fatalf("回查声明的 px-a1 关联失败: %v", err)
	}
	if pluginRel.Source != constant.PLUGIN {
		t.Fatalf("声明的 px-a1 关联 source 应为 PLUGIN，实际 %d", pluginRel.Source)
	}
	var manualRel entity2.ReWorkAuthor
	if err := db.Where("work_id = ? AND site_author_id = ?", workId, manualAuthor.GetID()).First(&manualRel).Error; err != nil {
		t.Fatalf("回查手动 px-a3 关联失败: %v", err)
	}
	if manualRel.Source != constant.MANUAL {
		t.Fatalf("手动挂的 px-a3 关联 source 应为 MANUAL，实际 %d", manualRel.Source)
	}
	if !manualRel.RoleName.Valid || manualRel.RoleName.String != "赞助" {
		t.Fatalf("手动挂的 px-a3 关联 role_name 应保留 赞助，实际 Valid=%v value=%q", manualRel.RoleName.Valid, manualRel.RoleName.String)
	}
}

// seedAuthorDbIdBySiteKey 按站点侧 ID 查站点作者行 DB ID（断言辅助；名带 seed 前缀区别于
// 直插助手，兼容入库链 upsert 建出的行）
func seededAuthorDbIdBySiteKey(t *testing.T, db *gorm.DB, siteId int64, siteAuthorId string) int64 {
	t.Helper()
	var sa entity2.SiteAuthor
	if err := db.Where("site_id = ? AND site_author_id = ?", siteId, siteAuthorId).First(&sa).Error; err != nil {
		t.Fatalf("查站点作者行失败 %s: %v", siteAuthorId, err)
	}
	return sa.GetID()
}

// TestRepullPluginRedeclareManualAuthorNoFlipNoDup 插件再声明用户已手动挂的作者：
// 重拉链不翻转来源（保持 MANUAL）、不重复建行；仓储 UpsertBatch 直击——冲突 DoUpdates
// 刷用户可编辑字段（role_name/sort_order）但不含 source，插件来源行不翻转手动行的来源。
func TestRepullPluginRedeclareManualAuthorNoFlipNoDup(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	manualAuthor := seedSiteAuthorRow(t, db, pixiv.GetID(), "px-a1")
	siteWorkId := "repull-author-conflict-work"
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
	})
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	// 用户先手动挂作者（此时插件未声明过）：行落 source=MANUAL + role_name
	linker := newManualAuthorLinker(db)
	if err := linker.LinkBatchToWork(context.Background(), workId, constant.SITE, []int64{manualAuthor.GetID()}, []string{"原作"}); err != nil {
		t.Fatalf("手动挂 SITE 作者失败: %v", err)
	}

	// 重拉：插件再声明同一作者
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, &sdkdto.WorkResponse{
		Work:        &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{{SiteAuthorId: "px-a1", AuthorName: "作者名"}},
	}); err != nil {
		t.Fatalf("重拉（再声明手动关联的作者）失败: %v", err)
	}

	var rel entity2.ReWorkAuthor
	if err := db.Where("work_id = ? AND site_author_id = ?", workId, manualAuthor.GetID()).First(&rel).Error; err != nil {
		t.Fatalf("回查冲突关联失败: %v", err)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND site_author_id = ?", workId, manualAuthor.GetID()); n != 1 {
		t.Fatalf("插件再声明手动关联的作者不应重复建行，实际 %d 条", n)
	}
	if rel.Source != constant.MANUAL {
		t.Fatalf("插件再声明不应翻转来源，应保持 MANUAL，实际 %d", rel.Source)
	}
	if !rel.RoleName.Valid || rel.RoleName.String != "原作" {
		t.Fatalf("插件再声明不应覆盖用户角色，实际 Valid=%v value=%q", rel.RoleName.Valid, rel.RoleName.String)
	}

	// 仓储直击：upsert 一条插件来源行（role/sort 带值），冲突只刷用户可编辑字段、不写 source
	pluginRel := entity2.NewReWorkAuthor()
	pluginRel.WorkID = sql.NullInt64{Int64: workId, Valid: true}
	pluginRel.AuthorType = sql.NullInt64{Int64: constant.SITE, Valid: true}
	pluginRel.SiteAuthorID = sql.NullInt64{Int64: manualAuthor.GetID(), Valid: true}
	pluginRel.RoleName = sql.NullString{String: "插旗角色", Valid: true}
	pluginRel.SortOrder = sql.NullInt64{Int64: 7, Valid: true}
	pluginRel.Source = constant.PLUGIN
	repo := reWorkAuthor.NewRepository(db)
	if err := repo.UpsertBatch(context.Background(), []*entity2.ReWorkAuthor{pluginRel}, constant.SITE); err != nil {
		t.Fatalf("仓储 UpsertBatch 直击失败: %v", err)
	}
	var after entity2.ReWorkAuthor
	if err := db.Where("work_id = ? AND site_author_id = ?", workId, manualAuthor.GetID()).First(&after).Error; err != nil {
		t.Fatalf("回查直击后关联失败: %v", err)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND site_author_id = ?", workId, manualAuthor.GetID()); n != 1 {
		t.Fatalf("UpsertBatch 冲突不应重复建行，实际 %d 条", n)
	}
	if after.Source != constant.MANUAL {
		t.Fatalf("UpsertBatch 冲突不应翻转来源，应保持 MANUAL，实际 %d", after.Source)
	}
	if !after.RoleName.Valid || after.RoleName.String != "插旗角色" {
		t.Fatalf("UpsertBatch 冲突应刷新 role_name，实际 Valid=%v value=%q", after.RoleName.Valid, after.RoleName.String)
	}
	if !after.SortOrder.Valid || after.SortOrder.Int64 != 7 {
		t.Fatalf("UpsertBatch 冲突应刷新 sort_order，实际 Valid=%v value=%d", after.SortOrder.Valid, after.SortOrder.Int64)
	}
}

// TestUnlinkAuthorRemovesManualRow Unlink 摘除：批量摘手动行、单条摘除均正常，未涉关联不动。
func TestUnlinkAuthorRemovesManualRow(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	manualAuthor := seedSiteAuthorRow(t, db, pixiv.GetID(), "px-a2")
	siteWorkId := "unlink-manual-author-work"
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, &sdkdto.WorkResponse{
		Work:        &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{{SiteAuthorId: "px-a1", AuthorName: "作者1"}},
	})
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	linker := newManualAuthorLinker(db)
	if err := linker.LinkBatchToWork(context.Background(), workId, constant.SITE, []int64{manualAuthor.GetID()}, nil); err != nil {
		t.Fatalf("手动挂 SITE 作者失败: %v", err)
	}

	// 批量摘除手动行：插件声明的 px-a1 不受影响
	if err := linker.RemoveBatchFromWork(context.Background(), workId, constant.SITE, []int64{manualAuthor.GetID()}); err != nil {
		t.Fatalf("批量摘除失败: %v", err)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND site_author_id = ?", workId, manualAuthor.GetID()); n != 0 {
		t.Fatalf("摘除后手动关联应无行，实际 %d 条", n)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND author_type = ?", workId, constant.SITE); n != 1 {
		t.Fatalf("插件声明的 px-a1 关联应保留，实际 %d 条", n)
	}

	// 单条摘除：摘掉插件声明的 px-a1
	a1 := seededAuthorDbIdBySiteKey(t, db, pixiv.GetID(), "px-a1")
	if err := linker.UnlinkAuthorFromWork(context.Background(), workId, constant.SITE, a1); err != nil {
		t.Fatalf("单条摘除失败: %v", err)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND author_type = ?", workId, constant.SITE); n != 0 {
		t.Fatalf("单条摘除后应无 SITE 作者关联，实际 %d 条", n)
	}
}
