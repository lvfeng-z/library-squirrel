package reWorkSetWorkSet

import (
	"sort"
	"strings"
	"testing"
)

// TestBuildParentCaseExpression 验证按 parent_work_set_id 匹配的 CASE 表达式构造
// （环境 CGO 不可用无法跑内存 SQLite，CASE 串构造由此纯函数覆盖）
func TestBuildParentCaseExpression(t *testing.T) {
	t.Run("多父集", func(t *testing.T) {
		expr, parentIds := buildParentCaseExpression(map[int64]int{1: 0, 2: 5, 3: 10})
		if !strings.HasPrefix(expr, "CASE parent_work_set_id ") || !strings.HasSuffix(expr, " END") {
			t.Fatalf("CASE 表达式结构错误: %q", expr)
		}
		// map 迭代序不定，逐条断言 WHEN/THEN 片段存在
		for _, frag := range []string{"WHEN 1 THEN 0", "WHEN 2 THEN 5", "WHEN 3 THEN 10"} {
			if !strings.Contains(expr, frag) {
				t.Errorf("CASE 缺少片段 %q，完整: %q", frag, expr)
			}
		}
		// parentIds 须包含全部 key
		sort.Slice(parentIds, func(i, j int) bool { return parentIds[i] < parentIds[j] })
		if !equalInt64(parentIds, []int64{1, 2, 3}) {
			t.Errorf("parentIds = %v, want [1 2 3]", parentIds)
		}
	})

	t.Run("单父集", func(t *testing.T) {
		expr, parentIds := buildParentCaseExpression(map[int64]int{7: 3})
		want := "CASE parent_work_set_id WHEN 7 THEN 3 END"
		if expr != want {
			t.Fatalf("单父集 CASE = %q, want %q", expr, want)
		}
		if !equalInt64(parentIds, []int64{7}) {
			t.Errorf("parentIds = %v, want [7]", parentIds)
		}
	})

	t.Run("空 map 返回空 CASE 与空 ID", func(t *testing.T) {
		// 调用方 UpdateSiteSortOrdersForChild 对空 map 提前 return，不会走到此；
		// 这里仅验证函数本身不 panic 且返回结构合法
		expr, parentIds := buildParentCaseExpression(map[int64]int{})
		if expr != "CASE parent_work_set_id  END" {
			t.Fatalf("空 map CASE = %q", expr)
		}
		if len(parentIds) != 0 {
			t.Errorf("空 map parentIds = %v, want 空", parentIds)
		}
	})
}

func equalInt64(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
