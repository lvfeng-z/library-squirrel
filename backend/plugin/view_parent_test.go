package plugin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	pluginsdktransport "github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// viewParentManifest frontendExtensions 条目原样拼接的最小清单（entries 为完整声明 JSON 串）
func viewParentManifest(publicId string, entries ...string) string {
	return `{"id":"` + publicId + `","name":"层级声明插件","version":"1.0.0","author":"tester",` +
		fmt.Sprintf(`"contractVersion":%d,`, pluginsdktransport.ContractVersion) +
		`"activation":{"type":1},"entryFile":"plugin.exe","extensions":{"frontendExtensions":[` +
		strings.Join(entries, ",") + `]}}`
}

// viewEntry kind=view 条目（parent 非空时写入 content）
func viewEntry(id, parent string) string {
	content := `{"contentType":"code","source":"1"`
	if parent != "" {
		content += `,"parent":"` + parent + `"`
	}
	return `{"id":"` + id + `","name":"页面` + id + `","kind":"view","content":` + content + `}}`
}

// embedWithParentEntry kind=embed 条目，content 内携带 parent（非 view 条目声明面错误形态）
func embedWithParentEntry(id string) string {
	return `{"id":"` + id + `","name":"嵌入` + id + `","kind":"embed",` +
		`"content":{"contentType":"code","source":"1","position":"work.toolbar","parent":"v1"}}`
}

// dialogEntry kind=dialog 条目（作 parent 的非 view 指向目标）
func dialogEntry(id string) string {
	return `{"id":"` + id + `","name":"弹窗` + id + `","kind":"dialog",` +
		`"content":{"contentType":"code","source":"1"}}`
}

// menuLeafViewId 叶子 menu 条目（viewId 指向给定 view 条目）
func menuLeafViewId(id, viewId string) string {
	return `{"id":"` + id + `","name":"菜单` + id + `","kind":"menu","content":{"viewId":"` + viewId + `"}}`
}

// menuParentChildViewId 父 menu 条目，对目标 view 的引用经子项叶子落在 children 内
func menuParentChildViewId(id, viewId string) string {
	return `{"id":"` + id + `","name":"菜单组` + id + `","kind":"menu","content":{"children":[` +
		menuLeafViewId("c"+id, viewId) + `]}}`
}

// TestInstallFromPathValidatesViewParent view 条目 parent 层级声明安装期闸门：合法形态
// （同插件指向 / 宿主路由名 / 混合链）照常安装；非 view 条目声明 parent、parent 指向本插件
// 非 view 条目、同插件链闭环（含自指）、带 parent 条目被 menu 引用（含子项引用形态）逐种
// 拒收（点名不合格项且不落库）；无 parent 条目被 menu 引用不受限（常规联动形态）
func TestInstallFromPathValidatesViewParent(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	ctx := context.Background()

	cases := []struct {
		name     string
		publicId string
		manifest string
		wantErr  bool
		wantText string
	}{
		{
			name:     "合法同插件指向",
			publicId: "com.viewparent.sameplugin",
			manifest: viewParentManifest("com.viewparent.sameplugin", viewEntry("v1", ""), viewEntry("v2", "v1")),
		},
		{
			name:     "合法宿主路由名",
			publicId: "com.viewparent.hostname",
			manifest: viewParentManifest("com.viewparent.hostname", viewEntry("v1", "pluginManage")),
		},
		{
			name:     "合法多级链（链内边 + 链尾宿主路由名）",
			publicId: "com.viewparent.chain",
			manifest: viewParentManifest("com.viewparent.chain",
				viewEntry("v1", "pluginManage"), viewEntry("v2", "v1"), viewEntry("v3", "v2")),
		},
		{
			name:     "无 parent 条目被 menu 引用不受限",
			publicId: "com.viewparent.menoroot",
			manifest: viewParentManifest("com.viewparent.menoroot",
				viewEntry("v1", ""), menuLeafViewId("m1", "v1")),
		},
		{
			name:     "非 view 条目（embed）content 内 parent 在场",
			publicId: "com.viewparent.embedparent",
			manifest: viewParentManifest("com.viewparent.embedparent",
				viewEntry("v1", ""), embedWithParentEntry("e1")),
			wantErr:  true,
			wantText: `仅对 kind=view 条目有意义`,
		},
		{
			name:     "非 view 条目（menu）content 内 parent 在场",
			publicId: "com.viewparent.menuparent",
			manifest: viewParentManifest("com.viewparent.menuparent",
				viewEntry("v1", ""), `{"id":"m1","name":"菜单","kind":"menu","content":{"viewId":"v1","parent":"v0"}}`),
			wantErr:  true,
			wantText: `仅对 kind=view 条目有意义`,
		},
		{
			name:     "parent 指向本插件非 view 条目",
			publicId: "com.viewparent.nonview",
			manifest: viewParentManifest("com.viewparent.nonview",
				viewEntry("v1", "d1"), dialogEntry("d1")),
			wantErr:  true,
			wantText: `同插件指向须指向 kind=view 的条目`,
		},
		{
			name:     "同插件链闭环",
			publicId: "com.viewparent.cycle",
			manifest: viewParentManifest("com.viewparent.cycle",
				viewEntry("v1", "v2"), viewEntry("v2", "v1")),
			wantErr:  true,
			wantText: `parent 链在本插件内闭环`,
		},
		{
			name:     "自指闭环",
			publicId: "com.viewparent.self",
			manifest: viewParentManifest("com.viewparent.self", viewEntry("v1", "v1")),
			wantErr:  true,
			wantText: `parent 链在本插件内闭环`,
		},
		{
			name:     "带 parent 条目被 menu 叶子项引用",
			publicId: "com.viewparent.menuleaf",
			manifest: viewParentManifest("com.viewparent.menuleaf",
				viewEntry("v1", ""), viewEntry("v2", "v1"), menuLeafViewId("m1", "v2")),
			wantErr:  true,
			wantText: `被 menu 条目引用`,
		},
		{
			name:     "带 parent 条目被 menu 子项引用",
			publicId: "com.viewparent.menuchild",
			manifest: viewParentManifest("com.viewparent.menuchild",
				viewEntry("v1", ""), viewEntry("v2", "v1"), menuParentChildViewId("m0", "v2")),
			wantErr:  true,
			wantText: `被 menu 条目引用`,
		},
		{
			name:     "宿主路由名形态的 parent 条目被 menu 引用同样拒载",
			publicId: "com.viewparent.hostmenu",
			manifest: viewParentManifest("com.viewparent.hostmenu",
				viewEntry("v1", "pluginManage"), menuLeafViewId("m1", "v1")),
			wantErr:  true,
			wantText: `被 menu 条目引用`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := svc.InstallFromPath(ctx, writePluginZip(t, tc.manifest), true)
			if tc.wantErr {
				if err == nil {
					t.Fatal("parent 层级声明不合格的插件包应拒收安装")
				}
				if !errors.Is(err, ErrInvalidManifest) {
					t.Errorf("拒收原因应为 ErrInvalidManifest, 实际: %v", err)
				}
				if !strings.Contains(err.Error(), tc.wantText) {
					t.Errorf("拒收原因应点名不合格项 %q, 实际: %v", tc.wantText, err)
				}
				row, rerr := svc.repo.GetByPublicId(ctx, tc.publicId)
				if rerr != nil || row != nil {
					t.Errorf("拒收后不应留下安装行: row=%v err=%v", row, rerr)
				}
				return
			}
			if err != nil {
				t.Fatalf("合格声明应安装成功: %v", err)
			}
		})
	}
}
