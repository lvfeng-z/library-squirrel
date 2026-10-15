import { describe, expect, it, vi } from 'vitest'
import { MessageBox } from '@element-plus/icons-vue'
import { resolvePluginIcon } from '@renderer/utils/PluginIcon.ts'

describe('resolvePluginIcon', () => {
  it('ep: 名字引用命中注册表时返回对应 Element Plus 图标组件', () => {
    expect(resolvePluginIcon('ep:MessageBox')).toBe(MessageBox)
  })

  it('ep: 名字引用未命中注册表时告警并原样返回（走图片兜底路径）', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    expect(resolvePluginIcon('ep:NoSuchIcon')).toBe('ep:NoSuchIcon')
    expect(warn).toHaveBeenCalledOnce()
    warn.mockRestore()
  })

  it('无前缀的图片 URL / 相对路径原样返回', () => {
    expect(resolvePluginIcon('/plugin/com.example/k/assets/icon.png')).toBe('/plugin/com.example/k/assets/icon.png')
    expect(resolvePluginIcon('assets/icon.png')).toBe('assets/icon.png')
  })

  it('空值返回 undefined', () => {
    expect(resolvePluginIcon(undefined)).toBeUndefined()
    expect(resolvePluginIcon('')).toBeUndefined()
  })
})
