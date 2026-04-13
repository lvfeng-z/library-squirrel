import BaseEntity from '../base/BaseEntity.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import { BOOL } from '@renderer/model/model/constant/BOOL.ts'
import { ActivationType } from '../constant/ActivationType.ts'

export default class Plugin extends BaseEntity {
  /**
   * 公开id
   */
  publicId: string | undefined | null

  /**
   * 作者
   */
  author: string | undefined | null

  /**
   * 名称
   */
  name: string | undefined | null

  /**
   * 版本
   */
  version: string | undefined | null

  /**
   * 描述
   */
  description: string | undefined | null

  /**
   * 更新日志
   */
  changelog: string | undefined | null

  /**
   * 入口路径
   */
  entryPath: string | undefined | null

  /**
   * 插件根目录相对路径
   */
  rootPath: string | undefined | null

  /**
   * 备份id
   */
  backupId: number | undefined | null

  /**
   * 激活类型
   */
  activationType: ActivationType | undefined | null

  /**
   * 排序号
   */
  sortNum: number | undefined | null

  /**
   * 插件数据
   */
  pluginData: string | undefined | null

  /**
   * 是否已卸载
   */
  uninstalled: BOOL | undefined | null

  constructor(plugin?: Plugin) {
    super(plugin)
    if (notNullish(plugin)) {
      this.publicId = plugin.publicId
      this.author = plugin.author
      this.name = plugin.name
      this.version = plugin.version
      this.description = plugin.description
      this.changelog = plugin.changelog
      this.entryPath = plugin.entryPath
      this.rootPath = plugin.rootPath
      this.backupId = plugin.backupId
      this.activationType = plugin.activationType
      this.sortNum = plugin.sortNum
      this.pluginData = plugin.pluginData
      this.uninstalled = plugin.uninstalled
    } else {
      this.uninstalled = BOOL.FALSE
    }
  }
}
