# 备份还原 persistent_store.file_path UNIQUE 冲突修复（任务 L）

> 谱系：multitrack-resource-lineage · 节点 L（派生自 G）
> 创建：2026-07-12 · 根因已定位（经数据流验证）

## 根因

替换场景下，任务在 `startDownload` 事务**提交后**、下载阶段失败时：

1. `BackupStores`（model.go:840）已备份旧 store——删旧 persistent_store 记录 + 移文件到 backup 目录
2. `startDownload` 事务提交（model.go:922-943）：本次新建 persistent_store（含 thumbnail）+ `mountResourceStores`（:1085）把 resource_store 改挂到本次新建 store（先 `DeleteByResourceIdAndTypes` 清旧关联，再 SaveBatch 插新）
3. 下载失败 → `comboFail`→`setFailed`（model.go:1578）：**只改状态 + 清 pending_resource_id，不清理本次新建的 persistent_store**
4. `run()` defer（model.go:754）检测 Failed + storeBackupItems → `RestoreAllStores` → `StoreFromExternal(旧路径)`（orchestrator:175）
5. `StoreFromExternal`（service.go:567）INSERT 新记录时，旧路径已被本次新建的同路径 store 占用 → **UNIQUE 冲突**

`m.streams` 在事务成功后赋值（model.go:977）、事务失败时清空（:973）——印证 L 场景正是"事务成功后下载阶段失败"，此时 `m.streams` 有值且 store 已落库。

**main 常不冲突的原因**：日志显示旧 main 的 `resource_store.StoreID` 常指向已不存在的 persistent_store（孤儿行，Y 分支）→ `Delete` 返回 record not found → BackupID=0 → `RestoreAllStores` 对 BackupID≤0 直接 skipped（orchestrator:143）→ 不还原 main → 不冲突。备份成功的板块（如 thumbnail）才在还原时撞本次新建 store。

**设计冲突**：`downloadLoop` 注释"任一 failed → 保留已完成轨的 store"（model.go:1110），但替换场景失败应回滚到备份点——保留的新 store 与旧 store 路径冲突。**替换场景须以"还原旧 store"优先**。

## 本次修复范围：A + B + X（Y 另起延后分支）

### A — 失败时清理本次新建 store（根因修复）

位置：`run()` defer 还原分支（model.go:753-762），`RestoreAllStores` **之前**调用新方法 `cleanupCreatedStores()`。

动作：
- 遍历 `m.streams`（本次 startDownload 新建的 storeId + relPath）
- 对每个 storeId：删 persistent_store 记录 + 磁盘文件（复用 `persistentStore.Delete(id, backup=false)`）
- 删本次挂的 resource_store 关联（`ResourceStoreWriter.DeleteByResourceIdAndTypes(resourceId, 本次 roles)`，roles 取自 m.runMode.storeRoles 或 m.streams 的 role）

语义：替换场景失败 → 回滚本次下载产物到备份点，再还原旧 store。

**延伸缺口（A-full，待拍板）**：还原旧 store 后，resource_store 应重挂到还原的 store——否则 resource_store 指向已删的本次 store（断裂孤儿），作品资源不可用。当前 `RestoreAllStores` 只建 persistent_store、不挂 resource_store（orchestrator:181 注释"由调用方重建"，但还原分支未重建）。完整修复需 `RestoreAllStores` 在 `StoreBackupItem` 上回填 `NewStoreID`，调用方清理本次 resource_store 后用 (role→NewStoreID) 重挂。

### B — StoreFromExternal 创建前清同路径 DB 记录（防御）

位置：`StoreFromExternal`（service.go:514），os.Remove 磁盘文件处（:534）。

动作：创建记录前先 `DeleteByFilePath(relPath, false)` 清同路径旧 DB 记录（当前只清磁盘文件、不清记录，:534-538）。

语义：导入路径被占时覆盖（删旧记录 + 旧文件），即使调用方未清理也不 UNIQUE 冲突。兜底 A。

### X — BackupStores 区分 record not found（语义澄清）

位置：`BackupStores`（orchestrator:115-124）+ `Delete`（service.go:459）。

现状：`Delete` 失败（含 record not found）一律 WARN + append BackupID=0。

分析：record not found 是孤儿行（Y）的表现，旧记录本就不存在，skipped 合理，非"静默丢失"。真失败（文件移动/删除出错）才需 WARN。

修复：`Delete` 用 `errors.Is(err, gorm.ErrRecordNotFound)` 区分返回；`BackupStores` 对 not found 降级为正常跳过（降 DEBUG，非错误），只对真失败 WARN。孤儿行来源排查归 Y。

## 测试要点
- 替换场景下载阶段失败 → 本次新建 store 被清理 + 旧 store 还原成功（无 UNIQUE 冲突）
- `StoreFromExternal` 同路径已有记录 → 覆盖成功（不报 UNIQUE）
- `BackupStores` 遇孤儿行（record not found）不打 WARN

## 待用户拍板
- **A-full 是否含"还原后重挂 resource_store"？** 含 → 改 `RestoreAllStores` 回填 NewStoreID + 调用方重挂（还原后资源可用）；不含 → 还原后 resource_store 断裂，待任务重试时 `mountResourceStores` 修复（中间态资源不可用）。
