package work

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/reWorkTag"
	"github.com/library-squirrel/backend/reWorkWorkSet"
	"github.com/library-squirrel/backend/siteTag"
	"github.com/library-squirrel/backend/workSet"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"

	"gorm.io/gorm"
)

// 关联来源（source 列）窄域重建语义锚定：插件来源关联按本次声明重建（未声明即清理）、用户来源关联
// （手动挂联）重拉永不触碰、冲突（插件再声明用户已手动挂的关联）不翻转来源不重复建行。

// newManualTagLinker 用户手动挂联的真实 reWorkTag 服务（SITE 关联 namespace 镜像需 siteTag 查询件）
func newManualTagLinker(db *gorm.DB) *reWorkTag.Service {
	return reWorkTag.NewService(reWorkTag.NewRepository(db), siteTag.NewService(siteTag.NewRepository(db), nil, nil, nil, nil, nil))
}

// newManualWorkSetLinker 用户手动挂联的真实 workSet 服务（Link 方法仅用作品集成员关联仓储）
func newManualWorkSetLinker(db *gorm.DB) *workSet.Service {
	return workSet.NewService(workSet.NewRepository(db), reWorkWorkSet.NewRepository(db), nil, nil, nil, nil)
}

// seedSiteTagRow 直插站点标签行（不走入库链——手动挂联的目标行须先于插件声明存在）
func seedSiteTagRow(t *testing.T, db *gorm.DB, siteId int64, siteTagId, namespace string) *entity2.SiteTag {
	t.Helper()
	st := entity2.NewSiteTag()
	st.SiteID = sql.NullInt64{Int64: siteId, Valid: true}
	st.SiteTagID = sql.NullString{String: siteTagId, Valid: true}
	st.SiteTagName = sql.NullString{String: siteTagId + "-名", Valid: true}
	st.Namespace = sql.NullString{String: namespace, Valid: namespace != ""}
	if err := db.Create(st).Error; err != nil {
		t.Fatalf("插站点标签行失败 %s: %v", siteTagId, err)
	}
	return st
}

// seedWorkSetRow 直插作品集行（手动挂联的目标行）
func seedWorkSetRow(t *testing.T, db *gorm.DB, siteId int64, siteWorkSetId string) *entity2.WorkSet {
	t.Helper()
	ws := entity2.NewWorkSet()
	ws.SiteID = sql.NullInt64{Int64: siteId, Valid: true}
	ws.SiteWorkSetID = sql.NullString{String: siteWorkSetId, Valid: true}
	ws.SiteWorkSetName = sql.NullString{String: siteWorkSetId + "-名", Valid: true}
	if err := db.Create(ws).Error; err != nil {
		t.Fatalf("插作品集行失败 %s: %v", siteWorkSetId, err)
	}
	return ws
}

// seedLocalTagRow 直插本地标签行（手动挂联的目标行）
func seedLocalTagRow(t *testing.T, db *gorm.DB, name string) *entity2.LocalTag {
	t.Helper()
	lt := entity2.NewLocalTag()
	lt.LocalTagName = sql.NullString{String: name, Valid: true}
	if err := db.Create(lt).Error; err != nil {
		t.Fatalf("插本地标签行失败 %s: %v", name, err)
	}
	return lt
}

// TestRepullRefreshesTagNamespaceMirror 重拉时标签 namespace 变化：site_tag.namespace 与插件来源
// SITE 关联的 re_work_tag.namespace 镜像同步刷新（插件权威字段，插入期与重拉期同权）
func TestRepullRefreshesTagNamespaceMirror(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	siteWorkId := "repull-ns-mirror-work"
	pull := func(ns string) *sdkdto.WorkResponse {
		return &sdkdto.WorkResponse{
			Work:     &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
			SiteTags: []*sdkdto.TaskSiteTagDTO{{SiteTagId: "px-t1", TagName: "标签名", Namespace: ns}},
		}
	}
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, pull("character"))
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	var tag entity2.SiteTag
	if err := db.Where("site_tag_id = ?", "px-t1").First(&tag).Error; err != nil {
		t.Fatalf("回查 site_tag 失败: %v", err)
	}
	if tag.Namespace.String != "character" {
		t.Fatalf("首插 site_tag.namespace 应为 character，实际 %q", tag.Namespace.String)
	}

	// 重拉：namespace 声明变化为 male
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, pull("male")); err != nil {
		t.Fatalf("重拉入库失败: %v", err)
	}

	if err := db.Where("site_tag_id = ?", "px-t1").First(&tag).Error; err != nil {
		t.Fatalf("重拉后回查 site_tag 失败: %v", err)
	}
	if tag.Namespace.String != "male" {
		t.Fatalf("重拉应刷新 site_tag.namespace 为 male，实际 %q", tag.Namespace.String)
	}
	var rel entity2.ReWorkTag
	if err := db.Where("work_id = ? AND tag_type = ?", workId, constant.SITE).First(&rel).Error; err != nil {
		t.Fatalf("回查 SITE 标签关联失败: %v", err)
	}
	if !rel.Namespace.Valid || rel.Namespace.String != "male" {
		t.Fatalf("重拉应刷新插件来源 SITE 关联 namespace 镜像为 male，实际 Valid=%v value=%q", rel.Namespace.Valid, rel.Namespace.String)
	}
	if rel.Source != constant.PLUGIN {
		t.Fatalf("入库链建的 SITE 标签关联 source 应为 PLUGIN，实际 %d", rel.Source)
	}
}

// TestRepullKeepsManualSiteLinkAndTidiesCrossSiteRef 用户手动挂 SITE 关联 + 插件挂跨站引用：
// 重拉后用户关联保留；跨站引用本次仍声明则保留、不再声明则清理（插件自己的跨站声明史归插件管辖）
func TestRepullKeepsManualSiteLinkAndTidiesCrossSiteRef(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")
	bili := seedSite(t, db, "bilibili")

	biliTag := seedSiteTagRow(t, db, bili.GetID(), "bili-t1", "")
	manualTag := seedSiteTagRow(t, db, pixiv.GetID(), "px-t2", "")

	siteWorkId := "repull-manual-cross-work"
	buildResp := func(cross bool) *sdkdto.WorkResponse {
		r := &sdkdto.WorkResponse{
			Work:     &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
			SiteTags: []*sdkdto.TaskSiteTagDTO{{SiteTagId: "px-t1", TagName: "本站标签"}},
		}
		if cross {
			r.SiteTags = append(r.SiteTags, &sdkdto.TaskSiteTagDTO{SiteKey: "bilibili", SiteTagId: "bili-t1", TagName: "跨站引用"})
		}
		return r
	}
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, buildResp(true))
	if err != nil {
		t.Fatalf("首次入库（含跨站引用）失败: %v", err)
	}

	// 用户手动挂 SITE 关联（真实手动链）
	linker := newManualTagLinker(db)
	if err := linker.LinkBatchToWork(context.Background(), workId, constant.SITE, []int64{manualTag.GetID()}, nil); err != nil {
		t.Fatalf("手动挂 SITE 关联失败: %v", err)
	}

	// 重拉一：跨站引用仍声明 → 保留；手动关联不受窄域重建影响
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, buildResp(true)); err != nil {
		t.Fatalf("重拉（仍声明跨站）失败: %v", err)
	}
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND site_tag_id = ?", workId, biliTag.GetID()); n != 1 {
		t.Fatalf("仍声明的跨站引用应保留 1 条，实际 %d", n)
	}
	var manualRel entity2.ReWorkTag
	if err := db.Where("work_id = ? AND site_tag_id = ?", workId, manualTag.GetID()).First(&manualRel).Error; err != nil {
		t.Fatalf("回查手动 SITE 关联失败: %v", err)
	}
	if manualRel.Source != constant.MANUAL {
		t.Fatalf("手动挂的 SITE 关联 source 应为 MANUAL，实际 %d", manualRel.Source)
	}

	// 重拉二：跨站引用不再声明 → 清理；手动关联仍在
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, buildResp(false)); err != nil {
		t.Fatalf("重拉（不再声明跨站）失败: %v", err)
	}
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND site_tag_id = ?", workId, biliTag.GetID()); n != 0 {
		t.Fatalf("不再声明的跨站引用应清理，实际残留 %d 条", n)
	}
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND site_tag_id = ?", workId, manualTag.GetID()); n != 1 {
		t.Fatalf("用户手动 SITE 关联应保留，实际 %d 条", n)
	}
}

// TestRepullCleansUndeclaredPluginLinksKeepsManual 站点移除标签/移出合集（插件本次不再声明）：
// 插件来源关联被清理、用户来源关联不动（窄域重建核心锚）
func TestRepullCleansUndeclaredPluginLinksKeepsManual(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	manualTag := seedSiteTagRow(t, db, pixiv.GetID(), "px-t3", "")
	manualWs := seedWorkSetRow(t, db, pixiv.GetID(), "px-ws3")

	siteWorkId := "repull-narrow-rebuild-work"
	fullPull := &sdkdto.WorkResponse{
		Work:        &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{{SiteAuthorId: "px-a1", AuthorName: "作者1"}, {SiteAuthorId: "px-a2", AuthorName: "作者2"}},
		SiteTags:    []*sdkdto.TaskSiteTagDTO{{SiteTagId: "px-t1", TagName: "标签1"}, {SiteTagId: "px-t2", TagName: "标签2"}},
		WorkSets:    []*sdkdto.TaskWorkSetDTO{{SiteWorkSetId: "px-ws1", WorkSetName: "集1"}, {SiteWorkSetId: "px-ws2", WorkSetName: "集2"}},
	}
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, fullPull)
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	// 用户手动挂联：SITE 标签 px-t3 + 作品集成员 px-ws3
	tagLinker := newManualTagLinker(db)
	if err := tagLinker.LinkBatchToWork(context.Background(), workId, constant.SITE, []int64{manualTag.GetID()}, nil); err != nil {
		t.Fatalf("手动挂 SITE 标签失败: %v", err)
	}
	wsLinker := newManualWorkSetLinker(db)
	if err := wsLinker.LinkWorkToWorkSet(context.Background(), workId, manualWs.GetID()); err != nil {
		t.Fatalf("手动挂作品集成员失败: %v", err)
	}

	// 重拉：站点移除标签 px-t2、移出合集 px-ws2、作者 px-a2 不再声明
	partialPull := &sdkdto.WorkResponse{
		Work:        &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{{SiteAuthorId: "px-a1", AuthorName: "作者1"}},
		SiteTags:    []*sdkdto.TaskSiteTagDTO{{SiteTagId: "px-t1", TagName: "标签1"}},
		WorkSets:    []*sdkdto.TaskWorkSetDTO{{SiteWorkSetId: "px-ws1", WorkSetName: "集1"}},
	}
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, partialPull); err != nil {
		t.Fatalf("重拉（部分声明）失败: %v", err)
	}

	// 标签：未声明的 px-t2 清理、声明的 px-t1 与手动的 px-t3 保留
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND tag_type = ?", workId, constant.SITE); n != 2 {
		t.Fatalf("SITE 标签关联应剩 2 条（声明 px-t1 + 手动 px-t3），实际 %d", n)
	}
	if n := countRows(t, db, &entity2.SiteTag{}, "site_tag_id = ?", "px-t2"); n != 1 {
		t.Fatalf("站点标签行本身不应被删（只清关联），实际 %d 行", n)
	}
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND site_tag_id = ?", workId, manualTag.GetID()); n != 1 {
		t.Fatalf("手动挂的 SITE 标签关联应保留，实际 %d 条", n)
	}
	// 作者：未声明的 px-a2 清理
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND author_type = ?", workId, constant.SITE); n != 1 {
		t.Fatalf("SITE 作者关联应剩 1 条（仅声明 px-a1），实际 %d", n)
	}
	// 作品集成员：未声明的 px-ws2 清理、声明的 px-ws1 与手动的 px-ws3 保留
	ws1DbId := workSetDbIdBySiteKey(t, db, pixiv.GetID(), "px-ws1")
	ws2DbId := workSetDbIdBySiteKey(t, db, pixiv.GetID(), "px-ws2")
	var ws1, ws3 entity2.ReWorkWorkSet
	if err := db.Where("work_id = ? AND work_set_id = ?", workId, ws1DbId).First(&ws1).Error; err != nil {
		t.Fatalf("回查 px-ws1 关联失败: %v", err)
	}
	if n := countRows(t, db, &entity2.ReWorkWorkSet{}, "work_id = ? AND work_set_id = ?", workId, ws2DbId); n != 0 {
		t.Fatalf("未声明的 px-ws2 关联应被清理，实际残留 %d 条", n)
	}
	if err := db.Where("work_id = ? AND work_set_id = ?", workId, manualWs.GetID()).First(&ws3).Error; err != nil {
		t.Fatalf("回查手动 px-ws3 关联失败: %v", err)
	}
	if ws1.Source != constant.PLUGIN {
		t.Fatalf("声明的 px-ws1 关联 source 应为 PLUGIN，实际 %d", ws1.Source)
	}
	if ws3.Source != constant.MANUAL {
		t.Fatalf("手动挂的 px-ws3 关联 source 应为 MANUAL，实际 %d", ws3.Source)
	}
}

// workSetDbIdBySiteKey 按站点侧 ID 查作品集行 DB ID（断言辅助）
func workSetDbIdBySiteKey(t *testing.T, db *gorm.DB, siteId int64, siteWorkSetId string) int64 {
	t.Helper()
	var ws entity2.WorkSet
	if err := db.Where("site_id = ? AND site_work_set_id = ?", siteId, siteWorkSetId).First(&ws).Error; err != nil {
		t.Fatalf("查作品集行失败 %s: %v", siteWorkSetId, err)
	}
	return ws.GetID()
}

// TestRepullPluginRedeclareManualLinkNoFlipNoDup 插件再声明用户已手动挂的关联：
// 不翻转来源（保持 MANUAL）、不重复建行、namespace 刷镜像、sort_order 不动
func TestRepullPluginRedeclareManualLinkNoFlipNoDup(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	siteWorkId := "repull-conflict-work"
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
	})
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	// 用户先手动挂 SITE 标签（此时插件未声明过）：行落 source=MANUAL，namespace 镜像挂联时的 site_tag.namespace
	manualTag := seedSiteTagRow(t, db, pixiv.GetID(), "px-t1", "character")
	tagLinker := newManualTagLinker(db)
	if err := tagLinker.LinkBatchToWork(context.Background(), workId, constant.SITE, []int64{manualTag.GetID()}, nil); err != nil {
		t.Fatalf("手动挂 SITE 标签失败: %v", err)
	}
	var manualRel entity2.ReWorkTag
	if err := db.Where("work_id = ? AND site_tag_id = ?", workId, manualTag.GetID()).First(&manualRel).Error; err != nil {
		t.Fatalf("回查手动关联失败: %v", err)
	}
	if !manualRel.Namespace.Valid || manualRel.Namespace.String != "character" {
		t.Fatalf("手动挂联时应镜像 site_tag.namespace=character，实际 %q", manualRel.Namespace.String)
	}

	// 用户手动挂作品集成员并拖出用户序 42（sort_order 属用户策展）
	manualWs := seedWorkSetRow(t, db, pixiv.GetID(), "px-ws1")
	wsLinker := newManualWorkSetLinker(db)
	if err := wsLinker.LinkWorkToWorkSet(context.Background(), workId, manualWs.GetID()); err != nil {
		t.Fatalf("手动挂作品集成员失败: %v", err)
	}
	if err := db.Model(&entity2.ReWorkWorkSet{}).Where("work_id = ? AND work_set_id = ?", workId, manualWs.GetID()).
		Update("sort_order", 42).Error; err != nil {
		t.Fatalf("注入用户拖拽序失败: %v", err)
	}

	// 重拉：插件再声明同一标签（namespace 已变化）与同一作品集成员
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, &sdkdto.WorkResponse{
		Work:     &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteTags: []*sdkdto.TaskSiteTagDTO{{SiteTagId: "px-t1", TagName: "标签名", Namespace: "male"}},
		WorkSets: []*sdkdto.TaskWorkSetDTO{{SiteWorkSetId: "px-ws1", WorkSetName: "集名"}},
	}); err != nil {
		t.Fatalf("重拉（再声明手动关联）失败: %v", err)
	}

	// 标签：单行、不翻转来源、namespace 刷镜像
	var rel entity2.ReWorkTag
	if err := db.Where("work_id = ? AND site_tag_id = ?", workId, manualTag.GetID()).First(&rel).Error; err != nil {
		t.Fatalf("回查冲突关联失败: %v", err)
	}
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND site_tag_id = ?", workId, manualTag.GetID()); n != 1 {
		t.Fatalf("插件再声明手动关联不应重复建行，实际 %d 条", n)
	}
	if rel.Source != constant.MANUAL {
		t.Fatalf("插件再声明不应翻转来源，应保持 MANUAL，实际 %d", rel.Source)
	}
	if !rel.Namespace.Valid || rel.Namespace.String != "male" {
		t.Fatalf("冲突应刷新 namespace 镜像为 male，实际 Valid=%v value=%q", rel.Namespace.Valid, rel.Namespace.String)
	}
	// 作品集成员：单行、不翻转来源、sort_order 不动
	var wsRel entity2.ReWorkWorkSet
	if err := db.Where("work_id = ? AND work_set_id = ?", workId, manualWs.GetID()).First(&wsRel).Error; err != nil {
		t.Fatalf("回查冲突作品集关联失败: %v", err)
	}
	if wsRel.Source != constant.MANUAL {
		t.Fatalf("插件再声明不应翻转作品集关联来源，应保持 MANUAL，实际 %d", wsRel.Source)
	}
	if !wsRel.SortOrder.Valid || wsRel.SortOrder.Int64 != 42 {
		t.Fatalf("插件再声明不应动用户拖拽序 sort_order，实际 Valid=%v value=%d", wsRel.SortOrder.Valid, wsRel.SortOrder.Int64)
	}
}

// TestRepullKeepsLocalTagUserNamespace LOCAL 关联用户自设 namespace 在重拉后不被清（隔离锚：
// 重拉的窄域清理只作用于 SITE 插件来源关联，LOCAL 关联不删；插件声明 LOCAL 增量冲突不写）
func TestRepullKeepsLocalTagUserNamespace(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	lt := seedLocalTagRow(t, db, "我的本地标签")
	siteWorkId := "repull-local-ns-work"
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
	})
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	linker := newManualTagLinker(db)
	if err := linker.LinkBatchToWork(context.Background(), workId, constant.LOCAL, []int64{lt.GetID()}, []string{"我的标记"}); err != nil {
		t.Fatalf("手动挂 LOCAL 标签（自设 namespace）失败: %v", err)
	}

	// 重拉：仅声明站点侧标签（LOCAL 无声明）
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, &sdkdto.WorkResponse{
		Work:     &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteTags: []*sdkdto.TaskSiteTagDTO{{SiteTagId: "px-t1", TagName: "站点标签"}},
	}); err != nil {
		t.Fatalf("重拉失败: %v", err)
	}

	var rel entity2.ReWorkTag
	if err := db.Where("work_id = ? AND local_tag_id = ?", workId, lt.GetID()).First(&rel).Error; err != nil {
		t.Fatalf("回查 LOCAL 关联失败: %v", err)
	}
	if rel.Source != constant.MANUAL {
		t.Fatalf("LOCAL 关联 source 应为 MANUAL，实际 %d", rel.Source)
	}
	if !rel.Namespace.Valid || rel.Namespace.String != "我的标记" {
		t.Fatalf("LOCAL 关联用户自设 namespace 应保留，实际 Valid=%v value=%q", rel.Namespace.Valid, rel.Namespace.String)
	}
}

// TestRepullAddsNewDeclarationsWithPluginSource 重拉新增标签/作者/作品集声明：正常增量挂联，
// 新旧关联 source=PLUGIN、既有声明不重复建行（回归锚）
func TestRepullAddsNewDeclarationsWithPluginSource(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	siteWorkId := "repull-increment-work"
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	firstPull := &sdkdto.WorkResponse{
		Work:        &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{{SiteAuthorId: "px-a1", AuthorName: "作者1"}},
		SiteTags:    []*sdkdto.TaskSiteTagDTO{{SiteTagId: "px-t1", TagName: "标签1"}},
		WorkSets:    []*sdkdto.TaskWorkSetDTO{{SiteWorkSetId: "px-ws1", WorkSetName: "集1"}},
	}
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, firstPull)
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	// 重拉：新增标签 px-t2、作者 px-a2、作品集 px-ws2
	secondPull := &sdkdto.WorkResponse{
		Work:        &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{{SiteAuthorId: "px-a1", AuthorName: "作者1"}, {SiteAuthorId: "px-a2", AuthorName: "作者2"}},
		SiteTags:    []*sdkdto.TaskSiteTagDTO{{SiteTagId: "px-t1", TagName: "标签1"}, {SiteTagId: "px-t2", TagName: "标签2"}},
		WorkSets:    []*sdkdto.TaskWorkSetDTO{{SiteWorkSetId: "px-ws1", WorkSetName: "集1"}, {SiteWorkSetId: "px-ws2", WorkSetName: "集2"}},
	}
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, secondPull); err != nil {
		t.Fatalf("重拉（新增声明）失败: %v", err)
	}

	// 既有声明不重复建行、新增正常挂联、全部 source=PLUGIN
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND tag_type = ? AND source = ?", workId, constant.SITE, constant.PLUGIN); n != 2 {
		t.Fatalf("SITE 标签关联应 2 条 PLUGIN，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND tag_type = ?", workId, constant.SITE); n != 2 {
		t.Fatalf("SITE 标签关联应恰 2 条（既有不重复），实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND author_type = ? AND source = ?", workId, constant.SITE, constant.PLUGIN); n != 2 {
		t.Fatalf("SITE 作者关联应 2 条 PLUGIN，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkWorkSet{}, "work_id = ? AND source = ?", workId, constant.PLUGIN); n != 2 {
		t.Fatalf("作品集关联应 2 条 PLUGIN，实际 %d", n)
	}
}
