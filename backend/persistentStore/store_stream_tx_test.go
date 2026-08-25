package persistentStore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
)

// TestStoreStreamInsideTransactionOnExistingPath 替换流自死锁回归：建资源事务内对同路径已有行调
// StoreStream（existing 分支经 ResetCompleted 重置 completed_at）。MaxOpenConns=1 下非事务感知
// 连接的写会向连接池另取连接，与外层事务占用的唯一连接互相等待形成自死锁（生产形态：替换流
// startDownload 事务内首个 store 落盘）；超时守卫锚定该形态，事务提交后断言行复活为未完成态。
func TestStoreStreamInsideTransactionOnExistingPath(t *testing.T) {
	svc, workDir := newLifecycleTestService(t)
	ctx := context.Background()
	db := svc.repo.(*PersistentStoreRepository).GORM()

	// 事务外先建同路径行（首跑形态），completed_at=1 模拟已完成
	row := insertStoreRow(t, svc, "store/resource/a.png")
	if err := os.WriteFile(filepath.Join(workDir, "store/resource/a.png"), []byte("old"), 0o644); err != nil {
		t.Fatalf("造源文件失败: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- database.WithTransactionContext(ctx, db, func(tx *gorm.DB) error {
			txCtx := context.WithValue(ctx, database.TxKey, tx)
			storeId, writer, err := svc.StoreStream(txCtx, "store/resource/a.png", "a.png")
			if err != nil {
				return err
			}
			if storeId != row.GetID() {
				writer.Close()
				t.Errorf("同路径应复用已有行 id=%d，实际 %d", row.GetID(), storeId)
			}
			return writer.Close()
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("事务内 StoreStream 失败: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("事务内 StoreStream 同路径 30s 无进展：疑似连接池自死锁（事务占唯一连接 + 内层写另取连接）")
	}

	// ResetCompleted 经事务生效：completed_at 重置为 0（未完成），行保持活行
	if got := getRowById(t, svc, row.GetID()); got.CompletedAt != 0 || got.DeletedAt != 0 {
		t.Fatalf("事务提交后行须为活行且 completed_at=0，实际 completed_at=%d deleted_at=%d", got.CompletedAt, got.DeletedAt)
	}
}
