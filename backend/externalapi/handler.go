package externalapi

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
)

// Handler 外部接口的 Wails 绑定面：设置页「外部操作记录」只读列表的读取入口。记录的
// 写入不经此处（统一在操作分发层落账，见 oprecord.go），本面只做问责回看的分页读取
type Handler struct {
	records OpRecordStore
}

// NewHandler 创建外部接口读取面 handler（记录面未装配时列表方法回失败）
func NewHandler(records OpRecordStore) *Handler {
	return &Handler{records: records}
}

// ListOpRecords 分页列出外部操作记录（最新调用在前）。只读：记录行不可编辑与删除，
// 老化行由轮转按 TTL 与容量自动裁剪
func (h *Handler) ListOpRecords(ctx context.Context, pageNumber, pageSize int) *model.ApiResponse[*model.Page[entity.ExternalApiOpRecord]] {
	if h.records == nil {
		return model.Error[*model.Page[entity.ExternalApiOpRecord]]("外部操作记录未接入")
	}
	page, err := h.records.PageRecent(ctx, normalizeQueryPageNumber(pageNumber), normalizeQueryPageSize(pageSize))
	if err != nil {
		return model.HandleError[*model.Page[entity.ExternalApiOpRecord]](err)
	}
	return model.Success(page)
}
