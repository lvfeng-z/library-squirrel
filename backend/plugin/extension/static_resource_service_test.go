package extension

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
)

// TestMain 初始化 nop logger——RegisterPlugin 会记 Infof，未初始化的 logger.Log 会 nil panic
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	os.Exit(m.Run())
}

// TestServeHTTPThreeSegmentURL 验证三段 URL 解析（/plugin/{publicId}/{cacheKey}/{relativePath}，
// publicId 即插件 id、占一段）与旧四段格式（author/id 展开两段）不再被接受
func TestServeHTTPThreeSegmentURL(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "views"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "views", "a.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewStaticResourceService()
	svc.RegisterPlugin("com.lvfeng.pixivSuite", root, "build-1")

	tests := []struct {
		name       string
		url        string
		wantStatus int
	}{
		{"三段新格式命中", "/plugin/com.lvfeng.pixivSuite/build-1/views/a.js", http.StatusOK},
		{"旧四段格式（author/id 两段）不再解析", "/plugin/lvfeng/com.lvfeng.pixivSuite/build-1/views/a.js", http.StatusNotFound},
		{"未注册插件", "/plugin/com.lvfeng.other/build-1/views/a.js", http.StatusNotFound},
		{"段数不足", "/plugin/com.lvfeng.pixivSuite/build-1", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			svc.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("GET %s = %d, want %d", tt.url, rec.Code, tt.wantStatus)
			}
		})
	}
}

// TestServeHTTPRootAsSiteRoot 插件根目录即静态站点根：目录内任意相对路径——根下直接文件、
// 任意深度子目录内的组件/图标/组件运行时自行取用的额外资源——均可经 /plugin/ 路由取到，
// 不存在需要声明的可服务目录集合
func TestServeHTTPRootAsSiteRoot(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"plugin.json":                `{"id":"com.lvfeng.pixivSuite"}`,
		"icon.png":                   "icon-bytes",
		"views/a.js":                 "console.log(1)",
		"views/sub/deep/btn.css":     ".btn{color:red}",
		"assets/img/icons/site.png":  "png-bytes",
		"extra/data/config.json":     `{"k":"v"}`,
		"dist/chunks/vendor.abc1.js": "chunk",
	}
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	svc := NewStaticResourceService()
	svc.RegisterPlugin("com.lvfeng.pixivSuite", root, "build-1")

	for rel, wantBody := range files {
		url := "/plugin/com.lvfeng.pixivSuite/build-1/" + rel
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		svc.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want %d", url, rec.Code, http.StatusOK)
			continue
		}
		if got := rec.Body.String(); got != wantBody {
			t.Errorf("GET %s body = %q, want %q", url, got, wantBody)
		}
	}
}

// TestServeHTTPRejectsPathEscape 穿越与插件根目录外路径仍被拒（404）：
// publicId→rootPath 路由隔离与 Clean + `..` 检查 + rootPath 前缀的逃逸防护是服务仅有的两道边界
func TestServeHTTPRejectsPathEscape(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "plugin-root")
	if err := os.MkdirAll(filepath.Join(root, "views"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "views", "a.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 插件根目录外的诱饵文件：穿越向量若逃逸成功会取到它
	outside := filepath.Join(base, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewStaticResourceService()
	svc.RegisterPlugin("com.lvfeng.pixivSuite", root, "build-1")

	tests := []struct {
		name       string
		url        string
		wantStatus int
	}{
		{"单点穿越上一级", "/plugin/com.lvfeng.pixivSuite/build-1/../secret.txt", http.StatusNotFound},
		{"穿越到根下文件再折返", "/plugin/com.lvfeng.pixivSuite/build-1/views/../../secret.txt", http.StatusNotFound},
		{"URL 编码的点段", "/plugin/com.lvfeng.pixivSuite/build-1/%2e%2e/secret.txt", http.StatusNotFound},
		// cacheKey 段位置的 `..` 不影响三段解析取对插件段；该 URL 到达底层文件服务时
		// 由 net/http 对含 `..` 的 URL path 直接拒绝（400），同样取不到文件
		{"中段穿越（cacheKey 段内）", "/plugin/com.lvfeng.pixivSuite/../build-1/views/a.js", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			svc.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("GET %s = %d, want %d", tt.url, rec.Code, tt.wantStatus)
			}
		})
	}
}

// TestManifestStaticResourcesKeyIgnored 清单 extensions 段内含 staticResources 键时，
// 普通 json.Unmarshal 忽略未知字段，manifest 解析不受影响（其余声明照常读出，旧清单可继续加载）
func TestManifestStaticResourcesKeyIgnored(t *testing.T) {
	raw := `{
		"id": "com.lvfeng.pixivSuite",
		"name": "pixiv 套件",
		"version": "1.0.0",
		"contractVersion": 9,
		"extensions": {
			"staticResources": {"directories": ["assets/", "views/"]},
			"frontendExtensions": [
				{
					"id": "card",
					"name": "站点入口卡片",
					"kind": "siteBrowserList",
					"content": {"icon": "assets/icon.png", "extensionId": "main"}
				}
			]
		}
	}`
	var manifest dto.PluginManifest
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		t.Fatalf("含 staticResources 键的清单解析失败: %v", err)
	}
	if manifest.ID != "com.lvfeng.pixivSuite" {
		t.Errorf("manifest.ID = %q, want %q", manifest.ID, "com.lvfeng.pixivSuite")
	}
	if manifest.Extensions == nil || len(manifest.Extensions.FrontendExtensions) != 1 {
		t.Fatalf("frontendExtensions 未按声明解析: %+v", manifest.Extensions)
	}
	ext := manifest.Extensions.FrontendExtensions[0]
	if ext.ID != "card" || ext.Kind != "siteBrowserList" {
		t.Errorf("前端扩展 = (%s, %s), want (card, siteBrowserList)", ext.ID, ext.Kind)
	}
}
