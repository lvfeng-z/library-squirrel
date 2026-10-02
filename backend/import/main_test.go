package importer

import (
	"fmt"
	"os"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/storeRegistry"

	"go.uber.org/zap"
)

// 存储目录白名单为装配期注册（生产在 app.go 装配面），测试进程按生产同值注册三条目，
// 令导入回灌的路径白名单闸门（validateRelPath 的 ValidatePath）以生产口径运行。
// 同时置 nop logger：错误路径经 model.HandleError 走全局 logger，未初始化会 nil panic
// （筛选运行下无 newTestSetup 先行铺垫）。
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	for _, d := range []storeRegistry.StoreDir{
		{Path: "store/work", Owner: "resource"},
		{Path: "store/avatar/local", Owner: "author"},
		{Path: "store/avatar/site", Owner: "author"},
	} {
		if err := storeRegistry.Register(d); err != nil {
			fmt.Fprintf(os.Stderr, "测试装配注册存储目录 %q 失败: %v\n", d.Path, err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
