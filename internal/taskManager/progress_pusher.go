package taskManager

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SSEvent SSE事件
type SSEvent struct {
	Type string // "state_change" | "progress" | "error"
	Data interface{}
}

// SSEProgressPusher SSE 进度推送器
type SSEProgressPusher struct {
	// 客户端连接Map
	clients map[string]chan<- SSEvent
	mu      sync.RWMutex
}

// NewSSEProgressPusher 创建SSE推送器
func NewSSEProgressPusher() *SSEProgressPusher {
	return &SSEProgressPusher{
		clients: make(map[string]chan<- SSEvent),
	}
}

// Register 注册客户端
func (p *SSEProgressPusher) Register(clientId string) chan SSEvent {
	p.mu.Lock()
	defer p.mu.Unlock()

	ch := make(chan SSEvent, 100) // 带缓冲channel
	p.clients[clientId] = ch
	return ch
}

// Unregister 取消注册
func (p *SSEProgressPusher) Unregister(clientId string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.clients, clientId)
}

// PushStateChange 推送状态变化
func (p *SSEProgressPusher) PushStateChange(taskId int64, state TaskState) {
	event := SSEvent{
		Type: "state_change",
		Data: map[string]interface{}{
			"taskId": taskId,
			"state":  state,
		},
	}
	p.broadcast(event)
}

// PushProgress 推送进度
func (p *SSEProgressPusher) PushProgress(taskId int64, progress int) {
	event := SSEvent{
		Type: "progress",
		Data: map[string]interface{}{
			"taskId":   taskId,
			"progress": progress,
		},
	}
	p.broadcast(event)
}

// PushError 推送错误
func (p *SSEProgressPusher) PushError(taskId int64, err string) {
	event := SSEvent{
		Type: "error",
		Data: map[string]interface{}{
			"taskId": taskId,
			"error":  err,
		},
	}
	p.broadcast(event)
}

// broadcast 向所有客户端广播事件
func (p *SSEProgressPusher) broadcast(event SSEvent) {
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
// 用于处理前端的SSE连接请求
func (p *SSEProgressPusher) SSEHandler(w http.ResponseWriter, r *http.Request) {
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

	// 确保刷新
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
			// 发送心跳
			fmt.Fprintf(w, "event: heartbeat\n")
			fmt.Fprintf(w, "data: {\"time\": %d}\n\n", time.Now().UnixMilli())
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}
