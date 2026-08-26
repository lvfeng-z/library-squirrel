package workdirGuard

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
)

// Handler 工作目录防护 Handler（暴露给前端：平台防护机制信息 + 可写性探测）
type Handler struct {
	guard Guard
}

// NewHandler 创建防护 Handler
func NewHandler(guard Guard) *Handler {
	return &Handler{guard: guard}
}

// GuardInfoResponse GetWorkDirGuardInfo 响应：平台防护机制信息 + 当前 workDir 可写性探测结果
type GuardInfoResponse struct {
	Info     Info   `json:"info"`     // 平台防护机制（机制名/受支持/引导文案）
	ProbeOk  bool   `json:"probeOk"`  // 探测通过（workDir 当前可写；workDir 为空未探测时为 false）
	ProbeErr string `json:"probeErr"` // 探测失败原因（ProbeOk=false 时给出）
}

// GetWorkDirGuardInfo 返回平台防护机制信息与当前 workDir 可写性探测结果。
// workDir 为空（首次进入设置页未配置目录）时跳过探测，仅返回机制信息。
func (h *Handler) GetWorkDirGuardInfo(ctx context.Context, workDir string) *model.ApiResponse[GuardInfoResponse] {
	if h == nil || h.guard == nil {
		return model.Error[GuardInfoResponse]("工作目录防护模块不可用")
	}
	resp := GuardInfoResponse{Info: h.guard.Info()}
	if workDir == "" {
		return model.Success(resp)
	}
	if err := h.guard.Probe(ctx, workDir); err != nil {
		resp.ProbeOk = false
		resp.ProbeErr = err.Error()
	} else {
		resp.ProbeOk = true
	}
	return model.Success(resp)
}
