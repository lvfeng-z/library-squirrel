package stickymemory

import (
	"net/url"
	"strings"
)

// ResolveTaskSiteDomain 任务创建冲突的站点域求值：候选监听条目声明的 siteKey 全部
// 一致且非空时取该声明值；否则退任务 URL 的 host 兜底。host 解析失败或为空时
// ok=false——调用方本轮既不查也不写记忆，冲突照常交用户显选。siteKey 为清单条目级
// 可选自由串（约定复用身份域站点串），候选间不一致即整体弃用声明值：站点域判定
// 永不依赖单个候选的自述
func ResolveTaskSiteDomain(candidateSiteKeys []string, rawURL string) (siteDomain string, ok bool) {
	if siteKey, consistent := unanimousSiteKey(candidateSiteKeys); consistent {
		return siteKey, true
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	host := parsed.Hostname()
	if host == "" {
		return "", false
	}
	return host, true
}

// unanimousSiteKey 判定候选 siteKey 列表是否全体一致且非空：是则返回 (该值, true)。
// 列表含空项、存在不一致项或列表本身为空时返回 ("", false)——均按无一致声明处理，
// 站点域退 URL host 兜底。输入逐项去首尾空白后比较（与派生索引登记侧的归一化同规）
func unanimousSiteKey(candidateSiteKeys []string) (string, bool) {
	unanimous := ""
	for _, key := range candidateSiteKeys {
		key = strings.TrimSpace(key)
		if key == "" || (unanimous != "" && key != unanimous) {
			return "", false
		}
		if unanimous == "" {
			unanimous = key
		}
	}
	return unanimous, unanimous != ""
}
