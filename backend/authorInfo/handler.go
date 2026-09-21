package authorInfo

// 作者个人信息 Handler：site 侧手动拉取入口（单条/批量）与 local 侧头像导入/移除入口。

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
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

// conflictResponse 候选冲突响应：Success=false + 引导文案，候选清单经数据载荷交回前端（按站点
// 分组，同一站点恒一组）（与任务创建面的冲突响应同形：冲突不是失败，但按失败响应下发以复用前端提示面）
func conflictResponse(resp *SiteAuthorFetchResponse) *model.ApiResponse[*SiteAuthorFetchResponse] {
	return &model.ApiResponse[*SiteAuthorFetchResponse]{
		Success: false,
		Msg:     siteAuthorFetchConflictMsg,
		Data:    resp,
	}
}

// FetchSiteAuthorInfo 手动拉取单个站点作者信息（前端行操作，loading 态由前端按调用挂起）。
// chosenPlugins 为交互面显选（站点键 → 插件，空=未显选）：该站点候选多于一个且未显选时返回
// 冲突载荷且未调用任何插件，由前端选择后带显选重发
func (h *Handler) FetchSiteAuthorInfo(ctx context.Context, siteAuthorId int64,
	chosenPlugins []*dto.SiteAuthorFetchChoice) *model.ApiResponse[*SiteAuthorFetchResponse] {
	resp, err := h.svc.FetchSiteAuthorInfoById(ctx, siteAuthorId, chosenPlugins)
	if err != nil {
		return model.HandleError[*SiteAuthorFetchResponse](err)
	}
	if len(resp.Conflicts) > 0 {
		return conflictResponse(resp)
	}
	return model.Success(resp)
}

// FetchSiteAuthorsInfo 手动批量拉取站点作者信息（逐作者串行）；候选冲突按站点分组整批前置返回
// （同一站点只交回一组，一次触发问一次），否则数据载荷的 items 为与入参顺序一致的逐条清单
func (h *Handler) FetchSiteAuthorsInfo(ctx context.Context, siteAuthorIds []int64,
	chosenPlugins []*dto.SiteAuthorFetchChoice) *model.ApiResponse[*SiteAuthorFetchResponse] {
	resp, err := h.svc.FetchSiteAuthorsInfoByIds(ctx, siteAuthorIds, chosenPlugins)
	if err != nil {
		return model.HandleError[*SiteAuthorFetchResponse](err)
	}
	if len(resp.Conflicts) > 0 {
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
