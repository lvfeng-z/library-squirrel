package participation

import (
	"errors"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
)

// installManifestWith 安装校验测试清单：前端扩展 menu + 作品拉取 main + 设置声明 toggle
// （默认 on），settingsResolver 声明随形参
func installManifestWith(decl *dto.SettingsResolverDeclaration) *dto.PluginManifest {
	return manifestWith(decl, dto.SettingDeclaration{Key: "toggle", Default: "on"})
}

// validInstallScript 安装校验测试用合格脚本：默认值输入下输出空参与度清单（全基线）
const validInstallScript = `function resolve(input) {
	var entries = [];
	if (input.settings.toggle === "off") {
		entries.push({point: "frontendExtensions", id: "menu", active: false, reason: "closed"});
	}
	return {version: 1, entries: entries};
}`

// TestValidateResolverForInstallNoDeclaration 清单未声明 settingsResolver：安装闸门零行为
func TestValidateResolverForInstallNoDeclaration(t *testing.T) {
	if err := ValidateResolverForInstall(installManifestWith(nil), validInstallScriptBytes()); err != nil {
		t.Fatalf("未声明 resolver 应零行为（nil）, 实际: %v", err)
	}
	if err := ValidateResolverForInstall(nil, nil); err != nil {
		t.Fatalf("nil 清单应零行为, 实际: %v", err)
	}
}

func validInstallScriptBytes() []byte { return []byte(validInstallScript) }

// TestValidateResolverForInstallRejectMatrix 安装闸门拒收矩阵：不合格形态逐种被点名拒收
func TestValidateResolverForInstallRejectMatrix(t *testing.T) {
	decl := &dto.SettingsResolverDeclaration{Script: "resolver.js", ContractVersion: 1}
	cases := []struct {
		name     string
		decl     *dto.SettingsResolverDeclaration
		script   string // 空串 = 包内无该文件（scriptBytes 传 nil）
		wantText string
	}{
		{"script 含路径分隔符", &dto.SettingsResolverDeclaration{Script: "../resolver.js", ContractVersion: 1}, validInstallScript, "须为非空裸文件名"},
		{"script 为空串", &dto.SettingsResolverDeclaration{Script: "", ContractVersion: 1}, validInstallScript, "须为非空裸文件名"},
		{"script 目录捷径名", &dto.SettingsResolverDeclaration{Script: "..", ContractVersion: 1}, validInstallScript, "须为非空裸文件名"},
		{"contractVersion 不受支持", &dto.SettingsResolverDeclaration{Script: "resolver.js", ContractVersion: 2}, validInstallScript, "contractVersion 须为 1"},
		{"包内缺少脚本文件", decl, "", "缺少 resolver 脚本文件"},
		{"语法不可解析", decl, "function resolve(", "dry-run 失败"},
		{"返回值非对象", decl, "function resolve(input) { return 42; }", "dry-run 失败"},
		{"输出 version 缺失", decl, "function resolve(input) { return {entries: []}; }", "dry-run 失败"},
		{"point 越界（单条拒收）", decl, `function resolve(input) {
	return {version: 1, entries: [{point: "bogusPoint", id: "menu", active: false}]};
}`, "含不合格条目"},
		{"active 非布尔（单条拒收）", decl, `function resolve(input) {
	return {version: 1, entries: [{point: "frontendExtensions", id: "menu", active: "no"}]};
}`, "含不合格条目"},
		{"声明集外条目", decl, `function resolve(input) {
	return {version: 1, entries: [{point: "frontendExtensions", id: "ghost", active: false}]};
}`, "不在清单声明集内"},
		{"脚本超体积上限", decl, strings.Repeat("//", 40*1024), "dry-run 失败"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var scriptBytes []byte
			if tc.script != "" {
				scriptBytes = []byte(tc.script)
			}
			err := ValidateResolverForInstall(installManifestWith(tc.decl), scriptBytes)
			if err == nil {
				t.Fatal("不合格声明应拒收安装")
			}
			if !errors.Is(err, ErrResolverInvalid) {
				t.Errorf("拒收原因应携带哨兵错误 %v, 实际: %v", ErrResolverInvalid, err)
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Errorf("拒收原因应点名 %q, 实际: %v", tc.wantText, err)
			}
		})
	}
}

// TestValidateResolverForInstallAcceptsValid 合格声明与脚本通过安装闸门（dry-run 输出
// 空清单 = 全基线；含合法条目输出同过）
func TestValidateResolverForInstallAcceptsValid(t *testing.T) {
	decl := &dto.SettingsResolverDeclaration{Script: "resolver.js", ContractVersion: 1}
	if err := ValidateResolverForInstall(installManifestWith(decl), validInstallScriptBytes()); err != nil {
		t.Fatalf("合格 resolver 应通过安装闸门: %v", err)
	}

	// dry-run（默认值 on）输出含合法条目（声明集内、布尔参与度）同过
	scriptWithEntry := `function resolve(input) {
	return {version: 1, entries: [{point: "workFetch", id: "main", active: true}]};
}`
	if err := ValidateResolverForInstall(installManifestWith(decl), []byte(scriptWithEntry)); err != nil {
		t.Fatalf("声明集内条目输出应通过安装闸门: %v", err)
	}
}
