package persistentStore

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
)

// insertFingerprintRow 造带指纹的完成行；deleted=true 时软删（模拟外部删除裁决/作品软删）
func insertFingerprintRow(t *testing.T, svc *Service, relPath, fp string, completed bool) *domain.PersistentStore {
	t.Helper()
	row := domain.NewPersistentStore()
	row.FilePath = sql.NullString{String: relPath, Valid: true}
	row.ContentFingerprint = sql.NullString{String: fp, Valid: true}
	if completed {
		row.CompletedAt = 1
	}
	if err := svc.repo.Create(context.Background(), row); err != nil {
		t.Fatalf("插入行失败: %v", err)
	}
	return row
}

// TestFsMonitorQueriesAgainstCurrentSchema fsmonitor 三供养方法在新表结构（无 invalid_at 列）上
// 真实 SQL 跑通，且软删行被排除——乙'退役 invalid_at 时三处硬编码列名条件残留曾致 SQL 运行时
// 报 no such column，关联/对账链路静默瘫痪（裸列名编译期不检查，故以内存库真跑锚定）
func TestFsMonitorQueriesAgainstCurrentSchema(t *testing.T) {
	svc, _ := newLifecycleTestService(t)
	ctx := context.Background()

	active := insertFingerprintRow(t, svc, "store/resource/active.mp4", "fp-a", true)
	_ = insertFingerprintRow(t, svc, "store/resource/unfinished.mp4", "fp-b", false)
	deletedRow := insertFingerprintRow(t, svc, "store/resource/deleted.mp4", "fp-c", true)
	if err := svc.repo.SoftDeleteWithBackup(ctx, deletedRow.GetID(), 0); err != nil {
		t.Fatalf("软删失败: %v", err)
	}

	// GetByFingerprint：命中在位完成行；软删行（含同指纹）不命中
	if got, err := svc.GetByFingerprint(ctx, "fp-a", "other/path.mp4"); err != nil || got == nil || got.GetID() != active.GetID() {
		t.Fatalf("GetByFingerprint 期望命中在位行 id=%d，实际 got=%v err=%v", active.GetID(), got, err)
	}
	if got, err := svc.GetByFingerprint(ctx, "fp-c", "other/path.mp4"); err != nil || got != nil {
		t.Fatalf("GetByFingerprint 软删行指纹须被排除（scope 过滤），实际 got=%v err=%v", got, err)
	}

	// GetByFilePathComplete：在位完成行命中；软删行/未完成行不命中
	if got, err := svc.GetByFilePathComplete(ctx, "store/resource/active.mp4"); err != nil || got == nil {
		t.Fatalf("GetByFilePathComplete 在位完成行期望命中，实际 got=%v err=%v", got, err)
	}
	if got, err := svc.GetByFilePathComplete(ctx, "store/resource/deleted.mp4"); err != nil || got != nil {
		t.Fatalf("GetByFilePathComplete 软删行须被排除，实际 got=%v err=%v", got, err)
	}
	if got, err := svc.GetByFilePathComplete(ctx, "store/resource/unfinished.mp4"); err != nil || got != nil {
		t.Fatalf("GetByFilePathComplete 未完成行须被排除，实际 got=%v err=%v", got, err)
	}

	// ListValidComplete：对账基线只含在位完成行
	rows, err := svc.ListValidComplete(ctx)
	if err != nil {
		t.Fatalf("ListValidComplete 失败（对账链路即此前报 no such column 处）: %v", err)
	}
	if len(rows) != 1 || rows[0].GetID() != active.GetID() {
		t.Fatalf("ListValidComplete 期望仅含在位完成行 id=%d，实际 %d 行", active.GetID(), len(rows))
	}
}
