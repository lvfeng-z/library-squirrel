package extension

import (
	"io"
	"sync"
)

// StreamManager 管理活跃的二进制流
// 主进程侧：接收插件发送的二进制帧，按 streamId 分发给对应的 StreamReader
type StreamManager struct {
	streams sync.Map // streamID -> chan []byte
}

// NewStreamManager 创建 StreamManager
func NewStreamManager() *StreamManager {
	return &StreamManager{}
}

// RegisterStream 注册一个新流，返回数据通道
// 必须在发送 taskHandler/start RPC 之前调用，避免竞态
func (m *StreamManager) RegisterStream(streamID string) chan []byte {
	ch := make(chan []byte, 64)
	m.streams.Store(streamID, ch)
	return ch
}

// UnregisterStream 注销流并关闭通道
func (m *StreamManager) UnregisterStream(streamID string) {
	if v, ok := m.streams.LoadAndDelete(streamID); ok {
		close(v.(chan []byte))
	}
}

// HandleFrame 处理二进制帧
// payload 格式: [1字节 streamID长度] [N字节 streamID] [剩余字节: 数据块]
// 数据块长度为 0 表示流结束（EOF）
func (m *StreamManager) HandleFrame(payload []byte) {
	if len(payload) == 0 {
		return
	}

	streamIDLen := int(payload[0])
	if 1+streamIDLen > len(payload) {
		return
	}

	streamID := string(payload[1 : 1+streamIDLen])
	data := payload[1+streamIDLen:]

	v, ok := m.streams.Load(streamID)
	if !ok {
		return
	}

	ch := v.(chan []byte)
	select {
	case ch <- data:
	default:
		// 通道已满，丢弃数据（流正在关闭时不阻塞调度循环）
	}
}

// CloseAll 关闭所有活跃流
// 在插件进程终止时调用
func (m *StreamManager) CloseAll() {
	m.streams.Range(func(key, value any) bool {
		m.streams.Delete(key)
		close(value.(chan []byte))
		return true
	})
}

// StreamReader 从 StreamManager 的数据通道读取数据，实现 io.ReadCloser
// 对接 ManagedTask.run() 的 32KB 分块读取循环
type StreamReader struct {
	ch       chan []byte
	buf      []byte
	bufOff   int
	closed   bool
	mu       sync.Mutex
	unregFn  func() // 注销回调，关闭时调用
}

// NewStreamReader 创建 StreamReader
func NewStreamReader(ch chan []byte, unregFn func()) *StreamReader {
	return &StreamReader{
		ch:      ch,
		unregFn: unregFn,
	}
}

// Read 实现 io.Reader
func (r *StreamReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, io.EOF
	}

	// 如果缓冲区有数据，先返回
	if r.bufOff < len(r.buf) {
		n := copy(p, r.buf[r.bufOff:])
		r.bufOff += n
		return n, nil
	}

	// 从通道读取下一块数据
	data, ok := <-r.ch
	if !ok {
		return 0, io.EOF // 通道已关闭
	}

	// 零长度数据表示 EOF
	if len(data) == 0 {
		return 0, io.EOF
	}

	n := copy(p, data)
	if n < len(data) {
		r.buf = data
		r.bufOff = n
	}
	return n, nil
}

// Close 实现 io.Closer
func (r *StreamReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.closed {
		r.closed = true
		if r.unregFn != nil {
			r.unregFn()
		}
	}
	return nil
}
