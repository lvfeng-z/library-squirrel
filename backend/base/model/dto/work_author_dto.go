package dto

// WorkAuthorDTO 作品作者信息（包含本地作者和站点作者）
type WorkAuthorDTO struct {
	LocalAuthors []*RankedLocalAuthor `json:"localAuthors,omitempty"`
	SiteAuthors  []*RankedSiteAuthor  `json:"siteAuthors,omitempty"`
}

// WorkAuthorsResultDTO 批量作品作者信息返回结果
type WorkAuthorsResultDTO struct {
	WorkId       int64                `json:"workId"`
	LocalAuthors []*RankedLocalAuthor `json:"localAuthors,omitempty"`
	SiteAuthors  []*RankedSiteAuthor  `json:"siteAuthors,omitempty"`
}
