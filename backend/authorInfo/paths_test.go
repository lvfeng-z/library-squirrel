package authorInfo

import (
	"errors"
	"strings"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/storepath"
)

// 期望桶段为独立预计算值（复合键 SHA256 前 2 位 hex），非经被测链路派生——
// 桶段值锚定 SDK 派生规则，被测函数只负责组装。
//
//	pixiv_12345 → 2e；local_42 → db；local_7 → 7a
func TestSiteAvatarRelPath(t *testing.T) {
	got, err := SiteAvatarRelPath("pixiv", "12345", "jpg")
	if err != nil {
		t.Fatalf("派生失败: %v", err)
	}
	if want := "store/avatar/site/2e/pixiv_12345.jpg"; got != want {
		t.Fatalf("站点头像路径 = %q, want %q", got, want)
	}
}

func TestSiteAvatarRelPathExtNormalize(t *testing.T) {
	// 带点形式兼容（不产生双点）；空串=无扩展名
	if got, err := SiteAvatarRelPath("pixiv", "12345", ".png"); err != nil || got != "store/avatar/site/2e/pixiv_12345.png" {
		t.Fatalf("带点 ext 派生 = %q, %v", got, err)
	}
	if got, err := SiteAvatarRelPath("pixiv", "12345", ""); err != nil || got != "store/avatar/site/2e/pixiv_12345" {
		t.Fatalf("空 ext 派生 = %q, %v", got, err)
	}
}

func TestSiteAvatarRelPathSanitizedIdSingleSegment(t *testing.T) {
	// 站点侧 ID 为任意字符串：Windows 非法字符经净化为全角等价字符（净化发生变更时
	// SDK 追加消歧哈希段），文件段恒为单一路径段（不因分隔符产生额外目录层级）
	got, err := SiteAvatarRelPath("pixiv", "a/b:c", "jpg")
	if err != nil {
		t.Fatalf("派生失败: %v", err)
	}
	if strings.ContainsAny(got, `\`) || strings.Count(got, "/") != 4 {
		t.Fatalf("站点头像路径 %q 须为 store/avatar/site/{桶段}/{单段文件} 四层正斜杠形态", got)
	}
	if !strings.Contains(got, "pixiv_a／b：c") || !strings.HasSuffix(got, ".jpg") {
		t.Fatalf("站点头像路径 %q 缺净化文件段或扩展名", got)
	}
}

func TestSiteAvatarRelPathInvalidIdentity(t *testing.T) {
	if _, err := SiteAvatarRelPath("", "12345", "jpg"); !errors.Is(err, storepath.ErrEmptySiteKey) {
		t.Fatalf("空 siteKey 期望 ErrEmptySiteKey，实际: %v", err)
	}
	if _, err := SiteAvatarRelPath("pixiv", "", "jpg"); !errors.Is(err, storepath.ErrEmptySiteWorkId) {
		t.Fatalf("空 siteAuthorId 期望 ErrEmptySiteWorkId，实际: %v", err)
	}
}

func TestLocalAvatarRelPath(t *testing.T) {
	got, err := LocalAvatarRelPath(42, "png")
	if err != nil {
		t.Fatalf("派生失败: %v", err)
	}
	if want := "store/avatar/local/db/local_42.png"; got != want {
		t.Fatalf("本地头像路径 = %q, want %q", got, want)
	}
	if got, err := LocalAvatarRelPath(7, ".jpg"); err != nil || got != "store/avatar/local/7a/local_7.jpg" {
		t.Fatalf("本地头像路径(带点 ext) = %q, %v", got, err)
	}
}

func TestAvatarRelPathDeterministic(t *testing.T) {
	// 同复合键恒同路径（路径锚定不变量——换头像判定与重拉去重依赖）
	a, err := SiteAvatarRelPath("bilibili", "2233", "webp")
	if err != nil {
		t.Fatalf("派生失败: %v", err)
	}
	b, _ := SiteAvatarRelPath("bilibili", "2233", "webp")
	if a != b || !strings.HasPrefix(a, "store/avatar/site/f0/bilibili_2233.webp") {
		t.Fatalf("同键派生不一致或前缀错误: %q vs %q", a, b)
	}
}
