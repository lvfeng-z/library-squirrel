import { ElMessage, ElMessageBox } from 'element-plus'
import { recycleBinApi } from '@renderer/apis/http'

// 后端作品锁哨兵文案（shareLock.ErrWorkLocked）。Wails 只透传错误消息字符串，
// 前端无法 errors.Is，按文案包含匹配（外层 fmt.Errorf 包装保留原文）
const WORK_LOCKED_MESSAGE = '该作品正在被分享拉取中'

/**
 * 作品分享拉取锁的前端交互组合式函数：锁命中识别 + 强制解锁确认回路。
 * 后端在作品软删、回收站复原置换/覆盖转移前置查锁，命中返回上述文案；
 * 用户知情接受「在途拉取可能失败」后强制解锁并重试原操作（锁为防误触软防护，本地资源由本地掌控）。
 */
export function useWorkLockConfirm() {
  // 判断错误消息是否为作品锁命中（抛出型接口的 Error.message 形态）
  function isWorkLockedMessage(message: string | null | undefined): boolean {
    return message != null && message.includes(WORK_LOCKED_MESSAGE)
  }

  // 判断非抛出型接口的失败响应是否为作品锁命中
  function isWorkLockedResponse(response: { success: boolean; msg: string } | null | undefined): boolean {
    return response != null && !response.success && isWorkLockedMessage(response.msg)
  }

  // 锁命中确认框：确认后强制解锁该作品并返回 true；取消返回 false（调用方中止原操作）；
  // 解锁接口自身失败提示错误并返回 false
  async function confirmWorkForceUnlock(workId: number): Promise<boolean> {
    try {
      await ElMessageBox.confirm('该作品正在被分享拉取，强制继续？（在途拉取可能失败）', '作品正在被拉取', {
        confirmButtonText: '强制继续',
        cancelButtonText: '取消',
        type: 'warning'
      })
    } catch {
      return false
    }
    try {
      await recycleBinApi.recycleBinForceUnlockWork(workId)
      return true
    } catch (e) {
      ElMessage.error((e as Error).message ?? '强制解锁失败')
      return false
    }
  }

  return { isWorkLockedMessage, isWorkLockedResponse, confirmWorkForceUnlock }
}
