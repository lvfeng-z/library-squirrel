package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"

	"gorm.io/plugin/soft_delete"
)

// Work 作品
type Work struct {
	*model.BaseEntity
	// 软删标志（毫秒时间戳，0=活）：Find/Count/Update 自动排除已删行、Delete 自动改写为打时间戳，Unscoped 逃逸
	DeletedAt           soft_delete.DeletedAt `gorm:"column:deleted_at;index;softDelete:milli" json:"deletedAt"`
	SiteID              sql.NullInt64         `gorm:"column:site_id" json:"siteId"`
	SiteWorkID          sql.NullString        `gorm:"column:site_work_id" json:"siteWorkId"`
	SiteWorkName        sql.NullString        `gorm:"column:site_work_name" json:"siteWorkName"`
	SiteAuthorID        sql.NullString        `gorm:"column:site_author_id" json:"siteAuthorId"`
	SiteWorkDescription sql.NullString        `gorm:"column:site_work_description" json:"siteWorkDescription"`
	SiteUploadTime      sql.NullInt64         `gorm:"column:site_upload_time" json:"siteUploadTime"`
	SiteUpdateTime      sql.NullInt64         `gorm:"column:site_update_time" json:"siteUpdateTime"`
	NickName            sql.NullString        `gorm:"column:nick_name" json:"nickName"`
	LocalAuthorID       sql.NullInt64         `gorm:"column:local_author_id" json:"localAuthorId"`
	LastView            sql.NullInt64         `gorm:"column:last_view" json:"lastView"`
}

func NewWork() *Work {
	return &Work{
		BaseEntity: &model.BaseEntity{},
	}
}

func (Work) TableName() string {
	return "work"
}
