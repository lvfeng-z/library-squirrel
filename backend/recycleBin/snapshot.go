package recycleBin

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// WorkSnapshot 作品快照（work 表业务字段）
type WorkSnapshot struct {
	SiteID              sql.NullInt64  `json:"siteId"`
	SiteWorkID          sql.NullString `json:"siteWorkId"`
	SiteWorkName        sql.NullString `json:"siteWorkName"`
	SiteAuthorID        sql.NullString `json:"siteAuthorId"`
	SiteWorkDescription sql.NullString `json:"siteWorkDescription"`
	SiteUploadTime      sql.NullInt64  `json:"siteUploadTime"`
	SiteUpdateTime      sql.NullInt64  `json:"siteUpdateTime"`
	NickName            sql.NullString `json:"nickName"`
	LocalAuthorID       sql.NullInt64  `json:"localAuthorId"`
	LastView            sql.NullInt64  `json:"lastView"`
	CreateTime          sql.NullInt64  `json:"createTime"` // 作品入库时间（work.create_time）；旧格式快照无此字段，反序列化为 NULL
}

// AuthorSnapshot 作者关联快照
type AuthorSnapshot struct {
	AuthorType    sql.NullInt64  `json:"authorType"`
	LocalAuthorID sql.NullInt64  `json:"localAuthorId"`
	SiteAuthorID  sql.NullInt64  `json:"siteAuthorId"`
	RoleName      sql.NullString `json:"roleName"`
	SortOrder     sql.NullInt64  `json:"sortOrder"`
}

// TagSnapshot 标签关联快照
type TagSnapshot struct {
	TagType    sql.NullInt64 `json:"tagType"`
	LocalTagID sql.NullInt64 `json:"localTagId"`
	SiteTagID  sql.NullInt64 `json:"siteTagId"`
}

// WorkSetSnapshot 作品集关联快照
type WorkSetSnapshot struct {
	WorkSetID     sql.NullInt64 `json:"workSetId"`
	IsCover       sql.NullBool  `json:"isCover"`
	SortOrder     sql.NullInt64 `json:"sortOrder"`
	SiteSortOrder sql.NullInt64 `json:"siteSortOrder"`
}

// StoreBackupRef 单个 store 的备份引用(v1 快照格式)
type StoreBackupRef struct {
	StoreType string `json:"storeType"` // main/thumbnail/videoTrack/...
	BackupID  int64  `json:"backupId"`  // Backup 记录 ID(0=删除时无文件)
}

// ResourceSnapshot 资源快照（业务字段 + Backup 记录 ID 映射）
// v0(旧): WorkStoreBackupID/ThumbnailStoreBackupID 两固定字段
// v1(新): StoreBackups 多轨集合
// 两种格式都保留 JSON tag,通过 SnapshotStoreBackups 适配器统一访问
type ResourceSnapshot struct {
	TaskID                 int64            `json:"taskId"`
	Enabled                bool             `json:"enabled"`
	SuggestName            sql.NullString   `json:"suggestName"`
	ResourceComplete       int              `json:"resourceComplete"`
	StoreBackups           []StoreBackupRef `json:"storeBackups,omitempty"`           // v1: 多轨备份引用
	WorkStoreBackupID      int64            `json:"workStoreBackupId,omitempty"`      // v0 兼容: 主资源 Backup ID
	ThumbnailStoreBackupID int64            `json:"thumbnailStoreBackupId,omitempty"` // v0 兼容: 缩略图 Backup ID
}

// SnapshotStoreBackups 从 ResourceSnapshot 提取统一的 StoreBackupRef 列表。
// v1(StoreBackups 非空)直接返回;v0(旧两字段)映射为 main/thumbnail 两条。
func SnapshotStoreBackups(rs *ResourceSnapshot) []StoreBackupRef {
	if rs == nil {
		return nil
	}
	if len(rs.StoreBackups) > 0 {
		return rs.StoreBackups
	}
	// v0 兼容:从旧两字段映射
	var refs []StoreBackupRef
	if rs.WorkStoreBackupID != 0 {
		refs = append(refs, StoreBackupRef{StoreType: "main", BackupID: rs.WorkStoreBackupID})
	}
	if rs.ThumbnailStoreBackupID != 0 {
		refs = append(refs, StoreBackupRef{StoreType: "thumbnail", BackupID: rs.ThumbnailStoreBackupID})
	}
	return refs
}

// WorkRecycleSnapshot 作品回收站完整快照
type WorkRecycleSnapshot struct {
	Work      WorkSnapshot       `json:"work"`
	Authors   []AuthorSnapshot   `json:"authors"`
	Tags      []TagSnapshot      `json:"tags"`
	WorkSets  []WorkSetSnapshot  `json:"workSets"`
	Resources []ResourceSnapshot `json:"resources"`
}

// MarshalSnapshot 序列化快照为 JSON 字符串
func MarshalSnapshot(s *WorkRecycleSnapshot) (string, error) {
	if s == nil {
		return "", nil
	}
	data, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("序列化作品回收快照失败: %w", err)
	}
	return string(data), nil
}

// UnmarshalSnapshot 反序列化 JSON 字符串为快照
func UnmarshalSnapshot(data string) (*WorkRecycleSnapshot, error) {
	if data == "" {
		return &WorkRecycleSnapshot{}, nil
	}
	var s WorkRecycleSnapshot
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		return nil, fmt.Errorf("反序列化作品回收快照失败: %w", err)
	}
	return &s, nil
}
