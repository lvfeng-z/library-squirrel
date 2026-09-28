package assetserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// withObservedLog 临时把全局 logger.Log 换成 observer 支撑的实例，返回日志采集器；
// 测试结束后恢复原实例（logger.Log 为包级 var，logger.go:20）
func withObservedLog(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zapcore.InfoLevel)
	old := logger.Log
	logger.Log = zap.New(core).Sugar()
	t.Cleanup(func() { logger.Log = old })
	return logs
}

func countWorkDirLogs(logs *observer.ObservedLogs) int {
	// 存量风格：SugaredLogger.Info(msg, zap.Field) 把 Field 以 fmt 参数拼进消息
	// （实际消息为「存储工作目录已设置{dir 15 0 ... <nil>}」），故用片段匹配
	return logs.FilterMessageSnippet("存储工作目录已设置").Len()
}

// TestSetWorkDirLogsOnlyOnChange 锚定日志语义：目录实际变化时记一条，同值刷新静默
// （afterSave 在每次设置保存/重置后都会同值刷新快照——历史缺陷：无差别记日志导致
// 切换任意设置开关都刷「存储工作目录已设置」）
func TestSetWorkDirLogsOnlyOnChange(t *testing.T) {
	logs := withObservedLog(t)
	h := NewStoreFileHandler(nil)

	h.SetWorkDir(`D:\bucket`)
	if got := countWorkDirLogs(logs); got != 1 {
		t.Fatalf("首次设置（空→有值）应记 1 条日志，实际 %d 条", got)
	}

	// 同值刷新：afterSave 每次保存后的快照刷新路径，不得再记日志
	h.SetWorkDir(`D:\bucket`)
	h.SetWorkDir(`D:\bucket`)
	if got := countWorkDirLogs(logs); got != 1 {
		t.Fatalf("同值刷新不应新增日志，实际累计 %d 条", got)
	}

	// 实际变化：再记一条
	h.SetWorkDir(`E:\other`)
	if got := countWorkDirLogs(logs); got != 2 {
		t.Fatalf("目录变化应新增 1 条日志（累计 2），实际 %d 条", got)
	}
}

// TestSetWorkDirAppliesToServing 功能冒烟：日志语义调整不得影响工作目录生效（设值后 /store/ 能服务文件）
func TestSetWorkDirAppliesToServing(t *testing.T) {
	withObservedLog(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewStoreFileHandler(nil)
	h.SetWorkDir(dir)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/store/a.txt", nil))
	if w.Code != http.StatusOK || w.Body.String() != "hello" {
		t.Fatalf("设置工作目录后应正常服务文件，got status=%d body=%q", w.Code, w.Body.String())
	}
}
