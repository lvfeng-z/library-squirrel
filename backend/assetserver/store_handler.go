package assetserver

import (
	"context"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/library-squirrel/backend/base/logger"
	"go.uber.org/zap"
)

// StoreStatusChecker 检查存储记录的状态
type StoreStatusChecker interface {
	// IsCompleteByPath 根据相对路径检查记录是否已完成
	IsCompleteByPath(ctx context.Context, relPath string) bool
}

// StoreFileHandler 从工作目录提供文件服务的 HTTP Handler
// URL 格式: /store/{relativePath}?params...
// 文件存储路径: {workDir}/{relativePath}
type StoreFileHandler struct {
	mu             sync.RWMutex
	workDir        string
	statusChecker  StoreStatusChecker
}

// NewStoreFileHandler 创建存储文件处理器
func NewStoreFileHandler(statusChecker StoreStatusChecker) *StoreFileHandler {
	return &StoreFileHandler{
		statusChecker: statusChecker,
	}
}

// SetStatusChecker 设置状态检查器
func (h *StoreFileHandler) SetStatusChecker(checker StoreStatusChecker) {
	h.statusChecker = checker
}

// SetWorkDir 设置工作目录
func (h *StoreFileHandler) SetWorkDir(dir string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.workDir = dir
	logger.Log.Info("存储工作目录已设置", zap.String("dir", dir))
}

// ServeHTTP 处理存储文件请求
// URL 格式: /store/{relativePath}
// relativePath 是相对于 workdir 的相对路径
func (h *StoreFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	relativePath := strings.TrimPrefix(r.URL.Path, "/store/")
	if relativePath == "" {
		http.NotFound(w, r)
		return
	}

	cleanedPath := filepath.Clean(relativePath)
	// 拒绝路径穿越:cleanedPath 不得以 ".." 路径组件开头(中间的 "/../" 已被 filepath.Clean 解析上溯)。
	// 勿用 strings.Contains(cleanedPath, "..")——会误伤合法文件名里含 ".." 子串的字符
	// (如 bilibili 标题 "..._thumbnail_000.jpg" 的省略号 "..." 被误判穿越返回 404)。
	// 纵深防御:下方 absPath HasPrefix(workDir) 是权威防穿越校验。
	if cleanedPath == ".." || strings.HasPrefix(cleanedPath, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}

	// 检查 PersistentStore 记录状态，未完成记录返回 404
	if h.statusChecker != nil && !h.statusChecker.IsCompleteByPath(r.Context(), cleanedPath) {
		http.NotFound(w, r)
		return
	}

	absPath := filepath.Join(workDir, cleanedPath)

	if !strings.HasPrefix(filepath.Clean(absPath), filepath.Clean(workDir)) {
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

var _ http.Handler = (*StoreFileHandler)(nil)
