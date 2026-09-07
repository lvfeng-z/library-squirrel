package dto

import (
	"strings"

	"github.com/library-squirrel/backend/base/model"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// AssembleTaskDTO 从核心任务行与领域行组装 TaskDTO：控制字段取 task，作品任务领域
// 字段取 workTask（nil 时领域字段输出零值）。shareTask 为分享接收领域行——当前 TaskDTO
// 无分享领域字段，参数保留供消费侧统一走三行组装口径
func AssembleTaskDTO(task *entity2.Task, workTask *entity2.WorkTask, shareTask *entity2.ShareTask) *sdkdto.TaskDTO {
	if task == nil {
		return nil
	}
	dto := &sdkdto.TaskDTO{
		Id:           task.GetID(),
		HasChild:     util.NullBoolToPointer(task.HasChild),
		Pid:          util.NullInt64ToPointer(task.Pid),
		TaskName:     util.NullStringToPointer(task.TaskName),
		Status:       int32(task.Status),
		ErrorMessage: util.NullStringToPointer(task.ErrorMessage),
		TaskType:     util.NullStringToPointer(task.TaskType),
		CreateTime:   task.GetCreateTime(),
		UpdateTime:   task.GetUpdateTime(),
	}
	if workTask == nil {
		return dto
	}
	// involvedRoles:创建期声明的涉及板块(universe),逗号分隔→切片;NULL/空=nil(前端走兜底集)
	var involvedRoles []string
	if workTask.InvolvedRoles.Valid && workTask.InvolvedRoles.String != "" {
		for _, p := range strings.Split(workTask.InvolvedRoles.String, ",") {
			if r := strings.TrimSpace(p); r != "" {
				involvedRoles = append(involvedRoles, r)
			}
		}
	}
	dto.SiteId = util.NullInt64ToPointer(workTask.SiteID)
	dto.SiteWorkId = util.NullStringToPointer(workTask.SiteWorkID)
	dto.Url = util.NullStringToPointer(workTask.URL)
	dto.PendingResourceId = util.NullInt64ToPointer(workTask.PendingResourceID)
	dto.Continuable = util.NullBoolToPointer(workTask.Continuable)
	dto.PluginPublicId = util.NullStringToPointer(workTask.PluginPublicID)
	dto.PluginExtensionId = util.NullStringToPointer(workTask.PluginExtensionID)
	dto.PluginData = util.NullStringToPointer(workTask.PluginData)
	dto.InvolvedRoles = involvedRoles
	dto.ResourceType = workTask.ResourceType.String // NULL/未声明=零值 ""
	return dto
}

// ToTaskEntity 将 TaskDTO 转换为 Task 核心控制行实体（领域字段经 ToWorkTaskEntity 转换）
func ToTaskEntity(dto *sdkdto.TaskDTO) *entity2.Task {
	if dto == nil {
		return nil
	}

	entity := entity2.NewTask()

	// 设置基础字段
	if dto.Id != 0 {
		entity.SetID(dto.Id)
	}

	if dto.HasChild != nil {
		entity.HasChild.Valid = true
		entity.HasChild.Bool = *dto.HasChild
	}

	// pid 外键引用 task.id（无 id=0 行）：nil 或 0 均为根级语义 → NULL，写 0 必外键违约
	if dto.Pid != nil && *dto.Pid != 0 {
		entity.Pid.Valid = true
		entity.Pid.Int64 = *dto.Pid
	}

	if dto.TaskName != nil {
		entity.TaskName.Valid = true
		entity.TaskName.String = *dto.TaskName
	}

	entity.Status = int(dto.Status)

	if dto.ErrorMessage != nil {
		entity.ErrorMessage.Valid = true
		entity.ErrorMessage.String = *dto.ErrorMessage
	}

	// taskType:空=插件下载任务的显式类型缺失,不设置(保持 NULL)
	if dto.TaskType != nil && *dto.TaskType != "" {
		entity.TaskType.Valid = true
		entity.TaskType.String = *dto.TaskType
	}

	// 设置时间字段（如果DTO中有值则使用，否则让Repository自动处理）
	if dto.CreateTime != 0 {
		entity.SetCreateTime(dto.CreateTime)
	}
	if dto.UpdateTime != 0 {
		entity.SetUpdateTime(dto.UpdateTime)
	}

	return entity
}

// ToWorkTaskEntity 将 TaskDTO 的作品任务领域字段转换为 WorkTask 字段载体。
// 主键不在此绑定（共享主键值=核心行 id，由落库口 CreateForTask 覆写）；DTO 不携带任何
// 领域字段时返回 nil=无作品领域行（内置类型任务）
func ToWorkTaskEntity(dto *sdkdto.TaskDTO) *entity2.WorkTask {
	if dto == nil {
		return nil
	}
	hasDomain := dto.SiteId != nil || dto.SiteWorkId != nil || dto.Url != nil ||
		dto.PendingResourceId != nil || dto.Continuable != nil ||
		dto.PluginPublicId != nil || dto.PluginExtensionId != nil || dto.PluginData != nil ||
		len(dto.InvolvedRoles) > 0 || dto.ResourceType != ""
	if !hasDomain {
		return nil
	}
	// 领域行主键不在此绑定（共享主键值=核心行 id，由落库口 CreateForTask 覆写）；
	// BaseEntity 显式初始化，主键写入在落库口才发生
	wt := &entity2.WorkTask{BaseEntity: &model.BaseEntity{}}
	if dto.SiteId != nil {
		wt.SiteID.Valid = true
		wt.SiteID.Int64 = *dto.SiteId
	}
	if dto.SiteWorkId != nil {
		wt.SiteWorkID.Valid = true
		wt.SiteWorkID.String = *dto.SiteWorkId
	}
	if dto.Url != nil {
		wt.URL.Valid = true
		wt.URL.String = *dto.Url
	}
	if dto.PendingResourceId != nil {
		wt.PendingResourceID.Valid = true
		wt.PendingResourceID.Int64 = *dto.PendingResourceId
	}
	if dto.Continuable != nil {
		wt.Continuable.Valid = true
		wt.Continuable.Bool = *dto.Continuable
	}
	if dto.PluginPublicId != nil {
		wt.PluginPublicID.Valid = true
		wt.PluginPublicID.String = *dto.PluginPublicId
	}
	if dto.PluginExtensionId != nil {
		wt.PluginExtensionID.Valid = true
		wt.PluginExtensionID.String = *dto.PluginExtensionId
	}
	if dto.PluginData != nil {
		wt.PluginData.Valid = true
		wt.PluginData.String = *dto.PluginData
	}
	// involvedRoles:DTO 切片→逗号分隔;空=不设置(保持 NULL=未确定)
	if len(dto.InvolvedRoles) > 0 {
		wt.InvolvedRoles.Valid = true
		wt.InvolvedRoles.String = strings.Join(dto.InvolvedRoles, ",")
	}
	// resourceType:非空=声明(预定义值);空=不设置(保持 NULL=未声明)
	if dto.ResourceType != "" {
		wt.ResourceType.Valid = true
		wt.ResourceType.String = dto.ResourceType
	}
	return wt
}

// ========== 任务进度相关 DTO ==========

// TaskProgressDTO 任务进度DTO（组合 TaskDTO + 进度/站点名称/进度百分比字段）
type TaskProgressDTO struct {
	Task     *sdkdto.TaskDTO `json:"task,omitempty"`
	Total    *int64          `json:"total,omitempty"`
	Finished *int64          `json:"finished,omitempty"`
	SiteName *string         `json:"siteName,omitempty"`
	Schedule *int            `json:"schedule,omitempty"` // 任务进度百分比（100 表示完成）
}

// NewTaskProgressDTO 从 TaskDTO 创建 TaskProgressDTO
func NewTaskProgressDTO(taskDTO *sdkdto.TaskDTO) *TaskProgressDTO {
	if taskDTO == nil {
		return nil
	}
	return &TaskProgressDTO{
		Task: taskDTO,
	}
}

// TaskProgressTreeDTO 任务进度树DTO（组合 TaskProgressDTO + 树形结构字段）
type TaskProgressTreeDTO struct {
	TaskProgress *TaskProgressDTO       `json:"taskProgress,omitempty"`
	Children     []*TaskProgressTreeDTO `json:"children,omitempty"`
	HasChildren  *bool                  `json:"hasChildren,omitempty"`
	IsLeaf       *bool                  `json:"isLeaf,omitempty"`
}

// NewTaskProgressTreeDTO 从 TaskDTO 创建 TaskProgressTreeDTO
func NewTaskProgressTreeDTO(taskDTO *sdkdto.TaskDTO) *TaskProgressTreeDTO {
	if taskDTO == nil {
		return nil
	}
	hasChildren := taskDTO.HasChild != nil && *taskDTO.HasChild
	return &TaskProgressTreeDTO{
		TaskProgress: NewTaskProgressDTO(taskDTO),
		Children:     make([]*TaskProgressTreeDTO, 0),
		HasChildren:  &hasChildren,
		IsLeaf:       new(!hasChildren),
	}
}

// ========== 任务创建/树数据请求 DTO ==========

// CreateTaskRequest 创建任务请求
type CreateTaskRequest struct {
	Pid               int64  `json:"pid"`
	TaskName          string `json:"taskName"`
	SiteID            int    `json:"siteId"`
	SiteWorkID        string `json:"siteWorkId"`
	URL               string `json:"url"`
	HasChild          bool   `json:"hasChild"`
	PluginPublicID    string `json:"pluginPublicId"`
	PluginExtensionID string `json:"pluginExtensionId"`
	PluginData        string `json:"pluginData"`
}

// TreeDataPageDTO 任务树数据分页DTO
type TreeDataPageDTO struct {
	TreeID   int64                  `json:"treeId"`
	TreeName string                 `json:"treeName"`
	Total    int64                  `json:"total"`
	Tasks    []*TaskProgressTreeDTO `json:"tasks"`
}
