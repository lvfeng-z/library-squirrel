package export

// 元信息文本净化：导出标题、分享中继载荷（title/source/worksName）、回灌侧作品名共用
// 同一净化口径，单点实现在本包——manifest 契约的消费方（import 包）依赖本包，本包不得
// 反向依赖 import，净化能力须落在依赖图的被依赖侧。

import (
	"strings"
)

// SanitizeMetaText 净化文本：剔除控制字符（含 \r\n\t）并按 rune 截断到上限。
// 分享中继对 title/source/worksName 有长度与控制字符校验，客户端先行净化避免注册被
// malformed 拒绝；导出标题与回灌侧作品名复用同款口径（净化结果空串=未设置，调用方按
// 空值回退无标题形态）。
func SanitizeMetaText(s string, maxRunes int) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	runes := []rune(b.String())
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	return string(runes)
}
