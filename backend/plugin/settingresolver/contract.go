// Package settingresolver 提供插件 resolver 脚本的求值运行器与契约校验。
// resolver = 插件自带的『全量设置 → 条目参与度』纯函数脚本：宿主只执行它、
// 不理解其内部逻辑；条目参与度 = 插件在各派生面（作品拉取/作者拉取/站点浏览器/
// 资源类型/前端扩展）上声明的条目是否参与的意愿表达。
package settingresolver

import "fmt"

// 脚本调用约定（宿主与 resolver 脚本之间的执行契约，SDK 的 TS 契约类型逐字镜像本节）：
//
//  1. 入口函数：脚本必须在顶层定义名为 EntryFuncName（"resolve"）的函数。每次求值
//     在全新运行时中先执行一遍脚本顶层代码，随后宿主以输入对象为唯一实参调用
//     resolve 一次，取其返回值作为求值输出。顶层代码的作用不跨求值保留（每次求值
//     都是全新运行时）。
//  2. 输入：resolve 收到单一对象参数，形状为
//     { "settings": { <key>: <value> } }
//     ——该插件的全量设置快照（加密项由调用方解密后喂入，本包不感知）。设置值为
//     JSON 形状（对象/数组/字符串/数字/布尔/null）；脚本对输入的任何改写都不会
//     泄回调用方数据。
//  3. 输出：resolve 的返回值必须是对象
//     { "version": 1, "entries": [ { "point": "...", "id": "...", "active": true|false, "reason": "..." } ] }
//     - point 取值限于 Point 常量集（派生面词汇表）；越界条目被单条拒收，不株连
//       其余条目（拒收记录见 RejectedEntry）；
//     - resourceTypes 条目的 id = 资源类型串；id 是否为该插件实际声明的条目由
//       消费方对照清单核对，本包只做形状校验；
//     - reason 可选，字符串，供管理页展示停用缘由；
//     - 输出为快照语义：未列出的条目 = 基线参与，该语义由消费方实现；
//     - 未知顶层字段忽略。
//  4. 运行环境：零宿主绑定（无 I/O、网络、文件、计时器），Date 与 Math.random
//     已从运行时删除；脚本须为纯函数——同一输入必产出同一输出。
//  5. 求值失败分类见 FailureKind；失败后「保留上一次覆盖表」的呈现语义由消费方实现。

// EntryFuncName resolver 脚本的入口函数名：脚本必须在顶层定义该名字的函数。
const EntryFuncName = "resolve"

// FailureKind 求值失败分类。调用方按分类决定日志文案与管理页降级标注。
type FailureKind string

const (
	FailureSyntax         FailureKind = "syntax"           // 脚本不可解析（语法错）
	FailureTimeout        FailureKind = "timeout"          // 求值超时被中断
	FailureRuntime        FailureKind = "runtime"          // 运行异常：脚本抛错、入口函数缺失或不可调用
	FailureInvalidOutput  FailureKind = "invalid_output"   // 返回值不符合输出形状
	FailureScriptTooLarge FailureKind = "script_too_large" // 脚本超过体积上限，装载即拒收
)

// EvalError resolver 求值失败：Kind 定分类，Message 人读信息，Err 为底层错误（可缺席）。
type EvalError struct {
	Kind    FailureKind
	Message string
	Err     error
}

func (e *EvalError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("resolver 求值失败（%s）：%s：%v", e.Kind, e.Message, e.Err)
	}
	return fmt.Sprintf("resolver 求值失败（%s）：%s", e.Kind, e.Message)
}

func (e *EvalError) Unwrap() error { return e.Err }

// Input resolver 求值输入：该插件的全量设置快照，key 为设置键、value 为 JSON 形状的值。
type Input struct {
	Settings map[string]interface{}
}

// Point 条目所属的派生面（契约词汇表中 point 字段的合法取值）。
type Point string

const (
	PointWorkFetch          Point = "workFetch"          // 作品拉取候选
	PointSiteAuthorFetch    Point = "siteAuthorFetch"    // 站点作者拉取候选
	PointSiteBrowsers       Point = "siteBrowsers"       // 站点浏览器入口
	PointResourceTypes      Point = "resourceTypes"      // 自定义资源类型（id = 类型串）
	PointFrontendExtensions Point = "frontendExtensions" // 前端扩展（view/menu 等）
)

// pointVocabulary point 字段的合法取值集（契约词汇表）。
var pointVocabulary = map[Point]bool{
	PointWorkFetch:          true,
	PointSiteAuthorFetch:    true,
	PointSiteBrowsers:       true,
	PointResourceTypes:      true,
	PointFrontendExtensions: true,
}

// Entry 通过校验的参与度条目：声明条目的参与开关与可选理由。
type Entry struct {
	Point  Point
	ID     string
	Active bool
	Reason string // 空串 = 脚本未提供理由
}

// RejectedEntry 被单条拒收的输出条目记录，供日志与管理页降级标注。
type RejectedEntry struct {
	Index  int    // 条目在脚本输出 entries 数组中的下标
	Reason string // 拒收原因
}

// Result 求值结果：通过形状校验的条目 + 被单条拒收的记录。整体输出的结构性
// 不合法（version/entries 缺失或形态错误、返回值不是对象）不产生 Result，
// 而是以 FailureInvalidOutput 失败返回。
type Result struct {
	Entries  []Entry
	Rejected []RejectedEntry
}

// ValidateOutput 校验脚本返回值的输出形状：结构性不合法时返回 FailureInvalidOutput
// 错误；条目级不合法仅拒收该条并记入 Rejected，不株连其余条目。
func ValidateOutput(raw interface{}) (*Result, error) {
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return nil, &EvalError{
			Kind:    FailureInvalidOutput,
			Message: fmt.Sprintf("返回值不是对象：%T", raw),
		}
	}
	if !versionIsOne(obj["version"]) {
		return nil, &EvalError{
			Kind:    FailureInvalidOutput,
			Message: fmt.Sprintf("version 字段不是 1（唯一受支持的契约版本），实际 %v", obj["version"]),
		}
	}
	rawEntries, ok := obj["entries"].([]interface{})
	if !ok {
		return nil, &EvalError{
			Kind:    FailureInvalidOutput,
			Message: fmt.Sprintf("entries 字段不是数组：%T", obj["entries"]),
		}
	}

	res := &Result{Entries: make([]Entry, 0, len(rawEntries))}
	for i, rawEntry := range rawEntries {
		entry, reason := validateEntry(rawEntry)
		if reason != "" {
			res.Rejected = append(res.Rejected, RejectedEntry{Index: i, Reason: reason})
			continue
		}
		res.Entries = append(res.Entries, *entry)
	}
	return res, nil
}

// validateEntry 校验单条条目：返回解析出的条目；不合法时条目为 nil 并返回拒收原因。
func validateEntry(raw interface{}) (*Entry, string) {
	item, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Sprintf("条目不是对象：%T", raw)
	}
	point, ok := item["point"].(string)
	if !ok {
		return nil, "point 字段缺失或不是字符串"
	}
	if !pointVocabulary[Point(point)] {
		return nil, fmt.Sprintf("point 越界：%q", point)
	}
	id, ok := item["id"].(string)
	if !ok {
		return nil, "id 字段缺失或不是字符串"
	}
	if id == "" {
		return nil, "id 字段是空字符串"
	}
	active, ok := item["active"].(bool)
	if !ok {
		return nil, "active 字段缺失或不是布尔值"
	}
	reason := ""
	if rawReason, exists := item["reason"]; exists && rawReason != nil {
		reason, ok = rawReason.(string)
		if !ok {
			return nil, "reason 字段不是字符串"
		}
	}
	return &Entry{Point: Point(point), ID: id, Active: active, Reason: reason}, ""
}

// versionIsOne 判断 version 字段是否为数值 1（引擎导出的数字可能是 int 或 float64）。
func versionIsOne(v interface{}) bool {
	switch n := v.(type) {
	case int:
		return n == 1
	case int64:
		return n == 1
	case float64:
		return n == 1
	}
	return false
}
