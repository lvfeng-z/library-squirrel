package extension

// SlotEventType 插槽事件类型
type SlotEventType string

const (
	// SlotEventRegister 插槽注册
	SlotEventRegister SlotEventType = "slot-register"
	// SlotEventUnregister 插槽注销
	SlotEventUnregister SlotEventType = "slot-unregister"
	// SlotEventBatchRegister 批量注册
	SlotEventBatchRegister SlotEventType = "slot-batch-register"
)

// SlotPusher 插槽事件推送器接口
type SlotPusher interface {
	PushRegister(slotID string, data SlotResponse)
	PushUnregister(slotID string, pluginID int64, slotType string)
	PushBatchUnregister(items []SlotUnregisterItem)
}

// WailsEventEmitter Wails 事件发射器接口
type WailsEventEmitter interface {
	Emit(eventName string, data ...any) bool
}

// WailsSlotPusher Wails 插槽事件推送器
// 使用 Wails Events 替代 HTTP SSE
type WailsSlotPusher struct {
	emitter WailsEventEmitter
}

// NewWailsSlotPusher 创建 Wails 插槽推送器
func NewWailsSlotPusher(emitter WailsEventEmitter) *WailsSlotPusher {
	return &WailsSlotPusher{
		emitter: emitter,
	}
}

// PushRegister 推送注册事件到前端
func (p *WailsSlotPusher) PushRegister(slotID string, data SlotResponse) {
	event := SlotEventData{
		Event:  string(SlotEventRegister),
		SlotID: slotID,
		Data:   data,
	}
	p.emitter.Emit("slot-register", event)
}

// PushUnregister 推送注销事件到前端
func (p *WailsSlotPusher) PushUnregister(slotID string, pluginID int64, slotType string) {
	event := SlotEventData{
		Event:    string(SlotEventUnregister),
		SlotID:   slotID,
		PluginID: pluginID,
		SlotType: slotType,
	}
	p.emitter.Emit("slot-unregister", event)
}

// PushBatchUnregister 推送批量注销事件到前端
func (p *WailsSlotPusher) PushBatchUnregister(items []SlotUnregisterItem) {
	event := SlotEventData{
		Event: string(SlotEventBatchRegister),
		Slots: items,
	}
	p.emitter.Emit("slot-batch-register", event)
}

// SlotEventData 插槽事件数据
type SlotEventData struct {
	Event    string      `json:"event"`
	SlotID   string      `json:"slotId,omitempty"`
	PluginID int64       `json:"pluginId,omitempty"`
	SlotType string      `json:"slotType,omitempty"`
	Data     interface{} `json:"data,omitempty"`
	Slots    interface{} `json:"slots,omitempty"`
}

// SlotUnregisterItem 批量注销项
type SlotUnregisterItem struct {
	SlotID   string `json:"slotId"`
	SlotType string `json:"slotType"`
}
