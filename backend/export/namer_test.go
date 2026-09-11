package export

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSanitizeComponent 锚定净化规则：非法字符全角替换、控制字符剔除、尾部点/空格去除、
// Windows 保留设备名前缀下划线、超长截断、空结果。
func TestSanitizeComponent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"普通名保持", "作品1", "作品1"},
		{"非法字符替换为全角", `a/b:c*d?e"f<g>h|i`, "a／b：c＊d？e＂f＜g＞h｜i"},
		{"反斜杠替换", `a\b`, "a＼b"},
		{"控制字符剔除", "a\x01b\x02c", "abc"},
		{"尾部点去除", "name.", "name"},
		{"尾部空格去除", "name. ", "name"},
		{"尾部点空格去除", "name. . ", "name"},
		{"全点串净化为空", "...", ""},
		{"保留设备名前缀下划线", "CON", "_CON"},
		{"保留设备名带扩展名", "con.txt", "_con.txt"},
		{"NUL 设备名", "NUL", "_NUL"},
		{"超长截断", strings.Repeat("a", maxNameComponentLen+10), strings.Repeat("a", maxNameComponentLen)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, sanitizeComponent(c.in))
		})
	}
}

// pathByStore 汇总 manifest 文件条目 StoreID → 包内路径。
func pathByStore(m *Manifest) map[int64]string {
	by := make(map[int64]string, len(m.Files))
	for _, f := range m.Files {
		by[f.StoreID] = f.Path
	}
	return by
}

// TestPlanNamesDeterminism 同输入同输出：两份相同 manifest（含 exportTime 占位符、
// 基准取 Meta.ExportedAt）产出完全一致的包内路径。
func TestPlanNamesDeterminism(t *testing.T) {
	exportedAt := int64(1725000000000)
	build := func() *Manifest {
		return &Manifest{
			Meta: Meta{ExportedAt: exportedAt},
			Works: []WorkRecord{
				{ID: 1, SiteWorkName: strp("作品"), SiteWorkID: strp("w1"),
					Resources: []ResourceRecord{
						{ID: 10, Stores: []StoreMount{
							{StoreType: "image", StoreSeq: 0, StoreID: 100},
							{StoreType: "thumbnail", StoreSeq: 1, StoreID: 101},
						}},
						{ID: 11, Stores: []StoreMount{
							{StoreType: "document", StoreSeq: 0, StoreID: 102},
						}},
					}},
				{ID: 2, SiteWorkName: strp("作品"), SiteWorkID: strp("w2"),
					Resources: []ResourceRecord{
						{ID: 20, Stores: []StoreMount{
							{StoreType: "image", StoreSeq: 0, StoreID: 200},
						}},
					}},
			},
			Files: []FileEntry{
				{StoreID: 100, StorePath: "store/resource/a/作品.jpg"},
				{StoreID: 101, StorePath: "store/resource/a/作品_thumbnail_001.png"},
				{StoreID: 102, StorePath: "store/resource/a/作品_document_000.md"},
				{StoreID: 200, StorePath: "store/resource/b/作品.txt"},
			},
		}
	}

	m1 := build()
	m2 := build()
	tpl := "${siteWorkName}_${exportTimeYear}${exportTimeMonth}${exportTimeDay}"
	require.NoError(t, PlanNames(m1, tpl))
	require.NoError(t, PlanNames(m2, tpl))
	for i := range m1.Files {
		assert.Equal(t, m1.Files[i].Path, m2.Files[i].Path, "文件条目 %d 路径应一致", i)
	}
}

// TestPlanNamesTemplateRender 模板渲染文件名：默认模板按作品字段渲染基底 + 源文件扩展名；
// ${author} 取作品关联作者（本地优先），uploadTime 取作品上传时间，exportTime 基准取导出时刻。
func TestPlanNamesTemplateRender(t *testing.T) {
	exportedAt := int64(1725000000000)
	uploadAt := int64(1779542400000)
	m := &Manifest{
		Meta:         Meta{ExportedAt: exportedAt},
		LocalAuthors: []AuthorRecord{{ID: 7, Name: strp("本地作者")}},
		Works: []WorkRecord{
			{ID: 1, SiteWorkName: strp("作品A"), SiteWorkID: strp("w-100"),
				SiteUploadTime: &uploadAt,
				AuthorLinks:    []AuthorLink{{AuthorType: 0, AuthorID: 7}}, // 0=local
				Resources: []ResourceRecord{
					{ID: 10, Stores: []StoreMount{{StoreType: "image", StoreSeq: 0, StoreID: 100}}},
				}},
		},
		Files: []FileEntry{{StoreID: 100, StorePath: "store/resource/a/pic.jpg"}},
	}
	require.NoError(t, PlanNames(m, "[${author}]_[${siteWorkId}]_${siteWorkName}.${uploadTimeYear}"))
	assert.Equal(t, "works/[本地作者]_[w-100]_作品A.2026/[本地作者]_[w-100]_作品A.2026.jpg", m.Files[0].Path)

	// exportTime 占位符按 Meta.ExportedAt 基准渲染（与渲染时刻当前时间无关）
	m2 := &Manifest{
		Meta: Meta{ExportedAt: exportedAt},
		Works: []WorkRecord{
			{ID: 1, SiteWorkName: strp("A"), Resources: []ResourceRecord{
				{ID: 10, Stores: []StoreMount{{StoreType: "image", StoreSeq: 0, StoreID: 100}}},
			}},
		},
		Files: []FileEntry{{StoreID: 100, StorePath: "store/resource/a/pic.jpg"}},
	}
	require.NoError(t, PlanNames(m2, "${exportTimeYear}${exportTimeMonth}${exportTimeDay}"))
	want := time.UnixMilli(exportedAt).Format("20060102")
	assert.Equal(t, "works/"+want+"/"+want+".jpg", m2.Files[0].Path)
}

// TestPlanNamesEmptyRenderFallback 渲染为空（模板仅含未提供值的占位符）回退源文件名净化；
// 源文件名净化也为空回退 <bas>_<role>_<seq>。
func TestPlanNamesEmptyRenderFallback(t *testing.T) {
	m := &Manifest{
		Works: []WorkRecord{
			{ID: 1, SiteWorkName: strp("A"),
				Resources: []ResourceRecord{
					{ID: 10, Stores: []StoreMount{
						// 原名净化为空（全点串）→ 兜底 <目录名>_<role>_<seq>
						{StoreType: "image", StoreSeq: 0, StoreID: 100},
						{StoreType: "thumbnail", StoreSeq: 1, StoreID: 101},
					}},
				}},
		},
		Files: []FileEntry{
			{StoreID: 100, StorePath: "store/resource/a/pic.jpg"},
			{StoreID: 101, StorePath: "store/resource/a/..."},
		},
	}
	require.NoError(t, PlanNames(m, "${description}"))
	by := pathByStore(m)
	assert.Equal(t, "works/A/pic.jpg", by[100])
	assert.Equal(t, "works/A/A_thumbnail_001", by[101])
}

// TestPlanNamesFileConflictChain 同作品多文件同扩展名的冲突消解链：
// 渲染名 → 追加 _siteWorkId → 仍冲突追加序号。
func TestPlanNamesFileConflictChain(t *testing.T) {
	m := &Manifest{
		Works: []WorkRecord{
			{ID: 1, SiteWorkName: strp("A"), SiteWorkID: strp("w-1"),
				Resources: []ResourceRecord{
					{ID: 10, Stores: []StoreMount{
						{StoreType: "image", StoreSeq: 0, StoreID: 100},
						{StoreType: "image", StoreSeq: 1, StoreID: 101},
						{StoreType: "image", StoreSeq: 2, StoreID: 102},
					}},
				}},
		},
		Files: []FileEntry{
			{StoreID: 100, StorePath: "store/resource/a/pic.jpg"},
			{StoreID: 101, StorePath: "store/resource/b/pic.jpg"},
			{StoreID: 102, StorePath: "store/resource/c/pic.jpg"},
		},
	}
	require.NoError(t, PlanNames(m, "${siteWorkName}"))
	by := pathByStore(m)
	assert.Equal(t, "works/A/A.jpg", by[100])
	assert.Equal(t, "works/A/A_w-1.jpg", by[101])
	assert.Equal(t, "works/A/A_w-1_2.jpg", by[102])
}

// TestPlanNamesCrossWorkRenderedConflict 同渲染名不同作品：目录与文件名均全局唯一，
// 后作品目录与文件名同走追加 _siteWorkId 后缀消解。
func TestPlanNamesCrossWorkRenderedConflict(t *testing.T) {
	m := &Manifest{
		Works: []WorkRecord{
			{ID: 1, SiteWorkName: strp("同题"), SiteWorkID: strp("w-1"),
				Resources: []ResourceRecord{
					{ID: 10, Stores: []StoreMount{{StoreType: "image", StoreSeq: 0, StoreID: 100}}},
				}},
			{ID: 2, SiteWorkName: strp("同题"), SiteWorkID: strp("w-2"),
				Resources: []ResourceRecord{
					{ID: 20, Stores: []StoreMount{{StoreType: "image", StoreSeq: 0, StoreID: 200}}},
				}},
			{ID: 3, SiteWorkName: strp("同题"), SiteWorkID: strp("w-3"),
				Resources: []ResourceRecord{
					{ID: 30, Stores: []StoreMount{{StoreType: "image", StoreSeq: 0, StoreID: 300}}},
				}},
		},
		Files: []FileEntry{
			{StoreID: 100, StorePath: "store/resource/a/pic.jpg"},
			{StoreID: 200, StorePath: "store/resource/b/pic.jpg"},
			{StoreID: 300, StorePath: "store/resource/c/pic.jpg"},
		},
	}
	require.NoError(t, PlanNames(m, "${siteWorkName}"))
	by := pathByStore(m)
	assert.Equal(t, "works/同题/同题.jpg", by[100])
	assert.Equal(t, "works/同题_w-2/同题_w-2.jpg", by[200])
	assert.Equal(t, "works/同题_w-3/同题_w-3.jpg", by[300])
}

// TestPlanNamesConflictSiteWorkIDEmpty siteWorkId 为空时跳过 ID 后缀直入序号消解。
func TestPlanNamesConflictSiteWorkIDEmpty(t *testing.T) {
	m := &Manifest{
		Works: []WorkRecord{
			{ID: 1, SiteWorkName: strp("X"),
				Resources: []ResourceRecord{
					{ID: 10, Stores: []StoreMount{{StoreType: "image", StoreSeq: 0, StoreID: 100}}},
				}},
			{ID: 2, SiteWorkName: strp("X"),
				Resources: []ResourceRecord{
					{ID: 20, Stores: []StoreMount{{StoreType: "image", StoreSeq: 0, StoreID: 200}}},
				}},
		},
		Files: []FileEntry{
			{StoreID: 100, StorePath: "store/resource/a/pic.jpg"},
			{StoreID: 200, StorePath: "store/resource/b/pic.jpg"},
		},
	}
	require.NoError(t, PlanNames(m, "${siteWorkName}"))
	by := pathByStore(m)
	assert.Equal(t, "works/X/X.jpg", by[100])
	assert.Equal(t, "works/X_2/X_2.jpg", by[200])
}

// TestPlanNamesEmptyFallback 作品目录名回退链：siteWorkName 空 → siteWorkId → work_<id>；再冲突追加序号。
func TestPlanNamesEmptyFallback(t *testing.T) {
	m := &Manifest{
		Works: []WorkRecord{
			{ID: 1, SiteWorkName: strp(""), SiteWorkID: strp("w-1")}, // 名空 → 站点ID
			{ID: 2, SiteWorkName: strp(""), SiteWorkID: strp("")},    // 双空 → work_2
			{ID: 3, SiteWorkName: strp("w-1"), SiteWorkID: strp("")}, // 名与作品1 目录冲突 → w-1_2
		},
		Files: []FileEntry{
			{StoreID: 100, StorePath: "store/resource/a/x.jpg"},
			{StoreID: 200, StorePath: "store/resource/b/y.jpg"},
			{StoreID: 300, StorePath: "store/resource/c/z.jpg"},
		},
	}
	// 每个作品挂一个文件以触发目录命名
	m.Works[0].Resources = []ResourceRecord{{ID: 10, Stores: []StoreMount{{StoreType: "image", StoreSeq: 0, StoreID: 100}}}}
	m.Works[1].Resources = []ResourceRecord{{ID: 20, Stores: []StoreMount{{StoreType: "image", StoreSeq: 0, StoreID: 200}}}}
	m.Works[2].Resources = []ResourceRecord{{ID: 30, Stores: []StoreMount{{StoreType: "image", StoreSeq: 0, StoreID: 300}}}}

	require.NoError(t, PlanNames(m, ""))
	by := pathByStore(m)
	assert.Equal(t, "works/w-1/x.jpg", by[100])
	assert.Equal(t, "works/work_2/y.jpg", by[200])
	assert.Equal(t, "works/w-1_2/z.jpg", by[300])
}

// TestSplitNameExt 拆分基底与扩展名。
func TestSplitNameExt(t *testing.T) {
	base, ext := splitNameExt("a.b.c.jpg")
	assert.Equal(t, "a.b.c", base)
	assert.Equal(t, ".jpg", ext)

	base, ext = splitNameExt("noext")
	assert.Equal(t, "noext", base)
	assert.Equal(t, "", ext)

	base, ext = splitNameExt(".hidden")
	assert.Equal(t, ".hidden", base)
	assert.Equal(t, "", ext)
}
