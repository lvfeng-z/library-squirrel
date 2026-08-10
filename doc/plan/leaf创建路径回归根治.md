# 单文件/独立任务（leaf）创建路径回归根治

> 任务图节点 N（reveal 自 A「插件约定发布准备」，G audio 端到端测试发现）；已激活为 pre-release-decision-lock 节点 F，2026-08-10 实施完成。与插件约定正交，属 task 基建。

## 审查摘要

**关键声明（抽查项）：**
- 声明1：`handleCreateTaskStream` 对无 `Children` 的响应 `continue` 跳过（`backend/task/service.go:771-774`）；`handleCreateTaskArray` 经 `assignTask` 处理单 response（`service.go:605`）。双路径对 leaf（无 children 的独立任务响应）处理不对称。
- 声明2：local 单文件导入发 leaf 响应（不设 `Children`，`library-squirrel-plugin-local/task_handler.go:134-143`），走 stream 被跳过 → count=0 静默失败（service.go 该分支不记日志）。
- 声明3：`backend/task` 下创建路径（`CreateTaskByURL`/`handleCreateTaskStream`/`handleCreateTaskArray`）**零单测**（`grep *_test.go` 无匹配）。
- 声明4：`len(children) == 0` 判断由 `8a43ee3 refactor` 引入（`git log -S` 证据）；独立任务（pid=0 leaf）多次回归：执行（`1fc3bf1`）、暂停/停止（`eeb27f7` + memory `standalone-task-pause-regression`）。
- 临时修复：`handleCreateTaskStream` 已加 leaf 分支（`service.go` 当前），把无 Children 的响应作为独立叶子任务创建。**本临时修复是治标，非根治**。

**已决策（2026-08-10，节点 F 实施）：**
- 决策1：根治范围全做——机制层（提取 `planCreateResponse`/`fillTaskFromResponse` 单点判定 leaf/parent/child 三态）+ 守护层（`backend/task/service_create_test.go` 8 例回归测试）+ 三处入口不变量注释 + memory `leaf-task-regression-hotspot`。统一契约：无 Children→独立 leaf、Children=[1] 不折叠、leaf pid=0 Valid=true、计数=叶子单元；stream 保留合并、键=PluginTaskId（非 TaskName）。详见 `doc/plugin-dev-guide.md`「Create 返回的任务结构契约」。

**自曝风险：**
- 风险1：提取 `applyCreateResponse` 重构创建路径（核心高频逻辑），若测试未先行可能引入新回归；须「测试先行 + 小步重构 + 每步编译测试」。

---

## 根因（为什么反复回归）

1. **leaf/独立任务是横切多维度的特殊形态，却无单点权威实现**：创建（stream/array）、执行、暂停/停止、状态推送——每维度各自编码 leaf 处理。重构任一维度只看该维度"正常路径"（parent+children），leaf 被遗漏，他维度不感知。
2. **handleCreateTaskStream / handleCreateTaskArray 双路径不对称**：array 认 leaf（assignTask），stream 不认（要求 Children）。两套并行创建逻辑，leaf 处理不一致，无"两路径行为必须对齐"约束。
3. **`continue` 静默吞没**：leaf 被跳过时不报错、不记日志，前端只显通用失败。bug 静默，只能手动测单文件发现。
4. **核心任务逻辑零测试**：无"创建 leaf""创建 parent+children"回归测试，重构破坏 leaf 编译过、CI 绿、测试全过，直到用户单文件导入才暴露。

## 根治方案（三层）

| 层 | 措施 | 解决 |
|---|---|---|
| 机制 | 提取 `applyCreateResponse(taskResp, listener, pid)` 公共函数，stream 与 array 都调用——leaf/parent/child 三态在**单点**判定与创建 | 消除双路径不对称，leaf 只有一个权威实现 |
| 守护 | 创建路径回归测试：`TestHandleCreateTaskStream_Leaf`/`_ParentChildren`、`TestHandleTaskArray_Leaf`，覆盖 stream×{leaf,parent+children} 与 array×leaf | 重构破坏 leaf 时测试立即红 |
| 不变量+记忆 | 创建/执行/控制三处入口注释「须覆盖 leaf(pid=0)，参见 memory」；memory `leaf-task-regression-hotspot` | 跨会话防遗忘，与 `standalone-task-pause-regression` 形成 leaf 回归完整档案 |

## 实施步骤（落地时）

1. **测试先行**：为当前（已临时修复）创建路径加回归测试，固化 leaf/parent+children × stream/array 期望行为。
2. **提取公共函数**：`applyCreateResponse` 收敛 leaf/parent/child 创建，stream/array 共享。
3. **不变量注释 + memory**：三处入口注释 + memory 警示。
4. **验证**：全编译 + 单测 + 端到端（单文件导入 audio/image/video、目录导入、pixiv/bilibili 创建）。
