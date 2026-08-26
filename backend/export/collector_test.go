package export

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ===== 纯函数测试 =====

// TestComputeWorkSetClosure 锚定作品集闭包计算（选中 + 递归后代，去重保序）。
// collectDesc 与仓库 CollectDescendantWorkSetIds 语义一致：返回节点的全部后代（含深层）。
func TestComputeWorkSetClosure(t *testing.T) {
	// 1 的全部后代：3、4、5（4 经 5）；2 的全部后代：6
	desc := map[int64][]int64{1: {3, 4, 5}, 2: {6}, 3: {}, 4: {5}, 5: {}, 6: {}}
	collect := func(id int64) ([]int64, error) { return desc[id], nil }

	t.Run("选中+后代递归闭包", func(t *testing.T) {
		closure, err := computeWorkSetClosure([]int64{1, 2}, collect)
		require.NoError(t, err)
		assert.Equal(t, []int64{1, 3, 4, 5, 2, 6}, closure)
	})

	t.Run("选中项为后代时去重", func(t *testing.T) {
		closure, err := computeWorkSetClosure([]int64{1, 3}, collect)
		require.NoError(t, err)
		assert.Equal(t, []int64{1, 3, 4, 5}, closure)
	})

	t.Run("空选择返回空", func(t *testing.T) {
		closure, err := computeWorkSetClosure(nil, collect)
		require.NoError(t, err)
		assert.Empty(t, closure)
	})

	t.Run("后代收集错误传播", func(t *testing.T) {
		_, err := computeWorkSetClosure([]int64{1}, func(id int64) ([]int64, error) {
			return nil, errors.New("boom")
		})
		assert.Error(t, err)
	})
}

// TestCollectSelectionTooLarge 决策5 上限保护：超限报错提示分批。
func TestCollectSelectionTooLarge(t *testing.T) {
	c := NewCollector(nil, func() string { return "test" })
	huge := make([]int64, maxCollectWorkIDs+1)
	_, err := c.Collect(context.Background(), huge, nil)
	assert.ErrorIs(t, err, ErrSelectionTooLarge)

	_, err = c.Collect(context.Background(), nil, make([]int64, maxCollectWorkSetIDs+1))
	assert.ErrorIs(t, err, ErrSelectionTooLarge)
}

// ===== 集成测试（内存 SQLite）=====

// exportFixture 集成测试数据夹具
type exportFixture struct {
	siteID     int64
	laID       int64 // 本地作者
	saID       int64 // 站点作者
	ltParentID int64 // 本地标签根
	ltChildID  int64 // 本地标签子（挂父）
	stID       int64 // 站点标签（namespace + 桥接子标签）
	w1ID       int64
	w2ID       int64
	w3ID       int64
	w4ID       int64
	wsAID      int64 // 作品集 A（父）
	wsBID      int64 // 作品集 B（A 的子集）
	wsCID      int64 // 作品集 C（未选中集）
	s1ID       int64 // 活行 store
	s2ID       int64 // 软删 store
	r1ID       int64 // 作品1 的资源
}

// seedExportFixture 播种导出测试数据
func seedExportFixture(t *testing.T, db *gorm.DB) *exportFixture {
	t.Helper()
	f := &exportFixture{}

	site := entity.NewSite()
	site.SiteName = ns("pixiv")
	require.NoError(t, db.Create(site).Error)
	f.siteID = site.GetID()

	la := entity.NewLocalAuthor()
	la.AuthorName = ns("画师A")
	require.NoError(t, db.Create(la).Error)
	f.laID = la.GetID()

	sa := entity.NewSiteAuthor()
	sa.SiteID = ni(f.siteID)
	sa.SiteAuthorID = ns("sa-1")
	sa.AuthorName = ns("站画师A")
	sa.LocalAuthorID = ni(f.laID)
	require.NoError(t, db.Create(sa).Error)
	f.saID = sa.GetID()

	ltParent := entity.NewLocalTag()
	ltParent.LocalTagName = ns("父标签")
	require.NoError(t, db.Create(ltParent).Error)
	f.ltParentID = ltParent.GetID()

	ltChild := entity.NewLocalTag()
	ltChild.LocalTagName = ns("子标签")
	ltChild.BaseLocalTagID = ni(f.ltParentID)
	require.NoError(t, db.Create(ltChild).Error)
	f.ltChildID = ltChild.GetID()

	st := entity.NewSiteTag()
	st.SiteID = ni(f.siteID)
	st.SiteTagID = ns("st-1")
	st.SiteTagName = ns("女仆")
	st.Namespace = ns("character")
	st.LocalTagID = ni(f.ltChildID)
	require.NoError(t, db.Create(st).Error)
	f.stID = st.GetID()

	w1 := entity.NewWork()
	w1.SiteID = ni(f.siteID)
	w1.SiteWorkID = ns("w-1")
	w1.SiteWorkName = ns("作品1")
	w1.LocalAuthorID = ni(f.laID)
	require.NoError(t, db.Create(w1).Error)
	f.w1ID = w1.GetID()

	w2 := entity.NewWork()
	w2.SiteID = ni(f.siteID)
	w2.SiteWorkID = ns("w-2")
	w2.SiteWorkName = ns("作品2")
	require.NoError(t, db.Create(w2).Error)
	f.w2ID = w2.GetID()

	w3 := entity.NewWork()
	w3.SiteID = ni(f.siteID)
	w3.SiteWorkID = ns("w-3")
	w3.SiteWorkName = ns("作品3")
	require.NoError(t, db.Create(w3).Error)
	f.w3ID = w3.GetID()

	w4 := entity.NewWork()
	w4.SiteID = ni(f.siteID)
	w4.SiteWorkID = ns("w-4")
	w4.SiteWorkName = ns("作品4")
	require.NoError(t, db.Create(w4).Error)
	f.w4ID = w4.GetID()

	wsA := entity.NewWorkSet()
	wsA.SiteID = ni(f.siteID)
	wsA.SiteWorkSetName = ns("集A")
	require.NoError(t, db.Create(wsA).Error)
	f.wsAID = wsA.GetID()

	wsB := entity.NewWorkSet()
	wsB.SiteID = ni(f.siteID)
	wsB.SiteWorkSetName = ns("集B")
	require.NoError(t, db.Create(wsB).Error)
	f.wsBID = wsB.GetID()

	wsC := entity.NewWorkSet()
	wsC.SiteID = ni(f.siteID)
	wsC.SiteWorkSetName = ns("集C")
	require.NoError(t, db.Create(wsC).Error)
	f.wsCID = wsC.GetID()

	// 作品-标签关联：W1 挂本地子标签 + 站点标签（含 namespace）
	rwt1 := entity.NewReWorkTag()
	rwt1.WorkID = ni(f.w1ID)
	rwt1.TagType = ni(int64(constant.LOCAL))
	rwt1.LocalTagID = ni(f.ltChildID)
	require.NoError(t, db.Create(rwt1).Error)

	rwt2 := entity.NewReWorkTag()
	rwt2.WorkID = ni(f.w1ID)
	rwt2.TagType = ni(int64(constant.SITE))
	rwt2.SiteTagID = ni(f.stID)
	rwt2.Namespace = ns("character")
	require.NoError(t, db.Create(rwt2).Error)

	// 作品-作者关联：W1 挂本地作者 + 站点作者（re_work_author 无工厂方法，生产代码同用字面量构建）
	rwa1 := &entity.ReWorkAuthor{
		BaseEntity:    &model.BaseEntity{},
		AuthorType:    ni(int64(constant.LOCAL)),
		WorkID:        ni(f.w1ID),
		LocalAuthorID: ni(f.laID),
	}
	require.NoError(t, db.Create(rwa1).Error)

	rwa2 := &entity.ReWorkAuthor{
		BaseEntity:   &model.BaseEntity{},
		AuthorType:   ni(int64(constant.SITE)),
		WorkID:       ni(f.w1ID),
		SiteAuthorID: ni(f.saID),
	}
	require.NoError(t, db.Create(rwa2).Error)

	// 作品-作品集成员关系：W1∈{A,B}、W2∈{A}、W3∈{B}、W4∈{C}
	seedWorkSetLink(t, db, f.w1ID, f.wsAID, 0)
	seedWorkSetLink(t, db, f.w1ID, f.wsBID, 1)
	seedWorkSetLink(t, db, f.w2ID, f.wsAID, 0)
	seedWorkSetLink(t, db, f.w3ID, f.wsBID, 0)
	seedWorkSetLink(t, db, f.w4ID, f.wsCID, 0)

	// 作品集间父子：A→B（B 是 A 的子集）
	edge := entity.NewReWorkSetWorkSet()
	edge.ParentWorkSetID = ni(f.wsAID)
	edge.ChildWorkSetID = ni(f.wsBID)
	edge.SortOrder = ni(0)
	require.NoError(t, db.Create(edge).Error)

	// 资源与 store：R1 挂两个 store，其一活行、其一软删（liveness 过滤用）
	r1 := entity.NewResource()
	r1.WorkID = f.w1ID
	r1.ResourceType = "image"
	require.NoError(t, db.Create(r1).Error)
	f.r1ID = r1.GetID()

	s1 := entity.NewPersistentStore()
	s1.FilePath = ns("store/resource/作品1.jpg")
	s1.FileName = ns("作品1.jpg")
	s1.CompletedAt = 100
	require.NoError(t, db.Create(s1).Error)
	f.s1ID = s1.GetID()

	s2 := entity.NewPersistentStore()
	s2.FilePath = ns("store/resource/作品1_thumb.jpg")
	s2.FileName = ns("作品1_thumb.jpg")
	s2.CompletedAt = 100
	require.NoError(t, db.Create(s2).Error)
	require.NoError(t, db.Delete(s2).Error) // 软删
	f.s2ID = s2.GetID()

	rs1 := entity.NewResourceStore()
	rs1.ResourceID = f.r1ID
	rs1.StoreType = entity.StoreTypeImage
	rs1.Generation = entity.GenerationDownloaded
	rs1.StoreID = f.s1ID
	rs1.StoreSeq = 0
	require.NoError(t, db.Create(rs1).Error)

	rs2 := entity.NewResourceStore()
	rs2.ResourceID = f.r1ID
	rs2.StoreType = entity.StoreTypeThumbnail
	rs2.Generation = entity.GenerationDerived
	rs2.StoreID = f.s2ID
	rs2.StoreSeq = 1
	require.NoError(t, db.Create(rs2).Error)

	return f
}

func seedWorkSetLink(t *testing.T, db *gorm.DB, workID, workSetID int64, sortOrder int) {
	t.Helper()
	rel := entity.NewReWorkWorkSet()
	rel.WorkID = ni(workID)
	rel.WorkSetID = ni(workSetID)
	rel.SortOrder = ni(int64(sortOrder))
	require.NoError(t, db.Create(rel).Error)
}

func newTestCollector(db *gorm.DB) *Collector {
	return NewCollector(NewRepository(db), func() string { return "test-version" })
}

func TestCollectSelectionUnit(t *testing.T) {
	db, err := migration.OpenTestDB()
	require.NoError(t, err)
	f := seedExportFixture(t, db)
	c := newTestCollector(db)
	ctx := context.Background()

	t.Run("选作品集A：闭包含子集B，成员作品W1/W2/W3，指向未选集C的关系丢弃", func(t *testing.T) {
		model, err := c.Collect(ctx, nil, []int64{f.wsAID})
		require.NoError(t, err)
		m := model.Manifest

		// 作品集：A + B（B 为 A 后代），不含 C
		require.Len(t, m.WorkSets, 2)
		var bRec *WorkSetRecord
		for i := range m.WorkSets {
			if m.WorkSets[i].ID == f.wsBID {
				bRec = &m.WorkSets[i]
			}
		}
		require.NotNil(t, bRec, "子集 B 应在导出闭包内")
		require.Len(t, bRec.Parents, 1)
		assert.Equal(t, f.wsAID, bRec.Parents[0].ParentWorkSetID)

		// 作品：W1/W2/W3（A、B 成员），W4（仅属未选集 C）不导出
		workIDs := collectWorkRecordIDs(m)
		assert.ElementsMatch(t, []int64{f.w1ID, f.w2ID, f.w3ID}, workIDs)

		// W1 的作品集成员关系：A + B（跨选择项保留）
		w1Rec := findWorkRecord(m, f.w1ID)
		require.NotNil(t, w1Rec)
		assert.ElementsMatch(t, []int64{f.wsAID, f.wsBID}, collectWorkSetLinkIDs(w1Rec))
		w2Rec := findWorkRecord(m, f.w2ID)
		require.NotNil(t, w2Rec)
		assert.ElementsMatch(t, []int64{f.wsAID}, collectWorkSetLinkIDs(w2Rec))
		w3Rec := findWorkRecord(m, f.w3ID)
		require.NotNil(t, w3Rec)
		assert.ElementsMatch(t, []int64{f.wsBID}, collectWorkSetLinkIDs(w3Rec))

		// 资源与文件：R1 只挂活行 store（软删 S2 排除）
		require.Len(t, w1Rec.Resources, 1)
		require.Len(t, w1Rec.Resources[0].Stores, 1)
		assert.Equal(t, entity.StoreTypeImage, w1Rec.Resources[0].Stores[0].StoreType)
		assert.Equal(t, f.s1ID, w1Rec.Resources[0].Stores[0].StoreID)

		require.Len(t, m.Files, 1)
		assert.Equal(t, f.s1ID, m.Files[0].StoreID)
		assert.Equal(t, "store/resource/作品1.jpg", m.Files[0].StorePath)
		assert.False(t, m.Files[0].Missing) // 阶段2 数据面：按活行关联纳入，缺失标记阶段3 判定

		// 标签：本地含祖先链（子+父），站点含 namespace
		require.Len(t, m.LocalTags, 2)
		require.Len(t, m.SiteTags, 1)
		assert.Equal(t, "character", *m.SiteTags[0].Namespace)

		// W1 标签关联：本地子标签 + 站点标签（namespace 随行）
		assert.Len(t, w1Rec.TagLinks, 2)
		siteTagLink := findTagLinkByType(w1Rec, constant.SITE)
		require.NotNil(t, siteTagLink)
		assert.Equal(t, f.stID, siteTagLink.TagID)
		require.NotNil(t, siteTagLink.Namespace)
		assert.Equal(t, "character", *siteTagLink.Namespace)

		// 作者：本地 + 站点（site→local 桥接随行）
		require.Len(t, m.LocalAuthors, 1)
		require.Len(t, m.SiteAuthors, 1)
		assert.Len(t, w1Rec.AuthorLinks, 2)

		// 站点
		require.Len(t, m.Sites, 1)
		assert.Equal(t, "pixiv", *m.Sites[0].SiteName)

		// 计数
		assert.Equal(t, 2, m.Meta.WorkSetCount)
		assert.Equal(t, 3, m.Meta.WorkCount)
		assert.Equal(t, 1, m.Meta.FileCount)
	})

	t.Run("只选作品W2：作品集未选，成员关系丢弃", func(t *testing.T) {
		model, err := c.Collect(ctx, []int64{f.w2ID}, nil)
		require.NoError(t, err)
		m := model.Manifest

		require.Len(t, m.Works, 1)
		assert.Equal(t, f.w2ID, m.Works[0].ID)
		assert.Empty(t, m.Works[0].WorkSetLinks) // A 未选，指向它的成员关系丢弃
		assert.Empty(t, m.WorkSets)
		assert.Empty(t, m.Files)
	})

	t.Run("选作品集A+作品W4：W4 直接选中被导出，但其指向未选集C的关系丢弃", func(t *testing.T) {
		model, err := c.Collect(ctx, []int64{f.w4ID}, []int64{f.wsAID})
		require.NoError(t, err)
		m := model.Manifest

		workIDs := collectWorkRecordIDs(m)
		assert.ElementsMatch(t, []int64{f.w1ID, f.w2ID, f.w3ID, f.w4ID}, workIDs)

		w4Rec := findWorkRecord(m, f.w4ID)
		require.NotNil(t, w4Rec)
		assert.Empty(t, w4Rec.WorkSetLinks) // C 未选，关系丢弃
	})

	t.Run("选作品集C：仅导出C及其成员W4", func(t *testing.T) {
		model, err := c.Collect(ctx, nil, []int64{f.wsCID})
		require.NoError(t, err)
		m := model.Manifest

		require.Len(t, m.WorkSets, 1)
		assert.Equal(t, f.wsCID, m.WorkSets[0].ID)
		require.Len(t, m.Works, 1)
		assert.Equal(t, f.w4ID, m.Works[0].ID)
		assert.Len(t, m.Works[0].WorkSetLinks, 1)
		assert.Equal(t, f.wsCID, m.Works[0].WorkSetLinks[0].WorkSetID)
	})

	t.Run("空选择：空导出模型", func(t *testing.T) {
		model, err := c.Collect(ctx, nil, nil)
		require.NoError(t, err)
		m := model.Manifest
		assert.Empty(t, m.Works)
		assert.Empty(t, m.WorkSets)
		assert.Empty(t, m.Files)
		assert.Zero(t, m.Meta.WorkCount)
	})
}

// ===== 测试辅助 =====

func collectWorkRecordIDs(m *Manifest) []int64 {
	ids := make([]int64, 0, len(m.Works))
	for _, w := range m.Works {
		ids = append(ids, w.ID)
	}
	return ids
}

func findWorkRecord(m *Manifest, id int64) *WorkRecord {
	for i := range m.Works {
		if m.Works[i].ID == id {
			return &m.Works[i]
		}
	}
	return nil
}

func collectWorkSetLinkIDs(w *WorkRecord) []int64 {
	ids := make([]int64, 0, len(w.WorkSetLinks))
	for _, l := range w.WorkSetLinks {
		ids = append(ids, l.WorkSetID)
	}
	return ids
}

func findTagLinkByType(w *WorkRecord, tagType int) *TagLink {
	for i := range w.TagLinks {
		if w.TagLinks[i].TagType == tagType {
			return &w.TagLinks[i]
		}
	}
	return nil
}

func ns(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
func ni(i int64) sql.NullInt64   { return sql.NullInt64{Int64: i, Valid: true} }
