package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// SiteAuthor 站点作者
type SiteAuthor struct {
	*model.BaseEntity
	SiteID               sql.NullInt64  `gorm:"column:site_id;uniqueIndex:idx_site_author_site_site_author" json:"siteId"`
	SiteAuthorID         sql.NullString `gorm:"column:site_author_id;uniqueIndex:idx_site_author_site_site_author" json:"siteAuthorId"`
	AuthorName           sql.NullString `gorm:"column:author_name" json:"authorName"`
	FixedAuthorName      sql.NullString `gorm:"column:fixed_author_name" json:"fixedAuthorName"`
	SiteAuthorNameBefore sql.NullString `gorm:"column:site_author_name_before" json:"siteAuthorNameBefore"`
	Introduce            sql.NullString `gorm:"column:introduce" json:"introduce"`
	Homepage             sql.NullString `gorm:"column:homepage" json:"homepage"`
	LocalAuthorID        sql.NullInt64  `gorm:"column:local_author_id" json:"localAuthorId"`
	LastUse              sql.NullInt64  `gorm:"column:last_use" json:"lastUse"`
	// AvatarStoreID 头像文件的 persistent_store 行内嵌引用（作者行发起入库，拉取/导入编排事务内写；
	// 重拉与元数据回写均不进 upsert 覆盖域，随行保留）
	AvatarStoreID sql.NullInt64 `gorm:"column:avatar_store_id" json:"avatarStoreId"`
	// AvatarSourceURL 拉取期插件回报的头像来源 URL（站点声明权威域，进 upsert 覆盖域；
	// 后续拉取以其为变更检测比对键）
	AvatarSourceURL sql.NullString `gorm:"column:avatar_source_url" json:"avatarSourceUrl"`
}

func NewSiteAuthor() *SiteAuthor {
	return &SiteAuthor{
		BaseEntity: &model.BaseEntity{},
	}
}

func (SiteAuthor) TableName() string {
	return "site_author"
}
