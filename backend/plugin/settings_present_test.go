package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/util"
	pluginsdktransport "github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// settingsPresentManifest 根级 settingsPage + settingsPresent 声明（空串 = 缺省该字段的旧清单
// 形态）；menuEntry 非空时在 frontendExtensions 追加一个 menu 条目（原样嵌入，供引用形态取用）
func settingsPresentManifest(publicId, settingsPage, settingsPresent, menuEntry string) string {
	fields := ""
	if settingsPage != "" {
		fields += `"settingsPage":"` + settingsPage + `",`
	}
	if settingsPresent != "" {
		fields += `"settingsPresent":"` + settingsPresent + `",`
	}
	menu := ""
	if menuEntry != "" {
		menu = "," + menuEntry
	}
	return `{"id":"` + publicId + `","name":"设置页呈现插件","version":"1.0.0","author":"tester",` +
		fmt.Sprintf(`"contractVersion":%d,`, pluginsdktransport.ContractVersion) +
		fields +
		`"activation":{"type":1},"entryFile":"plugin.exe","extensions":{"frontendExtensions":[` +
		`{"id":"v1","name":"设置页","kind":"view","content":{"contentType":"code","source":"1"}}` + menu + `]}}`
}

// menuLeafViewV1 叶子 menu 条目（viewId 指向 v1）
const menuLeafViewV1 = `{"id":"m1","name":"菜单","kind":"menu","content":{"viewId":"v1"}}`

// menuParentChildViewV1 父 menu 条目，对 v1 的引用经子项叶子落在 children 内
const menuParentChildViewV1 = `{"id":"m0","name":"菜单组","kind":"menu","content":{"children":[` +
	`{"id":"m1","name":"子菜单","kind":"menu","content":{"viewId":"v1"}}]}}`

// TestInstallFromPathValidatesSettingsPresent settingsPresent 安装期闸门：合法两值与缺省字段
// 照常安装；非法枚举、字段在场而 settingsPage 缺席、dialog 呈现条目被 menu 条目引用（含子项
// 引用形态）逐种拒收（点名不合格项且不落库）；menu 引用 route 呈现条目不受限（常规联动形态）
func TestInstallFromPathValidatesSettingsPresent(t *testing.T) {
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
			name:     "合法 route 呈现",
			publicId: "com.settingspresent.route",
			manifest: settingsPresentManifest("com.settingspresent.route", "v1", "route", ""),
		},
		{
			name:     "合法 dialog 呈现",
			publicId: "com.settingspresent.dialog",
			manifest: settingsPresentManifest("com.settingspresent.dialog", "v1", "dialog", ""),
		},
		{
			name:     "缺省字段（旧清单零值兼容）",
			publicId: "com.settingspresent.legacy",
			manifest: settingsPresentManifest("com.settingspresent.legacy", "v1", "", ""),
		},
		{
			name:     "非法枚举值",
			publicId: "com.settingspresent.badenum",
			manifest: settingsPresentManifest("com.settingspresent.badenum", "v1", "popup", ""),
			wantErr:  true,
			wantText: `settingsPresent 取值 "popup" 非法`,
		},
		{
			name:     "字段在场而 settingsPage 缺失",
			publicId: "com.settingspresent.nopage",
			manifest: settingsPresentManifest("com.settingspresent.nopage", "", "dialog", ""),
			wantErr:  true,
			wantText: "settingsPresent 在场但 settingsPage 为空",
		},
		{
			name:     "menu 叶子项引用 dialog 呈现条目",
			publicId: "com.settingspresent.menuleaf",
			manifest: settingsPresentManifest("com.settingspresent.menuleaf", "v1", "dialog", menuLeafViewV1),
			wantErr:  true,
			wantText: "被 menu 条目引用",
		},
		{
			name:     "menu 子项引用 dialog 呈现条目",
			publicId: "com.settingspresent.menuchild",
			manifest: settingsPresentManifest("com.settingspresent.menuchild", "v1", "dialog", menuParentChildViewV1),
			wantErr:  true,
			wantText: "被 menu 条目引用",
		},
		{
			name:     "menu 引用 route 呈现条目不受限",
			publicId: "com.settingspresent.menuroute",
			manifest: settingsPresentManifest("com.settingspresent.menuroute", "v1", "route", menuLeafViewV1),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := svc.InstallFromPath(ctx, writePluginZip(t, tc.manifest), true)
			if tc.wantErr {
				if err == nil {
					t.Fatal("settingsPresent 声明不合格的插件包应拒收安装")
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

// TestGetPluginStatusReturnsSettingsPresent settingsPresent 为清单静态声明面：未激活插件同样
// 返回（判据现读安装目录清单）；显式两值原样透出，未声明（旧清单）归一为缺省 route
func TestGetPluginStatusReturnsSettingsPresent(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	ctx := context.Background()

	cases := []struct {
		name      string
		publicId  string
		manifest  string
		wantValue string
	}{
		{
			name:      "dialog 声明（未激活同样返回）",
			publicId:  "com.status.settingspresent.dialog",
			manifest:  settingsPresentManifest("com.status.settingspresent.dialog", "v1", "dialog", ""),
			wantValue: "dialog",
		},
		{
			name:      "route 显式声明",
			publicId:  "com.status.settingspresent.route",
			manifest:  settingsPresentManifest("com.status.settingspresent.route", "v1", "route", ""),
			wantValue: "route",
		},
		{
			name:      "未声明归一为缺省 route（旧清单零值兼容）",
			publicId:  "com.status.settingspresent.legacy",
			manifest:  settingsPresentManifest("com.status.settingspresent.legacy", "v1", "", ""),
			wantValue: "route",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plantPluginWithManifestOnDisk(t, svc, tc.publicId, tc.manifest)
			t.Cleanup(func() {
				_ = os.RemoveAll(filepath.Join(util.RootPath(), PluginPackageRoot, tc.publicId))
			})
			status, err := svc.GetPluginStatus(ctx, tc.publicId)
			if err != nil {
				t.Fatalf("获取插件状态失败: %v", err)
			}
			if status.SettingsPresent != tc.wantValue {
				t.Errorf("settingsPresent 应为 %q, 实际: %q", tc.wantValue, status.SettingsPresent)
			}
			if status.SettingsPageExtensionId != "v1" {
				t.Errorf("settingsPageExtensionId 应同源返回 v1, 实际: %q", status.SettingsPageExtensionId)
			}
		})
	}
}
