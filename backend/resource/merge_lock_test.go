package resource

import (
	"context"
	"errors"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/shareLock"
)

// ==== overwrite 原轨道置换前置作品锁守卫测试 ====
// 守卫命中返回 shareLock.ErrWorkLocked 且不触碰原轨道 store 行；强制解锁后放行

// mergeLockResource 反查桩：资源 700 属指定作品
type mergeLockResource struct{ workId int64 }

func (r mergeLockResource) GetById(ctx context.Context, id int64) (*domain.Resource, error) {
	res := domain.NewResource()
	res.ID = 700
	res.WorkID = r.workId
	return res, nil
}

func (mergeLockResource) Updates(ctx context.Context, resource *domain.Resource) error { return nil }

// mergeLockStoreOps 记账软删原轨道调用（断言守卫命中时不触碰 store 行）
type mergeLockStoreOps struct{ deletedIds []int64 }

func (o *mergeLockStoreOps) GetById(ctx context.Context, id int64) (*domain.PersistentStore, error) {
	return nil, nil
}
func (o *mergeLockStoreOps) GetAbsPath(store *domain.PersistentStore) string { return "" }
func (o *mergeLockStoreOps) StoreFromFile(ctx context.Context, relPath, fileName, srcAbsPath string) (int64, error) {
	return 0, nil
}
func (o *mergeLockStoreOps) HardDelete(ctx context.Context, id int64, backup bool) (int64, error) {
	return 0, nil
}
func (o *mergeLockStoreOps) DeleteWithBackup(ctx context.Context, id int64) (int64, error) {
	o.deletedIds = append(o.deletedIds, id)
	return 1, nil
}
func (o *mergeLockStoreOps) BuildVariantPath(sourceRelPath, suffix string) string { return "" }

// mergeLockSettings 固定 overwrite 策略（置换路径唯一被测分支）
type mergeLockSettings struct{}

func (mergeLockSettings) GetMergeStrategy() string { return settings.MergeStrategyOverwrite }

// newMergeLockService 装配带真实锁注册中心与记账桩的合并服务（合并/落盘等其余依赖不触达）
func newMergeLockService(ops *mergeLockStoreOps, lock shareLock.ShareLockRegistry) *MergeService {
	return NewMergeService(nil, mergeLockResource{workId: 500}, nil, ops, mergeLockSettings{}, nil, nil, nil, lock)
}

// newMergeTrackPair 构造原视频/音频轨 store 行（901/902）
func newMergeTrackPair() (videoPS, audioPS *domain.PersistentStore) {
	videoPS = domain.NewPersistentStore()
	videoPS.SetID(901)
	audioPS = domain.NewPersistentStore()
	audioPS.SetID(902)
	return videoPS, audioPS
}

// TestOverwriteOriginalTracksRejectsLockedWork 作品锁命中：拒绝置换、不触碰原轨道 store 行；
// 强制解锁后同一调用放行，软删两条原轨道
func TestOverwriteOriginalTracksRejectsLockedWork(t *testing.T) {
	ctx := context.Background()
	lock := shareLock.NewShareLockRegistry()
	ops := &mergeLockStoreOps{}
	svc := newMergeLockService(ops, lock)
	videoPS, audioPS := newMergeTrackPair()

	lock.Register(ctx, []int64{500}, "session-a")
	if err := svc.overwriteOriginalTracks(ctx, 700, videoPS, audioPS); !errors.Is(err, shareLock.ErrWorkLocked) {
		t.Fatalf("锁命中应返回 shareLock.ErrWorkLocked，实际 %v", err)
	}
	if len(ops.deletedIds) != 0 {
		t.Fatalf("锁命中不应触碰原轨道行，实际软删 %v", ops.deletedIds)
	}

	// 强制解锁后重试：放行并完成两条原轨道软删
	lock.ForceUnlock(ctx, 500)
	if err := svc.overwriteOriginalTracks(ctx, 700, videoPS, audioPS); err != nil {
		t.Fatalf("解锁后置换应放行，实际失败: %v", err)
	}
	if len(ops.deletedIds) != 2 || ops.deletedIds[0] != 901 || ops.deletedIds[1] != 902 {
		t.Fatalf("解锁后应软删视频轨 901 与音频轨 902，实际 %v", ops.deletedIds)
	}
}

// TestOverwriteOriginalTracksUnlockedWorkProceeds 未登记作品：行为与无守卫一致（直接置换）
func TestOverwriteOriginalTracksUnlockedWorkProceeds(t *testing.T) {
	ops := &mergeLockStoreOps{}
	svc := newMergeLockService(ops, shareLock.NewShareLockRegistry())
	videoPS, audioPS := newMergeTrackPair()

	if err := svc.overwriteOriginalTracks(context.Background(), 700, videoPS, audioPS); err != nil {
		t.Fatalf("未登记作品置换应放行，实际失败: %v", err)
	}
	if len(ops.deletedIds) != 2 {
		t.Fatalf("未登记作品应完成原轨道软删，实际 %v", ops.deletedIds)
	}
}
