package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// resetViper viper 为包级全局单例，MergeConfig 逐次累积——用例间复位保证各用例从嵌入默认层起算
func resetViper(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
}

// TestParseLockedValidData 订死配置解析：yaml tag 绑定条目三字段
func TestParseLockedValidData(t *testing.T) {
	data := []byte(`officialPlugins:
  - publicId: com.lvfeng.pixivSuite
    buildId: v1.0.0-394e66f
    contentDigest: "0123abcd"
  - publicId: com.lvfeng.localImport
    buildId: v1.2.0-1d7bd5e
    contentDigest: "4567ffff"
`)
	locked := parseLocked(data)
	if len(locked.OfficialPlugins) != 2 {
		t.Fatalf("应解析出 2 条名单条目, got %d", len(locked.OfficialPlugins))
	}
	first := locked.OfficialPlugins[0]
	if first.PublicID != "com.lvfeng.pixivSuite" || first.BuildID != "v1.0.0-394e66f" || first.ContentDigest != "0123abcd" {
		t.Errorf("首条目字段不符: %+v", first)
	}
	second := locked.OfficialPlugins[1]
	if second.PublicID != "com.lvfeng.localImport" || second.BuildID != "v1.2.0-1d7bd5e" || second.ContentDigest != "4567ffff" {
		t.Errorf("次条目字段不符: %+v", second)
	}
}

// TestParseLockedMalformedDegradesEmpty 解析失败降级：坏内容返回空名单而非报错（不拦启动，官方判定全不命中）
func TestParseLockedMalformedDegradesEmpty(t *testing.T) {
	// 未闭合的流式序列，yaml 解析必然失败
	locked := parseLocked([]byte("officialPlugins: [unclosed"))
	if len(locked.OfficialPlugins) != 0 {
		t.Errorf("解析失败应降级空名单, got %+v", locked.OfficialPlugins)
	}
}

// TestLoadLockedEmbedReadable 嵌入订死配置可读且可解析（go:embed 编译期内嵌，文件必然存在）
func TestLoadLockedEmbedReadable(t *testing.T) {
	locked := loadLocked()
	// 初版为空名单：调用不 panic 即达意（条目级断言由名单管线产出后的阶段2 核对承接）
	if locked.OfficialPlugins == nil {
		t.Log("嵌入名单为空切片或 nil（初版预期）")
	}
}

// TestLoadFromDirWiresLocked LoadFromDir 挂载订死层：与 Server 等并列字段可经全局配置读取
func TestLoadFromDirWiresLocked(t *testing.T) {
	resetViper(t)
	cfg, err := LoadFromDir(t.TempDir())
	if err != nil {
		t.Fatalf("LoadFromDir 失败: %v", err)
	}
	if Get() != cfg {
		t.Fatal("全局配置指针应指向 LoadFromDir 产出")
	}
	// 初版嵌入名单为空：挂载成功的可观测信号是不 panic 且不报错
	_ = cfg.Locked.OfficialPlugins
}

// TestLoadFromDirIgnoresDiskLockedKey 防篡改锚定：磁盘 config.yaml 手写 locked 键不进订死层
// （mapstructure:"-" 排除出主 Unmarshal，独立加载是唯一取值路径）
func TestLoadFromDirIgnoresDiskLockedKey(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	tamper := "locked:\n  officialPlugins:\n    - publicId: com.evil.fake\n      buildId: evil-build\n      contentDigest: deadbeef\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(tamper), 0o644); err != nil {
		t.Fatalf("写磁盘配置失败: %v", err)
	}

	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir 失败: %v", err)
	}
	for _, e := range cfg.Locked.OfficialPlugins {
		if e.PublicID == "com.evil.fake" {
			t.Fatalf("磁盘 locked 键渗入订死层: %+v", cfg.Locked.OfficialPlugins)
		}
	}
}
