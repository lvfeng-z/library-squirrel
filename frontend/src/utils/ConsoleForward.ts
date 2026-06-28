/**
 * 前端 console 日志转发：劫持 console 全级别方法 + 捕获未处理异常，
 * 批量缓冲后转发到后端 frontend.log。
 *
 * 忠实复刻 DevTools Console 面板内容：
 * - console.log/info/warn/error/debug/trace（主体，含 Vue/第三方库的 console 调用）
 * - window.onerror（DevTools 的 Uncaught）
 * - unhandledrejection（DevTools 的 Uncaught in promise）
 *
 * 仅在 dev（import.meta.env.DEV）下由 main.ts 调用 setupConsoleForward 接入。
 */

import { frontendLogWrite } from '@renderer/apis/http/wrappers/frontendLog'
import type { FrontendLogEntry } from '@renderer/apis/http/wrappers/frontendLog'

type LogLevel = 'debug' | 'info' | 'warn' | 'error'

const BUFFER_LIMIT = 20
const FLUSH_INTERVAL = 200

let buffer: FrontendLogEntry[] = []
let timer: ReturnType<typeof setInterval> | null = null

// 序列化单个参数：对象尝试 JSON.stringify，循环引用/失败兜底为 String
function serializeArg(arg: unknown): string {
  if (typeof arg === 'string') return arg
  if (arg instanceof Error) return arg.stack || arg.message
  if (typeof arg === 'object' && arg !== null) {
    try {
      return JSON.stringify(arg)
    } catch {
      return String(arg)
    }
  }
  return String(arg)
}

function append(level: LogLevel, args: unknown[]) {
  buffer.push({
    level,
    message: args.map(serializeArg).join(' '),
    timestamp: Date.now()
  })
  if (buffer.length >= BUFFER_LIMIT) flush()
}

function flush() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
  if (buffer.length === 0) return
  const batch = buffer
  buffer = []
  // 日志上报失败静默，不得影响业务
  frontendLogWrite(batch).catch(() => {})
}

/**
 * 劫持 console 全级别方法 + 捕获未处理异常，批量转发到后端 frontend.log。
 * 调用后仍会执行原始 console 方法，DevTools 行为不变。
 */
export function setupConsoleForward() {
  const original = {
    log: console.log,
    info: console.info,
    warn: console.warn,
    error: console.error,
    debug: console.debug,
    trace: console.trace
  }
  const wrap = (level: LogLevel, fn: (...a: unknown[]) => void) =>
    (...args: unknown[]) => {
      append(level, args)
      fn.apply(console, args)
    }

  console.log = wrap('info', original.log)
  console.info = wrap('info', original.info)
  console.warn = wrap('warn', original.warn)
  console.error = wrap('error', original.error)
  console.debug = wrap('debug', original.debug)
  console.trace = wrap('debug', original.trace)

  timer = setInterval(flush, FLUSH_INTERVAL)

  // DevTools 的 Uncaught
  window.addEventListener('error', (e) => {
    const loc = e.filename ? ` ${e.filename}:${e.lineno}:${e.colno}` : ''
    append('error', [e.message + loc])
  })
  // DevTools 的 Uncaught (in promise)
  window.addEventListener('unhandledrejection', (e) => {
    append('error', ['unhandledrejection', e.reason])
  })
  // 页面隐藏/卸载前尽力 flush，减少丢日志
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') flush()
  })
  window.addEventListener('beforeunload', flush)
}
