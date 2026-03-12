import { ref } from 'vue'
import { useAgentStore } from '../stores/agent'

// 单例 WebSocket
let wsInstance = null
let reconnectTimer = null
let connectPromise = null
let bootstrapPromise = null
let reconnectEnabled = false
const connected = ref(false)

export function useWebSocket() {
  const store = useAgentStore()

  function authHeaders() {
    return store.token ? { Authorization: `Bearer ${store.token}` } : {}
  }

  function clearReconnectTimer() {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }

  async function loadRunReport(runId) {
    if (!runId) return
    const res = await fetch(`/api/runs/${runId}/report`, {
      headers: authHeaders(),
    })
    if (!res.ok) {
      if (res.status !== 404) {
        throw new Error(await res.text())
      }
      return
    }
    const html = await res.text()
    store.updateReport(html)
  }

  async function tryLoadRunReport(runId) {
    try {
      await loadRunReport(runId)
    } catch (err) {
      console.warn(`load run report failed for ${runId}:`, err)
    }
  }

  function applySessionState(sessionId, files, runs) {
    store.resetAnalysis({ keepFiles: false })
    store.setSession(sessionId || '')
    store.replaceFiles(files || [])
    store.setRuns(runs || [])

    const latestRun = (runs || [])[0]
    store.setSelectedRun(latestRun?.id || '')
    if (latestRun?.status === 'running') {
      store.startRun(latestRun.id)
    } else {
      store.finishRun()
      store.setSelectedRun(latestRun?.id || '')
    }
    return latestRun
  }

  function deriveSessionTitle(input) {
    const value = String(input || '').trim().replace(/\s+/g, ' ')
    if (!value) return '未命名分析'
    return value.length > 28 ? `${value.slice(0, 28)}...` : value
  }

  function restoreBootstrapState(data) {
    const nextSessionId = data.session?.id || ''
    store.setSessions(data.sessions || [])
    return applySessionState(nextSessionId, data.files || [], data.runs || [])
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
    let latestRun = restoreBootstrapState(data)
    if (!data.session?.id) {
      const session = await createSession({ refreshSessions: true })
      latestRun = session?.latestRun || null
    }
    store.updateReport('')
    if (latestRun?.reportFileId) {
      await tryLoadRunReport(latestRun.id)
    }
  }

  async function initializeApp() {
    if (!store.token) {
      throw new Error('未登录')
    }
    if (bootstrapPromise) {
      return bootstrapPromise
    }

    const pending = (async () => {
      store.setBootstrapState('loading')
      try {
        await bootstrap()
        await connect()
        store.setBootstrapState('ready')
      } catch (err) {
        disconnect()
        const message = err instanceof Error ? err.message : '工作区恢复失败'
        store.setBootstrapState('error', message)
        throw err
      } finally {
        if (bootstrapPromise === pending) {
          bootstrapPromise = null
        }
      }
    })()

    bootstrapPromise = pending
    return pending
  }

  async function createSession({ refreshSessions = true } = {}) {
    const res = await fetch('/api/sessions', {
      method: 'POST',
      headers: authHeaders(),
    })
    if (!res.ok) {
      throw new Error(await res.text())
    }
    const data = await res.json()
    if (data.session) {
      store.upsertSession(data.session)
    }
    const latestRun = applySessionState(data.session?.id || '', data.files || [], data.runs || [])
    if (refreshSessions) {
      await loadSessions()
    }
    return { ...data.session, latestRun }
  }

  async function ensureSession() {
    if (store.sessionId) return store.sessionId
    const session = await createSession({ refreshSessions: true })
    if (!session?.id) {
      throw new Error('创建会话失败')
    }
    return session.id
  }

  async function loadSessions() {
    const res = await fetch('/api/sessions', {
      headers: authHeaders(),
    })
    if (!res.ok) {
      throw new Error(await res.text())
    }
    const data = await res.json()
    store.setSessions(data.sessions || [])
    return data.sessions || []
  }

  async function openSession(sessionId) {
    const res = await fetch(`/api/sessions/${encodeURIComponent(sessionId)}`, {
      headers: authHeaders(),
    })
    if (!res.ok) {
      throw new Error(await res.text())
    }
    const data = await res.json()
    disconnect()
    const latestRun = applySessionState(data.session?.id || '', data.files || [], data.runs || [])
    store.updateReport('')
    try {
      if (latestRun?.reportFileId) {
        await tryLoadRunReport(latestRun.id)
      }
    } finally {
      await connect()
    }
  }

  async function openRun(runId) {
    if (!runId) return
    store.setSelectedRun(runId)
    store.updateReport('')
    await loadRunReport(runId)
  }

  async function downloadRunReport(runId) {
    if (!runId) {
      throw new Error('缺少 runId')
    }
    const res = await fetch(`/api/runs/${runId}/report`, {
      headers: authHeaders(),
    })
    if (!res.ok) {
      throw new Error(await res.text())
    }

    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = getDownloadFilename(res.headers.get('Content-Disposition')) || `report-${runId}.html`
    a.click()
    URL.revokeObjectURL(url)
  }

  function connect() {
    if (!store.token) {
      return Promise.reject(new Error('未登录'))
    }
    if (wsInstance?.readyState === WebSocket.OPEN) {
      reconnectEnabled = true
      return Promise.resolve(wsInstance)
    }
    if (wsInstance?.readyState === WebSocket.CONNECTING && connectPromise) {
      reconnectEnabled = true
      return connectPromise
    }

    reconnectEnabled = true
    clearReconnectTimer()
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const params = new URLSearchParams()
    if (store.sessionId) params.set('session_id', store.sessionId)
    if (store.workspace?.id) params.set('workspace_id', store.workspace.id)
    params.set('token', store.token)
    const sessionQuery = params.toString() ? `?${params.toString()}` : ''
    const url = `${protocol}//${location.host}/ws${sessionQuery}`
    const socket = new WebSocket(url)
    wsInstance = socket
    store.setConnectionState('connecting')
    connected.value = false

    const pending = new Promise((resolve, reject) => {
      let settled = false

      function resolveOnce(value) {
        if (settled) return
        settled = true
        if (connectPromise === pending) {
          connectPromise = null
        }
        resolve(value)
      }

      function rejectOnce(error) {
        if (settled) return
        settled = true
        if (connectPromise === pending) {
          connectPromise = null
        }
        reject(error)
      }

      socket.onopen = () => {
        if (wsInstance !== socket) {
          resolveOnce(socket)
          return
        }
        connected.value = true
        store.setConnectionState('connected')
        console.log('WebSocket 已连接')
        resolveOnce(socket)
      }

      socket.onmessage = (event) => {
        if (wsInstance !== socket) {
          return
        }
        const data = JSON.parse(event.data)
        handleEvent(data, store)
      }

      socket.onclose = () => {
        if (wsInstance !== socket) {
          rejectOnce(new Error('连接已被替换'))
          return
        }
        wsInstance = null
        connected.value = false
        rejectOnce(new Error('WebSocket 连接已关闭'))
        if (!store.token || !reconnectEnabled) {
          store.setConnectionState('disconnected')
          return
        }
        store.setConnectionState('reconnecting')
        console.log('WebSocket 断开，3 秒后重连...')
        clearReconnectTimer()
        reconnectTimer = setTimeout(() => {
          void connect().catch((err) => {
            console.error('WebSocket 重连失败:', err)
          })
        }, 3000)
      }

      socket.onerror = (err) => {
        if (wsInstance !== socket) {
          return
        }
        console.error('WebSocket 错误:', err)
      }
    })

    connectPromise = pending
    return pending
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
        store.upsertSession({
          id: event.data.sessionId,
          title: '未命名分析',
          lastSeenAt: new Date().toISOString(),
        })
        break
      case 'session_reset':
        store.resetAnalysis({ keepFiles: event.data.keepFiles })
        store.replaceFiles(event.data.files || [])
        break
      case 'run_started':
        store.startRun(event.data.runId)
        store.upsertRun({
          id: event.data.runId,
          sessionId: store.sessionId,
          status: 'running',
          inputMessage: store.messages.filter(msg => msg.type === 'user').at(-1)?.content || '',
          createdAt: new Date().toISOString(),
        })
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
        if (!store.selectedRunId || store.selectedRunId === event.runId) {
          store.updateReport(event.data.html)
        }
        break
      case 'report_final':
        store.setSelectedRun(event.runId)
        store.updateReport(event.data.html)
        if (event.data.title && store.sessionId) {
          store.upsertSession({
            id: store.sessionId,
            title: event.data.title,
            lastSeenAt: new Date().toISOString(),
          })
        }
        store.addMessage({ type: 'complete', content: '✅ 研究报告已生成完成，可点击右上角导出。' })
        break
      case 'run_completed':
        store.upsertRun({
          id: event.runId,
          status: 'completed',
          summary: event.data.summary,
          updatedAt: new Date().toISOString(),
        })
        store.addMessage({ type: 'complete', content: event.data.summary })
        store.finishRun(event.runId)
        break
      case 'run_cancelled':
        store.upsertRun({
          id: event.runId,
          status: 'cancelled',
          updatedAt: new Date().toISOString(),
        })
        store.addMessage({ type: 'cancelled', content: event.data.message || '任务已取消' })
        store.finishRun(event.runId)
        break
      case 'error':
        if (event.runId) {
          store.upsertRun({
            id: event.runId,
            status: 'failed',
            errorMessage: event.data.message,
            updatedAt: new Date().toISOString(),
          })
        }
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
    store.setSessions([])
    store.setBootstrapState('idle')
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
    store.setSessions([])
    store.setBootstrapState('idle')
    await initializeApp()
  }

  function disconnect() {
    reconnectEnabled = false
    clearReconnectTimer()
    if (wsInstance) {
      const socket = wsInstance
      wsInstance = null
      socket.close()
    }
    connected.value = false
    connectPromise = null
    store.setConnectionState('disconnected')
  }

  async function ensureSocketOpen() {
    if (wsInstance?.readyState === WebSocket.OPEN) {
      return wsInstance
    }
    await connect()
    if (wsInstance?.readyState !== WebSocket.OPEN) {
      throw new Error('连接尚未建立，请稍后重试。')
    }
    return wsInstance
  }

  async function sendMessage(content) {
    const value = String(content || '').trim()
    if (!value) return

    try {
      await ensureSession()
      await ensureSocketOpen()
    } catch (err) {
      const message = err instanceof Error ? err.message : '连接尚未建立，请稍后重试。'
      store.addMessage({ type: 'error', content: message })
      return
    }

    store.setRunning(true)
    store.addMessage({ type: 'user', content: value })
    if (store.sessionId) {
      store.upsertSession({
        id: store.sessionId,
        title: deriveSessionTitle(value),
        lastSeenAt: new Date().toISOString(),
      })
    }
    send('user_message', { content: value })
  }

  function stop() {
    send('stop_run', { runId: store.activeRunId }, store.activeRunId)
  }

  function resetSession(keepFiles = true) {
    send('reset_session', { keepFiles })
  }

  async function createNewSession() {
    disconnect()
    store.resetAnalysis({ keepFiles: false })
    store.updateReport('')
    await createSession({ refreshSessions: true })
    await connect()
  }

  return { connected, bootstrap, initializeApp, connect, login, switchWorkspace, loadSessions, openSession, openRun, downloadRunReport, disconnect, sendMessage, stop, resetSession, createNewSession, ensureSession }
}

function getDownloadFilename(contentDisposition) {
  const value = String(contentDisposition || '')
  const utf8Match = value.match(/filename\*=UTF-8''([^;]+)/i)
  if (utf8Match?.[1]) {
    return decodeURIComponent(utf8Match[1])
  }
  const basicMatch = value.match(/filename="?([^"]+)"?/i)
  return basicMatch?.[1] || ''
}
