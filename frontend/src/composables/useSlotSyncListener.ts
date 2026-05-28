import type { MenuSlotItem, SiteBrowserListSlotItem } from '@renderer/store/SlotRegistryStore'
import { useSlotRegistryStore } from '@renderer/store/SlotRegistryStore'
import type { EmbedSlot, PanelSlot, ViewSlot } from '@renderer/model/slot'
import { isNullish } from '@renderer/utils/CommonUtil.ts'
import { parse } from '@vue/compiler-sfc'
import { compile, defineComponent } from 'vue'
import * as Vue from 'vue'
import type { SlotResponse } from '@bindings/github.com/library-squirrel/backend/slot/models'
import { AnySlotContent } from '@renderer/model/model/constant/SlotTypes.ts'
import type { HtmlContent, PrecompiledContent, VueSourceContent } from '@renderer/model/model/interface/SlotConfigs.ts'
import { DefineComponent } from 'vue'
import { Handler as SlotHandler } from '@bindings/github.com/library-squirrel/backend/slot'
import { Events } from '@wailsio/runtime'
import * as WailsRuntime from '@wailsio/runtime'

/**
 * 转换视图插槽配置
 */
function convertToViewSlot(config: SlotResponse): ViewSlot {
  const componentLoader = () => loadPluginComponent(config.contentType, config.content as AnySlotContent, config.pluginPublicId)
  return {
    slotId: config.slotId,
    name: config.name,
    component: componentLoader,
    order: config.order ?? 100,
    isPlugin: true,
    props: config.props as Record<string, unknown> | undefined
  }
}

/**
 * 转换嵌入插槽配置
 */
function convertToEmbedSlot(config: SlotResponse): EmbedSlot {
  return {
    slotId: config.slotId,
    position: config.position as 'topbar' | 'statusbar' | 'toolbar' | 'dialog',
    component: () => loadPluginComponent(config.contentType, config.content as AnySlotContent, config.pluginPublicId),
    props: config.props as Record<string, unknown> | undefined,
    order: config.order ?? 100
  }
}

/**
 * 转换面板插槽配置
 */
function convertToPanelSlot(config: SlotResponse): PanelSlot {
  return {
    slotId: config.slotId,
    position: config.position as 'left-sidebar' | 'right-sidebar' | 'bottom',
    width: config.width ?? undefined,
    height: config.height ?? undefined,
    component: () => loadPluginComponent(config.contentType, config.content as AnySlotContent, config.pluginPublicId),
    props: config.props as Record<string, unknown> | undefined,
    order: config.order ?? 100
  }
}

/**
 * 转换菜单插槽配置
 */
function convertToMenuSlot(config: SlotResponse): MenuSlotItem {
  // 递归转换子菜单
  const convertChildren = (children?: SlotResponse[]): MenuSlotItem[] | undefined => {
    if (!children || children.length === 0) return undefined
    return children.map((child) => ({
      slotId: child.slotId,
      index: child.pluginPublicId,
      label: child.name,
      icon: child.icon,
      order: child.order ?? 100,
      viewId: child.viewId,
      children: convertChildren(child.children)
    }))
  }

  return {
    slotId: config.slotId,
    index: config.pluginPublicId,
    label: config.name,
    icon: config.icon,
    order: config.order ?? 100,
    viewId: config.viewId,
    children: convertChildren(config.children)
  }
}

/**
 * 转换站点浏览器列表插槽配置
 */
function convertToSiteBrowserListSlot(config: SlotResponse): SiteBrowserListSlotItem {
  return {
    slotId: config.slotId,
    pluginId: config.pluginId,
    pluginPublicId: config.pluginPublicId ?? '',
    name: config.name,
    order: config.order ?? 100,
    contributionId: config.contributionId ?? '',
    icon: config.icon ?? ''
  }
}

/**
 * 加载插件CSS样式
 * @param pluginPublicId 插件ID，用于标识CSS
 * @param cssPath CSS文件路径
 */
async function loadPluginStyles(pluginPublicId: string, cssPath: string): Promise<void> {
  // 检查CSS是否已加载
  const existingLink = document.querySelector(`link[data-plugin-id="${pluginPublicId}"][data-css-path="${cssPath}"]`)
  if (existingLink) {
    return
  }

  return new Promise((resolve, reject) => {
    const link = document.createElement('link')
    link.rel = 'stylesheet'
    link.type = 'text/css'
    link.href = cssPath
    link.setAttribute('data-plugin-id', pluginPublicId)
    link.setAttribute('data-css-path', cssPath)

    link.onload = () => {
      resolve()
    }
    link.onerror = () => {
      reject(new Error(`CSS加载失败: ${cssPath}`))
    }

    document.head.appendChild(link)
  })
}

/**
 * 卸载插件CSS样式
 * @param pluginId 插件ID
 */
function unloadPluginStyles(pluginId: number): void {
  const links = document.querySelectorAll(`link[data-plugin-id="${pluginId}"]`)
  links.forEach((link) => {
    link.remove()
  })
}

/**
 * 加载预编译的插件组件
 * 路径已经是完整的 URL（由后端 resolveContentURLs 生成）
 * @param jsUrl 编译后的 JS 文件 URL
 * @param cssUrl 编译后的 CSS 文件 URL（可选）
 * @param pluginPublicId 插件公开ID
 */
async function loadCompiledComponent(jsUrl: string, cssUrl: string | undefined, pluginPublicId: string): Promise<DefineComponent> {
  // 如果有CSS文件，先加载CSS
  if (cssUrl) {
    await loadPluginStyles(pluginPublicId, cssUrl)
  }

  // 动态导入 JS 文件获取组件
  try {
    const module = await import(/* @vite-ignore */ jsUrl)
    return defineComponent(module.default(Vue, WailsRuntime) as Record<string, unknown>)
  } catch (error) {
    console.error('加载编译后的组件失败:', error)
    throw new Error(`加载编译后的组件失败: ${error}`)
  }
}

/**
 * 根据内容类型加载插件组件
 * @param contentType 内容类型
 * @param content 内容(字符串或对象)
 * @param pluginPublicId 插件公开ID
 */
async function loadPluginComponent(contentType: string, content: AnySlotContent, pluginPublicId: string): Promise<DefineComponent> {
  // Vue源码加载 - 优先使用预编译的缓存文件
  if (contentType === 'vueSource') {
    const compiledJsPath = (content as VueSourceContent).js
    // 如果有编译后的缓存文件，直接加载
    if (compiledJsPath) {
      const compiledCssPath = (content as VueSourceContent).css
      return loadCompiledComponent(compiledJsPath, compiledCssPath, pluginPublicId)
    }

    // 否则fallback到运行时编译
    return loadVueSourceComponent((content as VueSourceContent).vue, pluginPublicId)
  }

  // 预编译组件加载 - 加载JS/CSS
  if (contentType === 'precompiled') {
    const precompiledContent = content as PrecompiledContent
    return loadCompiledComponent(precompiledContent.js, precompiledContent.css, pluginPublicId)
  }

  // 代码片段加载
  if (contentType === 'code') {
    // 代码片段: 执行代码并返回 Vue 组件
    // content对于code类型应该是字符串
    let codeContent: string
    if (typeof content === 'string') {
      codeContent = content
    } else {
      throw new Error('code 类型的内容需要提供代码片段')
    }
    return createCodeComponent(codeContent)
  }

  // HTML 文件加载
  if (contentType === 'html') {
    const htmlContent = content as HtmlContent
    return createHtmlComponent(htmlContent.html)
  }

  throw new Error(`未知的内容类型: ${contentType}`)
}

/**
 * 创建代码片段组件
 * 注意: 代码片段需要返回一个 Vue 组件定义
 */
function createCodeComponent(code: string): Promise<DefineComponent> {
  return new Promise((resolve, reject) => {
    try {
      // 使用 Function 构造函数创建组件工厂函数
      // 注意: 这需要在渲染进程的安全上下文中执行
      const componentFactory = new Function('return ' + code)()
      resolve(componentFactory)
    } catch (error) {
      reject(new Error(`代码片段执行失败: ${error}`))
    }
  })
}

/**
 * 创建 HTML 组件
 * htmlPath 已经是完整的 URL（由后端 resolveContentURLs 生成）
 * @param htmlPath HTML 文件 URL
 */
async function createHtmlComponent(htmlPath: string): Promise<DefineComponent> {
  const response = await fetch(htmlPath)
  if (!response.ok) {
    throw new Error(`加载HTML文件失败: HTTP ${response.status}`)
  }
  const html = await response.text()
  return defineComponent({
    template: html
  })
}

/**
 * 加载并编译 Vue 源码
 * vuePath 已经是完整的 URL（由后端 resolveContentURLs 生成）
 * @param vuePath Vue 文件 URL
 * @param pluginPublicId 插件公开ID
 */
async function loadVueSourceComponent(vuePath: string, pluginPublicId: string): Promise<DefineComponent> {
  try {
    // 阶段 1: 文件获取 - 通过 URL 获取 .vue 文件内容
    const response = await fetch(vuePath)
    if (!response.ok) {
      throw new Error(`加载Vue源码失败: HTTP ${response.status}`)
    }
    const sourceCode = await response.text()
    if (!sourceCode) {
      throw new Error(`加载Vue源码失败，返回内容为空`)
    }

    // 阶段 2: SFC 解析 - 解析 Vue 单文件组件
    const parseResult = parse(sourceCode)
    const errors = parseResult.errors as unknown[] | undefined
    if (errors && errors.length > 0) {
      throw new Error(`SFC 解析错误: ${errors.map((e) => String(e)).join(', ')}`)
    }

    const descriptor = parseResult.descriptor
    if (!descriptor) {
      throw new Error('无法解析 Vue 文件')
    }

    // 阶段 3: 模板编译 - 使用运行时编译器生成 render 函数
    if (isNullish(descriptor.template)) {
      throw new Error('未解析出 Vue 文件的模板')
    }
    const renderFunction: ((...args: unknown[]) => unknown) | undefined = compile(descriptor.template.content, {
      mode: 'function'
    })

    // 阶段 4: 脚本执行 - 执行 script 获取组件选项
    let componentOptions: Record<string, unknown> = {}
    if (descriptor.script) {
      let scriptCode = descriptor.script.content
      // 移除所有 import 语句，因为 eval/new Function 不能执行 import
      scriptCode = scriptCode.replace(/^import\s+.*from\s+['"].*['"];?\s*$/gm, '')
      // 移除独立的 import 语句（没有 from 的情况）
      scriptCode = scriptCode.replace(/^import\s+['"].*['"];?\s*$/gm, '')
      // 移除 export default
      scriptCode = scriptCode.replace(/export\s+default\s+/, '').replace(/^export\s+default\s+/, '')
      // 使用 eval 执行，因为 new Function 不能执行包含 const/let 的代码
      try {
        // 使用 with 或者直接返回对象字面量
        const result = eval(`(function() { return (${scriptCode}) })()`)
        componentOptions = result || {}
      } catch (error) {
        console.error('Script 执行错误:', error)
        throw new Error(`Script 执行失败: ${error}`)
      }
    } else if (descriptor.scriptSetup) {
      // 处理 script setup
      const scriptSetupContent = descriptor.scriptSetup.content
      // 对于 script setup，需要额外处理 - 这里简化处理
      // 实际项目中可能需要使用 @vue/compiler-sfc 的 compileScript 方法
      try {
        componentOptions = eval(`(({ setup: () => { ${scriptSetupContent} } }))`) || {}
      } catch (error) {
        console.error('Script Setup 执行错误:', error)
        throw new Error(`Script Setup 执行失败: ${error}`)
      }
    }

    // 阶段 5: 样式注入 - 处理 scoped 样式
    if (descriptor.styles && descriptor.styles.length > 0) {
      for (const style of descriptor.styles) {
        const isScoped = style.scoped
        let cssContent = style.content

        if (isScoped) {
          // 使用 compilerSFC 生成的 CSS scope ID
          // 由于运行时编译，我们生成一个简单的 scope ID
          const scopeId = `data-v-${pluginPublicId}-${Math.random().toString(36).slice(2, 8)}`
          // 为 scoped 样式添加属性选择器
          // 注意：这里简化处理，实际需要更复杂的 AST 转换
          cssContent = cssContent.replace(/([^}]+)\s*\{/g, (_, selector) => {
            // 避免重复添加属性选择器
            if (selector.includes(scopeId)) {
              return `${selector} {`
            }
            // 为选择器添加 scoped 属性
            const modifiedSelector = selector
              .split(',')
              .map((s: string) => {
                const trimmed = s.trim()
                if (trimmed.startsWith('.') || trimmed.startsWith('#') || trimmed.startsWith('[')) {
                  return `${trimmed}[${scopeId}]`
                }
                return `${trimmed}[${scopeId}]`
              })
              .join(', ')
            return `${modifiedSelector} {`
          })
        }

        // 注入 CSS 到页面
        injectStyle(cssContent, pluginPublicId, isScoped ? String(pluginPublicId) : undefined)
      }
    }

    // 阶段 6: 组件组装
    const componentDef: Record<string, unknown> = {
      ...componentOptions
    }

    if (renderFunction) {
      componentDef.render = renderFunction
      delete componentDef.template
    }

    return defineComponent(componentDef)
  } catch (error) {
    console.error('Vue 源码编译失败:', error)
    throw error
  }
}

/**
 * 动态注入 CSS 样式
 */
function injectStyle(css: string, pluginPublicId: string, scopeId?: string): void {
  const styleId = `plugin-style-${pluginPublicId}${scopeId ? `-${scopeId}` : ''}`
  const existingStyle = document.getElementById(styleId)
  if (existingStyle) {
    return
  }

  const styleElement = document.createElement('style')
  styleElement.id = styleId
  styleElement.textContent = css
  document.head.appendChild(styleElement)
}

/**
 * 根据类型注册 slot 到 store
 */
function registerSlotByType(store: ReturnType<typeof useSlotRegistryStore>, slot: SlotResponse) {
  if (slot.type === 'view') {
    store.registerViewSlot(convertToViewSlot(slot))
  } else if (slot.type === 'menu') {
    store.registerMenuSlot(convertToMenuSlot(slot))
  } else if (slot.type === 'embed') {
    store.registerEmbedSlot(convertToEmbedSlot(slot))
  } else if (slot.type === 'panel') {
    store.registerPanelSlot(convertToPanelSlot(slot))
  } else if (slot.type === 'siteBrowserList') {
    store.registerSiteBrowserSlot(convertToSiteBrowserListSlot(slot))
  }
}

/**
 * 根据 slotType 注销 slot
 */
function unregisterSlotByType(store: ReturnType<typeof useSlotRegistryStore>, slotId: string, slotType: string) {
  if (slotType === 'view') {
    store.unregisterViewSlot(slotId)
  } else if (slotType === 'menu') {
    store.unregisterMenuSlot(slotId)
  } else if (slotType === 'embed') {
    store.unregisterEmbedSlot(slotId)
  } else if (slotType === 'panel') {
    store.unregisterPanelSlot(slotId)
  } else if (slotType === 'siteBrowserList') {
    store.unregisterSiteBrowserSlot(slotId)
  }
}

/**
 * 初始化插槽同步监听器
 */
export function initSlotSyncListener() {
  const store = useSlotRegistryStore()

  // 初始同步：获取所有已注册的插槽
  SlotHandler.GetAllSlots().then((resp) => {
    const slots = resp?.data ?? []
    slots.forEach((config: unknown) => {
      registerSlotByType(store, config as SlotResponse)
    })
  })

  // 监听运行时 slot 注册事件
  Events.On('slot-register', (event: unknown) => {
    const data = (event as { data: { slotId: string; data: SlotResponse } }).data
    if (data?.data) {
      registerSlotByType(store, data.data)
    }
  })

  // 监听运行时 slot 注销事件
  Events.On('slot-unregister', (event: unknown) => {
    const data = (event as { data: { slotId: string; slotType: string } }).data
    if (data?.slotId && data?.slotType) {
      unregisterSlotByType(store, data.slotId, data.slotType)
    }
  })

  // 监听运行时 slot 批量注销事件
  Events.On('slot-batch-register', (event: unknown) => {
    const data = (event as { data: { slots: Array<{ slotId: string; slotType: string }> } }).data
    if (data?.slots) {
      data.slots.forEach((item) => {
        unregisterSlotByType(store, item.slotId, item.slotType)
      })
    }
  })
}
