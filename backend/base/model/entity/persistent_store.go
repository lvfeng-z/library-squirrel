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
	// Status 落盘状态：0=未完成、1=完成。0 是合法取值（断点续传重置），用 NullInt64 区分未设置以走 GORM Updates
	Status            sql.NullInt64  `gorm:"column:status;default:0" json:"status"`
	Width             sql.NullInt64  `gorm:"column:width;default:0" json:"width"`  // 图片宽度（像素），非图片为 0
	Height            sql.NullInt64  `gorm:"column:height;default:0" json:"height"` // 图片高度（像素），非图片为 0
}

func (PersistentStore) TableName() string {
	return "persistent_store"
}

func NewPersistentStore() *PersistentStore {
	return &PersistentStore{
		BaseEntity: &model.BaseEntity{},
	}
}
