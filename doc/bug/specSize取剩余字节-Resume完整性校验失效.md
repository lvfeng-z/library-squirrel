# spec.Size 取剩余字节 → Resume 完整性校验失效

> 类型：已确认缺陷（代码 + 日志实证）
> 分析日期：2026-07-07
> 状态：已修复(2026-07-08,解析 Content-Range 还原完整大小)
> 严重程度：中（校验失效；正常 EOF 路径文件仍完整，但会掩盖其他原因导致的不完整/错位）
> 日志：`log/pixiv任务频繁启停出现资源损坏（资源大小与正常资源相同但是图像出现错位或变色）-1.log`

## 现象

pixiv 任务续传（Resume）后，`handleEOF` 的完整性校验（`written < s.size` 判失败）**恒为 false（永远通过）**，校验完全失效。任务即使在续传期间 reader 返回的字节数不足，也会被判 `Complete` → 任务 Finished。

## 根因

pixiv `Resume`（路径一 `Probe` / 路径二 `GetHeaders`）返回的 `spec.Size` 在 HTTP **206（Range）响应**下取自 `Content-Length` 头，而 **206 的 Content-Length 是"剩余字节数"（从 offset 到文件末尾），不是完整文件大小**。完整大小需从 `Content-Range: bytes start-end/total` 头取 total，或由 `offset + Content-Length` 得出。

### 证据（任务 258，日志 L1870-L1886）

```
L1870  建连 code=206 offset=1212416 contentLength=197278
L1873  ResumeMount taskId=258 writeOffset=1212416 streamOffset=1212416
L1886  downloadLoop 结束 taskId=258 totalSize=197278
L1887  任务状态变更 258: Processing → Finished
```

- pixiv `GetHeaders` 返回 `size = Content-Length = 197278` → `spec.Size = 197278`
- 258 完整大小 = `1212416 + 197278 = 1409694`（与首次 Start 的 `totalSize=1409694` 吻合，日志 L677）
- `streamController.size = 197278`，`initialOffset = writeOffset = 1212416`
- `downloadLoop 结束 totalSize=197278`（= `totalStreamSize` 只累加 `s.size`，L1586-1593）

### handleEOF 校验链（model.go L1201-1212）

```
s.size = 197278          // spec.Size（剩余字节）
written 从 initialOffset = 1212416 累加
判定: written < s.size  →  1212416 + x < 197278  →  恒 false
```

`written` 的起点 `initialOffset`（1212416）已大于 `s.size`（197278），故无论本次续传下载多少字节（哪怕 0 字节后立即 EOF），`written < s.size` 永远为 false，校验永远通过。

对比首次 `Start`（200 OK）：`Content-Length = 完整大小`，`spec.Size` 正确，校验有效。

## 影响范围

| 用途 | 影响 |
| --- | --- |
| `handleEOF` 完整性校验（model.go L1207 `written < s.size`） | **失效**（Resume 场景恒通过） |
| `totalStreamSize` 进度总量（model.go L1586，只累加 `s.size`） | 偏小（= 剩余字节，非完整） |
| `reportProgress` 的 total（model.go L1604，`s.size + s.initialOffset`） | 数值上恰好接近完整，但语义混乱（两个"剩余/偏移"相加巧合等于完整） |
| writeOffset / reader 数据对齐 | **不影响**（writeOffset 来自 stat 文件大小，reader.validBytes 来自 `SetValidBytes(stat)`，二者均与 spec.Size 无关） |

> 关键：正常 EOF 路径下（206 body 读完返回 `io.EOF`），reader 会返回完整的剩余字节，文件仍完整正确。**本 bug 不直接导致内容损坏**，但使校验失效，会**掩盖**其他原因导致的不完整/错位——错位文件若累计字节数恰好够，也会被判 `Complete`。

## 与历史 264/265 事件的关系

同一 `spec.Size` 语义陷阱的不同表现（见 `Pause边界EOF丢失-潜在Resume416假失败.md` 第 17、116 行）：

| | 264/265（已回退） | 本 bug |
| --- | --- | --- |
| 误用点 | 主程序过滤器 `offset >= spec.Size` 判完成 | `handleEOF` 用 `written < spec.Size` 做校验 |
| spec.Size 语义 | 剩余字节（误当完整） | 剩余字节（误当完整） |
| 后果 | 半成品误判完成 → 截断损坏 | 校验失效 → 掩盖不完整/错位 |
| 状态 | 过滤器已回退 | 已修复(2026-07-08) |

教训一致：`spec.Size` 在 Range 续传语义下不是完整大小，任何把它当完整大小用的判定都会出错。

## 修复(已实施,2026-07-08)

`RetryReadCloser.GetHeaders`(`library-squirrel-plugin-pixiv/internal/download/retry_reader.go`)新增 `fullSize` 辅助方法:206 时据 `Content-Range: bytes start-end/total` 解析 `total` 作为完整大小;`Content-Range` 缺失/解析失败时 fallback 到 `validBytes + Content-Length`;200 的 `Content-Length` 保持不变(本就是完整大小)。采用原"方案 1"。

**连锁修复**:`backend/taskManager/model.go` `reportProgress` 的 total 公式从 `s.size + s.initialOffset` 改为 `s.size`——修复前依赖"Resume 时 spec.Size 是剩余字节",靠 `size + initialOffset` 凑出完整;修复后 spec.Size 已是完整,再加 `initialOffset` 会偏大。改后 Start(`initialOffset=0`)与 Resume 两种场景 total 都等于完整大小,`finished/total` 进度比例正确。

修复后 `handleEOF` 校验 `written < s.size` 在 Resume 场景恢复有效:完整下载时 `written = initialOffset + (total - initialOffset) = total = s.size`(校验通过),不完整时 `written < total`(判 Failed)。

## 验证

- [x] `CGO_ENABLED=0 go build` pixiv + taskManager 通过
- [x] gofmt 合规
- [ ] 运行时验证:Resume 后若续传不完整,应被正确判 Failed(而非误判 Finished)

## 相关

- 关联缺陷：`doc/bug/Pause边界EOF丢失-潜在Resume416假失败.md`（`spec.Size` 语义陷阱的历史记录与教训）
- 配套：`doc/bug/频繁启停资源内容错位-Resume并发竞态.md`（内容错位根因,已修复;本 bug 是其校验失效、错位文件能被判 Finished 的必要条件）
- 代码定位：
  - `library-squirrel-plugin-pixiv/internal/download/retry_reader.go` `GetHeaders`（L57）、`ensureResponse`（L131，已知 StatusCode / Content-Length / Content-Range 可取）
  - `library-squirrel-plugin-pixiv/task_handler.go` `Resume`（L507，spec.Size 填充 L581）、`Start`（L315）
  - `backend/taskManager/model.go` `handleEOF`（L1186，校验 L1207）、`totalStreamSize`（L1586）、`reportProgress`（L1596）
