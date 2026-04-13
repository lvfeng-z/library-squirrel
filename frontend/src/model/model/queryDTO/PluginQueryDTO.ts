import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import { BOOL } from '@renderer/model/model/constant/BOOL.ts'
import { ActivationType } from '@renderer/model/model/constant/ActivationType.ts'

export default class PluginQueryDTO extends BaseQueryDTO {
  /**
   * 公开id
   */
  publicId?: string | undefined | null

  /**
   * 作者
   */
  author?: string | undefined | null

  /**
   * 名称
   */
  name?: string | undefined | null

  /**
   * 版本
   */
  version?: string | undefined | null

  /**
   * 描述
   */
  description?: string | undefined | null

  /**
   * 更新日志
   */
  changelog?: string | undefined | null

  /**
   * 入口文件名
   */
  fileName?: string | undefined | null

  /**
   * 备份id
   */
  backupId?: number | undefined | null

  /**
   * 激活类型
   */
  activationType?: ActivationType | undefined | null

  /**
   * 排序号
   */
  sortNum?: number | undefined | null

  /**
   * 是否已卸载
   */
  uninstalled?: BOOL | undefined | null

  constructor(plugin?: PluginQueryDTO) {
    super(plugin)
    if (notNullish(plugin)) {
      this.publicId = plugin.publicId
      this.author = plugin.author
      this.name = plugin.name
      this.version = plugin.version
      this.fileName = plugin.fileName
      this.backupId = plugin.backupId
      this.sortNum = plugin.sortNum
      this.uninstalled = plugin.uninstalled
    }
  }
}
