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
  const connectionState = ref('connecting')
  const user = ref(null)
  const workspace = ref(null)
  const workspaces = ref([])
  const sessions = ref([])
  const runs = ref([])

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
    const index = runs.value.findIndex(item => item.id === run.id)
    if (index >= 0) {
      runs.value.splice(index, 1, { ...runs.value[index], ...run })
      return
    }
    runs.value.unshift(run)
  }

  function setRuns(items) {
    runs.value = items || []
  }

  function setConnectionState(state) {
    connectionState.value = state
  }

  function startRun(runId) {
    activeRunId.value = runId
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
    sessionId.value = ''
    runs.value = []
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
  }

  return {
    messages,
    reportHTML,
    isRunning,
    uploadedFiles,
    token,
    sessionId,
    activeRunId,
    connectionState,
    user,
    workspace,
    workspaces,
    sessions,
    runs,
    addMessage,
    updateReport,
    setRunning,
    setSession,
    setIdentity,
    setWorkspace,
    setToken,
    setWorkspaces,
    setSessions,
    upsertSession,
    setRuns,
    upsertRun,
    setConnectionState,
    startRun,
    finishRun,
    addFile,
    replaceFiles,
    resetAnalysis,
    logout,
  }
})
