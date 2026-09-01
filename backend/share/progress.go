package share

// 分享事件载荷与推送器（share-events topic，与 export-events 同信封范式）。
// 事件类型：progress（发布进行中阶段推进）/ complete（发布终态：成功在线或失败）/
// state（会话运行态变化：重连/撤销/过期/失败/服务统计更新）/
// receive-link（深链到达：分享拉取入口链接，前端打开接收对话框）/
// dial-quota-full（收件拨号配额耗尽：门控进入阻塞态，前端轻提示「等待配额恢复」）。

// ShareSessionDTO 分享会话快照（IPC DTO：状态查询返回值与 state 事件载荷共用）
type ShareSessionDTO struct {
	ShareID           string `json:"shareId"`
	Token             string `json:"token"`        // 中继会话 token（访问凭证）
	Link              string `json:"link"`         // 完整分享链接（含 fragment 密钥；未在线为空）
	Title             string `json:"title"`        // 落地页标题
	WorkCount         int64  `json:"workCount"`    // 分享作品数
	FileCount         int64  `json:"fileCount"`    // 白名单文件数（不含缺失）
	TotalBytes        int64  `json:"totalBytes"`   // 白名单文件总字节
	MissingFiles      int64  `json:"missingFiles"` // 源文件缺失数
	ExpiresAt         int64  `json:"expiresAt"`    // 到期时刻（unix 毫秒，0=无限期）
	PasswordProtected bool   `json:"passwordProtected"`
	State             string `json:"state"` // connecting/online/reconnecting/revoked/expired/failed
	StreamsServed     int64  `json:"streamsServed"`
	BytesServed       int64  `json:"bytesServed"`
	CreatedAt         int64  `json:"createdAt"`
	RelayAddress      string `json:"relayAddress"`
	ErrMsg            string `json:"errMsg"`
}

// ShareProgressData 发布进行中事件载荷
type ShareProgressData struct {
	ShareID string `json:"shareId"`
	Phase   string `json:"phase"` // collecting=收集中 / registering=注册中继中
}

// ShareCompleteData 发布终态事件载荷
type ShareCompleteData struct {
	ShareID string           `json:"shareId"`
	Success bool             `json:"success"`
	Link    string           `json:"link"`              // 成功时的完整分享链接
	ErrMsg  string           `json:"errMsg"`            // 失败原因（含「已取消」）
	Session *ShareSessionDTO `json:"session,omitempty"` // 成功时的会话快照
}

// ipcShareEvent share-events topic 的信封（type 区分 progress/complete/state）
type ipcShareEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// ShareEventEmitter 分享事件推送器（无状态 emit，goroutine 安全）
type ShareEventEmitter interface {
	PushProgress(shareID, phase string)
	PushComplete(data ShareCompleteData)
	PushState(dto *ShareSessionDTO)
	// PushReceiveLink 深链到达（raw 为完整深链 URL；前端打开接收分享对话框）
	PushReceiveLink(link string)
	// PushDialQuotaFull 收件拨号配额耗尽（门控阻塞态进入；前端轻提示，无载荷）
	PushDialQuotaFull()
}

// EventEmitter Wails 事件发射能力（本地接口，避免 share → taskManager/plugin 包耦合）
type EventEmitter interface {
	Emit(eventName string, data ...any) bool
}

// wailsShareEmitter 基于 Wails Events 的分享事件推送器，推 share-events topic。
// emitter 经闭包延迟读取（LAZY_EMITTER_CLOSURE：服务构造期 Wails emitter 可能尚未注入）。
type wailsShareEmitter struct {
	emitterFn func() EventEmitter
}

// NewWailsShareEmitter 构造分享事件推送器
func NewWailsShareEmitter(emitterFn func() EventEmitter) ShareEventEmitter {
	return &wailsShareEmitter{emitterFn: emitterFn}
}

func (e *wailsShareEmitter) emit(eventType string, data any) {
	if em := e.emitterFn(); em != nil {
		em.Emit("share-events", &ipcShareEvent{Type: eventType, Data: data})
	}
}

func (e *wailsShareEmitter) PushProgress(shareID, phase string) {
	e.emit("progress", &ShareProgressData{ShareID: shareID, Phase: phase})
}

func (e *wailsShareEmitter) PushComplete(data ShareCompleteData) {
	e.emit("complete", &data)
}

func (e *wailsShareEmitter) PushState(dto *ShareSessionDTO) {
	e.emit("state", dto)
}

func (e *wailsShareEmitter) PushReceiveLink(link string) {
	e.emit("receive-link", link)
}

func (e *wailsShareEmitter) PushDialQuotaFull() {
	e.emit("dial-quota-full", nil)
}
