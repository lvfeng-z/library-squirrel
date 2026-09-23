package plugin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/plugin/extension"
	"github.com/library-squirrel/backend/util"
	pluginsdktransport "github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// frontendExtensionsField 合法的前端扩展声明字段（不带外层花括号）——声明面不合格的清单同样要过
// 安装预检的「至少一条扩展点」门槛，故各形态清单都并列挂上它
const frontendExtensionsField = `"frontendExtensions":[{"id":"v1","name":"视图","kind":"view","content":{"contentType":"code","source":"1"}}]`

// declarationManifestWith 合法清单骨架 + 给定 extensions 段内容
func declarationManifestWith(publicId, extensions string) string {
	return `{"id":"` + publicId + `","name":"测试插件","version":"1.0.0","author":"tester",` +
		fmt.Sprintf(`"contractVersion":%d,`, pluginsdktransport.ContractVersion) +
		`"activation":{"type":1},"entryFile":"plugin.exe","extensions":` + extensions + `}`
}

// declarationManifestLegacy 顶层残留 capabilities 段（extensions 段本身合法）的清单
func declarationManifestLegacy(publicId string) string {
	return `{"id":"` + publicId + `","name":"测试插件","version":"1.0.0","author":"tester",` +
		fmt.Sprintf(`"contractVersion":%d,`, pluginsdktransport.ContractVersion) +
		`"activation":{"type":1},"entryFile":"plugin.exe","capabilities":["siteAuthorFetch","workOrderQuery"],` +
		`"extensions":{` + frontendExtensionsField + `}}`
}

// invalidDeclarationCases 声明面不合格形态矩阵（拒收矩阵的清单侧承载，安装期与加载期共用同一组样例）
func invalidDeclarationCases() []struct {
	name     string
	build    func(publicId string) string
	wantText string
} {
	return []struct {
		name     string
		build    func(publicId string) string
		wantText string
	}{
		{
			name: "siteAuthorFetch 空数组",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"siteAuthorFetch":[],`+frontendExtensionsField+`}`)
			},
			wantText: "siteAuthorFetch 为空数组",
		},
		{
			name: "siteAuthorFetch 条目缺 id",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"siteAuthorFetch":[{"name":"作者源","sites":["bilibili"]}],`+frontendExtensionsField+`}`)
			},
			wantText: "siteAuthorFetch[0] 条目缺 id",
		},
		{
			name: "siteAuthorFetch 条目 id 重复",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"siteAuthorFetch":[{"id":"main","name":"作者源","sites":["bilibili"]},{"id":"main","name":"备份源","sites":["pixiv"]}],`+frontendExtensionsField+`}`)
			},
			wantText: `含重复条目 id "main"`,
		},
		{
			name: "siteAuthorFetch 条目缺 name",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"siteAuthorFetch":[{"id":"main","sites":["bilibili"]}],`+frontendExtensionsField+`}`)
			},
			wantText: "siteAuthorFetch[main].name 为空",
		},
		{
			name: "siteAuthorFetch 条目缺 sites",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"siteAuthorFetch":[{"id":"main","name":"作者源"}],`+frontendExtensionsField+`}`)
			},
			wantText: "siteAuthorFetch[main].sites 为空",
		},
		{
			name: "siteAuthorFetch 未注册站点键",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"siteAuthorFetch":[{"id":"main","name":"作者源","sites":["nosite"]}],`+frontendExtensionsField+`}`)
			},
			wantText: `siteAuthorFetch[main].sites 含未注册站点键 "nosite"`,
		},
		{
			name: "workFetch 条目缺 name",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"workFetch":[{"id":"main","options":["workOrderQuery"]}],`+frontendExtensionsField+`}`)
			},
			wantText: "workFetch[main].name 为空",
		},
		{
			name: "未识别 options",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"workFetch":[{"id":"main","name":"主处理器","options":["workOrderQuer"]}],`+frontendExtensionsField+`}`)
			},
			wantText: `workFetch[main].options 含未识别的可选方法组 "workOrderQuer"`,
		},
		{
			name: "urlPatterns 空数组",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"workFetch":[{"id":"main","name":"主处理器","urlPatterns":[]}],`+frontendExtensionsField+`}`)
			},
			wantText: "workFetch[main].urlPatterns 为空数组",
		},
		{
			name: "urlPatterns 坏正则",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"workFetch":[{"id":"main","name":"主处理器","urlPatterns":["[invalid("]}],`+frontendExtensionsField+`}`)
			},
			wantText: `含无法编译的正则 "[invalid("`,
		},
		{
			name: "siteBrowsers 条目缺 name",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"siteBrowsers":[{"id":"main"}],`+frontendExtensionsField+`}`)
			},
			wantText: "siteBrowsers[main].name 为空",
		},
		{
			name:     "残留顶层 capabilities",
			build:    declarationManifestLegacy,
			wantText: "顶层 capabilities 段不在声明面内",
		},
		{
			name: "extensions 内 settings 键",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"settings":[{"key":"a","type":"string","title":"甲"}],`+frontendExtensionsField+`}`)
			},
			wantText: "settings 须住清单根级",
		},
	}
}

// validDeclarationManifest 声明面合格的清单（含 workFetch 条目（带 urlPatterns）、
// siteAuthorFetch 条目与根级 settings 段）
func validDeclarationManifest(publicId string) string {
	return `{"id":"` + publicId + `","name":"测试插件","version":"1.0.0","author":"tester",` +
		fmt.Sprintf(`"contractVersion":%d,`, pluginsdktransport.ContractVersion) +
		`"activation":{"type":1},"entryFile":"plugin.exe",` +
		`"settings":[{"key":"apiToken","type":"string","title":"访问令牌","default":"anon","encrypted":true}],` +
		`"extensions":{"workFetch":[{"id":"main","name":"主处理器","options":["workOrderQuery"],"urlPatterns":["^https://www\\.bilibili\\.com/video/"]}],` +
		`"siteAuthorFetch":[{"id":"main","name":"作者源","sites":["bilibili"]}],` + frontendExtensionsField + `}}`
}

// TestInstallFromPathRejectsInvalidDeclarations 安装期闸门：不合格形态逐种拒收安装（安装失败且
// 点名不合格项与期望形态），且安装主体不落库——拒收发生在解压/建行之前
func TestInstallFromPathRejectsInvalidDeclarations(t *testing.T) {
	for i, tc := range invalidDeclarationCases() {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newCleanupTestService(t)
			ctx := context.Background()
			publicId := fmt.Sprintf("com.reject.install.case%d", i)

			_, _, err := svc.InstallFromPath(ctx, writePluginZip(t, tc.build(publicId)), true)
			if err == nil {
				t.Fatal("声明面不合格的插件包应拒收安装")
			}
			if !errors.Is(err, extension.ErrManifestDeclarationInvalid) {
				t.Errorf("拒收原因应携带哨兵错误 %v, 实际: %v", extension.ErrManifestDeclarationInvalid, err)
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Errorf("拒收原因应点名不合格项 %q, 实际: %v", tc.wantText, err)
			}
			row, rerr := svc.repo.GetByPublicId(ctx, publicId)
			if rerr != nil || row != nil {
				t.Errorf("拒收后不应留下安装行: row=%v err=%v", row, rerr)
			}
		})
	}
}

// TestInstallFromPathAcceptsValidDeclarations 反向控制：声明面合格的插件包照常安装
func TestInstallFromPathAcceptsValidDeclarations(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	const publicId = "com.accept.install"
	if _, _, err := svc.InstallFromPath(context.Background(), writePluginZip(t, validDeclarationManifest(publicId)), true); err != nil {
		t.Fatalf("合格声明应安装成功: %v", err)
	}
}

// pureSiteAuthorFetchManifest 仅声明站点作者拉取能力包的清单（无作品拉取/站点浏览器/前端扩展）
func pureSiteAuthorFetchManifest(publicId, entryFile string) string {
	return `{"id":"` + publicId + `","name":"拉取插件","version":"1.0.0","author":"tester",` +
		fmt.Sprintf(`"contractVersion":%d,`, pluginsdktransport.ContractVersion) +
		`"activation":{"type":1},"entryFile":"` + entryFile + `",` +
		`"extensions":{"siteAuthorFetch":[{"id":"main","name":"作者源","sites":["bilibili"]}]}}`
}

// TestInstallFromPathAcceptsPureSiteAuthorFetchPlugin 安装闸门把 siteAuthorFetch 数组计入扩展点与
// 运行时判定：纯站点作者拉取插件（无其他扩展点）可安装；其拉取 RPC 由插件进程承载，缺 entryFile
// 同样拒收
func TestInstallFromPathAcceptsPureSiteAuthorFetchPlugin(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	const publicId = "com.accept.fetchonly"
	if _, _, err := svc.InstallFromPath(context.Background(),
		writePluginZip(t, pureSiteAuthorFetchManifest(publicId, "plugin.exe")), true); err != nil {
		t.Fatalf("纯 siteAuthorFetch 插件应安装成功: %v", err)
	}

	_, _, err := svc.InstallFromPath(context.Background(),
		writePluginZip(t, pureSiteAuthorFetchManifest("com.reject.fetchonly.noentry", "")), true)
	if err == nil {
		t.Fatal("纯 siteAuthorFetch 插件缺 entryFile 应拒收")
	}
	if !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("缺 entryFile 拒收原因应为 ErrInvalidManifest, 实际: %v", err)
	}
}

// plantPluginWithManifestOnDisk 预置插件行与其安装目录下的 plugin.json（RootPath 指向
// <根目录>/plugin/package/<publicId>/1.0.0）。磁盘清单可直接指定，用以构造「旧版本装的、
// 新版本跑」——DB 行已存在，待读的清单不合规
func plantPluginWithManifestOnDisk(t *testing.T, svc *Service, publicId, manifestJSON string) *entity.Plugin {
	t.Helper()
	relRoot := filepath.Join(PluginPackageRoot, publicId, "1.0.0")
	absRoot := filepath.Join(util.RootPath(), relRoot)
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		t.Fatalf("建插件安装目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(absRoot, "plugin.json"), []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("写插件清单失败: %v", err)
	}
	row := entity.NewPlugin()
	row.PublicID = sql.NullString{String: publicId, Valid: true}
	row.Name = sql.NullString{String: "遗留插件", Valid: true}
	row.Version = sql.NullString{String: "1.0.0", Valid: true}
	row.RootPath = sql.NullString{String: relRoot, Valid: true}
	row.Trusted = sql.NullBool{Bool: true, Valid: true}
	if err := svc.repo.Create(context.Background(), row); err != nil {
		t.Fatalf("预置插件行失败: %v", err)
	}
	return row
}

// TestActivateSkipsPluginWithInvalidDeclarations 加载期闸门：不合格形态逐种在激活时被跳过
// （行存量不合规时读盘清单校验不合格即中止，插件保持未激活、原因落日志）
func TestActivateSkipsPluginWithInvalidDeclarations(t *testing.T) {
	for i, tc := range invalidDeclarationCases() {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newCleanupTestService(t)
			ctx := context.Background()
			publicId := fmt.Sprintf("com.reject.activate.case%d", i)
			row := plantPluginWithManifestOnDisk(t, svc, publicId, tc.build(publicId))

			err := svc.ActivatePlugin(ctx, row)
			if err == nil {
				t.Fatal("声明面不合格的行应跳过激活")
			}
			if !errors.Is(err, extension.ErrManifestDeclarationInvalid) {
				t.Errorf("跳过原因应携带哨兵错误 %v, 实际: %v", extension.ErrManifestDeclarationInvalid, err)
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Errorf("跳过原因应点名不合格项 %q, 实际: %v", tc.wantText, err)
			}
			state, lastErr := svc.lifecycle.lifecycleStatusOf(publicId)
			if state != lifecycleStateInactive {
				t.Errorf("跳过激活后插件应保持未激活, 实际状态: %s", state)
			}
			if lastErr == nil {
				t.Error("最近一次激活失败原因应记录在案（供状态面板如实快照）")
			}
		})
	}
}

// TestActivateRunsPluginWithValidDeclarations 反向控制：声明面合格的行照常激活（闸门不误伤合规行）
func TestActivateRunsPluginWithValidDeclarations(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	const publicId = "com.accept.activate"
	row := plantPluginWithManifestOnDisk(t, svc, publicId, validDeclarationManifest(publicId))

	if err := svc.ActivatePlugin(context.Background(), row); err != nil {
		t.Fatalf("合格声明应激活成功: %v", err)
	}
	if state, _ := svc.lifecycle.lifecycleStatusOf(publicId); state != lifecycleStateActive {
		t.Errorf("激活成功后状态应为 %s, 实际: %s", lifecycleStateActive, state)
	}
}
