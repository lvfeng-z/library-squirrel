package importer

// 回灌前置编排的作品名净化：净化名是「落地页 worksName / 收件子任务名 / Receive 返回列表 /
// 导入子任务名」多处一致的契约。净化口径 SanitizeMetaText 实现在 export 包——本包依赖
// export 取 manifest 契约，export 不得反向依赖本包，共用净化能力须落在被依赖侧。

import (
	"fmt"

	"github.com/library-squirrel/backend/export"
)

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
	return export.SanitizeMetaText(name, 200)
}
