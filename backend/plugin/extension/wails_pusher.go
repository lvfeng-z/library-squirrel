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
	PushRegister(id string, data FrontendExtensionResponse)
	PushUnregister(id string, pluginID int64, kind string)
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

// PushRegister 推送注册事件到前端
func (p *WailsFrontendExtensionPusher) PushRegister(id string, data FrontendExtensionResponse) {
	event := FrontendExtensionEventData{
		Event: string(FrontendExtensionEventRegister),
		ID:    id,
		Data:  data,
	}
	p.emitter.Emit("frontend-extension-register", event)
}

// PushUnregister 推送注销事件到前端
func (p *WailsFrontendExtensionPusher) PushUnregister(id string, pluginID int64, kind string) {
	event := FrontendExtensionEventData{
		Event:    string(FrontendExtensionEventUnregister),
		ID:       id,
		PluginID: pluginID,
		Kind:     kind,
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

// FrontendExtensionEventData 前端扩展事件数据
type FrontendExtensionEventData struct {
	Event    string      `json:"event"`
	ID       string      `json:"frontendExtensionId,omitempty"`
	PluginID int64       `json:"pluginId,omitempty"`
	Kind     string      `json:"kind,omitempty"`
	Data     interface{} `json:"data,omitempty"`
	Items    interface{} `json:"items,omitempty"`
}

// FrontendExtensionUnregisterItem 批量注销项
type FrontendExtensionUnregisterItem struct {
	ID   string `json:"frontendExtensionId"`
	Kind string `json:"kind"`
}
