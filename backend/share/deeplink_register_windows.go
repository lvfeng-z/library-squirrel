//go:build windows

package share

// 深链协议自注册（便携分发形态）：App 每次启动幂等自写 HKCU\Software\Classes，使
// `library-squirrel://` 深链唤起本程序（安装版的 HKLM 注册由 NSIS 安装/卸载段管理，
// 与本自注册并存时 HKCU 对当前用户优先）。exe 移动后的悬空路径经每次启动重写自愈；
// 注册键被其他现存程序占用时记日志跳过（不覆盖他人注册）。

import (
	"fmt"
	"os"
	"strings"

	"github.com/library-squirrel/backend/base/logger"
	"golang.org/x/sys/windows/registry"
)

// 深链协议注册表键路径（HKCU 视图）
const (
	protocolRootKey    = `Software\Classes\` + DeepLinkScheme
	protocolCommandKey = protocolRootKey + `\shell\open\command`
)

// protocolDescription 协议注册描述（注册表默认值）
const protocolDescription = "URL:LibrarySquirrel 分享链接"

// ShareProtocolRegStatus 深链协议注册状态（HKCU 视图）
type ShareProtocolRegStatus struct {
	Registered bool   `json:"registered"` // HKCU 注册键存在且 command 非空
	Command    string `json:"command"`    // 当前注册的启动命令
	CurrentExe bool   `json:"currentExe"` // 注册命令是否指向当前运行的 exe
}

// EnsureShareProtocolRegistered 幂等自注册深链协议（HKCU）。已指向本 exe 时 no-op；
// 悬空路径（本程序旧位置残留）重写自愈；被其他现存程序占用时报错跳过。
func EnsureShareProtocolRegistered() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取程序路径失败: %w", err)
	}
	wantCmd := fmt.Sprintf(`"%s" "%%1"`, exe)
	if prev := readProtocolCommand(); prev != "" {
		if strings.EqualFold(prev, wantCmd) {
			return nil // 已注册到当前 exe
		}
		if prevExe := extractCommandExe(prev); prevExe != "" && fileExists(prevExe) {
			return fmt.Errorf("深链协议 %s 已被其他程序注册: %s", DeepLinkScheme, prev)
		}
		// 悬空路径（本程序旧位置/损坏注册）：重写自愈
	}
	base, _, err := registry.CreateKey(registry.CURRENT_USER, protocolRootKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册键失败: %w", err)
	}
	defer func() { _ = base.Close() }()
	if err := base.SetStringValue("", protocolDescription); err != nil {
		return err
	}
	if err := base.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}
	cmdKey, _, err := registry.CreateKey(registry.CURRENT_USER, protocolCommandKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开命令注册键失败: %w", err)
	}
	defer func() { _ = cmdKey.Close() }()
	if err := cmdKey.SetStringValue("", wantCmd); err != nil {
		return err
	}
	logger.Log.Infof("[share] 深链协议已自注册（HKCU）: %s", wantCmd)
	return nil
}

// QueryShareProtocolRegStatus 查询 HKCU 注册状态（设置页展示）
func QueryShareProtocolRegStatus() ShareProtocolRegStatus {
	cmd := readProtocolCommand()
	if cmd == "" {
		return ShareProtocolRegStatus{}
	}
	exe, err := os.Executable()
	current := err == nil && strings.EqualFold(extractCommandExe(cmd), exe)
	return ShareProtocolRegStatus{Registered: true, Command: cmd, CurrentExe: current}
}

// UnregisterShareProtocol 删除 HKCU 自注册键（便携版无卸载器，设置页清理入口）。
// 仅删 HKCU 视图；安装版的 HKLM 键由 NSIS 卸载段管理。
func UnregisterShareProtocol() error {
	for _, sub := range []string{
		protocolCommandKey,
		protocolRootKey + `\shell\open`,
		protocolRootKey + `\shell`,
		protocolRootKey + `\DefaultIcon`,
	} {
		if err := registry.DeleteKey(registry.CURRENT_USER, sub); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除注册键 %s 失败: %w", sub, err)
		}
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, protocolRootKey); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除注册键 %s 失败: %w", protocolRootKey, err)
	}
	logger.Log.Infof("[share] 深链协议注册已取消（HKCU）")
	return nil
}

// readProtocolCommand 读取当前注册的启动命令（无注册为空）
func readProtocolCommand() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, protocolCommandKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetStringValue("")
	if err != nil {
		return ""
	}
	return v
}

// extractCommandExe 从 `"exe路径" "%1"` 形态的启动命令提取 exe 路径
func extractCommandExe(cmd string) string {
	s := strings.TrimSpace(cmd)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, `"`) {
		if end := strings.Index(s[1:], `"`); end >= 0 {
			return s[1 : 1+end]
		}
		return ""
	}
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

// fileExists 路径存在且为文件
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
