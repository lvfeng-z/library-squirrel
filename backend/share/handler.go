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

// SharePublish 发布分享（直跑，不经任务模块；异步：立即返回 shareID，进度/完成/会话状态
// 经 share-events 推送）。workIDs/workSetIds 为前端选中 id 列表（与导出同形态）；options 见
// SharePublishOptions。
func (h *Handler) SharePublish(ctx context.Context, workIDs []int64, workSetIDs []int64, options SharePublishOptions) *model.ApiResponse[string] {
	id, err := h.svc.Publish(ctx, workIDs, workSetIDs, options)
	if err != nil {
		return model.HandleError[string](err)
	}
	return model.Success(id)
}

// ShareCancelPublish 取消分享发布（发布弹窗「取消」，直接终止会话主体；已在线会话的撤销走 ShareRevoke）
func (h *Handler) ShareCancelPublish(ctx context.Context, shareID string) *model.ApiResponse[any] {
	h.svc.CancelPublish(ctx, shareID)
	return model.Success[any](nil)
}

// ShareRevoke 撤销分享会话（在线即在中继即时生效，后续拨号被拒；离线 active 记录行本地落 revoked）
func (h *Handler) ShareRevoke(ctx context.Context, shareID string) *model.ApiResponse[any] {
	if err := h.svc.Revoke(ctx, shareID); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// ShareSessions 查询全部分享会话快照（含终态；运行态实时源，按 share_id 与分享记录关联展示）
func (h *Handler) ShareSessions(ctx context.Context) *model.ApiResponse[[]*ShareSessionDTO] {
	return model.Success(h.svc.Sessions(ctx))
}

// ShareRecords 查询全部分享记录（历史分享账本：状态/链接重建要素/统计，create_time 倒序）
func (h *Handler) ShareRecords(ctx context.Context) *model.ApiResponse[[]*ShareRecordDTO] {
	records, err := h.svc.Records(ctx)
	if err != nil {
		return model.HandleError[[]*ShareRecordDTO](err)
	}
	return model.Success(records)
}

// ShareDeleteRecord 删除分享记录（物理删行；在驻会话先撤销——活跃分享删除即链接失效）
func (h *Handler) ShareDeleteRecord(ctx context.Context, shareID string) *model.ApiResponse[any] {
	if err := h.svc.DeleteRecord(ctx, shareID); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// ShareReceiveResult 收件拉取建树结果（决策4 之①：作品名列表供收件侧创建反馈展示）
type ShareReceiveResult struct {
	ParentTaskID int64    `json:"parentTaskId"` // 父任务 ID（进度/终态由任务面板树形承载）
	WorkCount    int      `json:"workCount"`    // 分享作品数（子任务数）
	WorkNames    []string `json:"workNames"`    // 作品名列表（净化后；与子任务命名一致）
}

// ShareReceive 启动收件拉取：解析分享链接（深链或 https 分享链接，可含访问密码）→ 同步预拉
// manifest → 建父子任务树（父容器 + 每作品一子任务）→ 共享 manifest 落盘 → 整树启动。
// 返回 {parentTaskId, workCount, workNames}（进度/终态由任务面板承载）。
func (h *Handler) ShareReceive(ctx context.Context, link string, password string) *model.ApiResponse[*ShareReceiveResult] {
	res, err := h.svc.Receive(ctx, link, password)
	if err != nil {
		return model.HandleError[*ShareReceiveResult](err)
	}
	return model.Success(res)
}

// ShareConsumePendingLink 取走深链到达时缓存的待处理链接（前端启动衔接：深链事件可能
// 先于前端就绪，消费式拉取兜底；空串=无待处理）
func (h *Handler) ShareConsumePendingLink(ctx context.Context) *model.ApiResponse[string] {
	return model.Success(h.svc.ConsumeIncomingLink())
}

// ShareProtocolRegStatus 深链协议注册状态（Windows 为 HKCU 自注册视图，其余平台恒未注册）
func (h *Handler) ShareProtocolRegStatus(ctx context.Context) *model.ApiResponse[*ShareProtocolRegStatus] {
	status := QueryShareProtocolRegStatus()
	return model.Success(&status)
}

// ShareUnregisterProtocol 取消深链协议注册（便携版无卸载器的清理入口；安装版 HKLM 键
// 由卸载器管理，不受影响）
func (h *Handler) ShareUnregisterProtocol(ctx context.Context) *model.ApiResponse[any] {
	if err := UnregisterShareProtocol(); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}
