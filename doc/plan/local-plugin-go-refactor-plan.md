# 本地文件导入插件 Go 重构计划

> **状态**：待审核
> **前置条件**：插件系统 gRPC 迁移已完成（hashicorp/go-plugin + gRPC）

## 需求概述

将旧版 JavaScript 本地文件导入插件（`library-squirrel-plugin-local`）完全重写为 Go 子进程插件，使其兼容当前 Go/Wails 主程序的插件系统。

核心需求：
1. **`Create` 方法**：插件自选批量或流式，主程序保留两种 handler
2. **路径语义交互**：通过 PluginContext 新增的前后端通信方法，在扫描过程中实时与 Slot 交互（替代旧版 `explainPath`）
3. **前端 UI**：通过 Slot 系统注册交互式分类面板

## 关键设计决策

### 1. Create 方法：插件自选批量或流式

`Create` 返回 `*TaskCreateResult`，插件通过 `BatchResult(array)` 或 `StreamResult(channel)` 构造。主程序保留 `handleCreateTaskArray` 和 `handleCreateTaskStream`，根据返回类型选择。

详见"Create 方法全链路改动"章节。

### 2. 插件前后端通信

**问题**：旧版 `explainPath()` 是主程序面向插件提供的专用接口，违反"主程序不能面向插件开发"的原则。但插件确实需要前后端交互能力。

**方案：PluginContext 新增两个通用通信方法，主程序侧直接桥接 Wails Events**

不引入 EventBus 抽象层。HostService 新增 `PublishToFrontend` / `SubscribeFrontend` 两个 RPC，主程序侧直接调用 `wails.Events.Emit` / `runtime.EventsOn` 实现。前端 Slot 直接使用 Wails 原生 `Events.On` / `Events.Emit`。

- **最小改动**：只新增两个 HostService RPC + 对应实现，不迁移现有代码
- **不引入新抽象**：现有 `WailsSlotPusher` 和 `WailsTaskProgressPusher` 保持不变（它们的领域专用接口比通用 pub/sub 类型安全性更高）
- **复用现有基础设施**：底层就是 Wails Events，Slot 前端直接用原生 API

数据流：
```
插件 → 前端：
  PublishToFrontend(topic, data)
    → gRPC HostService.PublishToFrontend
      → wails.Events.Emit(topic, data)
        → Slot Events.On(topic, callback)

前端 → 插件：
  Slot Events.Emit(topic, data)
    → 主程序 runtime.EventsOn(topic, handler)
      → 匹配已注册的 gRPC stream
        → 插件 SubscribeFrontend 的 <-chan 收到 data
```

详见"插件前后端通信"章节。

### 3. 扫描时交互型 explainPath

**流程**：
1. 插件注册一个 Slot（embed 类型，嵌入任务创建视图）
2. 用户输入本地路径 → 主程序调用插件 `Create()`
3. 插件扫描目录，遇到未分类目录时暂停
4. 通过 `PublishToFrontend` 向 Slot 发送分类请求，阻塞等待 `SubscribeFrontend` 的响应
5. Slot 展示分类 UI，用户选择后通过 `Events.Emit` 发送响应
6. 插件收到响应，继续扫描（相似目录自动应用已学规则）
7. 插件通过 channel 流式产出 `TaskCreateResponse`

### 4. 文件与目录语义策略

- **单个文件 = 一个 Work**：每个文件对应一个独立的作品，`siteWorkId` 为文件内容 SHA-256
- **目录层级 = 语义继承**：目录的父子层级代表含义的继承关系，同层级目录共享父级已确定的语义
- **语义来源**：目录的含义（作者、标签、作品名等）通过 `PublishToFrontend`/`SubscribeFrontend` 交互从用户处获取，用户对某一层级的分类会自动应用到同级所有目录

### 5. SHA-256 作为 siteWorkId

延续旧版方案，用文件内容 SHA-256 哈希作为 `siteWorkId`，确保同一文件不重复导入。

## 插件前后端通信

### Proto 变更（HostService）

```protobuf
service HostService {
    // ... 现有 RPC ...

    // 插件前后端通信
    rpc PublishToFrontend(PublishToFrontendRequest) returns (Empty);
    rpc SubscribeFrontend(SubscribeFrontendRequest) returns (stream FrontendMessage);
    rpc UnsubscribeFrontend(UnsubscribeFrontendRequest) returns (Empty);
}

message PublishToFrontendRequest {
    string topic = 1;
    bytes data = 2;
}

message SubscribeFrontendRequest {
    string topic = 1;
}

message FrontendMessage {
    string topic = 1;
    bytes data = 2;
}

message UnsubscribeFrontendRequest {
    string topic = 1;
}
```

`SubscribeFrontend` 使用 server-streaming：主程序通过 `runtime.EventsOn` 监听前端事件，推送到 gRPC stream。插件在 Activate 时建立订阅，保持到 Shutdown。

### PluginContext 新增方法

```go
type PluginContext interface {
    // ... 现有方法 ...

    // 发布事件到前端
    PublishToFrontend(topic string, data []byte) error
    // 订阅前端事件，返回消息 channel
    SubscribeFrontend(topic string) (<-chan []byte, error)
    // 取消订阅
    UnsubscribeFrontend(topic string) error
}
```

### 主程序侧实现

在 `HostServiceServer` 中新增三个 handler，直接桥接 Wails Events，不引入中间抽象：

```go
// PublishToFrontend 插件 → 前端
func (s *HostServiceServer) PublishToFrontend(ctx context.Context, req *gen.PublishToFrontendRequest) (*gen.Empty, error) {
    // 直接调用 wails.Events.Emit
    s.emitter.Emit(req.Topic, req.Data)
    return &gen.Empty{}, nil
}

// SubscribeFrontend 前端 → 插件（长连接 stream）
func (s *HostServiceServer) SubscribeFrontend(req *gen.SubscribeFrontendRequest, stream gen.HostService_SubscribeFrontendServer) error {
    ch := make(chan []byte, 16)
    // 通过 runtime.EventsOn 监听前端事件
    cancel := s.eventListener.On(req.Topic, func(data []byte) {
        ch <- data
    })
    defer cancel()

    for {
        select {
        case data := <-ch:
            if err := stream.Send(&gen.FrontendMessage{Topic: req.Topic, Data: data}); err != nil {
                return err
            }
        case <-stream.Context().Done():
            return nil
        }
    }
}
```

具体实现需要从 `wailsEventEmitter`（用于 Go→JS）和 `runtime.EventsOn`（用于 JS→Go）两个方向桥接。`emitter` 已有（`WailsEventEmitter`），`eventListener` 需要新增（封装 `runtime.EventsOn`）。

### 插件侧实现

```go
// context_client.go
func (c *PluginContextClient) PublishToFrontend(topic string, data []byte) error {
    _, err := c.hostClient.PublishToFrontend(context.Background(), &gen.PublishToFrontendRequest{
        Topic: topic,
        Data:  data,
    })
    return err
}

func (c *PluginContextClient) SubscribeFrontend(topic string) (<-chan []byte, error) {
    stream, err := c.hostClient.SubscribeFrontend(context.Background(), &gen.SubscribeFrontendRequest{Topic: topic})
    if err != nil {
        return nil, err
    }
    ch := make(chan []byte, 16)
    go func() {
        defer close(ch)
        for {
            msg, err := stream.Recv()
            if err != nil { return }
            ch <- msg.Data
        }
    }()
    return ch, nil
}
```

### 改动文件清单

| 文件 | 改动类型 | 说明 |
|------|---------|------|
| `plugin-sdk/proto/plugin.proto` | 修改 | HostService 新增 3 个 RPC |
| `plugin-sdk/gen/*` | 重新生成 | protoc |
| `plugin-sdk/context.go` | 修改 | PluginContext 新增 3 个方法 |
| `plugin-sdk/context_client.go` | 修改 | PluginContextClient 实现新方法 |
| `plugin-sdk/host.go` | 修改 | HostServiceServer 新增 3 个 handler + 新增 `emitter`/`eventListener` 依赖 |
| `backend/plugin/extension/plugin_context.go` | 修改 | 注入 emitter/eventListener 到 HostServiceServer |

**不改动**：
- `wails_pusher.go` — 保持现有 SlotPusher
- `progress_pusher.go` — 保持现有 TaskProgressPusher
- 前端 — Slot 直接用 Wails 原生 `Events.On` / `Events.Emit`，不封装 composable

## Create 方法全链路改动

### Proto

将 unary `Create` 改为 server-streaming：

```protobuf
// 改动前
rpc Create(CreateRequest) returns (CreateResponse);
message CreateResponse { repeated TaskCreateResponse responses = 1; }

// 改动后
rpc Create(CreateRequest) returns (stream CreateChunk);

message CreateChunk {
    oneof payload {
        CreateMode mode = 1;              // 首条消息：模式标记
        TaskCreateResponse task = 2;      // 后续消息：任务数据
        string error = 3;
    }
}
message CreateMode {
    bool is_stream = 1;                   // true=流式, false=批量
}
```

### SDK TaskHandler 接口

```go
type TaskHandler interface {
    Create(url string) (*TaskCreateResult, error)
    CreateWorkInfo(task *Task) (*WorkResponse, error)
    Start(task *Task) (io.ReadCloser, *WorkResponse, error)
    Retry(task *Task) (*WorkResponse, error)
    Pause(param *TaskResParam) error
    Stop(param *TaskResParam) error
    Resume(param *TaskResParam) (*WorkResponse, error)
}

// TaskCreateResult 任务创建结果
type TaskCreateResult struct {
    array  []*TaskCreateResponse
    stream <-chan *TaskCreateResponse
}
func BatchResult(responses []*TaskCreateResponse) *TaskCreateResult
func StreamResult(ch <-chan *TaskCreateResponse) *TaskCreateResult
func (r *TaskCreateResult) IsStream() bool
func (r *TaskCreateResult) Array() []*TaskCreateResponse
func (r *TaskCreateResult) Stream() <-chan *TaskCreateResponse
```

### SDK gRPC Server

`Create` handler 根据 `TaskCreateResult` 类型发送模式标记，然后逐条发送任务数据。

### TaskHandlerProxy

读首条 `CreateChunk` 的 mode，批量模式收集到数组返回 `BatchResult`，流式模式创建 channel 返回 `StreamResult`。

### task.Service.CreateTaskByURL

```go
result, err := taskHandler.Create(url)
if result.IsStream() {
    streamCh, _ := s.handleCreateTaskStream(ctx, result.Stream(), listener, 100)
    count = s.collectStreamResults(streamCh)
} else {
    count, _ = s.handleCreateTaskArray(ctx, result.Array(), url, listener)
}
```

### 改动文件清单

| 文件 | 改动类型 | 说明 |
|------|---------|------|
| `plugin-sdk/proto/plugin.proto` | 修改 | `Create` 改为 streaming，新增 `CreateChunk`/`CreateMode` |
| `plugin-sdk/task_handler.go` | 修改 | `Create` 返回 `*TaskCreateResult` |
| `plugin-sdk/plugin.go` | 修改 | gRPC server/client `Create` 改为 streaming |
| `backend/base/model/dto/task_handler.go` | 修改 | `Create` 返回 `*TaskCreateResult` |
| `backend/plugin/extension/task_handler_proxy.go` | 修改 | `Create` 根据 mode 返回对应 result |
| `backend/plugin/extension/convert.go` | 新增 | `TaskCreateResponse` proto↔DTO |
| `backend/task/service.go` | 修改 | `CreateTaskByURL` 根据 `IsStream()` 分支 |

现有插件（如 pixiv）适配：`return results, nil` → `return BatchResult(results), nil`。

## 插件本体实现

### 项目结构

```
library-squirrel-plugin-local/
├── go.mod
├── go.sum
├── main.go                       # 入口：pluginsdk.Serve()
├── task_handler.go               # TaskHandler 实现
├── scanner.go                    # 目录扫描与分组
├── path_rule.go                  # 路径规则 + 扫描时交互逻辑
├── file_hash.go                  # SHA-256 哈希计算
├── views/                        # Slot 前端资源
│   └── classify/
│       └── ClassifyPanel.vue     # 目录分类交互面板
├── plugin.json
└── build.ps1
```

### 核心流程：扫描时交互

```go
// path_rule.go
type PathClassifier struct {
    ctx           pluginsdk.PluginContext
    learnedRules  map[int]string      // level → type（用户已分类的规则）
    responseCh    chan *ClassifyResponse
}

func (c *PathClassifier) ClassifyDir(level int, dirName string) (string, error) {
    // 1. 检查已学规则
    if rule, ok := c.learnedRules[level]; ok {
        return rule, nil
    }

    // 2. 询问前端 Slot
    question := &ClassifyQuestion{
        Level:   level,
        DirName: dirName,
        Options: []string{"author", "tag", "workName", "workSet"},
    }
    data, _ := json.Marshal(question)
    c.ctx.PublishToFrontend("plugin:local-import:classify:request", data)

    // 3. 等待用户响应（阻塞）
    select {
    case resp := <-c.responseCh:
        c.learnedRules[level] = resp.Type
        return resp.Type, nil
    case <-time.After(5 * time.Minute):
        return "workName", nil
    }
}
```

```go
// main.go — Activate 中订阅前端响应
pluginsdk.WithActivate(func(ctx pluginsdk.PluginContext) {
    ctx.RegisterUrlListener("local-import", buildUrlPatterns())
    handler.ctx = ctx

    ch, _ := ctx.SubscribeFrontend("plugin:local-import:classify:response")
    go func() {
        for data := range ch {
            var resp ClassifyResponse
            json.Unmarshal(data, &resp)
            handler.classifier.responseCh <- &resp
        }
    }()
})
```

### Slot 前端（ClassifyPanel.vue）

直接使用 Wails 原生 `Events.On` / `Events.Emit`：

```vue
<script setup lang="ts">
import { Events } from '@wailsio/runtime'

const question = ref(null)

onMounted(() => {
    Events.On('plugin:local-import:classify:request', (event: any) => {
        question.value = event.data
    })
})

function selectType(type: string) {
    Events.Emit('plugin:local-import:classify:response', {
        level: question.value.level,
        dirName: question.value.dirName,
        type,
    })
    question.value = null
}
</script>
```

### plugin.json

```json
{
  "id": "local-import",
  "name": "本地文件导入",
  "version": "1.0.0",
  "author": "lvfeng",
  "description": "从本地路径批量导入文件到资源库",
  "entryFile": "local-import.exe",
  "activation": { "type": 1 },
  "extensions": {
    "taskHandlers": [{
      "id": "local-import",
      "name": "本地导入",
      "description": "从本地路径导入文件"
    }],
    "slots": [{
      "id": "classify-panel",
      "name": "目录分类",
      "slotType": "embed",
      "contentType": "vueSource",
      "content": { "entry": "views/classify/ClassifyPanel.vue" },
      "position": "task-create-toolbar"
    }],
    "staticResources": {
      "directories": ["views/"]
    }
  }
}
```

### 其他 TaskHandler 方法

**Create(url)**：返回 `StreamResult(channel)`，扫描过程中通过 `PublishToFrontend` 与 Slot 交互，每处理完一个叶目录产出一条 `TaskCreateResponse`。

**CreateWorkInfo(task)**：从 `task.PluginData` 反序列化路径元数据，构建 `WorkResponse`。

**Start(task)**：`os.Open()` 打开文件，返回 `file`（io.ReadCloser）+ `WorkResponse`。

**Retry(task)**：委托到 Start。

**Pause/Stop(param)**：从 `sync.Map` 查找并关闭文件句柄。

**Resume(param)**：从偏移量重新打开文件。

### 边界条件

| 场景 | 处理方式 |
|------|---------|
| 空目录 | channel 立即关闭 |
| 单文件 | 一个 child task |
| 超大目录 | 流式逐步产出 |
| 权限不足 | 跳过不可读文件/目录 |
| 文件被占用 | `Start()` 返回错误 |
| 分类超时（5 分钟） | 使用默认规则（目录名 = 作品名） |
| 用户取消分类 | 插件收到取消消息，停止扫描 |
| Create 中途错误 | 关闭 channel，主程序检测到 |
| Slot 未挂载时发送分类请求 | 超时后使用默认规则 |

## 开发步骤

### Phase 0: 插件前后端通信

1. `plugin-sdk/proto/plugin.proto` — HostService 新增 `PublishToFrontend` / `SubscribeFrontend` / `UnsubscribeFrontend`
2. `plugin-sdk/gen/*` — protoc 重新生成
3. `plugin-sdk/context.go` — PluginContext 新增 3 个方法声明
4. `plugin-sdk/context_client.go` — PluginContextClient 实现
5. `plugin-sdk/host.go` — HostServiceServer 新增 3 个 handler，依赖 `emitter` + `eventListener`
6. `backend/plugin/extension/plugin_context.go` — 注入 emitter/eventListener
7. 验证：编译通过

### Phase 1: Create 全链路改动

1. `plugin.proto` — `Create` 改为 streaming，新增 `CreateChunk`/`CreateMode`
2. `plugin-sdk` — `TaskHandler.Create` 返回 `*TaskCreateResult`
3. `plugin-sdk/plugin.go` — gRPC server/client 改为 streaming
4. 后端 DTO、Proxy、Service 同步修改
5. pixiv 插件适配：`BatchResult(results)`
6. 验证：编译通过，现有插件正常工作

### Phase 2: 插件项目骨架

1. 清理旧仓库，初始化 Go 模块
2. `main.go` 入口（`pluginsdk.Serve` + `SubscribeFrontend`）
3. `plugin.json`（含 Slot 声明）
4. `build.ps1` 构建脚本
5. 验证：编译通过，主程序加载成功

### Phase 3: 插件核心功能

1. `scanner.go` — 目录扫描、分组
2. `path_rule.go` — PathClassifier + `PublishToFrontend`/`SubscribeFrontend` 交互
3. `file_hash.go` — SHA-256 计算
4. `task_handler.go` — 完整 TaskHandler 实现
5. `views/classify/ClassifyPanel.vue` — Slot 前端
6. 验证：完整流程测试

### Phase 4: 验证与完善

1. 集成测试
2. 错误处理
3. 构建打包

## 验收标准

### Phase 0（前后端通信）
- [ ] HostService 3 个新 RPC 的 proto 定义正确，protoc 生成通过
- [ ] PluginContext 新增 3 个方法通过 HostService gRPC 正常工作
- [ ] `PublishToFrontend` 调用后前端 `Events.On` 能收到消息
- [ ] 前端 `Events.Emit` 后插件 `SubscribeFrontend` channel 能收到消息
- [ ] 插件进程退出后 gRPC stream 正确关闭

### Phase 1（Create）
- [ ] Proto `Create` 改为 streaming
- [ ] `TaskCreateResult` 类型、`BatchResult`/`StreamResult` 构造函数
- [ ] 主程序根据 `IsStream()` 分支调用两个 handler
- [ ] pixiv 插件使用 `BatchResult` 正常工作

### Phase 2-3（插件本体）
- [ ] 插件加载、URL 监听器匹配
- [ ] Create 流式产出任务
- [ ] 扫描过程中通过 `PublishToFrontend` / `SubscribeFrontend` 与 Slot 实时交互
- [ ] 用户分类后，相似目录自动应用已学规则
- [ ] Start/Pause/Stop/Resume 正常工作
- [ ] SHA-256 正确计算

### Phase 4
- [ ] 边界条件正确处理
- [ ] 无资源泄漏

## 预计工作量

| 阶段 | 工作量 | 说明 |
|------|--------|------|
| Phase 0: 前后端通信 | 1.5-2 天 | Proto + PluginContext + HostService handler |
| Phase 1: Create 改动 | 2-3 天 | Proto + SDK + Proxy + Service + pixiv 适配 |
| Phase 2: 骨架 | 0.5-1 天 | |
| Phase 3: 核心功能 | 2-3 天 | 扫描、交互、Slot 前端 |
| Phase 4: 验证完善 | 1-2 天 | |
| **合计** | **7-11 天** | |

## 后续扩展（不在本期范围）

1. **路径规则持久化**：用户分类的规则保存到 PluginData，下次导入自动应用
2. **增量导入**：SHA-256 匹配跳过重复文件
3. **文件类型过滤**：按扩展名过滤
4. **导入预览**：Create 后展示解析结果，用户确认后执行
