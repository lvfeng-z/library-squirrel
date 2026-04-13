/**
 * 站点浏览器 DTO
 */
export default class SiteBrowserDTO {
  /** 贡献点id（站点浏览器在插件内的唯一标识） */
  contributionId: string
  /** 插件公开 ID */
  pluginPublicId: string
  /** 名称 */
  name: string
  /** 插件 ID */
  pluginId: number

  constructor(data?: Partial<SiteBrowserDTO>) {
    this.contributionId = data?.contributionId ?? ''
    this.pluginPublicId = data?.pluginPublicId ?? ''
    this.name = data?.name ?? ''
    this.pluginId = data?.pluginId ?? 0
  }

  /**
   * 获取完整 ID（pluginPublicId + "-" + contributionId）
   */
  get id(): string {
    return `${this.pluginPublicId}-${this.contributionId}`
  }
}
