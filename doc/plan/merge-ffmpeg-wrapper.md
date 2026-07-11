# ffmpeg 封装子层（K 视频多轨合并动作·实现步骤）

> 归属：K 视频多轨合并动作的实现步骤（非派生节点）；merge 业务依赖此前置（实现先后关系，非阻塞派生）
> 状态：已完成 · 类型：基建（项目零先例）
> 范围：仅封装 ffmpeg 调用（查找/执行/判错/超时/校验）；业务编排（MergeResource）、落盘、设置项、DTO、前端归 merge 业务/前端步骤，不在本子层。

## 1. 背景与定位

K（合并动作）需要把一个 Resource 的 videoTrack + audioTrack 两个本地完整文件合并为可播放的单文件，靠 ffmpeg 做无重编码 remux：

```
ffmpeg -i video -i audio -c copy -movflags +faststart -y out
```

项目此前**零 ffmpeg 调用先例**（全后端 grep 无命中；仅有的两处 `exec.Command` 是 appLauncher 开文件、插件子进程，都不是“调外部 CLI 处理文件取产物”模式）。因此把 ffmpeg 封装独立成一步——merge 业务依赖此前置，无封装则 K 无法实现。

ffmpeg 封装是纯工具层：不碰 DB、不注册 handler、不被前端直接调用。merge 业务的 service 注入本封装提供的能力。

## 2. 设计决策

### 2.1 ffmpeg 查找：仅 PATH

`exec.LookPath("ffmpeg")` 在系统 PATH 上查找；找不到时返回明确中文错误（合并是用户主动触发，不静默降级）。

**不做捆绑分发、不做设置项可配置路径**（经讨论否决）：
- 捆绑：+约 80MB 体积、LGPL/GPL 再分发许可与多平台构建复杂度，收益不抵成本。
- 可配置路径：增加设置项 UI 与持久化面，当前无需求。

后续若需要，可在此层扩展（`NewFFmpegMuxer` 增加可选路径来源），不影响调用方 merge 业务。

### 2.2 无重编码 remux

`-c copy`：不重新编解码，仅复用流到新容器；速度接近 IO 上限，无质量损失。`+faststart`：moov atom 前移，利于流式/外部播放器即时起播。

### 2.3 产物校验：轻量

仅校验产物文件存在且 `size > 0`。**不引入 ffprobe 依赖**做深度流校验——remux 是平凡操作，ffmpeg 退出码 0 已足够可信；深度校验的复杂度与额外依赖不匹配当前价值。

## 3. 模块结构

```
backend/merge/
├── ffmpeg.go       — FFmpegMuxer 封装（查找/调用/判错/超时/校验）
└── ffmpeg_test.go  — 测试（无 ffmpeg 时 t.Skip）
```

> handler/service/repository 归 merge 业务，ffmpeg 封装阶段不创建这些文件。

## 4. 接口设计

```go
package merge

// 错误：PATH 上找不到 ffmpeg。
var ErrFFmpegNotFound = errors.New("未在系统 PATH 中找到 ffmpeg，请先安装 ffmpeg 后重试")

// FFmpegMuxer 封装对 ffmpeg 的调用，把独立的视频轨与音频轨无重编码合并为单个媒体文件。
type FFmpegMuxer struct {
    binaryPath string        // ffmpeg 可执行文件路径（构造时由 LookPath 确定）
    timeout    time.Duration // 单次合并超时
}

// NewFFmpegMuxer 在系统 PATH 上查找 ffmpeg；找不到返回 ErrFFmpegNotFound。
func NewFFmpegMuxer() (*FFmpegMuxer, error)

// MergeRemux 对两个完整的本地媒体文件做无重编码合并（remux），
// 产物写到 outPath（调用方负责路径唯一性与父目录存在）。
// 视频轨或音频轨任一缺失、ffmpeg 失败、超时，均返回携带 stderr 关键信息的错误。
func (m *FFmpegMuxer) MergeRemux(ctx context.Context, videoPath, audioPath, outPath string) error
```

## 5. 实现要点

### 5.1 查找

```go
p, err := exec.LookPath("ffmpeg")
if err != nil {
    return nil, ErrFFmpegNotFound
}
```

默认超时 `defaultMergeTimeout = 10 * time.Minute`（remux 通常秒级，10min 是大文件兜底）。

### 5.2 调用

```go
ctx, cancel := context.WithTimeout(ctx, m.timeout)
defer cancel()

cmd := exec.CommandContext(ctx, m.binaryPath,
    "-i", videoPath, "-i", audioPath,
    "-c", "copy",
    "-movflags", "+faststart",
    "-y", outPath,
)
```

- 捕获 stderr（ffmpeg 进度与错误均写 stderr）。
- `-y`：覆盖输出；outPath 由 merge 业务保证唯一（如带 merged 标识的临时名），避免覆盖既有文件。

### 5.3 判错

- 退出码 0 → 成功；再校验产物文件存在且 `size > 0`，否则报“合并完成但产物异常”。
- 退出码非零 → 取 stderr 末尾若干行（ffmpeg 错误通常在末尾）拼进错误：`合并失败：<ffmpeg 末尾错误>`。
- `ctx.Err() == context.DeadlineExceeded` → “合并超时（已超过设定阈值）”。
- 输入文件不存在：ffmpeg 自身会报错并经 stderr 透出；本封装不单独预检（保持封装薄），merge 业务在调用前应已通过 `GetAbsPath` 取得存在的 store 文件。

### 5.4 资源清理

超时或失败时，若产物 outPath 已部分写入，删除残留文件（避免半成品占位）。

## 6. 测试策略

`ffmpeg_test.go`：外部依赖 ffmpeg，遵循“无则跳过”模式。

```go
func skipIfNoFFmpeg(t *testing.T) {
    if _, err := exec.LookPath("ffmpeg"); err != nil {
        t.Skip("ffmpeg 未安装，跳过合并测试")
    }
}
```

用例：
1. **正常合并**：testdata 小视频 + 小音频 → 产物存在、`size > 0`（可选：若 `ffprobe` 可用，断言产物含双轨）。
2. **ffmpeg 失败**：给一个损坏/非媒体输入 → 返回的错误含 stderr 关键信息、退出码非零。
3. **超时**：极小 timeout + 较大输入验证 context 取消能杀掉子进程（构造困难时用 mock binary 替代 ffmpeg）。

testdata 准备：测试启动时用 ffmpeg lavfi 在 `t.TempDir()` 临时生成 1 秒测试视频/音频（`testsrc` + `anullsrc`，mpeg4/aac 编码避开 GPL 的 libx264 依赖），**不进仓库**（零体积、自包含）；生成失败（发行版缺编码器）时 `t.Skip`。

## 7. 不在范围（归 merge 业务/前端）

| 归属 | 内容 |
|------|------|
| merge 业务 | `MergeResource` handler/service 业务编排、PersistentStore 落产物、resource_store 挂 merged store（事务）、`mergeStrategy` 设置项、app.go 注册 |
| 前端 | DTO `mergeable`/`MergedStore`、SDK DTO 字段、bindings 重新生成、WorkDialog「合并」按钮、打开优先级 `merged > main` |

## 8. 验收清单

- [ ] 系统未装 ffmpeg 时，`NewFFmpegMuxer()` 返回 `ErrFFmpegNotFound`（中文消息）。
- [ ] 给定有效 video+audio 文件，`MergeRemux` 产出可播放的合并文件（`-c copy` 无重编码、`+faststart` moov 前移）。
- [ ] ffmpeg 退出非零时，错误信息携带 stderr 末尾关键内容。
- [ ] 超时能取消子进程，不残留僵尸进程；残留产物被清理。
- [ ] 不引入 ffprobe 等额外外部依赖。
- [ ] ffmpeg 封装不注册 handler、不碰 DB（纯工具层）。

## 9. 风险

- **testdata 媒体文件体积**：测试用 `t.TempDir()` 临时生成、不进仓库，无膨胀风险；代价是跑测试需本机有 ffmpeg（无则 `t.Skip`）。
- **跨平台 ffmpeg 行为差异**：Windows 上 `ffmpeg.exe` 同样在 PATH 即可；`exec.LookPath` 跨平台一致。`+faststart` 与 `-c copy` 在主流版本行为稳定。
- **大文件超时阈值**：10min 兜底可能对超大 4K 源偏紧；merge 业务落地后据实测再调（常量集中在 `FFmpegMuxer`，易改）。
