package duplicate

import (
	"context"
	"sort"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
)

// DuplicateCheckItem 查重输入：站点名（manifest/URL 域，内部做名称→本库站点映射）+
// 站点侧作品 ID + 该作品期望的板块角色集合（空=全量语义）。
type DuplicateCheckItem struct {
	SiteName   string
	SiteWorkID string
	Roles      []string
}

// DuplicateClass 查重输出三分类。
type DuplicateClass int

const (
	// DuplicateMiss 未命中：本库无此作品，全新建。
	DuplicateMiss DuplicateClass = iota
	// DuplicateHitNoConflict 命中无冲突：已有作品活行角色与期望角色零交集——
	// 不弹窗，保留已有作品 ID 供替换定位。
	DuplicateHitNoConflict
	// DuplicateHitConflict 命中冲突：交集非空，携带交集角色明细供弹窗展示。
	DuplicateHitConflict
)

// DuplicateCheckResult 查重输出，与输入 items 按下标一一对应。
type DuplicateCheckResult struct {
	Class DuplicateClass
	// WorkID 命中时已有作品 ID（未命中为 0）。
	WorkID int64
	// WorkName 命中时已有作品的站点侧名称（弹窗展示）。
	WorkName string
	// ConflictRoles 命中冲突时的交集角色（保持期望角色原序）；行级信息不可得时为 nil，
	// 消费方按「保守弹窗」处理（载荷不带交集角色）。
	ConflictRoles []string
}

// StoreRoleSetProvider 已有作品活行 store 角色集合提供方（resource.Service 实现）：
// 批量查询多个作品的 {store_type 集合}；无 resource 行的作品不出键，零 store 行的作品出空集合。
type StoreRoleSetProvider interface {
	ListStoreTypeSetsByWorkIds(ctx context.Context, workIds []int64) (map[int64]map[string]struct{}, error)
}

// DuplicateChecker 作品查重判定能力：输入查重键 + 期望板块角色，输出三分类判定。
// 纯查询判定，无副作用。站点名映射、作品批量定位、活行角色集合与交集计算全内聚，
// 供插件任务（taskManager）、分享收件与 zip 导入等消费方复用。
type DuplicateChecker interface {
	Check(ctx context.Context, items []DuplicateCheckItem) ([]DuplicateCheckResult, error)
}

// Service 查重判定实现。站点名映射、作品定位、行级角色集合为内部组合：
// 站点映射与作品定位任一失败返回 error（消费方按需降级——批量派发降级逐任务检查、
// 逐任务按未命中处理）；仅行级角色集合查询失败时命中作品落「保守冲突」（宁多弹不漏弹）。
type Service struct {
	repo     Repository
	roleSets StoreRoleSetProvider
}

// NewService 创建查重判定服务。
func NewService(repo Repository, roleSets StoreRoleSetProvider) *Service {
	return &Service{repo: repo, roleSets: roleSets}
}

// Check 实现 DuplicateChecker。站点名无法映射到本库站点（空名/站点行不存在）的作品
// 按未命中处理——站点不存在则本库不可能有作品引用它。
func (s *Service) Check(ctx context.Context, items []DuplicateCheckItem) ([]DuplicateCheckResult, error) {
	results := make([]DuplicateCheckResult, len(items))

	// 站点名 → 本库站点（一次批量查询，名称去重）
	nameToSite, err := s.mapSitesByNames(ctx, items)
	if err != nil {
		return nil, err
	}

	// 作品批量定位：按本库站点分组查（site_id+site_work_id 等长配对语义）
	located, err := s.locateWorks(ctx, items, nameToSite)
	if err != nil {
		return nil, err
	}

	// 命中作品收集活行角色集合（一次批量查询）
	storeTypeSets, setFailed, err := s.collectStoreRoleSets(ctx, items, nameToSite, located)
	if err != nil {
		return nil, err
	}

	// 逐项分类判定
	for i, item := range items {
		site, ok := nameToSite[item.SiteName]
		if !ok {
			continue // 未命中
		}
		w, ok := located[locateKey{site.GetID(), item.SiteWorkID}]
		if !ok {
			continue // 未命中
		}
		res := &results[i]
		res.WorkID = w.GetID()
		if w.SiteWorkName.Valid {
			res.WorkName = w.SiteWorkName.String
		}
		existingTypes := storeTypeSets[w.GetID()]
		if setFailed || s.roleSets == nil {
			// 行级信息不可得：保守弹窗，载荷不带交集角色
			res.Class = DuplicateHitConflict
			continue
		}
		if len(existingTypes) == 0 {
			// 零行：无覆盖对象，不弹窗，保留已有作品 ID 供替换定位
			res.Class = DuplicateHitNoConflict
			continue
		}
		if len(item.Roles) == 0 {
			// 板块空（插件自决全量）：已有任意活行即冲突，载荷取已有行角色全集
			res.Class = DuplicateHitConflict
			res.ConflictRoles = sortedStoreRoles(existingTypes)
			continue
		}
		conflict := intersectRoles(item.Roles, existingTypes)
		if len(conflict) == 0 {
			// 交集为空：无任何 store 行将被覆盖，不弹窗，保留替换定位
			res.Class = DuplicateHitNoConflict
			continue
		}
		res.Class = DuplicateHitConflict
		res.ConflictRoles = conflict
	}
	return results, nil
}

// locateKey 作品定位键：本库站点 ID + 站点侧作品 ID。
type locateKey struct {
	siteID     int64
	siteWorkID string
}

// mapSitesByNames 站点名 → 本库站点映射（名称去重；空名不参与匹配）。
func (s *Service) mapSitesByNames(ctx context.Context, items []DuplicateCheckItem) (map[string]*entity.Site, error) {
	seen := make(map[string]struct{}, len(items))
	names := make([]string, 0, len(items))
	for _, it := range items {
		if it.SiteName == "" {
			continue
		}
		if _, dup := seen[it.SiteName]; dup {
			continue
		}
		seen[it.SiteName] = struct{}{}
		names = append(names, it.SiteName)
	}
	result := make(map[string]*entity.Site)
	if len(names) == 0 {
		return result, nil
	}
	rows, err := s.repo.ListSitesByNames(ctx, names)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.SiteName.Valid {
			result[row.SiteName.String] = row
		}
	}
	return result, nil
}

// locateWorks 作品批量定位：站点名映射后按本库站点分组查询。
func (s *Service) locateWorks(ctx context.Context, items []DuplicateCheckItem, nameToSite map[string]*entity.Site) (map[locateKey]*entity.Work, error) {
	workIDsBySite := make(map[int64][]string)
	for _, it := range items {
		site, ok := nameToSite[it.SiteName]
		if !ok {
			continue
		}
		workIDsBySite[site.GetID()] = append(workIDsBySite[site.GetID()], it.SiteWorkID)
	}
	located := make(map[locateKey]*entity.Work)
	for siteID, workIDs := range workIDsBySite {
		rows, err := s.repo.ListWorksBySiteAndWorkIDs(ctx, siteID, workIDs)
		if err != nil {
			return nil, err
		}
		for _, w := range rows {
			if w.SiteID.Valid && w.SiteWorkID.Valid {
				located[locateKey{w.SiteID.Int64, w.SiteWorkID.String}] = w
			}
		}
	}
	return located, nil
}

// collectStoreRoleSets 命中作品批量收集活行角色集合。
// 行级查询失败不算整体失败（命中作品落保守冲突）；角色集合提供方未装配同样按不可得处理。
func (s *Service) collectStoreRoleSets(ctx context.Context, items []DuplicateCheckItem, nameToSite map[string]*entity.Site, located map[locateKey]*entity.Work) (map[int64]map[string]struct{}, bool, error) {
	if s.roleSets == nil {
		return nil, false, nil
	}
	hitWorkIds := make([]int64, 0, len(items))
	seen := make(map[int64]struct{})
	for _, it := range items {
		site, ok := nameToSite[it.SiteName]
		if !ok {
			continue
		}
		w, ok := located[locateKey{site.GetID(), it.SiteWorkID}]
		if !ok {
			continue
		}
		if _, dup := seen[w.GetID()]; dup {
			continue
		}
		seen[w.GetID()] = struct{}{}
		hitWorkIds = append(hitWorkIds, w.GetID())
	}
	if len(hitWorkIds) == 0 {
		return make(map[int64]map[string]struct{}), false, nil
	}
	sets, err := s.roleSets.ListStoreTypeSetsByWorkIds(ctx, hitWorkIds)
	if err != nil {
		logger.Log.Errorf("[Duplicate] 行级覆盖判定查询失败: %v，命中作品退回保守弹窗", err)
		return nil, true, nil
	}
	return sets, false, nil
}

// intersectRoles 求期望板块角色与已有 store 行角色集合的交集（保持 roles 原序，供弹窗展示）。
func intersectRoles(roles []string, existing map[string]struct{}) []string {
	var result []string
	for _, r := range roles {
		if _, ok := existing[r]; ok {
			result = append(result, r)
		}
	}
	return result
}

// sortedStoreRoles 已有 store 行角色集合转有序切片（字母序，保证弹窗载荷确定性）。
func sortedStoreRoles(set map[string]struct{}) []string {
	result := make([]string, 0, len(set))
	for r := range set {
		result = append(result, r)
	}
	sort.Strings(result)
	return result
}
