package extension

import (
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// EntityTaskToSDK 将 entity.Task 转换为 sdkdto.TaskDTO
func EntityTaskToSDK(task *entity.Task) *sdkdto.TaskDTO {
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
	t.SiteId = util.NullInt64ToPointer(task.SiteID)
	t.SiteWorkId = util.NullStringToPointer(task.SiteWorkID)
	t.Url = util.NullStringToPointer(task.URL)
	t.PendingResourceId = util.NullInt64ToPointer(task.PendingResourceID)
	t.Continuable = util.NullBoolToPointer(task.Continuable)
	t.PluginPublicId = util.NullStringToPointer(task.PluginPublicID)
	t.PluginExtensionId = util.NullStringToPointer(task.PluginExtensionID)
	t.PluginData = util.NullStringToPointer(task.PluginData)
	t.ErrorMessage = util.NullStringToPointer(task.ErrorMessage)
	return t
}
