package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// PersistentStore 文件持久存储记录
type PersistentStore struct {
	*model.BaseEntity
	FilePath          sql.NullString `gorm:"column:file_path;uniqueIndex" json:"filePath"`
	FileName          sql.NullString `gorm:"column:file_name" json:"fileName"`
	FilenameExtension sql.NullString `gorm:"column:filename_extension" json:"filenameExtension"`
	FileSize          sql.NullInt64  `gorm:"column:file_size" json:"fileSize"`
}

func (PersistentStore) TableName() string {
	return "persistent_store"
}

func NewPersistentStore() *PersistentStore {
	return &PersistentStore{
		BaseEntity: &model.BaseEntity{},
	}
}
