package extension

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
func (p *WailsSlotPusher) PushRegister(slotID string, data interface{}) {
	event := SlotEventData{
		Event:  string(SlotEventRegister),
		SlotID: slotID,
		Data:   data,
	}
	p.emitter.Emit("slot-register", event)
}

// PushUnregister 推送注销事件到前端
func (p *WailsSlotPusher) PushUnregister(slotID string, pluginID int64) {
	event := SlotEventData{
		Event:    string(SlotEventUnregister),
		SlotID:   slotID,
		PluginID: pluginID,
	}
	p.emitter.Emit("slot-unregister", event)
}

// PushBatchRegister 推送批量注册事件到前端
func (p *WailsSlotPusher) PushBatchRegister(slots []interface{}) {
	event := SlotEventData{
		Event: string(SlotEventBatchRegister),
		Slots: slots,
	}
	p.emitter.Emit("slot-batch-register", event)
}

// SlotEventData 插槽事件数据
type SlotEventData struct {
	Event    string      `json:"event"`
	SlotID   string      `json:"slotId,omitempty"`
	PluginID int64       `json:"pluginId,omitempty"`
	Data     interface{} `json:"data,omitempty"`
	Slots    interface{} `json:"slots,omitempty"`
}
