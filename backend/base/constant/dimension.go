package constant

import "strings"

// 关联级维度清单行（tag_namespace / author_role）的来源三态。零值 = plugin（与 LinkSource 的
// 「PLUGIN=0 为零值」同款：写入方未显式落来源时缺省取最低优先级来源）。
// 优先级 = BUILTIN > USER > PLUGIN（数值大小同序，高值权威）：同值被多来源使用时取最高优先级
// （升级不降级）；值属于内置集的行其 label/origin 为内置权威值，任何 user/plugin 写入不得改写
const (
	ORIGIN_PLUGIN  = 0
	ORIGIN_USER    = 1
	ORIGIN_BUILTIN = 2
)

// BuiltinDimensionItem 维度内置集条目：value 已为归一化形态（小写），label 为显示名
type BuiltinDimensionItem struct {
	Value string
	Label string
}

// BuiltinTagNamespaces tag namespace 内置集（权威来源；启动投影 insert-only 落 tag_namespace 清单，
// origin=builtin）。tag namespace 是开放字符串，本集合仅提供常见取值的候选与显示名
var BuiltinTagNamespaces = []BuiltinDimensionItem{
	{Value: "language", Label: "语言"},
	{Value: "character", Label: "角色"},
	{Value: "parody", Label: "原作"},
	{Value: "female", Label: "女性"},
	{Value: "male", Label: "男性"},
	{Value: "misc", Label: "杂项"},
	{Value: "general", Label: "通用"},
}

// BuiltinAuthorRoles author role 内置集：当前为空（无权威来源，清单靠用户使用与插件声明
// 自然生长）；将来加项时启动投影按本集合自然补入
var BuiltinAuthorRoles = []BuiltinDimensionItem{}

// NormalizeDimensionValue 关联级维度值归一化：去首尾空白 + 折叠小写。清单唯一键与关联写入同规则，
// 避免大小写/空白变体（Character/character/character␣）分裂为不同清单行与不同关联值
func NormalizeDimensionValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
