package export

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/util/fingerprint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildPackFixture 构建打包测试夹具：真实源文件 + 一个缺失源文件（决策4 分支）。
func buildPackFixture(t *testing.T) (workDir string, model *ExportModel) {
	t.Helper()
	workDir = t.TempDir()
	writeFile := func(rel string, content []byte) {
		t.Helper()
		abs := filepath.Join(workDir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, content, 0o644))
	}
	writeFile("store/work/a/作品1.jpg", []byte("image-data-1"))
	writeFile("store/work/a/作品1.png", []byte("image-data-2-longer-content"))
	// StoreID 102 指向的源文件缺失：不创建

	model = NewExportModel(&Manifest{
		SchemaVersion: SchemaVersion,
		Meta:          Meta{ExportedAt: 1725000000000, AppVersion: "test"},
		Works: []WorkRecord{
			{ID: 1, SiteWorkName: strp("作品1"),
				Resources: []ResourceRecord{
					{ID: 10, Stores: []StoreMount{
						{StoreType: "image", StoreSeq: 0, StoreID: 100},
						{StoreType: "thumbnail", StoreSeq: 1, StoreID: 101},
					}},
					{ID: 11, Stores: []StoreMount{
						{StoreType: "image", StoreSeq: 0, StoreID: 102},
					}},
				}},
		},
		Files: []FileEntry{
			{StoreID: 100, StorePath: "store/work/a/作品1.jpg"},
			{StoreID: 101, StorePath: "store/work/a/作品1.png"},
			{StoreID: 102, StorePath: "store/work/a/missing.jpg"},
		},
	})
	return workDir, model
}

// packFixture 执行 Plan+Pack 到指定目标，返回 zip 条目与方法。
// 模板传空串=包内文件名回退源文件名命名（本文件锚定打包面行为，命名面由 namer_test 锚定）。
func packFixture(t *testing.T, workDir string, model *ExportModel, target string) []*zip.File {
	t.Helper()
	p := NewPacker()
	stats, err := p.Plan(context.Background(), workDir, model, "")
	require.NoError(t, err)
	require.NoError(t, p.Pack(context.Background(), workDir, model, target, stats, nil))

	zr, err := zip.OpenReader(target)
	require.NoError(t, err)
	t.Cleanup(func() { _ = zr.Close() })
	return zr.File
}

// readPackedManifest 读回产物内 manifest.json 并反序列化为清单（往返断言入口）。
func readPackedManifest(t *testing.T, files []*zip.File) *Manifest {
	t.Helper()
	for _, zf := range files {
		if zf.Name != "manifest.json" {
			continue
		}
		rc, err := zf.Open()
		require.NoError(t, err)
		defer func() { _ = rc.Close() }()
		var buf bytes.Buffer
		_, err = buf.ReadFrom(rc)
		require.NoError(t, err)
		m, err := Deserialize(buf.Bytes())
		require.NoError(t, err)
		return m
	}
	t.Fatal("产物内应含 manifest.json")
	return nil
}

// TestPackStructure 锚定 zip 结构：manifest.json + works/<目录>/<文件>，缺失文件不写入。
func TestPackStructure(t *testing.T) {
	workDir, model := buildPackFixture(t)
	target := filepath.Join(t.TempDir(), "out.zip")
	files := packFixture(t, workDir, model, target)

	names := make([]string, 0, len(files))
	for _, zf := range files {
		names = append(names, zf.Name)
	}
	assert.Contains(t, names, "manifest.json")
	assert.Contains(t, names, "works/作品1/作品1.jpg")
	assert.Contains(t, names, "works/作品1/作品1.png")
	assert.NotContains(t, names, "works/作品1/missing.jpg", "缺失源文件不应写入 zip")

	// manifest.json 内容包含缺失标记与 sha256
	m := readPackedManifest(t, files)
	require.Len(t, m.Files, 3)
	assert.True(t, m.Files[2].Missing, "缺失源文件应标注 Missing")
	assert.Empty(t, m.Files[2].Sha256)
	assert.False(t, m.Files[0].Missing)
	assert.NotEmpty(t, m.Files[0].Sha256)
	assert.Equal(t, int64(len("image-data-1")), m.Files[0].Size)
}

// TestPackContentFingerprint 锚定打包为 files[] 补填头部指纹（方案第八节，schemaVersion 3 起为
// 契约必填面）：往返断言——写包 → 读包内 manifest.json → 非缺失条目 ContentFingerprint 与库内
// 同口径指纹器（backend/util/fingerprint）对源文件直算一致，缺失条目留空。
func TestPackContentFingerprint(t *testing.T) {
	workDir, model := buildPackFixture(t)
	target := filepath.Join(t.TempDir(), "out.zip")
	files := packFixture(t, workDir, model, target)
	m := readPackedManifest(t, files)
	require.Len(t, m.Files, 3)

	computer := fingerprint.NewHeadComputer()
	for _, f := range m.Files {
		if f.Missing {
			assert.Empty(t, f.ContentFingerprint, "缺失源文件不填头部指纹: %s", f.StorePath)
			continue
		}
		require.NotEmpty(t, f.ContentFingerprint, "非缺失条目应填头部指纹: %s", f.StorePath)
		want, err := computer.Fingerprint(context.Background(), filepath.Join(workDir, filepath.FromSlash(f.StorePath)))
		require.NoError(t, err)
		assert.Equal(t, want.Digest, f.ContentFingerprint, "头部指纹须与库内同口径指纹器直算一致: %s", f.StorePath)

		// 格式 `<size>:<hex>`（回灌/收件侧解析前提）
		parts := strings.SplitN(f.ContentFingerprint, ":", 2)
		require.Len(t, parts, 2, "头部指纹格式应为 <size>:<hex>: %s", f.ContentFingerprint)
		assert.Equal(t, strconv.FormatInt(f.Size, 10), parts[0], "指纹 size 分量应等于条目字节数")
	}
}

// TestPackContentFingerprintLargeFile 头部采样窗口锚定：>64KB 文件只采样前 64KB（与库内口径
// 一致），且采样不截断写入流——包内条目字节与源文件全量一致、全量 sha256 为整个文件的哈希。
func TestPackContentFingerprintLargeFile(t *testing.T) {
	workDir := t.TempDir()
	content := bytes.Repeat([]byte("0123456789abcdef"), 130*1024/16) // 130KB
	abs := filepath.Join(workDir, "store", "work", "b", "big.bin")
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, content, 0o644))

	model := NewExportModel(&Manifest{
		SchemaVersion: SchemaVersion,
		Meta:          Meta{ExportedAt: 1725000000000},
		Works: []WorkRecord{{ID: 1, SiteWorkName: strp("B"), Resources: []ResourceRecord{
			{ID: 10, Stores: []StoreMount{{StoreType: "image", StoreSeq: 0, StoreID: 100}}},
		}}},
		Files: []FileEntry{{StoreID: 100, StorePath: "store/work/b/big.bin"}},
	})
	target := filepath.Join(t.TempDir(), "out.zip")
	files := packFixture(t, workDir, model, target)
	m := readPackedManifest(t, files)
	require.Len(t, m.Files, 1)
	entry := m.Files[0]

	headSum := sha256.Sum256(content[:64*1024])
	assert.Equal(t, fmt.Sprintf("%d:%s", len(content), hex.EncodeToString(headSum[:])), entry.ContentFingerprint,
		"头部指纹应只采样前 64KB")
	fullSum := sha256.Sum256(content)
	assert.Equal(t, hex.EncodeToString(fullSum[:]), entry.Sha256, "全量哈希应为整个文件的哈希")
	assert.Equal(t, int64(len(content)), entry.Size)

	packed := false
	for _, zf := range files {
		if zf.Name != entry.Path {
			continue
		}
		rc, err := zf.Open()
		require.NoError(t, err)
		got, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		assert.Equal(t, content, got, "头部采样不应截断包内条目字节")
		packed = true
	}
	assert.True(t, packed, "应写入条目 %s", entry.Path)
}

// TestPackMethodMode 锚定压缩模式：manifest deflate，媒体文件 store 模式（风险5）。
func TestPackMethodMode(t *testing.T) {
	workDir, model := buildPackFixture(t)
	target := filepath.Join(t.TempDir(), "out.zip")
	files := packFixture(t, workDir, model, target)

	methodByPath := map[string]uint16{}
	for _, zf := range files {
		methodByPath[zf.Name] = zf.Method
	}
	assert.Equal(t, uint16(zip.Deflate), methodByPath["manifest.json"])
	assert.Equal(t, uint16(zip.Store), methodByPath["works/作品1/作品1.jpg"])
	assert.Equal(t, uint16(zip.Store), methodByPath["works/作品1/作品1.png"])
}

// TestPackMissingOnlyTotal 全缺失时 TotalFiles=0，磁盘预检无需空间（runner 层语义）。
func TestPackMissingOnlyTotal(t *testing.T) {
	workDir := t.TempDir()
	model := NewExportModel(&Manifest{
		SchemaVersion: SchemaVersion,
		Meta:          Meta{ExportedAt: 1725000000000},
		Works: []WorkRecord{
			{ID: 1, SiteWorkName: strp("A"), Resources: []ResourceRecord{
				{ID: 10, Stores: []StoreMount{{StoreType: "image", StoreSeq: 0, StoreID: 100}}},
			}},
		},
		Files: []FileEntry{{StoreID: 100, StorePath: "store/work/a/none.jpg"}},
	})
	p := NewPacker()
	stats, err := p.Plan(context.Background(), workDir, model, "")
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.TotalFiles)
	assert.Equal(t, int64(1), stats.MissingFiles)
}

// TestPackProgress 锚定进度回调：已处理文件数/累计字节按写入文件递增，总数为非缺失数。
func TestPackProgress(t *testing.T) {
	workDir, model := buildPackFixture(t)
	p := NewPacker()
	stats, err := p.Plan(context.Background(), workDir, model, "")
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.TotalFiles)

	target := filepath.Join(t.TempDir(), "out.zip")
	var callbacks []struct{ pf, pb, tf, tb int64 }
	require.NoError(t, p.Pack(context.Background(), workDir, model, target, stats, func(pf, pb, tf, tb int64) {
		callbacks = append(callbacks, struct{ pf, pb, tf, tb int64 }{pf, pb, tf, tb})
	}))
	require.Len(t, callbacks, 2)
	assert.Equal(t, int64(2), callbacks[1].pf, "最后一次回调已处理文件数=总数")
	assert.Equal(t, stats.TotalBytes, callbacks[1].pb, "累计字节=源文件总字节")
	assert.Equal(t, stats.TotalBytes, callbacks[1].tb)
	assert.Equal(t, int64(1), callbacks[0].pf)
	assert.Equal(t, int64(len("image-data-1")), callbacks[0].pb)
}

// TestPackDeterminism 同输入同输出：两次打包字节级一致（固定导出时刻下 zip 完全可复现）。
func TestPackDeterminism(t *testing.T) {
	workDir, model := buildPackFixture(t)
	p := NewPacker()

	packOnce := func() []byte {
		stats, err := p.Plan(context.Background(), workDir, model, "")
		require.NoError(t, err)
		target := filepath.Join(t.TempDir(), "out.zip")
		require.NoError(t, p.Pack(context.Background(), workDir, model, target, stats, nil))
		data, err := os.ReadFile(target)
		require.NoError(t, err)
		return data
	}
	first := packOnce()
	second := packOnce()
	assert.Equal(t, first, second, "同输入两次打包字节应一致")
}

// TestPackSha256 锚定 sha256：与直接对源文件计算一致（回灌校验用）。
func TestPackSha256(t *testing.T) {
	workDir, model := buildPackFixture(t)
	p := NewPacker()
	stats, err := p.Plan(context.Background(), workDir, model, "")
	require.NoError(t, err)
	target := filepath.Join(t.TempDir(), "out.zip")
	require.NoError(t, p.Pack(context.Background(), workDir, model, target, stats, nil))

	src, err := os.ReadFile(filepath.Join(workDir, "store/work/a/作品1.jpg"))
	require.NoError(t, err)
	sum := sha256.Sum256(src)
	assert.Equal(t, hex.EncodeToString(sum[:]), model.Manifest.Files[0].Sha256)
}

// TestPackContextCancel 打包途中 ctx 取消：返回取消错误，不产出完整 zip。
func TestPackContextCancel(t *testing.T) {
	workDir, model := buildPackFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	p := NewPacker()
	stats, err := p.Plan(ctx, workDir, model, "")
	require.NoError(t, err)
	cancel() // 打包前取消：Pack 首个 ctx 检查即中断
	target := filepath.Join(t.TempDir(), "out.zip")
	err = p.Pack(ctx, workDir, model, target, stats, nil)
	require.ErrorIs(t, err, context.Canceled)
}
