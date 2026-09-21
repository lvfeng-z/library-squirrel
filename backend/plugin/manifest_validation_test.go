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
			name: "缺 sites",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"siteAuthorFetch":{},`+frontendExtensionsField+`}`)
			},
			wantText: "siteAuthorFetch.sites 为空",
		},
		{
			name: "空 sites",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"siteAuthorFetch":{"sites":[]},`+frontendExtensionsField+`}`)
			},
			wantText: "siteAuthorFetch.sites 为空",
		},
		{
			name: "未注册站点键",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"siteAuthorFetch":{"sites":["nosite"]},`+frontendExtensionsField+`}`)
			},
			wantText: `未注册站点键 "nosite"`,
		},
		{
			name: "未识别 options",
			build: func(publicId string) string {
				return declarationManifestWith(publicId, `{"taskHandlers":[{"id":"main","options":["workOrderQuer"]}],`+frontendExtensionsField+`}`)
			},
			wantText: `taskHandlers[main].options 含未识别的可选方法组 "workOrderQuer"`,
		},
		{
			name:     "残留顶层 capabilities",
			build:    declarationManifestLegacy,
			wantText: "顶层 capabilities 段不在声明面内",
		},
	}
}

// validDeclarationManifest 声明面合格的清单（含 taskHandlers 条目与 siteAuthorFetch 段）
func validDeclarationManifest(publicId string) string {
	return declarationManifestWith(publicId,
		`{"taskHandlers":[{"id":"main","name":"主处理器","options":["workOrderQuery"]}],`+
			`"siteAuthorFetch":{"sites":["bilibili"]},`+frontendExtensionsField+`}`)
}

// TestInstallFromPathRejectsInvalidDeclarations 安装期闸门：五种不合格形态逐种拒收安装（安装失败且
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

// TestActivateSkipsPluginWithInvalidDeclarations 加载期闸门：五种不合格形态逐种在激活时被跳过
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
