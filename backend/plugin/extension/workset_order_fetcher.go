package extension

import (
	"context"
	"fmt"

	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// workSetOrderFetcher 原站序获取能力实现：按 plugin 身份从 registry 取 proxy，调 QueryWorkSetOrder。
// 实现 work.WorkSetOrderFetcher 接口（work 模块定义、plugin 实现——ORCHESTRATION_BY_CALLER）。
type workSetOrderFetcher struct {
	registry *TaskHandlerRegistry
}

// NewWorkSetOrderFetcher 创建原站序获取器
func NewWorkSetOrderFetcher(registry *TaskHandlerRegistry) *workSetOrderFetcher {
	return &workSetOrderFetcher{registry: registry}
}

// QueryWorkSetOrder 按 (pluginPublicId, extensionId) 定位插件 proxy，拉取作品集内作品的原站全序。
// 插件未实现该能力时，proxy 内部 gRPC 返回空（见 transport/plugin_server.go 的类型断言兜底）。
func (f *workSetOrderFetcher) QueryWorkSetOrder(ctx context.Context, pluginPublicId, extensionId string, siteId int64, siteWorkSetId string) ([]*pluginsdkdto.WorkOrderEntry, error) {
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
