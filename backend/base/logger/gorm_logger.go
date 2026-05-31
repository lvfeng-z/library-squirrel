package logger

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm/logger"
)

// GormLogger 适配 GORM logger.Interface 到 zap SugaredLogger
type GormLogger struct {
	level logger.LogLevel
}

// NewGormLogger 创建 GORM 日志适配器
func NewGormLogger() *GormLogger {
	return &GormLogger{
		level: logger.Warn, // 默认 Warn 级别：输出 Warn、Error 和慢查询
	}
}

// LogMode 设置日志级别
func (l *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newLogger := *l
	newLogger.level = level
	return &newLogger
}

// Info 输出 Info 级别日志
func (l *GormLogger) Info(_ context.Context, msg string, args ...interface{}) {
	if l.level >= logger.Info && Log != nil {
		Log.Infof("[GORM] "+msg, args...)
	}
}

// Warn 输出 Warn 级别日志
func (l *GormLogger) Warn(_ context.Context, msg string, args ...interface{}) {
	if l.level >= logger.Warn && Log != nil {
		Log.Warnf("[GORM] "+msg, args...)
	}
}

// Error 输出 Error 级别日志
func (l *GormLogger) Error(_ context.Context, msg string, args ...interface{}) {
	if l.level >= logger.Error && Log != nil {
		Log.Errorf("[GORM] "+msg, args...)
	}
}

// Trace 输出 SQL 执行日志
func (l *GormLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if Log == nil {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	switch {
	case err != nil:
		Log.Errorf("[GORM] %s | %d rows | %s | %s", elapsed, rows, sql, err)
	case elapsed > 200*time.Millisecond:
		Log.Warnf("[GORM] slow sql >= 200ms | %s | %d rows | %s", elapsed, rows, sql)
	case l.level >= logger.Info:
		Log.Infof("[GORM] %s | %d rows | %s", elapsed, rows, sql)
	}
}

// FormatDuration 格式化持续时间
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000.0)
	}
	return d.String()
}
