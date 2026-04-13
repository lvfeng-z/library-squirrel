import * as ElementPlusIconsVue from '@element-plus/icons-vue'

export const elementIconRegister = (app) => {
  // 全局注册图标
  Object.values(ElementPlusIconsVue).forEach((component) => {
    app.component(component.name, component)
  })
}
