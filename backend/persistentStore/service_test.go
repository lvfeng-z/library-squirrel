package persistentStore

import "testing"

// TestUpdateFilePathRejectsOutsideWhitelist UpdateFilePath 拒绝 store/ 白名单外路径
// （防误报的"移动到 backup"被确认同步后 file_path 指向备份目录等非受管文件）
func TestUpdateFilePathRejectsOutsideWhitelist(t *testing.T) {
	s := &Service{}
	for _, p := range []string{
		"backup/2026/08/14/a.mp4", // 备份目录
		"log/server.log",          // 日志目录
		"store/avatar",            // 仅 store/avatar，未注册叶子目录
	} {
		if err := s.UpdateFilePath(t.Context(), 1, p); err == nil {
			t.Errorf("UpdateFilePath(%q) 期望拒绝白名单外路径，实际通过", p)
		}
	}
}

func TestBuildVariantPath(t *testing.T) {
	s := &Service{}
	tests := []struct {
		name   string
		source string
		suffix string
		want   string
	}{
		{"追加合并后缀", "store/resource/作者/视频.mp4", "_merged", "store/resource/作者/视频_merged.mp4"},
		{"空源路径", "", "_merged", ""},
		{"空白源路径", "   ", "_merged", ""},
		{"无扩展名", "store/resource/作者/视频", "_merged", "store/resource/作者/视频_merged"},
		{"文件名非法字符净化", "store/resource/a/视*频.mp4", "_merged", "store/resource/a/视＊频_merged.mp4"},
		{"空 suffix 原样保留", "store/resource/a/v.mp4", "", "store/resource/a/v.mp4"},
		{"多级目录保留", "store/resource/a/b/c.mp4", "_x", "store/resource/a/b/c_x.mp4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.BuildVariantPath(tt.source, tt.suffix); got != tt.want {
				t.Errorf("BuildVariantPath(%q, %q) = %q, want %q", tt.source, tt.suffix, got, tt.want)
			}
		})
	}
}
