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
    allowUnsafeEval: false
  },
  recycleBin: {
    autoCleanupEnabled: true,
    retentionDays: 30
  },
  tour: {
    completed: {}
  }
}
