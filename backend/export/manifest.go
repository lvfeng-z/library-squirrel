package export

import (
	"encoding/json"
)

// SchemaVersion manifest 契约版本（回灌硬约束的版本锚）。
// 版本纪律对齐插件 plugin_data 的 schemaVersion（见 doc/plugin-dev-guide.md「plugin_data 格式版本约定」）：
// 仅在结构破坏性变更（删字段/改字段语义/改类型）时递增；加可选字段不必递增（向前兼容）。
const SchemaVersion = 1

// Manifest 导出产物清单（方案第3节契约）。导出格式一经上线即成为既有产物，
// 不可回灌的导出等于半成品——字段增删必须受 SchemaVersion 约束。
type Manifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	Meta          Meta            `json:"meta"`
	Sites         []SiteRecord    `json:"sites"`
	LocalAuthors  []AuthorRecord  `json:"localAuthors"`
	SiteAuthors   []AuthorRecord  `json:"siteAuthors"`
	LocalTags     []TagRecord     `json:"localTags"`
	SiteTags      []TagRecord     `json:"siteTags"`
	WorkSets      []WorkSetRecord `json:"workSets"`
	Works         []WorkRecord    `json:"works"`
	Files         []FileEntry     `json:"files"`
}

// Meta 导出元信息：导出时间、来源 app 版本、各域计数。
type Meta struct {
	ExportedAt       int64  `json:"exportedAt"` // 导出时间（毫秒时间戳）
	AppVersion       string `json:"appVersion"` // 来源 app 版本
	SiteCount        int    `json:"siteCount"`
	LocalAuthorCount int    `json:"localAuthorCount"`
	SiteAuthorCount  int    `json:"siteAuthorCount"`
	LocalTagCount    int    `json:"localTagCount"`
	SiteTagCount     int    `json:"siteTagCount"`
	WorkSetCount     int    `json:"workSetCount"`
	WorkCount        int    `json:"workCount"`
	FileCount        int    `json:"fileCount"`
}

// SiteRecord 站点（site 表全字段）。
type SiteRecord struct {
	ID              int64   `json:"id"`
	SiteName        *string `json:"siteName,omitempty"`
	SiteDescription *string `json:"siteDescription,omitempty"`
	Homepage        *string `json:"homepage,omitempty"`
	CreateTime      int64   `json:"createTime"`
	UpdateTime      int64   `json:"updateTime"`
}

// AuthorRecord 作者（local_author / site_author 两表并型；站点专属字段对本地作者省略）。
type AuthorRecord struct {
	ID   int64   `json:"id"`
	Name *string `json:"name,omitempty"`

	// 站点作者专属
	SiteID               *int64  `json:"siteId,omitempty"`
	SiteAuthorID         *string `json:"siteAuthorId,omitempty"`
	FixedAuthorName      *string `json:"fixedAuthorName,omitempty"`
	SiteAuthorNameBefore *string `json:"siteAuthorNameBefore,omitempty"`
	Homepage             *string `json:"homepage,omitempty"`
	LocalAuthorID        *int64  `json:"localAuthorId,omitempty"` // site→local 桥接

	// 本地作者专属
	Introduce *string `json:"introduce,omitempty"`

	LastUse    *int64 `json:"lastUse,omitempty"`
	CreateTime int64  `json:"createTime"`
	UpdateTime int64  `json:"updateTime"`
}

// TagRecord 标签（local_tag / site_tag 两表并型；站点专属字段对本地标签省略）。
type TagRecord struct {
	ID   int64   `json:"id"`
	Name *string `json:"name,omitempty"`

	// 站点标签专属
	SiteID        *int64  `json:"siteId,omitempty"`
	SiteTagID     *string `json:"siteTagId,omitempty"`
	BaseSiteTagID *string `json:"baseSiteTagId,omitempty"`
	Namespace     *string `json:"namespace,omitempty"`  // 站点侧 namespace（language/character/...；null=站点无 namespace）
	LocalTagID    *int64  `json:"localTagId,omitempty"` // site→local 桥接

	// 本地标签专属
	BaseLocalTagID *int64 `json:"baseLocalTagId,omitempty"` // 父标签引用（标签树，null=根）

	Description *string `json:"description,omitempty"`
	LastUse     *int64  `json:"lastUse,omitempty"`
	CreateTime  int64   `json:"createTime"`
	UpdateTime  int64   `json:"updateTime"`
}

// WorkSetRecord 作品集（work_set 表全字段 + 层级关系）。
type WorkSetRecord struct {
	ID int64 `json:"id"`

	SiteID                 *int64  `json:"siteId,omitempty"`
	SiteWorkSetID          *string `json:"siteWorkSetId,omitempty"`
	SiteWorkSetName        *string `json:"siteWorkSetName,omitempty"`
	SiteAuthorID           *string `json:"siteAuthorId,omitempty"`
	SiteWorkSetDescription *string `json:"siteWorkSetDescription,omitempty"`
	SiteUploadTime         *int64  `json:"siteUploadTime,omitempty"`
	SiteUpdateTime         *int64  `json:"siteUpdateTime,omitempty"`
	NickName               *string `json:"nickName,omitempty"`
	Description            *string `json:"description,omitempty"`
	LastView               *int64  `json:"lastView,omitempty"`
	CoverWorkID            *int64  `json:"coverWorkId,omitempty"` // 封面作品引用（可指向传递包含内任意作品）

	CreateTime int64 `json:"createTime"`
	UpdateTime int64 `json:"updateTime"`

	// Parents 父作品集引用（re_work_set_work_set 边，多父 DAG；仅保留两端均在导出闭包内的边）
	Parents []WorkSetParentEdge `json:"parents,omitempty"`
}

// WorkSetParentEdge 作品集间父子边（re_work_set_work_set 一行）。
type WorkSetParentEdge struct {
	ParentWorkSetID int64  `json:"parentWorkSetId"`
	SortOrder       *int64 `json:"sortOrder,omitempty"`
	SiteSortOrder   *int64 `json:"siteSortOrder,omitempty"`
}

// WorkRecord 作品（work 表全字段 + 资源挂载 + 标签/作者/作品集关联）。
type WorkRecord struct {
	ID int64 `json:"id"`

	SiteID              *int64  `json:"siteId,omitempty"`
	SiteWorkID          *string `json:"siteWorkId,omitempty"`
	SiteWorkName        *string `json:"siteWorkName,omitempty"`
	SiteAuthorID        *string `json:"siteAuthorId,omitempty"`
	SiteWorkDescription *string `json:"siteWorkDescription,omitempty"`
	SiteUploadTime      *int64  `json:"siteUploadTime,omitempty"`
	SiteUpdateTime      *int64  `json:"siteUpdateTime,omitempty"`
	NickName            *string `json:"nickName,omitempty"`
	LocalAuthorID       *int64  `json:"localAuthorId,omitempty"`
	LastView            *int64  `json:"lastView,omitempty"`
	CreateTime          int64   `json:"createTime"`
	UpdateTime          int64   `json:"updateTime"`

	Resources    []ResourceRecord `json:"resources,omitempty"`
	TagLinks     []TagLink        `json:"tagLinks,omitempty"`     // re_work_tag 关联（含 namespace）
	AuthorLinks  []AuthorLink     `json:"authorLinks,omitempty"`  // re_work_author 关联
	WorkSetLinks []WorkSetLink    `json:"workSetLinks,omitempty"` // re_work_work_set 成员关系（仅跨选择项保留）
}

// ResourceRecord 作品下的资源（resource 表字段 + resource_store 活行挂载）。
type ResourceRecord struct {
	ID               int64        `json:"id"`
	TaskID           *int64       `json:"taskId,omitempty"` // 产生本资源的任务（溯源用，null=非任务产）
	SuggestName      *string      `json:"suggestName,omitempty"`
	ResourceComplete *int64       `json:"resourceComplete,omitempty"`
	ResourceType     string       `json:"resourceType"`
	CreateTime       int64        `json:"createTime"`
	UpdateTime       int64        `json:"updateTime"`
	Stores           []StoreMount `json:"stores,omitempty"` // resource_store 活行挂载
}

// StoreMount 资源-文件挂载（resource_store 活行；按 STORE_ASSOCIATION_LIVENESS_FILTER 只取活行）。
type StoreMount struct {
	StoreType  string `json:"storeType"`  // image | document | thumbnail | videoTrack | audioTrack | videoMain
	Generation string `json:"generation"` // downloaded | derived
	StoreSeq   int    `json:"storeSeq"`
	StoreID    int64  `json:"storeId"` // 引用 files[] 中 StoreID 匹配的文件条目
}

// TagLink 作品-标签关联（re_work_tag 一行；namespace 为关联级维度）。
type TagLink struct {
	TagType   int     `json:"tagType"` // 0=local（constant.LOCAL）/ 1=site（constant.SITE），按 tag_id 列非空判定
	TagID     int64   `json:"tagId"`   // local_tag.id 或 site_tag.id（引用顶层 LocalTags/SiteTags）
	Namespace *string `json:"namespace,omitempty"`
}

// AuthorLink 作品-作者关联（re_work_author 一行）。
type AuthorLink struct {
	AuthorType int     `json:"authorType"` // 0=local / 1=site
	AuthorID   int64   `json:"authorId"`   // local_author.id 或 site_author.id
	RoleName   *string `json:"roleName,omitempty"`
	SortOrder  *int64  `json:"sortOrder,omitempty"`
}

// WorkSetLink 作品-作品集成员关系（re_work_work_set 一行；仅保留作品与作品集均在导出闭包内的边）。
type WorkSetLink struct {
	WorkSetID     int64  `json:"workSetId"`
	SortOrder     *int64 `json:"sortOrder,omitempty"`
	SiteSortOrder *int64 `json:"siteSortOrder,omitempty"`
}

// FileEntry 文件条目（files[]；被 work 的 store 挂载按 StoreID 引用）。
// 阶段2 数据面：按活行关联纳入全部源文件条目，Path/Size/Sha256/Missing 由阶段3 打包时填充
// （决策4：源文件缺失 → Missing=true，该 store 缺席、其余照常）。
type FileEntry struct {
	// StoreID 源 persistent_store 行 ID（导出模型内锚：store 挂载按此引用文件条目）
	StoreID int64 `json:"storeId"`
	// StorePath 源文件 workDir 相对路径（正斜杠；relPath 域）——打包阶段读取源文件、导入阶段落盘参照
	StorePath string `json:"storePath"`
	// Path 包内相对路径（按方案第3节确定性规则；阶段3 namer 填充）
	Path string `json:"path"`
	Size int64  `json:"size"`
	// Sha256 文件内容 SHA256（回灌校验用；阶段3 填充）
	Sha256 string `json:"sha256,omitempty"`
	// Missing 源文件缺失标记（决策4）：true=源文件不存在，该 store 缺席、其余照常
	Missing bool `json:"missing"`
}

// Serialize 序列化为 JSON（缩进，供产物 manifest.json 与契约测试）。
func (m *Manifest) Serialize() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// Deserialize 从 JSON 反序列化（往返一致校验用；阶段4 回灌入口）。
func Deserialize(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
