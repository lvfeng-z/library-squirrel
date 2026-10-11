import { defineConfig } from 'vitest/config'
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
    environment: 'jsdom'
  }
})
