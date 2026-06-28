package frontendLog

// FrontendLogEntry 前端单条日志（由前端序列化后批量上报）
type FrontendLogEntry struct {
	Level     string `json:"level"`     // 日志级别：debug/info/warn/error
	Message   string `json:"message"`   // 序列化后的消息文本
	Timestamp int64  `json:"timestamp"` // 前端时间戳（毫秒）
}
