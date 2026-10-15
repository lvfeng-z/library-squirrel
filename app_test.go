package main

import (
	"encoding/json"
	"testing"

	"github.com/library-squirrel/backend/base"
	"github.com/library-squirrel/backend/base/model/dto"
)

// menuDeclForIcon 构造带 icon 的 menu 前端扩展声明（viewId 叶子项形态）
func menuDeclForIcon(t *testing.T, icon string) dto.FrontendExtensionDeclaration {
	t.Helper()
	content, err := json.Marshal(map[string]any{"viewId": "workbench", "icon": icon})
	if err != nil {
		t.Fatalf("构造 menu content 失败: %v", err)
	}
	return dto.FrontendExtensionDeclaration{ID: "workbench-menu", Name: "菜单", Kind: "menu", Content: content}
}

// TestParseMenuContentHostIconRefPassthrough menu content.icon 的 ep: 宿主图标名引用须原样透传，
// 不拼接静态资源 URL（前端摄入层按名解析为 Element Plus 图标组件）
func TestParseMenuContentHostIconRefPassthrough(t *testing.T) {
	cfg := base.NewFrontendExtensionConfig()
	cfg.Kind = base.FrontendExtensionKindMenu
	if err := parseFrontendExtensionContent(menuDeclForIcon(t, "ep:MessageBox"), cfg, "com.lvfeng.localImport", "build1"); err != nil {
		t.Fatalf("解析 menu content 失败: %v", err)
	}
	if cfg.Icon != "ep:MessageBox" {
		t.Fatalf("ep: 名字引用应原样透传，得到 %q", cfg.Icon)
	}
}

// TestParseMenuContentIconPathResolvesToURL 图片相对路径仍解析为插件静态资源完整 URL（既有行为）
func TestParseMenuContentIconPathResolvesToURL(t *testing.T) {
	cfg := base.NewFrontendExtensionConfig()
	cfg.Kind = base.FrontendExtensionKindMenu
	if err := parseFrontendExtensionContent(menuDeclForIcon(t, "assets/icon.png"), cfg, "com.lvfeng.localImport", "build1"); err != nil {
		t.Fatalf("解析 menu content 失败: %v", err)
	}
	want := "/plugin/com.lvfeng.localImport/build1/assets/icon.png"
	if cfg.Icon != want {
		t.Fatalf("图片相对路径应解析为完整 URL %q，得到 %q", want, cfg.Icon)
	}
}

// TestParseMenuContentChildHostIconRefPassthrough 子菜单项的 ep: 名字引用同样透传（children 递归同路径）
func TestParseMenuContentChildHostIconRefPassthrough(t *testing.T) {
	childContentBytes, err := json.Marshal(map[string]any{"viewId": "child-view", "icon": "ep:MessageBox"})
	if err != nil {
		t.Fatalf("构造子项 content 失败: %v", err)
	}
	childContent := json.RawMessage(childContentBytes)
	parentContent, err := json.Marshal(map[string]any{
		"children": []map[string]any{{
			"id": "child-menu", "name": "子菜单", "kind": "menu", "content": childContent,
		}},
	})
	if err != nil {
		t.Fatalf("构造父项 content 失败: %v", err)
	}
	decl := dto.FrontendExtensionDeclaration{ID: "parent-menu", Name: "父菜单", Kind: "menu", Content: parentContent}
	cfg := base.NewFrontendExtensionConfig()
	cfg.Kind = base.FrontendExtensionKindMenu
	if err := parseFrontendExtensionContent(decl, cfg, "com.lvfeng.localImport", "build1"); err != nil {
		t.Fatalf("解析 menu content 失败: %v", err)
	}
	if len(cfg.Children) != 1 {
		t.Fatalf("应解析出 1 个子菜单项，得到 %d", len(cfg.Children))
	}
	if cfg.Children[0].Icon != "ep:MessageBox" {
		t.Fatalf("子项 ep: 名字引用应原样透传，得到 %q", cfg.Children[0].Icon)
	}
}
