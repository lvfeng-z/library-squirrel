package authorInfo

// 手动拉取面候选冲突显选的粘性记忆读写：站点域 = 站点键本身（冲突本按站点聚合，候选也按
// 站点收窄），候选组合按候选全键字典序编码。读点在冲突检测分支内（命中即直填显选、跳过
// 冲突记录），写点在该站点拉取成功后（显选带「记住此选择」勾选才落表）；自动触发面不做
// 冲突检测，两个入口均不会被其触达。

import (
	"context"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/stickymemory"
)

// siteFetchMemoryContextKey 拉取冲突记忆的上下文键：站点域 = 站点键，候选组合 = 该站点
// 候选清单的候选全键列表（stickymemory.BuildContextKey 内部按全键字典序排序，候选发现序
// 不稳定不分裂记忆）。读点与写点共用本函数，保证键同构
func siteFetchMemoryContextKey(siteKey string, candidates []*dto.PluginCandidate) string {
	fullKeys := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		fullKeys = append(fullKeys, stickymemory.CandidateFullKey(candidate.PluginPublicId, candidate.ExtensionId))
	}
	return stickymemory.BuildContextKey(siteKey, fullKeys)
}

// recallRememberedChoice 取回该站点当前候选组合下记住的显选：按候选全键列表构造上下文键
// 查记忆，命中的候选全键映射回候选集内的具体条目（补全为条目级两键显选）。未命中、或
// 记住的候选已不在当前候选集（所选插件停用/卸载后候选组合即变，键不匹配自然未命中；行内
// 值指向不在场候选亦同此处理）均返回 nil，调用方照常记冲突交用户显选
func (s *Service) recallRememberedChoice(ctx context.Context, siteKey string, candidates []*dto.PluginCandidate) *dto.SiteAuthorFetchChoice {
	value, ok := s.memory.Recall(ctx, entity.DomainSiteAuthorFetchDisambiguation, siteFetchMemoryContextKey(siteKey, candidates))
	if !ok {
		return nil
	}
	pluginPublicId, extensionId := stickymemory.SplitCandidateFullKey(value)
	for _, candidate := range candidates {
		if candidate.PluginPublicId == pluginPublicId && candidate.ExtensionId == extensionId {
			return &dto.SiteAuthorFetchChoice{
				SiteKey:        siteKey,
				PluginPublicId: pluginPublicId,
				ExtensionId:    extensionId,
			}
		}
	}
	return nil
}

// rememberFetchChoice 拉取成功后落显选记忆（粒度 = 站点级，与冲突/显选粒度一致）：显选带
// 「记住此选择」勾选才写，上下文键按当前候选组合现算（与读点同构）；候选不足两个不写
// ——单候选无从显选，写入也不会被任何冲突读取命中。写入失败仅记日志不影响拉取结果；
// 键值编码单一来源 stickymemory 包
func (s *Service) rememberFetchChoice(ctx context.Context, siteKey string, chosen *dto.SiteAuthorFetchChoice) {
	if chosen == nil || !chosen.Remember {
		return
	}
	candidates, err := s.fetcher.ListSiteAuthorFetchCandidates(ctx, siteKey)
	if err != nil {
		logger.Log.Warnf("[authorInfo] 记住拉取显选：枚举站点 %s 候选失败: %v", siteKey, err)
		return
	}
	if len(candidates) < 2 {
		return
	}
	value := stickymemory.CandidateFullKey(chosen.PluginPublicId, chosen.ExtensionId)
	if err := s.memory.Remember(ctx, entity.DomainSiteAuthorFetchDisambiguation,
		siteFetchMemoryContextKey(siteKey, candidates), value); err != nil {
		logger.Log.Warnf("[authorInfo] 记住站点 %s 的拉取显选失败: %v", siteKey, err)
	}
}
