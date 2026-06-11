# 计划：使用 Windows Job Object 将插件子进程归入主程序分组

## 目标

在 Windows 任务管理器中，将插件子进程归入主应用程序分组下显示，而非独立显示在"后台进程"分类中。

## 方案

使用 Windows Job Object 将主进程和所有插件子进程组合在一起。设置 `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` 标志，使主程序退出时自动终止所有插件子进程。

## 实现细节

### 1. 新建 `backend/plugin/extension/process_group_windows.go`

使用构建标签 `//go:build windows` 限制为 Windows 平台编译。

```go
//go:build windows

package extension

// ProcessGroup Windows 实现，使用 Job Object 管理子进程分组
type ProcessGroup struct {
    job windows.Handle
}

func NewProcessGroup() (*ProcessGroup, error) {
    // 1. windows.CreateJobObject(nil, nil) 创建匿名 Job Object
    // 2. 设置 JobObjectExtendedLimitInformation：
    //    - JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE（主进程退出时自动终止子进程）
    // 3. 返回 ProcessGroup 实例
}

func (pg *ProcessGroup) Assign(pid int) error {
    // 1. windows.OpenProcess(PROCESS_SET_QUOTA | PROCESS_TERMINATE, false, pid) 获取进程句柄
    // 2. windows.AssignProcessToJobObject(pg.job, handle) 将进程加入 Job
    // 3. windows.CloseHandle(handle) 关闭进程句柄（已由 Job 持有引用）
}

func (pg *ProcessGroup) Close() {
    // windows.CloseHandle(pg.job) 关闭 Job Object 句柄
    // 关闭后若设置了 KILL_ON_JOB_CLOSE，所有子进程会被自动终止
}
```

### 2. 新建 `backend/plugin/extension/process_group_other.go`

使用构建标签 `//go:build !windows` 提供其他平台的空实现。

```go
//go:build !windows

package extension

type ProcessGroup struct{}

func NewProcessGroup() (*ProcessGroup, error) {
    return &ProcessGroup{}, nil // 非 Windows 平台不做任何处理
}

func (pg *ProcessGroup) Assign(pid int) error {
    return nil
}

func (pg *ProcessGroup) Close() {
    // no-op
}
```

### 3. 修改 `backend/plugin/extension/loader.go`

**Loader 结构体新增字段：**

```go
type Loader struct {
    taskHandlerRegistry *TaskHandlerRegistry
    siteBrowserRegistry *SiteBrowserRegistry
    processes           map[string]*pluginEntry
    processGroup        *ProcessGroup // 新增：进程分组管理器
    mu                  sync.RWMutex
}
```

**NewLoader 中初始化：**

```go
func NewLoader(...) *Loader {
    pg, err := NewProcessGroup()
    if err != nil {
        logger.Log.Warnf("创建进程分组失败（不影响功能）: %v", err)
    }
    return &Loader{
        // ...
        processGroup: pg,
    }
}
```

**LoadPluginProcess 中分配子进程到 Job Object：**

在 `client.Client()` 成功返回后（此时 `cmd.Process` 已填充），将子进程加入 Job Object：

```go
// client.Client() 已返回，cmd.Process 已填充
if l.processGroup != nil && cmd.Process != nil {
    if err := l.processGroup.Assign(cmd.Process.Pid); err != nil {
        logger.Log.Warnf("将插件进程 %s 加入 Job Object 失败: %v", pluginPublicId, err)
    }
}
```

**UnloadAll 中清理：**

```go
func (l *Loader) UnloadAll() []string {
    // ... 现有逻辑 ...
    if l.processGroup != nil {
        l.processGroup.Close()
    }
    return ids
}
```

### 4. 升级 `golang.org/x/sys` 为直接依赖

当前 `golang.org/x/sys` 是间接依赖（由 hashicorp/go-plugin 引入）。Windows 实现需要直接引用 `golang.org/x/sys/windows`，需在 `go.mod` 中升级为直接依赖：

```bash
go get golang.org/x/sys
```

## 修改文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `backend/plugin/extension/process_group_windows.go` | 新建 | Windows Job Object 实现 |
| `backend/plugin/extension/process_group_other.go` | 新建 | 非 Windows 平台空实现 |
| `backend/plugin/extension/loader.go` | 修改 | 集成 ProcessGroup |
| `go.mod` / `go.sum` | 修改 | 升级 x/sys 为直接依赖 |

## 设计要点

1. **优雅降级**：Job Object 创建失败仅打印警告日志，不阻塞插件加载流程
2. **平台隔离**：通过构建标签隔离 Windows API 调用，非 Windows 平台零开销
3. **KILL_ON_JOB_CLOSE**：主进程退出时自动清理所有插件子进程，作为 `UnloadAll()` 的兜底保障
4. **无侵入性**：仅修改主程序，不需要修改插件 SDK 或现有插件
