# 合并业务编排（K 视频多轨合并动作·实现步骤）

> 归属：K 视频多轨合并动作的实现步骤（非派生节点，不入 TREE）；前置 ffmpeg 封装已完成。
> 状态：待做（设计草稿，供实现）· 类型：业务编排
> 范围：`MergeResource` 业务编排（**归 resource 模块**）+ `mergeStrategy` 设置项 + handler；merge 包仅保留纯合并工具；落盘/路径/`store_type` 归 persistentStore。DTO `mergeable`/前端归前端。

## 1. 背景与定位

K（合并动作）的业务编排层。ffmpeg 封装已提供 `merge.FFmpegMuxer.MergeRemux(ctx, videoPath, audioPath, outPath)`（`backend/merge/ffmpeg.go`，3 测试通过）——纯合并能力，文件路径进、文件路径出。

按 **MODULE_BOUNDARY_PURITY**（`.claude/rules/backend.md`）：merge 包不感知 store/resource；“取 resource 的 store、落盘到 `store/resource/...`、挂 `resource_store`(merged)”这些业务编排归 **resource 模块**（合并的领域对象是 Resource），落盘/路径规范归 **persistentStore**。

## 2. 模块边界（关键：守住 MODULE_BOUNDARY_PURITY）

| 模块 | 职责 | 不做 |
|------|------|------|
| `merge` | 纯合并：`FFmpegMuxer.MergeRemux(ctx, video, audio, out)` | 不感知 store/resource/落盘路径/`store_type`；无 handler/service |
| `resource` | 合并编排：取 store → 调 merge → 落产物 store → 挂 `resource_store`(merged)；handler 暴露 `MergeResource` | 不直接做文件合并（调 merge）；不直接拼 `store/resource/...` 路径（调 persistentStore） |
| `persistentStore` | 落盘原语（`StoreFromFile`/`GetById`/`GetAbsPath`/`Delete`）+ 路径变换（`BuildVariantPath`） | 不感知“合并”业务概念 |

> merge 包的输入输出永远是**文件路径**（string），不出现 store/resource 实体。

## 3. 设计决策

### 3.1 mergeStrategy 设置项（归 resource）

`mergeStrategy`（select）：`keep`（默认，新建 merged store、保留原轨道）/ `overwrite`（产物作 main、删原轨道 store 及文件）。挂 Settings 新分组 `MergeSettings`（`Strategy` 默认 `"keep"`）。resource 编排读它决定挂载策略。两处默认值同步（`backend/settings/model.go` 的 `NewSettings()`/`defaultSettings()`）。

### 3.2 产物落盘（归 persistentStore）

- 产物 relPath 从**源 videoTrack store 的 FilePath 推导**：同目录、源文件名 + `_merged`、保留扩展名。作者目录信息已在源路径里，**无需查 work/作者**。
- persistentStore 新增 `BuildVariantPath(sourceRelPath, suffix) string`：通用路径变体原语（同目录 + 文件名追加 suffix + 保扩展 + 净化），是 store 路径变换的所有者。resource 调它生成 `_merged` 产物路径，不直接拼 `store/resource/...`。
- `taskManager.resolveMainPath` 是**下载命名策略**（耦合文件名模板 `FileNameFormatProvider` + 作品元数据 `ExtractTokenData` + 插件建议名 `buildSuggestedFileName`），**不移动**——合并产物命名（源名 + `_merged`）远比下载模板命名简单；强行整体提取会让 persistentStore 感知“文件名模板/作品元数据”，反而违反 MODULE_BOUNDARY_PURITY。
- 落盘：ffmpeg 输出到临时目录 → `persistentStore.Service.StoreFromFile(ctx, relPath, fileName, tmpAbsPath)` 注册为 PersistentStore。

### 3.3 事务（补偿语义）

`StoreFromFile` 内部自带“建 PersistentStore 记录 + 写文件”的原子性，但其 repo 操作**不一定感知外部事务**。故编排采用**补偿语义**而非单一大事务：

1. `StoreFromFile` 落产物 PersistentStore（自带原子性）→ 得 `mergedPsId`
2. 事务内挂 `resource_store`(merged)（`resourceStoreRepo` 走 `dbFromCtx` 事务感知）
3. 步骤 2 失败 → 补偿：删产物 PersistentStore 记录 + `CleanupFile` 删磁盘文件

> `mergeStrategy=overwrite` 的“删原轨”在挂 merged 成功后执行（先建后删，避免中途无成品）。

### 3.4 Backup/Restore 透明

`store_type` 开放枚举红利：Backup/Restore/软删 按 store 集合遍历，merged 自动被“全部”语义覆盖，**零改动**（resource 编排守住此不变量，见 TREE 决策约束）。

## 4. 模块结构

```
backend/merge/
├── ffmpeg.go          — 已完成（FFmpegMuxer，纯合并工具）
└── ffmpeg_test.go     — 已完成
（无 handler/service——merge 是工具包，被 resource 注入调用）

backend/resource/
├── merge_service.go   — 新增：MergeService 合并编排（取 store→调 merge→落盘→挂载）
├── handler.go         — 增 MergeResource 方法 + mergeSvc 依赖
├── service.go         — 原有 CRUD 不变
└── resource_store_repository.go — 复用 GetByType/DeleteByResourceIdAndTypes

backend/persistentStore/
└── service.go         — 增 BuildVariantPath（路径变体原语）；落盘原语已有。（不动 taskManager）
```

> 合并编排独立成 `MergeService`（而非塞进 resource.Service），因其依赖密集（merger/storeOps/settings/tx）且独立；原 resource.Service 保持纯 CRUD。

## 5. 接口设计

```go
package resource

// Merger 文件合并能力（由 merge.FFmpegMuxer 实现）
type Merger interface {
	MergeRemux(ctx context.Context, videoPath, audioPath, outPath string) error
}

// StoreOps store 落盘/查询/删除/路径原语（由 persistentStore.Service 实现）
type StoreOps interface {
	GetById(ctx context.Context, id int64) (*domain.PersistentStore, error)
	GetAbsPath(store *domain.PersistentStore) string
	StoreFromFile(ctx context.Context, relPath, fileName, srcAbsPath string) (int64, error)
	Delete(ctx context.Context, id int64, backup bool) (int64, error)
	BuildVariantPath(sourceRelPath, suffix string) string // 同目录+suffix+保扩展，store 路径变体
}

// MergeSettingsReader 读合并策略（由 settings.Service 实现）
type MergeSettingsReader interface {
	GetMergeStrategy() string // "keep" | "overwrite"
}

// MergeService 合并业务编排
type MergeService struct {
	resourceStoreRepo *ResourceStoreRepository
	merger            Merger
	storeOps          StoreOps
	settings          MergeSettingsReader
	tx                Transactor // resource 定义，database.WithTransaction 适配
}

func NewMergeService(resourceStoreRepo *ResourceStoreRepository, merger Merger, storeOps StoreOps, settings MergeSettingsReader, tx Transactor) *MergeService

// MergeResource 对指定 Resource 执行音视频合并（用户主动触发）。
func (s *MergeService) MergeResource(ctx context.Context, resourceId int64) (*MergeResult, error)
```

handler 增：
```go
// Handler 增 mergeSvc 依赖（NewHandler 扩参 或 单独 setter）
func (h *Handler) MergeResource(ctx context.Context, resourceId int64) *model.ApiResponse[*MergeResult]
```

> 依赖通过构造函数注入（SERVICE_DEPENDENCY_VIA_INTERFACE：resource 定义接口，merge/persistentStore/settings 实现）。

## 6. 实现要点（编排流程）

```
MergeResource(ctx, resourceId):
  1. resourceStoreRepo.GetByType(ctx, resourceId, StoreTypeVideoTrack) → videoRS（缺报“缺视频轨”）
     resourceStoreRepo.GetByType(ctx, resourceId, StoreTypeAudioTrack) → audioRS（缺报“缺音频轨”）
  2. storeOps.GetById(videoRS.StoreID) → videoPS；storeOps.GetAbsPath(videoPS) → videoAbs（audio 同理）
  3. tmpOut = 临时文件路径（os.TempDir() + 唯一名）
  4. merger.MergeRemux(ctx, videoAbs, audioAbs, tmpOut) → ffmpeg 合并（muxer 自处理超时/失败/残留）
  5. mergedRelPath = storeOps.BuildVariantPath(videoPS.FilePath.String, "_merged")
  6. mergedPsId = storeOps.StoreFromFile(ctx, mergedRelPath, 产物展示名, tmpOut)  // 落盘+建 PersistentStore
  7. 事务 tx.WithTransaction: 挂 resource_store{StoreType:merged, Generation:downloaded, StoreID:mergedPsId, ResourceID:resourceId}
     - 失败补偿: storeOps.Delete(mergedPsId) + storeOps.CleanupFile(mergedRelPath)
  8. mergeStrategy=overwrite: 删原轨 storeOps.Delete(videoPS/audioPS) + resourceStoreRepo.DeleteByResourceIdAndTypes([videoTrack,audioTrack])
  9. 删 tmpOut；返回 MergeResult{mergedStoreId: mergedPsId}
```

## 7. 侦察到的关键 file:line（实现时直接用）

| 用途 | 位置 | 说明 |
|------|------|------|
| merge 纯合并 | `backend/merge/ffmpeg.go:56` | `FFmpegMuxer.MergeRemux(ctx, video, audio, out)`（已完成） |
| ResourceStore 实体/常量 | `backend/base/model/entity/resource_store.go:9-45` | `StoreTypeVideoTrack/AudioTrack/Merged`、`GenerationDownloaded`、字段 StoreID/ResourceID/Generation |
| PersistentStore 实体 | `backend/base/model/entity/persistent_store.go:17-25` | FilePath/FileName/FilenameExtension（sql.NullString） |
| GetByType | `backend/resource/resource_store_repository.go:54-62` | 按 resource_id+store_type 查 ResourceStore |
| DeleteByResourceIdAndTypes | `backend/resource/resource_store_repository.go:64-77` | overwrite 删原轨关联（dbFromCtx 事务感知） |
| 挂 resource_store | `ResourceStoreRepository` 继承 `BaseRepository[ResourceStore]` | Save；事务内须 dbFromCtx 感知 |
| persistentStore.GetById | `backend/persistentStore/service.go:428` | StoreID → PersistentStore |
| persistentStore.GetAbsPath | `backend/persistentStore/service.go:601` | PersistentStore → 绝对路径 |
| persistentStore.StoreFromFile | `backend/persistentStore/service.go:417` | 临时文件 → 注册 PersistentStore |
| persistentStore.Delete | `backend/persistentStore/service.go:458` | 删 store（overwrite 删原轨/补偿） |
| persistentStore.CleanupFile | `backend/persistentStore/service.go:203` | 删磁盘文件（补偿） |
| **BuildVariantPath** | `backend/persistentStore/service.go` **新增** | 从源 FilePath 推导 `_merged` 路径（同目录+suffix+保扩展+净化） |
| 文件名净化 | `backend/util/filename/sanitize.go:19` | `filename.SanitizeFileName`（纯函数） |
| 设置项体系 | `backend/settings/model.go:4-13` + `service.go:52-135` | 加 MergeSettings 分组 |
| 事务 | `database.WithTransaction` + `database.DBFromContext` | resourceStoreRepo 已用 dbFromCtx |
| app.go 注入点 | `app.go:695`(resourceStoreRepo) `:696`(ResourceService) `:705`(PersistentStoreService) `:989`(ResourceHandler) | MergeService 在此 new、注入 handler |

## 8. 不在范围（归前端）

- DTO `mergeable`/`MergedStore`（`NewResourceFullDTO` 加 `case StoreTypeMerged` + 据“含 videoTrack+audioTrack”算 `Mergeable`）
- SDK DTO 加字段 + `wails3 generate bindings -ts`
- WorkDialog「合并」按钮 + 打开优先级 `merged > main`

> `mergeable` 主要服务前端按钮，归前端；后端 DTO 组装若顺手加 case 可在 resource 编排做，按钮消费在前端。

## 9. 验收清单

- [ ] merge 包无 store/resource 感知（只 `MergeRemux`，输入输出文件路径）。
- [ ] persistentStore 新增 `BuildVariantPath`（纯路径变换，不感知合并）。
- [ ] `MergeResource` 编排在 resource 模块，对含 videoTrack+audioTrack 的 Resource 产出 merged store。
- [ ] 产物路径从源 videoTrack FilePath 推导（同目录 `_merged`），不查 work/作者、不依赖 taskManager。
- [ ] `mergeStrategy=keep`：新建 merged store、原轨道保留；`=overwrite`：删原轨 store+文件。
- [ ] 缺 videoTrack/audioTrack 返回明确中文错误；ffmpeg 缺失返回 `ErrFFmpegNotFound`。
- [ ] 挂 resource_store 事务化，失败补偿删产物 store+文件。
- [ ] resource.Handler.MergeResource 注册（bindings 生成）。
- [ ] Backup/Restore 对 merged 透明。

## 10. 风险与决策点

- **路径方案（已定）**：从源 FilePath 推导 + persistentStore 新增 `BuildVariantPath`；`resolveMainPath` 不移动（下载命名策略，耦合模板/元数据）。**不查作者、不动 taskManager**。
- **MergeService vs resource.Service 增方法**：本文档选独立 `MergeService`（职责清晰）；若倾向最小改动，也可在 resource.Service 增 `MergeResource` + 扩构造函数。
- **事务边界（补偿语义）**：`StoreFromFile` 不保证感知外部事务，故用补偿（挂载失败 → 删产物 store+文件），而非单一大事务。实现时确认 `StoreFromFile` 内部 repo 是否事务感知——若已感知，可简化为单事务。
- **overwrite 删原轨**：破坏性操作，建议先 `keep` 验证，`overwrite` 作增强。
- **大文件阻塞 IPC**：同步 ffmpeg 可能阻塞 Wails IPC（参考 IPC 阻塞 memory）；remux 通常秒级，先同步观察。
