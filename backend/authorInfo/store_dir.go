package authorInfo

// 作者个人信息（头像）存储域：store 存储目录注册与库内落盘路径派生。
// 本文件为域条目声明单一源——头像两条目的路径与属主在此登记，装配（app.go
// registerStoreDirs）只按调用时序接入。拉取/导入编排（service/handler）属后续阶段，
// 落地时以本包为宿主。

import (
	"fmt"

	"github.com/library-squirrel/backend/storeRegistry"
)

// RegisterStoreDirs 注册作者头像域的 store 存储目录：store/avatar/site 与
// store/avatar/local 分别是站点/本地作者头像文件的存储根，子树内部布局按身份键
// 桶段摊薄（路径派生见 paths.go）。注册后 persistentStore 据白名单代管该子树
// （落盘校验、指纹、记录行、抑制登记），fsmonitor 据注册快照圈定对账扫描根与
// USN 过滤范围。调用时机：装配期（app.go）、fsmonitor 启动之前——扫描根与 USN
// 缓存种子在监控启动时取注册快照；前缀冲突/路径非法属装配错误，调用方 fail-fast
// 阻断启动。
func RegisterStoreDirs() error {
	for _, d := range []storeRegistry.StoreDir{
		{Path: "store/avatar/local", Owner: "authorInfo"}, // 本地作者头像
		{Path: "store/avatar/site", Owner: "authorInfo"},  // 站点作者头像
	} {
		if err := storeRegistry.Register(d); err != nil {
			return fmt.Errorf("注册作者头像存储目录 %q（属主 %s）失败: %w", d.Path, d.Owner, err)
		}
	}
	return nil
}
