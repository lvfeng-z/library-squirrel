import type { MenuSlotItem, SiteBrowserListSlotItem } from '@renderer/store/SlotRegistryStore'
import { useSlotRegistryStore } from '@renderer/store/SlotRegistryStore'
import { useHandlerRegistryStore } from '@renderer/store/HandlerRegistryStore'
import type { EmbedSlot, DialogSlot, ReplaceViewSlot, ViewSlot } from '@renderer/model/slot'
import type { ResourceViewerHandler } from '@renderer/model/handler/ResourceViewerHandler'
import { isNullish } from '@renderer/utils/CommonUtil.ts'
import { parse } from '@vue/compiler-sfc'
import { compile, defineComponent, h } from 'vue'
import * as Vue from 'vue'
import type { FrontendExtensionResponse } from '@bindings/github.com/library-squirrel/backend/plugin/extension/models'
import { AnySlotContent } from '@renderer/model/constant/SlotTypes.ts'
import type { HtmlContent, PrecompiledContent, VueSourceContent } from '@renderer/model/interface/SlotConfigs.ts'
import { DefineComponent } from 'vue'
import { FrontendExtensionHandler } from '@bindings/github.com/library-squirrel/backend/plugin/extension'
import { Events } from '@wailsio/runtime'
import * as WailsRuntime from '@wailsio/runtime'
import {isBlank} from "@renderer/utils/StringUtil.ts";
import PluginBoundary from '@renderer/components/common/PluginBoundary.vue'

/**
 * 用 PluginBoundary 包裹插件视图组件，隔离其渲染错误
 * 返回新的 loader：解析出的组件渲染时外层套一层错误边界，
 * 插件视图 render 抛错只降级该视图，不波及主程序
 */
function wrapWithBoundary(loader: () => Promise<DefineComponent>, name: string): () => Promise<DefineComponent> {
  return async () => {
    const child = await loader()
    return defineComponent({
      name: 'PluginViewBoundary',
      setup() {
        // 通过 component prop 让 PluginBoundary 直接挂载子组件，
        // 边界即其 parent，onErrorCaptured 稳定生效
        return () => h(PluginBoundary, { name, component: child })
      }
    }) as DefineComponent
  }
}

/**
 * 转换视图插槽配置
 */
function convertToViewSlot(config: FrontendExtensionResponse): ViewSlot {
  const componentLoader = () => loadPluginComponent(config.contentType, config.content as AnySlotContent, config.pluginPublicId)
  return {
    slotId: config.frontendExtensionId,
    name: config.name,
    component: wrapWithBoundary(componentLoader, config.name),
    order: config.order ?? 100,
    isPlugin: true,
    props: config.props as Record<string, unknown> | undefined
  }
}

/**
 * 转换嵌入插槽配置
 */
function convertToEmbedSlot(config: FrontendExtensionResponse): EmbedSlot {
  if (isBlank(config.position)) {
    throw new Error('转换嵌入插槽配置失败，position不能为空')
  }
  return {
    slotId: config.frontendExtensionId,
    position: config.position,  // 主程序具名插槽位标识
    component: () => loadPluginComponent(config.contentType, config.content as AnySlotContent, config.pluginPublicId),
    props: config.props as Record<string, unknown> | undefined,
    order: config.order ?? 100
  }
}

/**
 * 转换资源渲染器配置（被动响应型扩展，与 slot 正交）
 */
function convertToResourceViewerHandler(config: FrontendExtensionResponse): ResourceViewerHandler {
  if (isBlank(config.resourceType)) {
    throw new Error('转换资源渲染器配置失败，resourceType 不能为空')
  }
  return {
    slotId: config.frontendExtensionId,
    resourceType: config.resourceType,
    component: () => loadPluginComponent(config.contentType, config.content as AnySlotContent, config.pluginPublicId),
    props: config.props as Record<string, unknown> | undefined,
    order: config.order ?? 100
  }
}

/**
 * 转换弹窗插槽配置
 */
function convertToDialogSlot(config: FrontendExtensionResponse): DialogSlot {
  return {
    slotId: config.frontendExtensionId,
    component: () => loadPluginComponent(config.contentType, config.content as AnySlotContent, config.pluginPublicId),
    props: config.props as Record<string, unknown> | undefined,
    order: config.order ?? 100
  }
}

/**
 * 转换替换视图插槽配置
 */
function convertToReplaceViewSlot(config: FrontendExtensionResponse): ReplaceViewSlot {
  if (isBlank(config.target)) {
    throw new Error('转换替换视图插槽配置失败，target不能为空')
  }
  return {
    slotId: config.frontendExtensionId,
    target: config.target,
    component: wrapWithBoundary(() => loadPluginComponent(config.contentType, config.content as AnySlotContent, config.pluginPublicId), config.name),
    props: config.props as Record<string, unknown> | undefined
  }
}

/**
 * 转换菜单插槽配置
 */
function convertToMenuSlot(config: FrontendExtensionResponse): MenuSlotItem {
  // 递归转换子菜单
  const convertChildren = (children?: FrontendExtensionResponse[]): MenuSlotItem[] | undefined => {
    if (!children || children.length === 0) return undefined
    return children.map((child) => ({
      slotId: child.frontendExtensionId,
      index: child.pluginPublicId,
      label: child.name,
      icon: child.icon,
      order: child.order ?? 100,
      viewId: child.viewId,
      children: convertChildren(child.children)
    }))
  }

  return {
    slotId: config.frontendExtensionId,
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
function convertToSiteBrowserListSlot(config: FrontendExtensionResponse): SiteBrowserListSlotItem {
  return {
    slotId: config.frontendExtensionId,
    pluginId: config.pluginId,
    pluginPublicId: config.pluginPublicId ?? '',
    name: config.name,
    order: config.order ?? 100,
    extensionId: config.extensionId ?? '',
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
function registerSlotByType(store: ReturnType<typeof useSlotRegistryStore>, slot: FrontendExtensionResponse) {
  // 被动响应型扩展，路由到独立 HandlerRegistry（不进 SlotRegistryStore）
  if (slot.type === 'resourceViewer') {
    useHandlerRegistryStore().registerResourceViewerHandler(convertToResourceViewerHandler(slot))
    return
  }
  if (slot.type === 'view') {
    store.registerViewSlot(convertToViewSlot(slot))
  } else if (slot.type === 'replaceView') {
    store.registerReplaceViewSlot(convertToReplaceViewSlot(slot))
  } else if (slot.type === 'menu') {
    store.registerMenuSlot(convertToMenuSlot(slot))
  } else if (slot.type === 'embed') {
    store.registerEmbedSlot(convertToEmbedSlot(slot))
  } else if (slot.type === 'dialog') {
    store.registerDialogSlot(convertToDialogSlot(slot))
  } else if (slot.type === 'siteBrowserList') {
    store.registerSiteBrowserSlot(convertToSiteBrowserListSlot(slot))
  }
}

/**
 * 根据 slotType 注销 slot
 */
function unregisterSlotByType(store: ReturnType<typeof useSlotRegistryStore>, slotId: string, slotType: string) {
  if (slotType === 'resourceViewer') {
    useHandlerRegistryStore().unregisterResourceViewerHandler(slotId)
    return
  }
  if (slotType === 'view') {
    store.unregisterViewSlot(slotId)
  } else if (slotType === 'replaceView') {
    store.unregisterReplaceViewSlot(slotId)
  } else if (slotType === 'menu') {
    store.unregisterMenuSlot(slotId)
  } else if (slotType === 'embed') {
    store.unregisterEmbedSlot(slotId)
  } else if (slotType === 'dialog') {
    store.unregisterDialogSlot(slotId)
  } else if (slotType === 'siteBrowserList') {
    store.unregisterSiteBrowserSlot(slotId)
  }
}

/**
 * 初始化插槽同步监听器
 */
export function initSlotSyncListener() {
  const store = useSlotRegistryStore()

  // 初始同步：获取所有已注册的前端扩展
  FrontendExtensionHandler.GetAllFrontendExtensions().then((resp) => {
    const extensions = resp?.data ?? []
    extensions.forEach((config: unknown) => {
      registerSlotByType(store, config as FrontendExtensionResponse)
    })
  })

  // 监听运行时前端扩展注册事件
  Events.On('frontend-extension-register', (event: unknown) => {
    const data = (event as { data: { frontendExtensionId: string; data: FrontendExtensionResponse } }).data
    if (data?.data) {
      registerSlotByType(store, data.data)
    }
  })

  // 监听运行时前端扩展注销事件
  Events.On('frontend-extension-unregister', (event: unknown) => {
    const data = (event as { data: { frontendExtensionId: string; kind: string } }).data
    if (data?.frontendExtensionId && data?.kind) {
      unregisterSlotByType(store, data.frontendExtensionId, data.kind)
    }
  })

  // 监听运行时前端扩展批量注销事件
  Events.On('frontend-extension-batch-unregister', (event: unknown) => {
    const data = (event as { data: { items: Array<{ frontendExtensionId: string; kind: string }> } }).data
    if (data?.items) {
      data.items.forEach((item) => {
        unregisterSlotByType(store, item.frontendExtensionId, item.kind)
      })

      // 含 view/replaceView 时刷新清除已渲染页面与模块缓存（保持当前 URL）
      // useBuiltinMenus 注册路由后会 replace 重新匹配，避免 reload 后路由未注册灰屏
      const affectsRoutes = data.items.some(
        (item) => item.kind === 'view' || item.kind === 'replaceView'
      )
      if (affectsRoutes) {
        window.location.reload()
      }
    }
  })
}
