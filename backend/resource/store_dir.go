package resource

import (
	"fmt"

	"github.com/library-squirrel/backend/storeRegistry"
)

// RegisterStoreDirs 注册作品资源域的 store 存储目录：store/work 是库内全部作品 store 文件
// （插件下载落盘与合并、导入回灌产物）的唯一存储根，子树内部布局按站点复合键派生目录段并
// 桶段摊薄。注册后 persistentStore 据白名单代管该子树（落盘校验、指纹、记录行、抑制登记），
// fsmonitor 据注册快照圈定对账扫描根与 USN 过滤范围。调用时机：装配期（app.go）、fsmonitor
// 启动之前——扫描根与 USN 缓存种子在监控启动时取注册快照；前缀冲突/路径非法属装配错误，
// 调用方 fail-fast 阻断启动。
func RegisterStoreDirs() error {
	if err := storeRegistry.Register(storeRegistry.StoreDir{
		Path:  "store/work", // relPath 域基准：正斜杠、相对 {workDir}
		Owner: "resource",
	}); err != nil {
		return fmt.Errorf("注册作品资源存储目录 store/work 失败: %w", err)
	}
	return nil
}
