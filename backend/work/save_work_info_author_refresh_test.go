package work

// 作品入库后站点作者信息刷新调度的触发锚定：saveWorkInfoInTx 在 upsert 站点作者后通知注入的
// SiteAuthorRefreshScheduler，只传作者 DB ID 集合（含跨站引用作者——实现侧按行 JOIN site
// 反查各自站点键）；未注入调度时入库链零行为变化。

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	entity2 "github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// recordingScheduler 记录 OnSiteAuthorsUpserted 通知的调度替身
type recordingScheduler struct {
	mu    sync.Mutex
	calls [][]int64
}

func (r *recordingScheduler) OnSiteAuthorsUpserted(ids []int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]int64(nil), ids...))
}

func (r *recordingScheduler) snapshot() [][]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]int64, len(r.calls))
	copy(out, r.calls)
	return out
}

// TestSaveWorkInfoNotifiesSiteAuthorRefreshScheduler 入库链 upsert 站点作者后触发一次刷新调度，
// 通知集=upsert 返回的 DB ID 集（本站新建 + 跨站引用既有行均在集内）
func TestSaveWorkInfoNotifiesSiteAuthorRefreshScheduler(t *testing.T) {
	svc, db := newCrossSiteTestEnv(t)
	pixiv := seedSite(t, db, "pixiv")
	bilibili := seedSite(t, db, "bilibili")
	// 预建跨站引用目标（bilibili 站作者行），作品声明的跨站引用解析到该行
	cross := entity2.NewSiteAuthor()
	cross.SiteID = sql.NullInt64{Int64: bilibili.GetID(), Valid: true}
	cross.SiteAuthorID = sql.NullString{String: "bili-x1", Valid: true}
	if err := db.Create(cross).Error; err != nil {
		t.Fatalf("插跨站作者种子失败: %v", err)
	}

	scheduler := &recordingScheduler{}
	svc.SetSiteAuthorRefreshScheduler(scheduler)

	siteWorkId := "refresh-notify-work"
	workResp := &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{SiteWorkId: &siteWorkId, SiteWorkName: &siteWorkId},
		SiteAuthors: []*sdkdto.TaskSiteAuthorDTO{
			{SiteAuthorId: "px-a1", AuthorName: "本站作者"},
			{SiteAuthorId: "bili-x1", AuthorName: "跨站引用作者", SiteKey: "bilibili"},
		},
	}
	task, wt := newWorkTaskOfSite(pixiv.GetID())
	if _, err := svc.saveWorkInfoInTx(context.Background(), task, wt, workResp); err != nil {
		t.Fatalf("入库失败: %v", err)
	}

	calls := scheduler.snapshot()
	if len(calls) != 1 {
		t.Fatalf("应触发一次刷新调度, 实际 %d 次", len(calls))
	}
	ids := calls[0]
	if len(ids) != 2 {
		t.Fatalf("通知应携带 2 个作者 DB ID, 实际 %v", ids)
	}
	crossHit := false
	for _, id := range ids {
		var n int64
		if err := db.Model(&entity2.SiteAuthor{}).Where("id = ?", id).Count(&n).Error; err != nil || n != 1 {
			t.Fatalf("通知集中的 id=%d 应为库内真实作者行 (n=%d, err=%v)", id, n, err)
		}
		if id == cross.GetID() {
			crossHit = true
		}
	}
	if !crossHit {
		t.Fatalf("通知集应含跨站引用作者 DB id %d, 实际 %v", cross.GetID(), ids)
	}
}
