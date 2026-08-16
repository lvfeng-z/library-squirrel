package extension

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/library-squirrel/backend/base/logger"
	"go.uber.org/zap"
)

// pluginResourceMapping 插件资源路径映射
type pluginResourceMapping struct {
	rootPath    string   // 插件根目录绝对路径
	allowedDirs []string // staticResources 中声明的允许目录（如 "views/", "assets/"）
	cacheKey    string   // 缓存键（构建身份 buildId，未打标包为 version；进入资产 URL 与 ETag，令 immutable 长缓存随构建失效）
}

// StaticResourceService 插件静态资源服务，管理插件的静态资源路径映射和文件服务
type StaticResourceService struct {
	mu      sync.RWMutex
	plugins map[string]*pluginResourceMapping // key: pluginPublicId
}

// NewStaticResourceService 创建静态资源服务
func NewStaticResourceService() *StaticResourceService {
	return &StaticResourceService{
		plugins: make(map[string]*pluginResourceMapping),
	}
}

// RegisterPlugin 注册插件的静态资源路径（cacheKey 为缓存键：构建身份 buildId，未打标包为 version）
func (s *StaticResourceService) RegisterPlugin(publicId, absRootPath string, allowedDirs []string, cacheKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plugins[publicId] = &pluginResourceMapping{
		rootPath:    absRootPath,
		allowedDirs: allowedDirs,
		cacheKey:    cacheKey,
	}
	logger.Log.Info("插件静态资源已注册",
		zap.String("plugin", publicId),
		zap.Strings("dirs", allowedDirs),
	)
}

// UnregisterPlugin 注销插件的静态资源路径
func (s *StaticResourceService) UnregisterPlugin(publicId string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.plugins, publicId)
	logger.Log.Info("插件静态资源已注销", zap.String("plugin", publicId))
}

// ServeHTTP 处理插件静态资源请求
// URL 格式: /plugin/{publicId}/{cacheKey}/{relativePath}
// publicId 即插件 id（反向域名，不含 "/"，占一段）；cacheKey 为缓存键（构建身份 buildId，未打标包为 version）
func (s *StaticResourceService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析路径: /plugin/{publicId}/{cacheKey}/{relativePath}
	path := strings.TrimPrefix(r.URL.Path, "/plugin/")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}

	publicId := parts[0]
	// parts[1] = cacheKey（用于缓存，不参与文件路径解析）
	relativePath := parts[2]

	s.mu.RLock()
	mapping, ok := s.plugins[publicId]
	s.mu.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	// 安全校验：验证路径在 allowedDirs 内
	if !s.isPathAllowed(relativePath, mapping.allowedDirs) {
		http.NotFound(w, r)
		return
	}

	// 清理路径，防止路径穿越
	cleanedPath := filepath.Clean(relativePath)
	if strings.Contains(cleanedPath, "..") {
		http.NotFound(w, r)
		return
	}

	absPath := filepath.Join(mapping.rootPath, cleanedPath)

	// 最终安全检查：确保解析后的路径仍在插件根目录下
	if !strings.HasPrefix(absPath, mapping.rootPath) {
		http.NotFound(w, r)
		return
	}

	// 设置缓存头：URL 含缓存键（buildId/version），可长期缓存；ETag 同源，重构建必变
	etag := `"` + mapping.cacheKey + ":" + cleanedPath + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	// 检查 If-None-Match
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// 设置 Content-Type
	ext := filepath.Ext(absPath)
	if mimeType := mime.TypeByExtension(ext); mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}

	http.ServeFile(w, r, absPath)
}

// isPathAllowed 检查相对路径是否在声明的允许目录内
func (s *StaticResourceService) isPathAllowed(relativePath string, allowedDirs []string) bool {
	cleaned := filepath.Clean(relativePath)
	for _, dir := range allowedDirs {
		// 确保 allowedDir 也经过 Clean 处理
		cleanDir := filepath.Clean(dir)
		if strings.HasPrefix(cleaned, cleanDir) {
			return true
		}
	}
	return false
}

// ResolveURL 构建插件资源 URL（供主程序构建 FrontendExtensionConfig 时使用；cacheKey 为缓存键：buildId，未打标包为 version）
func (s *StaticResourceService) ResolveURL(publicId, cacheKey, relativePath string) string {
	return "/plugin/" + publicId + "/" + cacheKey + "/" + relativePath
}

// HasPlugin 检查插件是否已注册
func (s *StaticResourceService) HasPlugin(publicId string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.plugins[publicId]
	return ok
}

// 接口合规检查
var _ http.Handler = (*StaticResourceService)(nil)
