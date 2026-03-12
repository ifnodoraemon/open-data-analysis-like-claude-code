import { ref } from 'vue'
import { useAgentStore } from '../stores/agent'

// 单例 WebSocket
let wsInstance = null
let reconnectTimer = null
const connected = ref(false)

export function useWebSocket() {
  const store = useAgentStore()

  function authHeaders() {
    return store.token ? { Authorization: `Bearer ${store.token}` } : {}
  }

  async function bootstrap() {
    if (!store.token) {
      throw new Error('未登录')
    }
    const res = await fetch('/api/bootstrap', {
      headers: authHeaders(),
    })
    if (!res.ok) {
      if (res.status === 401) {
        store.logout()
      }
      throw new Error('bootstrap 失败')
    }
    const data = await res.json()
    store.setIdentity(data.user, data.workspace)
    store.setWorkspaces(data.workspaces || [])
  }

  function connect() {
    if (!store.token) return
    if (wsInstance && [WebSocket.OPEN, WebSocket.CONNECTING].includes(wsInstance.readyState)) return

    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const params = new URLSearchParams()
    if (store.sessionId) params.set('session_id', store.sessionId)
    if (store.workspace?.id) params.set('workspace_id', store.workspace.id)
    params.set('token', store.token)
    const sessionQuery = params.toString() ? `?${params.toString()}` : ''
    const url = `${protocol}//${location.host}/ws${sessionQuery}`
    wsInstance = new WebSocket(url)
    store.setConnectionState('connecting')

    wsInstance.onopen = () => {
      connected.value = true
      store.setConnectionState('connected')
      console.log('WebSocket 已连接')
    }

    wsInstance.onmessage = (event) => {
      const data = JSON.parse(event.data)
      handleEvent(data, store)
    }

    wsInstance.onclose = () => {
      connected.value = false
      wsInstance = null
      store.setConnectionState('disconnected')
      if (!store.token) return
      console.log('WebSocket 断开，3 秒后重连...')
      clearTimeout(reconnectTimer)
      reconnectTimer = setTimeout(connect, 3000)
    }

    wsInstance.onerror = (err) => {
      console.error('WebSocket 错误:', err)
    }
  }

  function handleEvent(event, store) {
    if (event.sessionId && store.sessionId && event.sessionId !== store.sessionId) {
      return
    }
    if (event.runId && store.activeRunId && event.runId !== store.activeRunId && event.type !== 'run_started') {
      return
    }

    switch (event.type) {
      case 'session_ready':
        store.setSession(event.data.sessionId)
        store.replaceFiles(event.data.files || [])
        break
      case 'session_reset':
        store.resetAnalysis({ keepFiles: event.data.keepFiles })
        store.replaceFiles(event.data.files || [])
        break
      case 'run_started':
        store.startRun(event.data.runId)
        break
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
        break
      case 'run_completed':
        store.addMessage({ type: 'complete', content: event.data.summary })
        store.finishRun(event.runId)
        break
      case 'run_cancelled':
        store.addMessage({ type: 'cancelled', content: event.data.message || '任务已取消' })
        store.finishRun(event.runId)
        break
      case 'error':
        store.addMessage({ type: 'error', content: event.data.message })
        store.finishRun(event.runId)
        break
    }
  }

  function send(type, data = {}, runId = '') {
    if (wsInstance?.readyState === WebSocket.OPEN) {
      wsInstance.send(JSON.stringify({ type, sessionId: store.sessionId, runId, data }))
    }
  }

  async function login(email, password, workspaceId = '') {
    const res = await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password, workspaceId }),
    })
    if (!res.ok) {
      throw new Error(await res.text())
    }
    const data = await res.json()
    store.setToken(data.token)
    store.setIdentity(data.user, data.workspace)
    store.setWorkspaces(data.workspaces || [])
    store.resetAnalysis({ keepFiles: false })
  }

  async function switchWorkspace(workspaceId) {
    const res = await fetch('/api/auth/switch-workspace', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...authHeaders(),
      },
      body: JSON.stringify({ workspaceId }),
    })
    if (!res.ok) {
      throw new Error(await res.text())
    }
    const data = await res.json()
    disconnect()
    store.setToken(data.token)
    store.setWorkspace(data.workspace)
    store.resetAnalysis({ keepFiles: false })
    await bootstrap()
    connect()
  }

  function disconnect() {
    clearTimeout(reconnectTimer)
    if (wsInstance) {
      wsInstance.close()
      wsInstance = null
    }
    connected.value = false
  }

  function sendMessage(content) {
    if (wsInstance?.readyState !== WebSocket.OPEN) {
      store.addMessage({ type: 'error', content: '连接尚未建立，请稍后重试。' })
      return
    }
    store.setRunning(true)
    store.addMessage({ type: 'user', content })
    send('user_message', { content })
  }

  function stop() {
    send('stop_run', { runId: store.activeRunId }, store.activeRunId)
  }

  function resetSession(keepFiles = true) {
    send('reset_session', { keepFiles })
  }

  return { connected, bootstrap, connect, login, switchWorkspace, disconnect, sendMessage, stop, resetSession }
}
