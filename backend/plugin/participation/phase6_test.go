package participation

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/plugin/settingresolver"
)

// TestPhase6BundledTestPluginPassesInstallGate 阶段6演示验收：捆绑 test 插件安装包
// （resources/bundled-plugins/test-plugin.zip，携带根级 settings + settingsResolver 声明与
// resolver.js）以安装闸门做 dry-run——清单与脚本字节均从 zip 内读出，与 service.go 安装
// 路径同源。安装包缺席（独立检出主仓未构建插件）时跳过，不作失败。
func TestPhase6BundledTestPluginPassesInstallGate(t *testing.T) {
	zipPath := filepath.Join("..", "..", "..", "resources", "bundled-plugins", "test-plugin.zip")
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Skipf("捆绑 test 插件安装包缺席，跳过：%v", err)
	}
	defer reader.Close()

	var manifest *dto.PluginManifest
	var scriptBytes []byte
	for _, f := range reader.File {
		switch f.Name {
		case "plugin.json":
			data, err := readZipEntry(t, f, "plugin.json")
			if err != nil {
				t.Fatal(err)
			}
			manifest = &dto.PluginManifest{}
			if err := json.Unmarshal(data, manifest); err != nil {
				t.Fatalf("zip 内 plugin.json 解析失败: %v", err)
			}
		case "resolver.js":
			data, err := readZipEntry(t, f, "resolver.js")
			if err != nil {
				t.Fatal(err)
			}
			scriptBytes = data
		}
	}
	if manifest == nil {
		t.Fatal("zip 内缺少 plugin.json")
	}
	if manifest.SettingsResolver == nil {
		t.Fatal("test 插件清单应声明 settingsResolver（阶段6演示改造）")
	}
	if scriptBytes == nil {
		t.Fatal("zip 内缺少 resolver.js（构建管线应打包脚本工件）")
	}
	if err := ValidateResolverForInstall(manifest, scriptBytes); err != nil {
		t.Fatalf("test 插件应通过安装闸门 dry-run: %v", err)
	}

	// 布尔开关关闭态直接求值：两条条目（resourceViewer 参与 + workFetch 候选）均停用
	result, err := settingresolver.NewRunner().Evaluate(context.Background(), string(scriptBytes),
		settingresolver.Input{Settings: map[string]interface{}{"enableParticipation": "false"}})
	if err != nil {
		t.Fatalf("关闭态求值失败: %v", err)
	}
	want := map[settingresolver.Point]map[string]bool{
		settingresolver.PointFrontendExtensions: {"test-article-viewer": false},
		settingresolver.PointWorkFetch:          {"main": false},
	}
	for _, e := range result.Entries {
		active, ok := want[e.Point][e.ID]
		if !ok {
			t.Errorf("关闭态不应输出条目 %s/%s", e.Point, e.ID)
			continue
		}
		if e.Active != active {
			t.Errorf("关闭态条目 %s/%s 参与度应为 %v, 实际 %v", e.Point, e.ID, active, e.Active)
		}
	}
	if len(result.Entries) != len(want) {
		t.Errorf("关闭态应输出 %d 条停用条目, 实际 %d 条", len(want), len(result.Entries))
	}

	// 开关打开态（默认值）：空参与度清单 = 全基线参与
	baseline, err := settingresolver.NewRunner().Evaluate(context.Background(), string(scriptBytes),
		settingresolver.Input{Settings: map[string]interface{}{"enableParticipation": "true"}})
	if err != nil {
		t.Fatalf("打开态求值失败: %v", err)
	}
	if len(baseline.Entries) != 0 || len(baseline.Rejected) != 0 {
		t.Errorf("打开态应输出空参与度清单（全基线参与）, 实际 entries=%d rejected=%d",
			len(baseline.Entries), len(baseline.Rejected))
	}
}

// readZipEntry 读取 zip 内单文件内容（安装路径 readPackageFile 的测试侧等价物）
func readZipEntry(t *testing.T, f *zip.File, name string) ([]byte, error) {
	t.Helper()
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
