import {
  EmbedSlotConfig,
  MenuSlotConfig,
  PanelSlotConfig,
  PrecompiledContent,
  SiteBrowserListSlotConfig,
  ViewSlotConfig,
  VueSourceContent
} from '@renderer/model/model/interface/SlotConfigs.ts'

/** 插槽内容类型 */
export type SlotContentType = 'vueSource' | 'precompiled' | 'code'

/**
 * 任意插槽内容类型 - 用于 IPC 序列化
 * 包含所有可能的内容格式
 */
export type AnySlotContent = PrecompiledContent | VueSourceContent | string

/**
 * 插槽配置联合类型 - 使用可辨识联合实现类型安全
 */
export type SyncSlotConfig = MenuSlotConfig | SiteBrowserListSlotConfig | ViewSlotConfig | EmbedSlotConfig | PanelSlotConfig

/**
 * 所有插槽配置的联合类型
 */
export type SlotConfig = EmbedSlotConfig | PanelSlotConfig | ViewSlotConfig | MenuSlotConfig | SiteBrowserListSlotConfig
