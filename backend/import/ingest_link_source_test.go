package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/migration"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
	"gorm.io/gorm"
)

// 测试锚定9（D5/D7）：关联来源 manifest 往返——
// ① 导出收集把 DB 行 source 写进 manifest 三类 Link；
// ② 新包回灌保真：source=MANUAL 的用户手动关联回灌到新库后落 MANUAL；
// ③ 旧包缺省：JSON 无 source 字段（旧版导出包）反序列化零值 PLUGIN、回灌落 PLUGIN；
// ④ 冲突不翻转：回灌库已有同键关联行时 manifest 关联不重复建行、既有行来源不变。

// linkSourceFixture 源库播种夹具：作品挂 LOCAL+SITE 双标签关联、LOCAL 作者关联、
// 作品集成员关联各一组，全部 source=MANUAL（用户手动挂联形态）。
type linkSourceFixture struct {
	workID int64
	wsID   int64
}

func seedLinkSourceFixture(t *testing.T, db *gorm.DB) *linkSourceFixture {
	t.Helper()
	f := &linkSourceFixture{}

	site := entity.NewSite()
	site.SiteKey = identity.Pixiv.Key
	site.SiteName = nullStr("pixiv")
	createSourceRow(t, db, site)

	lt := entity.NewLocalTag()
	lt.LocalTagName = nullStr("手动标签")
	createSourceRow(t, db, lt)

	st := entity.NewSiteTag()
	st.SiteID = nullInt(site.GetID())
	st.SiteTagID = nullStr("st-manual")
	st.SiteTagName = nullStr("原站手动标签")
	createSourceRow(t, db, st)

	la := entity.NewLocalAuthor()
	la.AuthorName = nullStr("本地作者")
	createSourceRow(t, db, la)

	w := entity.NewWork()
	w.SiteID = nullInt(site.GetID())
	w.SiteWorkID = nullStr("w-manual-1")
	w.SiteWorkName = nullStr("手动关联作品")
	createSourceRow(t, db, w)
	f.workID = w.GetID()

	ws := entity.NewWorkSet()
	ws.SiteID = nullInt(site.GetID())
	ws.SiteWorkSetID = nullStr("ws-manual-1")
	ws.SiteWorkSetName = nullStr("手动合集")
	createSourceRow(t, db, ws)
	f.wsID = ws.GetID()

	rwtLocal := entity.NewReWorkTag()
	rwtLocal.WorkID = nullInt(f.workID)
	rwtLocal.TagType = nullInt(int64(constant.LOCAL))
	rwtLocal.LocalTagID = nullInt(lt.GetID())
	rwtLocal.Source = constant.MANUAL
	createSourceRow(t, db, rwtLocal)

	rwtSite := entity.NewReWorkTag()
	rwtSite.WorkID = nullInt(f.workID)
	rwtSite.TagType = nullInt(int64(constant.SITE))
	rwtSite.SiteTagID = nullInt(st.GetID())
	rwtSite.Namespace = "character"
	rwtSite.Source = constant.MANUAL
	createSourceRow(t, db, rwtSite)

	rwa := &entity.ReWorkAuthor{
		BaseEntity:    &model.BaseEntity{},
		AuthorType:    nullInt(int64(constant.LOCAL)),
		WorkID:        nullInt(f.workID),
		LocalAuthorID: nullInt(la.GetID()),
		Source:        constant.MANUAL,
	}
	createSourceRow(t, db, rwa)

	rel := entity.NewReWorkWorkSet()
	rel.WorkID = nullInt(f.workID)
	rel.WorkSetID = nullInt(ws.GetID())
	rel.Source = constant.MANUAL
	createSourceRow(t, db, rel)

	return f
}

// createSourceRow 源库播种（失败即终止；行 ID 经实体回填）。
func createSourceRow(t *testing.T, db *gorm.DB, row any) {
	t.Helper()
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("播种源库失败: %v", err)
	}
}

// assertAllLinkSource 断言三类关联表全部回灌行的来源值（测试起始库无关联行，全局计数即回灌产物）。
func assertAllLinkSource(t *testing.T, db *gorm.DB, want int64) {
	t.Helper()
	cases := []struct {
		table string
		total int64
	}{
		{"re_work_tag", 2},
		{"re_work_author", 1},
		{"re_work_work_set", 1},
	}
	for _, c := range cases {
		if n := queryInt64(t, db, "SELECT COUNT(*) FROM "+c.table); n != c.total {
			t.Fatalf("表 %s 行数=%d，期望 %d", c.table, n, c.total)
		}
		if n := queryInt64(t, db, "SELECT COUNT(*) FROM "+c.table+" WHERE source = ?", want); n != c.total {
			t.Fatalf("表 %s source=%d 行数=%d，期望 %d", c.table, want, n, c.total)
		}
	}
}

// stripLinkSourceKeys 从导出 JSON 中剥除三类关联 Link 的 source 字段，模拟不含该字段的旧版导出包。
func stripLinkSourceKeys(t *testing.T, data []byte) []byte {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("解析导出 JSON 失败: %v", err)
	}
	works, _ := raw["works"].([]any)
	for _, w := range works {
		wm, _ := w.(map[string]any)
		for _, key := range []string{"tagLinks", "authorLinks", "workSetLinks"} {
			links, _ := wm[key].([]any)
			for _, l := range links {
				lm, _ := l.(map[string]any)
				delete(lm, "source")
			}
		}
	}
	out, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("重编旧包 JSON 失败: %v", err)
	}
	return out
}

func TestIngestLinkSourceRoundTrip(t *testing.T) {
	ctx := context.Background()

	// ===== ① 导出收集：DB 行 source 写进 manifest Link =====
	srcDB, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	f := seedLinkSourceFixture(t, srcDB)
	collector := export.NewCollector(export.NewRepository(srcDB), func() string { return "test" })
	model, err := collector.Collect(ctx, nil, []int64{f.wsID})
	if err != nil {
		t.Fatalf("导出收集失败: %v", err)
	}
	m := model.Manifest
	if len(m.Works) != 1 {
		t.Fatalf("导出作品数=%d，期望 1", len(m.Works))
	}
	w := &m.Works[0]
	if len(w.TagLinks) != 2 || len(w.AuthorLinks) != 1 || len(w.WorkSetLinks) != 1 {
		t.Fatalf("关联数异常：tag=%d author=%d workSet=%d，期望 2/1/1",
			len(w.TagLinks), len(w.AuthorLinks), len(w.WorkSetLinks))
	}
	for _, l := range w.TagLinks {
		if l.Source != constant.MANUAL {
			t.Fatalf("manifest 标签关联 source=%d，期望 MANUAL", l.Source)
		}
	}
	for _, l := range w.AuthorLinks {
		if l.Source != constant.MANUAL {
			t.Fatalf("manifest 作者关联 source=%d，期望 MANUAL", l.Source)
		}
	}
	for _, l := range w.WorkSetLinks {
		if l.Source != constant.MANUAL {
			t.Fatalf("manifest 作品集关联 source=%d，期望 MANUAL", l.Source)
		}
	}

	data, err := m.Serialize()
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	// ===== ② 新包回灌保真：MANUAL 关联回灌到新库后落 MANUAL =====
	newM, err := export.Deserialize(data)
	if err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	ingA, dbA, _, _ := newTestSetup(t)
	if _, err := ingA.Ingest(ctx, newM, mapFileSource(nil), nil); err != nil {
		t.Fatalf("新包回灌失败: %v", err)
	}
	assertAllLinkSource(t, dbA, constant.MANUAL)

	// ===== ③ 旧包缺省：JSON 无 source 字段 → 零值 PLUGIN → 回灌落 PLUGIN =====
	oldM, err := export.Deserialize(stripLinkSourceKeys(t, data))
	if err != nil {
		t.Fatalf("旧包反序列化失败: %v", err)
	}
	ow := &oldM.Works[0]
	for _, l := range ow.TagLinks {
		if l.Source != constant.PLUGIN {
			t.Fatalf("旧包反序列化标签关联 source=%d，期望零值 PLUGIN", l.Source)
		}
	}
	ingB, dbB, _, _ := newTestSetup(t)
	if _, err := ingB.Ingest(ctx, oldM, mapFileSource(nil), nil); err != nil {
		t.Fatalf("旧包回灌失败: %v", err)
	}
	assertAllLinkSource(t, dbB, constant.PLUGIN)

	// ===== ④ 冲突不翻转：同作品替换式再回灌携带 MANUAL 的新包，既有行不翻转、不重复建行 =====
	if _, err := ingB.Ingest(ctx, newM, mapFileSource(nil), &IngestOptions{ReplaceWorks: map[int64]struct{}{f.workID: {}}}); err != nil {
		t.Fatalf("替换回灌失败: %v", err)
	}
	assertAllLinkSource(t, dbB, constant.PLUGIN)
}

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
func nullInt(i int64) sql.NullInt64   { return sql.NullInt64{Int64: i, Valid: true} }
