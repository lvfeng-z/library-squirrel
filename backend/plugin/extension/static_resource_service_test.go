package extension

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/library-squirrel/backend/base/logger"
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
	svc.RegisterPlugin("com.lvfeng.pixivSuite", root, []string{"views/"}, "build-1")

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
