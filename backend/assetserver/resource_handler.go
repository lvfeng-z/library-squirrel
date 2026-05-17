package assetserver

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/library-squirrel/backend/base/logger"
	"go.uber.org/zap"
)

// ResourceHandler 从工作目录的 resource 子目录提供文件服务的 HTTP Handler
// URL 格式: /resource/{relativePath}?params...
// 文件存储路径: {workDir}/resource/{relativePath}
type ResourceHandler struct {
	mu      sync.RWMutex
	workDir string
}

// NewResourceHandler 创建资源文件处理器
func NewResourceHandler() *ResourceHandler {
	return &ResourceHandler{}
}

// SetWorkDir 设置工作目录
func (h *ResourceHandler) SetWorkDir(dir string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.workDir = dir
	logger.Log.Info("资源工作目录已设置", zap.String("dir", dir))
}

// ServeHTTP 处理资源文件请求
// URL 格式: /resource/{relativePath}
// relativePath 是相对于 workdir/resource/ 的相对路径
// 支持查询参数（预留后续扩展，如缩略图尺寸等）
func (h *ResourceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	h.mu.RLock()
	workDir := h.workDir
	h.mu.RUnlock()

	if workDir == "" {
		http.NotFound(w, r)
		return
	}

	relativePath := strings.TrimPrefix(r.URL.Path, "/resource/")
	if relativePath == "" {
		http.NotFound(w, r)
		return
	}

	cleanedPath := filepath.Clean(relativePath)
	if strings.Contains(cleanedPath, "..") {
		http.NotFound(w, r)
		return
	}

	resourceDir := filepath.Join(workDir, "resource")
	absPath := filepath.Join(resourceDir, cleanedPath)

	if !strings.HasPrefix(filepath.Clean(absPath), filepath.Clean(resourceDir)) {
		http.NotFound(w, r)
		return
	}

	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	ext := filepath.Ext(absPath)
	if mimeType := mime.TypeByExtension(ext); mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}

	http.ServeFile(w, r, absPath)
}

// ResolveURL 构建资源文件的 URL
func (h *ResourceHandler) ResolveURL(relativePath string) string {
	return "/resource/" + relativePath
}

var _ http.Handler = (*ResourceHandler)(nil)
