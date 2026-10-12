package settings

import (
	"errors"

	"github.com/library-squirrel/backend/base/model"
)

// ErrExternalApiRuntimeUnavailable 外部接口运行时联动未接线（装配期异常；设置持久化
// 不受影响，仅缺监听起停与发现文件联动）
var ErrExternalApiRuntimeUnavailable = errors.New("外部接口运行时未接入")

// 外部接口设置的存储路径（SettingChange.Path 用）
const (
	externalApiEnabledPath = "externalApi.enabled"
	externalApiTokenPath   = "externalApi.token"
)

// ExternalApiRuntime 外部接口运行时联动接口（由外部接口模块的生命周期管理器实现，
// app 装配注入）。设置侧完成持久化后经此把目标态同步到监听器与发现文件；入出参用
// 原语，本包不感知监听器细节
type ExternalApiRuntime interface {
	// SyncExternalApi 按设置目标态差量同步：开关→监听起停与发现文件写删、固定端口
	// 变更→换绑、token 变更→监听器换钥。返回错误=本次同步未完全达成（降级语义，
	// 设置已持久化）
	SyncExternalApi(enabled bool, port int, token string) error
	// NewExternalApiToken 生成新 token 随机值（仅生成；换钥与发现文件更新经持久化
	// 后的 SyncExternalApi 差量完成）
	NewExternalApiToken() (string, error)
	// ExternalApiRuntimeStatus 返回运行时状态（是否监听、监听地址、最近一次同步失败原因）
	ExternalApiRuntimeStatus() (running bool, addr string, lastErr string)
}

// ExternalApiStatus 外部接口运行时状态（开关切换响应载荷，前端降级提示与监听地址展示用）
type ExternalApiStatus struct {
	Running bool `json:"running"`
	// Addr 监听地址（未监听为空）
	Addr string `json:"addr"`
	// LastError 最近一次同步失败原因（无失败为空）
	LastError string `json:"lastError"`
}

// Handler 设置 Handler
type Handler struct {
	svc *Service
	// externalApi 外部接口运行时联动（nil=未接线，控制方法降级为纯持久化）
	externalApi ExternalApiRuntime
}

// NewHandler 创建设置 Handler。externalApi 为外部接口运行时联动（app 装配期注入，
// 允许 nil——装配失败时控制方法降级为纯持久化）
func NewHandler(svc *Service, externalApi ExternalApiRuntime) *Handler {
	return &Handler{svc: svc, externalApi: externalApi}
}

// Get 获取设置
func (h *Handler) Get() *model.ApiResponse[*Settings] {
	result := h.svc.GetSettings()
	return model.Success(result)
}

// Save 保存设置
func (h *Handler) Save(changes []SettingChange) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.SaveSettings(changes))
}

// Reset 重置设置。重置会把外部接口 token 清回空默认值——随后重新生成持久化，
// 维持 token 恒非空不变量（设置保存回调完成运行时同步：默认关停监听、删发现文件、
// 监听器换新 token）
func (h *Handler) Reset() *model.ApiResponse[any] {
	if err := h.svc.ResetSettings(); err != nil {
		return model.HandleVoid(err)
	}
	if err := h.ensureExternalApiToken(); err != nil {
		return model.HandleVoid(err)
	}
	return model.HandleVoid(nil)
}

// SetExternalApiEnabled 切换外部接口开关：持久化设置并同步运行时（起停监听、发现文件
// 写删）。运行时同步失败为降级——设置已保存，成功信封以附注说明失败原因，状态载荷
// 同步给出（前端提示用户，重开一次开关即重试）
func (h *Handler) SetExternalApiEnabled(enabled bool) *model.ApiResponse[*ExternalApiStatus] {
	if enabled {
		// 开启前置保证 token 非空（恒非空不变量；空则生成持久化，随本开关保存一并同步）
		if err := h.ensureExternalApiToken(); err != nil {
			return model.HandleError[*ExternalApiStatus](err)
		}
	}
	if err := h.svc.SaveSettings([]SettingChange{{Path: externalApiEnabledPath, Value: enabled}}); err != nil {
		return model.HandleError[*ExternalApiStatus](err)
	}
	return h.externalApiSyncedStatus()
}

// ResetExternalApiToken 重置外部接口 token：生成新 token 持久化并同步运行时（监听器
// 换钥——旧 token 立即失效、启用中发现文件随写）；返回新 token 供前端刷新展示
func (h *Handler) ResetExternalApiToken() *model.ApiResponse[string] {
	if h.externalApi == nil {
		return model.HandleError[string](ErrExternalApiRuntimeUnavailable)
	}
	newToken, err := h.externalApi.NewExternalApiToken()
	if err != nil {
		return model.HandleError[string](err)
	}
	if err := h.svc.SaveSettings([]SettingChange{{Path: externalApiTokenPath, Value: newToken}}); err != nil {
		return model.HandleError[string](err)
	}
	return h.externalApiSyncedToken(newToken)
}

// ensureExternalApiToken 设置中的外部接口 token 为空时生成新值持久化（运行时未接线则
// 跳过——纯持久化路径无生成来源；恒非空不变量由应用装配兜底）
func (h *Handler) ensureExternalApiToken() error {
	if h.externalApi == nil {
		return nil
	}
	if h.svc.GetSettings().ExternalApi.Token != "" {
		return nil
	}
	newToken, err := h.externalApi.NewExternalApiToken()
	if err != nil {
		return err
	}
	return h.svc.SaveSettings([]SettingChange{{Path: externalApiTokenPath, Value: newToken}})
}

// externalApiSyncedStatus 开关切换后同步一次运行时并组装状态响应：有失败时以附注说明
//（降级——设置已生效，运行时态待下次同步收敛）
func (h *Handler) externalApiSyncedStatus() *model.ApiResponse[*ExternalApiStatus] {
	if h.externalApi == nil {
		return model.Success(&ExternalApiStatus{})
	}
	s := h.svc.GetSettings().ExternalApi
	if err := h.externalApi.SyncExternalApi(s.Enabled, s.Port, s.Token); err != nil {
		running, addr, lastErr := h.externalApi.ExternalApiRuntimeStatus()
		return model.SuccessWithMsg(&ExternalApiStatus{Running: running, Addr: addr, LastError: lastErr},
			"设置已保存，但外部接口同步失败: "+lastErr)
	}
	running, addr, _ := h.externalApi.ExternalApiRuntimeStatus()
	return model.Success(&ExternalApiStatus{Running: running, Addr: addr})
}

// externalApiSyncedToken token 重置后同步一次运行时并携带新 token 返回
func (h *Handler) externalApiSyncedToken(newToken string) *model.ApiResponse[string] {
	if h.externalApi == nil {
		return model.Success(newToken)
	}
	s := h.svc.GetSettings().ExternalApi
	if err := h.externalApi.SyncExternalApi(s.Enabled, s.Port, s.Token); err != nil {
		return model.SuccessWithMsg(newToken, "token 已保存，但外部接口同步失败: "+err.Error())
	}
	return model.Success(newToken)
}
