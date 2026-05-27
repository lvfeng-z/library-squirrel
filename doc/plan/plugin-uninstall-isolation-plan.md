# 插件卸载逻辑隔离计划

## 背景

当前 `Service.uninstall()` 将三个操作耦合在一起：
1. **运行时清理**：通过 `onUnload` 回调停止子进程、注销 TaskHandler/SiteBrowser/Slot 扩展点、取消静态资源路由
2. **文件删除**：移除插件目录
3. **数据库标记**：设置 `uninstalled = true`

`ReinstallFromPath` 复用 `uninstall()` 后再 `install()`，导致更新过程中产生不必要的 "已卸载" 中间状态。为后续插件更新功能做准备，需要将数据库记录修改与其他卸载操作隔离。

## 方案

将 `Service.uninstall()` 拆分为两个独立步骤，保留现有公开接口不变：

### 1. 新增 `deactivate` 私有方法

从 `uninstall()` 中提取运行时清理 + 文件删除逻辑为独立的 `deactivate()` 方法：

```go
// deactivate 停止插件运行时并删除插件文件
func (s *Service) deactivate(ctx context.Context, pluginPublicId string) error {
    plugin, err := s.repo.GetByPublicId(ctx, pluginPublicId)
    if err != nil {
        return err
    }
    if plugin == nil {
        return ErrPluginNotFound
    }

    // 停止运行时插件（子进程、注册中心等）
    if s.onUnload != nil {
        s.onUnload(pluginPublicId)
    }

    // 删除插件目录
    appRoot := s.getAppRoot()
    rootPath := ""
    if plugin.RootPath.Valid {
        rootPath = plugin.RootPath.String
    }
    pluginPath := filepath.Join(appRoot, rootPath)
    if err := util.RemoveDir(pluginPath); err != nil {
        logger.Log.Warnf("删除插件目录失败: %v", err)
    }

    return nil
}
```

### 2. 重构 `uninstall` 方法

`uninstall()` 改为调用 `deactivate()` + 设置数据库标记：

```go
func (s *Service) uninstall(ctx context.Context, pluginPublicId string) error {
    if err := s.deactivate(ctx, pluginPublicId); err != nil {
        return err
    }

    // 设置为已卸载状态
    plugin, err := s.repo.GetByPublicId(ctx, pluginPublicId)
    if err != nil {
        return err
    }
    if plugin == nil {
        return ErrPluginNotFound
    }

    plugin.Uninstalled = sql.NullBool{Bool: true, Valid: true}
    if err := s.repo.Update(ctx, plugin); err != nil {
        return err
    }

    logger.Log.Infof("插件已卸载: %s", pluginPublicId)
    return nil
}
```

注意：`deactivate` 已经查询过一次插件实体，`uninstall` 仍需再次查询（因为 `deactivate` 可能失败，此时不应修改数据库）。如果性能是考量点，可以让 `deactivate` 返回 plugin 实体，但会增加耦合。当前方案保持职责清晰。

### 3. 修改 `ReinstallFromPath`

将 `s.uninstall(ctx, pluginPublicId)` 改为 `s.deactivate(ctx, pluginPublicId)`，避免更新过程中产生 "已卸载" 中间状态：

```go
func (s *Service) ReinstallFromPath(ctx context.Context, pluginPublicId string, packagePath string, installType domain.InstallType) (*entity2.Plugin, error) {
    // ... 参数校验和原插件信息获取不变 ...

    // 停止旧插件运行时并删除旧文件（不修改数据库状态）
    if err := s.deactivate(ctx, pluginPublicId); err != nil {
        return nil, err
    }

    // 加载新插件包并安装 ...
}
```

## 影响范围

| 文件 | 变更 |
|------|------|
| `backend/plugin/service.go` | 新增 `deactivate` 方法，重构 `uninstall`，修改 `ReinstallFromPath` |

- Handler 层无需变更（`Uninstall` 和 `SetUninstalled` 接口不变）
- 前端无需变更
- `Uninstall` 公开行为不变（完整卸载：清理 + 删除 + 标记）
- `SetUninstalled` 行为不变（仅标记数据库）
- `ReinstallFromPath` 行为变化：不再设置 `uninstalled = true` 中间状态

## 后续扩展

此拆分为插件更新功能提供了基础：
- 更新流程：`deactivate`（停旧）→ `loadPluginPackage`（加载新包）→ `install`（装新）
- 数据库记录保持连续性，无需经历 uninstalled → installed 的状态翻转
