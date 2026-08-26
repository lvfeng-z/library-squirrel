//go:build !windows

package workdirGuard

import (
	"context"
	"runtime"
)

// otherGuard 无内置阻止机制平台的 no-op 实现。
// macOS 无受控文件夹对等物（TCC 管的是应用能否访问目录，不阻止外部修改）；
// Linux chattr +i 需 root/CAP_LINUX_IMMUTABLE，POSIX ACL 是身份级、无法按进程区分。
// 均无用户态可用的「阻止外部修改」机制 → 防护退化为 fsmonitor 检测兜底。
type otherGuard struct{}

// newOtherGuard 创建 no-op 防护实现
func newOtherGuard() Guard {
	return &otherGuard{}
}

// NewPlatformGuard 按当前操作系统返回可用的 Guard 实现。
// Windows：受控文件夹访问引导 + 探针；其余平台：no-op（无用户态阻止机制，fsmonitor 检测兜底）。
// 装配范式同 fsmonitor.NewPlatformDeps（runtime.GOOS 分支注入平台能力）。
func NewPlatformGuard() Guard {
	return newOtherGuard()
}

// Probe no-op：无阻止机制，可写性探测无意义（应用能运行即目录可写）
func (g *otherGuard) Probe(ctx context.Context, workDir string) error {
	return nil
}

// Info 无内置机制平台的信息与兜底说明
func (g *otherGuard) Info() Info {
	return Info{
		Platform:  runtime.GOOS,
		Mechanism: "无内置机制",
		Supported: false,
		Guide:     "当前平台无内置阻止机制，依赖 fsmonitor 检测兜底（外部修改会被检测并提示确认）。",
	}
}
