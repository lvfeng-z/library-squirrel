package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"

	"gorm.io/plugin/soft_delete"
)

// PersistentStore 文件持久存储记录
type PersistentStore struct {
	*model.BaseEntity
	// file_path 的唯一性由部分唯一索引 idx_persistent_store_file_path_active（WHERE deleted_at = 0）承担：
	// 已删行释放路径，重新下载同路径文件合法插入新行
	FilePath          sql.NullString `gorm:"column:file_path" json:"filePath"`
	FileName          sql.NullString `gorm:"column:file_name" json:"fileName"`
	FilenameExtension sql.NullString `gorm:"column:filename_extension" json:"filenameExtension"`
	// CompletedAt 落盘完成时刻（毫秒时间戳，0=未完成）
	// 原 0/1 枚举 status 列改名改型：SQLite 无列注释，_at 家族自释义且携带完成时间信息
	CompletedAt int64         `gorm:"column:completed_at;default:0" json:"completedAt"`
	Width       sql.NullInt64 `gorm:"column:width;default:0" json:"width"`   // 图片宽度（像素），非图片为 0
	Height      sql.NullInt64 `gorm:"column:height;default:0" json:"height"` // 图片高度（像素），非图片为 0
	// ContentFingerprint 内容指纹（size + 头部 64KB SHA256），用于文件移动/重命名的内容关联匹配
	// 落盘完成时同步算入；存量记录由回填补入；缺失时无法参与移动匹配（降级为删除/新增）
	ContentFingerprint sql.NullString `gorm:"column:content_fingerprint" json:"contentFingerprint"`
	// DeletedAt 软删标志（毫秒时间戳，0=在位）：文件已被移离原路径、记录保留待追踪。
	// 写入方两类：作品软删链（文件移 backup，复原链清除）；fsmonitor 外部变更裁决不复原（原 invalid_at 退役并入）。
	// 彻底删除时行物理消亡。查询经 GORM 自动排除已删行（Unscoped/IncludeDeleted 逃逸）
	DeletedAt soft_delete.DeletedAt `gorm:"column:deleted_at;index;softDelete:milli" json:"deletedAt"`
}

func (PersistentStore) TableName() string {
	return "persistent_store"
}

func NewPersistentStore() *PersistentStore {
	return &PersistentStore{
		BaseEntity: &model.BaseEntity{},
	}
}
