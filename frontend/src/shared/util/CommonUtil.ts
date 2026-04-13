// Common utility functions

export function isNullish(value: any): boolean {
  return value === null || value === undefined;
}

export function notNullish(value: any): boolean {
  return value !== null && value !== undefined;
}

export function arrayNotEmpty<T>(arr: T[]): boolean {
  return arr !== null && arr !== undefined && arr.length > 0;
}
