import {Settings} from "@bindings/github.com/library-squirrel/backend/settings";

export const emptySettings: Settings = {
  initialized: false,
  workdir: '',
  workSettings: {
    fileNameFormat: '[${author}]_[${siteWorkId}]_${siteWorkName}'
  },
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
    theme: 'default-light'
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
    outputDir: ''
  }
}
