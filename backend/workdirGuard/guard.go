// Package workdirGuard 工作目录外部操作防护模块。
//
// 职责：按平台自动装配「阻止外部修改 workDir」的能力引导与探测——Windows 用受控文件夹访问
// （系统级功能）引导用户配置 + 探针探测 workDir 当前可写性；无内置阻止机制的平台
// （macOS/Linux）返回 no-op 实现，防护退化为 fsmonitor 检测兜底。本模块只做引导 + 探针，
// 不强制启用系统防护（受控文件夹访问需用户手动开启）。
package workdirGuard

import (
	"context"
)

// Guard 工作目录外部操作防护能力（平台相关；无可用机制的平台返回 no-op 实现）
type Guard interface {
	// Probe 探测 workDir 当前是否可写（被系统保护机制拦截时返回明确错误）
	Probe(ctx context.Context, workDir string) error
	// Info 返回当前平台防护机制与用户引导文案（供前端渲染）
	Info() Info
}

// Info 平台防护机制信息（供前端渲染「目录保护」卡片）
type Info struct {
	Platform  string `json:"platform"`  // 平台标识：windows / linux / darwin
	Mechanism string `json:"mechanism"` // 机制名：受控文件夹访问 / 无内置机制
	Supported bool   `json:"supported"` // 该平台是否有可用阻止机制
	Guide     string `json:"guide"`     // 引导文案（如何配置防护）
}
