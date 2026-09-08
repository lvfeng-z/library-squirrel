package extension

import (
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// EntityTaskToSDK 将任务核心行+作品任务领域行组装为 sdkdto.TaskDTO（跨进程序列化契约，
// 字段集不随表拆分变化：控制字段取 task，领域字段取 workTask）
func EntityTaskToSDK(task *entity.Task, workTask *entity.WorkTask) *sdkdto.TaskDTO {
	if task == nil {
		return nil
	}
	t := &sdkdto.TaskDTO{
		Status: int32(task.Status),
	}
	if task.BaseEntity != nil {
		t.Id = task.GetID()
		t.CreateTime = task.GetCreateTime()
		t.UpdateTime = task.GetUpdateTime()
	}
	if task.HasChild.Valid {
		t.HasChild = &task.HasChild.Bool
	}
	if task.Pid.Valid {
		t.Pid = &task.Pid.Int64
	}
	t.TaskName = util.NullStringToPointer(task.TaskName)
	t.ErrorMessage = util.NullStringToPointer(task.ErrorMessage)
	if workTask == nil {
		return t
	}
	t.SiteId = util.NullInt64ToPointer(workTask.SiteID)
	t.SiteWorkId = util.NullStringToPointer(workTask.SiteWorkID)
	t.Url = util.NullStringToPointer(workTask.URL)
	t.Continuable = util.NullBoolToPointer(workTask.Continuable)
	t.PluginPublicId = util.NullStringToPointer(workTask.PluginPublicID)
	t.PluginExtensionId = util.NullStringToPointer(workTask.PluginExtensionID)
	t.PluginData = util.NullStringToPointer(workTask.PluginData)
	return t
}
