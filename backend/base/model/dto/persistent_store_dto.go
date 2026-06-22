package dto

import (
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// NewPersistentStoreDTO 从 entity.PersistentStore 创建 PersistentStoreDTO
func NewPersistentStoreDTO(store *entity.PersistentStore) *sdkdto.PersistentStoreDTO {
	if store == nil {
		return nil
	}
	return &sdkdto.PersistentStoreDTO{
		ID:                store.GetID(),
		FilePath:          util.NullStringToPointer(store.FilePath),
		FileName:          util.NullStringToPointer(store.FileName),
		FilenameExtension: util.NullStringToPointer(store.FilenameExtension),
		Status:            store.Status,
		Width:             store.Width,
		Height:            store.Height,
		CreateTime:        store.GetCreateTime(),
		UpdateTime:        store.GetUpdateTime(),
	}
}

// ToPersistentStoreEntity 将 PersistentStoreDTO 转换为 PersistentStore 实体
func ToPersistentStoreEntity(dto *sdkdto.PersistentStoreDTO) *entity.PersistentStore {
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

	store.Status = dto.Status
	store.Width = dto.Width
	store.Height = dto.Height

	if dto.CreateTime != 0 {
		store.SetCreateTime(dto.CreateTime)
	}
	if dto.UpdateTime != 0 {
		store.SetUpdateTime(dto.UpdateTime)
	}

	return store
}
