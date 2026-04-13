import { ElMessage } from 'element-plus'
import ApiResponse from '../model/util/ApiResponse.ts'

function check(response: ApiResponse | undefined): boolean {
  if (response) {
    return !!response?.success
  } else {
    return false
  }
}

function data<T>(response: ApiResponse | undefined): T | undefined {
  if (response) {
    if (response?.data) {
      return response.data as T
    } else {
      return undefined
    }
  } else {
    return undefined
  }
}

function msg(response: ApiResponse | undefined): void {
  if (response === undefined) {
    ElMessage({
      message: '无响应',
      type: 'warning'
    })
    return
  }

  if (response?.success) {
    if (response?.msg) {
      ElMessage({
        message: response?.msg ?? null,
        type: 'success',
        icon: 'SuccessFilled'
      })
    }
  } else {
    if (response?.msg) {
      ElMessage({
        message: response?.msg ?? '未知错误',
        type: 'error'
      })
    }
  }
}

function failedMsg(response: ApiResponse | undefined): void {
  if (response === undefined) {
    ElMessage({
      message: '无响应',
      type: 'warning'
    })
    return
  }

  if (response?.success) {
    if (response?.msg) {
      ElMessage({
        type: 'success',
        duration: 1000
      })
    }
  } else {
    if (response?.msg) {
      ElMessage({
        message: response?.msg ?? '未知错误',
        type: 'error'
      })
    }
  }
}

export default {
  check,
  data,
  msg,
  failedMsg
}
