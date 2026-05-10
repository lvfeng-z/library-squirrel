package extension

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
)

// syncWriter 包装 io.ReadWriter，为 Write 操作提供互斥保护
// 确保多 goroutine 并发写入（RPCClient 请求、handler 响应）时不会交错
type syncWriter struct {
	io.ReadWriter
	mu sync.Mutex
}

func (w *syncWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ReadWriter.Write(b)
}

// PluginProcess 管理单个插件子进程的完整生命周期
type PluginProcess struct {
	publicId   string
	pluginInfo *PluginInfo
	cmd        *exec.Cmd
	conn       net.Conn
	listener   net.Listener
	socketPath string

	codec     *pluginsdk.FrameCodec
	rpcClient *pluginsdk.RPCClient
	streamMgr *StreamManager

	// RPC 处理函数表
	handlers map[string]pluginsdk.Handler

	// 委托给已有的 PluginContext 处理 ctx/* 中的数据/存储/查询等调用
	pluginCtx pluginsdk.PluginContext

	// 注册中心（用于 ctx/registerTaskHandler 等创建代理并注册）
	taskHandlerRegistry  *TaskHandlerRegistry
	siteBrowserRegistry  *SiteBrowserRegistry

	done chan struct{} // dispatch loop 退出信号
	once sync.Once
	intentionalShutdown atomic.Bool
}

// PluginProcessDeps 创建 PluginProcess 所需的依赖
type PluginProcessDeps struct {
	PluginInfo *PluginInfo
	// PluginContext 用于处理 ctx/* 中的数据/存储/查询等调用
	PluginCtx pluginsdk.PluginContext
	// 注册中心
	TaskHandlerRegistry *TaskHandlerRegistry
	SiteBrowserRegistry *SiteBrowserRegistry
}

// NewPluginProcess 创建 PluginProcess
func NewPluginProcess(deps PluginProcessDeps) *PluginProcess {
	return &PluginProcess{
		publicId:             deps.PluginInfo.PublicID,
		pluginInfo:           deps.PluginInfo,
		pluginCtx:            deps.PluginCtx,
		taskHandlerRegistry:  deps.TaskHandlerRegistry,
		siteBrowserRegistry:  deps.SiteBrowserRegistry,
		streamMgr:            NewStreamManager(),
		handlers:             make(map[string]pluginsdk.Handler),
		done:                 make(chan struct{}),
	}
}

// Start 启动插件子进程
// exePath: 插件可执行文件路径 (.exe)
func (p *PluginProcess) Start(exePath string) error {
	// 使用系统临时目录存放 socket 文件，避免 Windows 路径长度限制（~108字符）
	socketDir := filepath.Join(os.TempDir(), "library-squirrel")
	os.MkdirAll(socketDir, 0700)

	// 使用短哈希作为文件名，避免 Windows AF_UNIX 路径长度限制（~108字符）
	h := fnv.New32a()
	h.Write([]byte(p.publicId))
	p.socketPath = filepath.Join(socketDir, fmt.Sprintf("%08x.sock", h.Sum32()))

	// 清理残留 socket 文件
	os.Remove(p.socketPath)

	// 创建 UDS 监听
	var err error
	p.listener, err = net.Listen("unix", p.socketPath)
	if err != nil {
		return fmt.Errorf("listen unix socket %s: %w", p.socketPath, err)
	}

	// 启动子进程
	p.cmd = exec.Command(exePath, "--socket", p.socketPath)
	p.cmd.Stderr = os.Stderr // 插件日志输出到 stderr
	if err := p.cmd.Start(); err != nil {
		p.listener.Close()
		os.Remove(p.socketPath)
		return fmt.Errorf("start plugin process: %w", err)
	}

	// 等待子进程连接（超时 10s）
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := p.listener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		p.conn = conn
		acceptErr <- nil
	}()

	select {
	case err := <-acceptErr:
		if err != nil {
			p.kill()
			return fmt.Errorf("accept connection: %w", err)
		}
	case <-time.After(10 * time.Second):
		p.kill()
		return fmt.Errorf("plugin %s: connection timeout", p.publicId)
	}

	// 创建帧编解码器（使用 syncWriter 保护并发写入）
	syncConn := &syncWriter{ReadWriter: p.conn}
	p.codec = pluginsdk.NewFrameCodec(syncConn)
	p.rpcClient = pluginsdk.NewRPCClient(p.codec)

	// 注册 ctx/* RPC 处理函数
	registerHostHandlers(p.handlers, p)

	// 启动帧调度循环
	go p.dispatchLoop()

	// 监控子进程退出
	go p.watchProcess()

	logger.Log.Infof("插件子进程已启动: %s (pid=%d)", p.publicId, p.cmd.Process.Pid)
	return nil
}

// SendActivate 发送 activate 通知给插件子进程
func (p *PluginProcess) SendActivate(init *pluginsdk.PluginContextInit) error {
	return p.rpcClient.Notify("activate", init)
}

// SendShutdown 发送 shutdown 通知给插件子进程
func (p *PluginProcess) SendShutdown() error {
	return p.rpcClient.Notify("shutdown", nil)
}

// Stop 停止插件子进程并清理资源
func (p *PluginProcess) Stop() {
	p.once.Do(func() {
		p.intentionalShutdown.Store(true)
		logger.Log.Infof("正在停止插件子进程: %s", p.publicId)

		// 发送 shutdown 通知
		_ = p.SendShutdown()

		time.Sleep(100 * time.Millisecond)

		p.kill()
	})
}

// Done 返回一个通道，在子进程退出时关闭
func (p *PluginProcess) Done() <-chan struct{} {
	return p.done
}

// dispatchLoop 帧调度循环
// 从 UDS 连接读取帧，根据类型分发到对应的处理器
func (p *PluginProcess) dispatchLoop() {
	defer func() {
		if !p.intentionalShutdown.Load() {
			p.streamMgr.SetCloseError(pluginsdk.ErrPluginCrashed)
		}
		p.rpcClient.Close()
		p.streamMgr.CloseAll()
		close(p.done)
	}()

	for {
		payload, frameType, err := p.codec.ReadFrame()
		if err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				logger.Log.Errorf("插件 %s 帧读取错误: %v", p.publicId, err)
			}
			return
		}

		switch frameType {
		case pluginsdk.FrameTypeJSON:
			p.handleJSONFrame(payload)
		case pluginsdk.FrameTypeBinary:
			p.streamMgr.HandleFrame(payload)
		}
	}
}

// handleJSONFrame 处理 JSON-RPC 帧
// 区分请求（有 method 字段）和响应（有 id 无 method）
func (p *PluginProcess) handleJSONFrame(payload []byte) {
	var raw struct {
		Method string `json:"method"`
		ID     *int64 `json:"id"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		logger.Log.Errorf("插件 %s: JSON 帧解析失败: %v", p.publicId, err)
		return
	}

	if raw.Method != "" {
		// 请求：来自插件的 ctx/* 调用
		p.handleRequest(payload)
	} else if raw.ID != nil {
		// 响应：对应我们发送的 taskHandler/* 等请求
		if err := p.rpcClient.HandleResponse(payload); err != nil {
			logger.Log.Errorf("插件 %s: 响应处理失败: %v", p.publicId, err)
		}
	}
}

// handleRequest 处理来自插件的 JSON-RPC 请求
func (p *PluginProcess) handleRequest(payload []byte) {
	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID     int64           `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params,omitempty"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return
	}

	handler, ok := p.handlers[req.Method]
	if !ok {
		p.sendErrorResponse(req.ID, -32601, fmt.Sprintf("方法未找到: %s", req.Method))
		return
	}

	result, err := handler(req.Params)
	if err != nil {
		p.sendErrorResponse(req.ID, -32000, err.Error())
	} else {
		p.sendResultResponse(req.ID, result)
	}
}

func (p *PluginProcess) sendResultResponse(id int64, result any) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
	}
	if result != nil {
		resp["result"] = result
	}
	data, _ := json.Marshal(resp)
	if err := p.codec.WriteJSON(data); err != nil {
		logger.Log.Errorf("插件 %s: 写入响应失败: %v", p.publicId, err)
	}
}

func (p *PluginProcess) sendErrorResponse(id int64, code int, message string) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	data, _ := json.Marshal(resp)
	if err := p.codec.WriteJSON(data); err != nil {
		logger.Log.Errorf("插件 %s: 写入错误响应失败: %v", p.publicId, err)
	}
}

// watchProcess 监控子进程退出
func (p *PluginProcess) watchProcess() {
	err := p.cmd.Wait()
	if err != nil {
		logger.Log.Warnf("插件 %s 进程异常退出: %v", p.publicId, err)
	} else {
		logger.Log.Infof("插件 %s 进程正常退出", p.publicId)
	}

	// 关闭连接以中断 dispatchLoop 的阻塞读
	if p.conn != nil {
		p.conn.Close()
	}
}

// kill 强制终止子进程
func (p *PluginProcess) kill() {
	if p.conn != nil {
		p.conn.Close()
	}
	if p.listener != nil {
		p.listener.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}
	os.Remove(p.socketPath)
}
