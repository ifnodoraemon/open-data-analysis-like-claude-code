import { ref } from 'vue'
import { useAgentStore } from '../stores/agent'

// 单例 WebSocket
let wsInstance = null
const connected = ref(false)

export function useWebSocket() {
  const store = useAgentStore()

  function connect() {
    if (wsInstance && wsInstance.readyState === WebSocket.OPEN) return

    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const url = `${protocol}//${location.host}/ws`
    wsInstance = new WebSocket(url)

    wsInstance.onopen = () => {
      connected.value = true
      console.log('WebSocket 已连接')
    }

    wsInstance.onmessage = (event) => {
      const data = JSON.parse(event.data)
      handleEvent(data, store)
    }

    wsInstance.onclose = () => {
      connected.value = false
      wsInstance = null
      store.setRunning(false)
      console.log('WebSocket 断开，3 秒后重连...')
      setTimeout(connect, 3000)
    }

    wsInstance.onerror = (err) => {
      console.error('WebSocket 错误:', err)
    }
  }

  function handleEvent(event, store) {
    switch (event.type) {
      case 'thinking':
        store.addMessage({ type: 'thinking', content: event.data.content })
        break
      case 'tool_call':
        store.addMessage({
          type: 'tool_call',
          name: event.data.name,
          arguments: event.data.arguments,
          id: event.data.id,
        })
        break
      case 'tool_result':
        store.addMessage({
          type: 'tool_result',
          name: event.data.name,
          result: event.data.result,
          duration: event.data.duration,
          success: event.data.success,
          id: event.data.id,
        })
        break
      case 'report_update':
        store.updateReport(event.data.html)
        break
      case 'report_final':
        store.updateReport(event.data.html)
        store.addMessage({ type: 'complete', content: '✅ 研究报告已生成完成，可点击右上角导出。' })
        store.setRunning(false)
        break
      case 'complete':
        store.addMessage({ type: 'complete', content: event.data.summary })
        store.setRunning(false)
        break
      case 'error':
        store.addMessage({ type: 'error', content: event.data.message })
        store.setRunning(false)
        break
    }
  }

  function send(type, data) {
    if (wsInstance?.readyState === WebSocket.OPEN) {
      wsInstance.send(JSON.stringify({ type, data }))
    }
  }

  function sendMessage(content) {
    store.setRunning(true)
    store.addMessage({ type: 'user', content })
    send('user_message', { content, files: store.uploadedFiles.map(f => f.name) })
  }

  function stop() {
    send('stop', {})
    store.setRunning(false)
  }

  return { connected, connect, sendMessage, stop }
}
