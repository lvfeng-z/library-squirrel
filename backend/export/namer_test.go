package export

import (
	"strings"
	"testing"

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

// TestPlanNamesDeterminism 同输入同输出：两份相同 manifest 产出完全一致的包内路径。
func TestPlanNamesDeterminism(t *testing.T) {
	build := func() *Manifest {
		return &Manifest{
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
	require.NoError(t, PlanNames(m1))
	require.NoError(t, PlanNames(m2))
	for i := range m1.Files {
		assert.Equal(t, m1.Files[i].Path, m2.Files[i].Path, "文件条目 %d 路径应一致", i)
	}
}

// TestPlanNamesLayout 锚定布局：works/<作品目录>/<文件>，同名作品目录追加序号消解。
func TestPlanNamesLayout(t *testing.T) {
	m := &Manifest{
		Works: []WorkRecord{
			{ID: 1, SiteWorkName: strp("作品"), SiteWorkID: strp("w1"),
				Resources: []ResourceRecord{
					{ID: 10, Stores: []StoreMount{
						{StoreType: "image", StoreSeq: 0, StoreID: 100},
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
			{StoreID: 200, StorePath: "store/resource/b/作品.txt"},
		},
	}
	require.NoError(t, PlanNames(m))
	pathByStore := map[int64]string{}
	for _, f := range m.Files {
		pathByStore[f.StoreID] = f.Path
	}
	assert.Equal(t, "works/作品/作品.jpg", pathByStore[100])
	assert.Equal(t, "works/作品_2/作品.txt", pathByStore[200])
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

	require.NoError(t, PlanNames(m))
	pathByStore := map[int64]string{}
	for _, f := range m.Files {
		pathByStore[f.StoreID] = f.Path
	}
	assert.Equal(t, "works/w-1/x.jpg", pathByStore[100])
	assert.Equal(t, "works/work_2/y.jpg", pathByStore[200])
	assert.Equal(t, "works/w-1_2/z.jpg", pathByStore[300])
}

// TestPlanNamesFileConflict 同作品目录内同名文件按 store 命名规约 <bas>_<role>_<seq> 消解。
func TestPlanNamesFileConflict(t *testing.T) {
	m := &Manifest{
		Works: []WorkRecord{
			{ID: 1, SiteWorkName: strp("A"),
				Resources: []ResourceRecord{
					{ID: 10, Stores: []StoreMount{
						{StoreType: "image", StoreSeq: 0, StoreID: 100},
						{StoreType: "thumbnail", StoreSeq: 1, StoreID: 101},
					}},
				}},
		},
		Files: []FileEntry{
			{StoreID: 100, StorePath: "store/resource/a/pic.jpg"},
			{StoreID: 101, StorePath: "store/resource/a/pic.jpg"}, // 同名
		},
	}
	require.NoError(t, PlanNames(m))
	pathByStore := map[int64]string{}
	for _, f := range m.Files {
		pathByStore[f.StoreID] = f.Path
	}
	assert.Equal(t, "works/A/pic.jpg", pathByStore[100])
	assert.Equal(t, "works/A/pic_thumbnail_001.jpg", pathByStore[101])
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
