package siteAuthor

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// TestUpdateByIdClearsNullableColumns 编辑链置 NULL 锚定：用户编辑清空可空列（取消绑定本地作者、
// 清空介绍/固定名）时，提交面携带 sql.Null*{Valid:false}，UpdateById 须把对应列真正写为 NULL。
// 历史缺陷：UpdateById 走 GORM struct 部分更新，Valid:false 被当零值跳过，置 NULL 被静默丢弃
// （取消绑定从未生效过），故此处同时锚定「写入 NULL 生效」与「未提交列不被清」两个方向
func TestUpdateByIdClearsNullableColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	svc, db := newDeleteTestEnv(t)
	ctx := context.Background()

	// 外键父行种子：site（site_author.site_id 引用）
	if err := db.Exec("INSERT INTO site (id, site_key, create_time, update_time) VALUES (1, ?, 0, 0)", identity.Local.Key).Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
	seed := domain.NewSiteAuthor()
	seed.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	seed.SiteAuthorID = sql.NullString{String: "sa-clear", Valid: true}
	seed.AuthorName = sql.NullString{String: "原名", Valid: true}
	seed.Introduce = sql.NullString{String: "原介绍", Valid: true}
	seed.FixedAuthorName = sql.NullString{String: "原固定名", Valid: true}
	seed.LocalAuthorID = sql.NullInt64{Int64: 4, Valid: true}
	if err := db.Create(seed).Error; err != nil {
		t.Fatalf("插 site_author 种子失败: %v", err)
	}

	// 编辑保存载荷（与前端 saveRowEdit / 编辑对话框同构）：清空介绍、取消绑定、清空固定名，
	// 保留名称；DTO 指针 nil 经 ToSiteAuthorEntity 映射为 Valid:false
	keptName := "新名称"
	authorDTO := &dto.SiteAuthorDTO{
		ID:              seed.GetID(),
		AuthorName:      &keptName,
		SiteID:          nil,
		Introduce:       nil,
		FixedAuthorName: nil,
		LocalAuthorID:   nil,
	}
	if err := svc.UpdateById(ctx, dto.ToSiteAuthorEntity(authorDTO)); err != nil {
		t.Fatalf("更新站点作者失败: %v", err)
	}

	var row domain.SiteAuthor
	if err := db.First(&row, seed.GetID()).Error; err != nil {
		t.Fatalf("读回 site_author 失败: %v", err)
	}
	if row.LocalAuthorID.Valid {
		t.Fatalf("取消绑定未生效：local_author_id 仍为 %d", row.LocalAuthorID.Int64)
	}
	if row.Introduce.Valid && row.Introduce.String != "" {
		t.Fatalf("清空介绍未生效：introduce 仍为 %q", row.Introduce.String)
	}
	if row.FixedAuthorName.Valid && row.FixedAuthorName.String != "" {
		t.Fatalf("清空固定名未生效：fixed_author_name 仍为 %q", row.FixedAuthorName.String)
	}
	if !row.AuthorName.Valid || row.AuthorName.String != keptName {
		t.Fatalf("名称应更新为提交值 %q，实际 %v", keptName, row.AuthorName)
	}
	// 未在编辑面的列不受影响：站点侧作者 ID（身份键）保持原值
	if !row.SiteAuthorID.Valid || row.SiteAuthorID.String != "sa-clear" {
		t.Fatalf("未提交的身份键列被改动：site_author_id = %v", row.SiteAuthorID)
	}
}
