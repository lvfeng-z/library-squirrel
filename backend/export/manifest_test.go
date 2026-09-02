package export

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManifestRoundTrip 锚定 manifest 序列化往返一致（退出标准：契约测试）。
// 全字段类型覆盖：指针可空字段用非空指针（omitempty 下空指针往返为 nil，非空指针往返不变）。
func TestManifestRoundTrip(t *testing.T) {
	siteWorkID := "site-work-1"
	siteTagID := "st-1"
	m := &Manifest{
		SchemaVersion: SchemaVersion,
		Meta: Meta{
			ExportedAt:       1725000000000,
			AppVersion:       "0.0.1",
			SiteCount:        1,
			LocalAuthorCount: 1,
			SiteAuthorCount:  1,
			LocalTagCount:    1,
			SiteTagCount:     1,
			WorkSetCount:     1,
			WorkCount:        1,
			FileCount:        1,
		},
		Sites: []SiteRecord{
			{ID: 10, SiteKey: "pixiv", SiteName: strp("pixiv"), Homepage: strp("https://pixiv.net"), CreateTime: 1, UpdateTime: 2},
		},
		LocalAuthors: []AuthorRecord{
			{ID: 20, Name: strp("画师"), Introduce: strp("简介"), LastUse: int64p(100), CreateTime: 1, UpdateTime: 2},
		},
		SiteAuthors: []AuthorRecord{
			{ID: 30, Name: strp("站画师"), SiteID: int64p(10), SiteAuthorID: strp("sa-1"), FixedAuthorName: strp("固定名"), Homepage: strp("https://x.com"), LocalAuthorID: int64p(20), CreateTime: 1, UpdateTime: 2},
		},
		LocalTags: []TagRecord{
			{ID: 40, Name: strp("R-18"), BaseLocalTagID: int64p(41), Description: strp("成人"), LastUse: int64p(200), CreateTime: 1, UpdateTime: 2},
		},
		SiteTags: []TagRecord{
			{ID: 50, Name: strp("女仆"), SiteID: int64p(10), SiteTagID: &siteTagID, Namespace: strp("character"), LocalTagID: int64p(40), CreateTime: 1, UpdateTime: 2},
		},
		WorkSets: []WorkSetRecord{
			{
				ID: 60, SiteID: int64p(10), SiteWorkSetName: strp("收藏夹"), SiteAuthorID: strp("sa-1"),
				CoverWorkID: int64p(70), CreateTime: 1, UpdateTime: 2,
				Parents: []WorkSetParentEdge{
					{ParentWorkSetID: 61, SortOrder: int64p(3), SiteSortOrder: int64p(1)},
				},
			},
		},
		Works: []WorkRecord{
			{
				ID: 70, SiteID: int64p(10), SiteWorkID: &siteWorkID, SiteWorkName: strp("作品名"),
				SiteAuthorID: strp("sa-1"), LocalAuthorID: int64p(20), LastView: int64p(300),
				CreateTime: 1, UpdateTime: 2,
				Resources: []ResourceRecord{
					{
						ID: 80, TaskID: int64p(90), SuggestName: strp("p1"), ResourceComplete: int64p(1),
						ResourceType: "image", CreateTime: 1, UpdateTime: 2,
						Stores: []StoreMount{
							{StoreType: "image", Generation: "downloaded", StoreSeq: 0, StoreID: 100},
						},
					},
				},
				TagLinks: []TagLink{
					{TagType: 1, TagID: 50, Namespace: strp("character")},
				},
				AuthorLinks: []AuthorLink{
					{AuthorType: 1, AuthorID: 30, RoleName: strp("artist"), SortOrder: int64p(0)},
				},
				WorkSetLinks: []WorkSetLink{
					{WorkSetID: 60, SortOrder: int64p(2), SiteSortOrder: int64p(0)},
				},
			},
		},
		Files: []FileEntry{
			{StoreID: 100, StorePath: "store/resource/画师/作品名.jpg", Path: "works/作品名/作品名.jpg", Size: 12345, ContentFingerprint: "12345:abc123", Sha256: "abc123", Missing: false},
		},
	}

	data, err := m.Serialize()
	require.NoError(t, err)

	back, err := Deserialize(data)
	require.NoError(t, err)
	require.NotNil(t, back)

	assert.Equal(t, m, back)

	// schemaVersion 与 siteKey 字段须如实序列化（回灌的版本锚与站点身份键）
	assert.Contains(t, string(data), `"schemaVersion": 2`)
	assert.Contains(t, string(data), `"siteKey": "pixiv"`)
}

// TestManifestEmptyPointerRoundTrip 空指针字段（omitempty）往返：序列化为 null 语义，反序列化为 nil。
func TestManifestEmptyPointerRoundTrip(t *testing.T) {
	m := &Manifest{
		SchemaVersion: SchemaVersion,
		Meta:          Meta{AppVersion: "0.0.1"},
		Works: []WorkRecord{
			{ID: 1, CreateTime: 1, UpdateTime: 1}, // 全部可空指针保持 nil
		},
	}

	data, err := m.Serialize()
	require.NoError(t, err)

	back, err := Deserialize(data)
	require.NoError(t, err)

	assert.Equal(t, m, back)
}

func strp(s string) *string { return &s }

func int64p(i int64) *int64 { return &i }
