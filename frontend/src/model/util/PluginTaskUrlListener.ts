import { notNullish } from '@renderer/utils/CommonUtil.ts'

export default class PluginTaskUrlListener {
  /**
   * 插件id
   */
  pluginId: number | undefined | null

  /**
   * 监听表达式
   */
  listener: string | undefined | null

  constructor(pluginTaskUrlListener?: PluginTaskUrlListener) {
    if (notNullish(pluginTaskUrlListener)) {
      this.pluginId = pluginTaskUrlListener.pluginId
      this.listener = pluginTaskUrlListener.listener
    }
  }
}
