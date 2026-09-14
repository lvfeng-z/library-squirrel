package extension

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/util"
	"github.com/library-squirrel/backend/work"
	"github.com/lvfeng-z/library-squirrel-sdk/gen"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// ===== Tier 1 库查询：宿主侧查询核心 =====
// 实现 SDK LibraryQuery 契约的库内只读查询：查询统一走各域 repository 的 GORM 管线
// （软删行经 scope 自动排除；resource_store 关联经活行过滤），provider 不自拼 SQL。
// 错误以 gRPC status 携带语义码：Get* 未命中 NotFound、GetWorkDir 未配置 FailedPrecondition。

// 库查询分页边界：page 从 1 起；page_size 缺省/越界的钳制值（数据库单连接，重查询需节制）
const (
	defaultQueryPageSize = 20
	maxQueryPageSize     = 200
)

// clampPage 钳制分页参数到合法区间（page<1 按 1；page_size<1 取默认、超上限截到上限）
func clampPage(page, pageSize int32) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultQueryPageSize
	}
	if pageSize > maxQueryPageSize {
		pageSize = maxQueryPageSize
	}
	return int(page), int(pageSize)
}

// pageRequestOf 取请求的分页参数并钳制（请求未携带分页时按缺省处理）
func pageRequestOf(p *gen.PageRequest) (int, int) {
	if p == nil {
		return 1, defaultQueryPageSize
	}
	return clampPage(p.Page, p.PageSize)
}

// pageInfoOf 由仓储分页结果构造回执
func pageInfoOf(total int64, page, pageSize int) *gen.PageInfo {
	return &gen.PageInfo{Total: total, Page: int32(page), PageSize: int32(pageSize)}
}

// --- 注入接口（调用方 extension 定义，各域 repository / settings.Service 实现）---

// WorkQuerySource work 域只读查询面（work.WorkRepository 实现）
type WorkQuerySource interface {
	GetById(ctx context.Context, id int64) (*entity.Work, error)
	GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*entity.Work, error)
	PageByFilter(ctx context.Context, filter *work.WorkQueryFilter, page, pageSize int) (*model.Page[entity.Work], error)
}

// SiteQuerySource site 域只读查询面（site.SiteRepository 实现）
type SiteQuerySource interface {
	GetByKey(ctx context.Context, siteKey string) (*entity.Site, error)
	ListAll(ctx context.Context) ([]*entity.Site, error)
}

// ResourceQuerySource resource 域只读查询面（resource.ResourceRepository 实现）
type ResourceQuerySource interface {
	ListByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error)
}

// ResourceStoreQuerySource resource_store 关联只读查询面（resource.ResourceStoreRepository 实现）
type ResourceStoreQuerySource interface {
	// ListAliveByResourceIds 仅返回指向活行 store 的关联（软删残留关联不出现）
	ListAliveByResourceIds(ctx context.Context, resourceIds []int64) ([]*entity.ResourceStore, error)
}

// PersistentStoreQuerySource persistent_store 域只读查询面（persistentStore.PersistentStoreRepository 实现）
type PersistentStoreQuerySource interface {
	ListByIds(ctx context.Context, ids []int64) ([]*entity.PersistentStore, error)
}

// LocalAuthorQuerySource 本地作者只读查询面（localAuthor.LocalAuthorRepository 实现）
type LocalAuthorQuerySource interface {
	GetById(ctx context.Context, id int64) (*entity.LocalAuthor, error)
	PageByNameKeyword(ctx context.Context, nameKeyword string, page, pageSize int) (*model.Page[entity.LocalAuthor], error)
	ListByWorkId(ctx context.Context, workId int64) ([]*dto.RankedLocalAuthor, error)
}

// SiteAuthorQuerySource 站点作者只读查询面（siteAuthor.SiteAuthorRepository 实现）
type SiteAuthorQuerySource interface {
	GetBySiteAndSiteAuthorID(ctx context.Context, siteId int64, siteAuthorId string) (*entity.SiteAuthor, error)
	PageByFilter(ctx context.Context, siteId int64, nameKeyword string, page, pageSize int) (*model.Page[entity.SiteAuthor], error)
	ListByWorkId(ctx context.Context, workId int64) ([]*dto.RankedSiteAuthor, error)
}

// LocalTagQuerySource 本地标签只读查询面（localTag.LocalTagRepository 实现）
type LocalTagQuerySource interface {
	GetById(ctx context.Context, id int64) (*entity.LocalTag, error)
	PageByNameKeyword(ctx context.Context, nameKeyword string, page, pageSize int) (*model.Page[entity.LocalTag], error)
	ListByIds(ctx context.Context, ids []int64) ([]*entity.LocalTag, error)
}

// SiteTagQuerySource 站点标签只读查询面（siteTag.SiteTagRepository 实现）
type SiteTagQuerySource interface {
	GetBySiteAndSiteTagID(ctx context.Context, siteId int64, siteTagId string) (*entity.SiteTag, error)
	PageByFilter(ctx context.Context, siteId int64, nameKeyword string, page, pageSize int) (*model.Page[entity.SiteTag], error)
	ListBySiteTagIds(ctx context.Context, siteTagIds []int64) ([]*entity.SiteTag, error)
}

// WorkTagRelationSource 作品-标签关联只读查询面（reWorkTag.ReWorkTagRepository 实现）；
// 关联行携带 re_work_tag.namespace（关联级 namespace 维度）
type WorkTagRelationSource interface {
	ListByWorkId(ctx context.Context, workId int64) ([]*entity.ReWorkTag, error)
}

// WorkSetQuerySource workSet 域只读查询面（workSet.WorkSetRepository 实现）
type WorkSetQuerySource interface {
	GetById(ctx context.Context, id int64) (*entity.WorkSet, error)
	GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*entity.WorkSet, error)
	ListByIds(ctx context.Context, ids []int64) ([]*entity.WorkSet, error)
}

// WorkWorkSetRelationSource 作品-作品集关联只读查询面（reWorkWorkSet.ReWorkWorkSetRepository 实现）
type WorkWorkSetRelationSource interface {
	ListByWorkId(ctx context.Context, workId int64) ([]int64, error)
}

// WorkSetHierarchySource 作品集父子关系只读查询面（reWorkSetWorkSet.ReWorkSetWorkSetRepository 实现）
type WorkSetHierarchySource interface {
	ListParentWorkSetIds(ctx context.Context, childWorkSetId int64) ([]int64, error)
	ListChildWorkSetIds(ctx context.Context, parentWorkSetId int64) ([]int64, error)
}

// WorkDirSource 工作目录读取面（settings.Service 实现）
type WorkDirSource interface {
	GetWorkDir() string
}

// LibraryQueryDeps 库查询核心的域只读依赖（装配处构造一次、全体插件共享）
type LibraryQueryDeps struct {
	Works          WorkQuerySource
	Sites          SiteQuerySource
	Resources      ResourceQuerySource
	ResourceStores ResourceStoreQuerySource
	Stores         PersistentStoreQuerySource
	LocalAuthors   LocalAuthorQuerySource
	SiteAuthors    SiteAuthorQuerySource
	LocalTags      LocalTagQuerySource
	SiteTags       SiteTagQuerySource
	WorkTagRels    WorkTagRelationSource
	WorkSets       WorkSetQuerySource
	WorkWorkSets   WorkWorkSetRelationSource
	WorkSetGraph   WorkSetHierarchySource
	WorkDir        WorkDirSource
}

// libraryQueryProvider 库查询核心：逐端点桥接各域 repository 并映射为契约消息。
// 无插件态（诊断日志由 pluginContext 调用点按调用方插件记录）
type libraryQueryProvider struct {
	deps LibraryQueryDeps
}

// NewLibraryQueryProvider 创建库查询核心（装配处构造一次，经 PluginContextDeps 注入各插件上下文）
func NewLibraryQueryProvider(deps LibraryQueryDeps) *libraryQueryProvider {
	return &libraryQueryProvider{deps: deps}
}

// --- 错误映射 ---

// errNotFound Get* 族未命中的语义码错误
func errNotFound(what string) error {
	return status.Error(codes.NotFound, what+"未找到")
}

// mapRecordNotFound 仓储未命中错误转 NotFound 语义码（其余错误原样透传）
func mapRecordNotFound(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errNotFound(what)
	}
	return err
}

// --- 实体 → 契约消息映射 ---

func workEntityToGen(w *entity.Work) *gen.Work {
	return &gen.Work{
		Id:                  w.GetID(),
		CreateTime:          w.GetCreateTime(),
		UpdateTime:          w.GetUpdateTime(),
		SiteId:              util.NullInt64ToPointer(w.SiteID),
		SiteWorkId:          util.NullStringToPointer(w.SiteWorkID),
		SiteWorkName:        util.NullStringToPointer(w.SiteWorkName),
		SiteAuthorId:        util.NullStringToPointer(w.SiteAuthorID),
		SiteWorkDescription: util.NullStringToPointer(w.SiteWorkDescription),
		SiteUploadTime:      util.NullInt64ToPointer(w.SiteUploadTime),
		SiteUpdateTime:      util.NullInt64ToPointer(w.SiteUpdateTime),
		NickName:            util.NullStringToPointer(w.NickName),
		LocalAuthorId:       util.NullInt64ToPointer(w.LocalAuthorID),
		LastView:            util.NullInt64ToPointer(w.LastView),
	}
}

func workSetEntityToGen(ws *entity.WorkSet) *gen.WorkSet {
	return &gen.WorkSet{
		Id:                     ws.GetID(),
		CreateTime:             ws.GetCreateTime(),
		UpdateTime:             ws.GetUpdateTime(),
		SiteId:                 util.NullInt64ToPointer(ws.SiteID),
		SiteWorkSetId:          util.NullStringToPointer(ws.SiteWorkSetID),
		SiteWorkSetName:        util.NullStringToPointer(ws.SiteWorkSetName),
		SiteAuthorId:           util.NullStringToPointer(ws.SiteAuthorID),
		SiteWorkSetDescription: util.NullStringToPointer(ws.SiteWorkSetDescription),
		SiteUploadTime:         util.NullInt64ToPointer(ws.SiteUploadTime),
		SiteUpdateTime:         util.NullInt64ToPointer(ws.SiteUpdateTime),
		NickName:               util.NullStringToPointer(ws.NickName),
		LastView:               util.NullInt64ToPointer(ws.LastView),
		Description:            util.NullStringToPointer(ws.Description),
	}
}

// siteAuthorEntityToInfo 站点作者行 → SiteAuthorInfo（siteKey 经站点表补全）
func siteAuthorEntityToInfo(a *entity.SiteAuthor, siteKey string) *gen.SiteAuthorInfo {
	return &gen.SiteAuthorInfo{
		Id:              a.GetID(),
		SiteKey:         siteKey,
		SiteAuthorId:    nullStringOr(a.SiteAuthorID),
		AuthorName:      nullStringOr(a.AuthorName),
		Homepage:        nullStringOr(a.Homepage),
		FixedAuthorName: nullStringOr(a.FixedAuthorName),
		Introduce:       nullStringOr(a.Introduce),
		LocalAuthorId:   util.NullInt64ToPointer(a.LocalAuthorID),
	}
}

// siteTagEntityToInfo 站点标签行 → SiteTagInfo（siteKey 经站点表补全）
func siteTagEntityToInfo(t *entity.SiteTag, siteKey string) *gen.SiteTagInfo {
	return &gen.SiteTagInfo{
		Id:          t.GetID(),
		SiteKey:     siteKey,
		SiteTagId:   nullStringOr(t.SiteTagID),
		TagName:     nullStringOr(t.SiteTagName),
		Description: nullStringOr(t.Description),
		Namespace:   nullStringOr(t.Namespace),
		LocalTagId:  util.NullInt64ToPointer(t.LocalTagID),
	}
}

// siteKeyOf 从站点索引取身份键（行缺失返回空串）
func siteKeyOf(siteMap map[int64]*entity.Site, siteID int64) string {
	if s, ok := siteMap[siteID]; ok {
		return s.SiteKey
	}
	return ""
}

// siteMapAll 全量站点按 id 建索引（站点注册表投影个位数级，单次查询供页内组装复用）
func (p *libraryQueryProvider) siteMapAll(ctx context.Context) (map[int64]*entity.Site, error) {
	sites, err := p.deps.Sites.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[int64]*entity.Site, len(sites))
	for _, s := range sites {
		m[s.GetID()] = s
	}
	return m, nil
}

// emptyWorksPage 站点键过滤未命中注册表时的空结果（过滤条件非寻址，按空集返回而非报错）
func emptyWorksPage(req *gen.QueryWorksRequest) *gen.QueryWorksResponse {
	page, pageSize := pageRequestOf(req.Page)
	return &gen.QueryWorksResponse{Items: []*gen.WorkWithSite{}, Page: pageInfoOf(0, page, pageSize)}
}

// ===== 作品 =====

func (p *libraryQueryProvider) getWorkById(ctx context.Context, workId int64) (*gen.WorkWithSite, error) {
	w, err := p.deps.Works.GetById(ctx, workId)
	if err != nil {
		return nil, mapRecordNotFound(err, "作品")
	}
	siteMap, err := p.siteMapAll(ctx)
	if err != nil {
		return nil, err
	}
	return workWithSiteOf(w, siteMap), nil
}

func (p *libraryQueryProvider) getWorkBySiteKey(ctx context.Context, siteKey, siteWorkId string) (*gen.WorkWithSite, error) {
	if siteKey == "" || siteWorkId == "" {
		return nil, errNotFound("作品")
	}
	site, err := p.deps.Sites.GetByKey(ctx, siteKey)
	if err != nil {
		return nil, mapRecordNotFound(err, "站点键 "+siteKey)
	}
	w, err := p.deps.Works.GetBySiteAndSiteWorkID(ctx, site.GetID(), siteWorkId)
	if err != nil {
		return nil, mapRecordNotFound(err, "作品")
	}
	return &gen.WorkWithSite{Work: workEntityToGen(w), Site: dto.NewSiteDTO(site)}, nil
}

// workWithSiteOf 作品行 + 站点索引 → WorkWithSite（站点行缺失时 site 缺省）
func workWithSiteOf(w *entity.Work, siteMap map[int64]*entity.Site) *gen.WorkWithSite {
	item := &gen.WorkWithSite{Work: workEntityToGen(w)}
	if w.SiteID.Valid {
		if s, ok := siteMap[w.SiteID.Int64]; ok {
			item.Site = dto.NewSiteDTO(s)
		}
	}
	return item
}

func (p *libraryQueryProvider) queryWorks(ctx context.Context, req *gen.QueryWorksRequest) (*gen.QueryWorksResponse, error) {
	filter := &work.WorkQueryFilter{
		NameKeyword:     strings.TrimSpace(req.NameKeyword),
		AuthorKeyword:   strings.TrimSpace(req.AuthorKeyword),
		TagKeyword:      strings.TrimSpace(req.TagKeyword),
		CreateTimeStart: req.CreateTimeStart,
		CreateTimeEnd:   req.CreateTimeEnd,
	}
	if req.SiteKey != "" {
		site, err := p.deps.Sites.GetByKey(ctx, req.SiteKey)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return emptyWorksPage(req), nil
			}
			return nil, err
		}
		filter.SiteID = site.GetID()
	}
	page, pageSize := pageRequestOf(req.Page)
	result, err := p.deps.Works.PageByFilter(ctx, filter, page, pageSize)
	if err != nil {
		return nil, err
	}
	siteMap, err := p.siteMapAll(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*gen.WorkWithSite, 0, len(result.Data))
	for _, w := range result.Data {
		items = append(items, workWithSiteOf(w, siteMap))
	}
	return &gen.QueryWorksResponse{Items: items, Page: pageInfoOf(result.DataCount, result.PageNumber, result.PageSize)}, nil
}

// ===== 资源与 store =====

func (p *libraryQueryProvider) listResourcesByWorkId(ctx context.Context, workId int64) (*gen.ListResourcesByWorkIdResponse, error) {
	if _, err := p.deps.Works.GetById(ctx, workId); err != nil {
		return nil, mapRecordNotFound(err, "作品")
	}
	resources, err := p.deps.Resources.ListByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	resourceIds := make([]int64, 0, len(resources))
	for _, r := range resources {
		resourceIds = append(resourceIds, r.GetID())
	}
	relations, err := p.deps.ResourceStores.ListAliveByResourceIds(ctx, resourceIds)
	if err != nil {
		return nil, err
	}
	storeIds := make([]int64, 0, len(relations))
	for _, rel := range relations {
		storeIds = append(storeIds, rel.StoreID)
	}
	stores, err := p.deps.Stores.ListByIds(ctx, storeIds)
	if err != nil {
		return nil, err
	}
	storeMap := make(map[int64]*entity.PersistentStore, len(stores))
	for _, s := range stores {
		storeMap[s.GetID()] = s
	}
	// 资源 → 活行 store 摘要（按 resource_id 分桶）
	storesByResource := make(map[int64][]*gen.StoreInfo, len(resources))
	for _, rel := range relations {
		store, ok := storeMap[rel.StoreID]
		if !ok {
			continue
		}
		storesByResource[rel.ResourceID] = append(storesByResource[rel.ResourceID], storeEntityToInfo(rel, store))
	}
	items := make([]*gen.ResourceInfo, 0, len(resources))
	for _, r := range resources {
		items = append(items, &gen.ResourceInfo{
			Id:               r.GetID(),
			TaskId:           nullInt64Or(r.TaskID),
			SuggestName:      util.NullStringToPointer(r.SuggestName),
			ResourceType:     r.ResourceType,
			ResourceComplete: int32(nullInt64Or(r.ResourceComplete)),
			Stores:           storesByResource[r.GetID()],
			CreateTime:       r.GetCreateTime(),
			UpdateTime:       r.GetUpdateTime(),
		})
	}
	return &gen.ListResourcesByWorkIdResponse{Items: items}, nil
}

// storeEntityToInfo store 行 + 关联行 → StoreInfo。
// file_path 为库内存储原值（relPath 域正斜杠，不转换不重拼）；size 无库列来源，缺省不置
func storeEntityToInfo(rel *entity.ResourceStore, store *entity.PersistentStore) *gen.StoreInfo {
	info := &gen.StoreInfo{
		Role:        rel.StoreType,
		StoreSeq:    int32(rel.StoreSeq),
		FilePath:    util.NullStringToPointer(store.FilePath),
		Format:      util.NullStringToPointer(store.FilenameExtension),
		CompletedAt: store.CompletedAt,
	}
	if store.Width.Valid && store.Width.Int64 > 0 {
		v := int32(store.Width.Int64)
		info.Width = &v
	}
	if store.Height.Valid && store.Height.Int64 > 0 {
		v := int32(store.Height.Int64)
		info.Height = &v
	}
	return info
}

// ===== 作者（本地轨 / 站点轨 / 作品关联）=====

func (p *libraryQueryProvider) getLocalAuthorById(ctx context.Context, localAuthorId int64) (*gen.LocalAuthorDTO, error) {
	a, err := p.deps.LocalAuthors.GetById(ctx, localAuthorId)
	if err != nil {
		return nil, mapRecordNotFound(err, "本地作者")
	}
	return dto.NewLocalAuthorDTO(a), nil
}

func (p *libraryQueryProvider) queryLocalAuthors(ctx context.Context, req *gen.QueryLocalAuthorsRequest) (*gen.QueryLocalAuthorsResponse, error) {
	page, pageSize := pageRequestOf(req.Page)
	result, err := p.deps.LocalAuthors.PageByNameKeyword(ctx, strings.TrimSpace(req.NameKeyword), page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]*gen.LocalAuthorDTO, 0, len(result.Data))
	for _, a := range result.Data {
		items = append(items, dto.NewLocalAuthorDTO(a))
	}
	return &gen.QueryLocalAuthorsResponse{Items: items, Page: pageInfoOf(result.DataCount, result.PageNumber, result.PageSize)}, nil
}

func (p *libraryQueryProvider) getSiteAuthorBySiteKey(ctx context.Context, siteKey, siteAuthorId string) (*gen.SiteAuthorInfo, error) {
	if siteKey == "" || siteAuthorId == "" {
		return nil, errNotFound("站点作者")
	}
	site, err := p.deps.Sites.GetByKey(ctx, siteKey)
	if err != nil {
		return nil, mapRecordNotFound(err, "站点键 "+siteKey)
	}
	a, err := p.deps.SiteAuthors.GetBySiteAndSiteAuthorID(ctx, site.GetID(), siteAuthorId)
	if err != nil {
		return nil, mapRecordNotFound(err, "站点作者")
	}
	return siteAuthorEntityToInfo(a, site.SiteKey), nil
}

func (p *libraryQueryProvider) querySiteAuthors(ctx context.Context, req *gen.QuerySiteAuthorsRequest) (*gen.QuerySiteAuthorsResponse, error) {
	siteId := int64(0)
	if req.SiteKey != "" {
		site, err := p.deps.Sites.GetByKey(ctx, req.SiteKey)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				page, pageSize := pageRequestOf(req.Page)
				return &gen.QuerySiteAuthorsResponse{Items: []*gen.SiteAuthorInfo{}, Page: pageInfoOf(0, page, pageSize)}, nil
			}
			return nil, err
		}
		siteId = site.GetID()
	}
	page, pageSize := pageRequestOf(req.Page)
	result, err := p.deps.SiteAuthors.PageByFilter(ctx, siteId, strings.TrimSpace(req.NameKeyword), page, pageSize)
	if err != nil {
		return nil, err
	}
	siteMap, err := p.siteMapAll(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*gen.SiteAuthorInfo, 0, len(result.Data))
	for _, a := range result.Data {
		items = append(items, siteAuthorEntityToInfo(a, siteKeyOf(siteMap, nullInt64Or(a.SiteID))))
	}
	return &gen.QuerySiteAuthorsResponse{Items: items, Page: pageInfoOf(result.DataCount, result.PageNumber, result.PageSize)}, nil
}

func (p *libraryQueryProvider) listAuthorsByWorkId(ctx context.Context, workId int64) (*gen.ListAuthorsByWorkIdResponse, error) {
	if _, err := p.deps.Works.GetById(ctx, workId); err != nil {
		return nil, mapRecordNotFound(err, "作品")
	}
	localRanked, err := p.deps.LocalAuthors.ListByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	siteRanked, err := p.deps.SiteAuthors.ListByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	siteMap, err := p.siteMapAll(ctx)
	if err != nil {
		return nil, err
	}
	localItems := make([]*gen.LocalAuthorDTO, 0, len(localRanked))
	for _, r := range localRanked {
		// 取字段地址而非按值拷贝（proto 消息含 MessageState 锁，按值拷贝非法）；
		// RankedLocalAuthor.Author 即 SDK 契约类型本体（gen.LocalAuthorDTO 别名）
		localItems = append(localItems, &r.Author)
	}
	siteItems := make([]*gen.SiteAuthorInfo, 0, len(siteRanked))
	for _, r := range siteRanked {
		siteItems = append(siteItems, siteAuthorDTOToInfo(r.Author, siteKeyOf(siteMap, derefOrZero(r.Author.SiteID))))
	}
	return &gen.ListAuthorsByWorkIdResponse{LocalAuthors: localItems, SiteAuthors: siteItems}, nil
}

// siteAuthorDTOToInfo 关联查询结果（域内 SiteAuthorDTO）→ SiteAuthorInfo（siteKey 经站点表补全）
func siteAuthorDTOToInfo(a dto.SiteAuthorDTO, siteKey string) *gen.SiteAuthorInfo {
	info := &gen.SiteAuthorInfo{
		Id:            a.ID,
		SiteKey:       siteKey,
		LocalAuthorId: a.LocalAuthorID,
	}
	if a.SiteAuthorID != nil {
		info.SiteAuthorId = *a.SiteAuthorID
	}
	if a.AuthorName != nil {
		info.AuthorName = *a.AuthorName
	}
	if a.Homepage != nil {
		info.Homepage = *a.Homepage
	}
	if a.FixedAuthorName != nil {
		info.FixedAuthorName = *a.FixedAuthorName
	}
	if a.Introduce != nil {
		info.Introduce = *a.Introduce
	}
	return info
}

// derefOrZero 指针解引用（nil = 0）
func derefOrZero(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// ===== 标签（与作者对称；作品关联含关联级 namespace 维度）=====

func (p *libraryQueryProvider) getLocalTagById(ctx context.Context, localTagId int64) (*gen.LocalTagDTO, error) {
	t, err := p.deps.LocalTags.GetById(ctx, localTagId)
	if err != nil {
		return nil, mapRecordNotFound(err, "本地标签")
	}
	return dto.NewLocalTagDTO(t), nil
}

func (p *libraryQueryProvider) queryLocalTags(ctx context.Context, req *gen.QueryLocalTagsRequest) (*gen.QueryLocalTagsResponse, error) {
	page, pageSize := pageRequestOf(req.Page)
	result, err := p.deps.LocalTags.PageByNameKeyword(ctx, strings.TrimSpace(req.NameKeyword), page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]*gen.LocalTagDTO, 0, len(result.Data))
	for _, t := range result.Data {
		items = append(items, dto.NewLocalTagDTO(t))
	}
	return &gen.QueryLocalTagsResponse{Items: items, Page: pageInfoOf(result.DataCount, result.PageNumber, result.PageSize)}, nil
}

func (p *libraryQueryProvider) getSiteTagBySiteKey(ctx context.Context, siteKey, siteTagId string) (*gen.SiteTagInfo, error) {
	if siteKey == "" || siteTagId == "" {
		return nil, errNotFound("站点标签")
	}
	site, err := p.deps.Sites.GetByKey(ctx, siteKey)
	if err != nil {
		return nil, mapRecordNotFound(err, "站点键 "+siteKey)
	}
	t, err := p.deps.SiteTags.GetBySiteAndSiteTagID(ctx, site.GetID(), siteTagId)
	if err != nil {
		return nil, mapRecordNotFound(err, "站点标签")
	}
	return siteTagEntityToInfo(t, site.SiteKey), nil
}

func (p *libraryQueryProvider) querySiteTags(ctx context.Context, req *gen.QuerySiteTagsRequest) (*gen.QuerySiteTagsResponse, error) {
	siteId := int64(0)
	if req.SiteKey != "" {
		site, err := p.deps.Sites.GetByKey(ctx, req.SiteKey)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				page, pageSize := pageRequestOf(req.Page)
				return &gen.QuerySiteTagsResponse{Items: []*gen.SiteTagInfo{}, Page: pageInfoOf(0, page, pageSize)}, nil
			}
			return nil, err
		}
		siteId = site.GetID()
	}
	page, pageSize := pageRequestOf(req.Page)
	result, err := p.deps.SiteTags.PageByFilter(ctx, siteId, strings.TrimSpace(req.NameKeyword), page, pageSize)
	if err != nil {
		return nil, err
	}
	siteMap, err := p.siteMapAll(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*gen.SiteTagInfo, 0, len(result.Data))
	for _, t := range result.Data {
		items = append(items, siteTagEntityToInfo(t, siteKeyOf(siteMap, nullInt64Or(t.SiteID))))
	}
	return &gen.QuerySiteTagsResponse{Items: items, Page: pageInfoOf(result.DataCount, result.PageNumber, result.PageSize)}, nil
}

func (p *libraryQueryProvider) listTagsByWorkId(ctx context.Context, workId int64) (*gen.ListTagsByWorkIdResponse, error) {
	if _, err := p.deps.Works.GetById(ctx, workId); err != nil {
		return nil, mapRecordNotFound(err, "作品")
	}
	rels, err := p.deps.WorkTagRels.ListByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	localTagIds := make([]int64, 0, len(rels))
	siteTagIds := make([]int64, 0, len(rels))
	for _, rel := range rels {
		if rel.LocalTagID.Valid && rel.LocalTagID.Int64 > 0 {
			localTagIds = append(localTagIds, rel.LocalTagID.Int64)
		}
		if rel.SiteTagID.Valid && rel.SiteTagID.Int64 > 0 {
			siteTagIds = append(siteTagIds, rel.SiteTagID.Int64)
		}
	}
	localTags, err := p.deps.LocalTags.ListByIds(ctx, localTagIds)
	if err != nil {
		return nil, err
	}
	siteTags, err := p.deps.SiteTags.ListBySiteTagIds(ctx, siteTagIds)
	if err != nil {
		return nil, err
	}
	siteMap, err := p.siteMapAll(ctx)
	if err != nil {
		return nil, err
	}
	localTagMap := make(map[int64]*entity.LocalTag, len(localTags))
	for _, t := range localTags {
		localTagMap[t.GetID()] = t
	}
	siteTagMap := make(map[int64]*entity.SiteTag, len(siteTags))
	for _, t := range siteTags {
		siteTagMap[t.GetID()] = t
	}
	localItems := make([]*gen.WorkLocalTagEntry, 0, len(localTagIds))
	siteItems := make([]*gen.WorkSiteTagEntry, 0, len(siteTagIds))
	for _, rel := range rels {
		if rel.LocalTagID.Valid && rel.LocalTagID.Int64 > 0 {
			if t, ok := localTagMap[rel.LocalTagID.Int64]; ok {
				localItems = append(localItems, &gen.WorkLocalTagEntry{
					Tag:       dto.NewLocalTagDTO(t),
					Namespace: nullStringOr(rel.Namespace),
				})
			}
		}
		if rel.SiteTagID.Valid && rel.SiteTagID.Int64 > 0 {
			if t, ok := siteTagMap[rel.SiteTagID.Int64]; ok {
				siteItems = append(siteItems, &gen.WorkSiteTagEntry{
					Tag:       siteTagEntityToInfo(t, siteKeyOf(siteMap, nullInt64Or(t.SiteID))),
					Namespace: nullStringOr(rel.Namespace),
				})
			}
		}
	}
	return &gen.ListTagsByWorkIdResponse{LocalTags: localItems, SiteTags: siteItems}, nil
}

// ===== 作品集 =====

func (p *libraryQueryProvider) getWorkSetById(ctx context.Context, workSetId int64) (*gen.WorkSet, error) {
	ws, err := p.deps.WorkSets.GetById(ctx, workSetId)
	if err != nil {
		return nil, mapRecordNotFound(err, "作品集")
	}
	return workSetEntityToGen(ws), nil
}

func (p *libraryQueryProvider) getWorkSetBySiteKey(ctx context.Context, siteKey, siteWorkSetId string) (*gen.WorkSet, error) {
	if siteKey == "" || siteWorkSetId == "" {
		return nil, errNotFound("作品集")
	}
	site, err := p.deps.Sites.GetByKey(ctx, siteKey)
	if err != nil {
		return nil, mapRecordNotFound(err, "站点键 "+siteKey)
	}
	ws, err := p.deps.WorkSets.GetBySiteAndSiteWorkSetID(ctx, site.GetID(), siteWorkSetId)
	if err != nil {
		return nil, mapRecordNotFound(err, "作品集")
	}
	return workSetEntityToGen(ws), nil
}

func (p *libraryQueryProvider) listWorkSetsByWorkId(ctx context.Context, workId int64) (*gen.ListWorkSetsByWorkIdResponse, error) {
	if _, err := p.deps.Works.GetById(ctx, workId); err != nil {
		return nil, mapRecordNotFound(err, "作品")
	}
	ids, err := p.deps.WorkWorkSets.ListByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	sets, err := p.deps.WorkSets.ListByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]*gen.WorkSet, 0, len(sets))
	for _, ws := range sets {
		items = append(items, workSetEntityToGen(ws))
	}
	return &gen.ListWorkSetsByWorkIdResponse{Items: items}, nil
}

// listWorkSetNeighbors 锚定作品集存在性后按关系方向取相邻作品集（软删集行经 scope 排除不出现）
func (p *libraryQueryProvider) listWorkSetNeighbors(ctx context.Context, workSetId int64, parent bool) ([]*gen.WorkSet, error) {
	if _, err := p.deps.WorkSets.GetById(ctx, workSetId); err != nil {
		return nil, mapRecordNotFound(err, "作品集")
	}
	var ids []int64
	var err error
	if parent {
		ids, err = p.deps.WorkSetGraph.ListParentWorkSetIds(ctx, workSetId)
	} else {
		ids, err = p.deps.WorkSetGraph.ListChildWorkSetIds(ctx, workSetId)
	}
	if err != nil {
		return nil, err
	}
	sets, err := p.deps.WorkSets.ListByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]*gen.WorkSet, 0, len(sets))
	for _, ws := range sets {
		items = append(items, workSetEntityToGen(ws))
	}
	return items, nil
}

func (p *libraryQueryProvider) listParentWorkSets(ctx context.Context, workSetId int64) (*gen.ListParentWorkSetsResponse, error) {
	items, err := p.listWorkSetNeighbors(ctx, workSetId, true)
	if err != nil {
		return nil, err
	}
	return &gen.ListParentWorkSetsResponse{Items: items}, nil
}

func (p *libraryQueryProvider) listChildWorkSets(ctx context.Context, workSetId int64) (*gen.ListChildWorkSetsResponse, error) {
	items, err := p.listWorkSetNeighbors(ctx, workSetId, false)
	if err != nil {
		return nil, err
	}
	return &gen.ListChildWorkSetsResponse{Items: items}, nil
}

// ===== 站点（注册表投影，只读）=====

func (p *libraryQueryProvider) listSites(ctx context.Context) (*gen.ListSitesResponse, error) {
	sites, err := p.deps.Sites.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*gen.SiteDTO, 0, len(sites))
	for _, s := range sites {
		items = append(items, dto.NewSiteDTO(s))
	}
	return &gen.ListSitesResponse{Items: items}, nil
}

// ===== 工作目录 =====

// getWorkDir 返回资源库根目录绝对路径；未配置时对齐 RefuseIfUnconfigured 哨兵语义
// （统一通知前端 + 显式拒绝），以 FailedPrecondition 语义码上抛、不返回空路径
func (p *libraryQueryProvider) getWorkDir(caller string) (*gen.GetWorkDirResponse, error) {
	workDir := p.deps.WorkDir.GetWorkDir()
	if err := settings.RefuseIfUnconfigured(workDir, "plugin:"+caller); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	return &gen.GetWorkDirResponse{Path: workDir}, nil
}

// --- sql.Null* 取值便捷映射 ---

func nullStringOr(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

func nullInt64Or(i sql.NullInt64) int64 {
	if i.Valid {
		return i.Int64
	}
	return 0
}
