package dto

import (
	"database/sql"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/library-squirrel/backend/util"
)

// ========== 本地作者扫描行 ==========

// LocalAuthorScanRow 本地作者SQL扫描行
type LocalAuthorScanRow struct {
	ID         int64          `gorm:"column:id"`
	AuthorName sql.NullString `gorm:"column:author_name"`
	Introduce  sql.NullString `gorm:"column:introduce"`
	LastUse    sql.NullInt64  `gorm:"column:last_use"`
	CreateTime int64          `gorm:"column:create_time"`
	UpdateTime int64          `gorm:"column:update_time"`
}

func (r LocalAuthorScanRow) toDTO() sdkdto.LocalAuthorDTO {
	return sdkdto.LocalAuthorDTO{
		ID:         r.ID,
		AuthorName: util.NullStringToPointer(r.AuthorName),
		Introduce:  util.NullStringToPointer(r.Introduce),
		LastUse:    util.NullInt64ToPointer(r.LastUse),
		CreateTime: r.CreateTime,
		UpdateTime: r.UpdateTime,
	}
}

// LocalAuthorRankScanRow 带角色和排序的本地作者SQL扫描行
type LocalAuthorRankScanRow struct {
	LocalAuthorScanRow
	RoleName  sql.NullString `gorm:"column:role_name"`
	SortOrder sql.NullInt64  `gorm:"column:sort_order"`
}

// ToRankedLocalAuthor 将扫描行转换为带排序的本地作者DTO
func (r LocalAuthorRankScanRow) ToRankedLocalAuthor() *sdkdto.RankedLocalAuthor {
	return &sdkdto.RankedLocalAuthor{
		Author:    r.LocalAuthorScanRow.toDTO(),
		RoleName:  r.RoleName.String,
		SortOrder: int(r.SortOrder.Int64),
	}
}

// LocalAuthorRankWithWorkIdScanRow 带作品ID的本地作者SQL扫描行
type LocalAuthorRankWithWorkIdScanRow struct {
	LocalAuthorRankScanRow
	WorkId sql.NullInt64 `gorm:"column:work_id"`
}

// ToRankedLocalAuthorWithWorkId 将扫描行转换为带作品ID的本地作者DTO
func (r LocalAuthorRankWithWorkIdScanRow) ToRankedLocalAuthorWithWorkId() *sdkdto.RankedLocalAuthorWithWorkId {
	return &sdkdto.RankedLocalAuthorWithWorkId{
		Author:    r.LocalAuthorScanRow.toDTO(),
		RoleName:  r.RoleName.String,
		SortOrder: int(r.SortOrder.Int64),
		WorkId:    r.WorkId.Int64,
	}
}

// ========== 站点作者扫描行 ==========

// SiteAuthorScanRow 站点作者SQL扫描行
type SiteAuthorScanRow struct {
	ID                   int64          `gorm:"column:id"`
	SiteID               sql.NullInt64  `gorm:"column:site_id"`
	SiteAuthorID         sql.NullString `gorm:"column:site_author_id"`
	AuthorName           sql.NullString `gorm:"column:author_name"`
	FixedAuthorName      sql.NullString `gorm:"column:fixed_author_name"`
	SiteAuthorNameBefore sql.NullString `gorm:"column:site_author_name_before"`
	Introduce            sql.NullString `gorm:"column:introduce"`
	Homepage             sql.NullString `gorm:"column:homepage"`
	LocalAuthorID        sql.NullInt64  `gorm:"column:local_author_id"`
	LastUse              sql.NullInt64  `gorm:"column:last_use"`
	CreateTime           int64          `gorm:"column:create_time"`
	UpdateTime           int64          `gorm:"column:update_time"`
}

func (r SiteAuthorScanRow) toDTO() sdkdto.SiteAuthorDTO {
	return sdkdto.SiteAuthorDTO{
		ID:                   r.ID,
		SiteID:               util.NullInt64ToPointer(r.SiteID),
		SiteAuthorID:         util.NullStringToPointer(r.SiteAuthorID),
		AuthorName:           util.NullStringToPointer(r.AuthorName),
		FixedAuthorName:      util.NullStringToPointer(r.FixedAuthorName),
		SiteAuthorNameBefore: util.NullStringToPointer(r.SiteAuthorNameBefore),
		Introduce:            util.NullStringToPointer(r.Introduce),
		Homepage:             util.NullStringToPointer(r.Homepage),
		LocalAuthorID:        util.NullInt64ToPointer(r.LocalAuthorID),
		LastUse:              util.NullInt64ToPointer(r.LastUse),
		CreateTime:           r.CreateTime,
		UpdateTime:           r.UpdateTime,
	}
}

// SiteAuthorRankScanRow 带角色和排序的站点作者SQL扫描行
type SiteAuthorRankScanRow struct {
	SiteAuthorScanRow
	RoleName  sql.NullString `gorm:"column:role_name"`
	SortOrder sql.NullInt64  `gorm:"column:sort_order"`
}

// ToRankedSiteAuthor 将扫描行转换为带排序的站点作者DTO
func (r SiteAuthorRankScanRow) ToRankedSiteAuthor() *sdkdto.RankedSiteAuthor {
	return &sdkdto.RankedSiteAuthor{
		Author:    r.SiteAuthorScanRow.toDTO(),
		RoleName:  r.RoleName.String,
		SortOrder: int(r.SortOrder.Int64),
	}
}

// SiteAuthorRankWithWorkIdScanRow 带作品ID的站点作者SQL扫描行
type SiteAuthorRankWithWorkIdScanRow struct {
	SiteAuthorRankScanRow
	WorkId sql.NullInt64 `gorm:"column:work_id"`
}

// ToRankedSiteAuthorWithWorkId 将扫描行转换为带作品ID的站点作者DTO
func (r SiteAuthorRankWithWorkIdScanRow) ToRankedSiteAuthorWithWorkId() *sdkdto.RankedSiteAuthorWithWorkId {
	return &sdkdto.RankedSiteAuthorWithWorkId{
		Author:    r.SiteAuthorScanRow.toDTO(),
		RoleName:  r.RoleName.String,
		SortOrder: int(r.SortOrder.Int64),
		WorkId:    r.WorkId.Int64,
	}
}
