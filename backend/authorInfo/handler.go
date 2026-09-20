package authorInfo

// 作者个人信息 Handler：site 侧手动拉取入口（单条/批量）。local 侧导入入口
// （SetLocalAuthorAvatar/RemoveLocalAuthorAvatar）属删除联动+local 导入阶段。

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
