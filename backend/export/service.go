package export

import (
	"context"
)

// Service 导出服务：对外提供导出数据收集与异步打包执行能力。
type Service struct {
	collector *Collector
	runner    *Runner
}

// NewService 创建导出服务。
// versionProvider 提供来源 app 版本（写入 manifest.meta.appVersion）。
// workDirProvider 提供当前工作目录（源文件根 + 导出产物落盘根；启动时读取，避免持有过期值）。
// emitter 推送导出进度/完成事件（Wails 事件通道，export-events topic）。
func NewService(repo Repository, versionProvider func() string, workDirProvider func() string, emitter ExportEventEmitter) *Service {
	collector := NewCollector(repo, versionProvider)
	return &Service{
		collector: collector,
		runner:    NewRunner(collector, NewPacker(), emitter, workDirProvider),
	}
}

// Collect 按决策5 收集导出数据模型：前端把选中的 work/workSet id 列表透传，
// 后端完成作品集闭包、成员关系裁剪与全部关联数据收集，产出内存态导出模型。
func (s *Service) Collect(ctx context.Context, workIDs []int64, workSetIDs []int64) (*ExportModel, error) {
	return s.collector.Collect(ctx, workIDs, workSetIDs)
}

// StartExport 启动异步导出：立即返回 exportID，进度/完成经 emitter 推送（不阻塞 IPC）。
// outputDir 为空时沿用工作目录作落盘根，非空为自选输出目录。
func (s *Service) StartExport(ctx context.Context, workIDs []int64, workSetIDs []int64, outputDir string) (string, error) {
	return s.runner.Start(ctx, workIDs, workSetIDs, outputDir)
}

// CancelExport 取消指定导出（无进行中导出则 no-op）。
func (s *Service) CancelExport(exportID string) {
	s.runner.Cancel(exportID)
}

// CleanupResidualTempFiles 清理导出临时文件残留（应用启动时调用）。
func (s *Service) CleanupResidualTempFiles() error {
	return s.runner.CleanupResidualTempFiles()
}
