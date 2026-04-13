import { isBlank } from './StringUtil.ts'
import { arrayIsEmpty, isNullish } from './CommonUtil.ts'

/**
 * 断言为 true
 */
export function assertTrue(value: boolean, msg?: string): void {
  if (!value) {
    throw new Error(msg)
  }
}

/**
 * 断言为 false
 */
export function assertFalse(value: boolean, msg?: string): void {
  if (value) {
    throw new Error(msg)
  }
}

/**
 * 断言值不为 null 或 undefined
 */
export function assertNotNullish<T>(value: T | null | undefined, msg?: string): asserts value {
  if (isNullish(value)) {
    throw new Error(msg)
  }
}

/**
 * 断言数组不为空
 */
export function assertArrayNotEmpty(value: unknown, msg?: string): asserts value {
  if (arrayIsEmpty(value)) {
    throw new Error(msg)
  }
}

/**
 * 断言字符串不为空白
 */
export function assertNotBlank(value: string | undefined | null, msg?: string): asserts value {
  if (isBlank(value)) {
    throw new Error(msg)
  }
}
