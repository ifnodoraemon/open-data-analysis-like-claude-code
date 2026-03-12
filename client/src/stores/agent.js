import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useAgentStore = defineStore('agent', () => {
  const messages = ref([])
  const reportHTML = ref('')
  const isRunning = ref(false)
  const uploadedFiles = ref([])
  const sessionId = ref('')
  const activeRunId = ref('')
  const connectionState = ref('connecting')
  const user = ref(null)
  const workspace = ref(null)

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
    if (!keepFiles) {
      uploadedFiles.value = []
    }
  }

  return {
    messages,
    reportHTML,
    isRunning,
    uploadedFiles,
    sessionId,
    activeRunId,
    connectionState,
    user,
    workspace,
    addMessage,
    updateReport,
    setRunning,
    setSession,
    setIdentity,
    setConnectionState,
    startRun,
    finishRun,
    addFile,
    replaceFiles,
    resetAnalysis,
  }
})
