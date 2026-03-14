import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useAgentStore = defineStore('agent', () => {
  const messages = ref([])
  const reportHTML = ref('')
  const isRunning = ref(false)
  const uploadedFiles = ref([])
  const token = ref(localStorage.getItem('oda_token') || '')
  const sessionId = ref('')
  const activeRunId = ref('')
  const selectedRunId = ref('')
  const connectionState = ref('disconnected')
  const bootstrapState = ref('idle')
  const bootstrapError = ref('')
  const user = ref(null)
  const workspace = ref(null)
  const workspaces = ref([])
  const sessions = ref([])
  const runs = ref([])
  const subgoals = ref([])
  const memoryFacts = ref({})

  function findRunById(items, runId) {
    for (const item of items || []) {
      if (item.id === runId) return item
      const nested = findRunById(item.childRuns || [], runId)
      if (nested) return nested
    }
    return null
  }

  function patchRunInTree(items, runId, patch) {
    return (items || []).map(item => {
      if (item.id === runId) {
        return { ...item, ...patch }
      }
      if (item.childRuns?.length) {
        return { ...item, childRuns: patchRunInTree(item.childRuns, runId, patch) }
      }
      return item
    })
  }

  function replaceRunInTree(items, nextRun) {
    return (items || []).map(item => {
      if (item.id === nextRun.id) {
        return { ...item, ...nextRun }
      }
      if (item.childRuns?.length) {
        return { ...item, childRuns: replaceRunInTree(item.childRuns, nextRun) }
      }
      return item
    })
  }

  function setChildRunsInTree(items, parentRunId, childRuns) {
    return (items || []).map(item => {
      if (item.id === parentRunId) {
        return { ...item, childRuns: childRuns || [] }
      }
      if (item.childRuns?.length) {
        return { ...item, childRuns: setChildRunsInTree(item.childRuns, parentRunId, childRuns) }
      }
      return item
    })
  }

  function insertRunUnderParent(items, parentRunId, run) {
    return (items || []).map(item => {
      if (item.id === parentRunId) {
        const existingChildren = item.childRuns || []
        const nextChildren = [...existingChildren, run]
        return { ...item, childRuns: nextChildren }
      }
      if (item.childRuns?.length) {
        return { ...item, childRuns: insertRunUnderParent(item.childRuns, parentRunId, run) }
      }
      return item
    })
  }

  function addMessage(msg) {
    messages.value.push({
      ...msg,
      id: Date.now() + Math.random(),
      timestamp: new Date().toLocaleTimeString(),
    })
  }

  function updateReport(html) {
    reportHTML.value = html
  }

  function setRunning(val) {
    isRunning.value = val
  }

  function setSession(id) {
    sessionId.value = id
  }

  function setSelectedRun(runId) {
    selectedRunId.value = runId || ''
  }

  function setIdentity(nextUser, nextWorkspace) {
    user.value = nextUser
    workspace.value = nextWorkspace
  }

  function setWorkspace(nextWorkspace) {
    workspace.value = nextWorkspace
  }

  function setToken(nextToken) {
    token.value = nextToken
    if (nextToken) {
      localStorage.setItem('oda_token', nextToken)
    } else {
      localStorage.removeItem('oda_token')
    }
  }

  function setWorkspaces(items) {
    workspaces.value = items || []
  }

  function setSessions(items) {
    sessions.value = items || []
  }

  function upsertSession(session) {
    if (!session?.id) return
    const index = sessions.value.findIndex(item => item.id === session.id)
    if (index >= 0) {
      sessions.value.splice(index, 1, { ...sessions.value[index], ...session })
      return
    }
    sessions.value.unshift(session)
  }

  function upsertRun(run) {
    if (!run?.id) return
    if (findRunById(runs.value, run.id)) {
      runs.value = replaceRunInTree(runs.value, run)
      return
    }
    if (run.parentRunId && findRunById(runs.value, run.parentRunId)) {
      runs.value = insertRunUnderParent(runs.value, run.parentRunId, run)
      return
    }
    runs.value.unshift(run)
  }

  function setRuns(items) {
    runs.value = items || []
  }

  function setRunChildren(parentRunId, items) {
    if (!parentRunId) return
    runs.value = setChildRunsInTree(runs.value, parentRunId, items || [])
  }

  function patchRun(runId, patch) {
    if (!runId) return false
    if (!findRunById(runs.value, runId)) return false
    runs.value = patchRunInTree(runs.value, runId, patch)
    return true
  }

  function getRun(runId) {
    if (!runId) return null
    return findRunById(runs.value, runId)
  }

  function setMessages(items) {
    messages.value = items || []
  }

  function setSubgoals(items) {
    subgoals.value = items || []
  }

  function setMemoryFacts(items) {
    memoryFacts.value = items || {}
  }

  function setConnectionState(state) {
    connectionState.value = state
  }

  function setBootstrapState(state, error = '') {
    bootstrapState.value = state
    bootstrapError.value = error
  }

  function startRun(runId) {
    activeRunId.value = runId
    selectedRunId.value = runId
    isRunning.value = true
  }

  function finishRun(runId = '') {
    if (!runId || !activeRunId.value || activeRunId.value === runId) {
      activeRunId.value = ''
      isRunning.value = false
    }
  }

  function addFile(file) {
    const existing = uploadedFiles.value.findIndex(item => item.fileId === file.fileId)
    if (existing >= 0) {
      uploadedFiles.value.splice(existing, 1, file)
      return
    }
    uploadedFiles.value.push(file)
  }

  function replaceFiles(files) {
    uploadedFiles.value = files
  }

  function resetAnalysis({ keepFiles = true } = {}) {
    messages.value = []
    reportHTML.value = ''
    isRunning.value = false
    activeRunId.value = ''
    selectedRunId.value = ''
    sessionId.value = ''
    runs.value = []
    subgoals.value = []
    memoryFacts.value = {}
    if (!keepFiles) {
      uploadedFiles.value = []
    }
  }

  function logout() {
    setToken('')
    user.value = null
    workspace.value = null
    workspaces.value = []
    sessions.value = []
    resetAnalysis({ keepFiles: false })
    connectionState.value = 'disconnected'
    bootstrapState.value = 'idle'
    bootstrapError.value = ''
  }

  return {
    messages,
    reportHTML,
    isRunning,
    uploadedFiles,
    token,
    sessionId,
    activeRunId,
    selectedRunId,
    connectionState,
    bootstrapState,
    bootstrapError,
    user,
    workspace,
    workspaces,
    sessions,
    runs,
    subgoals,
    memoryFacts,
    addMessage,
    updateReport,
    setRunning,
    setSession,
    setSelectedRun,
    setIdentity,
    setWorkspace,
    setToken,
    setWorkspaces,
    setSessions,
    upsertSession,
    setRuns,
    setRunChildren,
    upsertRun,
    patchRun,
    getRun,
    setMessages,
    setSubgoals,
    setMemoryFacts,
    setConnectionState,
    setBootstrapState,
    startRun,
    finishRun,
    addFile,
    replaceFiles,
    resetAnalysis,
    logout,
  }
})
