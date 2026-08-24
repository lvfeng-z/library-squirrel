package fsmonitor

// ChangeKind 文件变更类型，平台无关语义
type ChangeKind int

const (
	// ChangeCreate 文件或目录出现
	ChangeCreate ChangeKind = iota
	// ChangeRemove 文件或目录消失
	ChangeRemove
	// ChangeMove 移动或重命名，携带 From(旧路径)→To(新路径)配对
	ChangeMove
)

// FileChange 一次文件变更。
// 所有路径相对 workDir，与 persistent_store.file_path 同基准(正斜杠)。
type FileChange struct {
	Kind       ChangeKind
	Path       string // 相对 workDir 的路径；Move 时为旧路径(From)
	ToPath     string // 仅 Move 有值：新路径(相对 workDir)
	IsDir      bool
	FromRename bool  // Remove 源于改名/移动的旧名腿（fsnotify Rename Op）：旧路径文件已不在原位
	DetectedAt int64 // 毫秒时间戳
}

// OfflineCursor 离线变更追溯的续读位置，平台特定(如 Windows USN 游标)。
// 上层不解析，仅持有与回传；跨重启持久化由上层负责。
type OfflineCursor []byte

// MissingRecord 对账缺失项：persistent_store 有记录、磁盘无文件(被删或移走)
type MissingRecord struct {
	StoreID  int64 // persistent_store.id
	FilePath string
}

// UntrackedFile 对账未追踪项：磁盘有文件、persistent_store 无记录(外部新增或移入)
type UntrackedFile struct {
	FilePath string
}

// BackupMissingRecord backup 域对账缺失项：backup 清单行存在、磁盘无文件
type BackupMissingRecord struct {
	BackupID int64 // backup.id
	FilePath string
}

// DiffSet 全量对账差异集
type DiffSet struct {
	Missing   []MissingRecord
	Untracked []UntrackedFile
	// BackupMissing backup 域缺失（孤儿文件方向不产出——外部文件落入 backup/ 不构成清单行变更）
	BackupMissing []BackupMissingRecord
}
