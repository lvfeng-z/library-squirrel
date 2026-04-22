package dto

import (
	"io"

	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
)

// TaskHandler 任务处理器接口
// 插件实现此接口来处理任务
type TaskHandler interface {
	// Create 创建任务
	// url: 需解析的url
	// 返回任务信息列表或错误
	Create(url string) ([]*TaskCreateResponse, error)
	// CreateWorkInfo 生成作品信息
	// task: 需处理的任务
	// 返回作品信息或错误
	CreateWorkInfo(task *entity2.Task) (*WorkResponse, error)
	// Start 开始任务
	// task: 需开始的任务
	// 返回资源读取器（io.ReadCloser）、WorkResponse 或错误
	// 调用方负责关闭返回的 ReadCloser
	Start(task *entity2.Task) (io.ReadCloser, *WorkResponse, error)
	// Retry 重试任务
	// task: 需要重试的任务
	// 返回作品信息或错误
	Retry(task *entity2.Task) (*WorkResponse, error)
	// Pause 暂停任务
	// param: 暂停任务所需的参数
	Pause(param *TaskResParam) error
	// Stop 停止任务
	// param: 停止任务所需的参数
	Stop(param *TaskResParam) error
	// Resume 恢复任务
	// param: 恢复任务所需的参数
	// 返回作品信息或错误
	Resume(param *TaskResParam) (*WorkResponse, error)
}

// TaskResParam 任务和资源参数
type TaskResParam struct {
	Task         *entity2.Task // 任务
	ResourceID   int64         // 资源ID
	ResourcePath string        // 资源路径
}

// TaskCreateResponse 任务创建响应
type TaskCreateResponse struct {
	PluginTaskID string                     `json:"pluginTaskId"` // 插件任务ID
	TaskName     string                     `json:"taskName"`     // 任务名称
	SiteWorkID   string                     `json:"siteWorkId"`   // 站点作品ID
	URL          string                     `json:"url"`          // 来源URL
	PluginData   string                     `json:"pluginData"`   // 插件数据(JSON字符串)
	SiteName     string                     `json:"siteName"`     // 站点名称
	Children     []*TaskCreateChildResponse `json:"children"`     // 子任务列表
}

// TaskCreateChildResponse 子任务创建响应
type TaskCreateChildResponse struct {
	TaskName   string `json:"taskName"`   // 任务名称
	SiteWorkID string `json:"siteWorkId"` // 站点作品ID
	URL        string `json:"url"`        // 来源URL
	PluginData string `json:"pluginData"` // 插件数据(JSON字符串)
	SiteName   string `json:"siteName"`   // 站点名称
}

// WorkResponse 作品响应
type WorkResponse struct {
	Work         *entity2.Work          `json:"work"`         // 作品信息
	Site         *entity2.Site          `json:"site"`         // 站点信息
	LocalAuthors []*entity2.LocalAuthor `json:"localAuthors"` // 本地作者数组
	LocalTags    []*entity2.LocalTag    `json:"localTags"`    // 本地标签数组
	SiteAuthors  []*SiteAuthorDTO       `json:"siteAuthors"`  // 站点作者数组
	SiteTags     []*SiteTagDTO          `json:"siteTags"`     // 站点标签数组
	WorkSets     []*WorkSetDTO          `json:"workSets"`     // 作品所属作品集
	Resource     *ResourceDTO           `json:"resource"`     // 资源信息
}

// SiteAuthorDTO 站点作者DTO
type SiteAuthorDTO struct {
	SiteAuthorID string `json:"siteAuthorId"` // 站点作者ID
	AuthorName   string `json:"authorName"`   // 作者名称
	URL          string `json:"url"`          // 作者页面URL
}

// SiteTagDTO 站点标签DTO
type SiteTagDTO struct {
	SiteTagID   string `json:"siteTagId"`   // 站点标签ID
	TagName     string `json:"tagName"`     // 标签名称
	Description string `json:"description"` // 标签描述
	URL         string `json:"url"`         // 标签页面URL
}

// WorkSetDTO 作品集DTO
type WorkSetDTO struct {
	WorkSetID   int64  `json:"workSetId"`   // 作品集ID
	WorkSetName string `json:"workSetName"` // 作品集名称
}

// ResourceDTO 资源DTO
type ResourceDTO struct {
	ResourceID   int64  `json:"resourceId"`   // 资源ID
	URL          string `json:"url"`          // 资源URL
	Type         string `json:"type"`         // 资源类型
	Format       string `json:"format"`       // 资源格式
	LocalPath    string `json:"localPath"`    // 本地路径
	RemotePath   string `json:"remotePath"`   // 远程路径
	Size         int64  `json:"size"`         // 文件大小
	Completeness int    `json:"completeness"` // 完整度
}
