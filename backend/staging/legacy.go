package staging

// 旧版暂存根退役：暂存总根 staging/ 落地前，下载与收件任务的暂存写在 workDir 顶层的 task-staging/
// （旧统一根）与 share-receive/（旧收件根）。两根整体作废、内容不迁移——旧版暂停态任务的下载
// 半成品随回收丢弃，任务恢复时从零重下。回收完成后旧根不再产生，本清扫随之可删。

import (
	"context"
	"os"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
)

// legacyRootNames 旧版暂存根目录名（workDir 顶层平级）。
var legacyRootNames = []string{"task-staging", "share-receive"}

// RetireLegacyRoots 启动时回收旧版暂存根：整目录移除并记 Info（告知旧版暂停态任务的半成品
// 已作废）。单目录回收失败（文件被占用等）不中断其余、留待下次启动重试（幂等）；根不存在为
// 静默 no-op。workDir 空串=未配置，直接返回（启动期服务不启动的既有守卫语义）。
func RetireLegacyRoots(ctx context.Context, workDir string) error {
	if workDir == "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, name := range legacyRootNames {
		if err := ctx.Err(); err != nil {
			return err
		}
		rootAbs := filepath.Join(workDir, name)
		if _, serr := os.Lstat(rootAbs); serr != nil {
			if os.IsNotExist(serr) {
				continue
			}
			logger.Log.Warnf("[staging] 检查旧版暂存根失败（留待下次启动重试）: %s 错误=%v", rootAbs, serr)
			continue
		}
		if err := removeAll(rootAbs); err != nil {
			logger.Log.Warnf("[staging] 回收旧版暂存根失败（留待下次启动重试）: %s 错误=%v", rootAbs, err)
			continue
		}
		logger.Log.Infof("[staging] 回收旧版暂存根: %s（旧版暂停态任务的下载半成品已作废，恢复后从零重下）", rootAbs)
	}
	return nil
}
