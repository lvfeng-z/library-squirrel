package export

// ExportProgressData 导出进度事件载荷（export-events topic，type=progress）。
// 按「已处理文件数 / 总文件数 + 累计字节」上报（方案第5节，风险5 进度模型）。
type ExportProgressData struct {
	ExportID       string `json:"exportId"`
	TotalFiles     int64  `json:"totalFiles"`
	ProcessedFiles int64  `json:"processedFiles"`
	TotalBytes     int64  `json:"totalBytes"`
	ProcessedBytes int64  `json:"processedBytes"`
}

// ExportCompleteData 导出完成事件载荷（export-events topic，type=complete）。
// Success=false 时 ErrMsg 为用户可读失败原因（含取消）；Success=true 时 TargetPath 为最终 zip 绝对路径。
type ExportCompleteData struct {
	ExportID   string `json:"exportId"`
	Success    bool   `json:"success"`
	TargetPath string `json:"targetPath"`
	ErrMsg     string `json:"errMsg"`
}

// ipcExportEvent export-events topic 的信封（type 区分 progress/complete），与 taskManager/merge 同范式。
type ipcExportEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// ExportEventEmitter 导出进度/完成事件推送器。仅做无状态 emit，goroutine 安全
// （进度由打包 goroutine 触发、完成由 run 流程触发）。
type ExportEventEmitter interface {
	PushProgress(data ExportProgressData)
	PushComplete(exportID string, success bool, targetPath, errMsg string)
}

// EventEmitter Wails 事件发射能力（本地接口，避免 export → taskManager/plugin 包耦合，
// 与 resource/merge_service.go 的本地接口同模式）。
type EventEmitter interface {
	Emit(eventName string, data ...any) bool
}

// wailsExportEmitter 基于 Wails Events 的导出事件推送器，推 export-events topic。
// 复用 Wails 事件管道与 ipcExportEvent 信封模式，独立 topic、不进 taskManager 控制面。
type wailsExportEmitter struct {
	emitterFn func() EventEmitter // 延迟读取：构造期 Wails emitter 可能尚未注入，emit 时再取
}

// NewWailsExportEmitter 用"延迟返回 Wails 事件发射器"的闭包构造导出事件推送器。
// 闭包模式（对齐 merge 先例）：导出服务在应用初始化早期构造（此时 Wails emitter 尚未经
// SetEventEmitter 注入），而进度/完成事件只在用户触发导出时才 emit（彼时 emitter 已就绪），
// 故 emit 时再读取，避免持有未就绪引用（LAZY_EMITTER_CLOSURE）。
func NewWailsExportEmitter(emitterFn func() EventEmitter) ExportEventEmitter {
	return &wailsExportEmitter{emitterFn: emitterFn}
}

// emit 向 export-events topic 推送带类型信封的事件；emitter 未就绪时静默跳过（无导出能在该阶段发生）。
func (e *wailsExportEmitter) emit(eventType string, data any) {
	if em := e.emitterFn(); em != nil {
		em.Emit("export-events", &ipcExportEvent{Type: eventType, Data: data})
	}
}

func (e *wailsExportEmitter) PushProgress(data ExportProgressData) {
	e.emit("progress", data)
}

func (e *wailsExportEmitter) PushComplete(exportID string, success bool, targetPath, errMsg string) {
	e.emit("complete", &ExportCompleteData{
		ExportID:   exportID,
		Success:    success,
		TargetPath: targetPath,
		ErrMsg:     errMsg,
	})
}
