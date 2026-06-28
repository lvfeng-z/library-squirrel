package frontendLog

import (
	"context"

	"github.com/library-squirrel/backend/base/logger"
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
		switch e.Level {
		case "error":
			logger.FrontendLog.Errorw(e.Message, "fe_ts", e.Timestamp)
		case "warn":
			logger.FrontendLog.Warnw(e.Message, "fe_ts", e.Timestamp)
		case "debug":
			logger.FrontendLog.Debugw(e.Message, "fe_ts", e.Timestamp)
		default: // info 及未知级别统一按 info
			logger.FrontendLog.Infow(e.Message, "fe_ts", e.Timestamp)
		}
	}
	return nil
}
