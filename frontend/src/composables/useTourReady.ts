import { onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useTourCenterStore, reportTourReady } from '@renderer/store/UseTourCenterStore'
import type { TourContext } from '@renderer/model/tour/TourDefinition'

/**
 * 向导就绪协议。
 *
 * 进入页面时若向导运行中，读取上下文据 data 定位数据，完成后报告就绪，
 * 引擎收到信号后才显示该步气泡。
 *
 * @param onLocate 数据定位回调（页面内据 ctx.data 判断是否与本页相关并执行定位，如查询并滚动到指定数据）
 */
export function useTourReady(onLocate?: (ctx: TourContext) => void | Promise<void>) {
  const store = useTourCenterStore()
  const { status, context } = storeToRefs(store)

  onMounted(async () => {
    if (status.value !== 'running' || !context.value) return
    if (onLocate) {
      try {
        await onLocate(context.value)
      } catch {
        // 定位失败不阻塞向导
      }
    }
    reportTourReady()
  })

  return { context }
}
