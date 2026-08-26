//go:build windows

package workdirGuard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/library-squirrel/backend/storeRegistry"
)

// probeRelPath 探针文件相对 workDir 的固定路径（workDir 根下隐藏文件名）。
// 固定在根、不进 store/backup 扫描白名单——即便 fsmonitor 实时事件到达，也会被
// handleFileChange 的扫描域过滤丢弃；再叠加 storeRegistry 抑制登记双保险（方案风险6）。
const probeRelPath = ".squirrel_guard_probe"

// windowsGuard Windows 平台防护实现：受控文件夹访问引导 + 可写性探针。
// 受控文件夹访问是系统级功能（依赖用户手动开启），本实现只负责探测与引导，不强制。
type windowsGuard struct{}

// newWindowsGuard 创建 Windows 平台防护实现
func newWindowsGuard() Guard {
	return &windowsGuard{}
}

// NewPlatformGuard 按当前操作系统返回可用的 Guard 实现。
// Windows：受控文件夹访问引导 + 探针；其余平台：no-op（无用户态阻止机制，fsmonitor 检测兜底）。
// 装配范式同 fsmonitor.NewPlatformDeps（runtime.GOOS 分支注入平台能力）。
func NewPlatformGuard() Guard {
	return newWindowsGuard()
}

// Probe 在 workDir 下创建并删除临时探针文件，验证目录可写。
// 失败语义：受控文件夹访问拦截或 ACL 只读都表现为写失败（ACCESS_DENIED），
// 多因无法区分（方案风险2），错误文案覆盖三类成因（目录不存在/只读/系统保护拦截）。
// 探针路径登记操作抑制：创建与删除各触发一次 fsnotify Create/Remove，
// 经 storeRegistry.Suppress/Release 丢弃，避免探测自身产生外部变更误报。
func (g *windowsGuard) Probe(ctx context.Context, workDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if workDir == "" {
		return fmt.Errorf("工作目录为空，无法探测")
	}
	storeRegistry.Suppress(probeRelPath)
	defer storeRegistry.Release(probeRelPath)

	probeAbs := filepath.Join(workDir, probeRelPath)
	if err := os.WriteFile(probeAbs, []byte("squirrel-guard-probe"), 0o600); err != nil {
		return fmt.Errorf("工作目录探测失败：目录不存在、只读，或被系统保护机制（如受控文件夹访问）拦截: %w", err)
	}
	// 清理探针：并发探测或此前崩溃残留已被本次写入覆盖，删除时 ENOENT 视为已清理
	if err := os.Remove(probeAbs); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("探针文件清理失败（目录权限异常）: %w", err)
	}
	return nil
}

// Info Windows 平台防护机制信息与引导文案
func (g *windowsGuard) Info() Info {
	return Info{
		Platform:  "windows",
		Mechanism: "受控文件夹访问",
		Supported: true,
		Guide:     "Windows 安全中心 → 病毒和威胁防护 → 勒索软件防护 → 受控文件夹访问 → 开启该功能；随后点击「受控文件夹」添加你的资源库目录，点击「允许应用通过受控文件夹访问」把本程序加入白名单。",
	}
}
