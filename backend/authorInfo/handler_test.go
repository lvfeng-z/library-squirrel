package authorInfo

import (
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
)

// TestConflictResponseEnvelopeShape 冲突响应的信封形态：外层成功、冲突标志与候选清单在数据载荷内、
// 引导文案作 msg 附注——失败信封不得携带数据载荷，冲突等非失败分支态一律住成功载荷
func TestConflictResponseEnvelopeShape(t *testing.T) {
	resp := &SiteAuthorFetchResponse{
		Conflicts: []*dto.SiteAuthorFetchConflict{
			{Conflict: true, SiteKey: "pixiv"},
		},
	}
	got := conflictResponse(resp)
	if !got.Success {
		t.Fatalf("冲突响应外层应为成功, 实际 success=false")
	}
	if got.Msg != siteAuthorFetchConflictMsg {
		t.Fatalf("冲突响应 msg 应为引导文案, 实际 %q", got.Msg)
	}
	if got.Data != resp {
		t.Fatalf("冲突响应数据载荷应原样交回, 实际 %+v", got.Data)
	}
	if len(got.Data.Conflicts) != 1 || !got.Data.Conflicts[0].Conflict {
		t.Fatalf("冲突标志与候选清单应在数据载荷内, 实际 %+v", got.Data.Conflicts)
	}
}
