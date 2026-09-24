// resolver 求值引擎（goja 沙箱）的能力锚定测试：超时中断实效、新鲜 Runtime +
// 预编译 Program 的求值成本、ES2015 常用语法支持面、剥离 Date/Math.random 的
// 受限构造、脚本体积预算下限。全部测试以 Spike 命名，供 -run Spike 定向执行。
package settingresolver

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

// 求值超时定值基线：求值超过该时长即向 Runtime 发 Interrupt
const spikeEvalTimeout = 500 * time.Millisecond

// 中断生效门槛：从定时器到点到求值返回的总时长上限
const spikeInterruptDeadline = 2 * time.Second

// 单次求值成本预算：预编译 Program 在新鲜 Runtime 上求值一次的时长上限
const spikeEvalBudget = 5 * time.Millisecond

// resolver 脚本体积上限基线（安装期拒收超限脚本）
const spikeScriptSizeLimit = 64 * 1024

// 中断定时器注入 Runtime.Interrupt 的原因值，供中断错误回读核对
const spikeInterruptReason = "求值超时"

// TestSpikeInterruptDeadline 验证死循环脚本能被超时中断实效打断：
// 定时器到点调用 Runtime.Interrupt，求值须在门槛时长内以中断错误返回。
func TestSpikeInterruptDeadline(t *testing.T) {
	vm := goja.New()
	timer := time.AfterFunc(spikeEvalTimeout, func() { vm.Interrupt(spikeInterruptReason) })
	defer timer.Stop()

	start := time.Now()
	_, err := vm.RunString("while (true) {}")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("死循环脚本未被中断，RunString 正常返回")
	}
	var intErr *goja.InterruptedError
	if !errors.As(err, &intErr) {
		t.Fatalf("求值错误不是中断错误：%v", err)
	}
	if intErr.Value() != spikeInterruptReason {
		t.Fatalf("中断错误携带的值 = %v，期望定时器注入的 %q", intErr.Value(), spikeInterruptReason)
	}
	if elapsed > spikeInterruptDeadline {
		t.Fatalf("中断生效过慢：总耗时 %v 超门槛 %v", elapsed, spikeInterruptDeadline)
	}
	t.Logf("超时定值 %v 后中断返回：总耗时 %v（中断额外延迟 %v）",
		spikeEvalTimeout, elapsed, elapsed-spikeEvalTimeout)
}

// 形态取自 resolver 契约的纯函数脚本：全量设置进、意愿清单出，无宿主对象访问
const spikeResolverScript = `"use strict";
function resolve(input) {
	var s = input.settings;
	var entries = [];
	if (s.highQuality === false) {
		entries.push({ point: "workFetch", id: "hd", active: false, reason: "高质量模式已关闭" });
	}
	if (s.sensitive === true) {
		entries.push({ point: "siteBrowsers", id: "sfw", active: false });
	}
	return { version: 1, entries: entries };
}
resolve(input);
`

// TestSpikeFreshRuntimePrecompiledEval 验证「Program 预编译一次 + 每次求值换新鲜
// Runtime」的复用模式：单次求值均值低于成本预算，且求值结果语义正确。
func TestSpikeFreshRuntimePrecompiledEval(t *testing.T) {
	compileStart := time.Now()
	prog, err := goja.Compile("resolver.js", spikeResolverScript, false)
	compileElapsed := time.Since(compileStart)
	if err != nil {
		t.Fatalf("resolver 脚本编译失败：%v", err)
	}
	t.Logf("脚本编译耗时 %v（安装期一次性成本）", compileElapsed)

	input := map[string]interface{}{
		"settings": map[string]interface{}{"highQuality": false, "sensitive": true},
	}

	evalOnce := func() map[string]interface{} {
		vm := goja.New()
		if err := vm.Set("input", input); err != nil {
			t.Fatalf("注入求值输入失败：%v", err)
		}
		v, err := vm.RunProgram(prog)
		if err != nil {
			t.Fatalf("预编译 Program 求值失败：%v", err)
		}
		out, ok := v.Export().(map[string]interface{})
		if !ok {
			t.Fatalf("求值结果不是对象：%T", v.Export())
		}
		return out
	}

	// 预热若干轮，排除首轮的分配器与运行时缓存抖动
	for i := 0; i < 10; i++ {
		evalOnce()
	}

	const rounds = 100
	start := time.Now()
	for i := 0; i < rounds; i++ {
		evalOnce()
	}
	avg := time.Since(start) / rounds

	first := evalOnce()
	if got := fmt.Sprint(first["version"]); got != "1" {
		t.Fatalf("求值结果 version = %s，期望 1", got)
	}
	entries, ok := first["entries"].([]interface{})
	if !ok {
		t.Fatalf("求值结果 entries 不是数组：%T", first["entries"])
	}
	if len(entries) != 2 {
		t.Fatalf("求值结果条目数 = %d，期望 2（两条关闭意愿各产出一条）", len(entries))
	}
	if hd, ok := entries[0].(map[string]interface{}); !ok || fmt.Sprint(hd["active"]) != "false" {
		t.Fatalf("首条目应为 workFetch/hd 停用，实际 %+v", entries[0])
	}

	if avg >= spikeEvalBudget {
		t.Fatalf("新鲜 Runtime 单次求值均值 %v 超预算 %v", avg, spikeEvalBudget)
	}
	t.Logf("新鲜 Runtime + RunProgram 单次求值均值 %v（预算 %v，%d 轮）", avg, spikeEvalBudget, rounds)
}

// 覆盖 ES2015 常用语法的脚本：const/let、箭头函数、模板串（含插值）、对象/数组
// 解构（含默认值与重命名）、for-of、展开运算符、class。以双引号 Go 字符串逐行拼接，
// 以便内嵌 JS 模板串的反引号。
var spikeES2015Script = strings.Join([]string{
	`"use strict";`,
	`const mkEntry = (point, id, active) => ({ point: point, id: id, active: active });`,
	`let entries = [];`,
	`for (const [key, value] of Object.entries({ hd: true, nsfw: false })) {`,
	`	entries.push(mkEntry(` + "`workFetch`" + `, ` + "`${key}`" + `, !!value));`,
	`}`,
	`const { hd, nsfw: sensitive = false } = { hd: true };`,
	`const parts = ["a", ...["b", "c"]];`,
	`const [firstPart] = parts;`,
	`class Rule {`,
	`	constructor(id) { this.id = id; }`,
	`	describe() { return ` + "`rule:${this.id}`" + `; }`,
	`}`,
	`({`,
	`	entries: entries,`,
	`	hd: hd,`,
	`	sensitive: sensitive,`,
	`	joined: parts.join("-"),`,
	`	firstPart: firstPart,`,
	`	rule: new Rule("x").describe(),`,
	`});`,
}, "\n")

// TestSpikeES2015Syntax 验证 goja 对 resolver 脚本依赖的 ES2015 常用语法
// （const/箭头函数/模板串/解构，另含 for-of/展开/class）可编译且语义正确。
func TestSpikeES2015Syntax(t *testing.T) {
	vm := goja.New()
	v, err := vm.RunString(spikeES2015Script)
	if err != nil {
		t.Fatalf("ES2015 语法脚本编译/运行失败：%v", err)
	}
	out, ok := v.Export().(map[string]interface{})
	if !ok {
		t.Fatalf("脚本结果不是对象：%T", v.Export())
	}

	expect := map[string]string{
		"hd":         "true",                        // const 解构取值
		"sensitive":  "false",                       // 解构默认值（源对象未提供该键）
		"joined":     "a-b-c",                       // 展开运算符 + 数组拼接
		"firstPart":  "a",                           // 数组解构
		"rule":       "rule:x",                      // class 方法 + 模板串插值
	}
	for field, want := range expect {
		if got := fmt.Sprint(out[field]); got != want {
			t.Errorf("字段 %s = %q，期望 %q", field, got, want)
		}
	}

	entries, ok := out["entries"].([]interface{})
	if !ok {
		t.Fatalf("entries 不是数组：%T", out["entries"])
	}
	if len(entries) != 2 {
		t.Fatalf("箭头函数 + for-of 产出的条目数 = %d，期望 2", len(entries))
	}
	hd, _ := entries[0].(map[string]interface{})
	nsfw, _ := entries[1].(map[string]interface{})
	if fmt.Sprint(hd["id"]) != "hd" || fmt.Sprint(hd["active"]) != "true" {
		t.Errorf("首条目应为 hd/参与，实际 %+v", hd)
	}
	if fmt.Sprint(nsfw["id"]) != "nsfw" || fmt.Sprint(nsfw["active"]) != "false" {
		t.Errorf("次条目应为 nsfw/停用，实际 %+v", nsfw)
	}
}

// TestSpikeStrippedGlobalsRuntime 验证受限构造可行：新建 Runtime 后从全局删除
// Date 与 Math.random，非确定性源不可达，纯函数求值不受影响。
func TestSpikeStrippedGlobalsRuntime(t *testing.T) {
	vm := goja.New()
	vm.GlobalObject().Delete("Date")
	math, ok := vm.Get("Math").(*goja.Object)
	if !ok {
		t.Fatal("全局 Math 不是对象")
	}
	if err := math.Delete("random"); err != nil {
		t.Fatalf("删除 Math.random 失败：%v", err)
	}

	v, err := vm.RunString(`"use strict";
const probes = {};
probes.dateGone = typeof Date === "undefined";
probes.randomGone = typeof Math.random === "undefined";
probes.dateConstructThrows = (function () { try { new Date(); return false; } catch (e) { return true; } })();
probes.randomCallThrows = (function () { try { Math.random(); return false; } catch (e) { return true; } })();
probes.pureEvalOk = (function (input) {
	var entries = [];
	if (input.settings.sfwOnly) {
		entries.push({ point: "workFetch", id: "main", active: false });
	}
	return { version: 1, entries: entries };
})({ settings: { sfwOnly: true } });
probes;`)
	if err != nil {
		t.Fatalf("受限构造探针脚本运行失败：%v", err)
	}
	probes, ok := v.Export().(map[string]interface{})
	if !ok {
		t.Fatalf("探针结果不是对象：%T", v.Export())
	}

	for _, field := range []string{"dateGone", "randomGone", "dateConstructThrows", "randomCallThrows"} {
		if fmt.Sprint(probes[field]) != "true" {
			t.Errorf("受限探针 %s 不为 true，实际 %v（剥离未生效）", field, probes[field])
		}
	}
	pure, ok := probes["pureEvalOk"].(map[string]interface{})
	if !ok {
		t.Fatalf("纯函数求值结果不是对象：%T", probes["pureEvalOk"])
	}
	if got := fmt.Sprint(pure["version"]); got != "1" {
		t.Errorf("受限构造下纯函数 version = %s，期望 1", got)
	}
	entries, _ := pure["entries"].([]interface{})
	if len(entries) != 1 {
		t.Errorf("受限构造下纯函数条目数 = %d，期望 1", len(entries))
	}
}

// TestSpikeScriptSizeBudget 验证体积上限基线可承载：构造 ≥64KB 的分支密集脚本，
// 编译与首次求值均正常完成，为安装期体积拒收线的取值提供实测锚点。
func TestSpikeScriptSizeBudget(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("\"use strict\";\nfunction resolve(input) {\n\tvar s = input.settings;\n\tvar entries = [];\n")
	for i := 0; sb.Len() < spikeScriptSizeLimit; i++ {
		fmt.Fprintf(&sb, "\tif (s.opt%d === %d) { entries.push({ point: \"workFetch\", id: \"branch%d\", active: true }); }\n", i, i, i)
	}
	sb.WriteString("\treturn { version: 1, entries: entries };\n}\nresolve(input);\n")
	src := sb.String()
	if len(src) < spikeScriptSizeLimit {
		t.Fatalf("构造的脚本仅 %d 字节，不足体积上限 %d", len(src), spikeScriptSizeLimit)
	}

	compileStart := time.Now()
	prog, err := goja.Compile("resolver-64k.js", src, false)
	compileElapsed := time.Since(compileStart)
	if err != nil {
		t.Fatalf("64KB 级脚本编译失败：%v", err)
	}

	vm := goja.New()
	if err := vm.Set("input", map[string]interface{}{"settings": map[string]interface{}{}}); err != nil {
		t.Fatalf("注入求值输入失败：%v", err)
	}
	evalStart := time.Now()
	v, err := vm.RunProgram(prog)
	evalElapsed := time.Since(evalStart)
	if err != nil {
		t.Fatalf("64KB 级脚本首次求值失败：%v", err)
	}
	out, ok := v.Export().(map[string]interface{})
	if !ok {
		t.Fatalf("64KB 级脚本求值结果不是对象：%T", v.Export())
	}
	if got := fmt.Sprint(out["version"]); got != "1" {
		t.Errorf("64KB 级脚本求值 version = %s，期望 1", got)
	}

	t.Logf("脚本 %d 字节（≥上限 %d）：编译 %v、首次求值 %v",
		len(src), spikeScriptSizeLimit, compileElapsed, evalElapsed)
}
