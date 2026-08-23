package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"

	"gorm.io/plugin/soft_delete"
)

// WorkSet 作品集
type WorkSet struct {
	*model.BaseEntity
	// 软删标志（毫秒时间戳，0=活）：Find/Count/Update 自动排除已删行、Delete 自动改写为打时间戳，Unscoped 逃逸。
	// 业务键唯一性由三列唯一索引 (site_id, site_work_set_id, deleted_at) 表达：活行占键、已删行按删除时刻互异释放键
	DeletedAt              soft_delete.DeletedAt `gorm:"column:deleted_at;index;softDelete:milli" json:"deletedAt"`
	SiteID                 sql.NullInt64         `gorm:"column:site_id" json:"siteId"`
	SiteWorkSetID          sql.NullString        `gorm:"column:site_work_set_id" json:"siteWorkSetId"`
	SiteWorkSetName        sql.NullString        `gorm:"column:site_work_set_name" json:"siteWorkSetName"`
	SiteAuthorID           sql.NullString        `gorm:"column:site_author_id" json:"siteAuthorId"`
	SiteWorkSetDescription sql.NullString        `gorm:"column:site_work_set_description" json:"siteWorkSetDescription"`
	SiteUploadTime         sql.NullInt64         `gorm:"column:site_upload_time" json:"siteUploadTime"`
	SiteUpdateTime         sql.NullInt64         `gorm:"column:site_update_time" json:"siteUpdateTime"`
	NickName               sql.NullString        `gorm:"column:nick_name" json:"nickName"`
	Description            sql.NullString        `gorm:"column:description" json:"description"`
	LastView               sql.NullInt64         `gorm:"column:last_view" json:"lastView"`
	// 封面作品引用（集级单值，NULL=未设置）：封面是作品集自身属性而非成员关系属性——
	// 可指向传递包含内任意作品（含子集作品）；解析时作品已删/不存在回退 MIN(sort_order) 直接成员
	CoverWorkID sql.NullInt64 `gorm:"column:cover_work_id" json:"coverWorkId"`
}

func NewWorkSet() *WorkSet {
	return &WorkSet{
		BaseEntity: &model.BaseEntity{},
	}
}

// TableName 指定表名
func (WorkSet) TableName() string {
	return "work_set"
}
