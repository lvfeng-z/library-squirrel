package fsmonitor

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
)

// Handler 工作目录监控 Handler（暴露给前端：待修复变更列表 + 确认修复）
type Handler struct {
	svc *Service
}

// NewHandler 创建监控 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// PendingChangeDTO 待修复变更（前端展示用）
type PendingChangeDTO struct {
	ID       int64  `json:"id"`
	Kind     int    `json:"kind"`     // 0=Move 1=Delete 2=Untracked
	KindName string `json:"kindName"` // 可读名称
	FromPath string `json:"fromPath"` // Move: 旧路径；Delete: 消失路径
	ToPath   string `json:"toPath"`   // Move: 新路径
	StoreID  int64  `json:"storeId"`
}

// ListPendingChanges 列出待修复变更（供前端确认列表展示）
func (h *Handler) ListPendingChanges(ctx context.Context) *model.ApiResponse[[]PendingChangeDTO] {
	if h.svc == nil {
		return model.Success([]PendingChangeDTO{})
	}
	items := h.svc.ListPendingChanges()
	dtos := make([]PendingChangeDTO, 0, len(items))
	for _, pc := range items {
		dtos = append(dtos, PendingChangeDTO{
			ID:       pc.ID,
			Kind:     int(pc.Kind),
			KindName: kindName(pc.Kind),
			FromPath: pc.FromPath,
			ToPath:   pc.ToPath,
			StoreID:  pc.StoreID,
		})
	}
	return model.Success(dtos)
}

// ConfirmChange 用户确认修复动作
// id: ListPendingChanges 返回的待修复 ID
// action: "sync"(同步DB路径) | "restore"(复原) | "ack"(确认/标记失效)
func (h *Handler) ConfirmChange(ctx context.Context, id int64, action string) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.ConfirmChange(ctx, id, RepairAction(action)))
}

// kindName 语义变更类型可读名称
func kindName(k SemanticKind) string {
	switch k {
	case SemanticMove:
		return "文件移动/重命名"
	case SemanticDelete:
		return "文件删除"
	case SemanticUntracked:
		return "外部新增文件"
	case SemanticDirMove:
		return "目录移动/改名"
	default:
		return "未知"
	}
}
