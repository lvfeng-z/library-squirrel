package model

// RankedSiteAuthor 带排名的站点作者
type RankedSiteAuthor struct {
	ID                   int64  `json:"id"`
	SiteID               int64  `json:"siteId"`
	SiteAuthorID         string `json:"siteAuthorId"`
	AuthorName           string `json:"authorName"`
	FixedAuthorName      string `json:"fixedAuthorName"`
	SiteAuthorNameBefore string `json:"siteAuthorNameBefore"`
	Introduce            string `json:"introduce"`
	LocalAuthorID        int64  `json:"localAuthorId"`
	LastUse              int64  `json:"lastUse"`
	CreateTime           int64  `json:"createTime"`
	UpdateTime           int64  `json:"updateTime"`
	AuthorRank           int    `json:"authorRank"`
}

// RankedSiteAuthorWithWorkId 带作品ID的站点作者
type RankedSiteAuthorWithWorkId struct {
	RankedSiteAuthor
	WorkId int64 `json:"workId"`
}
