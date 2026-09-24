import { configDefaults, defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

// 组件测试配置：与 vite.config.js 同一套别名（测试自 frontend/ 目录发起，resolve 相对 cwd）
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@renderer': resolve('src'),
      '@bindings': resolve('bindings'),
      '@apis': resolve('src/apis')
    }
  },
  test: {
    environment: 'jsdom',
    exclude: [
      ...configDefaults.exclude,
      // SlotRegistryStore.test.ts 是无测试运行器的可执行断言集（导出 runSlotRegistryStoreTests()，
      // 由 live-test 经 dev 动态 import 执行）——vitest glob 误拾取会报「No test suite found」，排除
      'src/store/SlotRegistryStore.test.ts'
    ]
  }
})
