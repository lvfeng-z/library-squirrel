package extension

import (
	"errors"
	"testing"

	pluginsdktransport "github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// TestValidateContractVersion 契约版本协商矩阵：currentContractVersion 跟随 SDK
// transport.ContractVersion，minSupportedContractVersion=12（扩展点正名的破坏性分界——作品拉取
// 扩展点 workFetch 的旧名退役，旧名标识符见 SDK 契约版本历史 transport/contract.go 第 12 条），
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
				t.Fatalf("契约版本 %d 期望错误 %v，实际 %v", tt.pluginVersion, tt.wantErr, err)
			}
		})
	}
}

// TestContractVersionBaseline 契约版本双点基线锚定：SDK 当前版本 15（宿主代理解析——
// HostService 线级新增 ResolveProxy RPC，插件出网代理三级检测〔显式 > 系统代理 > 环境变量〕
// 收归宿主，RPC 失败/超时/旧宿主 Unimplemented 时 SDK 降级为显式 > 环境变量；线级新增非破坏，
// 旧插件不调新 RPC 零行为变化），主程序最低支持版本 12（扩展点正名的破坏性分界——
// 代理解析不升 min：契约 12 插件在新宿主照常运行），任一侧升版须同步更新本基线
func TestContractVersionBaseline(t *testing.T) {
	if pluginsdktransport.ContractVersion != 15 {
		t.Errorf("SDK 当前契约版本 = %d, 期望 15", pluginsdktransport.ContractVersion)
	}
	if minSupportedContractVersion != 12 {
		t.Errorf("最低支持契约版本 = %d, 期望 12", minSupportedContractVersion)
	}
}
