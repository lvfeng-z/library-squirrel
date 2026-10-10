package extension

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	pluginsdktransport "github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// TestValidateContractVersion 契约版本协商矩阵：currentContractVersion 跟随 SDK
// transport.ContractVersion，minSupportedContractVersion=12（扩展点正名的破坏性分界——作品拉取
// 扩展点 workFetch 的旧名退役，旧名标识符见 SDK 契约版本史 transport.ContractHistory 第 12 条），
// 未声明（=0）视作低于 minSupported 拒载。
func TestValidateContractVersion(t *testing.T) {
	tests := []struct {
		name          string
		pluginVersion int
		wantErr       error
	}{
		{name: "等于当前版本放行", pluginVersion: pluginsdktransport.ContractVersion, wantErr: nil},
		{name: "低于最低支持版本拒绝", pluginVersion: minSupportedContractVersion - 1, wantErr: ErrPluginContractTooOld},
		{name: "等于最低支持版本放行", pluginVersion: minSupportedContractVersion, wantErr: nil},
		{name: "高于当前版本拒绝", pluginVersion: pluginsdktransport.ContractVersion + 1, wantErr: ErrPluginContractTooNew},
		{name: "未声明等于零拒载", pluginVersion: 0, wantErr: ErrPluginContractTooOld},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContractVersion(tt.pluginVersion)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("契约版本 %d 应放行，实际拒绝: %v", tt.pluginVersion, err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("契约版本 %d 期望错误 %v，实际 %v", tt.pluginVersion, err, tt.wantErr)
			}
		})
	}
}

// TestContractVersionBaseline 契约版本双点基线锚定：SDK 当前版本 16（插件设置变更通知——
// PluginLifecycle 线级新增 SettingChanged 一元 RPC，宿主在 SaveSetting/ResetSetting 落库
// 成功后向已激活插件异步推送 {source,keys} 纯通知，插件经 SDK WithSettingChangeHandler
// 处置函数感知；线级新增非破坏，旧插件内嵌 Unimplemented 兜底、宿主静默降级零行为变化），
// 主程序最低支持版本 12（扩展点正名的破坏性分界——设置通知不升 min：契约 12 插件在新宿主
// 照常运行），任一侧升版须同步更新本基线
func TestContractVersionBaseline(t *testing.T) {
	if pluginsdktransport.ContractVersion != 16 {
		t.Errorf("SDK 当前契约版本 = %d, 期望 16", pluginsdktransport.ContractVersion)
	}
	if minSupportedContractVersion != 12 {
		t.Errorf("最低支持契约版本 = %d, 期望 12", minSupportedContractVersion)
	}
}

// dev-guide 版本史生成区块的边界标记（与 doc/plugin-dev-guide.md「契约版本协商」节内的
// 标记对一一对应，缺一或乱序即校验失败）
const (
	devGuideContractHistoryBegin = "<!-- contract-history:begin -->"
	devGuideContractHistoryEnd   = "<!-- contract-history:end -->"
)

// renderContractHistory 按 dev-guide 版本史区块格式渲染 SDK 契约版本史（单一源）：
// 每条目一行 "- N — Summary"
func renderContractHistory() string {
	lines := make([]string, 0, len(pluginsdktransport.ContractHistory))
	for _, e := range pluginsdktransport.ContractHistory {
		lines = append(lines, fmt.Sprintf("- %d — %s", e.Number, e.Summary))
	}
	return strings.Join(lines, "\n")
}

// TestDevGuideContractHistorySync 校验 dev-guide「契约版本协商」的版本史区块与 SDK
// transport.ContractHistory（单一源）一致：标记对之间的内容必须逐条等于渲染结果。
// 失配时输出期望全文——按其整段替换文档中 begin/end 标记间的内容即可。
func TestDevGuideContractHistorySync(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	guidePath := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "doc", "plugin-dev-guide.md")
	raw, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("读取 dev-guide 失败（%s）: %v", guidePath, err)
	}
	lines := strings.Split(string(raw), "\n")
	begin, end := -1, -1
	for i, l := range lines {
		if strings.Contains(l, devGuideContractHistoryBegin) {
			begin = i
		}
		if strings.Contains(l, devGuideContractHistoryEnd) {
			end = i
		}
	}
	if begin < 0 || end < 0 || end <= begin {
		t.Fatalf("dev-guide 缺少成对的版本史区块标记（%s / %s）或顺序异常：begin=%d end=%d",
			devGuideContractHistoryBegin, devGuideContractHistoryEnd, begin, end)
	}
	got := strings.TrimSpace(strings.Join(lines[begin+1:end], "\n"))
	want := renderContractHistory()
	if got != want {
		t.Errorf("dev-guide 版本史区块与 SDK transport.ContractHistory 失配：\n--- 文档现状 ---\n%s\n--- 期望（整段替换标记间内容）---\n%s\n", got, want)
	}
}
