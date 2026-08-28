package resource

import (
	"context"
	"errors"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/shareLock"
)

// ==== 替换前置作品锁守卫测试 ====
// 守卫命中返回 shareLock.ErrWorkLocked 且不触碰任何 store 行；强制解锁后放行

// lockTestResourceLister 固定返回一个作品的资源集合
type lockTestResourceLister struct{ workId int64 }

func (l lockTestResourceLister) ListByWorkId(ctx context.Context, workId int64) ([]*domain.Resource, error) {
	res := domain.NewResource()
	res.ID = 700
	res.WorkID = l.workId
	return []*domain.Resource{res}, nil
}

// lockTestAssocLister 固定返回一条挂载关联（image 主图）
type lockTestAssocLister struct{}

func (lockTestAssocLister) ListByResourceIds(ctx context.Context, ids []int64) ([]*domain.ResourceStore, error) {
	rs := domain.NewResourceStore()
	rs.ResourceID = 700
	rs.StoreType = domain.StoreTypeImage
	rs.StoreSeq = 0
	rs.StoreID = 900
	return []*domain.ResourceStore{rs}, nil
}

// lockTestStoreRowReader 固定持有一条已完成活行,按请求的 id 集过滤返回(对齐真仓储空集语义)
type lockTestStoreRowReader struct{}

func (lockTestStoreRowReader) ListByIdsIncludeDeleted(ctx context.Context, ids []int64) []*domain.PersistentStore {
	for _, id := range ids {
		if id == 900 {
			row := domain.NewPersistentStore()
			row.SetID(900)
			row.CompletedAt = 1
			return []*domain.PersistentStore{row}
		}
	}
	return nil
}

func (lockTestStoreRowReader) RestoreByIds(ctx context.Context, ids []int64) error { return nil }

// lockTestDeleter 记账软删原语调用（断言守卫命中时不触碰 store 行）
type lockTestDeleter struct {
	backupDeletedIds  []int64
	discardedStoreIds []int64
}

func (d *lockTestDeleter) DeleteWithBackup(ctx context.Context, id int64) (int64, error) {
	d.backupDeletedIds = append(d.backupDeletedIds, id)
	return 55, nil
}

func (d *lockTestDeleter) SoftDeleteAndDiscardFile(ctx context.Context, id int64) error {
	d.discardedStoreIds = append(d.discardedStoreIds, id)
	return nil
}

// newLockTestService 装配带真实锁注册中心的替换链服务（其余依赖为可控记账桩）
func newLockTestService(deleter *lockTestDeleter, lock shareLock.ShareLockRegistry) *ReplacementService {
	return NewReplacementService(
		lockTestResourceLister{workId: 500},
		lockTestAssocLister{},
		lockTestStoreRowReader{},
		deleter,
		noopBackupRestorer{},
		noopReplaceWorkLiveness{},
		noopReplaceRecompute{},
		replaceWorkDir{workDir: "E:/lib"},
		lock,
	)
}

// noopBackupRestorer 备份还原桩（守卫测试不触达回滚链）
type noopBackupRestorer struct{}

func (noopBackupRestorer) GetById(ctx context.Context, id int64) (*domain.Backup, error) {
	return nil, nil
}
func (noopBackupRestorer) GetBackupPath(backup *domain.Backup) string { return "" }
func (noopBackupRestorer) RestoreFile(ctx context.Context, backupPath string, targetPath string) error {
	return nil
}
func (noopBackupRestorer) DeleteBackup(ctx context.Context, id int64) error { return nil }

// TestSoftDeleteWorkStoreRolesRejectsLockedWork 作品锁命中：拒绝软删、不触碰任何 store 行；
// 强制解锁后同一调用放行，走完整软删分派
func TestSoftDeleteWorkStoreRolesRejectsLockedWork(t *testing.T) {
	ctx := context.Background()
	lock := shareLock.NewShareLockRegistry()
	deleter := &lockTestDeleter{}
	svc := newLockTestService(deleter, lock)

	lock.Register(ctx, []int64{500}, "session-a")
	victims, err := svc.SoftDeleteWorkStoreRoles(ctx, 500, []string{domain.StoreTypeImage})
	if !errors.Is(err, shareLock.ErrWorkLocked) {
		t.Fatalf("锁命中应返回 shareLock.ErrWorkLocked，实际 %v", err)
	}
	if victims != nil {
		t.Fatalf("锁命中不应返回软删清单，实际 %v", victims)
	}
	if len(deleter.backupDeletedIds) != 0 || len(deleter.discardedStoreIds) != 0 {
		t.Fatalf("锁命中不应触碰 store 行，实际备份软删 %v 废弃 %v", deleter.backupDeletedIds, deleter.discardedStoreIds)
	}

	// 强制解锁后重试：放行并完成软删
	lock.ForceUnlock(ctx, 500)
	victims, err = svc.SoftDeleteWorkStoreRoles(ctx, 500, []string{domain.StoreTypeImage})
	if err != nil {
		t.Fatalf("解锁后软删应放行，实际失败: %v", err)
	}
	if len(victims) != 1 || victims[0].StoreID != 900 {
		t.Fatalf("解锁后应软删活行 900，实际 %v", victims)
	}
	if len(deleter.backupDeletedIds) != 1 || deleter.backupDeletedIds[0] != 900 {
		t.Fatalf("已完成活行应走备份软删，实际 %v", deleter.backupDeletedIds)
	}
}

// TestSoftDeleteWorkStoreRolesUnlockedWorkProceeds 未登记作品：行为与无守卫一致（直接放行）
func TestSoftDeleteWorkStoreRolesUnlockedWorkProceeds(t *testing.T) {
	ctx := context.Background()
	deleter := &lockTestDeleter{}
	svc := newLockTestService(deleter, shareLock.NewShareLockRegistry())

	victims, err := svc.SoftDeleteWorkStoreRoles(ctx, 500, []string{domain.StoreTypeImage})
	if err != nil {
		t.Fatalf("未登记作品软删应放行，实际失败: %v", err)
	}
	if len(victims) != 1 {
		t.Fatalf("未登记作品应完成软删，实际 %v", victims)
	}
}

// TestSoftDeleteWorkStoreRolesEmptyRolesNoOp 显式角色集合语义锚：空集=不软删任何行。
// 「空选择=全量板块」的展开归发起方（taskManager 展开为封闭枚举全集后传入），
// 能力对空集不再隐式扩为全量——防止调用点漏展开时把全量软删误当默认行为
func TestSoftDeleteWorkStoreRolesEmptyRolesNoOp(t *testing.T) {
	ctx := context.Background()
	deleter := &lockTestDeleter{}
	svc := newLockTestService(deleter, shareLock.NewShareLockRegistry())

	victims, err := svc.SoftDeleteWorkStoreRoles(ctx, 500, nil)
	if err != nil {
		t.Fatalf("空角色集应为 no-op，实际失败: %v", err)
	}
	if len(victims) != 0 {
		t.Fatalf("空角色集不应软删任何行，实际 %v", victims)
	}
	if len(deleter.backupDeletedIds) != 0 || len(deleter.discardedStoreIds) != 0 {
		t.Fatalf("空角色集不应触碰 store 行，实际备份软删 %v 废弃 %v", deleter.backupDeletedIds, deleter.discardedStoreIds)
	}
}
