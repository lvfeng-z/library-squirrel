package work

import (
	"context"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/tagNamespace"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// 入库链的关联级维度行为锚定：e-hentai 形态端到端（同纯名 site_tag_id 多 ns 声明）、
// 插件声明 role 入库与重拉刷新、维度清单登记（origin=plugin）。

// TestIngestMultiNamespaceSamePureTagId e-hentai 端到端（锚 7）：插件以同一纯名 site_tag_id
// 声明多 ns（内置集值 female + 自定义值 artist-group）→ 库内一行 site_tag + 两条 ns 关联并存；
// 重拉仅再声明 female → artist-group 关联清理、female 保留（PLUGIN 关联按本次声明窄域重建）；
// 自定义 ns 登记清单 origin=plugin，内置集值 female 的清单行不被插件使用降级
func TestIngestMultiNamespaceSamePureTagId(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	eh := seedSite(t, db, "ehentai")

	siteWorkId := "eh-multi-ns-work"
	pull := func(nss ...string) *sdkdto.WorkResponse {
		tags := make([]*sdkdto.TaskSiteTagDTO, 0, len(nss))
		for _, ns := range nss {
			tags = append(tags, &sdkdto.TaskSiteTagDTO{SiteTagId: "tagA", TagName: "tagA", Namespace: ns})
		}
		return &sdkdto.WorkResponse{
			Work:     &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
			SiteTags: tags,
		}
	}
	task, wt := newWorkTaskOfSite(eh.GetID())

	// 预跑内置集投影（模拟启动期 SyncBuiltins——测试库不走启动装配，female 先落 origin=builtin 行）
	if err := tagNamespace.NewService(tagNamespace.NewRepository(db)).SyncBuiltins(context.Background()); err != nil {
		t.Fatalf("预跑内置集投影失败: %v", err)
	}

	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, pull("female", "artist-group"))
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	// 一行 site_tag（纯名身份不按 ns 分裂）
	if n := countRows(t, db, &entity2.SiteTag{}, "site_tag_id = ?", "tagA"); n != 1 {
		t.Fatalf("同纯名多 ns 声明应落一行 site_tag，实际 %d", n)
	}
	// 两条 ns 关联并存
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND tag_type = ?", workId, constant.SITE); n != 2 {
		t.Fatalf("同标签两条 ns 关联应并存，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND tag_type = ? AND namespace = ?", workId, constant.SITE, "female"); n != 1 {
		t.Fatalf("female 关联应恰 1 条，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND tag_type = ? AND namespace = ?", workId, constant.SITE, "artist-group"); n != 1 {
		t.Fatalf("artist-group 关联应恰 1 条，实际 %d", n)
	}
	// ns 清单登记（origin=plugin）
	var customInv entity2.TagNamespace
	if err := db.Where("value = ?", "artist-group").First(&customInv).Error; err != nil {
		t.Fatalf("artist-group 应登记 ns 清单: %v", err)
	}
	if customInv.Origin != constant.ORIGIN_PLUGIN {
		t.Fatalf("入库链登记的 ns 清单 origin 应为 plugin，实际 %d", customInv.Origin)
	}
	// female 属内置集：投影行 origin=builtin 不被插件使用降级
	var femaleInv entity2.TagNamespace
	if err := db.Where("value = ?", "female").First(&femaleInv).Error; err != nil {
		t.Fatalf("female 应有清单行: %v", err)
	}
	if femaleInv.Origin != constant.ORIGIN_BUILTIN {
		t.Fatalf("内置集 female 的清单行不应被降级为 plugin，实际 origin=%d", femaleInv.Origin)
	}

	// 重拉：仅再声明 female → artist-group 关联清理（ns 不再声明 = 关联删除）
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, pull("female")); err != nil {
		t.Fatalf("重拉失败: %v", err)
	}
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND tag_type = ?", workId, constant.SITE); n != 1 {
		t.Fatalf("重拉后应仅剩 female 关联，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkTag{}, "work_id = ? AND tag_type = ? AND namespace = ?", workId, constant.SITE, "artist-group"); n != 0 {
		t.Fatalf("不再声明的 artist-group 关联应清理，实际 %d", n)
	}
}

// TestIngestPluginRoleDeclarationAndRepullRefresh 插件声明 role 入库（锚 13/11 插件侧/锚 12）：
// 声明值直写关联行（含空串无 role）；同作者多 role 声明落多行、同 role 重复声明折叠；
// 重拉按本次声明刷新 PLUGIN 行 role；用户手动挂的 role 行不动；role 清单登记（origin=plugin）
func TestIngestPluginRoleDeclarationAndRepullRefresh(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	siteWorkId := "role-ingest-work"
	pull := func(authors ...*sdkdto.TaskSiteAuthorDTO) *sdkdto.WorkResponse {
		return &sdkdto.WorkResponse{
			Work:        &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
			SiteAuthors: authors,
		}
	}
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, pull(
		&sdkdto.TaskSiteAuthorDTO{SiteAuthorId: "px-a1", AuthorName: "作者1", RoleName: "作画监督"},
		&sdkdto.TaskSiteAuthorDTO{SiteAuthorId: "px-a1", AuthorName: "作者1", RoleName: "脚本"},
		&sdkdto.TaskSiteAuthorDTO{SiteAuthorId: "px-a2", AuthorName: "作者2", RoleName: " 原画 "},
		&sdkdto.TaskSiteAuthorDTO{SiteAuthorId: "px-a3", AuthorName: "作者3"},
	))
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	// 同作者多 role 两行、无 role 作者空串行、role 归一化（" 原画 " → "原画"）
	// a1 两行 + a2 一行 + a3 一行 = 4 行
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND author_type = ?", workId, constant.SITE); n != 4 {
		t.Fatalf("作者关联应 4 行（a1 两 role + a2 + a3），实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND author_type = ? AND role_name = '作画监督'", workId, constant.SITE); n != 1 {
		t.Fatalf("a1 作画监督行应恰 1 条，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND author_type = ? AND role_name = '脚本'", workId, constant.SITE); n != 1 {
		t.Fatalf("a1 脚本行应恰 1 条，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND author_type = ? AND role_name = '原画'", workId, constant.SITE); n != 1 {
		t.Fatalf("a2 归一化 role 原画应恰 1 条，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND author_type = ? AND role_name = ''", workId, constant.SITE); n != 1 {
		t.Fatalf("无 role 作者应落空串行恰 1 条，实际 %d", n)
	}
	// role 清单登记（origin=plugin）
	for _, role := range []string{"作画监督", "脚本", "原画"} {
		var inv entity2.AuthorRole
		if err := db.Where("value = ?", role).First(&inv).Error; err != nil {
			t.Fatalf("role %q 应登记清单: %v", role, err)
		}
		if inv.Origin != constant.ORIGIN_PLUGIN {
			t.Fatalf("入库链登记的 role 清单 origin 应为 plugin，实际 %d", inv.Origin)
		}
	}

	// 用户手动挂 SITE 作者带 role（MANUAL 行）——重拉后不动
	linker := newManualAuthorLinker(db)
	manualAuthor := seedSiteAuthorRow(t, db, pixiv.GetID(), "px-manual")
	if err := linker.LinkBatchToWork(context.Background(), workId, constant.SITE, []int64{manualAuthor.GetID()}, []string{"赞助"}); err != nil {
		t.Fatalf("手动挂作者失败: %v", err)
	}

	// 重拉：a1 改声明仅 作画监督、a2 改 role 为 演出、a3 不再声明
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, pull(
		&sdkdto.TaskSiteAuthorDTO{SiteAuthorId: "px-a1", AuthorName: "作者1", RoleName: "作画监督"},
		&sdkdto.TaskSiteAuthorDTO{SiteAuthorId: "px-a2", AuthorName: "作者2", RoleName: "演出"},
	)); err != nil {
		t.Fatalf("重拉失败: %v", err)
	}
	// PLUGIN 行按本次声明刷新：a1 仅剩作画监督（脚本行清理）、a2 role 刷新为 演出、a3 清理
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND role_name = '脚本'", workId); n != 0 {
		t.Fatalf("重拉不再声明的 role 行应清理，实际 %d", n)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND role_name = '演出'", workId); n != 1 {
		t.Fatalf("a2 role 应刷新为 演出，实际 %d 行", n)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND role_name = '原画'", workId); n != 0 {
		t.Fatalf("a2 旧 role 原画行应随重建清理，实际 %d", n)
	}
	// MANUAL 行不动（含用户 role）
	var manualRel entity2.ReWorkAuthor
	if err := db.Where("work_id = ? AND site_author_id = ?", workId, manualAuthor.GetID()).First(&manualRel).Error; err != nil {
		t.Fatalf("回查手动作者关联失败: %v", err)
	}
	if manualRel.Source != constant.MANUAL || manualRel.RoleName != "赞助" {
		t.Fatalf("手动作者关联应保持 MANUAL+赞助，实际 source=%d role=%q", manualRel.Source, manualRel.RoleName)
	}
}

// TestManualLinkMultiRoleBothTracks 手动挂联多 role 并存两轨（锚 11 手动侧）：
// 同作品同作者不同 role 落独立关联行（SITE 与 LOCAL 各一）、重复写同三元组收敛、
// 空串 role 与具体 role 并存（锚 12）
func TestManualLinkMultiRoleBothTracks(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	siteWorkId := "multi-role-manual-work"
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
	})
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	linker := newManualAuthorLinker(db)
	siteAuthor := seedSiteAuthorRow(t, db, pixiv.GetID(), "px-mr")
	localAuthor := seedLocalAuthorRow(t, db, "多职作者")
	ctx := context.Background()

	// SITE 轨：同作者挂 原画 + 脚本 + 空串 三行
	if err := linker.LinkBatchToWork(ctx, workId, constant.SITE,
		[]int64{siteAuthor.GetID(), siteAuthor.GetID(), siteAuthor.GetID()},
		[]string{"原画", "脚本", ""}); err != nil {
		t.Fatalf("手动挂 SITE 多 role 失败: %v", err)
	}
	// 重复写同三元组收敛
	if err := linker.LinkBatchToWork(ctx, workId, constant.SITE, []int64{siteAuthor.GetID()}, []string{"原画"}); err != nil {
		t.Fatalf("重复挂 原画 失败: %v", err)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND site_author_id = ?", workId, siteAuthor.GetID()); n != 3 {
		t.Fatalf("SITE 轨同作者应 3 行（原画/脚本/空串）且重复写收敛，实际 %d", n)
	}

	// LOCAL 轨同型
	if err := linker.LinkBatchToWork(ctx, workId, constant.LOCAL,
		[]int64{localAuthor.GetID(), localAuthor.GetID()}, []string{"原画", ""}); err != nil {
		t.Fatalf("手动挂 LOCAL 多 role 失败: %v", err)
	}
	if n := countRows(t, db, &entity2.ReWorkAuthor{}, "work_id = ? AND local_author_id = ?", workId, localAuthor.GetID()); n != 2 {
		t.Fatalf("LOCAL 轨同作者应 2 行，实际 %d", n)
	}
}
