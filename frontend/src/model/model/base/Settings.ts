export interface Settings {
  initialized: boolean
  programVersion: string
  workdir: string
  workSettings: {
    fileNameFormat: string
  }
  importSettings: {
    maxParallelImport: number
    updateWorkInfoWhenImport: boolean
  }
  pluginSettings: {
    allowUnsafeEval: boolean
  }
  tour: {
    firstTimeTourPassed: boolean
    workdirTour: boolean
    taskTour: boolean
  }
}

export const emptySettings: Settings = {
  initialized: false,
  programVersion: '',
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
  tour: {
    firstTimeTourPassed: false,
    workdirTour: false,
    taskTour: false
  }
}
