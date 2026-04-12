package model

// SearchType 搜索类型
type SearchType int

const (
	SearchTypeLocalTag        SearchType = 1
	SearchTypeSiteTag         SearchType = 2
	SearchTypeLocalAuthor     SearchType = 3
	SearchTypeSiteAuthor      SearchType = 4
	SearchTypeWorksSiteName   SearchType = 5
	SearchTypeWorksNickname   SearchType = 6
	SearchTypeWorksUploadTime SearchType = 7
	SearchTypeWorksLastView   SearchType = 8
	SearchTypeMediaType       SearchType = 9
	SearchTypeSite            SearchType = 10
	SearchTypeWorkSet         SearchType = 11
)

// SearchCondition 搜索条件
type SearchCondition struct {
	// 查询参数类型
	Type SearchType `json:"type"`
	// 查询参数值
	Value interface{} `json:"value"`
	// 操作符 (等于、大于、小于等)
	Operator string `json:"operator,omitempty"`
}

// Operator 操作符
const (
	OperatorEqual          = "="
	OperatorNotEqual       = "!="
	OperatorGreaterThan    = ">"
	OperatorLessThan       = "<"
	OperatorGreaterOrEqual = ">="
	OperatorLessOrEqual    = "<="
	OperatorLike           = "LIKE"
)

// MediaType 媒体类型
type MediaType int

const (
	MediaTypePicture  MediaType = 1
	MediaTypeVideo    MediaType = 2
	MediaTypeDocument MediaType = 3
	MediaTypeAudio    MediaType = 4
)

// MediaExtMapping 媒体类型对应的扩展名映射
var MediaExtMapping = map[MediaType][]string{
	MediaTypePicture:  {".jpg", ".png", ".jpeg", ".gif"},
	MediaTypeVideo:    {".mp4", ".avi", ".mkv"},
	MediaTypeDocument: {".pdf", ".docx", ".doc", ".xlsx", ".txt"},
	MediaTypeAudio:    {".mp3", ".wav", ".aac"},
}

// SearchConditionQuery 搜索条件分页查询请求
type SearchConditionQuery struct {
	// 类型过滤
	Types []SearchType `json:"types,omitempty"`
	// 关键词
	Keyword string `json:"keyword,omitempty"`
}

// NewSearchCondition 创建搜索条件
func NewSearchCondition(searchType SearchType, value interface{}) *SearchCondition {
	return &SearchCondition{
		Type:  searchType,
		Value: value,
	}
}
