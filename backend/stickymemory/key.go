package stickymemory

import (
	"sort"
	"strings"
)

// 键编码分隔符取控制字符（\x00/\x01/\x02）：插件 ID、扩展点 ID、站点域的取值域
// （标识符与 URL host / 身份域站点键）均不含控制字符，编码天然无碰撞；三个分隔符互不
// 相同且层级分明（候选内 → 候选间 → 域界），拆解按层切分不依赖转义
const (
	candidateKeySeparator  = "\x00"
	candidateListSeparator = "\x01"
	siteDomainSeparator    = "\x02"
)

// CandidateFullKey 候选全键 = 插件 PublicID + NUL + 扩展点 ID。NUL 不在标识符取值域内，
// ("a","bc") 与 ("ab","c") 不会撞键；冲突弹窗的默认排序与此键的字典序同源
func CandidateFullKey(pluginPublicId, extensionId string) string {
	return pluginPublicId + candidateKeySeparator + extensionId
}

// SplitCandidateFullKey 拆解候选全键 → (插件 PublicID, 扩展点 ID)。展示层解析记忆条目的
// 选中者（value）与候选名单条目时使用；输入须为 CandidateFullKey 产物（不含 NUL 时
// 整串视为插件 ID、扩展点 ID 为空串）
func SplitCandidateFullKey(fullKey string) (pluginPublicId, extensionId string) {
	i := strings.Index(fullKey, candidateKeySeparator)
	if i < 0 {
		return fullKey, ""
	}
	return fullKey[:i], fullKey[i+len(candidateKeySeparator):]
}

// BuildContextKey 记忆上下文键 = 站点域 + \x02 + join(按全键字典序排序的候选全键, \x01)。
// 候选先排序使乱序输入产生同键——同一次冲突的候选发现序不稳定时不分裂记忆；调用方切片
// 不被改写（排序在副本上进行）
func BuildContextKey(siteDomain string, candidateFullKeys []string) string {
	sorted := make([]string, len(candidateFullKeys))
	copy(sorted, candidateFullKeys)
	sort.Strings(sorted)
	return siteDomain + siteDomainSeparator + strings.Join(sorted, candidateListSeparator)
}

// ParseContextKey 拆解记忆上下文键 → (站点域, 候选全键列表)。按首个 \x02 切出站点域、
// 其余按 \x01 切候选——域边界由 \x02 独占，站点域内出现 \x01 不产生拆解歧义；候选全键内
// 不含 \x01（标识符字符集保证）。无 \x02 时整串视为站点域、候选为空
func ParseContextKey(contextKey string) (siteDomain string, candidateFullKeys []string) {
	domain, rest, found := strings.Cut(contextKey, siteDomainSeparator)
	if !found {
		return contextKey, nil
	}
	if rest == "" {
		return domain, nil
	}
	return domain, strings.Split(rest, candidateListSeparator)
}
