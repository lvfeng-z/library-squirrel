# 本地文件导入插件重构计划

> **已废弃**：此方案（将本地插件重写为 Go 子进程插件）已被多语言插件系统方案取代。详见 [multi-language-plugin-system-plan.md](./multi-language-plugin-system-plan.md)。

## 需求概述

将旧版 JavaScript/Electron 本地文件导入插件 (`library-squirrel-plugin-local`) 完全重写为 Go 子进程插件，使其兼容当前 Go/Wails 主程序的插件系统（基于 JSON-RPC 2.0 over Unix Domain Socket 的子进程架构）。

## 需求详情

### 功能列表

1. **目录扫描**：递归扫描用户指定的本地目录，发现所有文件
2. **任务创建**：将扫描结果构建为父子任务结构（父=整个导入，子=每个文件）
3. **文件流式传输**：通过 `Start` 方法的二进制帧协议将本地文件流式传输到资源库
4. **断点续传**：支持 Pause/Stop/Resume 操作
5. **URL 监听**：注册 Windows/Unix 本地路径的 URL 监听器
6. **路径语义**：支持将目录路径映射为作品元数据（作者、标签、作品名、作品集等）

### 用户场景

1. 用户在主程序输入框输入本地路径（如 `D:\manga\author1\work1`）
2. 主程序通过 URL 监听器匹配到本地导入插件
3. 插件递归扫描目录，构建父子任务
4. 主程序展示任务列表，用户点击开始
5. 插件将文件内容流式传输到资源库目录
6. 支持 Pause/Stop/Resume 控制

### 数据模型

无新增实体。插件使用 SDK 定义的现有 DTO：
- `TaskCreateResponse` + `TaskCreateChildResponse` — 任务创建
- `WorkResponse` — 作品信息返回
- `TaskResourceDTO` — 资源元数据

### 边界条件

- **空目录**：`Create()` 返回空数组，主程序显示"无文件可导入"
- **单文件路径**：只有一个子任务，主程序会跳过父任务创建（`handleCreateTaskArray` 中 `len(children)==1` 的逻辑）
- **超大目录**（>10000 文件）：`Create()` 是同步 RPC 调用，大目录可能耗时较长。需要考虑扫描效率（使用 `filepath.WalkDir` 而非 `filepath.Walk`）
- **权限不足的目录**：跳过不可读的文件/目录，记录警告日志
- **文件被占用**：`Start()` 打开文件失败时返回错误
- **路径格式**：同时支持 Windows 路径（`C:\...`、`\\server\share\...`）和 Unix 路径（`/home/...`）

## 技术方案

### 涉及模块

新建独立仓库（或复用旧仓库目录）：
```
library-squirrel-plugin-local/
├── go.mod                    # 依赖 plugin-sdk
├── go.sum
├── main.go                   # 入口：Activate 函数
├── task_handler.go           # TaskHandler 实现
├── scanner.go                # 目录扫描逻辑
├── path_meaning.go           # 路径语义解析
├── plugin.json               # 插件清单
└── build.ps1                 # 构建脚本
```

### 依赖关系

```
library-squirrel-plugin-local (Go)
  └── github.com/lvfeng-z/library-squirrel-plugin-sdk (本地 replace)
```

### 技术要点

#### 1. 插件进程生命周期

插件作为独立可执行文件运行，通过 `--socket` 参数接收 UDS 路径：

```go
func main() {
    socketPath := flag.String("socket", "", "Unix domain socket path")
    flag.Parse()

    conn, err := net.Dial("unix", *socketPath)
    // ... 建立帧编解码器、RPC 客户端/服务端
    // ... 等待 activate 通知
    // ... 注册 TaskHandler、URL 监听器
    // ... 进入 RPC 分发循环
}
```

#### 2. TaskHandler.Create — 目录扫描与任务构建

旧版使用 Node.js Readable 流逐步 yield 任务。新版返回 `[]*pluginsdk.TaskCreateResponse`：

```go
func (h *LocalTaskHandler) Create(url string) ([]*pluginsdk.TaskCreateResponse, error) {
    // 1. 解析路径
    dirPath := parseLocalPath(url)

    // 2. 递归扫描目录
    files, err := scanDirectory(dirPath)

    // 3. 按目录分组（每个目录 = 一个 Work）
    groups := groupFilesByDirectory(files)

    // 4. 构建父子任务响应
    var responses []*pluginsdk.TaskCreateResponse
    for _, group := range groups {
        children := make([]*pluginsdk.TaskCreateChildResponse, 0, len(group.Files))
        for _, f := range group.Files {
            children = append(children, &pluginsdk.TaskCreateChildResponse{
                TaskName:   filepath.Base(f.Path),
                SiteWorkID: f.SHA256,
                URL:        f.Path,
                PluginData: serializePluginData(f, group),
                SiteName:   SiteNameLocal,
            })
        }
        responses = append(responses, &pluginsdk.TaskCreateResponse{
            PluginTaskID: generatePluginTaskID(group.Dir),
            TaskName:     fmt.Sprintf("从本地路径【%s】创建的导入", group.Dir),
            URL:          url,
            SiteName:     SiteNameLocal,
            Children:     children,
        })
    }
    return responses, nil
}
```

**关键差异**：旧版 `create()` 是流式返回（Readable stream），新版是同步批量返回。对于大目录，所有文件信息在内存中构建完成后一次性返回。

#### 3. TaskHandler.Start — 文件流式传输

```go
func (h *LocalTaskHandler) Start(task *pluginsdk.Task) (io.ReadCloser, *pluginsdk.WorkResponse, error) {
    // 1. 解析任务中的文件路径
    filePath := *task.URL

    // 2. 打开文件
    file, err := os.Open(filePath)

    // 3. 获取文件信息
    info, _ := file.Stat()

    // 4. 解析 pluginData 获取作品元数据
    workResp := buildWorkResponse(task, info)

    // 5. 返回文件读取器（主程序通过 StreamReader 读取二进制帧）
    return file, workResp, nil
}
```

#### 4. 路径语义解析（替代 explainPath）

旧版 `explainPath()` 是同步 RPC 调用，在扫描过程中逐个目录询问用户。新版采用**配置化方案**：

**方案：基于规则的路径解析**

通过插件数据（`GetPluginData`/`SetPluginData`）存储路径规则配置：

```json
{
  "rules": [
    {"level": 0, "type": "author"},
    {"level": 1, "type": "workName"},
    {"level": 2, "type": "tag"}
  ],
  "defaultRule": {"type": "workName"}
}
```

- Level 0 = 根目录的直接子目录
- Level 1 = 再下一级
- 以此类推

规则为空时，使用默认约定：**目录名 = 作品名，所有文件 = 作品的资源**。

后续可扩展为通过 Slot UI 让用户可视化配置路径规则。

#### 5. URL 监听器

```go
// Activate 中注册
ctx.RegisterUrlListener("main", []string{
    // Windows 路径（盘符或 UNC）
    `^(?:[a-zA-Z]:\\|\\\\[^\\/:*?"<>|\r\n]+\\[^\\/:*?"<>|\r\n]+)(?:[^\\/:*?"<>|\r\n]+\\)*[^\\/:*?"<>|\r\n]*(?:\.[^\\/:*?"<>|\r\n]+)?$`,
    // Unix 绝对路径
    `^/(?:[^/\0\n\r<>:"|?*]+/)*?(?:[^/\0\n\r<>:"|?*]+(?:\.[^/\0\n\r<>:"|?*\\.]*)?)?$`,
})
```

#### 6. SHA-256 哈希

复用旧版的核心逻辑，使用 Go 标准库：

```go
func calculateSHA256(filePath string) (string, error) {
    file, err := os.Open(filePath)
    if err != nil {
        return "", err
    }
    defer file.Close()

    hash := sha256.New()
    if _, err := io.Copy(hash, file); err != nil {
        return "", err
    }
    return hex.EncodeToString(hash.Sum(nil)), nil
}
```

#### 7. Pause/Stop/Resume

```go
type LocalTaskHandler struct {
    ctx      pluginsdk.PluginContext
    streams  sync.Map // taskID -> *os.File
}

func (h *LocalTaskHandler) Pause(param *pluginsdk.TaskResParam) error {
    // 关闭当前文件句柄（记录已读取字节数）
    if f, ok := h.streams.Load(taskID); ok {
        f.(*os.File).Close()
        h.streams.Delete(taskID)
    }
    return nil
}

func (h *LocalTaskHandler) Resume(param *pluginsdk.TaskResParam) (*pluginsdk.WorkResponse, error) {
    // 从 resourcePath（已传输字节数）处继续读取
    file, err := os.Open(filePath)
    file.Seek(resourcePath, io.SeekStart)
    // ... 构建 WorkResponse
    return workResp, nil
}
```

### plugin.json

```json
{
  "id": "com.lvfeng.localImport_<UUID>",
  "name": "localImport",
  "version": "1.0.0",
  "author": "lvfeng",
  "description": "本地文件批量导入插件",
  "entryFile": "local_import_plugin.exe",
  "activation": {"type": 1},
  "extensions": {
    "taskHandlers": [
      {"id": "main", "name": "本地导入", "description": "从本地路径导入文件"}
    ],
    "staticResources": {
      "directories": []
    }
  }
}
```

### 与旧版的关键差异

| 维度 | 旧版 | 新版 |
|------|------|------|
| 语言 | JavaScript (.mjs) | Go |
| 进程模型 | 主进程内加载 | 独立子进程 |
| 通信 | 直接函数调用 | JSON-RPC 2.0 over UDS |
| 流式传输 | Node.js Readable stream | io.ReadCloser + 二进制帧 |
| 任务创建 | 流式 yield | 同步返回 `[]*TaskCreateResponse` |
| 路径语义 | `explainPath()` 交互式 | 基于规则的配置化 |
| 清单 | `pluginInfo.yml` | `plugin.json` |
| 作品元数据 | 自定义 DTO 镜像 | SDK 标准化 DTO |
| 哈希算法 | SHA-256（Node.js crypto） | SHA-256（Go crypto/sha256） |

## 开发步骤

### Phase 1: 项目骨架与基础设施

1. **初始化 Go 模块**
   - 创建 `go.mod`，依赖 `library-squirrel-plugin-sdk`
   - 配置 `replace` 指令指向本地 SDK

2. **实现 main.go — 子进程入口**
   - 解析 `--socket` 参数
   - 连接 UDS
   - 创建 `FrameCodec`、`RPCClient`、`RPCServer`
   - 等待 `activate` 通知
   - 在 `Activate` 中注册 TaskHandler 和 URL 监听器
   - 启动 RPC 分发循环

3. **创建 plugin.json**
   - 定义插件 ID、TaskHandler 声明
   - 定义 URL 监听器正则（通过 `RegisterUrlListener` 动态注册）

4. **创建构建脚本**
   - 编译为 Windows 可执行文件
   - 打包为插件目录结构

### Phase 2: 核心功能实现

1. **scanner.go — 目录扫描**
   - 使用 `filepath.WalkDir` 递归扫描
   - 计算每个文件的 SHA-256 哈希
   - 按目录分组
   - 处理权限错误（跳过 + 日志）

2. **path_meaning.go — 路径语义解析**
   - 实现基于规则的路径层级解析
   - 从插件数据读取配置
   - 默认规则：目录名 = 作品名
   - 序列化 MeaningOfPath 到 PluginData

3. **task_handler.go — TaskHandler 接口实现**
   - `Create(url)` — 扫描 + 构建任务响应
   - `CreateWorkInfo(task)` — 从 PluginData 提取作品元数据
   - `Start(task)` — 打开文件，返回 io.ReadCloser
   - `Retry(task)` — 委托到 Start
   - `Pause(param)` — 关闭文件句柄
   - `Stop(param)` — 关闭并清理
   - `Resume(param)` — 从偏移量重新打开文件

### Phase 3: 验证与完善

1. **集成测试**
   - 在主程序中安装插件
   - 测试各种路径输入（Windows、UNC、Unix）
   - 测试空目录、单文件、多级目录
   - 测试 Pause/Stop/Resume 流程
   - 测试大目录（>1000 文件）的性能

2. **错误处理完善**
   - 文件被占用时的错误信息
   - 路径不存在的处理
   - 磁盘空间不足的处理

3. **构建打包**
   - 确定 plugin ID（需要 UUID）
   - 确定打包路径规范
   - 编写自动化打包脚本

## 验收标准

- [ ] 插件可通过主程序的插件系统加载（`plugin.json` 解析成功、子进程启动成功）
- [ ] URL 监听器正确匹配 Windows/Unix 本地路径
- [ ] 输入本地路径后，`Create()` 正确返回父子任务结构
- [ ] `Start()` 能将本地文件通过二进制帧流式传输到主程序
- [ ] `Pause`/`Stop`/`Resume` 正常工作
- [ ] SHA-256 哈希作为 `siteWorkId` 唯一标识文件
- [ ] 目录名作为默认作品名
- [ ] `CreateWorkInfo()` 正确提取作品元数据（作者、标签、作品集等）
- [ ] `Resume` 能从断点位置继续传输
- [ ] 无内存泄漏（文件句柄正确关闭）

## 主程序侧可能的改动

当前 `TaskService.CreateTaskByURL` 调用 `taskHandler.Create(url)` 后使用 `handleCreateTaskArray` 处理结果。此流程已支持父子任务结构，**预计不需要修改主程序代码**。

但以下情况可能需要主程序配合：

1. **"local" 站点**：插件需要通过 `AddSite()` 创建 `siteName="local"` 的站点记录，或主程序需要预置此站点
2. **大目录性能**：如果 `Create()` 扫描大目录耗时过长（>30s），可能需要主程序增加超时容忍或支持流式 `Create`

## 预计工作量

| 阶段 | 工作量 | 说明 |
|------|--------|------|
| Phase 1: 骨架 | 1-2 天 | 参照 pixiv 插件模式，复用 SDK 的 RPC 框架 |
| Phase 2: 核心功能 | 2-3 天 | 扫描、哈希、任务构建、文件流传输 |
| Phase 3: 验证完善 | 1-2 天 | 集成测试、错误处理、打包 |
| **合计** | **4-7 天** | |

## 后续扩展（不在本期范围）

1. **路径语义配置 UI**：通过 Slot 注册配置面板，让用户可视化编辑路径规则
2. **增量导入**：检测已导入的文件（通过 SHA-256 匹配），跳过重复
3. **导入进度通知**：通过 RPC 通知主程序扫描进度
4. **文件类型过滤**：按扩展名过滤（仅导入图片/视频等）
