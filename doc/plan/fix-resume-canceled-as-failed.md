# 修复:跨重启续传被 setup Pause 中断后误判 Failed

> 状态:已实现(2026-07-06)。

## 现象

任务在 `resumeFromPersistedState`(跨重启续传)执行期间,用户暂停任务树命中 setup 阶段(streams 为空),`setup Pause` 取消任务 ctx,经 gRPC stream ctx(stream ctx 继承任务 ctx)传播,中断进行中的 `Resume` / derived `Start` RPC,RPC 返回 `context canceled`。当前代码将其置为 `Failed`,但用户意图是暂停,正确状态应为 `Paused`。

实测日志(任务 263、268):
```
任务 263 跨重启 Resume 失败: rpc error: code = Canceled desc = context canceled
任务状态变更 [...](263): Paused → Failed
```

## 根因

`resumeFromPersistedState` 的 RPC 失败分支直接 `setFailed`,未像 `runSectionCombo` 的 `comboFail` 那样检查 `abortedByPause`:

- `pluginExec.Resume` 失败:`setFailed("跨重启续传失败: ...")`
- derived `pluginExec.Start` 失败(未完成 derived 轨整轨重产):`setFailed("重产资源失败: ...")`

而 `comboFail`(runSectionCombo 失败处理)有 `if m.abortedByPause() { return runResultPaused }` 兜底。两处失败处理不一致,导致 Pause 取消 RPC 时跨重启续传任务误判 Failed。

深层原因与项一(reader 响应 ctx)的关联:项一让 gRPC stream ctx 继承任务 ctx(`proxy.Start/Resume` 用 `context.WithCancel(ctx)`),使 `setup Pause` 的 `cancel m.ctx` 能传播到进行中的 RPC(项一前 RPC 用 `context.Background()` 不受影响)。项一让取消信号正确传递,但 `resumeFromPersistedState` 未处理这种"进行中被取消"的情况,暴露了既有遗漏。

## 修复

`resumeFromPersistedState` 的两处 RPC 失败分支加 `abortedByPause` 检查:Pause(状态 Pausing/Paused)导致的 ctx 取消返回 `runResultPaused`,任务进 Paused;Stop(状态 Stopping)导致的取消仍走 `setFailed`(Stop 语义即终止)。

`abortedByPause` 的状态判定天然区分 Pause 与 Stop:
- Pause 流程置 Pausing/Paused → `abortedByPause` true → `runResultPaused`
- Stop 流程置 Stopping → `abortedByPause` false → `setFailed`(符合 Stop 终止语义)

## 改动点

- `backend/taskManager/model.go` `resumeFromPersistedState`:
  - `pluginExec.Resume` 失败分支:加 `abortedByPause` → `runResultPaused`
  - derived `pluginExec.Start` 失败分支:加 `abortedByPause` → `runResultPaused`

## 验证

高频启停下,任务在跨重启续传期间被暂停:
- 修复前:任务进 Failed(日志 `跨重启 Resume 失败: context canceled` + `Paused → Failed`),下次需用户手动重试
- 修复后:任务进 Paused,下次 Resume 正常按 diskOffset 续传

## 关联

- 触发场景由项一(reader 响应 ctx,`refactor-task-manager-actor-model.md` 项一)的 stream ctx 继承任务 ctx 暴露;但根因(`resumeFromPersistedState` 失败分支缺失 `abortedByPause`)是既有遗漏,独立于项一与项三。
- `comboFail` 的 `abortedByPause` 兜底是参照范式;本修复使 `resumeFromPersistedState` 的失败处理与之对齐。
