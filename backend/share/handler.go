package share

// Handler 分享 Handler（Wails Bind 方法，经 IPC 暴露给前端）。

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
)

// Handler 分享 Handler（Wails Bind 方法，经 IPC 暴露给前端）
type Handler struct {
	svc *Service
}

// NewHandler 创建分享 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// SharePublish 启动分享发布（异步：立即返回 shareID，进度/完成/会话状态经 share-events 推送）。
// workIDs/workSetIds 为前端选中 id 列表（与导出同形态）；options 见 SharePublishOptions。
func (h *Handler) SharePublish(ctx context.Context, workIDs []int64, workSetIDs []int64, options SharePublishOptions) *model.ApiResponse[string] {
	id, err := h.svc.Publish(ctx, workIDs, workSetIDs, options)
	if err != nil {
		return model.HandleError[string](err)
	}
	return model.Success(id)
}

// ShareCancelPublish 取消进行中的发布（无进行中发布则 no-op）
func (h *Handler) ShareCancelPublish(ctx context.Context, shareID string) *model.ApiResponse[any] {
	h.svc.CancelPublish(shareID)
	return model.Success[any](nil)
}

// ShareRevoke 撤销分享会话（在线即在中继即时生效，后续拨号被拒）
func (h *Handler) ShareRevoke(ctx context.Context, shareID string) *model.ApiResponse[any] {
	if err := h.svc.Revoke(ctx, shareID); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// ShareSessions 查询全部分享会话快照（含终态）
func (h *Handler) ShareSessions(ctx context.Context) *model.ApiResponse[[]*ShareSessionDTO] {
	return model.Success(h.svc.Sessions(ctx))
}
