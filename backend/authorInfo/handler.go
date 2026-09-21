package authorInfo

// 作者个人信息 Handler：site 侧手动拉取入口（单条/批量）与 local 侧头像导入/移除入口。

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
)

// siteAuthorFetchConflictMsg 候选冲突的引导文案（前端据此提示用户选择插件后带显选键重发）
const siteAuthorFetchConflictMsg = "该作者信息可由多个插件拉取，请选择插件"

// Handler 作者个人信息 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建作者个人信息 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// conflictResponse 候选冲突响应：Success=false + 引导文案，候选清单经数据载荷交回前端
// （与任务创建面的冲突响应同形：冲突不是失败，但按失败响应下发以复用前端提示面）
func conflictResponse(resp *SiteAuthorFetchResponse) *model.ApiResponse[*SiteAuthorFetchResponse] {
	return &model.ApiResponse[*SiteAuthorFetchResponse]{
		Success: false,
		Msg:     siteAuthorFetchConflictMsg,
		Data:    resp,
	}
}

// FetchSiteAuthorInfo 手动拉取单个站点作者信息（前端行操作，loading 态由前端按调用挂起）。
// chosenPluginPublicId 为交互面显选键（空=未显选）：候选多于一个且未显选时返回冲突载荷且
// 未调用任何插件，由前端选择后带键重发
func (h *Handler) FetchSiteAuthorInfo(ctx context.Context, siteAuthorId int64, chosenPluginPublicId string) *model.ApiResponse[*SiteAuthorFetchResponse] {
	resp, err := h.svc.FetchSiteAuthorInfoById(ctx, siteAuthorId, chosenPluginPublicId)
	if err != nil {
		return model.HandleError[*SiteAuthorFetchResponse](err)
	}
	if resp.Conflict != nil {
		return conflictResponse(resp)
	}
	return model.Success(resp)
}

// FetchSiteAuthorsInfo 手动批量拉取站点作者信息（逐作者串行）；候选冲突整批前置返回冲突载荷
// （一次触发问一次），否则数据载荷的 items 为与入参顺序一致的逐条清单
func (h *Handler) FetchSiteAuthorsInfo(ctx context.Context, siteAuthorIds []int64, chosenPluginPublicId string) *model.ApiResponse[*SiteAuthorFetchResponse] {
	resp, err := h.svc.FetchSiteAuthorsInfoByIds(ctx, siteAuthorIds, chosenPluginPublicId)
	if err != nil {
		return model.HandleError[*SiteAuthorFetchResponse](err)
	}
	if resp.Conflict != nil {
		return conflictResponse(resp)
	}
	return model.Success(resp)
}

// SetLocalAuthorAvatar 为本地作者设置头像（源文件为前端文件对话框选取的绝对路径；换头像形态
// 先删旧，失败不伤现有头像）
func (h *Handler) SetLocalAuthorAvatar(ctx context.Context, localAuthorId int64, sourceAbsPath string) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.SetLocalAuthorAvatar(ctx, localAuthorId, sourceAbsPath))
}

// RemoveLocalAuthorAvatar 移除本地作者头像（显式破坏操作，前端二次确认）
func (h *Handler) RemoveLocalAuthorAvatar(ctx context.Context, localAuthorId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.RemoveLocalAuthorAvatar(ctx, localAuthorId))
}
