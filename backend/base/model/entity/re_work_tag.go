package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// ReWorkTag 作品与标签关联
type ReWorkTag struct {
	*model.BaseEntity
	WorkID     sql.NullInt64  `gorm:"column:work_id;uniqueIndex:idx_re_work_tag_work_local_tag;uniqueIndex:idx_re_work_tag_work_site_tag" json:"workId"`
	TagType    sql.NullInt64  `gorm:"column:tag_type" json:"tagType"`
	LocalTagID sql.NullInt64  `gorm:"column:local_tag_id;uniqueIndex:idx_re_work_tag_work_local_tag" json:"localTagId"`
	SiteTagID  sql.NullInt64  `gorm:"column:site_tag_id;uniqueIndex:idx_re_work_tag_work_site_tag" json:"siteTagId"`
	Namespace  sql.NullString `gorm:"column:namespace" json:"namespace"` // 关联级 namespace（site 关联=所指 site_tag.namespace 镜像；local 关联=用户自设/null）
	// Source 关联写入来源（constant.PLUGIN/MANUAL）：作品重拉只窄域重建插件来源关联，用户手动挂的关联不动；
	// 冲突 upsert 不写本列，先建行者的来源保持不变
	Source int64 `gorm:"column:source;not null;default:0" json:"source"`
}

func (ReWorkTag) TableName() string {
	return "re_work_tag"
}

func NewReWorkTag() *ReWorkTag {
	return &ReWorkTag{
		BaseEntity: &model.BaseEntity{},
	}
}
