package extension

// FrontendExtensionEventType 前端扩展事件类型
type FrontendExtensionEventType string

const (
	// FrontendExtensionEventRegister 前端扩展注册
	FrontendExtensionEventRegister FrontendExtensionEventType = "frontend-extension-register"
	// FrontendExtensionEventUnregister 前端扩展注销
	FrontendExtensionEventUnregister FrontendExtensionEventType = "frontend-extension-unregister"
	// FrontendExtensionEventBatchUnregister 批量注销
	FrontendExtensionEventBatchUnregister FrontendExtensionEventType = "frontend-extension-batch-unregister"
)

// FrontendExtensionPusher 前端扩展事件推送器接口
type FrontendExtensionPusher interface {
	PushRegister(data FrontendExtensionResponse)
	PushUnregister(item FrontendExtensionUnregisterItem)
	PushBatchUnregister(items []FrontendExtensionUnregisterItem)
}

// WailsEventEmitter Wails 事件发射器接口
type WailsEventEmitter interface {
	Emit(eventName string, data ...any) bool
}

// WailsFrontendExtensionPusher Wails 前端扩展事件推送器
// 使用 Wails Events 替代 HTTP SSE
type WailsFrontendExtensionPusher struct {
	emitter WailsEventEmitter
}

// NewWailsFrontendExtensionPusher 创建 Wails 前端扩展推送器
func NewWailsFrontendExtensionPusher(emitter WailsEventEmitter) *WailsFrontendExtensionPusher {
	return &WailsFrontendExtensionPusher{
		emitter: emitter,
	}
}

// PushRegister 推送注册事件到前端（信封 id 字段为 manifest 声明的裸 extensionId，与注销事件同语义）
func (p *WailsFrontendExtensionPusher) PushRegister(data FrontendExtensionResponse) {
	event := FrontendExtensionEventData{
		Event:          string(FrontendExtensionEventRegister),
		PluginPublicID: data.PluginPublicID,
		ID:             data.ID,
		Kind:           data.Type,
		Data:           data,
	}
	p.emitter.Emit("frontend-extension-register", event)
}

// PushUnregister 推送注销事件到前端
func (p *WailsFrontendExtensionPusher) PushUnregister(item FrontendExtensionUnregisterItem) {
	event := FrontendExtensionEventData{
		Event:          string(FrontendExtensionEventUnregister),
		PluginPublicID: item.PluginPublicID,
		ID:             item.ID,
		Kind:           item.Kind,
	}
	p.emitter.Emit("frontend-extension-unregister", event)
}

// PushBatchUnregister 推送批量注销事件到前端
func (p *WailsFrontendExtensionPusher) PushBatchUnregister(items []FrontendExtensionUnregisterItem) {
	event := FrontendExtensionEventData{
		Event: string(FrontendExtensionEventBatchUnregister),
		Items: items,
	}
	p.emitter.Emit("frontend-extension-batch-unregister", event)
}

// FrontendExtensionEventData 前端扩展事件数据（注册/注销/批量注销共用信封：
// pluginPublicId 与 frontendExtensionId 分列，frontendExtensionId 一律为 manifest 声明的裸 extensionId）
type FrontendExtensionEventData struct {
	Event          string      `json:"event"`
	PluginPublicID string      `json:"pluginPublicId,omitempty"`
	ID             string      `json:"frontendExtensionId,omitempty"`
	Kind           string      `json:"kind,omitempty"`
	Data           interface{} `json:"data,omitempty"`
	Items          interface{} `json:"items,omitempty"`
}

// FrontendExtensionUnregisterItem 批量注销项（frontendExtensionId 为 manifest 声明的裸 extensionId，
// 与注册事件 Response.ID 同语义；pluginPublicId 分列供前端派生复合 store 键）
type FrontendExtensionUnregisterItem struct {
	PluginPublicID string `json:"pluginPublicId"`
	ID             string `json:"frontendExtensionId"`
	Kind           string `json:"kind"`
}
