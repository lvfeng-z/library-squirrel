package siteTag

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// TestUpdateByIdClearsNullableColumns 编辑链置 NULL 锚定：用户编辑清空可空列（解绑本地标签、
// 清空详情）时，提交面携带 sql.Null*{Valid:false}，UpdateById 须把对应列真正写为 NULL。
// 历史缺陷：UpdateById 走 GORM struct 部分更新，Valid:false 被当零值跳过，置 NULL 被静默丢弃
// （解绑从未生效过），故此处同时锚定「写入 NULL 生效」与「未提交列不被清」两个方向
func TestUpdateByIdClearsNullableColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	svc, db := newDeleteTestEnv(t)
	ctx := context.Background()

	// 外键父行种子：site（site_tag.site_id 引用）+ local_tag（site_tag.local_tag_id 引用）
	if err := db.Exec("INSERT INTO site (id, site_key, create_time, update_time) VALUES (1, ?, 0, 0)", identity.Local.Key).Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
	baseTag := domain.NewLocalTag()
	baseTag.LocalTagName = sql.NullString{String: "本地标签种子", Valid: true}
	if err := db.Create(baseTag).Error; err != nil {
		t.Fatalf("插 local_tag 种子失败: %v", err)
	}

	seed := domain.NewSiteTag()
	seed.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	seed.SiteTagID = sql.NullString{String: "st-clear", Valid: true}
	seed.SiteTagName = sql.NullString{String: "原标签名", Valid: true}
	seed.Description = sql.NullString{String: "原详情", Valid: true}
	seed.LocalTagID = sql.NullInt64{Int64: baseTag.GetID(), Valid: true}
	if err := db.Create(seed).Error; err != nil {
		t.Fatalf("插 site_tag 种子失败: %v", err)
	}

	// 编辑保存载荷（与前端行内编辑/编辑对话框同构）：清空详情、解绑本地标签，保留标签名；
	// DTO 指针 nil 经 ToSiteTagEntity 映射为 Valid:false
	keptName := "新标签名"
	tagDTO := &dto.SiteTagDTO{
		ID:          seed.GetID(),
		SiteTagName: &keptName,
		SiteID:      nil,
		Description: nil,
		LocalTagID:  nil,
	}
	if err := svc.UpdateById(ctx, dto.ToSiteTagEntity(tagDTO)); err != nil {
		t.Fatalf("更新站点标签失败: %v", err)
	}

	var row domain.SiteTag
	if err := db.First(&row, seed.GetID()).Error; err != nil {
		t.Fatalf("读回 site_tag 失败: %v", err)
	}
	if row.LocalTagID.Valid {
		t.Fatalf("解绑未生效：local_tag_id 仍为 %d", row.LocalTagID.Int64)
	}
	if row.Description.Valid && row.Description.String != "" {
		t.Fatalf("清空详情未生效：description 仍为 %q", row.Description.String)
	}
	if !row.SiteTagName.Valid || row.SiteTagName.String != keptName {
		t.Fatalf("标签名应更新为提交值 %q，实际 %v", keptName, row.SiteTagName)
	}
	// 未在编辑面的列不受影响：站点侧标签 ID（身份键）保持原值
	if !row.SiteTagID.Valid || row.SiteTagID.String != "st-clear" {
		t.Fatalf("未提交的身份键列被改动：site_tag_id = %v", row.SiteTagID)
	}
}
