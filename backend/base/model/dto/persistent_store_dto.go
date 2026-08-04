package dto

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// PersistentStoreDTO 文件持久存储数据传输对象
type PersistentStoreDTO struct {
	ID                int64   `json:"id"`
	FilePath          *string `json:"filePath"`
	FileName          *string `json:"fileName"`
	FilenameExtension *string `json:"filenameExtension"`
	Status            int     `json:"status"`   // 0=未完成，1=完成
	Width             int     `json:"width"`    // 图片宽度（像素），非图片为 0
	Height            int     `json:"height"`   // 图片高度（像素），非图片为 0
	CreateTime        int64   `json:"createTime"`
	UpdateTime        int64   `json:"updateTime"`
}

// NewPersistentStoreDTO 从 entity.PersistentStore 创建 PersistentStoreDTO
func NewPersistentStoreDTO(store *entity.PersistentStore) *PersistentStoreDTO {
	if store == nil {
		return nil
	}
	return &PersistentStoreDTO{
		ID:                store.GetID(),
		FilePath:          util.NullStringToPointer(store.FilePath),
		FileName:          util.NullStringToPointer(store.FileName),
		FilenameExtension: util.NullStringToPointer(store.FilenameExtension),
		Status:            int(store.Status.Int64),
		Width:             int(store.Width.Int64),
		Height:            int(store.Height.Int64),
		CreateTime:        store.GetCreateTime(),
		UpdateTime:        store.GetUpdateTime(),
	}
}

// ToPersistentStoreEntity 将 PersistentStoreDTO 转换为 PersistentStore 实体
func ToPersistentStoreEntity(dto *PersistentStoreDTO) *entity.PersistentStore {
	if dto == nil {
		return nil
	}

	store := entity.NewPersistentStore()

	if dto.ID != 0 {
		store.SetID(dto.ID)
	}

	if dto.FilePath != nil {
		store.FilePath.Valid = true
		store.FilePath.String = *dto.FilePath
	}

	if dto.FileName != nil {
		store.FileName.Valid = true
		store.FileName.String = *dto.FileName
	}

	if dto.FilenameExtension != nil {
		store.FilenameExtension.Valid = true
		store.FilenameExtension.String = *dto.FilenameExtension
	}

	store.Status = sql.NullInt64{Int64: int64(dto.Status), Valid: true}
	store.Width = sql.NullInt64{Int64: int64(dto.Width), Valid: true}
	store.Height = sql.NullInt64{Int64: int64(dto.Height), Valid: true}

	if dto.CreateTime != 0 {
		store.SetCreateTime(dto.CreateTime)
	}
	if dto.UpdateTime != 0 {
		store.SetUpdateTime(dto.UpdateTime)
	}

	return store
}
