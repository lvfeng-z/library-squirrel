package authorInfo

// 作者个人信息 Handler：site 侧手动拉取入口（单条/批量）与 local 侧头像导入/移除入口。

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
)

// Handler 作者个人信息 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建作者个人信息 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// FetchSiteAuthorInfo 手动拉取单个站点作者信息（前端行操作，loading 态由前端按调用挂起）
func (h *Handler) FetchSiteAuthorInfo(ctx context.Context, siteAuthorId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.FetchSiteAuthorInfoById(ctx, siteAuthorId))
}

// FetchSiteAuthorsInfo 手动批量拉取站点作者信息（逐作者串行，返回与入参顺序一致的逐条结果清单）
func (h *Handler) FetchSiteAuthorsInfo(ctx context.Context, siteAuthorIds []int64) *model.ApiResponse[[]*SiteAuthorFetchItemResult] {
	results, err := h.svc.FetchSiteAuthorsInfoByIds(ctx, siteAuthorIds)
	if err != nil {
		return model.HandleError[[]*SiteAuthorFetchItemResult](err)
	}
	return model.Success(results)
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
