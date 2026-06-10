package dto

// SearchType 搜索类型
type SearchType int

// SearchCondition 搜索条件
type SearchCondition struct {
	// 查询参数类型
	Type SearchType `json:"type"`
	// 查询参数值
	Value interface{} `json:"value"`
	// 操作符 (等于、大于、小于等)
	Operator string `json:"operator,omitempty"`
}

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

// NewSearchCondition 创建搜索条件
func NewSearchCondition(searchType SearchType, value interface{}) *SearchCondition {
	return &SearchCondition{
		Type:  searchType,
		Value: value,
	}
}
