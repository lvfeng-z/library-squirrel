import { Readable } from 'node:stream'

export default class PluginResourceDTO {
  /**
   * 扩展名
   */
  filenameExtension: string | undefined | null

  /**
   * 建议名称
   */
  suggestedName: string | undefined | null

  /**
   * 作品资源的数据流
   */
  resourceStream: Readable | undefined | null

  /**
   * 作品资源的文件大小，单位：字节（Bytes）
   */
  resourceSize: number | undefined | null

  /**
   * 资源是否支持续传（只在恢复任务时生效，开始任务时没有作用）
   */
  continuable: boolean | undefined | null

  /**
   * 导入方式（0：本地导入，1：站点下载）
   */
  importMethod: number | undefined | null

  constructor(resourcePluginDTO?: PluginResourceDTO) {
    this.filenameExtension = resourcePluginDTO?.filenameExtension
    this.suggestedName = resourcePluginDTO?.suggestedName
    this.resourceStream = resourcePluginDTO?.resourceStream
    this.resourceSize = resourcePluginDTO?.resourceSize
    this.continuable = resourcePluginDTO?.continuable
    this.importMethod = resourcePluginDTO?.importMethod
  }
}
