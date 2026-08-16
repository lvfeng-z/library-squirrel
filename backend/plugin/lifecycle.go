package plugin

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
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

// LifecycleParticipant 插件生命周期参与者：凡持有插件运行时痕迹的模块注册为参与者，
// 参与者注册表是停用清理完备性的唯一审计点
type LifecycleParticipant interface {
	// PrepareStop 停用前的否决检查（op=发起操作；force=用户已在确认对话框承担代价后强制停）；
	// 返回 error 中止本次停用，进程保持运行
	PrepareStop(ctx context.Context, pluginPublicId string, op PluginStopOp, force bool) error
	// OnStopped 进程已停后的痕迹清理（清注册表、注销资源、推送前端注销事件）
	OnStopped(ctx context.Context, pluginPublicId string)
}

// RegisterLifecycleParticipant 注册生命周期参与者
func (s *Service) RegisterLifecycleParticipant(p LifecycleParticipant) {
	s.participants = append(s.participants, p)
}

// SetRuntimeStopper 设置运行时停止器：进程停止与其所属注册表（loader 持有的
// taskHandler/siteBrowser/URL 监听）清理的执行者
func (s *Service) SetRuntimeStopper(stopper func(pluginPublicId string) error) {
	s.runtimeStopper = stopper
}

// stopRuntime 停止插件运行时：否决检查 → 停进程 → 参与者清理痕迹。
// 不删插件文件、不修改数据库记录；force=true 跳过否决检查
func (s *Service) stopRuntime(ctx context.Context, pluginPublicId string, op PluginStopOp, force bool) error {
	if !force {
		for _, p := range s.participants {
			if err := p.PrepareStop(ctx, pluginPublicId, op, force); err != nil {
				return fmt.Errorf("停用被参与者否决: %w", err)
			}
		}
	}

	if s.runtimeStopper != nil {
		if err := s.runtimeStopper(pluginPublicId); err != nil {
			return err
		}
	}

	for _, p := range s.participants {
		p.OnStopped(ctx, pluginPublicId)
	}
	return nil
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
