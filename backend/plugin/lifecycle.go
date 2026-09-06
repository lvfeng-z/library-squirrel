package plugin

import (
	"context"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
	dto "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// PluginStopOp 发起停用的操作类型，参与者按操作分级处置否决与清理
type PluginStopOp string

const (
	// PluginStopOpUninstall 卸载
	PluginStopOpUninstall PluginStopOp = "uninstall"
	// PluginStopOpUpdate 重装/换版
	PluginStopOpUpdate PluginStopOp = "update"
	// PluginStopOpUntrust 取消信任
	PluginStopOpUntrust PluginStopOp = "untrust"
)

// LifecycleParticipant 插件生命周期参与者：凡持有插件运行痕迹的域注册为参与者。
// 激活相位注册本域痕迹（任一参与者失败触发全体统一回滚）；停用相位先全体否决检查、
// 后按注册逆序清理。参与者注册表是痕迹完备性的唯一审计点——新增持有插件痕迹的域时
// 必须在此登记。
// 重入约束：状态机互斥锁在相位回调执行期间保持持有，参与者实现禁止回调生命周期方法
// （ActivatePlugin/停用入口，重入死锁）；新增参与者时按此约束审计
type LifecycleParticipant interface {
	// Activate 注册本域的插件痕迹（manifest 已读妥；纯否决型参与者空实现）
	Activate(ctx context.Context, plugin *entity2.Plugin, manifest *dto.PluginManifest) error
	// PrepareStop 停用前的否决检查（op=发起操作；force=用户已在确认对话框承担代价后强制停）；
	// 返回 error 中止本次停用，进程保持运行
	PrepareStop(ctx context.Context, pluginPublicId string, op PluginStopOp, force bool) error
	// OnStopped 进程已停后的痕迹清理（清注册表、注销资源、推送前端注销事件）。
	// 须幂等——激活失败的统一回滚与崩溃清理在痕迹未必存在时也会调用
	OnStopped(ctx context.Context, pluginPublicId string)
}

// RegisterLifecycleParticipant 注册生命周期参与者（注册顺序即激活相位顺序，停用按逆序清理）
func (s *Service) RegisterLifecycleParticipant(p LifecycleParticipant) {
	s.lifecycle.registerParticipant(p)
}

// NotifyPluginCrashed 插件进程崩溃通知：loader 已摘除进程表条目，此处经状态机按注册
// 逆序执行参与者痕迹清理（仅运行中状态执行），使崩溃路径与显式停用的清理集合对称；
// 不执行 PrepareStop（崩溃无法否决）也不再停进程
func (s *Service) NotifyPluginCrashed(ctx context.Context, pluginPublicId string) {
	s.lifecycle.notifyCrashed(ctx, pluginPublicId)
}

// removeFiles 删除插件目录文件（不触运行时、不修改数据库记录）
func (s *Service) removeFiles(plugin *entity2.Plugin) {
	appRoot := s.getAppRoot()
	rootPath := ""
	if plugin.RootPath.Valid {
		rootPath = plugin.RootPath.String
	}
	pluginPath := filepath.Join(appRoot, rootPath)
	if err := util.RemoveDir(pluginPath); err != nil {
		logger.Log.Warnf("删除插件目录失败: %v", err)
	}
}
