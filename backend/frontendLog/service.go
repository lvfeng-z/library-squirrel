package frontendLog

import (
	"context"

	"github.com/library-squirrel/backend/base/logger"
	"go.uber.org/zap"
)

// Service 前端日志收集服务，将前端上报的日志写入独立的 frontend.log
type Service struct{}

// NewService 创建前端日志收集服务
func NewService() *Service {
	return &Service{}
}

// Write 批量写入前端日志到独立的 frontend.log（logger.FrontendLog）
func (s *Service) Write(ctx context.Context, entries []FrontendLogEntry) error {
	for _, e := range entries {
		ts := zap.Int64("fe_ts", e.Timestamp)
		switch e.Level {
		case "error":
			logger.FrontendLog.Error(e.Message, ts)
		case "warn":
			logger.FrontendLog.Warn(e.Message, ts)
		case "debug":
			logger.FrontendLog.Debug(e.Message, ts)
		default: // info 及未知级别统一按 info
			logger.FrontendLog.Info(e.Message, ts)
		}
	}
	return nil
}
