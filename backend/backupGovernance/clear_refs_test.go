package backupGovernance

import (
	"context"
	"testing"
)

// countingReferencer 记录清列调用的引用方替身
type countingReferencer struct {
	name string
	ids  [][]int64
}

func (c *countingReferencer) Name() string { return c.name }
func (c *countingReferencer) ListReferencedBackupIDs(ctx context.Context) ([]int64, error) {
	return nil, nil
}
func (c *countingReferencer) ClearBackupRefsByBackupIDs(ctx context.Context, ids []int64) error {
	c.ids = append(c.ids, ids)
	return nil
}

// TestClearBackupRefs 联动清列：各引用方均收到 ID 集；空集为无操作（不触达任何引用方）
func TestClearBackupRefs(t *testing.T) {
	refA := &countingReferencer{name: "A"}
	refB := &countingReferencer{name: "B"}
	svc := NewService(nil, []BackupReferencer{refA, refB}, &stubRetention{days: 7})
	ctx := context.Background()

	if err := svc.ClearBackupRefs(ctx, []int64{5, 9}); err != nil {
		t.Fatalf("联动清列失败: %v", err)
	}
	for _, ref := range []*countingReferencer{refA, refB} {
		if len(ref.ids) != 1 || len(ref.ids[0]) != 2 || ref.ids[0][0] != 5 || ref.ids[0][1] != 9 {
			t.Fatalf("引用方 %s 应收到 ID 集 [5 9]，实际 %v", ref.name, ref.ids)
		}
	}

	if err := svc.ClearBackupRefs(ctx, nil); err != nil {
		t.Fatalf("空集应为无操作: %v", err)
	}
	if len(refA.ids) != 1 || len(refB.ids) != 1 {
		t.Fatal("空集调用不应触达引用方")
	}
}
