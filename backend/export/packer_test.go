package export

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

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
	writeFile("store/resource/a/作品1.jpg", []byte("image-data-1"))
	writeFile("store/resource/a/作品1.png", []byte("image-data-2-longer-content"))
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
			{StoreID: 100, StorePath: "store/resource/a/作品1.jpg"},
			{StoreID: 101, StorePath: "store/resource/a/作品1.png"},
			{StoreID: 102, StorePath: "store/resource/a/missing.jpg"},
		},
	})
	return workDir, model
}

// packFixture 执行 Plan+Pack 到指定目标，返回 zip 条目与方法。
func packFixture(t *testing.T, workDir string, model *ExportModel, target string) []*zip.File {
	t.Helper()
	p := NewPacker()
	stats, err := p.Plan(context.Background(), workDir, model)
	require.NoError(t, err)
	require.NoError(t, p.Pack(context.Background(), workDir, model, target, stats, nil))

	zr, err := zip.OpenReader(target)
	require.NoError(t, err)
	t.Cleanup(func() { _ = zr.Close() })
	return zr.File
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
	var manifest *zip.File
	for _, zf := range files {
		if zf.Name == "manifest.json" {
			manifest = zf
		}
	}
	require.NotNil(t, manifest)
	rc, err := manifest.Open()
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()
	var buf bytes.Buffer
	_, err = buf.ReadFrom(rc)
	require.NoError(t, err)
	m, err := Deserialize(buf.Bytes())
	require.NoError(t, err)
	require.Len(t, m.Files, 3)
	assert.True(t, m.Files[2].Missing, "缺失源文件应标注 Missing")
	assert.Empty(t, m.Files[2].Sha256)
	assert.False(t, m.Files[0].Missing)
	assert.NotEmpty(t, m.Files[0].Sha256)
	assert.Equal(t, int64(len("image-data-1")), m.Files[0].Size)
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
		Files: []FileEntry{{StoreID: 100, StorePath: "store/resource/a/none.jpg"}},
	})
	p := NewPacker()
	stats, err := p.Plan(context.Background(), workDir, model)
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.TotalFiles)
	assert.Equal(t, int64(1), stats.MissingFiles)
}

// TestPackProgress 锚定进度回调：已处理文件数/累计字节按写入文件递增，总数为非缺失数。
func TestPackProgress(t *testing.T) {
	workDir, model := buildPackFixture(t)
	p := NewPacker()
	stats, err := p.Plan(context.Background(), workDir, model)
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
		stats, err := p.Plan(context.Background(), workDir, model)
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
	stats, err := p.Plan(context.Background(), workDir, model)
	require.NoError(t, err)
	target := filepath.Join(t.TempDir(), "out.zip")
	require.NoError(t, p.Pack(context.Background(), workDir, model, target, stats, nil))

	src, err := os.ReadFile(filepath.Join(workDir, "store/resource/a/作品1.jpg"))
	require.NoError(t, err)
	sum := sha256.Sum256(src)
	assert.Equal(t, hex.EncodeToString(sum[:]), model.Manifest.Files[0].Sha256)
}

// TestPackContextCancel 打包途中 ctx 取消：返回取消错误，不产出完整 zip。
func TestPackContextCancel(t *testing.T) {
	workDir, model := buildPackFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	p := NewPacker()
	stats, err := p.Plan(ctx, workDir, model)
	require.NoError(t, err)
	cancel() // 打包前取消：Pack 首个 ctx 检查即中断
	target := filepath.Join(t.TempDir(), "out.zip")
	err = p.Pack(ctx, workDir, model, target, stats, nil)
	require.ErrorIs(t, err, context.Canceled)
}
