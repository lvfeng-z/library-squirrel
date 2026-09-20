package authorInfo

// 头像落盘路径派生：头像子树扁平布局（无作品目录层），文件名即身份键派生段
// （store/avatar/site/{桶段}/{siteKey}_{siteAuthorId 派生段}.{ext}、
// store/avatar/local/{桶段}/local_{localAuthorId}.{ext}）。派生规则（净化/截断/
// 单射消歧、桶段哈希）由 SDK storepath 权威承载，本文件组装身份输入与 avatar
// 子树前缀（与 backend/download/naming.go 组装 store/work 前缀同型）。
// 全部产出属 relPath 域（workDir 相对、正斜杠，path.Join 构造），入库/查重/URL
// 映射基准一致；含 workDir 的绝对路径仅在落盘调用点现场拼接，不回流本域。

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/lvfeng-z/library-squirrel-sdk/storepath"
)

// localKeyPrefix 本地作者头像的复合键站点段：本地作者无站点身份，以 ("local",
// 作者行 DB id) 为复合键统一桶段与派生段形态（数值 id 派生段即原文，无净化变形）
const localKeyPrefix = "local"

// SiteAvatarRelPath 派生站点作者头像文件的库内落盘相对路径：
// store/avatar/site/{桶段}/{siteKey}_{siteAuthorId 派生段}.{ext}。
// 桶段 = 复合键 SHA256 前 2 位 hex（同键恒同桶，路径锚定不变量与 store/work 一致）；
// siteKey/siteAuthorId 非法（空值）时由 storepath 显式报错，调用方按执行失败收口。
// ext 为头像格式声明（无点形式如 "jpg"，带点形式亦兼容），空串=无扩展名。
func SiteAvatarRelPath(siteKey, siteAuthorId, ext string) (string, error) {
	fileBase, err := storepath.WorkDirName(siteKey, siteAuthorId)
	if err != nil {
		return "", fmt.Errorf("派生站点作者头像文件段失败: %w", err)
	}
	bucket := storepath.BucketSegment(siteKey, siteAuthorId)
	return path.Join("store", "avatar", "site", bucket, fileBase+normalizeExt(ext)), nil
}

// LocalAvatarRelPath 派生本地作者头像文件的库内落盘相对路径：
// store/avatar/local/{桶段}/local_{localAuthorId}.{ext}。复合键取
// (localKeyPrefix, strconv(id))，与 site 侧同一对派生函数出桶段与文件段。
func LocalAvatarRelPath(localAuthorId int64, ext string) (string, error) {
	idStr := strconv.FormatInt(localAuthorId, 10)
	fileBase, err := storepath.WorkDirName(localKeyPrefix, idStr)
	if err != nil {
		return "", fmt.Errorf("派生本地作者头像文件段失败: %w", err)
	}
	bucket := storepath.BucketSegment(localKeyPrefix, idStr)
	return path.Join("store", "avatar", "local", bucket, fileBase+normalizeExt(ext)), nil
}

// normalizeExt 规范化扩展名（确保以 "." 开头；空串保持空=无扩展名）
func normalizeExt(ext string) string {
	if ext != "" && !strings.HasPrefix(ext, ".") {
		return "." + ext
	}
	return ext
}
