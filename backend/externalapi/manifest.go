package externalapi

import (
	"net/http"
	"sync"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model"
)

// 能力清单：外部操作面的机器可读目录。操作目录由注册表枚举（注册即入清单，不存在第二份
// 操作表），每个操作附参数摘要；注册表外的固定端点单列一节。清单是接入文档的机器面，也是
// 边缘桥（把清单映射为工具表的薄桥）的数据源

const (
	// ManifestPath 能力清单端点路径（GET，鉴权同操作端点）
	ManifestPath = "/api/manifest"
	// ManifestVersion 清单契约版本：清单字段结构变化时递增，消费者据此判定兼容
	ManifestVersion = "1"
)

// OperationParam 操作参数摘要：描述一个参数的名称、JSON 类型、是否必填与说明语义四要素
// （不做 JSON Schema 全量建模；参数缺省语义写进说明）
type OperationParam struct {
	// Name 参数名（请求体 JSON 字段名；查询参数形态的端点用同名字段）
	Name string `json:"name"`
	// Type 参数类型（JSON 类型名：string/int/int64/bool/array/object）
	Type string `json:"type"`
	// Required 是否必填（false = 可省略，缺省语义见 Desc）
	Required bool `json:"required"`
	// Desc 参数说明（取值域与缺省语义）
	Desc string `json:"desc"`
}

// ManifestOperation 清单中的单个操作条目（调用形态：POST 操作端点路径 + 操作标识，
// 请求体为该操作的参数 JSON 对象）
type ManifestOperation struct {
	// ID 操作标识
	ID string `json:"id"`
	// Desc 操作描述
	Desc string `json:"desc"`
	// Params 参数摘要（无参数的操作为空列表）
	Params []OperationParam `json:"params"`
}

// ManifestEndpoint 注册表外的固定端点条目（启用态探测、能力清单自身、记账增量查询）
type ManifestEndpoint struct {
	// Method HTTP 方法
	Method string `json:"method"`
	// Path 请求路径（查询参数形态的端点在此路径上用查询参数传参）
	Path string `json:"path"`
	// Auth 是否需要 Bearer token 鉴权
	Auth bool `json:"auth"`
	// Desc 端点描述
	Desc string `json:"desc"`
	// Params 参数摘要（无参数为空列表）
	Params []OperationParam `json:"params"`
}

// Manifest 能力清单：操作目录 + 固定端点 + 调用所需的位置信息
type Manifest struct {
	// Version 清单契约版本
	Version string `json:"version"`
	// AppVersion 应用版本（消费者据此记录与归因）
	AppVersion string `json:"appVersion"`
	// OperationEndpoint 统一操作调用端点的路径模板（POST，{operationId} 处填操作标识）
	OperationEndpoint string `json:"operationEndpoint"`
	// Operations 注册表内全部操作（按注册序，序稳定）
	Operations []ManifestOperation `json:"operations"`
	// Endpoints 注册表外的固定端点
	Endpoints []ManifestEndpoint `json:"endpoints"`
}

// operationParams 已注册操作的参数摘要（键 = 操作标识）：注册时随注册登记，清单按标识取用。
// 仅装配期（监听启动前）写入，运行期只读；读写经互斥锁保护（清单端点可被并发请求）
var (
	operationParamsMu sync.Mutex
	operationParams   = make(map[string][]OperationParam)
)

// recordOperationParams 登记操作的参数摘要（随操作注册调用）
func recordOperationParams(operationID string, params []OperationParam) {
	operationParamsMu.Lock()
	defer operationParamsMu.Unlock()
	operationParams[operationID] = params
}

// operationParamSummary 取操作的参数摘要（未登记返回 nil，清单中该操作参数为空列表）
func operationParamSummary(operationID string) []OperationParam {
	operationParamsMu.Lock()
	defer operationParamsMu.Unlock()
	return operationParams[operationID]
}

// BuildManifest 按注册表当前内容生成能力清单：逐个注册项取标识与描述，参数摘要取注册时
// 登记的摘要。注册表在装配期定型、运行期只读，故每次请求现算与缓存等价
func BuildManifest(registry *Registry) *Manifest {
	registered := registry.List()
	operations := make([]ManifestOperation, 0, len(registered))
	for _, op := range registered {
		operations = append(operations, ManifestOperation{
			ID:     op.ID,
			Desc:   op.Desc,
			Params: operationParamSummary(op.ID),
		})
	}
	return &Manifest{
		Version:           ManifestVersion,
		AppVersion:        constant.AppVersion,
		OperationEndpoint: opPathPrefix + "{operationId}",
		Operations:        operations,
		Endpoints:         queryEndpoints(),
	}
}

// queryEndpoints 注册表外固定端点的清单：启用态探测（免鉴权）、能力清单自身、记账账表增量
// 查询与外部操作记录增量查询（查询参数形态；记录端点的客户端标签经请求头自报）
func queryEndpoints() []ManifestEndpoint {
	return []ManifestEndpoint{
		{Method: http.MethodGet, Path: healthPath, Auth: false, Desc: "启用态与应用版本探测：可达即接口已启用", Params: []OperationParam{}},
		{Method: http.MethodGet, Path: ManifestPath, Auth: true, Desc: "能力清单：操作目录与参数摘要（本清单）", Params: []OperationParam{}},
		{Method: http.MethodGet, Path: JournalPath, Auth: true, Desc: "事件记账增量查询（查询参数形态，同一实现的两种传参形态之一）", Params: journalQueryParamSpec},
		{Method: http.MethodGet, Path: OplogPath, Auth: true, Desc: "外部操作记录增量查询（问责用；调用方可经 X-Client-Label 请求头自报标签）", Params: oplogQueryParamSpec},
	}
}

// handleManifest 能力清单端点：按注册表当前内容生成清单，响应为同构信封（data = 清单），
// 失败语义与操作端点一致
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	if !s.tryAcquireDispatchSlot() {
		writeJSON(w, http.StatusServiceUnavailable, model.Error[any]("外部接口并发已达上限，请稍后重试"))
		return
	}
	defer s.releaseDispatchSlot()
	writeJSON(w, http.StatusOK, model.Success(BuildManifest(s.registry)))
}
