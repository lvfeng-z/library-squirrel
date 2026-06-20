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
	WorkSetID sql.NullInt64 `json:"workSetId"`
	IsCover   sql.NullBool  `json:"isCover"`
	SortOrder sql.NullInt64 `json:"sortOrder"`
}

// ResourceSnapshot 资源快照（业务字段 + Backup 记录 ID 映射）
type ResourceSnapshot struct {
	TaskID                 int64          `json:"taskId"`
	Enabled                bool           `json:"enabled"`
	SuggestName            sql.NullString `json:"suggestName"`
	ResourceComplete       int            `json:"resourceComplete"`
	WorkStoreBackupID      int64          `json:"workStoreBackupId"`      // 主资源 Backup 记录 ID（0 = 删除时无主资源文件）
	ThumbnailStoreBackupID int64          `json:"thumbnailStoreBackupId"` // 缩略图 Backup 记录 ID（0 = 无缩略图）
}

// WorkRecycleSnapshot 作品回收站完整快照
// 保存"无法从 Backup 表还原"的全部关联元数据；资源文件级信息由 Backup 记录承载
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
