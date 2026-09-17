package importer

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/migration"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// 关联级维度（ns/role）导出回灌往返与旧包兼容锚定：
// ① 关联侧 TagLink/AuthorLink 承载维度值往返保真（含空串与同标签多 ns / 同作者多 role）；
// ② 标签侧 TagRecord 无 namespace 字段（site_tag 行已无该列——序列化产物零残留）；
// ③ 旧导出包 siteTags 记录上的 namespace 键被静默忽略（不报错），关联侧 ns 由 TagLink 承载不丢。

func TestIngestDimensionRoundTrip(t *testing.T) {
	ctx := context.Background()

	// ===== 源库播种 + 导出收集 =====
	srcDB, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	site := entity.NewSite()
	site.SiteKey = identity.Pixiv.Key
	site.SiteName = nullStr("pixiv")
	createSourceRow(t, srcDB, site)

	st := entity.NewSiteTag()
	st.SiteID = nullInt(site.GetID())
	st.SiteTagID = nullStr("tagA")
	st.SiteTagName = nullStr("tagA")
	createSourceRow(t, srcDB, st)

	sa := entity.NewSiteAuthor()
	sa.SiteID = nullInt(site.GetID())
	sa.SiteAuthorID = nullStr("sa-1")
	sa.AuthorName = nullStr("作者甲")
	createSourceRow(t, srcDB, sa)

	la := entity.NewLocalAuthor()
	la.AuthorName = nullStr("本地作者乙")
	createSourceRow(t, srcDB, la)

	w := entity.NewWork()
	w.SiteID = nullInt(site.GetID())
	w.SiteWorkID = nullStr("w-dim")
	w.SiteWorkName = nullStr("维度往返作品")
	createSourceRow(t, srcDB, w)

	for _, ns := range []string{"female", "male", ""} {
		rwt := entity.NewReWorkTag()
		rwt.WorkID = nullInt(w.GetID())
		rwt.TagType = nullInt(int64(constant.SITE))
		rwt.SiteTagID = nullInt(st.GetID())
		rwt.Namespace = ns
		createSourceRow(t, srcDB, rwt)
	}
	for _, role := range []string{"原画", ""} {
		rwa := &entity.ReWorkAuthor{
			BaseEntity:   &model.BaseEntity{},
			AuthorType:   nullInt(int64(constant.SITE)),
			WorkID:       nullInt(w.GetID()),
			SiteAuthorID: nullInt(sa.GetID()),
			RoleName:     role,
			SortOrder:    nullInt(0),
		}
		createSourceRow(t, srcDB, rwa)
	}
	rwaLocal := &entity.ReWorkAuthor{
		BaseEntity:    &model.BaseEntity{},
		AuthorType:    nullInt(int64(constant.LOCAL)),
		WorkID:        nullInt(w.GetID()),
		LocalAuthorID: nullInt(la.GetID()),
		RoleName:      "脚本",
	}
	createSourceRow(t, srcDB, rwaLocal)

	collector := export.NewCollector(export.NewRepository(srcDB), func() string { return "test" })
	collected, err := collector.Collect(ctx, []int64{w.GetID()}, nil)
	if err != nil {
		t.Fatalf("导出收集失败: %v", err)
	}
	m := collected.Manifest
	if len(m.Works) != 1 {
		t.Fatalf("导出作品数=%d，期望 1", len(m.Works))
	}
	wr := &m.Works[0]
	if len(wr.TagLinks) != 3 {
		t.Fatalf("标签关联数=%d，期望 3（同标签多 ns）", len(wr.TagLinks))
	}
	gotNs := map[string]bool{}
	for _, l := range wr.TagLinks {
		if l.Namespace == nil {
			gotNs[""] = true
		} else {
			gotNs[*l.Namespace] = true
		}
	}
	if !gotNs["female"] || !gotNs["male"] || !gotNs[""] {
		t.Fatalf("manifest 标签关联 ns 往返面不全: %v", gotNs)
	}
	if len(wr.AuthorLinks) != 3 {
		t.Fatalf("作者关联数=%d，期望 3（SITE 两 role + LOCAL 一 role）", len(wr.AuthorLinks))
	}
	siteRoles := map[string]bool{}
	for _, l := range wr.AuthorLinks {
		if l.AuthorType != constant.SITE {
			continue
		}
		if l.RoleName == nil {
			siteRoles[""] = true
		} else {
			siteRoles[*l.RoleName] = true
		}
	}
	if !siteRoles["原画"] || !siteRoles[""] {
		t.Fatalf("manifest SITE 作者关联 role 往返面不全: %v", siteRoles)
	}

	data, err := m.Serialize()
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	// ===== 标签侧零残留：序列化产物 siteTags 记录无 namespace 键 =====
	var rawTagRecord struct {
		SiteTags []map[string]any `json:"siteTags"`
	}
	if err := json.Unmarshal(data, &rawTagRecord); err != nil {
		t.Fatalf("解析序列化产物失败: %v", err)
	}
	for _, rec := range rawTagRecord.SiteTags {
		if _, ok := rec["namespace"]; ok {
			t.Fatal("siteTags 记录不应残留 namespace 键（site_tag 行已无该列）")
		}
	}

	// ===== 回灌新库：维度值保真（含空串与多值并存） =====
	newM, err := export.Deserialize(data)
	if err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	ing, db, _, _ := newTestSetup(t)
	if _, err := ing.Ingest(ctx, newM, mapFileSource(nil), nil); err != nil {
		t.Fatalf("回灌失败: %v", err)
	}
	if n := queryInt64(t, db, "SELECT COUNT(*) FROM re_work_tag WHERE namespace = 'female'"); n != 1 {
		t.Fatalf("回灌后 female 关联应 1 行，实际 %d", n)
	}
	if n := queryInt64(t, db, "SELECT COUNT(*) FROM re_work_tag WHERE namespace = 'male'"); n != 1 {
		t.Fatalf("回灌后 male 关联应 1 行，实际 %d", n)
	}
	if n := queryInt64(t, db, "SELECT COUNT(*) FROM re_work_tag WHERE namespace = ''"); n != 1 {
		t.Fatalf("回灌后空串 ns 关联应 1 行，实际 %d", n)
	}
	if n := queryInt64(t, db, "SELECT COUNT(*) FROM re_work_author WHERE role_name = '原画' AND author_type = ?", constant.SITE); n != 1 {
		t.Fatalf("回灌后 SITE 原画 role 关联应 1 行，实际 %d", n)
	}
	if n := queryInt64(t, db, "SELECT COUNT(*) FROM re_work_author WHERE role_name = '' AND author_type = ?", constant.SITE); n != 1 {
		t.Fatalf("回灌后 SITE 空 role 关联应 1 行，实际 %d", n)
	}
	if n := queryInt64(t, db, "SELECT COUNT(*) FROM re_work_author WHERE role_name = '脚本' AND author_type = ?", constant.LOCAL); n != 1 {
		t.Fatalf("回灌后 LOCAL 脚本 role 关联应 1 行，实际 %d", n)
	}

	// ===== 旧包兼容：siteTags 记录携带 namespace 键被静默忽略，不报错、关联侧 ns 不丢 =====
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("解析导出 JSON 失败: %v", err)
	}
	for _, rec := range raw["siteTags"].([]any) {
		rm, _ := rec.(map[string]any)
		rm["namespace"] = "character" // 旧版导出包形态：标签记录级 namespace
	}
	oldData, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("重编旧包 JSON 失败: %v", err)
	}
	oldM, err := export.Deserialize(oldData)
	if err != nil {
		t.Fatalf("旧包反序列化应忽略未知键，实际报错: %v", err)
	}
	ing2, db2, _, _ := newTestSetup(t)
	if _, err := ing2.Ingest(ctx, oldM, mapFileSource(nil), nil); err != nil {
		t.Fatalf("旧包回灌应成功（标签记录级 namespace 静默忽略），实际报错: %v", err)
	}
	// 关联侧 ns 由 TagLink 承载，不受标签记录级键影响
	if n := queryInt64(t, db2, "SELECT COUNT(*) FROM re_work_tag WHERE namespace = 'female'"); n != 1 {
		t.Fatalf("旧包回灌后 female 关联仍应 1 行，实际 %d", n)
	}
	if n := queryInt64(t, db2, "SELECT COUNT(*) FROM re_work_tag WHERE namespace = 'character'"); n != 0 {
		t.Fatalf("标签记录级 namespace 不应泄漏到关联行，实际 %d 行", n)
	}
}
