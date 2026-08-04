package extension

import (
	"context"
	"fmt"

	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// workSetOrderFetcher 原站序获取能力实现：按 plugin 身份从 registry 取 proxy，调 QueryWorkSetOrder。
// 实现 work.WorkSetOrderFetcher 接口（work 模块定义、plugin 实现——ORCHESTRATION_BY_CALLER）。
// 声明驱动：插件未声明 workOrderQuery 能力时跳过，不盲调 gRPC（SDK 侧类型断言仅作兜底）。
type workSetOrderFetcher struct {
	registry  *TaskHandlerRegistry
	capsQuery CapabilityQuerier
}

// NewWorkSetOrderFetcher 创建原站序获取器。capsQuery 用于声明驱动（插件未声明能力则不调用）。
func NewWorkSetOrderFetcher(registry *TaskHandlerRegistry, capsQuery CapabilityQuerier) *workSetOrderFetcher {
	return &workSetOrderFetcher{registry: registry, capsQuery: capsQuery}
}

// QueryWorkSetOrder 按 (pluginPublicId, extensionId) 定位插件 proxy，拉取作品集内作品的原站全序。
// 插件未声明 workOrderQuery 能力时直接返回 nil（不盲调 gRPC）；声明后由 SDK 侧类型断言兜底。
func (f *workSetOrderFetcher) QueryWorkSetOrder(ctx context.Context, pluginPublicId, extensionId string, siteId int64, siteWorkSetId string) ([]*pluginsdkdto.WorkOrderEntry, error) {
	if !f.hasCapability(pluginPublicId, CapabilityWorkOrderQuery) {
		return nil, nil
	}
	handler, err := f.registry.GetTaskHandler(pluginPublicId, extensionId)
	if err != nil {
		return nil, fmt.Errorf("查找插件 TaskHandler 失败 %s/%s: %w", pluginPublicId, extensionId, err)
	}
	proxy, ok := handler.(*TaskHandlerProxy)
	if !ok {
		return nil, fmt.Errorf("TaskHandler 非 *TaskHandlerProxy，无法查询原站序")
	}
	return proxy.QueryWorkSetOrder(ctx, siteId, siteWorkSetId)
}

// hasCapability 查插件是否声明了指定能力；未注入查询器时保守返回 true（向后兼容）。
func (f *workSetOrderFetcher) hasCapability(pluginPublicId, capability string) bool {
	if f.capsQuery == nil {
		return true
	}
	for _, c := range f.capsQuery.GetCapabilities(pluginPublicId) {
		if c == capability {
			return true
		}
	}
	return false
}
