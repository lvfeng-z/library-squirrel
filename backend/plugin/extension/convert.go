package extension

import (
	"database/sql"

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

// EntityWorkSetToSDK 将 entity.WorkSet 转换为 sdkdto.WorkSetDTO
func EntityWorkSetToSDK(ws *entity.WorkSet) *sdkdto.WorkSetDTO {
	if ws == nil {
		return nil
	}
	s := &sdkdto.WorkSetDTO{}
	if ws.BaseEntity != nil {
		s.Id = ws.GetID()
		s.CreateTime = ws.GetCreateTime()
		s.UpdateTime = ws.GetUpdateTime()
	}
	s.SiteId = util.NullInt64ToPointer(ws.SiteID)
	s.SiteWorkSetId = util.NullStringToPointer(ws.SiteWorkSetID)
	s.SiteWorkSetName = util.NullStringToPointer(ws.SiteWorkSetName)
	s.SiteAuthorId = util.NullStringToPointer(ws.SiteAuthorID)
	s.SiteWorkSetDescription = util.NullStringToPointer(ws.SiteWorkSetDescription)
	s.SiteUploadTime = util.NullInt64ToPointer(ws.SiteUploadTime)
	s.SiteUpdateTime = util.NullInt64ToPointer(ws.SiteUpdateTime)
	s.NickName = util.NullStringToPointer(ws.NickName)
	s.LastView = util.NullInt64ToPointer(ws.LastView)
	return s
}

// SDKSiteToEntity 将 sdkdto.SiteDTO 转换为 entity.Site
func SDKSiteToEntity(s *sdkdto.SiteDTO) *entity.Site {
	if s == nil {
		return nil
	}
	e := entity.NewSite()
	if s.Id != 0 {
		e.SetID(s.Id)
	}
	if s.CreateTime != 0 {
		e.SetCreateTime(s.CreateTime)
	}
	if s.UpdateTime != 0 {
		e.SetUpdateTime(s.UpdateTime)
	}
	if s.SiteName != nil {
		e.SiteName = sql.NullString{String: *s.SiteName, Valid: true}
	}
	if s.SiteDescription != nil {
		e.SiteDescription = sql.NullString{String: *s.SiteDescription, Valid: true}
	}
	if s.Homepage != nil {
		e.Homepage = sql.NullString{String: *s.Homepage, Valid: true}
	}
	return e
}
