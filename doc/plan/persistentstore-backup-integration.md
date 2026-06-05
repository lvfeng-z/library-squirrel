# PersistentStore 接入 Backup 模块计划

## 现状分析

### 当前 PersistentStore.Delete 行为
`persistentStore.Service.Delete(ctx, id)` — 直接删除磁盘文件 + DB 记录，不可恢复。

### 当前 Backup 模块状态
- `backup.Service.MoveBackup()`（第 107 行）：**仍存在但无调用方**，将文件移动到 `{workdir}/backup/YYYY/MM/DD/` 目录
- `backup.Service.MoveBackupForResource()`（第 205 行）：**仍存在但无调用方**，封装了 MoveBackup + 记录原始路径元数据
- `ResourceBackupOrchestrator`：已简化为仅 disable/enable，不涉及文件操作

### 调用链
- `work.Service.DeleteWorkAndSurroundingData` → `storeDeleter.Delete(ctx, storeId)` — 级联删除时永久删除
- `persistentStore.Handler` — 前端直接调用（需移除，PersistentStore 不与前端交互）

### 依赖方向
当前：`work` → `persistentStore`（通过 `StoreDeleter` 接口），`persistentStore` 不依赖任何模块

## 目标设计

PersistentStore 删除已完成文件时，支持可选的移动备份：
- `Delete` 接受 `backup bool` 参数
- `backup=true` 时：移动文件到备份目录 + 创建 Backup DB 记录 + **删除 PersistentStore DB 记录** → 返回 `(backupId, error)`
- `backup=false` 时：直接删除文件 + DB 记录 → 返回 `(0, nil)`
- 仅对 `Status == StoreStatusComplete` 的记录生效；未完成记录始终直接清理
- PersistentStore 作为内部模块，不与前端交互，移除 handler

## 需要调整的内容

### 1. 移除 persistentStore.Handler

PersistentStore 是内部基础设施模块，不直接与前端交互。

**`backend/persistentStore/handler.go`** → 删除整个文件

**`app.go`**：
- 移除 `PersistentStoreHandler` 字段
- 移除 `app.PersistentStoreHandler = persistentStore.NewHandler(app.PersistentStoreService)` 初始化

### 2. PersistentStore.Service 新增 Backup 依赖

**`backend/persistentStore/service.go`**

新增调用方定义的接口（不直接引用 backup 包）：
```go
// FileMover 文件移动备份接口（由调用方定义，backup.Service 实现）
type FileMover interface {
    // MoveToBackup 将文件移动到备份目录并创建备份记录
    // sourceType: 备份来源类型（如 resource）
    // sourceId: 来源实体 ID
    // fileName: 文件名
    // absFilePath: 源文件绝对路径
    // 返回备份记录 ID
    MoveToBackup(ctx context.Context, sourceType int64, sourceId int64, fileName string, absFilePath string) (int64, error)
}
```

Service 新增字段：
```go
type Service struct {
    repo       Repository
    fileMover  FileMover  // 可选依赖，nil 时不备份
}
```

构造函数 `NewService` 新增 `fileMover FileMover` 参数。

### 3. PersistentStore.Service.Delete 签名变更

```go
// Delete 删除记录及对应文件
// backup: 是否对已完成文件进行移动备份（返回备份记录 ID）
func (s *Service) Delete(ctx context.Context, id int64, backup bool) (int64, error)
```

内部逻辑：
```
1. 查询记录，不存在则幂等返回 (0, nil)
2. if backup && Status == Complete && fileMover != nil:
     backupId, err = fileMover.MoveToBackup(ctx, sourceType, id, record.FileName, absPath)
     if err != nil: 记录日志，fallback 到直接 os.Remove(absPath)
   else:
     直接 os.Remove(absPath)
3. 删除 PersistentStore DB 记录
4. return backupId, nil
```

### 4. StoreWriter.Abort 不受影响
`storeWriter.Abort()` 调用的是 `w.repo.Delete()`（Repository 层直接删除），不走 Service.Delete，不需要备份。

### 5. work.Service StoreDeleter 接口调整

**`backend/work/service.go`**

```go
type StoreDeleter interface {
    // Delete 删除记录及对应文件
    // backup: 是否对已完成文件进行移动备份
    Delete(ctx context.Context, id int64, backup bool) (int64, error)
}
```

`DeleteWorkAndSurroundingData` 调用处：
```go
// 3. 删除关联的 PersistentStore 记录及磁盘文件
if s.storeDeleter != nil {
    resources, err := s.resourceDeleter.ListByWorkId(ctx, id)
    if err == nil {
        for _, res := range resources {
            if res.WorkStoreID.Valid {
                _, _ = s.storeDeleter.Delete(ctx, res.WorkStoreID.Int64, true)  // 备份后删除
            }
            if res.ThumbnailStoreID.Valid {
                _, _ = s.storeDeleter.Delete(ctx, res.ThumbnailStoreID.Int64, true)
            }
        }
    }
}
```

### 6. DeleteByFilePath 同步调整

**`backend/persistentStore/service.go`**

```go
func (s *Service) DeleteByFilePath(ctx context.Context, filePath string, backup bool) (int64, error) {
    ...
    return s.Delete(ctx, record.GetID(), backup)
}
```

### 7. backup.Service 适配

**`backend/backup/service.go`**

`MoveBackupForResource` 已存在（第 205 行）但参数针对旧 Resource 字段设计。需要新增或改造：

```go
// MoveToBackup 将文件移动到备份目录并创建备份记录
func (s *Service) MoveToBackup(ctx context.Context, sourceType int64, sourceId int64, fileName string, absFilePath string) (int64, error) {
    // 1. 调用已有的 MoveBackup 移动文件
    // 2. 创建 Backup DB 记录（记录 sourceType、sourceId、fileName、备份路径）
    // 3. 返回 backup ID
}
```

注意：Backup 实体的 `OriginalFilePath`/`OriginalFileName`/`OriginalFilenameExtension` 字段可用于记录 PersistentStore 的元数据。

### 8. app.go 依赖注入

```go
// persistentStore.Service 需要注入 backup.Service（作为 FileMover）
app.PersistentStoreService = persistentStore.NewService(persistentStoreRepo, app.BackupService)
```

注意初始化顺序：`BackupService` 需在 `PersistentStoreService` 之前创建。

## 调整文件清单

| 文件 | 变更 |
|------|------|
| `backend/persistentStore/handler.go` | **删除**（PersistentStore 不与前端交互） |
| `backend/persistentStore/service.go` | 新增 `FileMover` 接口、Service 新增 `fileMover` 字段、Delete 签名变更 |
| `backend/work/service.go` | `StoreDeleter` 接口签名变更、调用处传 `backup=true` |
| `backend/backup/service.go` | 新增 `MoveToBackup` 方法 |
| `app.go` | 移除 Handler 引用、`NewService` 注入 `BackupService`、调整初始化顺序 |

## 风险与注意事项

1. **循环依赖**：`persistentStore` → `backup` 单向依赖，`backup` 不依赖 `persistentStore`，无循环风险
2. **备份失败降级**：备份失败时应 fallback 到直接删除（记录日志），避免阻塞删除流程
3. **Backup 记录的清理**：备份后的 Backup 记录需要后续有机制清理（当前不在本计划范围）
4. **未完成记录**：`StoreWriter.Abort` 走 Repository 层直接删除，不经过 Service.Delete，无需备份
