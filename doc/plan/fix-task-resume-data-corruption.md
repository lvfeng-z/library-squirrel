# 修复：跨重启续传数据损坏（Resume spec 缺失 + 完整性校验放行）

## 问题描述

pixiv 插件执行多图作品任务（父任务 262，9 个子任务）过程中，在 **setup 阶段**（任务已派发、插件 `Start` 已建立 reader、但下载尚未真正开始）对父任务执行了一次暂停，恢复后出现：

1. **任务 269 失败**：插件 `Start` 请求 pixiv 时 TCP `EOF`。
2. **任务 263 / 266 / 267 / 270 对应的作品首页无法加载图像**：4 个任务均"成功"结束（`Finished`），但产出的 `persistent_store` 记录残缺——文件实际有内容却被前端拒绝渲染。

> 本次故障正是 `多轨重构实施进度.md` 中遗留的"运行时多流回归验证（下载/暂停/恢复/跨重启续传/备份）"所暴露的首批缺陷。

### 故障现场证据

| 任务 | 现象 | persistent_store 状态 | 磁盘文件 |
|------|------|------------------------|----------|
| 263 / 266 / 267 / 270 | `Finished` 但图像不显示 | `filename_extension=""`、`width=0/height=0`、`status=1` | 有效内容但**无扩展名**，另残留一个 0 字节 `.png` |
| 269 | `Failed`（`Start` EOF） | 0 字节空文件 | `p6[7].png` 为 0 字节，旧资源已丢失且备份还原返回 0/1 |

---

## 根因分析

### 因果链

```
setup 阶段暂停（model.go:1165 len(streams)==0 → cancel ctx，不通知插件）
   │
   │  ① 事务用 context.Background 不受 cancel 影响，照常提交（model.go:585）
   │     → pending_resource_id 持久化（8/9/12/15）
   │  ② 插件侧 activeReaders 残留 Start 时建立的 reader（HTTP response 对象仍非 nil）
   │
   ▼
恢复 → resumeFromPersistedState（offsets=map[main:0]）→ 调插件 Resume
   │
   │  ③ Resume 返回的 spec 缺 Size / Format（task_handler.go:537）
   │  ④ Resume 路径一复用旧 reader，GetHeaders 探活失效（retry_reader.go:75）
   │
   ▼
续传走 offset=0 重建 store（model.go:1122）
   │
   │  ⑤ 新 persistent_store 文件名无扩展名（spec.Format 空）
   │  ⑥ 落盘 Complete() 时 IsImageExt("")=false → 跳过 DecodeConfig → width/height=0
   │  ⑦ handleEOF 校验 s.size>0 因 Size=0 跳过 → written=任意值都判 Complete（model.go:914）
   │
   ▼
downloadLoop 全部"完成" → setState(Finished)
   │
   ▼
前端 WorkCard imagePath：无 thumbnailStore → 走 workStore，但
   isDisplayableImage(filenameExtension="")=false → 返回 '' → <img> 不加载（WorkCard.vue:42）
```

### 缺陷清单

| # | 位置 | 缺陷 | 归属 | 危害 |
|---|------|------|------|------|
| A | `taskManager/model.go:914` | `handleEOF` 完整性校验 `s.size > 0 && written < s.size`，`Size=0` 时整体跳过 | 主程序 | 空文件/未知大小产物被误判 Complete |
| B | `taskManager/model.go:1165` | setup 阶段暂停直接 `cancel()`，不通知插件清理 reader | 主程序 | activeReaders 残留死连接，是整条异常路径的入口 |
| C | `backup/store_backup_orchestrator.go:143` | 还原时 `BackupID<=0` 静默 `continue`，无告警 | 主程序 | 269 旧资源丢失不可逆，且无可见告警 |
| D | `plugin-pixiv/task_handler.go:537` | `Resume` 返回的 `StoreSpec` 缺 `Size` 与 `Format`（`Start` 有传） | 插件 | 本次故障直接触发器；主程序无法补救 Format |
| E | `plugin-pixiv/retry_reader.go:75` | `ensureResponse` 仅看 `response==nil`，不探活真连接 | 插件 | GetHeaders 探活失效，路径一误判死连接存活 |

### 契约层面：主程序的不自洽

`streamController.size` 注释明写 `// 远程大小;-1 未知`，承认大小可能未知；但 `handleEOF` 既不处理 `-1` 也不处理 `0`，一律放行。**主程序把"完整性校验"这一关键安全职责外包给了插件的可选字段，且自身语义与校验逻辑矛盾。** 这是宿主防御的缺失，不依赖插件是否正确。

---

## 修复方案

### 优先级与职责划分

- **主程序（A/B/C）= 防御根因，优先修复**：它是数据完整性的最后防线，修复后可防御所有插件（含未来/第三方）的同类错误。
- **pixiv 插件（D/E）= 信息源头，配套必修**：`Format` 信息只在插件手里（来自 HTTP `Content-Type`），主程序无法凭空补出文件扩展名。不修 D，本次 5 个作品即使主程序修好也无法恢复正常显示。

> **两者不可互相替代**：只修主程序，4 个任务会从"假完成"变成"真 Failed"，图片仍显示不出；只修插件，主程序的校验/暂停/还原隐患仍在，下一个插件的同类错误会再次制造数据损坏。

---

### 主程序修复

#### A. `model.go` — `handleEOF` 完整性校验：处理 Size 未知/为 0

当前校验对 `Size<=0` 一律放行。修复为：`Size` 已知时按预期校验；`Size` 未知时至少要求 `written > 0`（空产物必然不完整）。

```go
// 完整性校验：downloaded 轨
if s.generation == entity.GenerationDownloaded {
    switch {
    case s.size > 0 && written < s.size:
        // 已知大小且未下完
        return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("%s 下载不完整: 已下载 %d / 预期 %d", s.role, written, s.size)}
    case written == 0:
        // 大小未知（size<=0）但一字节未写：空产物，判定不完整
        return streamResult{kind: resultFailed, errMsg: fmt.Sprintf("%s 下载为空（written=0）", s.role)}
    }
    // size<=0 且 written>0：大小未知但已写入数据，放行（无法进一步校验）
}
```

> 此修复让本次 4 个任务在 Resume spec 仍缺 Size 的情况下会**失败而非假完成**——配合缺陷 D 修复（补 Size）后才能正常 Finished。故 A 与 D 必须同步上线，否则 pixiv 续传会全部失败。

#### B. `model.go` — setup 阶段暂停：通知插件清理 reader

setup 阶段暂停（`Pause()` 中 `len(m.streams)==0` 分支）目前只 `cancel()` 主程序 ctx，插件侧 reader 仍残留于 `activeReaders`。修复为：在 setup 暂停路径也调用插件 `Pause`（或 `Stop`），让插件关闭上游连接、清理缓存 reader，使恢复时 `Resume` 必然走"路径二（新建 reader）"。

```go
// Pause() setup 阶段分支
if len(m.streams) == 0 {
    logger.Log.Infof("[TaskManager] Pause: taskId=%d 在 setup 阶段暂停，直接取消", m.taskId)
    // 新增：通知插件暂停（关闭 Start 已建立的上游连接、清理 activeReaders）
    // 避免恢复时 Resume 路径一复用已失效的 reader
    if m.pluginExec != nil {
        if err := m.pluginExec.Pause(m.ctx, m.task); err != nil {
            logger.Log.Warnf("[TaskManager] Pause: taskId=%d 通知插件暂停失败: %v", m.taskId, err)
        }
    }
    m.cancel()
    m.setState(TaskStatePaused)
    return nil
}
```

> 依赖缺陷 E（`ensureResponse` 真正探活）作为双保险：即便插件 reader 未清理，恢复时 `GetHeaders` 也能识别死连接并走路径二。

#### C. `store_backup_orchestrator.go` — 还原静默跳过改为可观测 + 失败上报

当前 `RestoreAllStores` 对 `BackupID<=0` 静默 `continue`，最终只打 `0/1` 日志。修复：

1. 备份阶段（`BackupStores`）：`Delete` 失败时区分原因，`record not found`（无旧资源）与其他错误分别记录；返回的 `StoreBackupItem` 标注备份失败原因。
2. 还原阶段：`BackupID<=0` 跳过时统计并 `WARN`，连同"备份失败导致无法还原"的条目数一并返回给调用方，写入任务 `error_message`，避免"还原 0/1 但任务静默继续"。

```go
// RestoreAllStores 返回 (restored, skipped)
func (o *StoreBackupOrchestratorImpl) RestoreAllStores(ctx context.Context, items []*StoreBackupItem) (restored, skipped int) {
    ...
    for _, item := range items {
        if item.BackupID <= 0 {
            skipped++ // 备份失败/未备份：无可还原数据
            logger.Log.Warnf("[StoreBackupOrchestrator] 跳过还原: type=%s, BackupID=%d（备份未成功）", item.StoreType, item.BackupID)
            continue
        }
        ...
        restored++
    }
    return
}
```

调用方（任务失败还原路径）将 `skipped>0` 反映到任务错误信息：`"任务失败，N 个旧资源因备份缺失无法还原"`。

---

### pixiv 插件修复

#### D. `task_handler.go` — `Resume` 补齐 Size / Format

`Resume` 两条路径（复用旧 reader / 新建 reader）返回 spec 前统一调 `GetHeaders` 取 `size` 与 `format`，与 `Start` 对齐。

```go
// Resume 末尾，return 前补齐头部信息
headers, size, hdrErr := reader.GetHeaders()
if hdrErr != nil {
    // 路径一旧 reader 已失效：关闭后走路径二新建（原探活逻辑收敛到这里）
    reader.Close()
    reader = download.NewRetryReadCloser(pixivRequestFn(taskURL), 3)
    reader.SetValidBytes(offset)
    headers, size, hdrErr = reader.GetHeaders()
    if hdrErr != nil {
        return nil, nil, hdrErr
    }
}

format := detectFormat(headers.Get("Content-Type"))   // 抽出 Start 中的扩展名判定逻辑复用

h.activeReaders.Store(param.Task.ID, reader)
return []*sdkdto.StoreSpec{{
    Role:        sdkdto.StoreRoleMain,
    Generation:  sdkdto.GenerationDownloaded,
    ReadCloser:  reader,
    Size:        size,
    Format:      format,
    Continuable: boolPtr(true),
}}, nil, nil
```

> `detectFormat` 需把 `Start`（`task_handler.go:334-344`）中按 `Content-Type` 判定 `.jpg/.png/.mp4` 的逻辑抽成公共函数复用。

#### E. `retry_reader.go` — `GetHeaders` 真正探活

`ensureResponse` 仅在 `response==nil` 时重建。`GetHeaders` 用于探活时，若复用旧 response，底层 TCP 已被服务端关闭也识别不出。修复：`GetHeaders` 探活场景下，若 response 已存在但距建立过久（或显式探活模式），强制 `closeResponse` 后重建。

最小改法：为 `RetryReadCloser` 增加 `Probe()` 方法，强制关闭旧 response 并重新 `requestFn(validBytes)` 建连，返回 header/size/err；`Resume` 路径一用它替代 `GetHeaders` 做探活。

```go
// Probe 强制重建连接探活（用于 Resume 判断缓存 reader 是否仍可用）
func (r *RetryReadCloser) Probe() (http.Header, int64, error) {
    r.closeResponse()
    r.pendingError = nil
    return r.GetHeaders()
}
```

---

## 脏数据修复

本次任务 262 产生的脏数据需单独清理（代码修复无法自动回填已损坏的产物）：

| 作品 | 任务 | 当前状态 | 处理 |
|------|------|----------|------|
| 9 / 10 / 11 / 16 | 263 / 267 / 270 / 266 | 无扩展名有效文件 + 0 字节 `.png` 残留 + DB 元数据空 | 删除 `persistent_store` 37-40 及残留 0 字节 `.png`，重新执行对应子任务 |
| 8 | 269 | 0 字节 `p6[7].png`，旧资源已丢 | 删除空 store，重新执行 269 |

建议清理脚本步骤：
1. 按 `resource_id IN (8,9,12,15)` 删 `resource_store` / `persistent_store` 记录；
2. 删磁盘 `D:\LS\store\resource\瀬尾\` 下 `p0/p3/p4/p7` 的无扩展名文件与对应 0 字节 `.png`、`p6[7].png`；
3. 前端对这 5 个子任务触发"重新下载"。

---

## 验证方案

| 场景 | 预期 |
|------|------|
| setup 阶段暂停 → 恢复（pixiv） | reader 清理，Resume 走新建路径，文件有扩展名、`width/height>0`、前端正常显示 |
| setup 阶段暂停 → 恢复（任意插件 Resume 不传 Size） | 主程序按缺陷 A 判 Failed 并报错，而非假完成 |
| 插件 Resume 返回 `Size=0` 但实际下载完整（written>0） | 放行（A 的 `written==0` 兜底不误杀） |
| 插件 Resume 返回空流（written=0） | 判 Failed（A 兜底） |
| 下载中途网络 EOF（如 269） | 任务 Failed，备份缺失时 `error_message` 体现"旧资源无法还原" |
| 跨重启续传（关闭进程后重启恢复） | 路径二正常工作，与同进程恢复产物一致 |

回归覆盖 `多轨重构实施进度.md` 中标注的"运行时多流回归验证"全部项。

---

## 涉及文件

**主程序：**
- `backend/taskManager/model.go`：`handleEOF`（缺陷 A）、`Pause` setup 分支（缺陷 B）
- `backend/backup/store_backup_orchestrator.go`：`BackupStores` / `RestoreAllStores`（缺陷 C）

**pixiv 插件：**
- `task_handler.go`：`Resume`（缺陷 D）、`Start` 抽公共 `detectFormat`
- `internal/download/retry_reader.go`：新增 `Probe`（缺陷 E）

**编译/绑定：**
- 主程序若改 handler 签名：`wails3 generate bindings -ts`
- 本次插件改动未触及 Wails 绑定签名，bindings 无需重生；插件需重新 `go build` 并打包
