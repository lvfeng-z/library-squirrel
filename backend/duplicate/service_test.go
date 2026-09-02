package duplicate

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"

	"go.uber.org/zap"
)

// TestMain 全局初始化 no-op logger,使 Check 内行级查询失败的日志调用安全(logger.Log 默认 nil)
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	m.Run()
}

// ==== fakes ====

// fakeRepo 查重仓储桩:站点键→站点、站点作品定位均可预置,并支持查询失败模拟
type fakeRepo struct {
	sitesByKey  map[string]*entity.Site           // 站点键 → 站点行
	worksBySite map[int64]map[string]*entity.Work // 本库站点 ID → 站点作品 ID → 作品行
	siteErr     error
	workErr     error
}

func (f *fakeRepo) ListSitesByKeys(ctx context.Context, keys []string) ([]*entity.Site, error) {
	if f.siteErr != nil {
		return nil, f.siteErr
	}
	var out []*entity.Site
	for _, k := range keys {
		if s, ok := f.sitesByKey[k]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeRepo) ListWorksBySiteAndWorkIDs(ctx context.Context, siteId int64, siteWorkIds []string) ([]*entity.Work, error) {
	if f.workErr != nil {
		return nil, f.workErr
	}
	var out []*entity.Work
	for _, wid := range siteWorkIds {
		if w, ok := f.worksBySite[siteId][wid]; ok {
			out = append(out, w)
		}
	}
	return out, nil
}

// fakeRoleSets 活行角色集合桩:预置 {workId: 角色集},支持查询失败模拟
type fakeRoleSets struct {
	sets map[int64]map[string]struct{}
	err  error
}

func (f *fakeRoleSets) ListStoreTypeSetsByWorkIds(ctx context.Context, workIds []int64) (map[int64]map[string]struct{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sets, nil
}

// ==== 构造辅助 ====

func mkSite(id int64, key string) *entity.Site {
	s := entity.NewSite()
	s.ID = id
	s.SiteKey = key
	return s
}

func mkWork(id int64, siteId int64, siteWorkId string) *entity.Work {
	w := entity.NewWork()
	w.ID = id
	w.SiteID = sql.NullInt64{Int64: siteId, Valid: true}
	w.SiteWorkID = sql.NullString{String: siteWorkId, Valid: true}
	w.SiteWorkName = sql.NullString{String: "已有作品", Valid: true}
	return w
}

func roleSet(roles ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		set[r] = struct{}{}
	}
	return set
}

// stdEnv 标准环境:站点 key-a→id=10 挂作品 "w1"→id=500(角色 image+thumbnail)
func stdEnv() (*fakeRepo, *fakeRoleSets) {
	repo := &fakeRepo{
		sitesByKey: map[string]*entity.Site{"key-a": mkSite(10, "key-a")},
		worksBySite: map[int64]map[string]*entity.Work{
			10: {"w1": mkWork(500, 10, "w1")},
		},
	}
	roleSets := &fakeRoleSets{sets: map[int64]map[string]struct{}{
		500: roleSet(entity.StoreTypeImage, entity.StoreTypeThumbnail),
	}}
	return repo, roleSets
}

func checkOne(t *testing.T, svc *Service, item DuplicateCheckItem) DuplicateCheckResult {
	t.Helper()
	results, err := svc.Check(context.Background(), []DuplicateCheckItem{item})
	if err != nil {
		t.Fatalf("Check 失败: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("结果数期望 1, 实际 %d", len(results))
	}
	return results[0]
}

// ==== 三分类 ====

func TestCheck_Miss(t *testing.T) {
	t.Run("站点不存在_未命中", func(t *testing.T) {
		repo, roleSets := stdEnv()
		svc := NewService(repo, roleSets)
		res := checkOne(t, svc, DuplicateCheckItem{SiteKey: "unknown-key", SiteWorkID: "w1", Roles: []string{entity.StoreTypeImage}})
		if res.Class != DuplicateMiss || res.WorkID != 0 {
			t.Fatalf("期望未命中(WorkID=0), 实际 class=%d WorkID=%d", res.Class, res.WorkID)
		}
	})
	t.Run("站点存在_作品未收录_未命中", func(t *testing.T) {
		repo, roleSets := stdEnv()
		svc := NewService(repo, roleSets)
		res := checkOne(t, svc, DuplicateCheckItem{SiteKey: "key-a", SiteWorkID: "not-exist", Roles: []string{entity.StoreTypeImage}})
		if res.Class != DuplicateMiss {
			t.Fatalf("期望未命中, 实际 class=%d", res.Class)
		}
	})
	t.Run("空站点键_未命中", func(t *testing.T) {
		repo, roleSets := stdEnv()
		svc := NewService(repo, roleSets)
		res := checkOne(t, svc, DuplicateCheckItem{SiteKey: "", SiteWorkID: "w1", Roles: nil})
		if res.Class != DuplicateMiss {
			t.Fatalf("空站点键期望未命中, 实际 class=%d", res.Class)
		}
	})
}

func TestCheck_HitNoConflict(t *testing.T) {
	t.Run("显式板块_交集空_命中无冲突_保留作品ID", func(t *testing.T) {
		repo, roleSets := stdEnv()
		svc := NewService(repo, roleSets)
		res := checkOne(t, svc, DuplicateCheckItem{SiteKey: "key-a", SiteWorkID: "w1", Roles: []string{entity.StoreTypeVideoTrack, entity.StoreTypeAudioTrack}})
		if res.Class != DuplicateHitNoConflict || res.WorkID != 500 || res.WorkName != "已有作品" {
			t.Fatalf("期望命中无冲突(WorkID=500), 实际 class=%d WorkID=%d WorkName=%q", res.Class, res.WorkID, res.WorkName)
		}
	})
	t.Run("已有零行_命中无冲突", func(t *testing.T) {
		repo, roleSets := stdEnv()
		roleSets.sets[500] = map[string]struct{}{}
		svc := NewService(repo, roleSets)
		res := checkOne(t, svc, DuplicateCheckItem{SiteKey: "key-a", SiteWorkID: "w1", Roles: []string{entity.StoreTypeImage}})
		if res.Class != DuplicateHitNoConflict || res.WorkID != 500 {
			t.Fatalf("零行期望命中无冲突, 实际 class=%d WorkID=%d", res.Class, res.WorkID)
		}
	})
	t.Run("板块空_已有零行_命中无冲突", func(t *testing.T) {
		repo, roleSets := stdEnv()
		roleSets.sets[500] = map[string]struct{}{}
		svc := NewService(repo, roleSets)
		res := checkOne(t, svc, DuplicateCheckItem{SiteKey: "key-a", SiteWorkID: "w1", Roles: nil})
		if res.Class != DuplicateHitNoConflict {
			t.Fatalf("板块空+零行期望命中无冲突, 实际 class=%d", res.Class)
		}
	})
}

func TestCheck_HitConflict(t *testing.T) {
	t.Run("显式板块_交集非空_冲突且载荷为交集原序", func(t *testing.T) {
		repo, roleSets := stdEnv()
		svc := NewService(repo, roleSets)
		res := checkOne(t, svc, DuplicateCheckItem{SiteKey: "key-a", SiteWorkID: "w1", Roles: []string{entity.StoreTypeVideoTrack, entity.StoreTypeThumbnail, entity.StoreTypeImage}})
		if res.Class != DuplicateHitConflict {
			t.Fatalf("交集非空期望命中冲突, 实际 class=%d", res.Class)
		}
		if len(res.ConflictRoles) != 2 || res.ConflictRoles[0] != entity.StoreTypeThumbnail || res.ConflictRoles[1] != entity.StoreTypeImage {
			t.Fatalf("冲突载荷期望按期望角色原序 [thumbnail image], 实际 %v", res.ConflictRoles)
		}
	})
	t.Run("板块空_已有任意行_冲突且载荷为已有行全集", func(t *testing.T) {
		repo, roleSets := stdEnv()
		svc := NewService(repo, roleSets)
		res := checkOne(t, svc, DuplicateCheckItem{SiteKey: "key-a", SiteWorkID: "w1", Roles: nil})
		if res.Class != DuplicateHitConflict {
			t.Fatalf("板块空+已有行期望命中冲突, 实际 class=%d", res.Class)
		}
		if len(res.ConflictRoles) != 2 || res.ConflictRoles[0] != entity.StoreTypeImage || res.ConflictRoles[1] != entity.StoreTypeThumbnail {
			t.Fatalf("冲突载荷期望按字母序 [image thumbnail], 实际 %v", res.ConflictRoles)
		}
	})
	t.Run("命中作品ID与站点侧名透出", func(t *testing.T) {
		repo, roleSets := stdEnv()
		svc := NewService(repo, roleSets)
		res := checkOne(t, svc, DuplicateCheckItem{SiteKey: "key-a", SiteWorkID: "w1", Roles: []string{entity.StoreTypeImage}})
		if res.WorkID != 500 || res.WorkName != "已有作品" {
			t.Fatalf("期望透出 WorkID=500 WorkName=已有作品, 实际 %d %q", res.WorkID, res.WorkName)
		}
	})
}

func TestCheck_ConservativeConflict(t *testing.T) {
	t.Run("角色集合查询失败_保守冲突_载荷nil", func(t *testing.T) {
		repo, roleSets := stdEnv()
		roleSets.err = errors.New("db down")
		svc := NewService(repo, roleSets)
		res := checkOne(t, svc, DuplicateCheckItem{SiteKey: "key-a", SiteWorkID: "w1", Roles: []string{entity.StoreTypeImage}})
		if res.Class != DuplicateHitConflict || res.ConflictRoles != nil {
			t.Fatalf("行级查询失败期望保守冲突且载荷 nil, 实际 class=%d roles=%v", res.Class, res.ConflictRoles)
		}
		if res.WorkID != 500 {
			t.Fatalf("保守冲突仍应定位已有作品, 实际 WorkID=%d", res.WorkID)
		}
	})
	t.Run("角色集合提供方未装配_保守冲突_载荷nil", func(t *testing.T) {
		repo, _ := stdEnv()
		svc := NewService(repo, nil)
		res := checkOne(t, svc, DuplicateCheckItem{SiteKey: "key-a", SiteWorkID: "w1", Roles: []string{entity.StoreTypeImage}})
		if res.Class != DuplicateHitConflict || res.ConflictRoles != nil {
			t.Fatalf("提供方缺失期望保守冲突且载荷 nil, 实际 class=%d roles=%v", res.Class, res.ConflictRoles)
		}
	})
}

func TestCheck_QueryFailure(t *testing.T) {
	t.Run("站点查询失败_返回error", func(t *testing.T) {
		repo, roleSets := stdEnv()
		repo.siteErr = errors.New("db down")
		svc := NewService(repo, roleSets)
		if _, err := svc.Check(context.Background(), []DuplicateCheckItem{{SiteKey: "key-a", SiteWorkID: "w1"}}); err == nil {
			t.Fatal("站点查询失败应返回 error")
		}
	})
	t.Run("作品定位失败_返回error", func(t *testing.T) {
		repo, roleSets := stdEnv()
		repo.workErr = errors.New("db down")
		svc := NewService(repo, roleSets)
		if _, err := svc.Check(context.Background(), []DuplicateCheckItem{{SiteKey: "key-a", SiteWorkID: "w1"}}); err == nil {
			t.Fatal("作品定位失败应返回 error")
		}
	})
}

// TestCheck_IndexAligned 多站点多作品:结果与输入按下标一一对应,互不串位
func TestCheck_IndexAligned(t *testing.T) {
	repo := &fakeRepo{
		sitesByKey: map[string]*entity.Site{
			"key-a": mkSite(10, "key-a"),
			"key-b": mkSite(20, "key-b"),
		},
		worksBySite: map[int64]map[string]*entity.Work{
			10: {"w1": mkWork(500, 10, "w1"), "w2": mkWork(501, 10, "w2")},
			20: {"w9": mkWork(900, 20, "w9")},
		},
	}
	roleSets := &fakeRoleSets{sets: map[int64]map[string]struct{}{
		500: roleSet(entity.StoreTypeImage),
		501: roleSet(entity.StoreTypeVideoTrack),
		900: roleSet(entity.StoreTypeAudioTrack),
	}}
	svc := NewService(repo, roleSets)
	items := []DuplicateCheckItem{
		{SiteKey: "key-a", SiteWorkID: "w1", Roles: []string{entity.StoreTypeVideoTrack}}, // 交集空 → 命中无冲突
		{SiteKey: "key-b", SiteWorkID: "w9", Roles: []string{entity.StoreTypeAudioTrack}}, // 交集非空 → 命中冲突
		{SiteKey: "key-a", SiteWorkID: "nope", Roles: []string{entity.StoreTypeImage}},    // 未命中
	}
	results, err := svc.Check(context.Background(), items)
	if err != nil {
		t.Fatalf("Check 失败: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("结果数期望 3, 实际 %d", len(results))
	}
	if results[0].Class != DuplicateHitNoConflict || results[0].WorkID != 500 {
		t.Fatalf("第0项期望命中无冲突(500), 实际 class=%d WorkID=%d", results[0].Class, results[0].WorkID)
	}
	if results[1].Class != DuplicateHitConflict || results[1].WorkID != 900 || len(results[1].ConflictRoles) != 1 || results[1].ConflictRoles[0] != entity.StoreTypeAudioTrack {
		t.Fatalf("第1项期望命中冲突(900, [audioTrack]), 实际 class=%d WorkID=%d roles=%v", results[1].Class, results[1].WorkID, results[1].ConflictRoles)
	}
	if results[2].Class != DuplicateMiss {
		t.Fatalf("第2项期望未命中, 实际 class=%d", results[2].Class)
	}
}

// TestCheckByKeyMergesAcrossLibraries 键命中跨库合并：本库站点与 manifest 站点键一致时（站点
// 展示名可各自演化），同 site_work_id 的作品判定命中——跨库作品合并以键为唯一身份基准
func TestCheckByKeyMergesAcrossLibraries(t *testing.T) {
	repo := &fakeRepo{
		sitesByKey: map[string]*entity.Site{
			// 本库行展示名与注册表权威名不同（历史改名），键即身份
			identity.Pixiv.Key: mkSite(10, identity.Pixiv.Key),
		},
		worksBySite: map[int64]map[string]*entity.Work{
			10: {"1001": mkWork(500, 10, "1001")},
		},
	}
	roleSets := &fakeRoleSets{sets: map[int64]map[string]struct{}{
		500: roleSet(entity.StoreTypeImage),
	}}
	svc := NewService(repo, roleSets)
	res := checkOne(t, svc, DuplicateCheckItem{SiteKey: identity.Pixiv.Key, SiteWorkID: "1001", Roles: []string{entity.StoreTypeImage}})
	if res.Class != DuplicateHitConflict || res.WorkID != 500 {
		t.Fatalf("同键同作品ID期望命中冲突(WorkID=500), 实际 class=%d WorkID=%d", res.Class, res.WorkID)
	}
}

// TestCheckSameNameDifferentKeyNotMerged 同名不同键不合并：站点展示名重复的两行是两个身份，
// 查重按键定位到各自站点——键不匹配的一方永不命中另一方的作品
func TestCheckSameNameDifferentKeyNotMerged(t *testing.T) {
	sameName := "pixiv"
	siteA := mkSite(10, identity.Pixiv.Key)
	siteA.SiteName = sql.NullString{String: sameName, Valid: true}
	siteB := mkSite(20, identity.Bilibili.Key)
	siteB.SiteName = sql.NullString{String: sameName, Valid: true}
	repo := &fakeRepo{
		sitesByKey: map[string]*entity.Site{
			identity.Pixiv.Key:    siteA,
			identity.Bilibili.Key: siteB,
		},
		worksBySite: map[int64]map[string]*entity.Work{
			10: {"1001": mkWork(500, 10, "1001")},
		},
	}
	roleSets := &fakeRoleSets{sets: map[int64]map[string]struct{}{
		500: roleSet(entity.StoreTypeImage),
	}}
	svc := NewService(repo, roleSets)
	// bilibili 键下同 site_work_id 的作品：本库 bilibili 站点未收录该作品 → 未命中，
	// 不得因展示名与 pixiv 行相同而误命中 pixiv 的作品
	res := checkOne(t, svc, DuplicateCheckItem{SiteKey: identity.Bilibili.Key, SiteWorkID: "1001", Roles: []string{entity.StoreTypeImage}})
	if res.Class != DuplicateMiss || res.WorkID != 0 {
		t.Fatalf("同名不同键期望未命中(WorkID=0), 实际 class=%d WorkID=%d", res.Class, res.WorkID)
	}
	// pixiv 键下命中 pixiv 站点的作品
	res = checkOne(t, svc, DuplicateCheckItem{SiteKey: identity.Pixiv.Key, SiteWorkID: "1001", Roles: []string{entity.StoreTypeImage}})
	if res.Class != DuplicateHitConflict || res.WorkID != 500 {
		t.Fatalf("同键期望命中(WorkID=500), 实际 class=%d WorkID=%d", res.Class, res.WorkID)
	}
}

// ==== 交集纯函数 ====

func TestIntersectRoles(t *testing.T) {
	roles := []string{entity.StoreTypeVideoTrack, entity.StoreTypeImage, entity.StoreTypeThumbnail}
	existing := roleSet(entity.StoreTypeImage, entity.StoreTypeThumbnail, "other")
	got := intersectRoles(roles, existing)
	if len(got) != 2 || got[0] != entity.StoreTypeImage || got[1] != entity.StoreTypeThumbnail {
		t.Fatalf("交集期望按 roles 原序 [image thumbnail], 实际 %v", got)
	}
	if got := intersectRoles(roles, roleSet()); len(got) != 0 {
		t.Fatalf("空已有集合期望空交集, 实际 %v", got)
	}
}

func TestSortedStoreRoles(t *testing.T) {
	got := sortedStoreRoles(roleSet(entity.StoreTypeThumbnail, entity.StoreTypeImage, entity.StoreTypeVideoTrack))
	if len(got) != 3 || got[0] != entity.StoreTypeImage || got[1] != entity.StoreTypeThumbnail || got[2] != entity.StoreTypeVideoTrack {
		t.Fatalf("期望字母序 [image thumbnail videoTrack], 实际 %v", got)
	}
}
