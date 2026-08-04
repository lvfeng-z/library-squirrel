package dto

import (
	"strings"

	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// NewTaskDTO 从 entity.Task 创建 TaskDTO
func NewTaskDTO(task *entity2.Task) *sdkdto.TaskDTO {
	if task == nil {
		return nil
	}
	// involvedRoles:创建期声明的涉及板块(universe),逗号分隔→切片;NULL/空=nil(前端走兜底集)
	var involvedRoles []string
	if task.InvolvedRoles.Valid && task.InvolvedRoles.String != "" {
		for _, p := range strings.Split(task.InvolvedRoles.String, ",") {
			if r := strings.TrimSpace(p); r != "" {
				involvedRoles = append(involvedRoles, r)
			}
		}
	}
	return &sdkdto.TaskDTO{
		ID:                task.GetID(),
		HasChild:          util.NullBoolToPointer(task.HasChild),
		Pid:               util.NullInt64ToPointer(task.Pid),
		TaskName:          util.NullStringToPointer(task.TaskName),
		SiteID:            util.NullInt64ToPointer(task.SiteID),
		SiteWorkID:        util.NullStringToPointer(task.SiteWorkID),
		URL:               util.NullStringToPointer(task.URL),
		Status:            task.Status,
		PendingResourceID: util.NullInt64ToPointer(task.PendingResourceID),
		Continuable:       util.NullBoolToPointer(task.Continuable),
		PluginPublicID:    util.NullStringToPointer(task.PluginPublicID),
		PluginExtensionID: util.NullStringToPointer(task.PluginExtensionID),
		PluginData:        util.NullStringToPointer(task.PluginData),
		ErrorMessage:      util.NullStringToPointer(task.ErrorMessage),
		InvolvedRoles:     involvedRoles,
		ResourceType:      task.ResourceType.String, // NULL/未声明=零值 ""
		CreateTime:        task.GetCreateTime(),
		UpdateTime:        task.GetUpdateTime(),
	}
}

// ToTaskEntity 将 TaskDTO 转换为 Task 实体
func ToTaskEntity(dto *sdkdto.TaskDTO) *entity2.Task {
	if dto == nil {
		return nil
	}

	entity := entity2.NewTask()

	// 设置基础字段
	if dto.ID != 0 {
		entity.SetID(dto.ID)
	}

	// 设置业务字段
	if dto.HasChild != nil {
		entity.HasChild.Valid = true
		entity.HasChild.Bool = *dto.HasChild
	} else {
		entity.HasChild.Valid = false
	}

	if dto.Pid != nil {
		entity.Pid.Valid = true
		entity.Pid.Int64 = *dto.Pid
	} else {
		entity.Pid.Valid = false
	}

	if dto.TaskName != nil {
		entity.TaskName.Valid = true
		entity.TaskName.String = *dto.TaskName
	} else {
		entity.TaskName.Valid = false
	}

	if dto.SiteID != nil {
		entity.SiteID.Valid = true
		entity.SiteID.Int64 = *dto.SiteID
	} else {
		entity.SiteID.Valid = false
	}

	if dto.SiteWorkID != nil {
		entity.SiteWorkID.Valid = true
		entity.SiteWorkID.String = *dto.SiteWorkID
	} else {
		entity.SiteWorkID.Valid = false
	}

	if dto.URL != nil {
		entity.URL.Valid = true
		entity.URL.String = *dto.URL
	} else {
		entity.URL.Valid = false
	}

	entity.Status = dto.Status

	if dto.PendingResourceID != nil {
		entity.PendingResourceID.Valid = true
		entity.PendingResourceID.Int64 = *dto.PendingResourceID
	} else {
		entity.PendingResourceID.Valid = false
	}

	if dto.Continuable != nil {
		entity.Continuable.Valid = true
		entity.Continuable.Bool = *dto.Continuable
	} else {
		entity.Continuable.Valid = false
	}

	if dto.PluginPublicID != nil {
		entity.PluginPublicID.Valid = true
		entity.PluginPublicID.String = *dto.PluginPublicID
	} else {
		entity.PluginPublicID.Valid = false
	}

	if dto.PluginExtensionID != nil {
		entity.PluginExtensionID.Valid = true
		entity.PluginExtensionID.String = *dto.PluginExtensionID
	} else {
		entity.PluginExtensionID.Valid = false
	}

	if dto.PluginData != nil {
		entity.PluginData.Valid = true
		entity.PluginData.String = *dto.PluginData
	} else {
		entity.PluginData.Valid = false
	}

	if dto.ErrorMessage != nil {
		entity.ErrorMessage.Valid = true
		entity.ErrorMessage.String = *dto.ErrorMessage
	} else {
		entity.ErrorMessage.Valid = false
	}

	// involvedRoles:DTO 切片→逗号分隔;空=不设置(保持 NULL=未确定)
	if len(dto.InvolvedRoles) > 0 {
		entity.InvolvedRoles.Valid = true
		entity.InvolvedRoles.String = strings.Join(dto.InvolvedRoles, ",")
	}

	// resourceType:非空=声明(预定义值);空=不设置(保持 NULL=未声明)
	if dto.ResourceType != "" {
		entity.ResourceType.Valid = true
		entity.ResourceType.String = dto.ResourceType
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
	TreeID   int64                         `json:"treeId"`
	TreeName string                        `json:"treeName"`
	Total    int64                         `json:"total"`
	Tasks    []*TaskProgressTreeDTO `json:"tasks"`
}
