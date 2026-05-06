# DTO 重构开发计划

## 一、概述

### 1.1 目标

统一 LibrarySquirrel 项目中的 DTO 命名规范，修复 DTO 中直接嵌入 Entity 的违规问题，消除命名冲突，确保前后端 DTO 结构一致。

### 1.2 新的命名规范

| DTO 类型 | 命名规则 | 说明 | 位置 |
|---------|---------|------|------|
| 增删改参数 DTO | `${Module}ParamDTO` | 前端传入的增删改参数（字段精简） | `internal/{module}/handler.go` |
| 无 Null 实体映射 DTO | `${Module}DTO` | 实体的无 sql.Null* 版本（字段完整） | `backend/base/model/dto/` |
| 包含关联的完整 DTO | `${Module}FullDTO` | 组合 ${Module}DTO + 关联 DTO | `backend/base/model/dto/` |
| 关联关系 DTO | `${Module}RelateDTO` | 用于绑定/关联场景 | `backend/base/model/dto/` |

### 1.3 核心原则

- **DTO_COMPOSITION_OVER_EMBEDDING**: DTO 中禁止直接嵌入 Entity，允许组合其他 DTO
- **依赖倒置原则**: 接口由调用方定义
- **Wails Bindings 自动同步**: 后端 DTO 变更后，bindings 会自动生成 TypeScript 类型

---

## 二、变更总览

### 2.1 命名变更映射表

| 旧名称 | 新名称 | 位置变更 | 说明 |
|-------|-------|---------|------|
| `dto.SiteTagResultDTO` | `dto.SiteTagDTO` | `backend/base/model/dto/site_tag_dto.go` | 无 Null 实体映射 |
| `dto.LocalTagDTO` | `dto.LocalTagDTO` | `backend/base/model/dto/local_tag_dto.go` | 从 site_tag_dto.go 移出 |
| `dto.SiteTagFullDTO` | `dto.SiteTagFullDTO` | `backend/base/model/dto/site_tag_dto.go` | 修复 SiteTag 为指针类型，Site 改为 `*dto.SiteDTO` |
| `dto.SiteTagLocalRelateDTO` | `dto.SiteTagLocalRelateDTO` | `backend/base/model/dto/site_tag_dto.go` | 修复 LocalTag/Site 为 DTO 类型 |
| `dto.SiteAuthorFullDTO` | `dto.SiteAuthorFullDTO` | `backend/base/model/dto/site_author_dto.go` | 修复 LocalAuthor/Site 为 DTO 类型 |
| `dto.SiteAuthorLocalRelateDTO` | `dto.SiteAuthorLocalRelateDTO` | `backend/base/model/dto/site_author_dto.go` | 修复 LocalAuthor/Site 为 DTO 类型 |
| `siteTag.SiteTagDTO` | `siteTag.SiteTagParamDTO` | `internal/siteTag/handler.go` | 增删改参数 |
| `siteTag.SiteTagLocalRelateDTO` | `siteTag.SiteTagLocalRelateDTO` | `internal/siteTag/handler.go` | 保持不变（内部转换用） |
| `localTag.LocalTagDTO` | `localTag.LocalTagParamDTO` | `internal/localTag/handler.go` | 增删改参数 |
| `siteAuthor.SiteAuthorDTO` | `siteAuthor.SiteAuthorParamDTO` | `internal/siteAuthor/handler.go` | 增删改参数 |
| `siteAuthor.SiteAuthorResultDTO` | `dto.SiteAuthorDTO` | `backend/base/model/dto/site_author_dto.go` | 移至公共 DTO 层 |
| `siteAuthor.SiteAuthorFullDTO` | `dto.SiteAuthorFullDTO` | `backend/base/model/dto/site_author_dto.go` | 移至公共 DTO 层 |
| `siteAuthor.SiteAuthorLocalRelateDTO` | `dto.SiteAuthorLocalRelateDTO` | `backend/base/model/dto/site_author_dto.go` | 移至公共 DTO 层 |
| `siteAuthor.LocalAuthorDTO` | `dto.LocalAuthorDTO` | `backend/base/model/dto/local_author_dto.go` | 新建公共 DTO |
| `siteAuthor.RankedSiteAuthorWithWorkIdDTO` | `dto.RankedSiteAuthorWithWorkIdDTO` | `backend/base/model/dto/site_author_dto.go` | 移至公共 DTO 层 |
| **新建** | `dto.SiteDTO` | `backend/base/model/dto/site_dto.go` | 无 Null 版本的 Site 实体映射 |

### 2.2 实体字段映射参考

#### Site 实体 -> SiteDTO

```go
// entity.Site
SiteName        sql.NullString
SiteDescription sql.NullString
Homepage        sql.NullString

// dto.SiteDTO
SiteName        *string `json:"siteName"`
SiteDescription *string `json:"siteDescription"`
Homepage        *string `json:"homepage"`
```

#### LocalTag 实体 -> LocalTagDTO

```go
// entity.LocalTag
LocalTagName   sql.NullString
BaseLocalTagID sql.NullInt64
Description    sql.NullString
LastUse        sql.NullInt64

// dto.LocalTagDTO (已在 site_tag_dto.go 中定义，需移出)
ID             int64   `json:"id"`
LocalTagName   *string `json:"localTagName"`
BaseLocalTagID *int64  `json:"baseLocalTagId"`
Description    *string `json:"description"`
CreateTime     int64   `json:"createTime"`
UpdateTime     int64   `json:"updateTime"`
```

#### LocalAuthor 实体 -> LocalAuthorDTO

```go
// entity.LocalAuthor
AuthorName sql.NullString
Introduce  sql.NullString
LastUse    sql.NullInt64

// dto.LocalAuthorDTO (新建)
ID         int64   `json:"id"`
AuthorName *string `json:"authorName"`
Introduce  *string `json:"introduce"`
LastUse    *int64  `json:"lastUse"`
CreateTime int64   `json:"createTime"`
UpdateTime int64   `json:"updateTime"`
```

---

## 三、后端修改详细步骤

### 阶段 1: 新建公共 DTO 文件（无编译依赖，可最先执行）

#### 步骤 1.1: 新建 `backend/base/model/dto/site_dto.go`

**文件路径**: `E:\code\lvfeng\library-squirrel\pkg\model\dto\site_dto.go`

**内容**:

```go
package dto

import (
	"github.com/library-squirrel/wails/backend/util"
	"github.com/library-squirrel/wails/backend/base/model/entity"
)

// SiteDTO 站点数据传输对象（无 sql.Null* 版本）
type SiteDTO struct {
	ID              int64   `json:"id"`
	SiteName        *string `json:"siteName"`
	SiteDescription *string `json:"siteDescription"`
	Homepage        *string `json:"homepage"`
	CreateTime      int64   `json:"createTime"`
	UpdateTime      int64   `json:"updateTime"`
}

// NewSiteDTO 从 entity.Site 创建 SiteDTO
func NewSiteDTO(site *entity.Site) *SiteDTO {
	if site == nil {
		return nil
	}
	return &SiteDTO{
		ID:              site.GetID(),
		SiteName:        util.NullStringToPointer(site.SiteName),
		SiteDescription: util.NullStringToPointer(site.SiteDescription),
		Homepage:        util.NullStringToPointer(site.Homepage),
		CreateTime:      site.GetCreateTime(),
		UpdateTime:      site.GetUpdateTime(),
	}
}
```

#### 步骤 1.2: 新建 `backend/base/model/dto/local_author_dto.go`

**文件路径**: `E:\code\lvfeng\library-squirrel\pkg\model\dto\local_author_dto.go`

**内容**:

```go
package dto

import (
	"github.com/library-squirrel/wails/backend/util"
	"github.com/library-squirrel/wails/backend/base/model/entity"
)

// LocalAuthorDTO 本地作者数据传输对象（无 sql.Null* 版本）
type LocalAuthorDTO struct {
	ID         int64   `json:"id"`
	AuthorName *string `json:"authorName"`
	Introduce  *string `json:"introduce"`
	LastUse    *int64  `json:"lastUse"`
	CreateTime int64   `json:"createTime"`
	UpdateTime int64   `json:"updateTime"`
}

// NewLocalAuthorDTO 从 entity.LocalAuthor 创建 LocalAuthorDTO
func NewLocalAuthorDTO(author *entity.LocalAuthor) *LocalAuthorDTO {
	if author == nil {
		return nil
	}
	return &LocalAuthorDTO{
		ID:         author.GetID(),
		AuthorName: util.NullStringToPointer(author.AuthorName),
		Introduce:  util.NullStringToPointer(author.Introduce),
		LastUse:    util.NullInt64ToPointer(author.LastUse),
		CreateTime: author.GetCreateTime(),
		UpdateTime: author.GetUpdateTime(),
	}
}
```

#### 步骤 1.3: 新建 `backend/base/model/dto/local_tag_dto.go`

**文件路径**: `E:\code\lvfeng\library-squirrel\pkg\model\dto\local_tag_dto.go`

**内容**（从 `site_tag_dto.go` 移出并补充 `LastUse` 字段）:

```go
package dto

import (
	"github.com/library-squirrel/wails/backend/util"
	"github.com/library-squirrel/wails/backend/base/model/entity"
)

// LocalTagDTO 本地标签数据传输对象（无 sql.Null* 版本）
type LocalTagDTO struct {
	ID             int64   `json:"id"`
	LocalTagName   *string `json:"localTagName"`
	BaseLocalTagID *int64  `json:"baseLocalTagId"`
	Description    *string `json:"description"`
	LastUse        *int64  `json:"lastUse"`
	CreateTime     int64   `json:"createTime"`
	UpdateTime     int64   `json:"updateTime"`
}

// NewLocalTagDTO 从 entity.LocalTag 创建 LocalTagDTO
func NewLocalTagDTO(tag *entity.LocalTag) *LocalTagDTO {
	if tag == nil {
		return nil
	}
	return &LocalTagDTO{
		ID:             tag.GetID(),
		LocalTagName:   util.NullStringToPointer(tag.LocalTagName),
		BaseLocalTagID: util.NullInt64ToPointer(tag.BaseLocalTagID),
		Description:    util.NullStringToPointer(tag.Description),
		LastUse:        util.NullInt64ToPointer(tag.LastUse),
		CreateTime:     tag.GetCreateTime(),
		UpdateTime:     tag.GetUpdateTime(),
	}
}
```

---

### 阶段 2: 修改公共 DTO 文件

#### 步骤 2.1: 修改 `backend/base/model/dto/site_tag_dto.go`

**变更要点**:

1. `SiteTagResultDTO` -> `SiteTagDTO`
2. `SiteTagFullDTO.SiteTag` 从值类型改为指针类型 `*SiteTagDTO`
3. `SiteTagFullDTO.Site` 从 `*entity2.Site` 改为 `*dto.SiteDTO`
4. `SiteTagLocalRelateDTO.LocalTag` 从 `*entity2.LocalTag` 改为 `*dto.LocalTagDTO`
5. `SiteTagLocalRelateDTO.Site` 从 `*entity2.Site` 改为 `*dto.SiteDTO`
6. 移除内嵌的 `LocalTagDTO`（已移至 `local_tag_dto.go`）
7. 更新 `NewSiteTagFullDTO` 和 `NewSiteTagLocalRelateDTO` 构造函数
8. 移除 `entity2` import（如果不再使用）

**完整替换内容**:

```go
package dto

import (
	"github.com/library-squirrel/wails/backend/util"
	"github.com/library-squirrel/wails/backend/base/model/entity"
)

// SiteTagFullDTO 站点标签完整DTO（包含绑定的本地标签和来源站点信息）
type SiteTagFullDTO struct {
	SiteTag  *SiteTagDTO  `json:"siteTag,omitempty"`
	LocalTag *LocalTagDTO `json:"localTag,omitempty"`
	// 来源站点
	Site *SiteDTO `json:"site,omitempty"`
}

// NewSiteTagFullDTO 创建站点标签完整DTO
func NewSiteTagFullDTO(siteTag *entity.SiteTag) *SiteTagFullDTO {
	if siteTag == nil {
		return nil
	}
	dto := &SiteTagFullDTO{
		SiteTag: &SiteTagDTO{
			ID:            siteTag.GetID(),
			SiteID:        util.NullInt64ToPointer(siteTag.SiteID),
			SiteTagID:     util.NullStringToPointer(siteTag.SiteTagID),
			SiteTagName:   util.NullStringToPointer(siteTag.SiteTagName),
			BaseSiteTagID: util.NullStringToPointer(siteTag.BaseSiteTagID),
			Description:   util.NullStringToPointer(siteTag.Description),
			LocalTagID:    util.NullInt64ToPointer(siteTag.LocalTagID),
			LastUse:       util.NullInt64ToPointer(siteTag.LastUse),
			CreateTime:    siteTag.GetCreateTime(),
			UpdateTime:    siteTag.GetUpdateTime(),
		},
	}
	return dto
}

// SiteTagLocalRelateDTO 站点标签与本地标签关联DTO
// 注意：显式定义所有字段，不使用嵌入
type SiteTagLocalRelateDTO struct {
	// 基础实体字段
	ID         int64 `json:"id"`
	CreateTime int64 `json:"createTime"`
	UpdateTime int64 `json:"updateTime"`
	// 站点标签字段
	SiteID        int64  `json:"siteId"`
	SiteTagID     string `json:"siteTagId"`
	SiteTagName   string `json:"siteTagName"`
	BaseSiteTagID string `json:"baseSiteTagId"`
	Description   string `json:"description"`
	LocalTagID    int64  `json:"localTagId"`
	LastUse       int64  `json:"lastUse"`
	// 关联的本地标签
	LocalTag *LocalTagDTO `json:"localTag,omitempty"`
	// 来源站点
	Site *SiteDTO `json:"site,omitempty"`
	// 是否有同名本地标签
	HasSameNameLocalTag bool `json:"hasSameNameLocalTag"`
}

// NewSiteTagLocalRelateDTO 创建站点标签与本地标签关联DTO
func NewSiteTagLocalRelateDTO(siteTag *entity.SiteTag) *SiteTagLocalRelateDTO {
	if siteTag == nil {
		return nil
	}
	dto := &SiteTagLocalRelateDTO{
		ID:            siteTag.ID,
		CreateTime:    siteTag.CreateTime,
		UpdateTime:    siteTag.UpdateTime,
		SiteTagID:     siteTag.SiteTagID.String,
		SiteTagName:   siteTag.SiteTagName.String,
		BaseSiteTagID: siteTag.BaseSiteTagID.String,
		Description:   siteTag.Description.String,
	}
	if siteTag.SiteID.Valid {
		dto.SiteID = siteTag.SiteID.Int64
	}
	if siteTag.LocalTagID.Valid {
		dto.LocalTagID = siteTag.LocalTagID.Int64
	}
	if siteTag.LastUse.Valid {
		dto.LastUse = siteTag.LastUse.Int64
	}
	return dto
}

// SiteTagDTO 站点标签数据传输对象（无 sql.Null* 版本）
type SiteTagDTO struct {
	ID            int64   `json:"id"`
	SiteID        *int64  `json:"siteId"`
	SiteTagID     *string `json:"siteTagId"`
	SiteTagName   *string `json:"siteTagName"`
	BaseSiteTagID *string `json:"baseSiteTagId"`
	Description   *string `json:"description"`
	LocalTagID    *int64  `json:"localTagId"`
	LastUse       *int64  `json:"lastUse"`
	CreateTime    int64   `json:"createTime"`
	UpdateTime    int64   `json:"updateTime"`
}
```

#### 步骤 2.2: 修改 `backend/base/model/dto/site_author_dto.go`

**变更要点**:

1. `SiteAuthorFullDTO.LocalAuthor` 从 `*entity2.LocalAuthor` 改为 `*dto.LocalAuthorDTO`
2. `SiteAuthorFullDTO.Site` 从 `*entity2.Site` 改为 `*dto.SiteDTO`
3. `SiteAuthorLocalRelateDTO.LocalAuthor` 从 `*entity2.LocalAuthor` 改为 `*dto.LocalAuthorDTO`
4. `SiteAuthorLocalRelateDTO.Site` 从 `*entity2.Site` 改为 `*dto.SiteDTO`
5. 移除 `entity2` import
6. 更新 `NewSiteAuthorFullDTO` 和 `NewSiteAuthorLocalRelateDTO` 构造函数

**完整替换内容**:

```go
package dto

import (
	"github.com/library-squirrel/wails/backend/base/model/entity"
)

// SiteAuthorFullDTO 站点作者完整DTO（包含绑定的本地作者和来源站点信息）
// 注意：显式定义所有字段，不使用嵌入（embedding）来复现 TypeScript 的继承行为
type SiteAuthorFullDTO struct {
	// 基础实体字段
	ID         int64 `json:"id"`
	CreateTime int64 `json:"createTime"`
	UpdateTime int64 `json:"updateTime"`
	// 站点作者字段
	SiteID               int64  `json:"siteId"`
	SiteAuthorID         string `json:"siteAuthorId"`
	AuthorName           string `json:"authorName"`
	FixedAuthorName      string `json:"fixedAuthorName"`
	SiteAuthorNameBefore string `json:"siteAuthorNameBefore"`
	Introduce            string `json:"introduce"`
	LocalAuthorID        int64  `json:"localAuthorId"`
	LastUse              int64  `json:"lastUse"`
	// 关联的本地作者
	LocalAuthor *LocalAuthorDTO `json:"localAuthor,omitempty"`
	// 来源站点
	Site *SiteDTO `json:"site,omitempty"`
}

// NewSiteAuthorFullDTO 创建站点作者完整DTO
func NewSiteAuthorFullDTO(siteAuthor *entity.SiteAuthor) *SiteAuthorFullDTO {
	if siteAuthor == nil {
		return nil
	}
	dto := &SiteAuthorFullDTO{
		ID:                   siteAuthor.ID,
		CreateTime:           siteAuthor.CreateTime,
		UpdateTime:           siteAuthor.UpdateTime,
		SiteAuthorID:         siteAuthor.SiteAuthorID.String,
		AuthorName:           siteAuthor.AuthorName.String,
		FixedAuthorName:      siteAuthor.FixedAuthorName.String,
		SiteAuthorNameBefore: siteAuthor.SiteAuthorNameBefore.String,
		Introduce:            siteAuthor.Introduce.String,
	}
	if siteAuthor.SiteID.Valid {
		dto.SiteID = siteAuthor.SiteID.Int64
	}
	if siteAuthor.LocalAuthorID.Valid {
		dto.LocalAuthorID = siteAuthor.LocalAuthorID.Int64
	}
	if siteAuthor.LastUse.Valid {
		dto.LastUse = siteAuthor.LastUse.Int64
	}
	return dto
}

// SiteAuthorLocalRelateDTO 站点作者与本地作者关联DTO
// 注意：显式定义所有字段，不使用嵌入
type SiteAuthorLocalRelateDTO struct {
	// 基础实体字段
	ID         int64 `json:"id"`
	CreateTime int64 `json:"createTime"`
	UpdateTime int64 `json:"updateTime"`
	// 站点作者字段
	SiteID               int64  `json:"siteId"`
	SiteAuthorID         string `json:"siteAuthorId"`
	AuthorName           string `json:"authorName"`
	FixedAuthorName      string `json:"fixedAuthorName"`
	SiteAuthorNameBefore string `json:"siteAuthorNameBefore"`
	Introduce            string `json:"introduce"`
	LocalAuthorID        int64  `json:"localAuthorId"`
	LastUse              int64  `json:"lastUse"`
	// 关联的本地作者
	LocalAuthor *LocalAuthorDTO `json:"localAuthor,omitempty"`
	// 来源站点
	Site *SiteDTO `json:"site,omitempty"`
	// 是否有同名本地作者
	HasSameNameLocalAuthor bool `json:"hasSameNameLocalAuthor"`
}

// NewSiteAuthorLocalRelateDTO 创建站点作者与本地作者关联DTO
func NewSiteAuthorLocalRelateDTO(siteAuthor *entity.SiteAuthor) *SiteAuthorLocalRelateDTO {
	if siteAuthor == nil {
		return nil
	}
	dto := &SiteAuthorLocalRelateDTO{
		ID:                   siteAuthor.ID,
		CreateTime:           siteAuthor.CreateTime,
		UpdateTime:           siteAuthor.UpdateTime,
		SiteAuthorID:         siteAuthor.SiteAuthorID.String,
		AuthorName:           siteAuthor.AuthorName.String,
		FixedAuthorName:      siteAuthor.FixedAuthorName.String,
		SiteAuthorNameBefore: siteAuthor.SiteAuthorNameBefore.String,
		Introduce:            siteAuthor.Introduce.String,
	}
	if siteAuthor.SiteID.Valid {
		dto.SiteID = siteAuthor.SiteID.Int64
	}
	if siteAuthor.LocalAuthorID.Valid {
		dto.LocalAuthorID = siteAuthor.LocalAuthorID.Int64
	}
	if siteAuthor.LastUse.Valid {
		dto.LastUse = siteAuthor.LastUse.Int64
	}
	return dto
}
```

#### 步骤 2.3: 修改 `backend/base/model/dto/task_handler.go`

**变更要点**:

`WorkResponse` 中的 `SiteAuthors` 和 `SiteTags` 字段类型引用需要更新：

- `SiteAuthors []*SiteAuthorDTO` -> 保持不变（这是插件接口的 DTO，不是本次重构的 `dto.SiteAuthorDTO`）
- `SiteTags []*SiteTagDTO` -> 保持不变（同上）

> **注意**: `task_handler.go` 中的 `SiteAuthorDTO` 和 `SiteTagDTO` 是插件接口专用 DTO，与公共 DTO 层的命名冲突需要通过包名区分。当前 `task_handler.go` 在 `dto` 包内，所以 `SiteAuthorDTO` 和 `SiteTagDTO` 会与新定义的公共 DTO 冲突。
>
> **解决方案**: 将 `task_handler.go` 中的 `SiteAuthorDTO` 重命名为 `PluginSiteAuthorDTO`，`SiteTagDTO` 重命名为 `PluginSiteTagDTO`。

**修改内容**:

```go
// 在 task_handler.go 中，将：
type WorkResponse struct {
    // ...
    SiteAuthors []*SiteAuthorDTO `json:"siteAuthors"`
    SiteTags    []*SiteTagDTO    `json:"siteTags"`
    // ...
}

// SiteAuthorDTO 站点作者DTO
type SiteAuthorDTO struct {
    SiteAuthorID string `json:"siteAuthorId"`
    AuthorName   string `json:"authorName"`
    URL          string `json:"url"`
}

// SiteTagDTO 站点标签DTO
type SiteTagDTO struct {
    SiteTagID   string `json:"siteTagId"`
    TagName     string `json:"tagName"`
    Description string `json:"description"`
    URL         string `json:"url"`
}

// 改为：
type WorkResponse struct {
    // ...
    SiteAuthors []*PluginSiteAuthorDTO `json:"siteAuthors"`
    SiteTags    []*PluginSiteTagDTO    `json:"siteTags"`
    // ...
}

// PluginSiteAuthorDTO 插件站点作者DTO
type PluginSiteAuthorDTO struct {
    SiteAuthorID string `json:"siteAuthorId"`
    AuthorName   string `json:"authorName"`
    URL          string `json:"url"`
}

// PluginSiteTagDTO 插件站点标签DTO
type PluginSiteTagDTO struct {
    SiteTagID   string `json:"siteTagId"`
    TagName     string `json:"tagName"`
    Description string `json:"description"`
    URL         string `json:"url"`
}
```

**引用 `task_handler.go` 中 `SiteAuthorDTO`/`SiteTagDTO` 的文件清单**:

需要搜索并更新所有引用这两个类型的文件：

```bash
cd E:\code\lvfeng\library-squirrel
grep -r "dto\.SiteAuthorDTO\b" --include="*.go" .
grep -r "dto\.SiteTagDTO\b" --include="*.go" .
```

> 执行搜索后，逐一更新引用处。

---

### 阶段 3: 修改 Handler 层

#### 步骤 3.1: 修改 `internal/siteTag/handler.go`

**变更要点**:

1. `SiteTagDTO` -> `SiteTagParamDTO`（增删改参数 DTO）
2. `SiteTagResultDTO` -> `dto.SiteTagDTO`（所有返回和引用处）
3. `ToSiteTagResultDTO` -> `ToSiteTagDTO`（转换函数）
4. `ToLocalTagDTO` 函数中引用的 `dto.LocalTagDTO` 字段需要更新（新增 `LastUse` 字段）
5. `SiteTagLocalRelateDTO` 中嵌入的 `dto.SiteTagResultDTO` 改为 `dto.SiteTagDTO`
6. `ToSiteTagLocalRelateDTO` 函数中引用的字段需要更新

**具体修改**:

```go
// 1. 修改 Save 方法签名
func (h *Handler) Save(ctx context.Context, tag *SiteTagParamDTO) *model.ApiResponse[int64]

// 2. 修改 SaveBatch 方法签名
func (h *Handler) SaveBatch(ctx context.Context, tags []*SiteTagParamDTO) *model.ApiResponse[any]

// 3. 修改 Update 方法签名
func (h *Handler) Update(ctx context.Context, tag *SiteTagParamDTO) *model.ApiResponse[any]

// 4. 修改 GetById 返回类型
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto.SiteTagDTO]

// 5. 修改 QueryPage 返回类型
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.SiteTagDTO, SiteTagQueryDTO]) *model.ApiResponse[*model.Page[dto.SiteTagDTO, SiteTagQueryDTO]]

// 6. 修改 ListBySiteTagIds 返回类型
func (h *Handler) ListBySiteTagIds(ctx context.Context, siteTagIds []int64) *model.ApiResponse[[]*dto.SiteTagDTO]

// 7. 修改 ListByWorkId 返回类型
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*dto.SiteTagDTO]

// 8. 修改 CreateAndBindSameNameLocalTag 参数类型
func (h *Handler) CreateAndBindSameNameLocalTag(ctx context.Context, siteTag *SiteTagParamDTO) *model.ApiResponse[*dto.LocalTagDTO]

// 9. 重命名 DTO 定义
type SiteTagParamDTO struct {
    ID          int64   `json:"id"`
    SiteID      *int64  `json:"siteId"`
    SiteTagID   *string `json:"siteTagId"`
    SiteTagName *string `json:"siteTagName"`
    Description *string `json:"description"`
}

// 10. 修改 SiteTagLocalRelateDTO（Handler 内部使用的版本）
type SiteTagLocalRelateDTO struct {
    dto.SiteTagDTO  // 原来是 dto.SiteTagResultDTO
    LocalTag *dto.LocalTagDTO `json:"localTag,omitempty"`
}

// 11. 重命名转换函数
func ToSiteTagDTO(tag *entity.SiteTag) *dto.SiteTagDTO

// 12. 修改 ToSiteTagLocalRelateDTO 函数
func ToSiteTagLocalRelateDTO(fullDTO *dto.SiteTagLocalRelateDTO) *SiteTagLocalRelateDTO {
    // ... 将 SiteTagResultDTO 改为 SiteTagDTO
}

// 13. 修改 ToLocalTagDTO 函数（补充 LastUse 字段）
func ToLocalTagDTO(tag *entity.LocalTag) *dto.LocalTagDTO {
    if tag == nil {
        return nil
    }
    return &dto.LocalTagDTO{
        ID:             tag.GetID(),
        LocalTagName:   util.NullStringToPointer(tag.LocalTagName),
        BaseLocalTagID: util.NullInt64ToPointer(tag.BaseLocalTagID),
        Description:    util.NullStringToPointer(tag.Description),
        LastUse:        util.NullInt64ToPointer(tag.LastUse),  // 新增
        CreateTime:     tag.GetCreateTime(),
        UpdateTime:     tag.GetUpdateTime(),
    }
}
```

#### 步骤 3.2: 修改 `internal/localTag/handler.go`

**变更要点**:

1. `LocalTagDTO` -> `LocalTagParamDTO`（增删改参数 DTO）
2. `LocalTagResultDTO` -> `LocalTagDTO`（但注意：公共 DTO 层已有 `dto.LocalTagDTO`，Handler 中应直接使用 `dto.LocalTagDTO`）
3. `ToLocalTagResultDTO` -> `ToLocalTagDTO`

**具体修改**:

```go
// 1. 修改 Save 方法签名
func (h *Handler) Save(ctx context.Context, tag *LocalTagParamDTO) *model.ApiResponse[int64]

// 2. 修改 Update 方法签名
func (h *Handler) Update(ctx context.Context, tag *LocalTagParamDTO) *model.ApiResponse[any]

// 3. 修改 GetById 返回类型 - 使用 dto.LocalTagDTO
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto.LocalTagDTO]

// 4. 修改 QueryPage 返回类型
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.LocalTagDTO, LocalTagQueryDTO]) *model.ApiResponse[*model.Page[dto.LocalTagDTO, LocalTagQueryDTO]]

// 5. 修改 GetTree 返回类型
func (h *Handler) GetTree(ctx context.Context, rootId int64, depth int) *model.ApiResponse[[]*dto.LocalTagDTO]

// 6. 修改 ListByWorkId 返回类型
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*dto.LocalTagDTO]

// 7. 重命名 DTO 定义
type LocalTagParamDTO struct {
    ID             int64   `json:"id"`
    LocalTagName   *string `json:"localTagName"`
    BaseLocalTagID *int64  `json:"baseLocalTagId"`
}

// 8. 移除 LocalTagResultDTO 定义（使用 dto.LocalTagDTO 替代）
// 删除以下代码块：
// type LocalTagResultDTO struct { ... }

// 9. 修改转换函数
func ToLocalTagDTO(tag *domain.LocalTag) *dto.LocalTagDTO {
    if tag == nil {
        return nil
    }
    return &dto.LocalTagDTO{
        ID:             tag.GetID(),
        LocalTagName:   nullStringToPointer(tag.LocalTagName),
        BaseLocalTagID: nullInt64ToPointer(tag.BaseLocalTagID),
        LastUse:        nullInt64ToPointer(tag.LastUse),  // 新增
        CreateTime:     tag.GetCreateTime(),
        UpdateTime:     tag.GetUpdateTime(),
    }
}
```

#### 步骤 3.3: 修改 `internal/siteAuthor/handler.go`

**变更要点**:

1. `SiteAuthorDTO` -> `SiteAuthorParamDTO`
2. `SiteAuthorResultDTO` -> `dto.SiteAuthorDTO`
3. `SiteAuthorFullDTO` -> `dto.SiteAuthorFullDTO`
4. `SiteAuthorLocalRelateDTO` -> `dto.SiteAuthorLocalRelateDTO`
5. `LocalAuthorDTO` -> `dto.LocalAuthorDTO`
6. `RankedSiteAuthorWithWorkIdDTO` -> `dto.RankedSiteAuthorWithWorkIdDTO`
7. 移除 Handler 中定义的所有 DTO 类型和转换函数（全部移至 `backend/base/model/dto/`）
8. 所有转换函数改为引用 `backend/base/model/dto` 中的版本

**具体修改**:

```go
// 1. 修改 Save 方法签名
func (h *Handler) Save(ctx context.Context, author *SiteAuthorParamDTO) *model.ApiResponse[int64]

// 2. 修改 SaveBatch 方法签名
func (h *Handler) SaveBatch(ctx context.Context, authors []*SiteAuthorParamDTO) *model.ApiResponse[any]

// 3. 修改 Update 方法签名
func (h *Handler) Update(ctx context.Context, author *SiteAuthorParamDTO) *model.ApiResponse[any]

// 4. 修改 GetById 返回类型
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto.SiteAuthorDTO]

// 5. 修改 QueryPage 返回类型
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.SiteAuthorDTO, SiteAuthorQueryDTO]) *model.ApiResponse[*model.Page[dto.SiteAuthorDTO, SiteAuthorQueryDTO]]

// 6. 修改 QueryBoundOrUnboundToLocalAuthorPage 返回类型
func (h *Handler) QueryBoundOrUnboundToLocalAuthorPage(ctx context.Context, page *model.Page[dto.SiteAuthorFullDTO, SiteAuthorQueryDTO]) *model.ApiResponse[*model.Page[dto.SiteAuthorFullDTO, SiteAuthorQueryDTO]]

// 7. 修改 QueryLocalRelateDTOPage 返回类型
func (h *Handler) QueryLocalRelateDTOPage(ctx context.Context, page *model.Page[dto.SiteAuthorLocalRelateDTO, SiteAuthorQueryDTO]) *model.ApiResponse[*model.Page[dto.SiteAuthorLocalRelateDTO, SiteAuthorQueryDTO]]

// 8. 修改 ListBySiteAuthorIds 返回类型
func (h *Handler) ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) *model.ApiResponse[[]*dto.SiteAuthorDTO]

// 9. 修改 ListRankedSiteAuthorWithWorkIdByWorkIds 返回类型
func (h *Handler) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) *model.ApiResponse[[]*dto.RankedSiteAuthorWithWorkIdDTO]

// 10. 修改 CreateAndBindSameNameLocalAuthor 参数类型
func (h *Handler) CreateAndBindSameNameLocalAuthor(ctx context.Context, siteAuthor *SiteAuthorParamDTO) *model.ApiResponse[bool]

// 11. 重命名参数 DTO
type SiteAuthorParamDTO struct {
    ID           int64   `json:"id"`
    SiteID       *int64  `json:"siteId"`
    SiteAuthorID *string `json:"siteAuthorId"`
    AuthorName   *string `json:"authorName"`
    Introduce    *string `json:"introduce"`
}

// 12. 移除以下所有定义（已移至 backend/base/model/dto/）：
// - SiteAuthorResultDTO
// - SiteAuthorFullDTO
// - SiteAuthorLocalRelateDTO
// - RankedSiteAuthorWithWorkIdDTO
// - LocalAuthorDTO
// - ToSiteAuthorResultDTO
// - ToSiteAuthorFullDTO
// - ToSiteAuthorLocalRelateDTO
// - ToRankedSiteAuthorWithWorkIdDTO
// - ToLocalAuthorDTO
// - nullStringToPointer
// - nullInt64ToPointer
// - stringPtrIfValid
// - int64PtrIfValid

// 13. 保留但修改的转换函数：
// ToSiteAuthorDTO 从 entity 转换为 dto.SiteAuthorDTO
func ToSiteAuthorDTO(author *entity2.SiteAuthor) *dto.SiteAuthorDTO {
    if author == nil {
        return nil
    }
    return &dto.SiteAuthorDTO{
        ID:                   author.GetID(),
        SiteID:               nullInt64ToPointer(author.SiteID),
        SiteAuthorID:         nullStringToPointer(author.SiteAuthorID),
        AuthorName:           nullStringToPointer(author.AuthorName),
        FixedAuthorName:      nullStringToPointer(author.FixedAuthorName),
        SiteAuthorNameBefore: nullStringToPointer(author.SiteAuthorNameBefore),
        Introduce:            nullStringToPointer(author.Introduce),
        LocalAuthorID:        nullInt64ToPointer(author.LocalAuthorID),
        LastUse:              nullInt64ToPointer(author.LastUse),
        CreateTime:           author.GetCreateTime(),
        UpdateTime:           author.GetUpdateTime(),
    }
}
```

#### 步骤 3.4: 修改 `internal/localAuthor/handler.go`

**变更要点**:

1. `LocalAuthorDTO` -> `LocalAuthorParamDTO`
2. `LocalAuthorResultDTO` -> `dto.LocalAuthorDTO`
3. 移除 `LocalAuthorResultDTO` 定义和 `ToLocalAuthorResultDTO` 函数

**具体修改**:

```go
// 1. 修改 Save 方法签名
func (h *Handler) Save(ctx context.Context, author *LocalAuthorParamDTO) *model.ApiResponse[int64]

// 2. 修改 Update 方法签名
func (h *Handler) Update(ctx context.Context, author *LocalAuthorParamDTO) *model.ApiResponse[any]

// 3. 修改 GetById 返回类型
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto.LocalAuthorDTO]

// 4. 修改 QueryPage 返回类型
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.LocalAuthorDTO, LocalAuthorQueryDTO]) *model.ApiResponse[*model.Page[dto.LocalAuthorDTO, LocalAuthorQueryDTO]]

// 5. 重命名参数 DTO
type LocalAuthorParamDTO struct {
    ID         int64   `json:"id"`
    AuthorName *string `json:"authorName"`
    Introduce  *string `json:"introduce"`
}

// 6. 移除 LocalAuthorResultDTO 定义和 ToLocalAuthorResultDTO 函数
// 使用 dto.LocalAuthorDTO 和 dto.NewLocalAuthorDTO 替代
```

---

### 阶段 4: 修改 Repository 层

#### 步骤 4.1: 修改 `internal/siteTag/repository.go`

**变更要点**:

1. `QueryPageByWorkId` 方法中，查询关联数据时从直接赋值 `*entity2.Site` 改为构建 `*dto.SiteDTO`
2. `QueryLocalRelateDTOPage` 方法中，查询关联数据时从直接赋值 `*entity2.LocalTag` 和 `*entity2.Site` 改为构建 DTO
3. 移除 `dto2.LocalTagDTO{}` 的 GORM 直接查询（因为 `dto.LocalTagDTO` 不是 GORM 模型）

**具体修改**:

```go
// 在 QueryPageByWorkId 方法中，替换关联数据查询逻辑：

// 旧代码：
// dto.LocalTag = &dto2.LocalTagDTO{}
// if err := r.GORM().WithContext(ctx).First(dto.LocalTag, tag.LocalTagID.Int64).Error; ...
// dto.Site = &entity2.Site{}
// if err := r.GORM().WithContext(ctx).First(dto.Site, tag.SiteID.Int64).Error; ...

// 新代码：
// 查询关联的本地标签
if tag.LocalTagID.Valid && tag.LocalTagID.Int64 > 0 {
    var localTag entity2.LocalTag
    if err := r.GORM().WithContext(ctx).First(&localTag, tag.LocalTagID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
        return nil, err
    }
    if localTag.ID > 0 {
        dto.LocalTag = dto.NewLocalTagDTO(&localTag)
    }
}
// 查询关联的站点
if tag.SiteID.Valid && tag.SiteID.Int64 > 0 {
    var site entity2.Site
    if err := r.GORM().WithContext(ctx).First(&site, tag.SiteID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
        return nil, err
    }
    if site.ID > 0 {
        dto.Site = dto.NewSiteDTO(&site)
    }
}
```

```go
// 在 QueryLocalRelateDTOPage 方法中，同样替换关联数据查询逻辑：

// 旧代码：
// dto.LocalTag = &entity2.LocalTag{}
// if err := r.GORM().WithContext(ctx).First(dto.LocalTag, tag.LocalTagID.Int64).Error; ...
// dto.Site = &entity2.Site{}
// if err := r.GORM().WithContext(ctx).First(dto.Site, tag.SiteID.Int64).Error; ...

// 新代码（同 QueryPageByWorkId）：
// 查询关联的本地标签
if tag.LocalTagID.Valid && tag.LocalTagID.Int64 > 0 {
    var localTag entity2.LocalTag
    if err := r.GORM().WithContext(ctx).First(&localTag, tag.LocalTagID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
        return nil, err
    }
    if localTag.ID > 0 {
        dto.LocalTag = dto.NewLocalTagDTO(&localTag)
    }
}
// 查询关联的站点
if tag.SiteID.Valid && tag.SiteID.Int64 > 0 {
    var site entity2.Site
    if err := r.GORM().WithContext(ctx).First(&site, tag.SiteID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
        return nil, err
    }
    if site.ID > 0 {
        dto.Site = dto.NewSiteDTO(&site)
    }
}
```

#### 步骤 4.2: 修改 `internal/siteAuthor/repository.go`

**变更要点**:

与 `siteTag/repository.go` 类似，替换 `QueryBoundOrUnboundToLocalAuthorPage` 和 `QueryLocalRelateDTOPage` 中的关联数据查询逻辑。

**具体修改**:

```go
// 在 QueryBoundOrUnboundToLocalAuthorPage 方法中：

// 查询关联的本地作者
if author.LocalAuthorID.Valid && author.LocalAuthorID.Int64 > 0 {
    var localAuthor entity2.LocalAuthor
    if err := r.GORM().WithContext(ctx).First(&localAuthor, author.LocalAuthorID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
        return nil, err
    }
    if localAuthor.ID > 0 {
        dto.LocalAuthor = dto.NewLocalAuthorDTO(&localAuthor)
    }
}
// 查询关联的站点
if author.SiteID.Valid && author.SiteID.Int64 > 0 {
    var site entity2.Site
    if err := r.GORM().WithContext(ctx).First(&site, author.SiteID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
        return nil, err
    }
    if site.ID > 0 {
        dto.Site = dto.NewSiteDTO(&site)
    }
}
```

```go
// 在 QueryLocalRelateDTOPage 方法中，同样替换关联数据查询逻辑（同上）
```

---

### 阶段 5: 修改 Service 层

#### 步骤 5.1: 修改 `internal/siteTag/service.go`

**变更要点**:

1. `enrichSiteTagsWithRelations` 方法中，`siteMap` 的类型从 `map[int64]*entity2.Site` 改为 `map[int64]*dto.SiteDTO`
2. 构建 `siteMap` 时使用 `dto.NewSiteDTO` 转换
3. `siteTagOperator` 和 `siteQueryOp` 接口可能需要调整（如果它们返回实体类型）

**具体修改**:

```go
// 在 enrichSiteTagsWithRelations 方法中：

// 旧代码：
// siteMap := make(map[int64]*entity2.Site)
// if len(siteIds) > 0 {
//     sites, err := s.siteQueryOp.ListByIds(ctx, unique(siteIds))
//     ...
//     for _, st := range sites {
//         siteMap[st.ID] = st
//     }
// }

// 新代码：
siteMap := make(map[int64]*dto.SiteDTO)
if len(siteIds) > 0 {
    sites, err := s.siteQueryOp.ListByIds(ctx, unique(siteIds))
    if err != nil {
        return nil, err
    }
    for _, st := range sites {
        siteMap[st.ID] = dto.NewSiteDTO(st)
    }
}
```

#### 步骤 5.2: 修改 `internal/siteAuthor/service.go`

**变更要点**:

检查是否有类似 `enrichSiteTagsWithRelations` 的方法需要修改。当前 `siteAuthor/service.go` 中没有直接填充关联 DTO 的逻辑（由 Repository 层处理），所以改动较少。

主要检查接口签名是否匹配：

```go
// 确认 Repository 接口中返回 dto.SiteAuthorFullDTO 和 dto.SiteAuthorLocalRelateDTO 的方法签名
// 已在 repository.go 中修改，service.go 只需透传，无需额外改动
```

---

### 阶段 6: 编译验证

执行以下命令验证编译：

```bash
cd E:\code\lvfeng\library-squirrel
go build ./...
```

如果编译失败，根据错误信息修复：

1. **类型不匹配**: 检查 DTO 字段类型是否一致
2. **未定义的类型**: 检查 import 路径和包名
3. **方法签名不匹配**: 检查接口实现是否匹配新的签名

---

## 四、前端修改详细步骤

### 阶段 7: 重新生成 Wails Bindings

```bash
cd E:\code\lvfeng\library-squirrel
wails3 generate bindings -ts
```

> 如果 `wails3` 命令不可用，尝试 `wails generate bindings -ts` 或查看项目文档中的 bindings 生成命令。

### 阶段 8: 更新前端自定义 DTO

#### 步骤 8.1: 更新 `frontend/src/model/model/dto/SiteTagFullDTO.ts`

后端 `dto.SiteTagFullDTO` 的结构已变更：

- `siteTag` 从值类型变为指针类型（JSON 中不变，但 TypeScript 类型需要适配）
- `site` 从 `Site` 实体变为 `SiteDTO`
- `localTag` 从 `LocalTag` 实体变为 `LocalTagDTO`

**修改方案**:

由于 Wails bindings 会自动生成新的 TypeScript 类型，前端自定义 DTO 需要决定：

**方案 A**: 删除自定义 DTO，直接使用 bindings 生成的类型（推荐，如果不需要额外字段）
**方案 B**: 保留自定义 DTO，但改为组合而非继承，并适配新的字段类型

如果选择方案 B：

```typescript
// SiteTagFullDTO.ts
import SiteTagDTO from '@bindings/github.com/library-squirrel/backend/base/model/dto' // 需要确认实际路径
import LocalTagDTO from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import SiteDTO from '@bindings/github.com/library-squirrel/backend/base/model/dto'

export default class SiteTagFullDTO {
  siteTag: SiteTagDTO | undefined | null
  localTag: LocalTagDTO | undefined | null
  site: SiteDTO | undefined | null

  constructor(data?: any) {
    if (data) {
      this.siteTag = data.siteTag
      this.localTag = data.localTag
      this.site = data.site
    }
  }
}
```

#### 步骤 8.2: 更新 `frontend/src/model/model/dto/LocalTagDTO.ts`

当前前端自定义 `LocalTagDTO` 继承自 `LocalTag` 实体并添加 `isLeaf` 和 `baseTag` 字段。

Bindings 生成的新 `LocalTagDTO` 包含完整字段（ID, LocalTagName, BaseLocalTagID, Description, LastUse, CreateTime, UpdateTime）。

**决策**: 由于存在命名冲突（bindings 的 `LocalTagDTO` 和前端自定义的 `LocalTagDTO`），建议：

1. 将前端自定义的 `LocalTagDTO` 重命名为 `LocalTagTreeNode` 或 `LocalTagViewModel`
2. 或者删除自定义 DTO，在组件中直接使用 bindings 类型并添加需要的字段

#### 步骤 8.3: 更新 `frontend/src/model/model/dto/SiteAuthorFullDTO.ts`

与 `SiteTagFullDTO` 类似，改为组合 bindings 类型：

```typescript
import SiteAuthorDTO from '@bindings/...'
import LocalAuthorDTO from '@bindings/...'
import SiteDTO from '@bindings/...'

export default class SiteAuthorFullDTO {
  // 显式定义所有字段（与 Go 端一致）
  id: number
  createTime: number
  updateTime: number
  siteId: number
  siteAuthorId: string
  authorName: string
  fixedAuthorName: string
  siteAuthorNameBefore: string
  introduce: string
  localAuthorId: number
  lastUse: number
  localAuthor: LocalAuthorDTO | undefined | null
  site: SiteDTO | undefined | null

  constructor(data?: any) {
    // ...
  }
}
```

#### 步骤 8.4: 更新 `frontend/src/model/model/dto/SiteAuthorLocalRelateDTO.ts`

改为继承或组合新的 `SiteAuthorFullDTO`。

#### 步骤 8.5: 更新 `frontend/src/model/model/dto/SiteTagLocalRelateDTO.ts`

改为继承或组合新的 `SiteTagFullDTO`。

#### 步骤 8.6: 更新 `frontend/src/model/model/dto/SiteTagFullWithWorkIdDTO.ts`

改为继承或组合新的 `SiteTagFullDTO`。

### 阶段 9: 更新前端引用代码

#### 步骤 9.1: 更新 API 包装器

**文件**: `frontend/src/apis/http/wrappers/siteTag.ts`

变更要点：

1. `SiteTagDTO` -> `SiteTagParamDTO`（来自 `internal/siteTag` bindings）
2. `SiteTagResultDTO` -> `SiteTagDTO`（来自 `backend/base/model/dto` bindings）
3. `SiteTagLocalRelateDTO` 引用路径可能需要调整

```typescript
// 旧 import：
import {
  Handler as SiteTagHandler,
  SiteTagDTO, SiteTagFullDTO,
  SiteTagQueryDTO,
  SiteTagResultDTO
} from "@bindings/github.com/library-squirrel/backend/siteTag";

// 新 import：
import {
  Handler as SiteTagHandler,
  SiteTagParamDTO,  // 增删改参数
  SiteTagQueryDTO,
} from "@bindings/github.com/library-squirrel/backend/siteTag";
import {
  SiteTagDTO,       // 无 Null 实体映射
  SiteTagFullDTO,
  SiteTagLocalRelateDTO,
} from "@bindings/github.com/library-squirrel/backend/base/model/dto";
```

**文件**: `frontend/src/apis/http/wrappers/siteAuthor.ts`

类似修改：

```typescript
// 旧 import：
import { Handler as SiteAuthorHandler, SiteAuthorDTO, SiteAuthorQueryDTO, SiteAuthorResultDTO, SiteAuthorFullDTO, SiteAuthorLocalRelateDTO } from '@bindings/.../internal/siteAuthor'

// 新 import：
import {
  Handler as SiteAuthorHandler,
  SiteAuthorParamDTO,  // 增删改参数
  SiteAuthorQueryDTO,
} from "@bindings/github.com/library-squirrel/backend/siteAuthor";
import {
  SiteAuthorDTO,       // 无 Null 实体映射
  SiteAuthorFullDTO,
  SiteAuthorLocalRelateDTO,
  RankedSiteAuthorWithWorkIdDTO,
} from "@bindings/github.com/library-squirrel/backend/base/model/dto";
```

**文件**: `frontend/src/apis/http/wrappers/localTag.ts`

```typescript
// 旧 import：
import { Handler as LocalTagHandler, LocalTagDTO, LocalTagQueryDTO, LocalTagResultDTO } from '@bindings/.../internal/localTag'

// 新 import：
import {
  Handler as LocalTagHandler,
  LocalTagParamDTO,  // 增删改参数
  LocalTagQueryDTO,
} from "@bindings/github.com/library-squirrel/backend/localTag";
import {
  LocalTagDTO,       // 无 Null 实体映射
} from "@bindings/github.com/library-squirrel/backend/base/model/dto";
```

**文件**: `frontend/src/apis/http/wrappers/localAuthor.ts`

```typescript
// 旧 import：
import { Handler as LocalAuthorHandler, LocalAuthorDTO, LocalAuthorQueryDTO, LocalAuthorResultDTO } from '@bindings/.../internal/localAuthor'

// 新 import：
import {
  Handler as LocalAuthorHandler,
  LocalAuthorParamDTO,  // 增删改参数
  LocalAuthorQueryDTO,
} from "@bindings/github.com/library-squirrel/backend/localAuthor";
import {
  LocalAuthorDTO,       // 无 Null 实体映射
} from "@bindings/github.com/library-squirrel/backend/base/model/dto";
```

#### 步骤 9.2: 更新 Vue 组件中的类型引用

**文件**: `frontend/src/views/SiteTagManage.vue`

```typescript
// 旧 import：
import { LocalTagDTO, SelectItem, SiteTagFullDTO } from "@bindings/.../backend/base/model/dto"
import { SiteTagLocalRelateDTO } from '@bindings/.../internal/siteTag/models'
import { SiteTagDTO as SiteTag } from '@bindings/.../internal/siteTag/models'

// 新 import：
import { SelectItem, SiteTagFullDTO, SiteTagLocalRelateDTO, SiteTagDTO } from "@bindings/.../backend/base/model/dto"
import { SiteTagParamDTO as SiteTag } from '@bindings/.../internal/siteTag/models'
```

**文件**: `frontend/src/views/LocalTagManage.vue`

```typescript
// 旧 import：
import { SelectItem, SiteTagFullDTO } from "@bindings/.../backend/base/model/dto"
import { LocalTagQueryDTO, LocalTagDTO } from '@bindings/.../internal/localTag/models'

// 新 import：
import { SelectItem, SiteTagFullDTO, LocalTagDTO } from "@bindings/.../backend/base/model/dto"
import { LocalTagQueryDTO, LocalTagParamDTO } from '@bindings/.../internal/localTag/models'
```

**文件**: `frontend/src/views/SiteAuthorManage.vue`

```typescript
// 旧 import：
import { SiteAuthorLocalRelateDTO } from '@bindings/.../internal/siteAuthor/models'
import { LocalAuthorDTO as LocalAuthor } from '@bindings/.../internal/localAuthor/models'
import { SiteAuthorDTO as SiteAuthor } from '@bindings/.../internal/siteAuthor/models'

// 新 import：
import { SiteAuthorLocalRelateDTO, LocalAuthorDTO as LocalAuthor } from "@bindings/.../backend/base/model/dto"
import { SiteAuthorParamDTO as SiteAuthor } from '@bindings/.../internal/siteAuthor/models'
```

**文件**: `frontend/src/views/LocalAuthorManage.vue`

```typescript
// 旧 import：
import { LocalAuthorDTO as LocalAuthor } from '@bindings/.../internal/localAuthor/models'

// 新 import：
import { LocalAuthorDTO as LocalAuthor } from "@bindings/.../backend/base/model/dto"
```

#### 步骤 9.3: 更新 Dialog 组件

**文件**: `frontend/src/components/dialogs/SiteTagDialog.vue`

```typescript
// 旧 import：
import SiteTagLocalRelateDTO from '@renderer/model/model/dto/SiteTagLocalRelateDTO.ts'

// 新 import（如果使用 bindings 类型）：
import { SiteTagLocalRelateDTO } from "@bindings/.../backend/base/model/dto"
// 或继续使用自定义 DTO，但需适配新结构
```

**文件**: `frontend/src/components/dialogs/SiteAuthorDialog.vue`

类似修改。

**文件**: `frontend/src/components/dialogs/LocalTagDialog.vue`

```typescript
// 旧 import：
import LocalTagDTO from '@renderer/model/model/dto/LocalTagDTO.ts'

// 新 import：
import { LocalTagDTO } from "@bindings/.../backend/base/model/dto"
// 或重命名前端自定义 DTO
```

**文件**: `frontend/src/components/dialogs/WorkDialog.vue`

```typescript
// 检查 SiteTagFullDTO 引用是否需要更新
import SiteTagFullDTO from '@renderer/model/model/dto/SiteTagFullDTO.ts'
// 或使用 bindings 类型
```

---

## 五、修改顺序建议

为避免中间状态编译失败，建议按以下顺序执行：

### 第一轮：新建文件（无依赖）

1. 新建 `backend/base/model/dto/site_dto.go`
2. 新建 `backend/base/model/dto/local_author_dto.go`
3. 新建 `backend/base/model/dto/local_tag_dto.go`

### 第二轮：修改公共 DTO（依赖第一轮）

4. 修改 `backend/base/model/dto/site_tag_dto.go`
5. 修改 `backend/base/model/dto/site_author_dto.go`
6. 修改 `backend/base/model/dto/task_handler.go`（重命名 PluginSiteAuthorDTO/PluginSiteTagDTO）

### 第三轮：修改 Handler（依赖第二轮）

7. 修改 `internal/siteTag/handler.go`
8. 修改 `internal/localTag/handler.go`
9. 修改 `internal/siteAuthor/handler.go`
10. 修改 `internal/localAuthor/handler.go`

### 第四轮：修改 Repository（依赖第二轮）

11. 修改 `internal/siteTag/repository.go`
12. 修改 `internal/siteAuthor/repository.go`

### 第五轮：修改 Service（依赖第四轮）

13. 修改 `internal/siteTag/service.go`
14. 修改 `internal/siteAuthor/service.go`

### 第六轮：编译验证

15. 执行 `go build ./...`
16. 修复编译错误

### 第七轮：前端适配

17. 重新生成 Wails bindings
18. 更新前端自定义 DTO
19. 更新前端 API 包装器
20. 更新 Vue 组件
21. 前端构建验证

---

## 六、引用清单

### 6.1 后端引用清单

#### `dto.SiteTagResultDTO` 引用处

| 文件 | 行号 | 用途 |
|------|------|------|
| `internal/siteTag/handler.go` | 124, 133, 142, 213, 219, 265, 271, 316, 340 | 返回类型、转换函数 |
| `backend/base/model/dto/site_tag_dto.go` | 32, 97 | DTO 定义 |
| `backend/base/model/dto/task_handler.go` | 74 | WorkResponse.SiteTags（需检查） |

#### `dto.LocalTagDTO` 引用处

| 文件 | 行号 | 用途 |
|------|------|------|
| `internal/siteTag/handler.go` | 236, 261, 312, 357-368 | 返回类型、转换函数 |
| `internal/siteTag/repository.go` | 145 | 查询赋值 |
| `internal/siteTag/service.go` | 225-240 | 构建 LocalTagDTO |
| `backend/base/model/dto/site_tag_dto.go` | 16-24 | DTO 定义 |

#### `dto.SiteTagFullDTO` 引用处

| 文件 | 行号 | 用途 |
|------|------|------|
| `internal/siteTag/handler.go` | 157, 191, 200 | 返回类型 |
| `internal/siteTag/repository.go` | 102, 141, 160 | 返回类型、构建 |
| `internal/siteTag/service.go` | 156, 206, 256, 261, 285, 361 | 返回类型、构建 |

#### `dto.SiteTagLocalRelateDTO` 引用处

| 文件 | 行号 | 用途 |
|------|------|------|
| `internal/siteTag/handler.go` | 166, 175, 179, 185, 309-314, 335-354 | 返回类型、转换函数 |
| `internal/siteTag/repository.go` | 164, 203, 227 | 返回类型、构建 |
| `internal/siteTag/service.go` | 303, 366 | 返回类型 |

#### `dto.SiteAuthorFullDTO` 引用处

| 文件 | 行号 | 用途 |
|------|------|------|
| `internal/siteAuthor/handler.go` | 158, 167, 171, 317-320, 365-385 | 返回类型、转换函数、DTO 定义 |
| `internal/siteAuthor/repository.go` | 221, 264, 283 | 返回类型、构建 |
| `internal/siteAuthor/service.go` | 45, 136 | 返回类型 |

#### `dto.SiteAuthorLocalRelateDTO` 引用处

| 文件 | 行号 | 用途 |
|------|------|------|
| `internal/siteAuthor/handler.go` | 183, 192, 196, 202, 322-326, 388-407 | 返回类型、转换函数、DTO 定义 |
| `internal/siteAuthor/repository.go` | 287, 320, 343 | 返回类型、构建 |
| `internal/siteAuthor/service.go` | 47, 163, 249 | 返回类型 |

### 6.2 前端引用清单

#### Bindings 导入引用

| 文件 | 引用的 Bindings 类型 |
|------|---------------------|
| `frontend/src/apis/http/wrappers/siteTag.ts` | `SiteTagDTO`, `SiteTagFullDTO`, `SiteTagResultDTO`, `SiteTagQueryDTO` |
| `frontend/src/apis/http/wrappers/localTag.ts` | `LocalTagDTO`, `LocalTagResultDTO`, `LocalTagQueryDTO` |
| `frontend/src/apis/http/wrappers/siteAuthor.ts` | `SiteAuthorDTO`, `SiteAuthorResultDTO`, `SiteAuthorFullDTO`, `SiteAuthorLocalRelateDTO`, `SiteAuthorQueryDTO`, `RankedSiteAuthorWithWorkIdDTO` |
| `frontend/src/apis/http/wrappers/localAuthor.ts` | `LocalAuthorDTO`, `LocalAuthorResultDTO`, `LocalAuthorQueryDTO` |
| `frontend/src/views/SiteTagManage.vue` | `LocalTagDTO`, `SiteTagFullDTO`, `SiteTagLocalRelateDTO`, `SiteTagDTO` |
| `frontend/src/views/LocalTagManage.vue` | `SiteTagFullDTO`, `LocalTagDTO`, `LocalTagQueryDTO` |
| `frontend/src/views/SiteAuthorManage.vue` | `SiteAuthorLocalRelateDTO`, `LocalAuthorDTO`, `SiteAuthorDTO` |
| `frontend/src/views/LocalAuthorManage.vue` | `LocalAuthorDTO` |
| `frontend/src/components/dialogs/WorkDialog.vue` | `SiteTagFullDTO` |
| `frontend/src/components/dialogs/SiteTagDialog.vue` | `SiteTagLocalRelateDTO` |
| `frontend/src/components/dialogs/SiteAuthorDialog.vue` | `SiteAuthorLocalRelateDTO` |
| `frontend/src/components/dialogs/LocalTagDialog.vue` | `LocalTagDTO` |

#### 前端自定义 DTO 引用

| 文件 | 引用的自定义 DTO |
|------|-----------------|
| `frontend/src/model/model/dto/WorkFullDTO.ts` | `SiteTagFullDTO` |
| `frontend/src/model/model/dto/SiteTagLocalRelateDTO.ts` | `SiteTagFullDTO` |
| `frontend/src/model/model/dto/SiteTagFullWithWorkIdDTO.ts` | `SiteTagFullDTO` |
| `frontend/src/model/model/dto/SiteAuthorLocalRelateDTO.ts` | `SiteAuthorFullDTO` |

---

## 七、构建验证步骤

### 7.1 后端验证

```bash
cd E:\code\lvfeng\library-squirrel

# 1. 格式化代码
go fmt ./...

# 2. 编译检查
go build ./...

# 3. 运行测试（如果有）
go test ./...

# 4. vet 检查
go vet ./...
```

### 7.2 前端验证

```bash
cd E:\code\lvfeng\library-squirrel\frontend

# 1. 安装依赖
npm install

# 2. TypeScript 类型检查
npx vue-tsc --noEmit

# 3. 构建
npm run build

# 4. 或开发模式启动
npm run dev
```

### 7.3 完整应用验证

```bash
cd E:\code\lvfeng\library-squirrel

# Wails 构建
wails build
# 或
wails dev
```

---

## 八、风险与回滚方案

### 8.1 风险点

1. **Wails Bindings 生成失败**: 如果 Go 代码有编译错误，bindings 无法生成
2. **前端类型不匹配**: bindings 生成后，前端大量类型引用需要更新
3. **运行时 JSON 序列化问题**: 指针类型与值类型的 JSON 序列化行为差异
4. **循环依赖**: `backend/base/model/dto` 包引入 `backend/base/model/entity` 和 `internal/util`，需确认无循环依赖

### 8.2 回滚方案

1. 在执行修改前创建分支：
   ```bash
   git checkout -b dto-refactor
   ```
2. 每完成一个阶段提交一次：
   ```bash
   git add .
   git commit -m "dto refactor: 阶段 X - 描述"
   ```
3. 如遇到无法解决的问题，可回滚到上一个提交或切换回主分支

---

## 九、附录

### 9.1 完整的文件变更清单

#### 新建文件

- `backend/base/model/dto/site_dto.go`
- `backend/base/model/dto/local_author_dto.go`
- `backend/base/model/dto/local_tag_dto.go`

#### 修改文件

**公共 DTO 层**:
- `backend/base/model/dto/site_tag_dto.go`
- `backend/base/model/dto/site_author_dto.go`
- `backend/base/model/dto/task_handler.go`

**Handler 层**:
- `internal/siteTag/handler.go`
- `internal/localTag/handler.go`
- `internal/siteAuthor/handler.go`
- `internal/localAuthor/handler.go`

**Repository 层**:
- `internal/siteTag/repository.go`
- `internal/siteAuthor/repository.go`

**Service 层**:
- `internal/siteTag/service.go`
- `internal/siteAuthor/service.go`

**前端 Bindings**（自动生成）:
- `frontend/bindings/github.com/library-squirrel/wails/backend/base/model/dto/models.ts`
- `frontend/bindings/github.com/library-squirrel/wails/backend/base/model/dto/index.ts`
- `frontend/bindings/github.com/library-squirrel/wails/backend/siteTag/models.ts`
- `frontend/bindings/github.com/library-squirrel/wails/backend/siteTag/index.ts`
- `frontend/bindings/github.com/library-squirrel/wails/backend/siteAuthor/models.ts`
- `frontend/bindings/github.com/library-squirrel/wails/backend/siteAuthor/index.ts`
- `frontend/bindings/github.com/library-squirrel/wails/backend/localTag/models.ts`
- `frontend/bindings/github.com/library-squirrel/wails/backend/localTag/index.ts`
- `frontend/bindings/github.com/library-squirrel/wails/backend/localAuthor/models.ts`
- `frontend/bindings/github.com/library-squirrel/wails/backend/localAuthor/index.ts`

**前端自定义 DTO**:
- `frontend/src/model/model/dto/SiteTagFullDTO.ts`
- `frontend/src/model/model/dto/LocalTagDTO.ts`
- `frontend/src/model/model/dto/SiteAuthorFullDTO.ts`
- `frontend/src/model/model/dto/SiteAuthorLocalRelateDTO.ts`
- `frontend/src/model/model/dto/SiteTagLocalRelateDTO.ts`
- `frontend/src/model/model/dto/SiteTagFullWithWorkIdDTO.ts`

**前端 API 包装器**:
- `frontend/src/apis/http/wrappers/siteTag.ts`
- `frontend/src/apis/http/wrappers/localTag.ts`
- `frontend/src/apis/http/wrappers/siteAuthor.ts`
- `frontend/src/apis/http/wrappers/localAuthor.ts`

**前端 Vue 组件**:
- `frontend/src/views/SiteTagManage.vue`
- `frontend/src/views/LocalTagManage.vue`
- `frontend/src/views/SiteAuthorManage.vue`
- `frontend/src/views/LocalAuthorManage.vue`
- `frontend/src/components/dialogs/WorkDialog.vue`
- `frontend/src/components/dialogs/SiteTagDialog.vue`
- `frontend/src/components/dialogs/SiteAuthorDialog.vue`
- `frontend/src/components/dialogs/LocalTagDialog.vue`

### 9.2 需要搜索确认的其他引用

执行以下搜索命令，确认没有遗漏的引用：

```bash
cd E:\code\lvfeng\library-squirrel

# Go 代码中搜索旧 DTO 名称
grep -r "SiteTagResultDTO" --include="*.go" .
grep -r "SiteAuthorResultDTO" --include="*.go" .
grep -r "LocalTagResultDTO" --include="*.go" .
grep -r "LocalAuthorResultDTO" --include="*.go" .

# 前端代码中搜索旧 DTO 名称
grep -r "SiteTagResultDTO" --include="*.ts" --include="*.vue" frontend/
grep -r "SiteAuthorResultDTO" --include="*.ts" --include="*.vue" frontend/
grep -r "LocalTagResultDTO" --include="*.ts" --include="*.vue" frontend/
grep -r "LocalAuthorResultDTO" --include="*.ts" --include="*.vue" frontend/
```
