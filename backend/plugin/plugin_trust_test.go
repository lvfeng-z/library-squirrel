package plugin

import (
	"database/sql"
	"testing"
)

// TestResolveReinstallContext 验证重装时的来源沿用与信任判定（bundled 强制信任，第三方沿用透传值，NULL 来源兜底 local）
func TestResolveReinstallContext(t *testing.T) {
	tests := []struct {
		name        string
		source      sql.NullString
		trusted     bool
		wantSource  string
		wantTrusted bool
	}{
		{"bundled 强制信任（忽略透传 false）", sql.NullString{String: SourceBundled, Valid: true}, false, SourceBundled, true},
		{"bundled 透传 true 仍信任", sql.NullString{String: SourceBundled, Valid: true}, true, SourceBundled, true},
		{"local 沿用透传 true", sql.NullString{String: SourceLocal, Valid: true}, true, SourceLocal, true},
		{"local 沿用透传 false", sql.NullString{String: SourceLocal, Valid: true}, false, SourceLocal, false},
		{"NULL 来源兜底 local + 透传 false", sql.NullString{}, false, SourceLocal, false},
		{"url 来源沿用 + 透传 true", sql.NullString{String: SourceURL, Valid: true}, true, SourceURL, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveReinstallContext(tt.source, tt.trusted)
			if got.Source != tt.wantSource || got.Trusted != tt.wantTrusted {
				t.Errorf("resolveReinstallContext(%+v, %v) = {Source:%s, Trusted:%v}, want {Source:%s, Trusted:%v}",
					tt.source, tt.trusted, got.Source, got.Trusted, tt.wantSource, tt.wantTrusted)
			}
		})
	}
}
