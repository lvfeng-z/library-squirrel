package work

import (
	"context"
	"database/sql"
	"testing"

	entity2 "github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// 重拉语义在单测中的形态：对同一身份键（site_work_id / site_work_set_id / site_tag_id 等）的声明
// 再次走 saveWorkInfoInTx 入库链，命中既有行走冲突更新分支。各测先首插、再注入用户态（改名/
// 浏览痕迹/桥接绑定）、后重拉，断言插件权威列刷新而用户策展列保持——「没更新」与「更新但未碰
// 用户列」靠重拉时改变站点侧名称区分。

// TestRepullKeepsSiteTagAndSiteAuthorUserFields 重拉已有作品：site_tag 行的 local_tag_id/last_use、
// site_author 行的 local_author_id/last_use 是用户字段（site→local 桥接与使用痕迹），重拉只刷新插件
// 权威字段（名称等），不覆盖用户字段（回归锚）
func TestRepullKeepsSiteTagAndSiteAuthorUserFields(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	siteWorkId := "repull-periph-work"
	firstPull := &sdkdto.WorkResponse{
		Work:        &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{{SiteAuthorId: "px-a1", AuthorName: "作者名v1"}},
		SiteTags:    []*sdkdto.TaskSiteTagDTO{{SiteTagId: "px-t1", TagName: "标签名v1"}},
	}
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, firstPull); err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	// 注入用户态：site→local 桥接与使用痕迹（site_tag.local_tag_id / site_author.local_author_id 无外键，直接置值）
	if err := db.Model(&entity2.SiteTag{}).Where("site_tag_id = ?", "px-t1").
		Updates(map[string]any{"local_tag_id": 777, "last_use": 111222333}).Error; err != nil {
		t.Fatalf("注入 site_tag 用户态失败: %v", err)
	}
	if err := db.Model(&entity2.SiteAuthor{}).Where("site_author_id = ?", "px-a1").
		Updates(map[string]any{"local_author_id": 888, "last_use": 444555666}).Error; err != nil {
		t.Fatalf("注入 site_author 用户态失败: %v", err)
	}

	// 重拉：同键声明，站点侧名称已变化（证明冲突更新分支确实执行）
	secondPull := &sdkdto.WorkResponse{
		Work:        &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{{SiteAuthorId: "px-a1", AuthorName: "作者名v2"}},
		SiteTags:    []*sdkdto.TaskSiteTagDTO{{SiteTagId: "px-t1", TagName: "标签名v2"}},
	}
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, secondPull); err != nil {
		t.Fatalf("重拉入库失败: %v", err)
	}

	var tag entity2.SiteTag
	if err := db.Where("site_tag_id = ?", "px-t1").First(&tag).Error; err != nil {
		t.Fatalf("回查 site_tag 失败: %v", err)
	}
	if tag.SiteTagName.String != "标签名v2" {
		t.Fatalf("插件权威列应刷新为重拉声明名，实际 %q", tag.SiteTagName.String)
	}
	if !tag.LocalTagID.Valid || tag.LocalTagID.Int64 != 777 {
		t.Fatalf("重拉不应动 local_tag_id，实际 Valid=%v value=%d", tag.LocalTagID.Valid, tag.LocalTagID.Int64)
	}
	if !tag.LastUse.Valid || tag.LastUse.Int64 != 111222333 {
		t.Fatalf("重拉不应动 last_use，实际 Valid=%v value=%d", tag.LastUse.Valid, tag.LastUse.Int64)
	}

	var author entity2.SiteAuthor
	if err := db.Where("site_author_id = ?", "px-a1").First(&author).Error; err != nil {
		t.Fatalf("回查 site_author 失败: %v", err)
	}
	if author.AuthorName.String != "作者名v2" {
		t.Fatalf("插件权威列应刷新为重拉声明名，实际 %q", author.AuthorName.String)
	}
	if !author.LocalAuthorID.Valid || author.LocalAuthorID.Int64 != 888 {
		t.Fatalf("重拉不应动 local_author_id，实际 Valid=%v value=%d", author.LocalAuthorID.Valid, author.LocalAuthorID.Int64)
	}
	if !author.LastUse.Valid || author.LastUse.Int64 != 444555666 {
		t.Fatalf("重拉不应动 last_use，实际 Valid=%v value=%d", author.LastUse.Valid, author.LastUse.Int64)
	}
}

// TestRepullKeepsWorkSetUserFields 重拉已有作品集：用户 nick_name 改名与 last_view 浏览痕迹不被清空。
// 入库链映射（taskWorkSetDTOToEntity）只填站点侧三列，upsert 冲突更新列集须不含用户策展列——含它们时
// excluded（待插入行）的零值 NULL 会覆盖用户值（缺陷修复锚，先红后绿）
func TestRepullKeepsWorkSetUserFields(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	siteWorkId := "repull-workset-work"
	firstPull := &sdkdto.WorkResponse{
		Work:     &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		WorkSets: []*sdkdto.TaskWorkSetDTO{{SiteWorkSetId: "px-ws1", WorkSetName: "集名v1"}},
	}
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, firstPull); err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	// 注入用户态：改名 + 浏览痕迹
	if err := db.Model(&entity2.WorkSet{}).Where("site_work_set_id = ?", "px-ws1").
		Updates(map[string]any{"nick_name": "用户改的集名", "last_view": 987654321}).Error; err != nil {
		t.Fatalf("注入 work_set 用户态失败: %v", err)
	}

	// 重拉：同键声明，站点侧集名已变化（证明冲突更新分支确实执行）
	secondPull := &sdkdto.WorkResponse{
		Work:     &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		WorkSets: []*sdkdto.TaskWorkSetDTO{{SiteWorkSetId: "px-ws1", WorkSetName: "集名v2"}},
	}
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, secondPull); err != nil {
		t.Fatalf("重拉入库失败: %v", err)
	}

	var ws entity2.WorkSet
	if err := db.Where("site_work_set_id = ?", "px-ws1").First(&ws).Error; err != nil {
		t.Fatalf("回查 work_set 失败: %v", err)
	}
	if ws.SiteWorkSetName.String != "集名v2" {
		t.Fatalf("插件权威列应刷新为重拉声明名，实际 %q", ws.SiteWorkSetName.String)
	}
	if !ws.NickName.Valid || ws.NickName.String != "用户改的集名" {
		t.Fatalf("重拉不应清空用户改名 nick_name，实际 Valid=%v value=%q", ws.NickName.Valid, ws.NickName.String)
	}
	if !ws.LastView.Valid || ws.LastView.Int64 != 987654321 {
		t.Fatalf("重拉不应清空浏览痕迹 last_view，实际 Valid=%v value=%d", ws.LastView.Valid, ws.LastView.Int64)
	}
}

// TestRepullKeepsWorkUserFields 重拉已有作品：work 行的 nick_name/last_view/local_author_id 是用户
// 策展字段，重拉不填即不改写——保护依赖「插件 WorkDTO 三字段不填 + 结构体 Updates 跳零值」双层
// 行为约定（saveOrUpdateWork 注释锚定），本测为该约定的回归锚
func TestRepullKeepsWorkUserFields(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")

	// work.local_author_id 外键父行
	localAuthor := entity2.NewLocalAuthor()
	localAuthor.AuthorName = sql.NullString{String: "用户绑定的本地作者", Valid: true}
	if err := db.Create(localAuthor).Error; err != nil {
		t.Fatalf("插 local_author 失败: %v", err)
	}

	siteWorkId := "repull-work-row"
	firstName := "作品名v1"
	firstPull := &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &firstName},
	}
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	workId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, firstPull)
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}

	// 注入用户态：改名 + 浏览痕迹 + 绑定本地作者
	if err := db.Model(&entity2.Work{}).Where("id = ?", workId).
		Updates(map[string]any{
			"nick_name":       "用户改的作品名",
			"last_view":       123456789,
			"local_author_id": localAuthor.GetID(),
		}).Error; err != nil {
		t.Fatalf("注入 work 用户态失败: %v", err)
	}

	// 重拉：插件 WorkDTO 不填三用户字段，站点侧作品名已变化（证明 Updates 分支确实执行）
	secondName := "作品名v2"
	secondPull := &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &secondName},
	}
	repulledId, err := svc.saveWorkInfoInTx(context.Background(), task, wt, secondPull)
	if err != nil {
		t.Fatalf("重拉入库失败: %v", err)
	}
	if repulledId != workId {
		t.Fatalf("重拉应命中同一作品行，首次=%d 重拉=%d", workId, repulledId)
	}

	var w entity2.Work
	if err := db.Where("id = ?", workId).First(&w).Error; err != nil {
		t.Fatalf("回查 work 失败: %v", err)
	}
	if w.SiteWorkName.String != "作品名v2" {
		t.Fatalf("插件权威列应刷新为重拉声明名，实际 %q", w.SiteWorkName.String)
	}
	if !w.NickName.Valid || w.NickName.String != "用户改的作品名" {
		t.Fatalf("重拉不应改写用户改名 nick_name，实际 Valid=%v value=%q", w.NickName.Valid, w.NickName.String)
	}
	if !w.LastView.Valid || w.LastView.Int64 != 123456789 {
		t.Fatalf("重拉不应改写 last_view，实际 Valid=%v value=%d", w.LastView.Valid, w.LastView.Int64)
	}
	if !w.LocalAuthorID.Valid || w.LocalAuthorID.Int64 != localAuthor.GetID() {
		t.Fatalf("重拉不应改写 local_author_id，实际 Valid=%v value=%d", w.LocalAuthorID.Valid, w.LocalAuthorID.Int64)
	}
}
