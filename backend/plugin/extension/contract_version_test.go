package extension

import (
	"errors"
	"testing"

	pluginsdktransport "github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// TestValidateContractVersion 契约版本协商矩阵：currentContractVersion 跟随 SDK
// transport.ContractVersion，minSupportedContractVersion=10（清单 settings 段位置改至根级的
// 破坏性变更分界），未声明（=0）视作低于 minSupported 拒载。
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

// TestContractVersionBaseline 契约版本双点基线锚定：SDK 当前版本与主程序最低支持版本均为 10
// （清单 settings 段位置改至根级的破坏性变更分界），任一侧升版须同步更新本基线
func TestContractVersionBaseline(t *testing.T) {
	if pluginsdktransport.ContractVersion != 10 {
		t.Errorf("SDK 当前契约版本 = %d, 期望 10", pluginsdktransport.ContractVersion)
	}
	if minSupportedContractVersion != 10 {
		t.Errorf("最低支持契约版本 = %d, 期望 10", minSupportedContractVersion)
	}
}
