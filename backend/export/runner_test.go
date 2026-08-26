package export

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/library-squirrel/backend/migration"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingEmitter 测试事件推送器：complete 经 channel 消费，progress 累积。
type recordingEmitter struct {
	completeCh chan ExportCompleteData
	progress   []ExportProgressData
}

func newRecordingEmitter() *recordingEmitter {
	return &recordingEmitter{completeCh: make(chan ExportCompleteData, 8)}
}

func (e *recordingEmitter) PushProgress(data ExportProgressData) {
	e.progress = append(e.progress, data)
}
func (e *recordingEmitter) PushComplete(exportID string, success bool, targetPath, errMsg string) {
	e.completeCh <- ExportCompleteData{ExportID: exportID, Success: success, TargetPath: targetPath, ErrMsg: errMsg}
}

func (e *recordingEmitter) waitComplete(t *testing.T) ExportCompleteData {
	t.Helper()
	select {
	case c := <-e.completeCh:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("等待导出完成事件超时")
		return ExportCompleteData{}
	}
}

// blockingPacker 测试桩：Plan 返回基于模型文件的统计，Pack 创建临时文件后阻塞至 ctx 取消或 release。
type blockingPacker struct {
	entered chan struct{}
	release chan struct{}
}

func (p *blockingPacker) Plan(_ context.Context, _ string, model *ExportModel) (*PackStats, error) {
	stats := &PackStats{}
	for _, f := range model.Manifest.Files {
		stats.TotalFiles++
		stats.TotalBytes += f.Size
	}
	return stats, nil
}

func (p *blockingPacker) Pack(ctx context.Context, _ string, _ *ExportModel, targetPath string, _ *PackStats, _ ProgressFn) error {
	if err := os.WriteFile(targetPath, []byte("partial"), 0o644); err != nil {
		return err
	}
	close(p.entered)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.release:
		return nil
	}
}

// seedRunnerSourceFile 在测试 workDir 下播种夹具作品的源文件（store 活行指向）。
func seedRunnerSourceFile(t *testing.T, workDir string) {
	t.Helper()
	rel := filepath.Join("store", "resource", "作品1.jpg")
	abs := filepath.Join(workDir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte("image-content"), 0o644))
}

// TestRunnerStartExportProducesZip 端到端：StartExport 立即返回 exportID，后台产出 zip，结构正确。
func TestRunnerStartExportProducesZip(t *testing.T) {
	db, err := migration.OpenTestDB()
	require.NoError(t, err)
	f := seedExportFixture(t, db)
	workDir := t.TempDir()
	seedRunnerSourceFile(t, workDir)

	emitter := newRecordingEmitter()
	runner := NewRunner(newTestCollector(db), NewPacker(), emitter, func() string { return workDir })
	runner.freeSpaceFn = func(string) (uint64, error) { return 1 << 40, nil }

	id, err := runner.Start(context.Background(), []int64{f.w1ID}, nil, "")
	require.NoError(t, err)
	require.NotEmpty(t, id)

	comp := emitter.waitComplete(t)
	require.True(t, comp.Success)
	require.NotEmpty(t, comp.TargetPath)
	assert.FileExists(t, comp.TargetPath)

	zr, err := zip.OpenReader(comp.TargetPath)
	require.NoError(t, err)
	defer func() { _ = zr.Close() }()
	names := make([]string, 0, len(zr.File))
	for _, zf := range zr.File {
		names = append(names, zf.Name)
	}
	assert.Contains(t, names, "manifest.json")
	assert.Contains(t, names, "works/作品1/作品1.jpg")

	// 进度事件已上报
	assert.NotEmpty(t, emitter.progress)
	last := emitter.progress[len(emitter.progress)-1]
	assert.Equal(t, comp.ExportID, last.ExportID)
	assert.Equal(t, int64(1), last.TotalFiles)
	assert.Equal(t, int64(1), last.ProcessedFiles)

	// 工作目录下无临时残留
	entries, err := os.ReadDir(workDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), exportTempSuffix)
	}
}

// TestRunnerCancelCleansTemp 取消：打包桩创建临时文件后阻塞，Cancel 触发 ctx 取消，
// 推送「已取消」且临时文件被清理（风险1：不留半成品）。
func TestRunnerCancelCleansTemp(t *testing.T) {
	db, err := migration.OpenTestDB()
	require.NoError(t, err)
	f := seedExportFixture(t, db)
	workDir := t.TempDir()
	seedRunnerSourceFile(t, workDir)

	emitter := newRecordingEmitter()
	bp := &blockingPacker{entered: make(chan struct{}), release: make(chan struct{})}
	runner := NewRunner(newTestCollector(db), bp, emitter, func() string { return workDir })
	runner.freeSpaceFn = func(string) (uint64, error) { return 1 << 40, nil }

	id, err := runner.Start(context.Background(), []int64{f.w1ID}, nil, "")
	require.NoError(t, err)

	<-bp.entered // 等打包桩进入（临时文件已创建）
	runner.Cancel(id)

	comp := emitter.waitComplete(t)
	assert.False(t, comp.Success)
	assert.Equal(t, "已取消", comp.ErrMsg)

	// 临时文件被清理，无最终 zip
	entries, err := os.ReadDir(workDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), exportTempSuffix, "临时文件应被清理")
		assert.NotContains(t, e.Name(), exportZipPrefix, "不应留下最终 zip")
	}
}

// TestRunnerDiskSpacePrecheck 风险6：目标盘剩余空间不足时导出中止（预检报错），不产出 zip。
func TestRunnerDiskSpacePrecheck(t *testing.T) {
	db, err := migration.OpenTestDB()
	require.NoError(t, err)
	f := seedExportFixture(t, db)
	workDir := t.TempDir()
	seedRunnerSourceFile(t, workDir)

	emitter := newRecordingEmitter()
	runner := NewRunner(newTestCollector(db), NewPacker(), emitter, func() string { return workDir })
	runner.freeSpaceFn = func(string) (uint64, error) { return 1, nil } // 仅 1 字节可用

	_, err = runner.Start(context.Background(), []int64{f.w1ID}, nil, "")
	require.NoError(t, err) // Start 立即返回，失败经 complete 事件

	comp := emitter.waitComplete(t)
	assert.False(t, comp.Success)
	assert.Contains(t, comp.ErrMsg, "剩余空间不足")
}

// TestRunnerEmptySelection 空选择前置校验：Start 同步报错，不起 goroutine。
func TestRunnerEmptySelection(t *testing.T) {
	runner := NewRunner(nil, NewPacker(), newRecordingEmitter(), func() string { return "" })
	_, err := runner.Start(context.Background(), nil, nil, "")
	assert.ErrorIs(t, err, ErrExportEmptySelection)
}

// failingPacker 测试桩：Pack 创建临时文件后报错（模拟源文件读取/写入失败）。
type failingPacker struct{}

func (p *failingPacker) Plan(_ context.Context, _ string, _ *ExportModel) (*PackStats, error) {
	return &PackStats{}, nil
}

func (p *failingPacker) Pack(_ context.Context, _ string, _ *ExportModel, targetPath string, _ *PackStats, _ ProgressFn) error {
	if err := os.WriteFile(targetPath, []byte("partial"), 0o644); err != nil {
		return err
	}
	return errors.New("boom")
}

// TestRunnerFailureNoHalfZip 失败：打包报错 → 推送失败信息、清理半成品临时文件、不产出最终 zip。
func TestRunnerFailureNoHalfZip(t *testing.T) {
	db, err := migration.OpenTestDB()
	require.NoError(t, err)
	f := seedExportFixture(t, db)
	workDir := t.TempDir()

	emitter := newRecordingEmitter()
	runner := NewRunner(newTestCollector(db), &failingPacker{}, emitter, func() string { return workDir })
	runner.freeSpaceFn = func(string) (uint64, error) { return 1 << 40, nil }

	_, err = runner.Start(context.Background(), []int64{f.w1ID}, nil, "")
	require.NoError(t, err)

	comp := emitter.waitComplete(t)
	assert.False(t, comp.Success)
	assert.Equal(t, "boom", comp.ErrMsg)

	entries, err := os.ReadDir(workDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), exportTempSuffix, "半成品临时文件应被清理")
		assert.NotContains(t, e.Name(), exportZipPrefix, "不应留下最终 zip")
	}
}

// TestCleanupResidualTempFiles 启动清理：仅移除导出临时残留，保留最终 zip 与无关文件。
func TestCleanupResidualTempFiles(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "library-squirrel-export-123.zip.tmp"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "library-squirrel-export-456.zip"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "unrelated.txt"), []byte("x"), 0o644))

	runner := NewRunner(nil, NewPacker(), newRecordingEmitter(), func() string { return workDir })
	require.NoError(t, runner.CleanupResidualTempFiles())

	assert.NoFileExists(t, filepath.Join(workDir, "library-squirrel-export-123.zip.tmp"))
	assert.FileExists(t, filepath.Join(workDir, "library-squirrel-export-456.zip"))
	assert.FileExists(t, filepath.Join(workDir, "unrelated.txt"))
}

// TestCleanupResidualTempFilesWorkDirEmpty 工作目录未配置时清理为 no-op。
func TestCleanupResidualTempFilesWorkDirEmpty(t *testing.T) {
	runner := NewRunner(nil, NewPacker(), newRecordingEmitter(), func() string { return "" })
	require.NoError(t, runner.CleanupResidualTempFiles())
}

// TestRunnerCustomOutputDir 自选输出目录：Start 传入非空 outputDir 时产物落所选目录（源文件仍读工作目录）。
func TestRunnerCustomOutputDir(t *testing.T) {
	db, err := migration.OpenTestDB()
	require.NoError(t, err)
	f := seedExportFixture(t, db)
	workDir := t.TempDir()
	seedRunnerSourceFile(t, workDir)
	outDir := t.TempDir()

	emitter := newRecordingEmitter()
	runner := NewRunner(newTestCollector(db), NewPacker(), emitter, func() string { return workDir })
	runner.freeSpaceFn = func(string) (uint64, error) { return 1 << 40, nil }

	id, err := runner.Start(context.Background(), []int64{f.w1ID}, nil, outDir)
	require.NoError(t, err)

	comp := emitter.waitComplete(t)
	require.True(t, comp.Success)
	require.Equal(t, id, comp.ExportID)
	require.Contains(t, comp.TargetPath, outDir, "产物应落在自选输出目录")
	assert.FileExists(t, comp.TargetPath)

	// 工作目录（源文件根）不应新落最终 zip
	entries, err := os.ReadDir(workDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), exportZipPrefix, "工作目录不应出现导出产物")
	}
}
