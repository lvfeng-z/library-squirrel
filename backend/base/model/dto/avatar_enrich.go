package dto

import (
	"github.com/library-squirrel/backend/base/model/entity"
)

// AvatarFilePathByStoreID 按 persistent_store 行 DB id 建立可展示头像路径索引（作者展示 DTO 的
// 头像字段后置 enrich 消费）。可展示口径：行存活（软删行经 GORM 软删 scope 在查询侧已排除）、
// 落盘完成（completed_at > 0）且路径非空——引用指向软删/未完成/空路径行的作者头像字段保持 nil，
// 展示侧以占位图兜底
func AvatarFilePathByStoreID(stores []*entity.PersistentStore) map[int64]*string {
	result := make(map[int64]*string, len(stores))
	for _, st := range stores {
		if st == nil || st.GetID() <= 0 || st.CompletedAt <= 0 || !st.FilePath.Valid || st.FilePath.String == "" {
			continue
		}
		path := st.FilePath.String
		result[st.GetID()] = &path
	}
	return result
}
