import { describe, it, expect } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { routes } from './routes'

// 失配 hash catch-all 兜底回归：路由表尾部通配记录把未知路径重定向回主页，
// 消解「router-view 空匹配白屏」类别（动态路由注销后的残留 hash、手改/外部携带的过期路径）
describe('routes 失配路径 catch-all 兜底', () => {
  // 首次导航触发 MainLayout/MainView 懒组件全依赖图的 vite 冷转换，放宽单用例超时
  it('未知路径（单段/注销残留多段）导航后均落主页 mainPage', async () => {
    const router = createRouter({ history: createMemoryHistory(), routes })
    await router.push('/definitely-not-registered')
    expect(router.currentRoute.value.name).toBe('mainPage')
    await router.push('/plugin-vanished/child/page')
    expect(router.currentRoute.value.name).toBe('mainPage')
  }, 30000)

  it('catch-all 不劫持具名路由：解析仍命中具名记录', () => {
    const router = createRouter({ history: createMemoryHistory(), routes })
    const resolved = router.resolve('/pluginManage')
    expect(resolved.name).toBe('pluginManage')
    expect(resolved.matched.some((r) => r.path === '/:pathMatch(.*)*')).toBe(false)
  })

  it('根路径仍解析为主页而非 catch-all', () => {
    const router = createRouter({ history: createMemoryHistory(), routes })
    expect(router.resolve('/').name).toBe('mainPage')
  })
})
