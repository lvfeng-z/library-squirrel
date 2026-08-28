package recycleBin

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/shareLock"
)

// ==== 作品锁守卫测试 ====
// 复原置换（RestoreStore）与复原覆盖转移（RestoreWork overwrite）触碰活行 store 文件前查锁：
// 命中返回 shareLock.ErrWorkLocked 且不动任何行；强制解锁后放行

// TestRestoreStoreRejectsLockedWork 复原置换守卫：作品被持有→拒绝且占位活行不动；
// 强制解锁→重试放行，完成同键置换
func TestRestoreStoreRejectsLockedWork(t *testing.T) {
	env := newRestoreStoreEnv(t)
	const victimPath = "store/resource/a/被拉取中版本.png"
	resourceId, victimId := env.seedVictim(t, domain.StoreTypeImage, 601)
	placeholderId := env.seedLiveSameKey(t, resourceId, domain.StoreTypeImage, victimPath)
	var workId int64
	env.db.Raw("SELECT work_id FROM resource WHERE id = ?", resourceId).Scan(&workId)

	env.lock.Register(context.Background(), []int64{workId}, "session-a")
	if err := env.svc.RestoreStore(context.Background(), victimId); !errors.Is(err, shareLock.ErrWorkLocked) {
		t.Fatalf("锁命中应返回 shareLock.ErrWorkLocked，实际 %v", err)
	}
	// 守卫命中不得触碰任何行：占位活行仍活、victim 仍为已删态
	var placeholderDeleted, victimDeleted int64
	env.db.Raw("SELECT deleted_at FROM persistent_store WHERE id = ?", placeholderId).Scan(&placeholderDeleted)
	env.db.Raw("SELECT deleted_at FROM persistent_store WHERE id = ?", victimId).Scan(&victimDeleted)
	if placeholderDeleted != 0 {
		t.Fatalf("锁命中不应置换占位活行，实际已软删")
	}
	if victimDeleted == 0 {
		t.Fatalf("锁命中不应复活本行，实际已复活")
	}

	// 强制解锁后重试：放行并完成置换
	env.lock.ForceUnlock(context.Background(), workId)
	if err := env.svc.RestoreStore(context.Background(), victimId); err != nil {
		t.Fatalf("解锁后复原应放行，实际失败: %v", err)
	}
	env.db.Raw("SELECT deleted_at FROM persistent_store WHERE id = ?", placeholderId).Scan(&placeholderDeleted)
	env.db.Raw("SELECT deleted_at FROM persistent_store WHERE id = ?", victimId).Scan(&victimDeleted)
	if placeholderDeleted == 0 || victimDeleted != 0 {
		t.Fatalf("解锁后应完成置换（占位行软删、本行复活），实际占位 deleted_at=%d 本行 deleted_at=%d", placeholderDeleted, victimDeleted)
	}
}

// lockGateWorkRestorer WorkRestorer 记账桩：预置冲突占位作品并记录覆盖转移调用
type lockGateWorkRestorer struct {
	mockWorkRestorer
	deleted       *domain.Work // 被复原的已删作品（带业务键供冲突检测）
	placeholder   *domain.Work // 业务键占位的活作品（覆盖转移对象）
	softDeletedId []int64      // 覆盖转移调用记录
}

func (m *lockGateWorkRestorer) GetDeletedWork(ctx context.Context, id int64) (*domain.Work, error) {
	return m.deleted, nil
}

func (m *lockGateWorkRestorer) GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*domain.Work, error) {
	return m.placeholder, nil
}

func (m *lockGateWorkRestorer) SoftDeleteWork(ctx context.Context, workId int64) error {
	m.softDeletedId = append(m.softDeletedId, workId)
	return nil
}

// TestRestoreWorkRejectsLockedPlaceholder 复原覆盖转移守卫：占位作品被持有→拒绝且不转移；
// 强制解锁→重试放行，完成覆盖转移
func TestRestoreWorkRejectsLockedPlaceholder(t *testing.T) {
	ctx := context.Background()
	deleted := domain.NewWork()
	deleted.SetID(100)
	deleted.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	deleted.SiteWorkID = sql.NullString{String: "sw1", Valid: true}
	placeholder := domain.NewWork()
	placeholder.SetID(200)
	restorer := &lockGateWorkRestorer{deleted: deleted, placeholder: placeholder}
	lock := shareLock.NewShareLockRegistry()
	svc := NewService(restorer, nil, nil, nil, nil, nil, nil, nil, func() string { return t.TempDir() }, nil, nil, nil, nil, lock, nil)

	lock.Register(ctx, []int64{200}, "session-b")
	if _, err := svc.RestoreWork(ctx, 100, true); !errors.Is(err, shareLock.ErrWorkLocked) {
		t.Fatalf("锁命中应返回 shareLock.ErrWorkLocked，实际 %v", err)
	}
	if len(restorer.softDeletedId) != 0 {
		t.Fatalf("锁命中不应转移占位作品，实际已转移 %v", restorer.softDeletedId)
	}

	lock.ForceUnlock(ctx, 200)
	if _, err := svc.RestoreWork(ctx, 100, true); err != nil {
		t.Fatalf("解锁后复原应放行，实际失败: %v", err)
	}
	if len(restorer.softDeletedId) != 1 || restorer.softDeletedId[0] != 200 {
		t.Fatalf("解锁后应完成占位作品 200 覆盖转移，实际 %v", restorer.softDeletedId)
	}
}
