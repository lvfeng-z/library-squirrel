package plugin

import (
	"context"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/plugin/extension"
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// settingChangeNotifyTimeout 单次设置变更通知的超时预算：插件处置慢或无响应时放弃
// 本次通知（RPC 随 ctx 取消，插件侧处置函数若仍在执行由其自理）
const settingChangeNotifyTimeout = 5 * time.Second

// SettingChangeNotifier 设置变更通知器：向已激活的插件进程推送设置变更通知
// （source=save/reset + 变更键集）。调用立即返回、发送在独立 goroutine 进行；
// 尽力而为——插件未激活（无进程）跳过，发送失败（含旧契约插件 Unimplemented）或
// 超时均静默降级（debug 日志），不构成设置保存流程的失败面
type SettingChangeNotifier interface {
	// NotifyAsync 异步推送设置变更通知（source 取 SDK 词汇 save/reset）
	NotifyAsync(pluginPublicId, source string, keys []string)
}

// settingChangeNotifier SettingChangeNotifier 的访问器实现：经 extension.ServiceAccessor
// 取插件 gRPC 生命周期客户端发送 SettingChanged
type settingChangeNotifier struct {
	accessor extension.ServiceAccessor
	timeout  time.Duration
}

// NewSettingChangeNotifier 创建设置变更通知器（accessor 为插件 gRPC 服务访问器，
// 生产装配传入 extension.Loader）
func NewSettingChangeNotifier(accessor extension.ServiceAccessor) SettingChangeNotifier {
	return newSettingChangeNotifier(accessor, settingChangeNotifyTimeout)
}

// newSettingChangeNotifier 按显式超时预算构造（生产用 settingChangeNotifyTimeout，
// 测试可缩短以锚定超时放弃行为）
func newSettingChangeNotifier(accessor extension.ServiceAccessor, timeout time.Duration) *settingChangeNotifier {
	return &settingChangeNotifier{accessor: accessor, timeout: timeout}
}

// NotifyAsync 异步推送设置变更通知（发送在独立 goroutine 进行，见接口注释）
func (n *settingChangeNotifier) NotifyAsync(pluginPublicId, source string, keys []string) {
	go n.deliver(pluginPublicId, source, keys)
}

// deliver 单次通知投递：未激活跳过（常态，无日志），发送失败或超时 debug 日志后放弃
func (n *settingChangeNotifier) deliver(pluginPublicId, source string, keys []string) {
	services, ok := n.accessor.GetServices(pluginPublicId)
	if !ok {
		return
	}
	if services.Lifecycle == nil {
		logger.Log.Debugw("插件生命周期客户端缺失，跳过设置变更通知", "plugin", pluginPublicId)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()
	if _, err := services.Lifecycle.SettingChanged(ctx, &pluginsdkdto.SettingChangedRequest{
		Source: source,
		Keys:   keys,
	}); err != nil {
		logger.Log.Debugw("设置变更通知未送达（尽力而为，已放弃）",
			"plugin", pluginPublicId, "source", source, "error", err)
	}
}
