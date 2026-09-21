package extension

import (
	"context"
	"fmt"

	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// workSetOrderFetcher 原站序获取能力实现：按 plugin 身份从 registry 取 proxy，调 QueryWorkSetOrder。
// 实现 work.WorkSetOrderFetcher 接口（work 模块定义、plugin 实现——ORCHESTRATION_BY_CALLER）。
// 声明驱动为条目级：该（插件, 任务处理器条目）未声明 workOrderQuery 时跳过，不盲调 gRPC
// （SDK 侧类型断言仅作兜底）。
type workSetOrderFetcher struct {
	registry     *TaskHandlerRegistry
	optionsQuery TaskHandlerOptionQuerier
}

// NewWorkSetOrderFetcher 创建原站序获取器。optionsQuery 用于声明驱动（条目未声明方法组则不调用）。
func NewWorkSetOrderFetcher(registry *TaskHandlerRegistry, optionsQuery TaskHandlerOptionQuerier) *workSetOrderFetcher {
	return &workSetOrderFetcher{registry: registry, optionsQuery: optionsQuery}
}

// QueryWorkSetOrder 按 (pluginPublicId, extensionId) 定位插件 proxy，拉取作品集内作品的原站全序。
// 该（插件, 条目）未声明 workOrderQuery 方法组时直接返回 nil（不盲调 gRPC）；声明后由 SDK 侧类型断言兜底。
func (f *workSetOrderFetcher) QueryWorkSetOrder(ctx context.Context, pluginPublicId, extensionId string, siteId int64, siteWorkSetId string) ([]*pluginsdkdto.WorkOrderEntry, error) {
	if !f.declaresOption(pluginPublicId, extensionId, CapabilityWorkOrderQuery) {
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

// declaresOption 查插件该任务处理器条目是否声明了指定可选方法组；未注入查询器时保守返回 true（向后兼容）。
func (f *workSetOrderFetcher) declaresOption(pluginPublicId, extensionId, option string) bool {
	if f.optionsQuery == nil {
		return true
	}
	return f.optionsQuery.HasTaskHandlerOption(pluginPublicId, extensionId, option)
}
