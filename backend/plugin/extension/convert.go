package extension

import (
	"database/sql"
	"io"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
)

// --- Entity → SDK 转换 ---

// EntityTaskToSDK 将 entity.Task 转换为 pluginsdk.Task
func EntityTaskToSDK(task *entity.Task) *pluginsdk.Task {
	if task == nil {
		return nil
	}
	t := &pluginsdk.Task{
		Status: task.Status,
	}
	if task.BaseEntity != nil {
		t.ID = task.GetID()
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
	t.SiteID = util.NullInt64ToPointer(task.SiteID)
	t.SiteWorkID = util.NullStringToPointer(task.SiteWorkID)
	t.URL = util.NullStringToPointer(task.URL)
	t.PendingResourceID = util.NullInt64ToPointer(task.PendingResourceID)
	t.Continuable = util.NullBoolToPointer(task.Continuable)
	t.PluginPublicID = util.NullStringToPointer(task.PluginPublicID)
	t.PluginContributionID = util.NullStringToPointer(task.PluginContributionID)
	t.PluginData = util.NullStringToPointer(task.PluginData)
	t.ErrorMessage = util.NullStringToPointer(task.ErrorMessage)
	return t
}

// EntityWorkSetToSDK 将 entity.WorkSet 转换为 pluginsdk.WorkSet
func EntityWorkSetToSDK(ws *entity.WorkSet) *pluginsdk.WorkSet {
	if ws == nil {
		return nil
	}
	s := &pluginsdk.WorkSet{}
	if ws.BaseEntity != nil {
		s.ID = ws.GetID()
		s.CreateTime = ws.GetCreateTime()
		s.UpdateTime = ws.GetUpdateTime()
	}
	s.SiteID = util.NullInt64ToPointer(ws.SiteID)
	s.SiteWorkSetID = util.NullStringToPointer(ws.SiteWorkSetID)
	s.SiteWorkSetName = util.NullStringToPointer(ws.SiteWorkSetName)
	s.SiteAuthorID = util.NullStringToPointer(ws.SiteAuthorID)
	s.SiteWorkSetDescription = util.NullStringToPointer(ws.SiteWorkSetDescription)
	s.SiteUploadTime = util.NullInt64ToPointer(ws.SiteUploadTime)
	s.SiteUpdateTime = util.NullInt64ToPointer(ws.SiteUpdateTime)
	s.NickName = util.NullStringToPointer(ws.NickName)
	s.LastView = util.NullInt64ToPointer(ws.LastView)
	return s
}

// SDKSiteToEntity 将 pluginsdk.Site 转换为 entity.Site
func SDKSiteToEntity(s *pluginsdk.Site) *entity.Site {
	if s == nil {
		return nil
	}
	e := entity.NewSite()
	if s.ID != 0 {
		e.SetID(s.ID)
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

// --- SDK → DTO 转换 ---

// SDKWorkResponseToDTO 将 pluginsdk.WorkResponse 转换为 dto.WorkResponse
func SDKWorkResponseToDTO(resp *pluginsdk.WorkResponse) *dto.WorkResponse {
	if resp == nil {
		return nil
	}
	return &dto.WorkResponse{
		Work:         SDKWorkToEntity(resp.Work),
		Site:         SDKSiteDTOToDTO(resp.Site),
		LocalAuthors: SDKLocalAuthorsToDTO(resp.LocalAuthors),
		LocalTags:    SDKLocalTagsToDTO(resp.LocalTags),
		SiteAuthors:  SDKSiteAuthorsToDTO(resp.SiteAuthors),
		SiteTags:     SDKSiteTagsToDTO(resp.SiteTags),
		WorkSets:     SDKWorkSetsToDTO(resp.WorkSets),
		Resource:     SDKResourceToDTO(resp.Resource),
	}
}

// SDKTaskResParamToDTO 将 pluginsdk.TaskResParam 转换为 dto.TaskResParam
func SDKTaskResParamToDTO(param *pluginsdk.TaskResParam) *dto.TaskResParam {
	if param == nil {
		return nil
	}
	return &dto.TaskResParam{
		Task:         SDKTaskToEntity(param.Task),
		ResourceID:   param.ResourceID,
		ResourcePath: param.ResourcePath,
	}
}

// SDKTaskCreateResponsesToDTO 批量转换 pluginsdk.TaskCreateResponse → dto.TaskCreateResponse
func SDKTaskCreateResponsesToDTO(resps []*pluginsdk.TaskCreateResponse) []*dto.TaskCreateResponse {
	if resps == nil {
		return nil
	}
	result := make([]*dto.TaskCreateResponse, len(resps))
	for i, r := range resps {
		result[i] = SDKTaskCreateResponseToDTO(r)
	}
	return result
}

// --- 内部转换辅助函数 ---

func SDKTaskToEntity(t *pluginsdk.Task) *entity.Task {
	if t == nil {
		return nil
	}
	e := entity.NewTask()
	if t.ID != 0 {
		e.SetID(t.ID)
	}
	if t.CreateTime != 0 {
		e.SetCreateTime(t.CreateTime)
	}
	if t.UpdateTime != 0 {
		e.SetUpdateTime(t.UpdateTime)
	}
	e.Status = t.Status
	if t.HasChild != nil {
		e.HasChild = sql.NullBool{Bool: *t.HasChild, Valid: true}
	}
	if t.Pid != nil {
		e.Pid = sql.NullInt64{Int64: *t.Pid, Valid: true}
	}
	if t.TaskName != nil {
		e.TaskName = sql.NullString{String: *t.TaskName, Valid: true}
	}
	if t.SiteID != nil {
		e.SiteID = sql.NullInt64{Int64: *t.SiteID, Valid: true}
	}
	if t.SiteWorkID != nil {
		e.SiteWorkID = sql.NullString{String: *t.SiteWorkID, Valid: true}
	}
	if t.URL != nil {
		e.URL = sql.NullString{String: *t.URL, Valid: true}
	}
	if t.PendingResourceID != nil {
		e.PendingResourceID = sql.NullInt64{Int64: *t.PendingResourceID, Valid: true}
	}
	if t.Continuable != nil {
		e.Continuable = sql.NullBool{Bool: *t.Continuable, Valid: true}
	}
	if t.PluginPublicID != nil {
		e.PluginPublicID = sql.NullString{String: *t.PluginPublicID, Valid: true}
	}
	if t.PluginContributionID != nil {
		e.PluginContributionID = sql.NullString{String: *t.PluginContributionID, Valid: true}
	}
	if t.PluginData != nil {
		e.PluginData = sql.NullString{String: *t.PluginData, Valid: true}
	}
	if t.ErrorMessage != nil {
		e.ErrorMessage = sql.NullString{String: *t.ErrorMessage, Valid: true}
	}
	return e
}

func SDKWorkToEntity(w *pluginsdk.Work) *entity.Work {
	if w == nil {
		return nil
	}
	e := entity.NewWork()
	if w.ID != 0 {
		e.SetID(w.ID)
	}
	if w.CreateTime != 0 {
		e.SetCreateTime(w.CreateTime)
	}
	if w.UpdateTime != 0 {
		e.SetUpdateTime(w.UpdateTime)
	}
	if w.SiteID != nil {
		e.SiteID = sql.NullInt64{Int64: *w.SiteID, Valid: true}
	}
	if w.SiteWorkID != nil {
		e.SiteWorkID = sql.NullString{String: *w.SiteWorkID, Valid: true}
	}
	if w.SiteWorkName != nil {
		e.SiteWorkName = sql.NullString{String: *w.SiteWorkName, Valid: true}
	}
	if w.SiteAuthorID != nil {
		e.SiteAuthorID = sql.NullString{String: *w.SiteAuthorID, Valid: true}
	}
	if w.SiteWorkDescription != nil {
		e.SiteWorkDescription = sql.NullString{String: *w.SiteWorkDescription, Valid: true}
	}
	if w.SiteUploadTime != nil {
		e.SiteUploadTime = sql.NullInt64{Int64: *w.SiteUploadTime, Valid: true}
	}
	if w.SiteUpdateTime != nil {
		e.SiteUpdateTime = sql.NullInt64{Int64: *w.SiteUpdateTime, Valid: true}
	}
	if w.NickName != nil {
		e.NickName = sql.NullString{String: *w.NickName, Valid: true}
	}
	if w.LocalAuthorID != nil {
		e.LocalAuthorID = sql.NullInt64{Int64: *w.LocalAuthorID, Valid: true}
	}
	if w.LastView != nil {
		e.LastView = sql.NullInt64{Int64: *w.LastView, Valid: true}
	}
	return e
}

func SDKSiteDTOToDTO(s *pluginsdk.SiteDTO) *dto.SiteDTO {
	if s == nil {
		return nil
	}
	return &dto.SiteDTO{
		ID:              s.ID,
		SiteName:        s.SiteName,
		SiteDescription: s.SiteDescription,
		Homepage:        s.Homepage,
		CreateTime:      s.CreateTime,
		UpdateTime:      s.UpdateTime,
	}
}

func SDKLocalAuthorsToDTO(authors []*pluginsdk.LocalAuthorDTO) []*dto.LocalAuthorDTO {
	if authors == nil {
		return nil
	}
	result := make([]*dto.LocalAuthorDTO, len(authors))
	for i, a := range authors {
		if a == nil {
			continue
		}
		result[i] = &dto.LocalAuthorDTO{
			ID:         a.ID,
			AuthorName: a.AuthorName,
			Introduce:  a.Introduce,
			LastUse:    a.LastUse,
			CreateTime: a.CreateTime,
			UpdateTime: a.UpdateTime,
		}
	}
	return result
}

func SDKLocalTagsToDTO(tags []*pluginsdk.LocalTagDTO) []*dto.LocalTagDTO {
	if tags == nil {
		return nil
	}
	result := make([]*dto.LocalTagDTO, len(tags))
	for i, t := range tags {
		if t == nil {
			continue
		}
		result[i] = &dto.LocalTagDTO{
			ID:             t.ID,
			LocalTagName:   t.LocalTagName,
			BaseLocalTagID: t.BaseLocalTagID,
			Description:    t.Description,
			LastUse:        t.LastUse,
			CreateTime:     t.CreateTime,
			UpdateTime:     t.UpdateTime,
		}
	}
	return result
}

func SDKSiteAuthorsToDTO(authors []*pluginsdk.TaskSiteAuthorDTO) []*dto.TaskSiteAuthorDTO {
	if authors == nil {
		return nil
	}
	result := make([]*dto.TaskSiteAuthorDTO, len(authors))
	for i, a := range authors {
		if a == nil {
			continue
		}
		result[i] = &dto.TaskSiteAuthorDTO{
			SiteAuthorID:    a.SiteAuthorID,
			AuthorName:      a.AuthorName,
			Homepage:        a.Homepage,
			FixedAuthorName: a.FixedAuthorName,
			Introduce:       a.Introduce,
		}
	}
	return result
}

func SDKSiteTagsToDTO(tags []*pluginsdk.TaskSiteTagDTO) []*dto.TaskSiteTagDTO {
	if tags == nil {
		return nil
	}
	result := make([]*dto.TaskSiteTagDTO, len(tags))
	for i, t := range tags {
		if t == nil {
			continue
		}
		result[i] = &dto.TaskSiteTagDTO{
			SiteTagID:   t.SiteTagID,
			TagName:     t.TagName,
			Description: t.Description,
		}
	}
	return result
}

func SDKWorkSetsToDTO(sets []*pluginsdk.TaskWorkSetDTO) []*dto.TaskWorkSetDTO {
	if sets == nil {
		return nil
	}
	result := make([]*dto.TaskWorkSetDTO, len(sets))
	for i, s := range sets {
		if s == nil {
			continue
		}
		result[i] = &dto.TaskWorkSetDTO{
			SiteWorkSetID: s.SiteWorkSetID,
			WorkSetName: s.WorkSetName,
		}
	}
	return result
}

func SDKResourceToDTO(r *pluginsdk.TaskResourceDTO) *dto.TaskResourceDTO {
	if r == nil {
		return nil
	}
	return &dto.TaskResourceDTO{
		ResourceID:   r.ResourceID,
		URL:          r.URL,
		Type:         r.Type,
		Format:       r.Format,
		LocalPath:    r.LocalPath,
		RemotePath:   r.RemotePath,
		Size:         r.Size,
		Completeness: r.Completeness,
	}
}

func SDKTaskCreateResponseToDTO(r *pluginsdk.TaskCreateResponse) *dto.TaskCreateResponse {
	if r == nil {
		return nil
	}
	children := make([]*dto.TaskCreateChildResponse, len(r.Children))
	for i, c := range r.Children {
		if c == nil {
			continue
		}
		children[i] = &dto.TaskCreateChildResponse{
			TaskName:   c.TaskName,
			SiteWorkID: c.SiteWorkID,
			URL:        c.URL,
			PluginData: c.PluginData,
			SiteName:   c.SiteName,
		}
	}
	return &dto.TaskCreateResponse{
		PluginTaskID: r.PluginTaskID,
		TaskName:     r.TaskName,
		SiteWorkID:   r.SiteWorkID,
		URL:          r.URL,
		PluginData:   r.PluginData,
		SiteName:     r.SiteName,
		Children:     children,
	}
}

// --- DTO → SDK 转换 (用于 TaskResParam) ---

// DTOTaskResParamToSDK 将 dto.TaskResParam 转换为 pluginsdk.TaskResParam
func DTOTaskResParamToSDK(param *dto.TaskResParam) *pluginsdk.TaskResParam {
	if param == nil {
		return nil
	}
	return &pluginsdk.TaskResParam{
		Task:         EntityTaskToSDK(param.Task),
		ResourceID:   param.ResourceID,
		ResourcePath: param.ResourcePath,
	}
}

// --- taskHandlerAdapter ---
// 将 pluginsdk.TaskHandler 适配为 dto.TaskHandler

type taskHandlerAdapter struct {
	handler pluginsdk.TaskHandler
}

func (a *taskHandlerAdapter) Create(url string) (*dto.TaskCreateResult, error) {
	result, err := a.handler.Create(url)
	if err != nil {
		return nil, err
	}

	if result.IsStream() {
		// 流式模式：转换 channel 中的每个 response
		sdkCh := result.Stream()
		dtoCh := make(chan *dto.TaskCreateResponse, 16)
		go func() {
			defer close(dtoCh)
			for resp := range sdkCh {
				dtoCh <- SDKTaskCreateResponseToDTO(resp)
			}
		}()
		return dto.StreamResult(dtoCh), nil
	}

	// 批量模式
	return dto.BatchResult(SDKTaskCreateResponsesToDTO(result.Array())), nil
}

func (a *taskHandlerAdapter) CreateWorkInfo(task *entity.Task) (*dto.WorkResponse, error) {
	resp, err := a.handler.CreateWorkInfo(EntityTaskToSDK(task))
	if err != nil {
		return nil, err
	}
	return SDKWorkResponseToDTO(resp), nil
}

func (a *taskHandlerAdapter) Start(task *entity.Task) (io.ReadCloser, *dto.WorkResponse, error) {
	reader, resp, err := a.handler.Start(EntityTaskToSDK(task))
	if err != nil {
		return nil, nil, err
	}
	return reader, SDKWorkResponseToDTO(resp), nil
}

func (a *taskHandlerAdapter) Retry(task *entity.Task) (*dto.WorkResponse, error) {
	resp, err := a.handler.Retry(EntityTaskToSDK(task))
	if err != nil {
		return nil, err
	}
	return SDKWorkResponseToDTO(resp), nil
}

func (a *taskHandlerAdapter) Pause(param *dto.TaskResParam) error {
	return a.handler.Pause(DTOTaskResParamToSDK(param))
}

func (a *taskHandlerAdapter) Stop(param *dto.TaskResParam) error {
	return a.handler.Stop(DTOTaskResParamToSDK(param))
}

func (a *taskHandlerAdapter) Resume(param *dto.TaskResParam) (*dto.WorkResponse, error) {
	resp, err := a.handler.Resume(DTOTaskResParamToSDK(param))
	if err != nil {
		return nil, err
	}
	return SDKWorkResponseToDTO(resp), nil
}
