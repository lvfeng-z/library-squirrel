package authorInfo

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/storeRegistry"
	"go.uber.org/zap"
)

// 存储目录白名单为装配期注册（生产在 app.go 装配面经 RegisterStoreDirs 接入），测试进程按
// 生产同一入口注册，令入库链的路径白名单闸门（PrepareIngest 起的 ValidatePath）以生产口径运行
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	if err := RegisterStoreDirs(); err != nil && !errors.Is(err, storeRegistry.ErrDuplicateDir) {
		fmt.Fprintf(os.Stderr, "测试装配注册作者头像存储目录失败: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
