package authorInfo

import (
	"errors"
	"slices"
	"testing"

	"github.com/library-squirrel/backend/storeRegistry"
)

// TestRegisterStoreDirs 注册面锚定：两条头像子树以属主 authorInfo 登记，路径与既有
// 占位注册等价（store/avatar/local、store/avatar/site 不变——属主转移不改变子树范围，
// 对账扫描根与白名单口径不受影响）。
func TestRegisterStoreDirs(t *testing.T) {
	if err := RegisterStoreDirs(); err != nil {
		t.Fatalf("注册作者头像存储目录失败: %v", err)
	}
	want := []storeRegistry.StoreDir{
		{Path: "store/avatar/local", Owner: "authorInfo"},
		{Path: "store/avatar/site", Owner: "authorInfo"},
	}
	dirs := storeRegistry.RegisteredDirs()
	for _, w := range want {
		if !slices.Contains(dirs, w) {
			t.Fatalf("注册表缺条目 %+v，实际: %+v", w, dirs)
		}
	}
}

// TestDerivedPathWithinRegisteredDirs 派生路径与注册子树一致：路径派生 helper 的
// 产出须落在已注册头像子树内（persistentStore 落盘校验按白名单放行，fsmonitor
// 按注册快照纳入对账——两轨对派生路径的口径一致性锚定）。注册只发生在装配期
// （单一窗口，无重置入口），同进程前序用例已注册时容忍重复注册哨兵。
func TestDerivedPathWithinRegisteredDirs(t *testing.T) {
	if err := RegisterStoreDirs(); err != nil && !errors.Is(err, storeRegistry.ErrDuplicateDir) {
		t.Fatalf("注册作者头像存储目录失败: %v", err)
	}
	sitePath, err := SiteAvatarRelPath("pixiv", "12345", "jpg")
	if err != nil {
		t.Fatalf("派生站点头像路径失败: %v", err)
	}
	if err := storeRegistry.ValidatePath(sitePath); err != nil {
		t.Fatalf("站点头像路径 %q 未过白名单校验: %v", sitePath, err)
	}
	localPath, err := LocalAvatarRelPath(42, "png")
	if err != nil {
		t.Fatalf("派生本地头像路径失败: %v", err)
	}
	if err := storeRegistry.ValidatePath(localPath); err != nil {
		t.Fatalf("本地头像路径 %q 未过白名单校验: %v", localPath, err)
	}
}
