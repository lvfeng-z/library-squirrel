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

// StoreStateResolver 按路径解析存储记录状态（含已删行；由 persistentStore.Service 实现）
type StoreStateResolver interface {
	// ResolveFileState completed=文件曾完整落盘；deleted=记录已软删（文件移 backup 或外部裁决失效）；
	// backupId=行内嵌的备份清单行 ID（0=无备份）
	// 无记录时 completed=true 兜底（按磁盘文件 fallback）
	ResolveFileState(ctx context.Context, relPath string) (completed bool, deleted bool, backupId int64)
}

// BackupPathResolver 按备份清单行 ID 取备份文件绝对路径（/store/ 兜底：软删记录文件在 backup/）
type BackupPathResolver interface {
	// ResolveBackupPathById 无备份返回空串
	ResolveBackupPathById(ctx context.Context, backupId int64) string
}

// StoreFileHandler 从工作目录提供文件服务的 HTTP Handler
// URL 格式: /store/{relativePath}?params...
// 文件存储路径: {workDir}/{relativePath}
// 状态路由：记录软删（deleted）的请求按 backup.original_file_path 反查兜底服务；
// 活行请求服务 workDir 原路径文件，缺失即 404（外部变更归 fsmonitor 观测域）
type StoreFileHandler struct {
	mu             sync.RWMutex
	workDir        string
	stateResolver  StoreStateResolver
	backupResolver BackupPathResolver
}

// NewStoreFileHandler 创建存储文件处理器
func NewStoreFileHandler(stateResolver StoreStateResolver) *StoreFileHandler {
	return &StoreFileHandler{
		stateResolver: stateResolver,
	}
}

// SetStateResolver 设置状态解析器（记录完成态/软删态路由）
func (h *StoreFileHandler) SetStateResolver(resolver StoreStateResolver) {
	h.stateResolver = resolver
}

// SetBackupResolver 设置备份路径解析器（软删作品的 /store/ 请求兜底）
func (h *StoreFileHandler) SetBackupResolver(resolver BackupPathResolver) {
	h.backupResolver = resolver
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

	// DB/backup 记录的路径基准是正斜杠（file_path 同基准），Windows 上 filepath.Clean 会把
	// 分隔符换成反斜杠导致记录查询永不命中——查库键经 ToSlash 还原，文件系统操作仍用 cleanedPath
	lookupPath := filepath.ToSlash(cleanedPath)

	// 状态路由：未完成记录 404（防半成品）；软删记录按行内 backup_id 定位备份文件服务
	if h.stateResolver != nil {
		completed, deleted, backupId := h.stateResolver.ResolveFileState(r.Context(), lookupPath)
		if !completed {
			http.NotFound(w, r)
			return
		}
		if deleted {
			if h.backupResolver != nil && backupId > 0 {
				if backupPath := h.backupResolver.ResolveBackupPathById(r.Context(), backupId); backupPath != "" {
					if bInfo, bErr := os.Stat(backupPath); bErr == nil && !bInfo.IsDir() {
						h.serveFile(w, r, backupPath)
						return
					}
				}
			}
			http.NotFound(w, r)
			return
		}
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

	h.serveFile(w, r, absPath)
}

// serveFile 按扩展名设置 Content-Type 并发送文件
func (h *StoreFileHandler) serveFile(w http.ResponseWriter, r *http.Request, absPath string) {
	ext := filepath.Ext(absPath)
	if mimeType := mime.TypeByExtension(ext); mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}
	http.ServeFile(w, r, absPath)
}

var _ http.Handler = (*StoreFileHandler)(nil)
