import Plugin from '@renderer/model/model/entity/Plugin.ts'
import { ContributionKey } from '../../../main-go/plugin/types/ContributionTypes.ts'

export default class PluginWithContribution extends Plugin {
  /**
   * 贡献点类型
   */
  contributeKey: ContributionKey

  /**
   * 贡献点id
   */
  contributionId: string

  constructor(plugin: Plugin, contributeKey: ContributionKey, contributionId: string) {
    super(plugin)
    this.contributeKey = contributeKey
    this.contributionId = contributionId
  }
}
