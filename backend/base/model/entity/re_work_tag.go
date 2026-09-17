package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// ReWorkTag 作品与标签关联
type ReWorkTag struct {
	*model.BaseEntity
	WorkID     sql.NullInt64 `gorm:"column:work_id;uniqueIndex:idx_re_work_tag_work_local_tag_ns;uniqueIndex:idx_re_work_tag_work_site_tag_ns" json:"workId"`
	TagType    sql.NullInt64 `gorm:"column:tag_type" json:"tagType"`
	LocalTagID sql.NullInt64 `gorm:"column:local_tag_id;uniqueIndex:idx_re_work_tag_work_local_tag_ns" json:"localTagId"`
	SiteTagID  sql.NullInt64 `gorm:"column:site_tag_id;uniqueIndex:idx_re_work_tag_work_site_tag_ns" json:"siteTagId"`
	// Namespace 关联级 namespace 维度值（开放字符串，空串=无 ns）。唯一键含本列——同作品同标签
	// 多 ns 关联并存（如 e-hentai 的 female:tagA 与 male:tagA 共用同一纯名 site_tag 行）
	Namespace string `gorm:"column:namespace;not null;default:'';uniqueIndex:idx_re_work_tag_work_local_tag_ns;uniqueIndex:idx_re_work_tag_work_site_tag_ns" json:"namespace"`
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
