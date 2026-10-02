package importer

// 回灌前置编排的作品名净化（自 share 提取，行为保持）：净化名是「落地页 worksName /
// 收件子任务名 / Receive 返回列表 / 导入子任务名」多处一致的契约，单点实现在本模块。

import (
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/export"
)

// SanitizeMetaText 净化文本：剔除控制字符（含 \r\n\t）并按 rune 截断到上限
// （分享中继对 title/source/worksName 有长度与控制字符校验，客户端先行净化避免
// 注册被 malformed 拒绝；亦用于导出包回灌侧的作品名净化）
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

// SanitizedWorkName 作品名净化（与 title/source 同款）：site_work_name 优先、次 nick_name、
// 全空以「作品 {ID}」占位，净化控制字符并截断到 200 rune。收件侧子任务命名与落地页 worksName
// 共用同一净化后作品名；relay 侧对 worksName 单名校验 ≤200 rune 且禁控制字符，不净化会被
// 中继以 malformed 拒绝（跨仓契约，见分享方案风险8）。
func SanitizedWorkName(w *export.WorkRecord) string {
	name := ""
	if w.SiteWorkName != nil {
		name = *w.SiteWorkName
	}
	if name == "" && w.NickName != nil {
		name = *w.NickName
	}
	if name == "" {
		name = fmt.Sprintf("作品 %d", w.ID)
	}
	return SanitizeMetaText(name, 200)
}
