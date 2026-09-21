package extension

import (
	"context"
	"fmt"

	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// workSetRelationFetcher 作品集父集关系获取能力实现：按 plugin 身份从 registry 取 proxy，调 QueryWorkSetRelations。
// 实现 work.WorkSetRelationFetcher 接口（work 模块定义、plugin 实现——ORCHESTRATION_BY_CALLER）。
// 声明驱动为条目级：该（插件, 任务处理器条目）未声明 workSetRelationQuery 时跳过，不盲调 gRPC
// （SDK 侧类型断言仅作兜底）。
type workSetRelationFetcher struct {
	registry     *TaskHandlerRegistry
	optionsQuery TaskHandlerOptionQuerier
}

// NewWorkSetRelationFetcher 创建作品集父集关系获取器。optionsQuery 用于声明驱动（条目未声明方法组则不调用）。
func NewWorkSetRelationFetcher(registry *TaskHandlerRegistry, optionsQuery TaskHandlerOptionQuerier) *workSetRelationFetcher {
	return &workSetRelationFetcher{registry: registry, optionsQuery: optionsQuery}
}

// QueryWorkSetRelations 按 (pluginPublicId, extensionId) 定位插件 proxy，拉取本作品集的父集关系 + 在各父集下的原站序。
// 该（插件, 条目）未声明 workSetRelationQuery 方法组时直接返回 nil（不盲调 gRPC）；声明后由 SDK 侧类型断言兜底。
func (f *workSetRelationFetcher) QueryWorkSetRelations(ctx context.Context, pluginPublicId, extensionId string, siteId int64, siteWorkSetId string) ([]*pluginsdkdto.WorkSetRelationEntry, error) {
	if !f.declaresOption(pluginPublicId, extensionId, CapabilityWorkSetRelationQuery) {
		return nil, nil
	}
	handler, err := f.registry.GetTaskHandler(pluginPublicId, extensionId)
	if err != nil {
		return nil, fmt.Errorf("查找插件 TaskHandler 失败 %s/%s: %w", pluginPublicId, extensionId, err)
	}
	proxy, ok := handler.(*TaskHandlerProxy)
	if !ok {
		return nil, fmt.Errorf("TaskHandler 非 *TaskHandlerProxy，无法查询父集关系")
	}
	return proxy.QueryWorkSetRelations(ctx, siteId, siteWorkSetId)
}

// declaresOption 查插件该任务处理器条目是否声明了指定可选方法组；未注入查询器时保守返回 true（向后兼容）。
func (f *workSetRelationFetcher) declaresOption(pluginPublicId, extensionId, option string) bool {
	if f.optionsQuery == nil {
		return true
	}
	return f.optionsQuery.HasTaskHandlerOption(pluginPublicId, extensionId, option)
}
