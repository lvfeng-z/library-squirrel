package plugin

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	domain "github.com/library-squirrel/backend/base/model/dto"
	entity "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/config"
)

// officialFiles 官方判定测试的基准文件集（多目录层级、二进制内容混合）
var officialFiles = map[string]string{
	"plugin.json":     `{"id":"com.example.official","name":"官方测试插件","version":"1.0.0","buildId":"b1"}`,
	"assets/icon.png": "\x89PNG\r\n\x1a\nbinary",
	"views/btn.js":    "export default function(){return 1}",
}

// tamperedFiles 与基准集仅 plugin.json 内容不同（照抄 buildId、内容被改的伪造形态）
var tamperedFiles = map[string]string{
	"plugin.json":     `{"id":"com.example.official","name":"官方测试插件","version":"1.0.0","buildId":"b1","injected":true}`,
	"assets/icon.png": "\x89PNG\r\n\x1a\nbinary",
	"views/btn.js":    "export default function(){return 1}",
}

// independentDigest 测试内独立实现的内容摘要聚合（与生产算法同规约、独立编码）：
// 文件 sha256 → 路径字典序 → 逐条 "{path}\n{sha256hex}\n" 拼接 → 全文再 sha256
func independentDigest(t *testing.T, files map[string]string) string {
	t.Helper()
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var sb strings.Builder
	for _, p := range paths {
		sum := sha256.Sum256([]byte(files[p]))
		sb.WriteString(p)
		sb.WriteByte('\n')
		sb.WriteString(hex.EncodeToString(sum[:]))
		sb.WriteByte('\n')
	}
	total := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(total[:])
}

// writeZipFromFiles 造 zip 包（alternateMeta 为真时用 CreateHeader 写不同 mtime 与条目顺序，
// 用于锚定容器元数据不参与摘要）
func writeZipFromFiles(t *testing.T, files map[string]string, alternateMeta bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pkg.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("造包文件失败: %v", err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	if alternateMeta {
		sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	} else {
		sort.Strings(paths)
	}
	for _, name := range paths {
		if alternateMeta {
			fh := zip.FileHeader{Name: name, Modified: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
			entry, err := w.CreateHeader(&fh)
			if err != nil {
				t.Fatalf("造包条目失败: %v", err)
			}
			if _, err := entry.Write([]byte(files[name])); err != nil {
				t.Fatalf("写包条目失败: %v", err)
			}
		} else {
			entry, err := w.Create(name)
			if err != nil {
				t.Fatalf("造包条目失败: %v", err)
			}
			if _, err := entry.Write([]byte(files[name])); err != nil {
				t.Fatalf("写包条目失败: %v", err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("闭合包失败: %v", err)
	}
	return path
}

// writeDirFromFiles 造已解压目录树（相对路径正斜杠分隔），返回根目录绝对路径
func writeDirFromFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("建目录失败: %v", err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("写文件失败: %v", err)
		}
	}
	return root
}

// TestContentDigestZipMatchesIndependentComputation zip 源摘要与独立聚合重算一致（算法锚定）
func TestContentDigestZipMatchesIndependentComputation(t *testing.T) {
	digest, err := contentDigestZip(writeZipFromFiles(t, officialFiles, false))
	if err != nil {
		t.Fatalf("计算 zip 摘要失败: %v", err)
	}
	if want := independentDigest(t, officialFiles); digest != want {
		t.Fatalf("zip 源摘要 %s 与独立重算 %s 不一致", digest, want)
	}
}

// TestContentDigestDirMatchesZipSource 目录源与 zip 源对同内容产出同一摘要（双源一致）
func TestContentDigestDirMatchesZipSource(t *testing.T) {
	zipDigest, err := contentDigestZip(writeZipFromFiles(t, officialFiles, false))
	if err != nil {
		t.Fatalf("计算 zip 摘要失败: %v", err)
	}
	dirDigest, err := contentDigestDir(writeDirFromFiles(t, officialFiles))
	if err != nil {
		t.Fatalf("计算目录摘要失败: %v", err)
	}
	if zipDigest != dirDigest {
		t.Fatalf("双源摘要不一致: zip=%s dir=%s", zipDigest, dirDigest)
	}
}

// TestContentDigestIgnoresZipContainerMetadata 容器元数据排除：同内容不同 mtime/条目顺序重打包，摘要不变
func TestContentDigestIgnoresZipContainerMetadata(t *testing.T) {
	plain, err := contentDigestZip(writeZipFromFiles(t, officialFiles, false))
	if err != nil {
		t.Fatalf("计算基准摘要失败: %v", err)
	}
	repacked, err := contentDigestZip(writeZipFromFiles(t, officialFiles, true))
	if err != nil {
		t.Fatalf("计算重打包摘要失败: %v", err)
	}
	if plain != repacked {
		t.Fatalf("同内容重打包摘要应不变: %s != %s", plain, repacked)
	}
}

// TestMatchOfficialZipBranches 名单判定六分支：命中 / buildId 短路 / 照抄 buildId 内容不同 /
// publicId 无条目 / 空名单 / 包读取失败保守 false
func TestMatchOfficialZipBranches(t *testing.T) {
	const pid = "com.example.official"
	roster := []config.OfficialPluginEntry{{
		PublicID:      pid,
		BuildID:       "b1",
		ContentDigest: independentDigest(t, officialFiles),
	}}
	officialZip := writeZipFromFiles(t, officialFiles, false)
	tamperedZip := writeZipFromFiles(t, tamperedFiles, false)

	tests := []struct {
		name     string
		entries  []config.OfficialPluginEntry
		publicId string
		buildId  string
		path     string
		want     bool
	}{
		{"名单命中（同 digest 包）", roster, pid, "b1", officialZip, true},
		{"buildId 短路不命中（包 buildId 不在条目集）", roster, pid, "b2", officialZip, false},
		{"照抄 buildId 内容不同（伪造形态）", roster, pid, "b1", tamperedZip, false},
		{"publicId 无条目", roster, "com.other.plugin", "b1", officialZip, false},
		{"空名单降级全不命中", nil, pid, "b1", officialZip, false},
		{"空切片名单降级", []config.OfficialPluginEntry{}, pid, "b1", officialZip, false},
		{"包 buildId 空（未打标包）", roster, pid, "", officialZip, false},
		{"包路径不存在（摘要失败保守 false）", roster, pid, "b1", filepath.Join(t.TempDir(), "absent.zip"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchOfficialZip(tt.entries, tt.publicId, tt.buildId, tt.path); got != tt.want {
				t.Errorf("matchOfficialZip = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMatchOfficialNilConfig config 未加载（全局配置为 nil）时官方判定保守 false 不 panic
func TestMatchOfficialNilConfig(t *testing.T) {
	svc := NewService(nil, nil)
	got := svc.matchOfficial(&domain.PluginInstallDTO{
		PublicID:    "com.example.official",
		BuildID:     "b1",
		PackagePath: writeZipFromFiles(t, officialFiles, false),
	})
	if !got.Valid || got.Bool {
		t.Fatalf("config 未加载应判未证实（Valid=true, Bool=false）, got %+v", got)
	}
}

// plantLocalRow 预置 local 来源已装行（存量手动安装形态），返回行
func plantLocalRow(t *testing.T, svc *Service, publicId, buildId string) *entity.Plugin {
	t.Helper()
	row := entity.NewPlugin()
	row.PublicID = ns(publicId)
	row.Name = ns("存量插件")
	row.Version = ns("1.0.0")
	row.BuildID = ns(buildId)
	row.Source = ns(SourceLocal)
	row.Uninstalled = sql.NullBool{Bool: false, Valid: true}
	if err := svc.repo.Create(context.Background(), row); err != nil {
		t.Fatalf("预置插件行失败: %v", err)
	}
	return row
}

// TestVerifyExistingOfficialHit 存量证实命中：local 行 buildId 命中名单且已装目录摘要一致 → official=true 落库
func TestVerifyExistingOfficialHit(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	ctx := context.Background()
	const pid = "com.example.stale"
	row := plantLocalRow(t, svc, pid, "b1")
	dir := writeDirFromFiles(t, officialFiles)
	roster := []config.OfficialPluginEntry{{
		PublicID:      pid,
		BuildID:       "b1",
		ContentDigest: independentDigest(t, officialFiles),
	}}

	svc.verifyExistingOfficial(ctx, roster, row, dir)

	fresh, err := svc.repo.GetByPublicId(ctx, pid)
	if err != nil || fresh == nil {
		t.Fatalf("回查行失败: %v, %v", err, fresh)
	}
	if !fresh.Official.Valid || !fresh.Official.Bool {
		t.Fatalf("命中后行须 official=true, got %+v", fresh.Official)
	}
	if !fresh.Source.Valid || fresh.Source.String != SourceLocal {
		t.Fatalf("证实不应改写渠道字段, got %+v", fresh.Source)
	}
}

// TestVerifyExistingOfficialBuildIdMismatch 存量证实的 buildId 门槛：行 buildId 与名单条目不等 → 不动
func TestVerifyExistingOfficialBuildIdMismatch(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	ctx := context.Background()
	const pid = "com.example.stale"
	row := plantLocalRow(t, svc, pid, "b0")
	dir := writeDirFromFiles(t, officialFiles)
	roster := []config.OfficialPluginEntry{{
		PublicID:      pid,
		BuildID:       "b1",
		ContentDigest: independentDigest(t, officialFiles),
	}}

	svc.verifyExistingOfficial(ctx, roster, row, dir)

	fresh, _ := svc.repo.GetByPublicId(ctx, pid)
	if fresh.Official.Valid {
		t.Fatalf("buildId 不等不应证实, got %+v", fresh.Official)
	}
}

// TestVerifyExistingOfficialIdempotent 已 official=true 的行短路（不再触 Updates）
func TestVerifyExistingOfficialIdempotent(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	ctx := context.Background()
	const pid = "com.example.done"
	row := plantLocalRow(t, svc, pid, "b1")
	row.Official = sql.NullBool{Bool: true, Valid: true}
	if err := svc.repo.Updates(ctx, row); err != nil {
		t.Fatalf("预置 official=true 失败: %v", err)
	}
	counter := &updatesCountRepo{Repository: svc.repo}
	svc.repo = counter
	defer func() { svc.repo = counter.Repository }()

	svc.verifyExistingOfficial(ctx, nil, row, writeDirFromFiles(t, officialFiles))

	if counter.updates != 0 {
		t.Fatalf("已 true 行应短路不触 Updates, got %d 次", counter.updates)
	}
}

// updatesCountRepo 仓储装饰器：全方法委托真实仓储，Updates 计数
type updatesCountRepo struct {
	Repository
	updates int
}

func (r *updatesCountRepo) Updates(ctx context.Context, plugin *entity.Plugin) error {
	r.updates++
	return r.Repository.Updates(ctx, plugin)
}
