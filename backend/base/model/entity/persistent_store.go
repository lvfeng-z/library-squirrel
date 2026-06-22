package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

const (
	// StoreStatusIncomplete 未完成
	StoreStatusIncomplete = 0
	// StoreStatusComplete 已完成
	StoreStatusComplete = 1
)

// PersistentStore 文件持久存储记录
type PersistentStore struct {
	*model.BaseEntity
	FilePath          sql.NullString `gorm:"column:file_path;uniqueIndex" json:"filePath"`
	FileName          sql.NullString `gorm:"column:file_name" json:"fileName"`
	FilenameExtension sql.NullString `gorm:"column:filename_extension" json:"filenameExtension"`
	Status            int            `gorm:"column:status;default:0" json:"status"` // 0=未完成，1=完成
	Width             int            `gorm:"column:width;default:0" json:"width"`   // 图片宽度（像素），非图片为 0
	Height            int            `gorm:"column:height;default:0" json:"height"` // 图片高度（像素），非图片为 0
}

func (PersistentStore) TableName() string {
	return "persistent_store"
}

func NewPersistentStore() *PersistentStore {
	return &PersistentStore{
		BaseEntity: &model.BaseEntity{},
	}
}
