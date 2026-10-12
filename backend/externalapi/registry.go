package externalapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Operation 外部操作注册项。每个操作 = 标识 + 参数解码 + 目标方法调用 + 结果编码：
// 显式注册（类型安全、可枚举生成能力清单），不经反射泛调
type Operation struct {
	// ID 操作标识（统一操作端点 URL 的路径段，全局唯一）
	ID string
	// Desc 操作的中文描述（能力清单与接入文档的数据源之一）
	Desc string
	// Invoke 执行操作：入参为请求体原始 JSON（空请求体解码为零值参数，可选参数缺省
	// 由零值承载），返回值作为响应信封的 data 载荷；返回错误即纯失败信封（msg 须用户可读）
	Invoke func(ctx context.Context, params json.RawMessage) (any, error)
}

// Registry 外部操作注册表：外部操作面唯一的调用目录，注册表之外不存在任何动态路由
type Registry struct {
	mu    sync.RWMutex
	ops   map[string]Operation
	order []string
}

// NewRegistry 创建空操作注册表
func NewRegistry() *Registry {
	return &Registry{ops: make(map[string]Operation)}
}

// Register 注册操作；空标识、缺执行函数、标识重复均返回错误（属装配期错误）
func (r *Registry) Register(op Operation) error {
	if op.ID == "" {
		return fmt.Errorf("外部操作标识为空")
	}
	if op.Invoke == nil {
		return fmt.Errorf("外部操作 %s 未提供执行函数", op.ID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.ops[op.ID]; exists {
		return fmt.Errorf("外部操作 %s 重复注册", op.ID)
	}
	r.ops[op.ID] = op
	r.order = append(r.order, op.ID)
	return nil
}

// Lookup 按标识查找操作
func (r *Registry) Lookup(id string) (Operation, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	op, ok := r.ops[id]
	return op, ok
}

// List 按注册序列出全部操作（能力清单的枚举数据源，序稳定）
func (r *Registry) List() []Operation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Operation, 0, len(r.order))
	for _, id := range r.order {
		result = append(result, r.ops[id])
	}
	return result
}

// RegisterHandler 以具体参数类型注册操作：请求体 JSON 解码为 P 后调用 fn。
// 解码失败即该操作失败（不进入业务调用）
func RegisterHandler[P any](r *Registry, id, desc string, fn func(ctx context.Context, params P) (any, error)) error {
	return r.Register(Operation{
		ID:   id,
		Desc: desc,
		Invoke: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var params P
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &params); err != nil {
					return nil, fmt.Errorf("参数解码失败: %w", err)
				}
			}
			return fn(ctx, params)
		},
	})
}
