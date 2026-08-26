package export

import (
	"context"
	"errors"
	"sort"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// 选择数量上限（决策5：id 列表透传）。万级全选时收集接口设上限保护，超出提示分批导出。
const (
	maxCollectWorkIDs    = 10000
	maxCollectWorkSetIDs = 5000
)

// localTagAncestorMaxDepth 本地标签祖先链追溯深度上限。标签树通常很浅，此上限为防御成环/深链
const localTagAncestorMaxDepth = 20

// ErrSelectionTooLarge 选择数量超上限
var ErrSelectionTooLarge = errors.New("导出选择数量过大，请分批导出")

// Collector 导出数据收集器：把选中作品/作品集按「选择即单位」闭包收集为内存态导出模型。
type Collector struct {
	repo            Repository
	versionProvider func() string
	now             func() int64
}

// NewCollector 创建导出数据收集器。
func NewCollector(repo Repository, versionProvider func() string) *Collector {
	return &Collector{
		repo:            repo,
		versionProvider: versionProvider,
		now:             util.GetCurrentTimestamp,
	}
}

// Collect 收集导出数据模型（决策5 接收形态：前端把选中 work/workSet id 列表透传）。
func (c *Collector) Collect(ctx context.Context, workIDs []int64, workSetIDs []int64) (*ExportModel, error) {
	if len(workIDs) > maxCollectWorkIDs || len(workSetIDs) > maxCollectWorkSetIDs {
		return nil, ErrSelectionTooLarge
	}

	// 1. 作品集闭包：选中作品集 + 递归后代（决策2：选作品集 → 导出其成员作品，含子作品集递归闭包）
	worksetClosure, err := computeWorkSetClosure(workSetIDs, func(id int64) ([]int64, error) {
		return c.repo.CollectDescendantWorkSetIds(ctx, id)
	})
	if err != nil {
		return nil, err
	}

	// 2. 作品集实体（活行；软删行经 GORM 自动排除，闭包随之收敛为活集）
	worksets, err := c.repo.ListWorkSetsByIds(ctx, worksetClosure)
	if err != nil {
		return nil, err
	}
	worksetMap := make(map[int64]*entity.WorkSet, len(worksets))
	worksetClosure = worksetClosure[:0]
	for _, ws := range worksets {
		worksetMap[ws.GetID()] = ws
		worksetClosure = append(worksetClosure, ws.GetID())
	}

	// 3. 作品集合：选中作品（活行）+ 闭包作品集的成员作品
	allWorkIDs, err := c.collectWorkIDs(ctx, workIDs, worksetClosure)
	if err != nil {
		return nil, err
	}
	works, err := c.repo.ListWorkByIds(ctx, allWorkIDs)
	if err != nil {
		return nil, err
	}
	workMap := make(map[int64]*entity.Work, len(works))
	for _, w := range works {
		workMap[w.GetID()] = w
	}

	// 4. 批量收集关联数据并组装 manifest
	return c.buildManifest(ctx, workMap, worksetMap)
}

// computeWorkSetClosure 计算作品集闭包（选中 + 递归后代，去重保序）。
// 抽为纯函数便于测试：后代收集经 collectDesc 注入（fake 可测）。
func computeWorkSetClosure(selected []int64, collectDesc func(id int64) ([]int64, error)) ([]int64, error) {
	closure := make([]int64, 0, len(selected))
	seen := make(map[int64]struct{}, len(selected))
	for _, id := range selected {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		closure = append(closure, id)

		descendants, err := collectDesc(id)
		if err != nil {
			return nil, err
		}
		for _, d := range descendants {
			if _, ok := seen[d]; !ok {
				seen[d] = struct{}{}
				closure = append(closure, d)
			}
		}
	}
	return closure, nil
}

// collectWorkIDs 合并选中作品与作品集闭包成员作品（去重，输入顺序在前）。
func (c *Collector) collectWorkIDs(ctx context.Context, workIDs []int64, worksetClosure []int64) ([]int64, error) {
	ids := make([]int64, 0, len(workIDs)+len(worksetClosure))
	seen := make(map[int64]struct{}, len(workIDs)+len(worksetClosure))
	for _, id := range workIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	memberWorkIds, err := c.repo.ListWorkIdsByWorkSetIds(ctx, worksetClosure)
	if err != nil {
		return nil, err
	}
	for _, id := range memberWorkIds {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

// buildManifest 批量收集全部关联数据并组装 manifest。
func (c *Collector) buildManifest(ctx context.Context, workMap map[int64]*entity.Work, worksetMap map[int64]*entity.WorkSet) (*ExportModel, error) {
	workIDs := sortedInt64Keys(workMap)
	worksetIDs := sortedInt64Keys(worksetMap)

	// ===== 资源与文件（persistent_store.file_path + resource_store 活行关联）=====
	resources, err := c.repo.ListResourcesByWorkIds(ctx, workIDs)
	if err != nil {
		return nil, err
	}
	resourceMap := make(map[int64][]*entity.Resource, len(resources))
	var allResourceIDs []int64
	for _, res := range resources {
		resourceMap[res.WorkID] = append(resourceMap[res.WorkID], res)
		allResourceIDs = append(allResourceIDs, res.GetID())
	}

	// resource_store 仅取活行（STORE_ASSOCIATION_LIVENESS_FILTER：软删行关联是残留代，不进导出）
	resourceStoreList, err := c.repo.ListLiveResourceStoresByResourceIds(ctx, allResourceIDs)
	if err != nil {
		return nil, err
	}
	resourceStoreMap := make(map[int64][]*entity.ResourceStore, len(resourceStoreList))
	storeIDSet := make(map[int64]struct{}, len(resourceStoreList))
	for _, rs := range resourceStoreList {
		resourceStoreMap[rs.ResourceID] = append(resourceStoreMap[rs.ResourceID], rs)
		if rs.StoreID > 0 {
			storeIDSet[rs.StoreID] = struct{}{}
		}
	}
	storeIDs := make([]int64, 0, len(storeIDSet))
	for id := range storeIDSet {
		storeIDs = append(storeIDs, id)
	}
	sort.Slice(storeIDs, func(i, j int) bool { return storeIDs[i] < storeIDs[j] })

	storeList, err := c.repo.ListPersistentStoresByIds(ctx, storeIDs)
	if err != nil {
		return nil, err
	}
	storeMap := make(map[int64]*entity.PersistentStore, len(storeList))
	for _, st := range storeList {
		storeMap[st.GetID()] = st
	}

	// ===== 标签关联（re_work_tag，含 namespace）=====
	reWorkTags, err := c.repo.ListReWorkTagsByWorkIds(ctx, workIDs)
	if err != nil {
		return nil, err
	}
	reWorkTagMap := make(map[int64][]*entity.ReWorkTag, len(workIDs))
	for _, t := range reWorkTags {
		if t.WorkID.Valid {
			reWorkTagMap[t.WorkID.Int64] = append(reWorkTagMap[t.WorkID.Int64], t)
		}
	}

	// ===== 作者关联（re_work_author）=====
	reWorkAuthors, err := c.repo.ListReWorkAuthorsByWorkIds(ctx, workIDs)
	if err != nil {
		return nil, err
	}
	reWorkAuthorMap := make(map[int64][]*entity.ReWorkAuthor, len(workIDs))
	for _, a := range reWorkAuthors {
		if a.WorkID.Valid {
			reWorkAuthorMap[a.WorkID.Int64] = append(reWorkAuthorMap[a.WorkID.Int64], a)
		}
	}

	// ===== 作品集成员关系（re_work_work_set；按「选择即单位」裁剪到闭包内）=====
	reWorkWorkSets, err := c.repo.ListReWorkWorkSetsByWorkIds(ctx, workIDs)
	if err != nil {
		return nil, err
	}
	reWorkWorkSetMap := make(map[int64][]*entity.ReWorkWorkSet, len(workIDs))
	for _, rel := range reWorkWorkSets {
		if rel.WorkID.Valid {
			reWorkWorkSetMap[rel.WorkID.Int64] = append(reWorkWorkSetMap[rel.WorkID.Int64], rel)
		}
	}

	// ===== 作品集间父子边（re_work_set_work_set；两端均在闭包内）=====
	reWorkSetWorkSets, err := c.repo.ListReWorkSetWorkSetsBetween(ctx, worksetIDs)
	if err != nil {
		return nil, err
	}
	reWorkSetWorkSetByChild := make(map[int64][]*entity.ReWorkSetWorkSet, len(worksetIDs))
	for _, edge := range reWorkSetWorkSets {
		if edge.ChildWorkSetID.Valid {
			reWorkSetWorkSetByChild[edge.ChildWorkSetID.Int64] = append(reWorkSetWorkSetByChild[edge.ChildWorkSetID.Int64], edge)
		}
	}

	// ===== 站点（work.site_id + work_set.site_id）=====
	siteRecords, err := c.collectSiteRecords(ctx, workMap, worksetMap)
	if err != nil {
		return nil, err
	}

	// ===== 标签（local_tag + site_tag，含站点侧 namespace 与本地标签祖先链）=====
	localTagRecords, siteTagRecords, err := c.collectTagRecords(ctx, reWorkTags)
	if err != nil {
		return nil, err
	}

	// ===== 作者（local_author + site_author，含 site→local 桥接）=====
	localAuthorRecords, siteAuthorRecords, err := c.collectAuthorRecords(ctx, reWorkAuthors, workMap)
	if err != nil {
		return nil, err
	}

	// ===== 文件条目（files[]：按活行关联纳入全部源文件，Path/Size/Sha256/Missing 阶段3 填充）=====
	fileEntries := c.buildFileEntries(storeList)

	// ===== 作品集记录（含层级父边）=====
	workSetRecords := c.buildWorkSetRecords(worksetIDs, worksetMap, reWorkSetWorkSetByChild)

	// ===== 作品记录（含资源挂载/标签/作者/作品集关联）=====
	workRecords := c.buildWorkRecords(workIDs, workMap, resourceMap, resourceStoreMap, storeMap,
		reWorkTagMap, reWorkAuthorMap, reWorkWorkSetMap, worksetMap)

	manifest := &Manifest{
		SchemaVersion: SchemaVersion,
		Meta: Meta{
			ExportedAt:       c.now(),
			AppVersion:       c.versionProvider(),
			SiteCount:        len(siteRecords),
			LocalAuthorCount: len(localAuthorRecords),
			SiteAuthorCount:  len(siteAuthorRecords),
			LocalTagCount:    len(localTagRecords),
			SiteTagCount:     len(siteTagRecords),
			WorkSetCount:     len(workSetRecords),
			WorkCount:        len(workRecords),
			FileCount:        len(fileEntries),
		},
		Sites:        siteRecords,
		LocalAuthors: localAuthorRecords,
		SiteAuthors:  siteAuthorRecords,
		LocalTags:    localTagRecords,
		SiteTags:     siteTagRecords,
		WorkSets:     workSetRecords,
		Works:        workRecords,
		Files:        fileEntries,
	}
	return NewExportModel(manifest), nil
}

// collectSiteRecords 收集站点记录（work.site_id + work_set.site_id 去重，按 ID 排序）。
func (c *Collector) collectSiteRecords(ctx context.Context, workMap map[int64]*entity.Work, worksetMap map[int64]*entity.WorkSet) ([]SiteRecord, error) {
	siteIDSet := make(map[int64]struct{}, len(workMap)+len(worksetMap))
	for _, w := range workMap {
		if w.SiteID.Valid && w.SiteID.Int64 > 0 {
			siteIDSet[w.SiteID.Int64] = struct{}{}
		}
	}
	for _, ws := range worksetMap {
		if ws.SiteID.Valid && ws.SiteID.Int64 > 0 {
			siteIDSet[ws.SiteID.Int64] = struct{}{}
		}
	}
	ids := sortedSetKeys(siteIDSet)
	if len(ids) == 0 {
		return nil, nil
	}
	sites, err := c.repo.ListSitesByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	records := make([]SiteRecord, 0, len(sites))
	for _, s := range sites {
		records = append(records, siteToRecord(s))
	}
	return records, nil
}

// collectTagRecords 收集标签记录：local_tag（含祖先链）+ site_tag（含站点侧 namespace、site→local 桥接）。
func (c *Collector) collectTagRecords(ctx context.Context, reWorkTags []*entity.ReWorkTag) ([]TagRecord, []TagRecord, error) {
	localTagIDSet := make(map[int64]struct{})
	siteTagIDSet := make(map[int64]struct{})
	for _, t := range reWorkTags {
		if t.LocalTagID.Valid && t.LocalTagID.Int64 > 0 {
			localTagIDSet[t.LocalTagID.Int64] = struct{}{}
		}
		if t.SiteTagID.Valid && t.SiteTagID.Int64 > 0 {
			siteTagIDSet[t.SiteTagID.Int64] = struct{}{}
		}
	}

	localTags, err := c.repo.ListLocalTagsByIds(ctx, sortedSetKeys(localTagIDSet))
	if err != nil {
		return nil, nil, err
	}
	localTagMap := make(map[int64]*entity.LocalTag, len(localTags))
	for _, t := range localTags {
		localTagMap[t.GetID()] = t
	}

	siteTags, err := c.repo.ListSiteTagsByIds(ctx, sortedSetKeys(siteTagIDSet))
	if err != nil {
		return nil, nil, err
	}
	siteTagMap := make(map[int64]*entity.SiteTag, len(siteTags))
	for _, t := range siteTags {
		siteTagMap[t.GetID()] = t
	}

	// site_tag.local_tag_id 桥接 → 桥接的本地标签须随行（否则回灌 site→local 桥接悬空）
	var bridgeLocalIDs []int64
	for _, st := range siteTagMap {
		if st.LocalTagID.Valid && st.LocalTagID.Int64 > 0 {
			if _, ok := localTagMap[st.LocalTagID.Int64]; !ok {
				if _, exists := localTagIDSet[st.LocalTagID.Int64]; !exists {
					localTagIDSet[st.LocalTagID.Int64] = struct{}{}
					bridgeLocalIDs = append(bridgeLocalIDs, st.LocalTagID.Int64)
				}
			}
		}
	}
	if len(bridgeLocalIDs) > 0 {
		bridged, err := c.repo.ListLocalTagsByIds(ctx, bridgeLocalIDs)
		if err != nil {
			return nil, nil, err
		}
		for _, t := range bridged {
			localTagMap[t.GetID()] = t
		}
	}

	// 本地标签祖先链：base_local_tag_id 层级完整连带导出，祖先缺失则回灌层级断裂
	if err := c.collectLocalTagAncestors(ctx, localTagMap); err != nil {
		return nil, nil, err
	}

	localRecords := make([]TagRecord, 0, len(localTagMap))
	for _, id := range sortedInt64Keys(localTagMap) {
		localRecords = append(localRecords, localTagToRecord(localTagMap[id]))
	}
	siteRecords := make([]TagRecord, 0, len(siteTagMap))
	for _, id := range sortedInt64Keys(siteTagMap) {
		siteRecords = append(siteRecords, siteTagToRecord(siteTagMap[id]))
	}
	return localRecords, siteRecords, nil
}

// collectLocalTagAncestors 迭代补齐本地标签的祖先链（base_local_tag_id 逐层追溯）。
// 引用悬空（父标签已删）时无新行可加，自然终止。
func (c *Collector) collectLocalTagAncestors(ctx context.Context, tagMap map[int64]*entity.LocalTag) error {
	for depth := 0; depth < localTagAncestorMaxDepth; depth++ {
		var missingIDs []int64
		missingSet := make(map[int64]struct{})
		for _, t := range tagMap {
			if t.BaseLocalTagID.Valid && t.BaseLocalTagID.Int64 > 0 {
				id := t.BaseLocalTagID.Int64
				if _, exists := tagMap[id]; exists {
					continue
				}
				if _, added := missingSet[id]; !added {
					missingSet[id] = struct{}{}
					missingIDs = append(missingIDs, id)
				}
			}
		}
		if len(missingIDs) == 0 {
			return nil
		}
		rows, err := c.repo.ListLocalTagsByIds(ctx, missingIDs)
		if err != nil {
			return err
		}
		added := false
		for _, t := range rows {
			if _, exists := tagMap[t.GetID()]; !exists {
				tagMap[t.GetID()] = t
				added = true
			}
		}
		if !added {
			return nil
		}
	}
	return nil
}

// collectAuthorRecords 收集作者记录：local_author + site_author（含 site→local 桥接与 work.local_author_id 镜像）。
func (c *Collector) collectAuthorRecords(ctx context.Context, reWorkAuthors []*entity.ReWorkAuthor, workMap map[int64]*entity.Work) ([]AuthorRecord, []AuthorRecord, error) {
	localAuthorIDSet := make(map[int64]struct{})
	siteAuthorIDSet := make(map[int64]struct{})
	for _, a := range reWorkAuthors {
		if a.LocalAuthorID.Valid && a.LocalAuthorID.Int64 > 0 {
			localAuthorIDSet[a.LocalAuthorID.Int64] = struct{}{}
		}
		if a.SiteAuthorID.Valid && a.SiteAuthorID.Int64 > 0 {
			siteAuthorIDSet[a.SiteAuthorID.Int64] = struct{}{}
		}
	}
	// work.local_author_id 镜像列（作品的主本地作者引用，未必有 re_work_author 行，仍须随行）
	for _, w := range workMap {
		if w.LocalAuthorID.Valid && w.LocalAuthorID.Int64 > 0 {
			localAuthorIDSet[w.LocalAuthorID.Int64] = struct{}{}
		}
	}

	localAuthors, err := c.repo.ListLocalAuthorsByIds(ctx, sortedSetKeys(localAuthorIDSet))
	if err != nil {
		return nil, nil, err
	}
	localAuthorMap := make(map[int64]*entity.LocalAuthor, len(localAuthors))
	for _, a := range localAuthors {
		localAuthorMap[a.GetID()] = a
	}

	siteAuthors, err := c.repo.ListSiteAuthorsByIds(ctx, sortedSetKeys(siteAuthorIDSet))
	if err != nil {
		return nil, nil, err
	}
	siteAuthorMap := make(map[int64]*entity.SiteAuthor, len(siteAuthors))
	for _, a := range siteAuthors {
		siteAuthorMap[a.GetID()] = a
	}

	// site_author.local_author_id 桥接 → 桥接的本地作者须随行
	var bridgeLocalIDs []int64
	for _, sa := range siteAuthorMap {
		if sa.LocalAuthorID.Valid && sa.LocalAuthorID.Int64 > 0 {
			if _, exists := localAuthorMap[sa.LocalAuthorID.Int64]; !exists {
				if _, inSet := localAuthorIDSet[sa.LocalAuthorID.Int64]; !inSet {
					localAuthorIDSet[sa.LocalAuthorID.Int64] = struct{}{}
					bridgeLocalIDs = append(bridgeLocalIDs, sa.LocalAuthorID.Int64)
				}
			}
		}
	}
	if len(bridgeLocalIDs) > 0 {
		bridged, err := c.repo.ListLocalAuthorsByIds(ctx, bridgeLocalIDs)
		if err != nil {
			return nil, nil, err
		}
		for _, a := range bridged {
			localAuthorMap[a.GetID()] = a
		}
	}

	localRecords := make([]AuthorRecord, 0, len(localAuthorMap))
	for _, id := range sortedInt64Keys(localAuthorMap) {
		localRecords = append(localRecords, localAuthorToRecord(localAuthorMap[id]))
	}
	siteRecords := make([]AuthorRecord, 0, len(siteAuthorMap))
	for _, id := range sortedInt64Keys(siteAuthorMap) {
		siteRecords = append(siteRecords, siteAuthorToRecord(siteAuthorMap[id]))
	}
	return localRecords, siteRecords, nil
}

// buildFileEntries 构建 files[] 条目（按活行关联纳入全部源文件；按 StoreID 排序保证确定性）。
// 阶段2 数据面只做纳入判定，Path/Size/Sha256/Missing 由阶段3 打包时填充。
func (c *Collector) buildFileEntries(storeList []*entity.PersistentStore) []FileEntry {
	entries := make([]FileEntry, 0, len(storeList))
	for _, st := range storeList {
		storePath := ""
		if st.FilePath.Valid {
			storePath = st.FilePath.String
		}
		entries = append(entries, FileEntry{
			StoreID:   st.GetID(),
			StorePath: storePath,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].StoreID < entries[j].StoreID })
	return entries
}

// buildWorkSetRecords 构建作品集记录（含父作品集引用边，仅两端均在闭包内的边）。
func (c *Collector) buildWorkSetRecords(
	worksetIDs []int64,
	worksetMap map[int64]*entity.WorkSet,
	reWorkSetWorkSetByChild map[int64][]*entity.ReWorkSetWorkSet,
) []WorkSetRecord {
	records := make([]WorkSetRecord, 0, len(worksetIDs))
	for _, id := range worksetIDs {
		ws := worksetMap[id]
		record := workSetToRecord(ws)
		for _, edge := range reWorkSetWorkSetByChild[id] {
			if edge.ParentWorkSetID.Valid && worksetMap[edge.ParentWorkSetID.Int64] != nil {
				record.Parents = append(record.Parents, WorkSetParentEdge{
					ParentWorkSetID: edge.ParentWorkSetID.Int64,
					SortOrder:       util.NullInt64ToPointer(edge.SortOrder),
					SiteSortOrder:   util.NullInt64ToPointer(edge.SiteSortOrder),
				})
			}
		}
		records = append(records, record)
	}
	return records
}

// buildWorkRecords 构建作品记录（含资源挂载/标签关联/作者关联/作品集成员关系）。
func (c *Collector) buildWorkRecords(
	workIDs []int64,
	workMap map[int64]*entity.Work,
	resourceMap map[int64][]*entity.Resource,
	resourceStoreMap map[int64][]*entity.ResourceStore,
	storeMap map[int64]*entity.PersistentStore,
	reWorkTagMap map[int64][]*entity.ReWorkTag,
	reWorkAuthorMap map[int64][]*entity.ReWorkAuthor,
	reWorkWorkSetMap map[int64][]*entity.ReWorkWorkSet,
	worksetMap map[int64]*entity.WorkSet,
) []WorkRecord {
	records := make([]WorkRecord, 0, len(workIDs))
	for _, id := range workIDs {
		w := workMap[id]
		record := workToRecord(w)

		// 资源与 store 挂载（活行）
		resources := resourceMap[id]
		sort.Slice(resources, func(i, j int) bool { return resources[i].GetID() < resources[j].GetID() })
		for _, res := range resources {
			resRec := resourceToRecord(res)
			stores := resourceStoreMap[res.GetID()]
			sort.Slice(stores, func(i, j int) bool {
				if stores[i].StoreSeq != stores[j].StoreSeq {
					return stores[i].StoreSeq < stores[j].StoreSeq
				}
				return stores[i].StoreType < stores[j].StoreType
			})
			for _, rs := range stores {
				// storeMap 由活行批量查询构建：未命中的关联指向软删行（liveness 过滤的兜底）
				if storeMap[rs.StoreID] == nil {
					continue
				}
				resRec.Stores = append(resRec.Stores, StoreMount{
					StoreType:  rs.StoreType,
					Generation: rs.Generation,
					StoreSeq:   rs.StoreSeq,
					StoreID:    rs.StoreID,
				})
			}
			record.Resources = append(record.Resources, resRec)
		}

		// 标签关联（含 namespace）
		tagLinks := reWorkTagMap[id]
		sort.Slice(tagLinks, func(i, j int) bool {
			if tagLinks[i].TagType.Int64 != tagLinks[j].TagType.Int64 {
				return tagLinks[i].TagType.Int64 < tagLinks[j].TagType.Int64
			}
			return reWorkTagLinkKey(tagLinks[i]) < reWorkTagLinkKey(tagLinks[j])
		})
		for _, t := range tagLinks {
			if link, ok := reWorkTagToLink(t); ok {
				record.TagLinks = append(record.TagLinks, link)
			}
		}

		// 作者关联
		authorLinks := reWorkAuthorMap[id]
		sort.Slice(authorLinks, func(i, j int) bool {
			if authorLinks[i].AuthorType.Int64 != authorLinks[j].AuthorType.Int64 {
				return authorLinks[i].AuthorType.Int64 < authorLinks[j].AuthorType.Int64
			}
			return reWorkAuthorLinkKey(authorLinks[i]) < reWorkAuthorLinkKey(authorLinks[j])
		})
		for _, a := range authorLinks {
			if link, ok := reWorkAuthorToLink(a); ok {
				record.AuthorLinks = append(record.AuthorLinks, link)
			}
		}

		// 作品集成员关系（「选择即单位」：仅保留作品集在导出闭包内的关联）
		workSetLinks := reWorkWorkSetMap[id]
		sort.Slice(workSetLinks, func(i, j int) bool {
			return workSetLinks[i].WorkSetID.Int64 < workSetLinks[j].WorkSetID.Int64
		})
		for _, rel := range workSetLinks {
			if rel.WorkSetID.Valid && worksetMap[rel.WorkSetID.Int64] != nil {
				record.WorkSetLinks = append(record.WorkSetLinks, reWorkWorkSetToLink(rel))
			}
		}

		records = append(records, record)
	}
	return records
}

// ===== 实体 → manifest 记录转换 =====

func workToRecord(w *entity.Work) WorkRecord {
	return WorkRecord{
		ID:                  w.GetID(),
		SiteID:              util.NullInt64ToPointer(w.SiteID),
		SiteWorkID:          util.NullStringToPointer(w.SiteWorkID),
		SiteWorkName:        util.NullStringToPointer(w.SiteWorkName),
		SiteAuthorID:        util.NullStringToPointer(w.SiteAuthorID),
		SiteWorkDescription: util.NullStringToPointer(w.SiteWorkDescription),
		SiteUploadTime:      util.NullInt64ToPointer(w.SiteUploadTime),
		SiteUpdateTime:      util.NullInt64ToPointer(w.SiteUpdateTime),
		NickName:            util.NullStringToPointer(w.NickName),
		LocalAuthorID:       util.NullInt64ToPointer(w.LocalAuthorID),
		LastView:            util.NullInt64ToPointer(w.LastView),
		CreateTime:          w.GetCreateTime(),
		UpdateTime:          w.GetUpdateTime(),
	}
}

func workSetToRecord(ws *entity.WorkSet) WorkSetRecord {
	return WorkSetRecord{
		ID:                     ws.GetID(),
		SiteID:                 util.NullInt64ToPointer(ws.SiteID),
		SiteWorkSetID:          util.NullStringToPointer(ws.SiteWorkSetID),
		SiteWorkSetName:        util.NullStringToPointer(ws.SiteWorkSetName),
		SiteAuthorID:           util.NullStringToPointer(ws.SiteAuthorID),
		SiteWorkSetDescription: util.NullStringToPointer(ws.SiteWorkSetDescription),
		SiteUploadTime:         util.NullInt64ToPointer(ws.SiteUploadTime),
		SiteUpdateTime:         util.NullInt64ToPointer(ws.SiteUpdateTime),
		NickName:               util.NullStringToPointer(ws.NickName),
		Description:            util.NullStringToPointer(ws.Description),
		LastView:               util.NullInt64ToPointer(ws.LastView),
		CoverWorkID:            util.NullInt64ToPointer(ws.CoverWorkID),
		CreateTime:             ws.GetCreateTime(),
		UpdateTime:             ws.GetUpdateTime(),
	}
}

func resourceToRecord(res *entity.Resource) ResourceRecord {
	return ResourceRecord{
		ID:               res.GetID(),
		TaskID:           util.NullInt64ToPointer(res.TaskID),
		SuggestName:      util.NullStringToPointer(res.SuggestName),
		ResourceComplete: util.NullInt64ToPointer(res.ResourceComplete),
		ResourceType:     res.ResourceType,
		CreateTime:       res.GetCreateTime(),
		UpdateTime:       res.GetUpdateTime(),
	}
}

func siteToRecord(s *entity.Site) SiteRecord {
	return SiteRecord{
		ID:              s.GetID(),
		SiteName:        util.NullStringToPointer(s.SiteName),
		SiteDescription: util.NullStringToPointer(s.SiteDescription),
		Homepage:        util.NullStringToPointer(s.Homepage),
		CreateTime:      s.GetCreateTime(),
		UpdateTime:      s.GetUpdateTime(),
	}
}

func localAuthorToRecord(a *entity.LocalAuthor) AuthorRecord {
	return AuthorRecord{
		ID:         a.GetID(),
		Name:       util.NullStringToPointer(a.AuthorName),
		Introduce:  util.NullStringToPointer(a.Introduce),
		LastUse:    util.NullInt64ToPointer(a.LastUse),
		CreateTime: a.GetCreateTime(),
		UpdateTime: a.GetUpdateTime(),
	}
}

func siteAuthorToRecord(a *entity.SiteAuthor) AuthorRecord {
	return AuthorRecord{
		ID:                   a.GetID(),
		Name:                 util.NullStringToPointer(a.AuthorName),
		SiteID:               util.NullInt64ToPointer(a.SiteID),
		SiteAuthorID:         util.NullStringToPointer(a.SiteAuthorID),
		FixedAuthorName:      util.NullStringToPointer(a.FixedAuthorName),
		SiteAuthorNameBefore: util.NullStringToPointer(a.SiteAuthorNameBefore),
		Homepage:             util.NullStringToPointer(a.Homepage),
		LocalAuthorID:        util.NullInt64ToPointer(a.LocalAuthorID),
		Introduce:            util.NullStringToPointer(a.Introduce),
		LastUse:              util.NullInt64ToPointer(a.LastUse),
		CreateTime:           a.GetCreateTime(),
		UpdateTime:           a.GetUpdateTime(),
	}
}

func localTagToRecord(t *entity.LocalTag) TagRecord {
	return TagRecord{
		ID:             t.GetID(),
		Name:           util.NullStringToPointer(t.LocalTagName),
		BaseLocalTagID: util.NullInt64ToPointer(t.BaseLocalTagID),
		Description:    util.NullStringToPointer(t.Description),
		LastUse:        util.NullInt64ToPointer(t.LastUse),
		CreateTime:     t.GetCreateTime(),
		UpdateTime:     t.GetUpdateTime(),
	}
}

func siteTagToRecord(t *entity.SiteTag) TagRecord {
	return TagRecord{
		ID:            t.GetID(),
		Name:          util.NullStringToPointer(t.SiteTagName),
		SiteID:        util.NullInt64ToPointer(t.SiteID),
		SiteTagID:     util.NullStringToPointer(t.SiteTagID),
		BaseSiteTagID: util.NullStringToPointer(t.BaseSiteTagID),
		Namespace:     util.NullStringToPointer(t.Namespace),
		LocalTagID:    util.NullInt64ToPointer(t.LocalTagID),
		Description:   util.NullStringToPointer(t.Description),
		LastUse:       util.NullInt64ToPointer(t.LastUse),
		CreateTime:    t.GetCreateTime(),
		UpdateTime:    t.GetUpdateTime(),
	}
}

func reWorkTagToLink(t *entity.ReWorkTag) (TagLink, bool) {
	if t.LocalTagID.Valid && t.LocalTagID.Int64 > 0 {
		return TagLink{TagType: constant.LOCAL, TagID: t.LocalTagID.Int64, Namespace: util.NullStringToPointer(t.Namespace)}, true
	}
	if t.SiteTagID.Valid && t.SiteTagID.Int64 > 0 {
		return TagLink{TagType: constant.SITE, TagID: t.SiteTagID.Int64, Namespace: util.NullStringToPointer(t.Namespace)}, true
	}
	return TagLink{}, false
}

func reWorkAuthorToLink(a *entity.ReWorkAuthor) (AuthorLink, bool) {
	if a.LocalAuthorID.Valid && a.LocalAuthorID.Int64 > 0 {
		return AuthorLink{AuthorType: constant.LOCAL, AuthorID: a.LocalAuthorID.Int64, RoleName: util.NullStringToPointer(a.RoleName), SortOrder: util.NullInt64ToPointer(a.SortOrder)}, true
	}
	if a.SiteAuthorID.Valid && a.SiteAuthorID.Int64 > 0 {
		return AuthorLink{AuthorType: constant.SITE, AuthorID: a.SiteAuthorID.Int64, RoleName: util.NullStringToPointer(a.RoleName), SortOrder: util.NullInt64ToPointer(a.SortOrder)}, true
	}
	return AuthorLink{}, false
}

func reWorkWorkSetToLink(rel *entity.ReWorkWorkSet) WorkSetLink {
	return WorkSetLink{
		WorkSetID:     rel.WorkSetID.Int64,
		SortOrder:     util.NullInt64ToPointer(rel.SortOrder),
		SiteSortOrder: util.NullInt64ToPointer(rel.SiteSortOrder),
	}
}

// reWorkTagLinkKey 排序键：按所指 tag 行 ID（tag_id 区分 local/site，同一 type 内稳定）
func reWorkTagLinkKey(t *entity.ReWorkTag) int64 {
	if t.LocalTagID.Valid && t.LocalTagID.Int64 > 0 {
		return t.LocalTagID.Int64
	}
	return t.SiteTagID.Int64
}

// reWorkAuthorLinkKey 排序键：按所指 author 行 ID
func reWorkAuthorLinkKey(a *entity.ReWorkAuthor) int64 {
	if a.LocalAuthorID.Valid && a.LocalAuthorID.Int64 > 0 {
		return a.LocalAuthorID.Int64
	}
	return a.SiteAuthorID.Int64
}

// ===== 通用工具 =====

// sortedInt64Keys 返回 map 的 int64 键（升序，保证产物确定性）。
func sortedInt64Keys[T any](m map[int64]T) []int64 {
	keys := make([]int64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

// sortedSetKeys 返回 set（map[int64]struct{}）的键（升序）。
func sortedSetKeys(m map[int64]struct{}) []int64 {
	return sortedInt64Keys(m)
}
