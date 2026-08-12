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
	// ContentFingerprint 内容指纹（size + 头部 64KB SHA256），用于文件移动/重命名的内容关联匹配
	// 落盘完成时同步算入；存量记录由回填补入；缺失时无法参与移动匹配（降级为删除/新增）
	ContentFingerprint sql.NullString `gorm:"column:content_fingerprint" json:"contentFingerprint"`
	// InvalidAt 失效时间戳（毫秒，0=有效）。外部删除且用户不复原、或移出 workDir 时置位
	// 自定义软删除：fsmonitor 关联查询显式过滤 invalid_at=0；其他查询按需推广
	InvalidAt int64 `gorm:"column:invalid_at;default:0" json:"invalidAt"`
}

func (PersistentStore) TableName() string {
	return "persistent_store"
}

func NewPersistentStore() *PersistentStore {
	return &PersistentStore{
		BaseEntity: &model.BaseEntity{},
	}
}
