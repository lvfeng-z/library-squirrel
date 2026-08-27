package share

// 分享记录（share_record）：分享方持久化账本。运行态（connecting/online/reconnecting）
// 不入表——运行期由进程内会话注册表 + state 事件实时推；本表记录首次在线起的分享参数
// 与终态，并承载启动自动复原（active 行原 token bind 重绑复活，链接不变）。

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/library-squirrel/backend/base/model/entity"
)

// 记录状态（active 可复原；其余为终态不可逆）
const (
	// RecordStateActive 在线/可复原（启动时自动 bind 重绑）
	RecordStateActive = "active"
	// RecordStateRevoked 已撤销（终态）
	RecordStateRevoked = "revoked"
	// RecordStateExpired 已过期（终态）
	RecordStateExpired = "expired"
	// RecordStateFailed 失败终态（复原失败等；err_msg 记原因）
	RecordStateFailed = "failed"
)

// ShareRecordDTO 分享记录行（历史分享账本查询 DTO）。链接由 relayAddress+token+key
// 重建（与发布时完全一致——复原不换 token/密钥）；密钥/令牌缺失的异常行链接为空。
type ShareRecordDTO struct {
	ID                int64   `json:"id"`
	ShareID           string  `json:"shareId"`
	Token             string  `json:"token"`
	Title             string  `json:"title"`
	WorkIDs           []int64 `json:"workIds"`
	WorkSetIDs        []int64 `json:"workSetIds"`
	RelayAddress      string  `json:"relayAddress"`
	KeyB64            string  `json:"keyB64"`
	Link              string  `json:"link"` // 重建的完整分享链接（含 fragment 密钥）
	PasswordProtected bool    `json:"passwordProtected"`
	ExpireSeconds     int64   `json:"expireSeconds"`
	ExpiresAt         int64   `json:"expiresAt"`
	FileCount         int64   `json:"fileCount"`
	TotalBytes        int64   `json:"totalBytes"`
	MissingFiles      int64   `json:"missingFiles"`
	State             string  `json:"state"`
	ErrMsg            string  `json:"errMsg"`
	RevokedAt         int64   `json:"revokedAt"`
	CreatedAt         int64   `json:"createdAt"`
	UpdatedAt         int64   `json:"updatedAt"`
}

// toShareRecordDTO 实体 → DTO（含分享链接重建）
func toShareRecordDTO(rec *entity.ShareRecord) *ShareRecordDTO {
	workIDs, _ := unmarshalInt64s(rec.WorkIDs)
	workSetIDs, _ := unmarshalInt64s(rec.WorkSetIDs)
	dto := &ShareRecordDTO{
		ID:                rec.GetID(),
		ShareID:           rec.ShareID,
		Token:             rec.Token,
		Title:             rec.Title,
		WorkIDs:           workIDs,
		WorkSetIDs:        workSetIDs,
		RelayAddress:      rec.RelayAddress,
		KeyB64:            rec.KeyB64,
		PasswordProtected: rec.PasswordProtected,
		ExpireSeconds:     rec.ExpireSeconds,
		ExpiresAt:         rec.ExpiresAt,
		FileCount:         rec.FileCount,
		TotalBytes:        rec.TotalBytes,
		MissingFiles:      rec.MissingFiles,
		State:             rec.State,
		ErrMsg:            rec.ErrMsg,
		RevokedAt:         rec.RevokedAt,
		CreatedAt:         rec.GetCreateTime(),
		UpdatedAt:         rec.GetUpdateTime(),
	}
	if rec.Token != "" && rec.KeyB64 != "" {
		if key, err := base64.RawURLEncoding.DecodeString(rec.KeyB64); err == nil {
			dto.Link = BuildShareLink(rec.RelayAddress, rec.Token, key)
		}
	}
	return dto
}

// marshalInt64s ID 集序列化为 JSON 数组（nil 归一为空数组）
func marshalInt64s(ids []int64) string {
	if ids == nil {
		ids = []int64{}
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

// unmarshalInt64s 反序列化 ID 集（空串返回 nil 非错误）
func unmarshalInt64s(s string) ([]int64, error) {
	if s == "" {
		return nil, nil
	}
	var ids []int64
	if err := json.Unmarshal([]byte(s), &ids); err != nil {
		return nil, fmt.Errorf("ID 清单不可解析: %w", err)
	}
	return ids, nil
}
