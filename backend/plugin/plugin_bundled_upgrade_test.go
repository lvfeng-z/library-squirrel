package plugin

import (
	"database/sql"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/dto"
	entity "github.com/library-squirrel/backend/base/model/entity"
)

// TestNeedBundledUpgrade 验证捆绑插件升级判据：
// bundled 来源 + buildId（构建身份）不一致/缺失才重装；zip 未打标回落 version 比较；非 bundled 来源尊重用户手动版本
func TestNeedBundledUpgrade(t *testing.T) {
	bundled := sql.NullString{String: SourceBundled, Valid: true}
	tests := []struct {
		name        string
		source      sql.NullString
		buildID     sql.NullString
		version     sql.NullString
		zipBuildID  string
		zipVersion  string
		want        bool
	}{
		{"bundled 且 buildId 一致，跳过", bundled, ns("v1.0.0"), ns("1.0.0"), "v1.0.0", "1.0.0", false},
		{"bundled 且 buildId 变化，重装", bundled, ns("v1.0.0"), ns("1.0.0"), "v1.0.0-3-ga1b2c3d", "1.0.0", true},
		{"bundled 且已装 buildId 缺失（历史记录），重装对齐", bundled, sql.NullString{}, ns("1.0.0"), "v1.0.0", "1.0.0", true},
		{"bundled 且 zip 未打标，version 相同跳过", bundled, ns("v1.0.0"), ns("1.0.0"), "", "1.0.0", false},
		{"bundled 且 zip 未打标，version 变化重装", bundled, ns("v1.0.0"), ns("1.0.0"), "", "1.1.0", true},
		{"local 来源 buildId 变化，尊重手动版本", sql.NullString{String: SourceLocal, Valid: true}, ns("v1.0.0"), ns("1.0.0"), "v2.0.0", "2.0.0", false},
		{"来源缺失，跳过", sql.NullString{}, ns("v1.0.0"), ns("1.0.0"), "v2.0.0", "2.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := entity.NewPlugin()
			plugin.Source = tt.source
			plugin.BuildID = tt.buildID
			plugin.Version = tt.version
			installDTO := &domain.PluginInstallDTO{BuildID: tt.zipBuildID, Version: tt.zipVersion}
			if got := needBundledUpgrade(plugin, installDTO); got != tt.want {
				t.Errorf("needBundledUpgrade(source=%+v, buildID=%+v, zipBuildID=%q, zipVersion=%q) = %v, want %v",
					tt.source, tt.buildID, tt.zipBuildID, tt.zipVersion, got, tt.want)
			}
		})
	}
}

// ns 构造有效 NullString
func ns(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}
