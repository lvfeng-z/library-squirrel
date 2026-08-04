package dto

// SearchType 搜索类型
type SearchType int

const (
	LocalTag        SearchType = 1
	SiteTag         SearchType = 2
	LocalAuthor     SearchType = 3
	SiteAuthor      SearchType = 4
	WorksSiteName   SearchType = 5
	WorksNickname   SearchType = 6
	WorksUploadTime SearchType = 7
	WorksLastView   SearchType = 8
	Media           SearchType = 9
	Site            SearchType = 10
	WorkSet         SearchType = 11
)

// SearchCondition 搜索条件
type SearchCondition struct {
	Type     SearchType         `json:"type"`
	Value    interface{}        `json:"value"`
	Operator WorkSearchOperator `json:"operator,omitempty"`
}

// WorkSearchOperator 作品搜索操作符
type WorkSearchOperator string

// 操作符常量
const (
	Equal          WorkSearchOperator = "="
	NotEqual       WorkSearchOperator = "!="
	GreaterThan    WorkSearchOperator = ">"
	LessThan       WorkSearchOperator = "<"
	GreaterOrEqual WorkSearchOperator = ">="
	LessOrEqual    WorkSearchOperator = "<="
	Like           WorkSearchOperator = "LIKE"
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
	Types   []SearchType `json:"types,omitempty"`
	Keyword string       `json:"keyword,omitempty"`
}

// NewSearchCondition 创建搜索条件
func NewSearchCondition(searchType SearchType, value interface{}) *SearchCondition {
	return &SearchCondition{
		Type:  searchType,
		Value: value,
	}
}
