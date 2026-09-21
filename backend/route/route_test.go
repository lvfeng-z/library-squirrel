package route

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// recordingAdapter 记录候选尝试序的测试适配器。未登记的候选默认按 NotApplicable 处理；
// 未登记失败原因的候选按 Succeeded 处理并返回 "result-<候选>"。
type recordingAdapter struct {
	candidates    []string
	candidatesErr error
	keys          map[string]string
	outcomes      map[string]Outcome
	errs          map[string]error
	invoked       []string
}

func (a *recordingAdapter) Candidates(_ context.Context) ([]string, error) {
	if a.candidatesErr != nil {
		return nil, a.candidatesErr
	}
	return a.candidates, nil
}

func (a *recordingAdapter) OrderKey(c string) string { return a.keys[c] }

func (a *recordingAdapter) Describe(c string) string { return "候选-" + c }

func (a *recordingAdapter) Invoke(_ context.Context, c string) (string, Outcome, error) {
	a.invoked = append(a.invoked, c)
	if err := a.errs[c]; err != nil {
		return "", a.outcomes[c], err
	}
	return "result-" + c, a.outcomes[c], nil
}

// permutations 返回 items 的全排列，用于以任意乱序输入检验排序确定性。
func permutations(items []string) [][]string {
	if len(items) <= 1 {
		return [][]string{append([]string(nil), items...)}
	}
	var all [][]string
	for i := range items {
		rest := make([]string, 0, len(items)-1)
		rest = append(rest, items[:i]...)
		rest = append(rest, items[i+1:]...)
		for _, p := range permutations(rest) {
			all = append(all, append([]string{items[i]}, p...))
		}
	}
	return all
}

func describeAll(candidates []string) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, "候选-"+c)
	}
	return out
}

// TestRouteOrderDeterminism 排序确定性：复合全键两两不同（无键碰撞）时，任意乱序输入经基座
// 排序后的尝试序恒为全键字典序，输出唯一确定。
func TestRouteOrderDeterminism(t *testing.T) {
	candidates := []string{"pixiv", "bilibili", "ehentai", "danbooru"}
	keys := map[string]string{
		"pixiv":    "com.a.pixiv#main",
		"bilibili": "com.a.bilibili#main",
		"ehentai":  "com.a.ehentai#main",
		"danbooru": "com.a.danbooru#main",
	}
	seen := make(map[string]bool, len(keys))
	for _, c := range candidates {
		if seen[keys[c]] {
			t.Fatalf("夹具键碰撞，无法断言排序唯一确定: %s", keys[c])
		}
		seen[keys[c]] = true
	}

	// 全键字典序：com.a.bilibili#main < com.a.danbooru#main < com.a.ehentai#main < com.a.pixiv#main
	want := []string{"bilibili", "danbooru", "ehentai", "pixiv"}
	perms := permutations(candidates)
	if len(perms) != 24 {
		t.Fatalf("夹具应产生 24 种乱序输入，实际 %d", len(perms))
	}

	for _, perm := range perms {
		a := &recordingAdapter{
			candidates: perm,
			keys:       keys,
			outcomes:   map[string]Outcome{}, // 空登记取 NotApplicable 零值：全部候选不适配，遍历完整候选序
		}
		_, err := Route(context.Background(), a)

		var notApplicable *AllNotApplicableError
		if !errors.As(err, &notApplicable) {
			t.Fatalf("输入 %v 期望全不适配收口，实际: %v", perm, err)
		}
		if !reflect.DeepEqual(a.invoked, want) {
			t.Fatalf("输入 %v 尝试序应恒为全键字典序 %v，实际 %v", perm, want, a.invoked)
		}
		if !reflect.DeepEqual(notApplicable.Candidates, describeAll(want)) {
			t.Fatalf("输入 %v 全不适配清单应为 %v，实际 %v", perm, describeAll(want), notApplicable.Candidates)
		}
	}
}

// TestRouteOrderStableOnEqualKeys 键相同的候选保持适配器给出的相对序（稳定排序）。
func TestRouteOrderStableOnEqualKeys(t *testing.T) {
	keys := map[string]string{"first": "com.a.plugin#ext-a", "second": "com.a.plugin#ext-a"}

	forward := &recordingAdapter{
		candidates: []string{"first", "second"},
		keys:       keys,
		outcomes:   map[string]Outcome{},
	}
	if _, err := Route(context.Background(), forward); err == nil {
		t.Fatal("全部候选不适配应返回收口错误")
	}
	if !reflect.DeepEqual(forward.invoked, []string{"first", "second"}) {
		t.Fatalf("键相同的候选应保持适配器给定序，实际 %v", forward.invoked)
	}

	reversed := &recordingAdapter{
		candidates: []string{"second", "first"},
		keys:       keys,
		outcomes:   map[string]Outcome{},
	}
	if _, err := Route(context.Background(), reversed); err == nil {
		t.Fatal("全部候选不适配应返回收口错误")
	}
	if !reflect.DeepEqual(reversed.invoked, []string{"second", "first"}) {
		t.Fatalf("适配器给定序反转后，键相同的候选相对序应随之反转，实际 %v", reversed.invoked)
	}
}

// TestRouteNotApplicableContinuesToNext 不适配者顺延：前序候选明示不归属后继续尝试下一候选。
func TestRouteNotApplicableContinuesToNext(t *testing.T) {
	a := &recordingAdapter{
		candidates: []string{"first", "second"},
		keys:       map[string]string{"first": "com.a#1", "second": "com.b#1"},
		outcomes:   map[string]Outcome{"first": NotApplicable, "second": Succeeded},
	}

	got, err := Route(context.Background(), a)
	if err != nil {
		t.Fatalf("顺延至成功候选后不应报错: %v", err)
	}
	if got != "result-second" {
		t.Fatalf("结果应来自成功候选 second，实际 %q", got)
	}
	if !reflect.DeepEqual(a.invoked, []string{"first", "second"}) {
		t.Fatalf("不适配候选应顺延至次候选，实际尝试序 %v", a.invoked)
	}
}

// TestRouteSucceededStopsAtFirstCandidate 首中即胜：首个成功候选之后不再调用任何候选。
func TestRouteSucceededStopsAtFirstCandidate(t *testing.T) {
	a := &recordingAdapter{
		candidates: []string{"first", "second"},
		keys:       map[string]string{"first": "com.a#1", "second": "com.b#1"},
		outcomes:   map[string]Outcome{"first": Succeeded, "second": Succeeded},
	}

	got, err := Route(context.Background(), a)
	if err != nil {
		t.Fatalf("成功路径不应报错: %v", err)
	}
	if got != "result-first" {
		t.Fatalf("结果应来自首个成功候选，实际 %q", got)
	}
	if !reflect.DeepEqual(a.invoked, []string{"first"}) {
		t.Fatalf("首中即胜后不应再调用后续候选，实际尝试序 %v", a.invoked)
	}
}

// TestRouteFailedTerminatesAndNamesCandidate 真失败即终止：错误点名失败候选、保留失败原因，
// 后续候选不再被调用。
func TestRouteFailedTerminatesAndNamesCandidate(t *testing.T) {
	reason := errors.New("网络超时")
	a := &recordingAdapter{
		candidates: []string{"first", "second", "third"},
		keys:       map[string]string{"first": "com.a#1", "second": "com.b#1", "third": "com.c#1"},
		outcomes:   map[string]Outcome{"first": NotApplicable, "second": Failed, "third": Succeeded},
		errs:       map[string]error{"second": reason},
	}

	got, err := Route(context.Background(), a)
	if err == nil {
		t.Fatal("真失败应返回错误")
	}
	if got != "" {
		t.Fatalf("失败路径应返回结果零值，实际 %q", got)
	}
	if !strings.Contains(err.Error(), "候选-second") {
		t.Fatalf("错误应点名失败候选，实际: %v", err)
	}
	if !strings.Contains(err.Error(), "网络超时") {
		t.Fatalf("错误应保留失败原因，实际: %v", err)
	}
	if !errors.Is(err, reason) {
		t.Fatalf("失败原因应可经 errors.Is 判别，实际: %v", err)
	}
	if !reflect.DeepEqual(a.invoked, []string{"first", "second"}) {
		t.Fatalf("真失败后不应再调用后续候选，实际尝试序 %v", a.invoked)
	}
}

// TestRouteUndefinedOutcomeTreatedAsFailure 三态之外的值按真失败终止，不被静默当作顺延。
func TestRouteUndefinedOutcomeTreatedAsFailure(t *testing.T) {
	a := &recordingAdapter{
		candidates: []string{"first", "second"},
		keys:       map[string]string{"first": "com.a#1", "second": "com.b#1"},
		outcomes:   map[string]Outcome{"first": Outcome(99), "second": Succeeded},
	}

	_, err := Route(context.Background(), a)
	if err == nil {
		t.Fatal("未定义态应终止路由并报错")
	}
	if !strings.Contains(err.Error(), "候选-first") {
		t.Fatalf("错误应点名该候选，实际: %v", err)
	}
	if !reflect.DeepEqual(a.invoked, []string{"first"}) {
		t.Fatalf("未定义态不应顺延至后续候选，实际尝试序 %v", a.invoked)
	}
}

// TestRouteNoCandidates 零候选收口：返回 ErrNoCandidates 且不调用任何候选。
func TestRouteNoCandidates(t *testing.T) {
	a := &recordingAdapter{
		candidates: []string{},
		keys:       map[string]string{},
		outcomes:   map[string]Outcome{},
	}

	_, err := Route(context.Background(), a)
	if !errors.Is(err, ErrNoCandidates) {
		t.Fatalf("零候选应返回 ErrNoCandidates，实际: %v", err)
	}
	var notApplicable *AllNotApplicableError
	if errors.As(err, &notApplicable) {
		t.Fatalf("零候选不应收口为全不适配: %v", err)
	}
	if len(a.invoked) != 0 {
		t.Fatalf("零候选不应调用任何候选，实际 %v", a.invoked)
	}
}

// TestRouteAllNotApplicable 全不适配收口：返回的类型携带已试候选点名清单（按尝试序），
// 与零候选收口可判别。
func TestRouteAllNotApplicable(t *testing.T) {
	a := &recordingAdapter{
		candidates: []string{"pixiv", "bilibili"},
		keys:       map[string]string{"pixiv": "com.a.pixiv#main", "bilibili": "com.a.bilibili#main"},
		outcomes:   map[string]Outcome{"pixiv": NotApplicable, "bilibili": NotApplicable},
	}

	_, err := Route(context.Background(), a)

	var notApplicable *AllNotApplicableError
	if !errors.As(err, &notApplicable) {
		t.Fatalf("全不适配应收口为 AllNotApplicableError，实际: %v", err)
	}
	if !reflect.DeepEqual(notApplicable.Candidates, []string{"候选-bilibili", "候选-pixiv"}) {
		t.Fatalf("清单应为已试候选点名且按尝试序，实际 %v", notApplicable.Candidates)
	}
	if !strings.Contains(err.Error(), "候选-bilibili") {
		t.Fatalf("错误文案应列出候选名，实际: %v", err)
	}
	if errors.Is(err, ErrNoCandidates) {
		t.Fatalf("全不适配不应与零候选收口混同: %v", err)
	}
}

// TestRouteCandidatesErrorPropagates 候选发现失败原样上抛，可经 errors.Is 判别。
func TestRouteCandidatesErrorPropagates(t *testing.T) {
	discoverErr := errors.New("插件清单不可用")
	a := &recordingAdapter{
		candidatesErr: discoverErr,
		keys:          map[string]string{},
		outcomes:      map[string]Outcome{},
	}

	_, err := Route(context.Background(), a)
	if !errors.Is(err, discoverErr) {
		t.Fatalf("候选发现失败应原样上抛，实际: %v", err)
	}
	if len(a.invoked) != 0 {
		t.Fatalf("发现失败不应调用任何候选，实际 %v", a.invoked)
	}
}

// TestRouteKeepsAdapterCandidateSliceIntact 基座不改动适配器持有的候选切片顺序。
func TestRouteKeepsAdapterCandidateSliceIntact(t *testing.T) {
	a := &recordingAdapter{
		candidates: []string{"pixiv", "bilibili", "ehentai"},
		keys: map[string]string{
			"pixiv":    "com.a.pixiv#main",
			"bilibili": "com.a.bilibili#main",
			"ehentai":  "com.a.ehentai#main",
		},
		outcomes: map[string]Outcome{},
	}

	if _, err := Route(context.Background(), a); err == nil {
		t.Fatal("全部候选不适配应返回收口错误")
	}
	if !reflect.DeepEqual(a.candidates, []string{"pixiv", "bilibili", "ehentai"}) {
		t.Fatalf("基座不得改写适配器候选切片，实际 %v", a.candidates)
	}
}
