/**
 * 判断值是否不为 null 或 undefined
 */
export function notNullish<T>(value: T | undefined | null): value is T {
  return value !== undefined && value !== null
}

/**
 * 判断值是否为 null 或 undefined
 */
export function isNullish(value: unknown): value is undefined | null {
  return !notNullish(value)
}

/**
 * 判断数组是否不为空
 */
export function arrayNotEmpty(value: unknown): value is unknown[] {
  return notNullish(value) && Array.isArray(value) && value.length > 0
}

/**
 * 判断数组是否为空
 */
export function arrayIsEmpty(value: unknown): value is null | undefined | [] {
  return !arrayNotEmpty(value)
}
