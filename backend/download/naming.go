package download

// 命名派生：作品目录段与 store 文件名（store/resource/{siteKey}_{siteWorkId派生段}/
// {role}_{seq 三位零填充}.{ext}）。派生规则（净化/截断/单射消歧）由 SDK storepath 权威
// 承载，本文件组装身份输入（领域行站点复合键）与库内布局前缀（store/resource/，归
// storeRegistry 权威）。

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/lvfeng-z/library-squirrel-sdk/storepath"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// resolveStoreDir 解析作品落盘目录（relPath 域，正斜杠）：store/resource/{siteKey}_{siteWorkId派生段}。
// 身份输入取领域行站点复合键：siteWorkId 用原文，siteKey 经站点 ID 反查站点行；键缺失或
// 站点行查不到时显式报错（写入路径严格识别，不回落），由调用方按执行失败收口
func (sess *execSession) resolveStoreDir(ctx context.Context) (string, error) {
	if !sess.workTask.SiteID.Valid || !sess.workTask.SiteWorkID.Valid || sess.workTask.SiteWorkID.String == "" {
		return "", fmt.Errorf("任务 %d 缺少站点复合键（siteId valid=%v, siteWorkId valid=%v）",
			sess.taskId, sess.workTask.SiteID.Valid, sess.workTask.SiteWorkID.Valid)
	}
	siteKey, ok := sess.resolveSiteKey(ctx, sess.workTask.SiteID.Int64)
	if !ok {
		return "", fmt.Errorf("站点行缺失（siteId=%d），无法派生作品目录名", sess.workTask.SiteID.Int64)
	}
	dirName, err := storepath.WorkDirName(siteKey, sess.workTask.SiteWorkID.String)
	if err != nil {
		return "", fmt.Errorf("派生作品目录名失败: %w", err)
	}
	// relPath 域用 path.Join（正斜杠），落库/查重基准一致
	return path.Join("store", "resource", dirName), nil
}

// resolveStorePath 拼单个 store 的最终相对路径与文件名：{role}_{seq 三位零填充}.{ext}
// （恒带 role_seq——role 内序号是落盘文件与续传配对的身份键，单 store 资源不省略段）。
// ext 取 spec.Format 经 normalizeExt
func resolveStorePath(spec *sdkdto.StoreSpec, baseRelPath string, sameRoleSeq int) (relativePath, fileName string, err error) {
	fileName, err = storepath.StoreFileName(spec.Role, sameRoleSeq, normalizeExt(spec.Format))
	if err != nil {
		return "", "", fmt.Errorf("派生 store 文件名失败(role=%s seq=%d): %w", spec.Role, sameRoleSeq, err)
	}
	return path.Join(baseRelPath, fileName), fileName, nil
}

// normalizeExt 规范化扩展名(确保以 "." 开头)
func normalizeExt(format string) string {
	ext := format
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}
