package extension

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// PluginAwareAssetHandler 组合前端静态资源与插件静态资源的 HTTP Handler
type PluginAwareAssetHandler struct {
	frontendHandler http.Handler
	pluginService   *StaticResourceService
}

// NewPluginAwareAssetHandler 创建组合 asset handler
func NewPluginAwareAssetHandler(frontendAssets fs.FS, pluginService *StaticResourceService) *PluginAwareAssetHandler {
	return &PluginAwareAssetHandler{
		frontendHandler: application.AssetFileServerFS(frontendAssets),
		pluginService:   pluginService,
	}
}

// ServeHTTP 路由请求：/plugin/ 路径交给插件资源服务，其余交给前端资源
func (h *PluginAwareAssetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/plugin/") {
		h.pluginService.ServeHTTP(w, r)
		return
	}
	h.frontendHandler.ServeHTTP(w, r)
}

// 接口合规检查
var _ http.Handler = (*PluginAwareAssetHandler)(nil)
