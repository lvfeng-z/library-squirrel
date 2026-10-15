import { markRaw, type Component } from 'vue'
import {
  Brush,
  Coordinate,
  Delete,
  Discount,
  FolderChecked,
  Guide,
  HomeFilled,
  Link,
  List,
  MessageBox,
  Setting,
  Share,
  Star,
  TakeawayBox,
  User
} from '@element-plus/icons-vue'
import { isBlank } from '@renderer/utils/StringUtil.ts'

/**
 * 插件声明图标解析（menu 前端扩展 content.icon 的宿主侧消费面）
 *
 * 插件菜单声明的 icon 有两种形态：
 * - 图片相对路径：后端注册期已解析为完整 URL（/plugin/<publicId>/<cacheKey>/<path>），原样透传给 <el-image>
 * - 宿主图标名引用「ep:<Name>」：解析为宿主注册表内的 Element Plus 图标组件，
 *   与内置菜单同构渲染（矢量、随主题色与字号缩放），插件无须打包图片资产
 *
 * 注册表为封闭集（可用名以 doc/plugin-dev-guide.md「前端扩展」节登记为准），
 * 未命中的名字告警并按原字符串返回（落入图片 URL 渲染失败兜底图标，声明错误在界面可见）。
 * markRaw 与内置菜单同因：组件对象入响应式 store 会触发 Vue 的 reactive 代理告警。
 */

const HOST_ICON_REF_PREFIX = 'ep:'

const hostIconRegistry: Readonly<Record<string, Component>> = Object.freeze({
  MessageBox: markRaw(MessageBox),
  HomeFilled: markRaw(HomeFilled),
  Discount: markRaw(Discount),
  User: markRaw(User),
  Star: markRaw(Star),
  List: markRaw(List),
  Link: markRaw(Link),
  Share: markRaw(Share),
  TakeawayBox: markRaw(TakeawayBox),
  Setting: markRaw(Setting),
  Guide: markRaw(Guide),
  Coordinate: markRaw(Coordinate),
  Delete: markRaw(Delete),
  FolderChecked: markRaw(FolderChecked),
  Brush: markRaw(Brush)
})

/**
 * 解析插件声明的图标值：ep: 名字引用 → 注册表组件；其余（图片完整 URL/相对路径）原样返回；
 * 空值返回 undefined
 */
export function resolvePluginIcon(icon: string | null | undefined): Component | string | undefined {
  if (isBlank(icon)) return undefined
  if (!icon.startsWith(HOST_ICON_REF_PREFIX)) return icon
  const component = hostIconRegistry[icon.slice(HOST_ICON_REF_PREFIX.length)]
  if (!component) {
    console.warn(`插件声明图标名未命中宿主注册表，按图片路径兜底：${icon}`)
    return icon
  }
  return component
}
