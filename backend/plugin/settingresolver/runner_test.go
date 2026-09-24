// resolver 运行器与契约校验的行为测试：纯函数求值、死循环超时中断、越界条目
// 单条拒收、语法错、返回非对象，另覆盖体积上限、受限运行时（Date/Math.random
// 剥离）、输入隔离、编译缓存与调用方取消。
package settingresolver

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// assertEvalFailure 断言失败返回为 *EvalError 并给出其分类。
func assertEvalFailure(t *testing.T, err error) FailureKind {
	t.Helper()
	if err == nil {
		t.Fatal("求值失败未返回错误")
	}
	var evalErr *EvalError
	if !errors.As(err, &evalErr) {
		t.Fatalf("失败错误不是 *EvalError：%T（%v）", err, err)
	}
	return evalErr.Kind
}

// TestEvaluatePureFunction 验证契约脚本端到端求值：设置快照进、参与度条目出，
// 条目字段（含可选 reason）逐项还原；同一脚本第二次求值复用缓存的编译产物。
func TestEvaluatePureFunction(t *testing.T) {
	script := `"use strict";
const mkEntry = (point, id, active, reason) => ({ point: point, id: id, active: active, reason: reason });
function resolve(input) {
	const s = input.settings;
	const entries = [];
	if (s.hd === false) {
		entries.push(mkEntry("workFetch", "hd", false, "高质量模式已关闭"));
	}
	if (s.nsfw === true) {
		entries.push(mkEntry("siteBrowsers", "sfw", false));
	}
	entries.push(mkEntry("resourceTypes", "mymodel", true));
	return { version: 1, entries: entries };
}
`
	runner := NewRunner()
	in := Input{Settings: map[string]interface{}{"hd": false, "nsfw": true}}

	res, err := runner.Evaluate(context.Background(), script, in)
	if err != nil {
		t.Fatalf("纯函数求值失败：%v", err)
	}
	want := []Entry{
		{Point: PointWorkFetch, ID: "hd", Active: false, Reason: "高质量模式已关闭"},
		{Point: PointSiteBrowsers, ID: "sfw", Active: false},
		{Point: PointResourceTypes, ID: "mymodel", Active: true},
	}
	if !reflect.DeepEqual(res.Entries, want) {
		t.Fatalf("条目集合不符：\n期望 %+v\n实际 %+v", want, res.Entries)
	}
	if len(res.Rejected) != 0 {
		t.Fatalf("合法输出不应有拒收条目，实际 %+v", res.Rejected)
	}

	// 第二次求值（同脚本内容）：命中编译缓存，结果一致
	res2, err := runner.Evaluate(context.Background(), script, in)
	if err != nil {
		t.Fatalf("第二次求值失败：%v", err)
	}
	if !reflect.DeepEqual(res2.Entries, want) {
		t.Fatalf("第二次求值条目集合不符：\n期望 %+v\n实际 %+v", want, res2.Entries)
	}
	runner.mu.Lock()
	cached := len(runner.programs)
	runner.mu.Unlock()
	if cached != 1 {
		t.Fatalf("同内容脚本应只编译缓存一份，缓存条目数 = %d", cached)
	}
}

// TestEvaluateEmptyEntriesSnapshot 验证快照语义的最小输出：空 entries 合法，
// 表示全部条目回到基线参与。
func TestEvaluateEmptyEntriesSnapshot(t *testing.T) {
	script := `function resolve(input) { return { version: 1, entries: [] }; }`
	res, err := NewRunner().Evaluate(context.Background(), script, Input{})
	if err != nil {
		t.Fatalf("空条目输出求值失败：%v", err)
	}
	if len(res.Entries) != 0 || len(res.Rejected) != 0 {
		t.Fatalf("空条目输出应产生空结果，实际 %+v / 拒收 %+v", res.Entries, res.Rejected)
	}
}

// TestEvaluateTimeoutOnInfiniteLoop 验证死循环脚本在超时定值后被中断，
// 并归入超时分类。
func TestEvaluateTimeoutOnInfiniteLoop(t *testing.T) {
	script := `function resolve(input) { while (true) {} }`
	_, err := NewRunner().Evaluate(context.Background(), script, Input{})
	if kind := assertEvalFailure(t, err); kind != FailureTimeout {
		t.Fatalf("死循环失败分类 = %s，期望 %s（%v）", kind, FailureTimeout, err)
	}
}

// TestEvaluateSyntaxError 验证不可解析脚本归入语法错分类，且失败产物不进编译缓存。
func TestEvaluateSyntaxError(t *testing.T) {
	runner := NewRunner()
	_, err := runner.Evaluate(context.Background(), `function resolve(input) {`, Input{})
	if kind := assertEvalFailure(t, err); kind != FailureSyntax {
		t.Fatalf("语法错分类 = %s，期望 %s（%v）", kind, FailureSyntax, err)
	}
	runner.mu.Lock()
	cached := len(runner.programs)
	runner.mu.Unlock()
	if cached != 0 {
		t.Fatalf("语法错的脚本不应进入编译缓存，缓存条目数 = %d", cached)
	}
}

// TestEvaluateReturnsNonObject 验证入口函数返回非对象值归入输出不合法分类。
func TestEvaluateReturnsNonObject(t *testing.T) {
	for name, script := range map[string]string{
		"字符串":       `function resolve(input) { return "ok"; }`,
		"undefined": `function resolve(input) { return; }`,
		"数组":        `function resolve(input) { return [1, 2]; }`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewRunner().Evaluate(context.Background(), script, Input{})
			if kind := assertEvalFailure(t, err); kind != FailureInvalidOutput {
				t.Fatalf("返回非对象分类 = %s，期望 %s（%v）", kind, FailureInvalidOutput, err)
			}
		})
	}
}

// TestEvaluateMissingEntryFunction 验证脚本未定义入口函数 resolve 时归入运行异常分类。
func TestEvaluateMissingEntryFunction(t *testing.T) {
	script := `var answer = 42;`
	_, err := NewRunner().Evaluate(context.Background(), script, Input{})
	if kind := assertEvalFailure(t, err); kind != FailureRuntime {
		t.Fatalf("入口缺失分类 = %s，期望 %s（%v）", kind, FailureRuntime, err)
	}
	var evalErr *EvalError
	if !errors.As(err, &evalErr) || !strings.Contains(evalErr.Message, EntryFuncName) {
		t.Fatalf("入口缺失错误信息应指明入口函数名 %s，实际：%v", EntryFuncName, err)
	}
}

// TestEvaluateRejectsOutOfBoundsEntry 验证越界/不合法条目单条拒收：其余条目照常
// 通过，拒收记录带原始下标与原因。
func TestEvaluateRejectsOutOfBoundsEntry(t *testing.T) {
	script := `function resolve(input) {
	return {
		version: 1,
		entries: [
			{ point: "workFetch", id: "main", active: false, reason: "用户关闭" },
			{ point: "bogusPoint", id: "x", active: true },
			{ point: "siteBrowsers", id: "sfw", active: "yes" },
			{ point: "frontendExtensions", active: true },
			{ point: "resourceTypes", id: "mymodel", active: true }
		]
	};
}`
	res, err := NewRunner().Evaluate(context.Background(), script, Input{})
	if err != nil {
		t.Fatalf("含越界条目的求值不应整体失败：%v", err)
	}

	wantEntries := []Entry{
		{Point: PointWorkFetch, ID: "main", Active: false, Reason: "用户关闭"},
		{Point: PointResourceTypes, ID: "mymodel", Active: true},
	}
	if !reflect.DeepEqual(res.Entries, wantEntries) {
		t.Fatalf("存活条目不符：\n期望 %+v\n实际 %+v", wantEntries, res.Entries)
	}
	if len(res.Rejected) != 3 {
		t.Fatalf("拒收条目数 = %d，期望 3（%+v）", len(res.Rejected), res.Rejected)
	}
	wantRejectedIndexes := []int{1, 2, 3}
	for i, rejected := range res.Rejected {
		if rejected.Index != wantRejectedIndexes[i] {
			t.Errorf("拒收条目原始下标 = %d，期望 %d", rejected.Index, wantRejectedIndexes[i])
		}
		if rejected.Reason == "" {
			t.Errorf("下标 %d 的拒收条目缺少原因", rejected.Index)
		}
	}
	if res.Rejected[0].Reason != `point 越界："bogusPoint"` {
		t.Errorf("越界 point 拒收原因不符：%q", res.Rejected[0].Reason)
	}
}

// TestValidateOutputStructuralFailures 验证输出整体的结构性不合法（非对象、
// version 缺失或非 1、entries 非数组）都归入输出不合法分类且不产生 Result。
func TestValidateOutputStructuralFailures(t *testing.T) {
	cases := []struct {
		name string
		raw  interface{}
	}{
		{"nil", nil},
		{"字符串", "nope"},
		{"无 version 字段", map[string]interface{}{"entries": []interface{}{}}},
		{"version 非数值", map[string]interface{}{"version": "1", "entries": []interface{}{}}},
		{"version 为 2", map[string]interface{}{"version": 2, "entries": []interface{}{}}},
		{"version 浮点 1.5", map[string]interface{}{"version": 1.5, "entries": []interface{}{}}},
		{"无 entries 字段", map[string]interface{}{"version": 1}},
		{"entries 非数组", map[string]interface{}{"version": 1, "entries": map[string]interface{}{}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := ValidateOutput(c.raw)
			if err == nil {
				t.Fatalf("结构性不合法输出不应通过校验，得到 %+v", res)
			}
			if kind := assertEvalFailure(t, err); kind != FailureInvalidOutput {
				t.Fatalf("分类 = %s，期望 %s", kind, FailureInvalidOutput)
			}
		})
	}
}

// TestValidateOutputEntryLevelFailures 逐条核验条目级拒收的原因归类：
// 非对象条目、point/id/active/reason 各字段缺失或形态错误。
func TestValidateOutputEntryLevelFailures(t *testing.T) {
	raw := map[string]interface{}{
		"version": float64(1),
		"entries": []interface{}{
			"not an object",
			map[string]interface{}{"id": "a", "active": true},                                    // 缺 point
			map[string]interface{}{"point": "workFetch", "active": true},                         // 缺 id
			map[string]interface{}{"point": "workFetch", "id": ""},                               // 空 id
			map[string]interface{}{"point": "workFetch", "id": "a"},                              // 缺 active
			map[string]interface{}{"point": "workFetch", "id": "a", "active": 1},                 // active 非布尔
			map[string]interface{}{"point": "workFetch", "id": "a", "active": true, "reason": 5}, // reason 非字符串
		},
	}
	res, err := ValidateOutput(raw)
	if err != nil {
		t.Fatalf("条目级不合法不应整体失败：%v", err)
	}
	if len(res.Entries) != 0 {
		t.Fatalf("全部条目不合法时应无存活条目，实际 %+v", res.Entries)
	}
	if len(res.Rejected) != 7 {
		t.Fatalf("拒收条目数 = %d，期望 7", len(res.Rejected))
	}
	for i, rejected := range res.Rejected {
		if rejected.Index != i {
			t.Errorf("拒收条目下标 = %d，期望 %d", rejected.Index, i)
		}
	}
}

// TestEvaluateRestrictedRuntime 验证受限运行时：Date 与 Math.random 已剥离——
// typeof 探针不可见、调用 Math.random 抛运行异常。
func TestEvaluateRestrictedRuntime(t *testing.T) {
	probeScript := `function resolve(input) {
	var stripped = (typeof Date === "undefined") && (typeof Math.random === "undefined");
	return { version: 1, entries: [{ point: "workFetch", id: stripped ? "stripped" : "present", active: true }] };
}`
	res, err := NewRunner().Evaluate(context.Background(), probeScript, Input{})
	if err != nil {
		t.Fatalf("探针脚本求值失败：%v", err)
	}
	if len(res.Entries) != 1 || res.Entries[0].ID != "stripped" {
		t.Fatalf("Date/Math.random 剥离探针结果不符：%+v", res.Entries)
	}

	throwScript := `function resolve(input) { return { version: 1, entries: [{ point: "workFetch", id: "" + Math.random(), active: true }] }; }`
	_, err = NewRunner().Evaluate(context.Background(), throwScript, Input{})
	if kind := assertEvalFailure(t, err); kind != FailureRuntime {
		t.Fatalf("调用已剥离的 Math.random 分类 = %s，期望 %s（%v）", kind, FailureRuntime, err)
	}
}

// TestEvaluateInputIsolation 验证两向隔离：脚本改写输入不泄回调用方数据（含嵌套
// 值），且每次求值在新运行时中进行——脚本内全局状态不跨求值保留。
func TestEvaluateInputIsolation(t *testing.T) {
	script := `var counter;
function resolve(input) {
	counter = (counter === undefined) ? 1 : counter + 1;
	input.settings.polluted = counter;
	input.settings.hd = "改写";
	input.settings.inner.deep = counter;
	return { version: 1, entries: [{ point: "workFetch", id: "c" + counter, active: true }] };
}`
	runner := NewRunner()
	mkInput := func() Input {
		return Input{Settings: map[string]interface{}{"hd": false, "inner": map[string]interface{}{"deep": 0}}}
	}

	for round := 1; round <= 2; round++ {
		in := mkInput()
		res, err := runner.Evaluate(context.Background(), script, in)
		if err != nil {
			t.Fatalf("第 %d 轮求值失败：%v", round, err)
		}
		if len(res.Entries) != 1 || res.Entries[0].ID != "c1" {
			t.Fatalf("第 %d 轮应在新运行时中计数重启（id=c1），实际 %+v", round, res.Entries)
		}
		if _, polluted := in.Settings["polluted"]; polluted {
			t.Fatalf("第 %d 轮脚本新增的键泄回调用方设置：%+v", round, in.Settings)
		}
		if in.Settings["hd"] != false {
			t.Fatalf("第 %d 轮脚本改写的设置值泄回调用方：%+v", round, in.Settings)
		}
		if innerDeep := in.Settings["inner"].(map[string]interface{})["deep"]; innerDeep != 0 {
			t.Fatalf("第 %d 轮脚本改写的嵌套值泄回调用方：%+v", round, in.Settings["inner"])
		}
	}
}

// TestEvaluateScriptSizeLimit 验证超过体积上限的脚本装载即拒收，归入体积超限分类。
func TestEvaluateScriptSizeLimit(t *testing.T) {
	script := strings.Repeat("//", DefaultScriptSizeLimit) // 2 字节/组，总量 ≥ 上限
	if len(script) <= DefaultScriptSizeLimit {
		t.Fatalf("构造的脚本 %d 字节未超过上限 %d", len(script), DefaultScriptSizeLimit)
	}
	_, err := NewRunner().Evaluate(context.Background(), script, Input{})
	if kind := assertEvalFailure(t, err); kind != FailureScriptTooLarge {
		t.Fatalf("超限脚本分类 = %s，期望 %s（%v）", kind, FailureScriptTooLarge, err)
	}
}

// TestEvaluateCanceledContext 验证调用方提前取消时原样返回 context 的取消错误，
// 不计入脚本失败分类。
func TestEvaluateCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewRunner().Evaluate(ctx, `function resolve(input) { return { version: 1, entries: [] }; }`, Input{})
	if err == nil {
		t.Fatal("已取消的 context 不应完成求值")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("应返回 context.Canceled，实际：%v", err)
	}
}
