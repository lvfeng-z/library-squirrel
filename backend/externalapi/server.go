// Package externalapi 外部 AI 操作接口：本机回环 HTTP JSON 操作面，供部署在同一台机器
// 上的外部智能体运行时以程序化方式操作本软件——人用 GUI、机用接口，两入口汇入同一
// 服务层。监听器只绑 127.0.0.1，随设置开关起停；鉴权为 Bearer token 内存单点比对；
// 操作经显式注册表分发，响应信封与 Wails 前端面同构（复用 model.ApiResponse 序列化
// 形态：失败=纯失败、msg 用户可读）
package externalapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
)

const (
	// loopbackHost 监听绑定地址：只绑本机回环，不对局域网暴露
	loopbackHost = "127.0.0.1"
	// healthPath 免鉴权探测端点路径
	healthPath = "/api/health"
	// opPathPrefix 统一操作调用端点路径前缀（POST /api/op/{operationId}）
	opPathPrefix = "/api/op/"
	// maxRequestBodyBytes 单请求体大小上限：操作参数 JSON 为小载荷，超限即拒绝
	maxRequestBodyBytes = 1 << 20
	// maxConcurrentDispatches 分发层并发上限：HTTP 入口不限连接数，以信号量限制同时
	// 进入操作调用的请求数，超限快速失败
	maxConcurrentDispatches = 8
	// shutdownGracePeriod 停止监听时在途请求收尾的等待上界
	shutdownGracePeriod = 5 * time.Second
)

// Server 外部接口监听器：持有回环 HTTP 监听、token 鉴权器与操作注册表。Start/Stop
// 幂等，支持停止后再次启动
type Server struct {
	auth     *TokenAuthenticator
	registry *Registry
	// opRecords 外部操作记录面（装配期经 SetOpRecordStore 注入；nil = 未装配，调用不落记录）
	opRecords OpRecordStore

	mu       sync.Mutex
	listener net.Listener
	httpSrv  *http.Server

	// dispatchSem 分发层信号量（容量=并发上限，非阻塞获取）
	dispatchSem chan struct{}
}

// NewServer 创建外部接口服务。registry 由调用方构建并注册操作；token 为空时随机生成
// （外部侧经用户数据目录的发现文件获知 token）
func NewServer(registry *Registry, token string) (*Server, error) {
	if token == "" {
		generated, err := GenerateToken()
		if err != nil {
			return nil, err
		}
		token = generated
	}
	return &Server{
		auth:        NewTokenAuthenticator(token),
		registry:    registry,
		dispatchSem: make(chan struct{}, maxConcurrentDispatches),
	}, nil
}

// Token 返回当前鉴权 token（发现文件写入用）
func (s *Server) Token() string {
	return s.auth.Token()
}

// SetToken 更换鉴权 token，旧 token 随即失效（生命周期管理器的换钥通道；
// TokenAuthenticator 内部原子存取，无监听状态耦合）
func (s *Server) SetToken(token string) {
	s.auth.SetToken(token)
}

// Start 起监听并返回绑定地址（port 为 0 时由系统随机分配）。已在监听时为幂等空操作，
// 返回当前地址（此时请求的端口参数被忽略）。监听失败（如端口被占用）返回错误，
// 由调用方降级处理
func (s *Server) Start(port int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String(), nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", loopbackHost, port))
	if err != nil {
		return "", fmt.Errorf("外部接口监听启动失败: %w", err)
	}
	s.listener = ln
	// 局部变量持有本次监听的 http.Server：服务 goroutine 闭包捕获局部值而非结构体字段，
	// Stop 置空字段后不影响已在跑的 Serve
	httpSrv := &http.Server{
		Handler:           s.buildHandler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	s.httpSrv = httpSrv
	go func() {
		if serveErr := httpSrv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Log.Errorw("外部接口 HTTP 服务异常退出", "error", serveErr)
		}
	}()
	addr := ln.Addr().String()
	logger.Log.Infow("外部接口监听已启动", "addr", addr)
	return addr, nil
}

// Stop 停监听并等待在途请求收尾（上界 5 秒，超时返回错误）。未在监听时为幂等空操作
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpSrv == nil {
		return nil
	}
	srv := s.httpSrv
	s.httpSrv = nil
	s.listener = nil
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGracePeriod)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("外部接口监听停止超时: %w", err)
	}
	logger.Log.Info("外部接口监听已停止")
	return nil
}

// buildHandler 构建路由：免鉴权探测端点 + 鉴权后的统一操作端点
func (s *Server) buildHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+healthPath, s.handleHealth)
	mux.HandleFunc("POST "+opPathPrefix, s.authenticated(s.handleOperation))
	// 注册表外的只读固定端点（能力清单、记账账表增量查询）挂在同一张路由表上，与统一操作端点
	// 共用鉴权与信封语义（路径与 /api/op/ 前缀无重叠，注册顺序不影响匹配）
	MountQueryRoutes(mux, s)
	return mux
}

// authenticated 鉴权中间件：不放行的请求一律 401（无 token、非 Bearer 形态、伪 token
// 同归鉴权失败）
func (s *Server) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.auth.Authenticate(r) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeJSON(w, http.StatusUnauthorized, model.Error[any]("鉴权失败：缺少有效 token"))
			return
		}
		next(w, r)
	}
}

// handleHealth 免鉴权探测端点：仅回启用态与应用版本。接口未启用时无监听（外部侧
// 连接拒绝），可达即启用，故启用态恒真
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true,
		"version": constant.AppVersion,
	})
}

// handleOperation 统一操作调用分发。状态码语义：鉴权失败 401（见中间件）；操作标识
// 未注册 404；请求体超限 413；并发超限 503 快速失败；操作已调用的成败一律 200、由
// 信封表达（失败=纯失败，msg 用户可读）
func (s *Server) handleOperation(w http.ResponseWriter, r *http.Request) {
	// 并发信号量：非阻塞获取，超限快速失败
	select {
	case s.dispatchSem <- struct{}{}:
		defer func() { <-s.dispatchSem }()
	default:
		writeJSON(w, http.StatusServiceUnavailable, model.Error[any]("外部接口并发已达上限，请稍后重试"))
		return
	}
	operationID := strings.TrimPrefix(r.URL.Path, opPathPrefix)
	if operationID == "" || strings.Contains(operationID, "/") {
		writeJSON(w, http.StatusNotFound, model.Error[any]("操作标识缺失或非法"))
		return
	}
	op, ok := s.registry.Lookup(operationID)
	if !ok {
		writeJSON(w, http.StatusNotFound, model.Error[any](fmt.Sprintf("未注册的操作: %s", operationID)))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, model.Error[any]("请求体超过大小上限"))
			return
		}
		writeJSON(w, http.StatusBadRequest, model.Error[any]("读取请求体失败"))
		return
	}
	// 操作执行与落账：记录行在响应写出前落库（尽力而为，失败仅日志），外部侧收到响应
	// 即可经记录查询端点看到本次调用
	started := time.Now()
	result, invokeErr := op.Invoke(r.Context(), json.RawMessage(body))
	s.recordInvocation(r.Context(), operationID, body, invokeErr == nil, r.Header.Get(ClientLabelHeader), time.Since(started))
	if invokeErr != nil {
		writeJSON(w, http.StatusOK, model.Error[any](invokeErr.Error()))
		return
	}
	writeJSON(w, http.StatusOK, model.Success(result))
}

// writeJSON 写 JSON 响应并显式声明内容类型
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// sampleEchoParams 样例操作参数：回显消息
type sampleEchoParams struct {
	Message string `json:"message"`
}

// RegisterSampleOperation 注册样例操作（回显入参消息并附服务器时间）。无任何业务依赖
// （不触数据库与业务 handler），用于证明操作注册表与响应信封通路
func RegisterSampleOperation(r *Registry) error {
	return RegisterHandler(r, "sample.echo", "样例回显操作：回显入参消息并附服务器时间，用于通路自检",
		func(ctx context.Context, params sampleEchoParams) (any, error) {
			return map[string]any{
				"echo":       params.Message,
				"serverTime": time.Now().Format(time.RFC3339),
			}, nil
		})
}
