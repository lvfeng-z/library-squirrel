package resource

import (
	"testing"

	"github.com/library-squirrel/backend/storeRegistry"
)

// TestRegisterStoreDirsDeclaresWorkRoot 注册声明面锚定：作品资源域注册的存储目录恰为
// store/work（属主 resource）。白名单谓词（ValidatePath/InScanDirs）与 fsmonitor 扫描根、
// USN 过滤全部随注册内容派生，本断言锚定生产装配注册的条目值。前置：本包测试进程不经装配面、
// 注册表初始为空，注册后应恰含本域一条。
func TestRegisterStoreDirsDeclaresWorkRoot(t *testing.T) {
	if err := RegisterStoreDirs(); err != nil {
		t.Fatalf("注册作品资源存储目录失败: %v", err)
	}
	dirs := storeRegistry.RegisteredDirs()
	want := []storeRegistry.StoreDir{{Path: "store/work", Owner: "resource"}}
	if len(dirs) != len(want) || dirs[0] != want[0] {
		t.Fatalf("注册后条目 = %v, want %v", dirs, want)
	}
}
