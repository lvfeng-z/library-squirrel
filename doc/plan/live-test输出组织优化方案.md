# live-test 输出组织优化方案

## 审查摘要

**关键声明（抽查项）**

- 声明1：live-test 截图全量平铺落 `.claude/testing/shots/`，现存 33 个文件跨会话/跨谱系混杂（复现：`ls .claude/testing/shots | wc -l`）。
- 声明2：报告落点双路径规则源在技能文本——`.claude/skills/live-test/SKILL.md:52`（「落点：活跃谱系目录 `VERIFY-<任务>.md`；无谱系时 `.claude/testing/reports/`」）与 `SKILL.md:21`、`SKILL.md:110`（同语义重复三处）。
- 声明3：旧报告与部分证据已进版本库（`git ls-files .claude/testing`：8 份 VERIFY 报告 + `evidence/g2170.txt` + `evidence/goroutines-bug2.txt` 含 125KB goroutine dump）；`shots/` 被 ignore（`.gitignore:50`）；`evidence/` 无 ignore 规则，新文件以 `??` 噪声出现（现存 `?? .claude/testing/evidence/vite-live-lock.log`）。
- 声明4：已提交文档引用旧 reports/ 路径——`doc/todo.md:48`、`doc/todo.md:52`、`doc/plan/工作目录外部操作防护方案.md:142`；移动旧报告即断链，故存量必须冻结不动。
- 声明5：旧报告内部以 `.claude/testing/shots/xxx.png` 形态引用截图（如 `.claude/testing/reports/VERIFY-主数据删除关联清理.md:35/83/167`）；移动或清理 shots/ 即断链。
- 声明6：`cdp-shot.mjs` 输出路径是调用参数、无固化（`.claude/testing/cdp-shot.mjs:4`），截图落位完全由技能文本纪律约束，规则改文本即生效。
- 声明7：task-graph 技能文本无 VERIFY/报告落点引用（`grep -n "VERIFY\|谱系目录" .claude/skills/task-graph/SKILL.md` 零命中），改路径模型不触及该技能。

**已裁决（2026-08-28 用户拍板）**

- 决策1：报告路径模型 = **方案甲**（统一收口 `runs/` 目录）。
- 决策2：`runs/` 的 git 策略 = **整体 ignore**（报告/截图/证据全不进库）。
- 决策3：存量三目录（`shots/`、`reports/`、`evidence/`）= **冻结原地**（不移动不删除，旧引用全保）。

**自曝风险**

- 风险1：过渡期双轨——进行中谱系（`workflow/active/` 现存 5 个）的存量 VERIFY 文件按旧模型走完生命周期，短期内「报告出现在两处」现象仍存在，直至活跃谱系清账。
- 风险2：方案甲下谱系 TREE.md 需跨目录外链（workflow→testing）；谱系归档（active→archive）不改相对深度、链接不断，但若日后清理 `runs/` 则归档谱系链接悬空。
- 风险3：截图落位无机制强制（声明6），违规落点只会表现为 `shots/` 重新积灰或 `runs/` 外冒新目录，依赖终审时留意。
- 风险4：方案乙仅作对比展开、未细化到行级变更清单（推荐甲的前提下）。

## 现状与问题

三处输出混乱，根因同一：**live-test 输出没有 run 级作用域**——报告落点为谱系绑定优化（随 task-graph 归档），而证据（截图/dump）落点却是全局平铺，两者不绑定，且报告根有两个（谱系目录 / testing/reports），审阅者无法预判位置。

1. `shots/` 平铺：33 张截图来自历次测试（命名风格各异：`H-*`、`p4-*`、`lock-*`、`v*`……），新旧混杂无法审阅（声明1）。
2. 报告双路径：有谱系落谱系目录、无谱系落 `testing/reports/`（声明2），审阅者要猜。
3. 版本库口径不一致：报告进了库、截图被 ignore、evidence 半进半出（声明3）。

## 设计

### 方案甲（推荐）：统一收口 runs/ 目录

live-test 全部输出自持一域，报告与证据同目录绑定：

```
.claude/testing/
  cdp-eval.mjs / cdp-shot.mjs / backup-file-op.mjs   ← 脚本，git 跟踪（不变）
  runs/
    20260828-分享拉取作品锁/                          ← run 目录 = 一次测试的全生命周期
      VERIFY-分享拉取中的作品锁与自指禁止.md           ← 报告（文件名沿用，自描述）
      shots/   01-xxx.png ...                         ← 本 run 全部截图
      evidence/ xxx.log ...                           ← 本 run 的 dump/日志片段
```

- **run 目录名**：`YYYYMMDD-<任务短名>`；同日同短名冲突追加 `-2`。日期前缀使 `ls runs/` 天然按时间序。
- **截图命名**：`<报告项编号或短代号>-<语义描述>.png`（对齐报告项，如 `01-replace-confirm.png`）；复验不覆盖已有文件，追加 `-rN` 后缀（历史证据保留）。
- **报告内引用**：截图用 Markdown 嵌入语法 `![描述](shots/01-x.png)`（相对路径）——报告与证据同目录，IDE Markdown 预览直接见图，审阅无需跳转；证据文件（日志/dump）仍用行内代码路径提及（文本类不值嵌入）。
- **生命周期**：run 目录常驻不随谱系归档移动（与方案乙的本质差异）；谱系 TREE.md 节点行外链 `../../../testing/runs/<dir>/`（相对深度在 active→archive 移动中不变）。
- **git 策略（决策2 选 ignore 时）**：`runs/` 整体 ignore——报告/截图/证据全不进库，一条规则，与 `.claude/workflow/` 同性质（个人运行产物）。

**为什么甲优于乙**：用户痛点即「两种输出路径」，甲给出唯一根路径；谱系目录回归纯任务导航（TREE.md + 任务长文档），测试输出不再混入；归档不搬报告，链接长期稳定。代价：谱系归档不再自动收存报告（由 runs/ 常驻 + TREE.md 外链补偿）。

### 方案乙（备选，未细化）

报告落点规则不变（谱系目录 / testing/reports 兜底），仅给每份报告配同名证据子目录：`<落点>/VERIFY-x.md` + `<落点>/VERIFY-x/01.png`。保留「随谱系归档」语义，但双根仍在，不解决用户主诉。

## 实施变更清单（按方案甲）

| 位置 | 变更 |
| --- | --- |
| `.claude/skills/live-test/SKILL.md:21` | 「报告落在当前活跃谱系目录」→ 报告落 `runs/<YYYYMMDD>-<任务短名>/`，TREE.md 外链 |
| `SKILL.md:45`（cdp-shot 行） | 「存 `.claude/testing/shots/`」→ 存当前 run 目录 `shots/`，命名对齐报告项编号 |
| `SKILL.md:52`（落点规则） | 改为唯一落点 `.claude/testing/runs/<YYYYMMDD>-<任务短名>/VERIFY-<任务>.md` + 同目录 `shots/`、`evidence/`；含 run 目录命名/冲突/截图 `-rN` 规约与存量冻结说明（旧 shots/reports/evidence 为遗留冻结区，勿新增） |
| `SKILL.md:69` 附近（断言手段-截图/evidence 落盘） | 落盘路径改 run 目录内；报告内截图引用规定为嵌入语法 `![描述](shots/x.png)`，文本类证据仍行内路径提及 |
| `SKILL.md:90` | 「报告文件随谱系归档」→ 报告常驻 runs/，谱系归档不触及 |
| `SKILL.md:110`（task-graph 衔接） | 改外链语义：TREE.md 节点行链接 runs/ 目录 |
| `SKILL.md:113`（首跑范例路径） | 更新为归档后实际路径，并注明属旧落点模型遗留 |
| `.gitignore:50` 附近 | 追加 `.claude/testing/runs` 与 `.claude/testing/evidence` 两行（旧行 `.claude/testing/shots` 保留） |

不动的部分：三个工具脚本零改动（声明6）；`reports/` 不加 ignore——已冻结，若未来有新文件冒出（`??` 可见）即违规信号；CLAUDE.md 无报告路径表述，零改动。

## 存量处置（决策3 选冻结时）

- `shots/`（33 张）、`reports/`（8 份）、`evidence/`（3 文件）原地冻结：不移动不删除——声明4/5 的全部旧引用保持有效；tracked 文件保持 tracked。
- 进行中谱系的存量 VERIFY 文件（`workflow/active/share-lineage/` 等）同样按旧模型走完当前生命周期，不再搬家；此后新测试一律新模型。
- `?? .claude/testing/evidence/vite-live-lock.log`：近期排查暂存件，默认留原地（evidence/ 入 ignore 后不再产生 git 噪声），用户可随手删。

## 阶段清单

### 阶段1：规则修订与存量冻结

- **目标**：技能文本与 gitignore 切换到 runs/ 模型，旧目录冻结且全部既有引用可证不破。
- **涉及文件**：`.claude/skills/live-test/SKILL.md`、`.gitignore`。
- **验证命令**：`grep -n "runs" .claude/skills/live-test/SKILL.md`（命中新落点规则）；`grep -n "testing/shots\|testing/reports" .claude/skills/live-test/SKILL.md`（仅命中存量冻结说明）；`git status`（evidence/ 新 ignore 生效，无新增噪声）。
- **退出标准**：技能文本无残留的「新输出落旧路径」指令；`doc/todo.md:48`、`doc/todo.md:52`、`doc/plan/工作目录外部操作防护方案.md:142` 所指文件仍在原路径（冻结不破链）。
- **依赖**：无。
- **模型建议**：主模型（文本量小但涉及行为契约改写，需判断力）。
- **交接摘要**：改动两文件清单 / runs/ 落点规则要点 / 存量冻结边界 / 验证结果。
