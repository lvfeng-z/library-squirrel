package download

// 命名解析：作品命名元数据合并与 store 文件名/落盘路径生成（bas 基准名经文件名模板
// 占位符替换+净化产出，多 store 资源按 role+seq 消歧）。

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/util/filename"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// mergeWorkInfo 将 from(作品信息板块 A 的响应)合并到 to(Start/Resume 的响应),供文件名模板使用
func mergeWorkInfo(to, from *sdkdto.WorkResponse) {
	if to == nil || from == nil {
		return
	}
	if from.Work != nil {
		to.Work = from.Work
	}
	to.SiteAuthors = from.SiteAuthors
	to.LocalAuthors = from.LocalAuthors
	to.SiteTags = from.SiteTags
	to.LocalTags = from.LocalTags
}

// mergeWorkMetaForNaming 把作品命名元数据合并到 Start/Resume 的响应,供文件名模板使用。
// 本次执行跑了作品元数据板块(A)则用其结果;否则(资源板块单独重下)从已有作品加载命名元数据(作者等),
// 避免 ${author} 等占位符因元数据缺失回落到 unknownAuthor
func (sess *execSession) mergeWorkMetaForNaming(startResp, workResp *sdkdto.WorkResponse) {
	if startResp == nil {
		return
	}
	if workResp != nil {
		mergeWorkInfo(startResp, workResp)
		return
	}
	if sess.workId <= 0 || sess.deps.WorkMetaLoader == nil {
		return
	}
	meta, err := sess.deps.WorkMetaLoader.LoadWorkMeta(sess.runCtx(), sess.workId)
	if err != nil {
		logger.Log.Warnf("[Download] 任务 %d 加载已有作品 %d 命名元数据失败: %v", sess.taskId, sess.workId, err)
		return
	}
	if meta != nil {
		mergeWorkInfo(startResp, meta)
	}
}

// resolveBaseName 算 bas(基准名,不含 ext)与目录相对路径(store/resource/<作者>)。
// bas = FileNameFormat 模板经占位符替换+净化生成(模板保证非空);不依赖具体 spec,
// 模板仅消费作品元数据。多 store 文件名的 role/seq/desc 段由 resolveStorePath 按 spec 拼接
func (sess *execSession) resolveBaseName(workResp *sdkdto.WorkResponse) (relativePath, bas string) {
	tpl := sess.deps.FileNameFormatProvider.GetFileNameFormat()
	tokenData := filename.ExtractTokenData(workResp)
	formatted := filename.FormatFileName(tpl, tokenData)
	bas = filename.SanitizeFileName(formatted)
	authorDir := filename.SanitizeFileName(tokenData.Author)
	// relPath 域用 path.Join（正斜杠），落库/查重基准一致
	relativePath = path.Join("store", "resource", authorDir)
	return
}

// resolveStorePath 按资源级判定拼 store 文件名与路径(thumbnail 普通 role,无特例):
//   - 单 store 资源(multiStore=false):<bas>.<ext>
//   - 多 store 资源(multiStore=true):<bas>_<role>_<seq>[_<描述>].<ext>
//
// seq 为同 role 内 0-based 序号(= store_seq,续传身份键);描述取自 spec.Description,净化后为空则省略。
// StoreStream 据 relPath 创建文件,relPath 末段须与 fileName 一致,否则多 store 落盘同一 relPath 互相覆盖
func (sess *execSession) resolveStorePath(spec *sdkdto.StoreSpec, baseRelPath, bas string, sameRoleSeq int, multiStore bool) (relativePath, fileName string) {
	ext := normalizeExt(spec.Format)
	if !multiStore {
		fileName = bas + ext
	} else {
		name := fmt.Sprintf("%s_%s_%03d", bas, spec.Role, sameRoleSeq)
		if spec.Description != "" {
			if desc := filename.SanitizeFileName(spec.Description); desc != "" {
				name += "_" + desc
			}
		}
		fileName = name + ext
	}
	// filepath.ToSlash 统一正斜杠入库(跨平台规范,与 persistentStore 的变体路径生成一致;
	// 避免 Windows 下 filepath.Join 产生反斜杠致 DB 路径分隔符不一致)
	relativePath = filepath.ToSlash(filepath.Join(baseRelPath, fileName))
	return
}

// normalizeExt 规范化扩展名(确保以 "." 开头)
func normalizeExt(format string) string {
	ext := format
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}
