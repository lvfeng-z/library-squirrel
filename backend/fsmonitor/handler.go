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
	Domain   int    `json:"domain"`   // 0=store 资源文件域 1=backup 保管清单域
	Kind     int    `json:"kind"`     // 0=Move 1=Delete 2=Untracked 3=DirMove
	KindName string `json:"kindName"` // 可读名称（域感知）
	FromPath string `json:"fromPath"` // Move: 旧路径；Delete: 消失路径
	ToPath   string `json:"toPath"`   // Move: 新路径
	StoreID  int64  `json:"storeId"`  // store 域条目：关联记录 ID；其余 0
	BackupID int64  `json:"backupId"` // backup 域条目：关联清单行 ID；其余 0
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
			Domain:   int(pc.Domain),
			Kind:     int(pc.Kind),
			KindName: kindName(pc.Domain, pc.Kind),
			FromPath: pc.FromPath,
			ToPath:   pc.ToPath,
			StoreID:  pc.StoreID,
			BackupID: pc.BackupID,
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

// AutoRepairPolicyDTO 自动修复策略项（前端渲染策略下拉；仅 Options 多于一项的组合提供配置 UI）
type AutoRepairPolicyDTO struct {
	Key     string   `json:"key"`     // "<domain>:<kind>"，settings 保存键（fsmonitor.autoRepairPolicies 的键）
	Label   string   `json:"label"`   // 展示名
	Options []string `json:"options"` // 可选项（RepairAction 字符串，不可选项不暴露）
	Default string   `json:"default"` // 内置默认动作
}

// GetAutoRepairPolicySchema 返回自动修复策略可选项集（前端据此渲染策略下拉；可选项由 apply 实际能力约束）
func (h *Handler) GetAutoRepairPolicySchema(ctx context.Context) *model.ApiResponse[[]AutoRepairPolicyDTO] {
	items := make([]AutoRepairPolicyDTO, 0, len(autoRepairPolicies))
	for _, p := range autoRepairPolicies {
		opts := make([]string, 0, len(p.Options))
		for _, o := range p.Options {
			opts = append(opts, string(o))
		}
		items = append(items, AutoRepairPolicyDTO{
			Key:     p.Key,
			Label:   p.Label,
			Options: opts,
			Default: string(p.Default),
		})
	}
	return model.Success(items)
}

// kindName 语义变更类型可读名称（域感知：backup 域缺失/移动面向保管清单行）
func kindName(domain ChangeDomain, k SemanticKind) string {
	if domain == DomainBackup {
		switch k {
		case SemanticMove:
			return "备份文件移动/改名"
		case SemanticDelete:
			return "备份文件缺失"
		default:
			return "未知备份变更"
		}
	}
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
