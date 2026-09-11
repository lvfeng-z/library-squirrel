import {Settings} from "@bindings/github.com/library-squirrel/backend/settings/models";

// emptySettings 默认值骨架（与后端 NewSettings/defaultSettings 默认层对齐）：表单初始态与保存对比基准
export const emptySettings: Settings = {
  workdir: '',
  importSettings: {
    maxParallelImport: 3,
    updateWorkInfoWhenImport: true
  },
  pluginSettings: {
    allowUnsafeEval: false,
    restrictedMode: false
  },
  recycleBin: {
    autoCleanupEnabled: true,
    retentionDays: 30
  },
  backupGovernance: {
    retentionDays: 7
  },
  tour: {
    completed: {}
  },
  appearance: {
    theme: 'default-light',
    multiSelectEnabled: false
  },
  mergeSettings: {
    strategy: 'keep'
  },
  fsmonitor: {
    usnEnabled: false,
    suppressEnabled: true,
    autoRepairEnabled: false,
    autoRepairPolicies: {}
  },
  exportSettings: {
    outputDir: '',
    fileNameFormat: '[${author}]_[${siteWorkId}]_${siteWorkName}'
  },
  shareSettings: {
    relayAddress: 'relay.library-squirrel.cn'
  }
}
