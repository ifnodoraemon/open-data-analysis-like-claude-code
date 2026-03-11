import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useAgentStore = defineStore('agent', () => {
  const messages = ref([])
  const reportHTML = ref('')
  const isRunning = ref(false)
  const uploadedFiles = ref([])

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

  function addFile(file) {
    uploadedFiles.value.push(file)
  }

  function clearMessages() {
    messages.value = []
    reportHTML.value = ''
    uploadedFiles.value = []
  }

  return { messages, reportHTML, isRunning, uploadedFiles, addMessage, updateReport, setRunning, addFile, clearMessages }
})
