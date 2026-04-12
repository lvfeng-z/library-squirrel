package extension

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

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

// SSESlotEvent SSE插槽事件
type SSESlotEvent struct {
	Type SlotEventType `json:"type"`
	Data interface{}   `json:"data"`
}

// SSESlotPusher SSE 插槽事件推送器
type SSESlotPusher struct {
	clients map[string]chan<- SSESlotEvent
	mu      sync.RWMutex
}

// NewSSESlotPusher 创建SSE插槽推送器
func NewSSESlotPusher() *SSESlotPusher {
	return &SSESlotPusher{
		clients: make(map[string]chan<- SSESlotEvent),
	}
}

// Register 注册客户端
func (p *SSESlotPusher) Register(clientId string) chan SSESlotEvent {
	p.mu.Lock()
	defer p.mu.Unlock()

	ch := make(chan SSESlotEvent, 100)
	p.clients[clientId] = ch
	return ch
}

// Unregister 取消注册
func (p *SSESlotPusher) Unregister(clientId string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.clients, clientId)
}

// PushRegister 推送注册事件
func (p *SSESlotPusher) PushRegister(slotID string, data interface{}) {
	event := SSESlotEvent{
		Type: SlotEventRegister,
		Data: map[string]interface{}{
			"slotId": slotID,
			"slot":   data,
		},
	}
	p.broadcast(event)
}

// PushUnregister 推送注销事件
func (p *SSESlotPusher) PushUnregister(slotID string, pluginID int64) {
	event := SSESlotEvent{
		Type: SlotEventUnregister,
		Data: map[string]interface{}{
			"slotId":   slotID,
			"pluginId": pluginID,
		},
	}
	p.broadcast(event)
}

// PushBatchRegister 推送批量注册事件
func (p *SSESlotPusher) PushBatchRegister(slots []interface{}) {
	event := SSESlotEvent{
		Type: SlotEventBatchRegister,
		Data: map[string]interface{}{
			"slots": slots,
		},
	}
	p.broadcast(event)
}

// broadcast 向所有客户端广播事件
func (p *SSESlotPusher) broadcast(event SSESlotEvent) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, ch := range p.clients {
		select {
		case ch <- event:
		default:
			// channel 满了，跳过
		}
	}
}

// SSEHandler SSE HTTP Handler
func (p *SSESlotPusher) SSEHandler(w http.ResponseWriter, r *http.Request) {
	clientId := r.URL.Query().Get("clientId")
	if clientId == "" {
		http.Error(w, "clientId required", http.StatusBadRequest)
		return
	}

	// 设置SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 注册客户端
	ch := p.Register(clientId)
	defer p.Unregister(clientId)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// 心跳 ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// 监听事件和心跳
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(event.Data)
			fmt.Fprintf(w, "event: %s\n", event.Type)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

		case <-ticker.C:
			fmt.Fprintf(w, "event: heartbeat\n")
			fmt.Fprintf(w, "data: {\"time\": %d}\n\n", time.Now().UnixMilli())
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}
