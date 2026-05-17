package assetserver

import (
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/zap"
)

// routeEntry 路由条目
type routeEntry struct {
	prefix   string
	handler  http.Handler
	priority int
}

// Router 通用 HTTP 路由多路复用器
// 将 http://wails.localhost/ 请求按路径前缀分发到不同处理器，未匹配的请求回退到前端静态资源服务器
type Router struct {
	mu       sync.RWMutex
	routes   []*routeEntry
	fallback http.Handler
}

// NewRouter 创建路由多路复用器
func NewRouter(frontendAssets fs.FS) *Router {
	return &Router{
		routes:   make([]*routeEntry, 0),
		fallback: application.AssetFileServerFS(frontendAssets),
	}
}

// Handle 注册路由处理器
func (r *Router) Handle(prefix string, handler http.Handler, priority int) {
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.routes = append(r.routes, &routeEntry{
		prefix:   prefix,
		handler:  handler,
		priority: priority,
	})

	sort.SliceStable(r.routes, func(i, j int) bool {
		li, lj := len(r.routes[i].prefix), len(r.routes[j].prefix)
		if li != lj {
			return li > lj
		}
		return r.routes[i].priority > r.routes[j].priority
	})

	logger.Log.Info("路由已注册", zap.String("prefix", prefix), zap.Int("priority", priority))
}

// Remove 移除路由处理器
func (r *Router) Remove(prefix string) {
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for i, route := range r.routes {
		if route.prefix == prefix {
			r.routes = append(r.routes[:i], r.routes[i+1:]...)
			logger.Log.Info("路由已移除", zap.String("prefix", prefix))
			return
		}
	}
}

// ServeHTTP 实现 http.Handler 接口
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, route := range r.routes {
		if strings.HasPrefix(path, route.prefix) || path+"/" == route.prefix {
			route.handler.ServeHTTP(w, req)
			return
		}
	}

	r.fallback.ServeHTTP(w, req)
}

var _ http.Handler = (*Router)(nil)
