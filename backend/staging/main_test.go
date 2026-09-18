package staging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/library-squirrel/backend/base/logger"
)

// TestMain 测试环境挂 nop logger（生产 logger 由应用启动时 Init，测试中为 nil）。
func TestMain(m *testing.M) {
	if logger.Log == nil {
		logger.Log = zap.NewNop().Sugar()
	}
	os.Exit(m.Run())
}

// withLogObserver 以 observer logger 替换全局 logger.Log（供断言日志输出），测试结束还原。
func withLogObserver(t *testing.T, min zapcore.Level) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(min)
	old := logger.Log
	logger.Log = zap.New(core).Sugar()
	t.Cleanup(func() { logger.Log = old })
	return logs
}

// countWarnContaining 统计 Warn 级日志中含指定子串的条数。
func countWarnContaining(logs *observer.ObservedLogs, substr string) int {
	return countLevelContaining(logs, zapcore.WarnLevel, substr)
}

// countInfoContaining 统计 Info 级日志中含指定子串的条数。
func countInfoContaining(logs *observer.ObservedLogs, substr string) int {
	return countLevelContaining(logs, zapcore.InfoLevel, substr)
}

// countLevelContaining 统计指定级别日志中含指定子串的条数。
func countLevelContaining(logs *observer.ObservedLogs, level zapcore.Level, substr string) int {
	n := 0
	for _, entry := range logs.All() {
		if entry.Level == level && strings.Contains(entry.Message, substr) {
			n++
		}
	}
	return n
}

// aliveFor 构造按白名单判活的假归属谓词（生产为任务行存在性查询，由装配方注入）。
func aliveFor(aliveKeys ...string) ScopeOwnerAlive {
	set := make(map[string]bool, len(aliveKeys))
	for _, k := range aliveKeys {
		set[k] = true
	}
	return func(scopeKey string) bool { return set[scopeKey] }
}

// mkdirAllT 建目录（失败即 Fatal）并返回路径。
func mkdirAllT(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("创建目录失败 %s: %v", p, err)
	}
	return p
}

// writeTextT 写文本文件（失败即 Fatal）。
func writeTextT(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入文件失败 %s: %v", path, err)
	}
}

// pathExistsT 路径存在性检查（文件或目录，出错即 Fatal）。
func pathExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		t.Fatalf("检查路径失败 %s: %v", path, err)
	}
	return true
}
