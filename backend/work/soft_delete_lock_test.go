package work

import (
	"context"
	"errors"
	"testing"

	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/shareLock"
)

// ==== 软删除作品锁守卫测试 ====
// SoftDeleteWork 会把作品的活行 store 文件移入 backup，作品正被分享拉取持有时在途拉取
// 会读到源文件消失：锁命中拒绝且不动 work 行；强制解锁后放行；未登记作品行为不变。
// 纯桩构造（无内存 SQLite），CGO 关闭环境同样执行

// softDeleteLockRepo Repository 窄桩：仅覆盖 SoftDeleteWork 走到的 GetById/Delete，
// 其余方法经嵌入接口零值调用即 panic（本测试不触）
type softDeleteLockRepo struct {
	Repository
	deletedIds []int64
}

func (r *softDeleteLockRepo) GetById(ctx context.Context, id int64) (*entity2.Work, error) {
	w := entity2.NewWork()
	w.SetID(id)
	return w, nil
}

func (r *softDeleteLockRepo) Delete(ctx context.Context, id int64) error {
	r.deletedIds = append(r.deletedIds, id)
	return nil
}

// softDeleteEmptyResourceDeleter 无资源作品桩：资源收集链返回空集，软删仅走 work 行软删
type softDeleteEmptyResourceDeleter struct{}

func (softDeleteEmptyResourceDeleter) DeleteByWorkId(ctx context.Context, workId int64) error {
	return nil
}

func (softDeleteEmptyResourceDeleter) ListByWorkId(ctx context.Context, workId int64) ([]*entity2.Resource, error) {
	return nil, nil
}

// softDeleteEmptyRsBatchReader resource_store 收集空集桩
type softDeleteEmptyRsBatchReader struct{}

func (softDeleteEmptyRsBatchReader) ListStoresByResourceIds(ctx context.Context, resourceIds []int64) (map[int64][]*entity2.ResourceStore, error) {
	return map[int64][]*entity2.ResourceStore{}, nil
}

// softDeleteDirectTransactor 直接执行事务体的桩（无 DB 依赖）
type softDeleteDirectTransactor struct{}

func (softDeleteDirectTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// newSoftDeleteLockEnv 锁守卫焦点件：真实 shareLock 注册中心 + Repository/资源链窄桩，
// 其余构造参数 nil（无资源作品不触）
func newSoftDeleteLockEnv() (*Service, *softDeleteLockRepo, shareLock.ShareLockRegistry) {
	repo := &softDeleteLockRepo{}
	lock := shareLock.NewShareLockRegistry()
	svc := NewService(
		repo,                             // Repository（窄桩）
		softDeleteDirectTransactor{},     // Transactor
		nil,                              // LocalTagReader
		nil,                              // LocalAuthorReader
		nil,                              // SiteTagReader
		nil,                              // SiteAuthorReader
		nil,                              // SiteReader
		nil,                              // ResourceReader
		nil,                              // ReWorkTagWriter
		nil,                              // ReWorkWorkSetWriter
		softDeleteEmptyResourceDeleter{}, // ResourceDeleter（空集桩）
		nil,                              // SiteAuthorWriter
		nil,                              // SiteTagWriter
		nil,                              // WorkSetWriter
		nil,                              // ReWorkAuthorWriter
		nil,                              // LocalTagBatchReader
		nil,                              // SiteTagBatchReader
		nil,                              // SiteBatchReader
		nil,                              // LocalAuthorBatchReader
		nil,                              // SiteAuthorBatchReader
		nil,                              // ResourceBatchReader
		softDeleteEmptyRsBatchReader{},   // ResourceStoreBatchReader（空集桩）
		nil,                              // StoreBatchReader
		nil,                              // ReWorkTagBatchReader
		nil,                              // LocalTagFindOrCreator
		nil,                              // LocalAuthorFindOrCreator
		nil,                              // StoreDeleter（空资源链不触）
		nil,                              // RunningTaskStopper
		nil,                              // ResourceStoreHardDeleter
		nil,                              // WorkSetRelationWriter
		nil,                              // CoverReferenceClearer
		lock,                             // WorkLockChecker（真实件）
	)
	return svc, repo, lock
}

// TestSoftDeleteWorkRejectsLockedWork 锁命中：软删被拒返回 shareLock.ErrWorkLocked 且不动 work 行；
// 强制解锁后重试放行完成软删
func TestSoftDeleteWorkRejectsLockedWork(t *testing.T) {
	ctx := context.Background()
	svc, repo, lock := newSoftDeleteLockEnv()
	const workId int64 = 101

	lock.Register(ctx, []int64{workId}, "session-a")
	if err := svc.SoftDeleteWork(ctx, workId); !errors.Is(err, shareLock.ErrWorkLocked) {
		t.Fatalf("锁命中应返回 shareLock.ErrWorkLocked，实际 %v", err)
	}
	if len(repo.deletedIds) != 0 {
		t.Fatalf("锁命中不应软删 work 行，实际已软删 %v", repo.deletedIds)
	}

	// 强制解锁（用户知情）后重试：放行并完成 work 行软删
	lock.ForceUnlock(ctx, workId)
	if err := svc.SoftDeleteWork(ctx, workId); err != nil {
		t.Fatalf("强制解锁后软删应放行，实际失败: %v", err)
	}
	if len(repo.deletedIds) != 1 || repo.deletedIds[0] != workId {
		t.Fatalf("解锁后应完成作品 %d 软删，实际 %v", workId, repo.deletedIds)
	}
}

// TestSoftDeleteWorkAllowsUnlockedWork 未登记：软删不受锁守卫影响（行为不变锚定）
func TestSoftDeleteWorkAllowsUnlockedWork(t *testing.T) {
	ctx := context.Background()
	svc, repo, lock := newSoftDeleteLockEnv()
	const workId int64 = 202

	if lock.IsLocked(ctx, workId) {
		t.Fatalf("前置条件失败：作品 %d 不应被锁", workId)
	}
	if err := svc.SoftDeleteWork(ctx, workId); err != nil {
		t.Fatalf("未登记作品软删应放行，实际失败: %v", err)
	}
	if len(repo.deletedIds) != 1 || repo.deletedIds[0] != workId {
		t.Fatalf("未登记作品应完成软删，实际 %v", repo.deletedIds)
	}
}
