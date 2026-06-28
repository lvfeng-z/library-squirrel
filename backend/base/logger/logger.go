package logger

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/library-squirrel/backend/config"
	"github.com/library-squirrel/backend/util"
)

var (
	Log *zap.SugaredLogger
	// FrontendLog 前端转发日志（仅写入 log/frontend.log），与业务日志物理隔离
	FrontendLog *zap.SugaredLogger
	// logWriter 持有当前 lumberjack 实例，Reinit 重建前关闭以释放 server.log 句柄，
	// 避免旧实例句柄持续占用文件导致轮转 rename 失败
	logWriter io.Closer
	// frontendLogWriter 持有 frontend.log 的 lumberjack 实例，管理方式同 logWriter
	frontendLogWriter io.Closer
)

// Init 使用默认配置初始化 Logger（main() 最先调用，确保 logger.Log 可用）
func Init() error {
	return initWithConfig(defaultLogConfig())
}

// Reinit 使用用户配置重建 Logger（配置加载后调用，替换全局 logger.Log）
func Reinit(cfg config.LogConfig) error {
	Sync()
	// 关闭旧 lumberjack 实例，释放其持有的 server.log 文件句柄，
	// 否则旧句柄会持续占用文件，导致新实例轮转时 rename 失败
	if logWriter != nil {
		_ = logWriter.Close()
		logWriter = nil
	}
	if frontendLogWriter != nil {
		_ = frontendLogWriter.Close()
		frontendLogWriter = nil
	}
	return initWithConfig(cfg)
}

// Sync 刷新缓冲区
func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
	if FrontendLog != nil {
		_ = FrontendLog.Sync()
	}
}

// initWithConfig 根据配置构建 Logger
func initWithConfig(cfg config.LogConfig) error {
	logDir := filepath.Join(util.RootPath(), "log")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 解析日志级别
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return fmt.Errorf("解析日志级别失败: %w", err)
	}

	// lumberjack 文件写入器（支持轮转）
	writer := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "server.log"),
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	}
	logWriter = writer

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 文件 Core：始终使用 JSON 编码（便于日志分析），级别使用配置值
	fileEncoder := zapcore.NewJSONEncoder(encoderConfig)
	fileWriter := zapcore.AddSync(writer)
	fileCore := zapcore.NewCore(fileEncoder, fileWriter, level)

	// 控制台 Core：使用配置的格式，始终 DebugLevel（开发者需要全部日志）
	consoleEncoder := newEncoder(cfg.Format, encoderConfig)
	consoleWriter := zapcore.AddSync(os.Stdout)
	consoleCore := zapcore.NewCore(consoleEncoder, consoleWriter, zapcore.DebugLevel)

	logger := zap.New(zapcore.NewTee(fileCore, consoleCore))
	Log = logger.Sugar()

	// frontend.log：独立 lumberjack（惰性创建文件：无写入则不产生文件，故生产无此文件），
	// 仅写文件不回控制台，DebugLevel 全量落盘，不受 log.level 过滤
	feWriter := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "frontend.log"),
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	}
	frontendLogWriter = feWriter
	feCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(feWriter),
		zapcore.DebugLevel,
	)
	FrontendLog = zap.New(feCore).Sugar()

	return nil
}

// defaultLogConfig 返回与当前硬编码行为一致的默认配置
func defaultLogConfig() config.LogConfig {
	return config.LogConfig{
		Level:      "info",
		Format:     "console",
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     30,
		Compress:   false,
	}
}

// parseLevel 将字符串解析为 zapcore.Level
func parseLevel(s string) (zapcore.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, errors.New("未知的日志级别: " + s)
	}
}

// newEncoder 根据 format 配置创建编码器
func newEncoder(format string, encCfg zapcore.EncoderConfig) zapcore.Encoder {
	if strings.ToLower(format) == "json" {
		return zapcore.NewJSONEncoder(encCfg)
	}
	return zapcore.NewConsoleEncoder(encCfg)
}
