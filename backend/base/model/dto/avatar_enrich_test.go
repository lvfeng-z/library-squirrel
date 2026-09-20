package dto

import (
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
)

func newAvatarTestStore(completedAt int64, filePath string) *entity.PersistentStore {
	st := entity.NewPersistentStore()
	st.CompletedAt = completedAt
	st.FilePath = sql.NullString{String: filePath, Valid: true}
	return st
}

// TestAvatarFilePathByStoreIDLiveness 口径锚定：仅落盘完成（completed_at > 0）且路径非空的行
// 进入索引；未完成、空路径、零 id 行不出现——引用这些行的作者头像字段保持 nil（占位图兜底）
func TestAvatarFilePathByStoreIDLiveness(t *testing.T) {
	completed := newAvatarTestStore(1000, "store/avatar/site/ab/pixiv_1.jpg")
	completed.SetID(11)
	incomplete := newAvatarTestStore(0, "store/avatar/site/cd/pixiv_2.jpg")
	incomplete.SetID(12)
	blankPath := newAvatarTestStore(1000, "")
	blankPath.SetID(13)
	zeroID := newAvatarTestStore(1000, "store/avatar/local/ef/local_9.png")

	index := AvatarFilePathByStoreID([]*entity.PersistentStore{completed, incomplete, blankPath, zeroID, nil})

	if len(index) != 1 {
		t.Fatalf("仅完整行应进入索引，实际 %d 条: %v", len(index), index)
	}
	got, ok := index[11]
	if !ok || got == nil || *got != "store/avatar/site/ab/pixiv_1.jpg" {
		t.Fatalf("完整行应产出头像路径，实际 %v", got)
	}
}

// TestAvatarFilePathByStoreIDEmpty 输入空集产出非 nil 空索引（展示链无可展示头像的合法态）
func TestAvatarFilePathByStoreIDEmpty(t *testing.T) {
	index := AvatarFilePathByStoreID([]*entity.PersistentStore{})
	if index == nil || len(index) != 0 {
		t.Fatalf("空输入应产出非 nil 空索引，实际 %v", index)
	}
}
