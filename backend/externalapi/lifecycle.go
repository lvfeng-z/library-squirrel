package externalapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/logger"
)

// discoveryFileName 发现文件名（用户数据目录下）。文件即产品契约：外部智能体据此
// 寻址鉴权，含版本字段供契约向后演进
const discoveryFileName = "api-access.json"

// discoveryFile 发现文件内容。Port 为实际监听端口（端口设置 0=随机分配时写入系统
// 分配的实际值）；Enabled 恒真（接口未启用时文件不存在而非写 false）
type discoveryFile struct {
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
	Token   string `json:"token"`
	Version string `json:"version"`
}

// Lifecycle 外部接口生命周期管理器：设置目标态到运行时的差量同步单一入口。三项联动：
// 开关→监听起停与发现文件写删、固定端口变更→监听换绑、token 变更→监听器换钥（旧
// token 立即失效）与发现文件重写。本类型是对监听器唯一的运行期起停通道（应用关闭
// 收尾除外），运行态簿记与监听器实际状态一致；调用方可能以相同目标态重复进入
// （设置保存回调与设置页控制方法两条路径），差量判定使重复调用为空操作
type Lifecycle struct {
	server *Server
	// discoveryPath 发现文件绝对路径
	discoveryPath string

	mu sync.Mutex
	// running 监听运行态簿记（Start 成功置真、Stop 后置假）
	running bool
	// addr 当前监听地址（未监听为空）
	addr string
	// runningPort 实际监听端口（发现文件写入用；随机分配时为系统分配值）
	runningPort int
	// configuredPort 上次生效的固定端口设置（差量判定：监听中该值变更→换绑）
	configuredPort int
	// lastErr 最近一次同步失败原因（空=无失败；设置页降级提示用）
	lastErr string
}

// NewLifecycle 创建生命周期管理器。discoveryDir 为用户数据目录，发现文件写于其下
func NewLifecycle(server *Server, discoveryDir string) *Lifecycle {
	return &Lifecycle{
		server:        server,
		discoveryPath: filepath.Join(discoveryDir, discoveryFileName),
	}
}

// SyncExternalApi 按设置目标态差量同步运行时（实现 settings.ExternalApiRuntime 接口）。
// 返回错误仅代表本次同步未完全达成——设置已由调用方持久化，属降级语义：监听与发现
// 文件维持同步前的旧状态，下次同步（开关再切换/应用重启）收敛
func (l *Lifecycle) SyncExternalApi(enabled bool, port int, token string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lastErr = ""
	var firstErr error
	recordErr := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
		l.lastErr = err.Error()
		logger.Log.Errorw("外部接口运行时同步失败", "error", err)
	}

	// token 差量：与监听器当前值不同即换钥（内存单点比对，旧 token 立即失效）。
	// 空值不换——token 恒非空由应用装配与设置重置路径保证
	if token != "" && token != l.server.Token() {
		l.server.SetToken(token)
	}

	if enabled && token == "" {
		// 防御性拒绝：启用态不容忍空 token（否则鉴权形同虚设）。停监听、删发现文件，
		// 使运行时与「未启用」一致
		l.stopLocked(recordErr)
		if err := l.deleteDiscoveryFile(); err != nil {
			recordErr(err)
		}
		recordErr(errors.New("外部接口 token 为空，监听未启用"))
		l.configuredPort = port
		return firstErr
	}

	if enabled {
		// 固定端口变更且监听中：先停再按新端口起（端口设置 0=随机，两次同为 0 视为未变更）
		if l.running && port != l.configuredPort {
			l.stopLocked(recordErr)
		}
		if !l.running {
			addr, err := l.server.Start(port)
			if err != nil {
				recordErr(err)
				l.configuredPort = port
				return firstErr
			}
			actualPort, err := parseListenPort(addr)
			if err != nil {
				recordErr(err)
				l.configuredPort = port
				return firstErr
			}
			l.addr = addr
			l.runningPort = actualPort
			l.running = true
		}
		// 发现文件在监听确认可达后写：文件是外部侧的可用性宣告，不可宣告未起的服务
		if err := l.writeDiscoveryFile(); err != nil {
			recordErr(err)
		}
	} else {
		l.stopLocked(recordErr)
		if err := l.deleteDiscoveryFile(); err != nil {
			recordErr(err)
		}
	}
	l.configuredPort = port
	return firstErr
}

// NewExternalApiToken 生成新 token（实现 settings.ExternalApiRuntime 接口）。仅生成
// 随机值；换钥与发现文件更新由设置侧持久化后的 SyncExternalApi 差量完成
func (l *Lifecycle) NewExternalApiToken() (string, error) {
	return GenerateToken()
}

// ExternalApiRuntimeStatus 返回运行时状态（实现 settings.ExternalApiRuntime 接口）：
// 是否监听、监听地址、最近一次同步失败原因
func (l *Lifecycle) ExternalApiRuntimeStatus() (running bool, addr string, lastErr string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.running, l.addr, l.lastErr
}

// stopLocked 停监听并清簿记（调用方须持锁）。停止失败也清簿记——Shutdown 超时仅代表
// 在途请求未在宽限期内收尾，监听已关闭
func (l *Lifecycle) stopLocked(recordErr func(error)) {
	if !l.running {
		return
	}
	if err := l.server.Stop(); err != nil {
		recordErr(err)
	}
	l.running = false
	l.addr = ""
	l.runningPort = 0
}

// writeDiscoveryFile 写发现文件（启用态契约：enabled 恒真、实际监听端口、监听器当前
// token、应用版本）。token 取监听器当前值——它是鉴权的执行点，文件与执行点同源
func (l *Lifecycle) writeDiscoveryFile() error {
	content := discoveryFile{
		Enabled: true,
		Port:    l.runningPort,
		Token:   l.server.Token(),
		Version: constant.AppVersion,
	}
	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return fmt.Errorf("发现文件序列化失败: %w", err)
	}
	// 文件含鉴权 token，权限收紧到属主（POSIX 语义完整；Windows 上按平台常规约束）
	if err := os.WriteFile(l.discoveryPath, data, 0600); err != nil {
		return fmt.Errorf("发现文件写入失败: %w", err)
	}
	return nil
}

// deleteDiscoveryFile 删发现文件（不存在视为已删）
func (l *Lifecycle) deleteDiscoveryFile() error {
	if err := os.Remove(l.discoveryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("发现文件删除失败: %w", err)
	}
	return nil
}

// parseListenPort 从监听地址解析实际端口
func parseListenPort(addr string) (int, error) {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, fmt.Errorf("解析监听地址失败: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("解析监听端口失败: %w", err)
	}
	return port, nil
}
