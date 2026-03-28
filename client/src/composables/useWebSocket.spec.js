global.localStorage = {
  getItem: () => null,
  setItem: () => {},
  removeItem: () => {},
  clear: () => {}
}

import { setActivePinia, createPinia } from 'pinia'
import { useWebSocket } from './useWebSocket'
import { useAgentStore } from '../stores/agent'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

describe('useWebSocket deduplication', () => {
  let originalWebSocket

  beforeEach(() => {
    setActivePinia(createPinia())
    originalWebSocket = global.WebSocket
  })

  afterEach(() => {
    global.WebSocket = originalWebSocket
    vi.restoreAllMocks()
  })

  it('deduplicates run_completed event when reportFileId is set by report_final', async () => {
    const store = useAgentStore()
    store.setToken('test-token')
    store.setSession('sess-1')
    
    // Simulate setting up a run
    store.upsertRun({ id: 'run-1', sessionId: 'sess-1', status: 'running' })
    store.setSelectedRun('run-1')
    store.setMessages([]) // Start with empty messages

    global.location = { protocol: 'http:', host: 'localhost' }
    
    global.location = { protocol: 'http:', host: 'localhost' }
    
    // Mock WebSocket
    let activeSocket = null
    class MockWebSocket {
      constructor(url, protocols) {
        this.url = url
        this.protocols = protocols
        this.readyState = 1 // OPEN
        activeSocket = this
        setTimeout(() => {
          this.onopen?.()
        }, 1)
      }
      send() {}
      close() { this.onclose?.() }
      
      triggerMessage(dataObj) {
        if (this.onmessage) {
          this.onmessage({ data: JSON.stringify(dataObj) })
        }
      }
    }
    MockWebSocket.CONNECTING = 0
    MockWebSocket.OPEN = 1
    MockWebSocket.CLOSING = 2
    MockWebSocket.CLOSED = 3
    vi.stubGlobal('WebSocket', MockWebSocket)

    const { connect, disconnect } = useWebSocket()
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({}),
      text: async () => ''
    })

    const wsPromise = connect()
    await new Promise(resolve => setTimeout(resolve, 5)) 

    await wsPromise
    expect(activeSocket).not.toBeNull()

    // Simulate report_final event incoming
    activeSocket.triggerMessage({
      type: 'report_final',
      runId: 'run-1',
      data: {
        html: '<p>report</p>',
        title: 'Report Title',
        reportFileId: 'file-xyz' // The key element of Finding 2 fix
      }
    })

    // Assert that the run object now securely holds the reportFileId
    expect(store.getRun('run-1')?.reportFileId).toBe('file-xyz')
    
    // There should currently be 1 completion message added by report_final
    const msgCountAfterFinal = store.messages.length
    expect(msgCountAfterFinal).toBe(1)
    expect(store.messages[0].content).toContain('✅ 研究报告已生成完成')

    // Simulate run_completed incoming immediately after report_final
    activeSocket.triggerMessage({
      type: 'run_completed',
      runId: 'run-1',
      data: {
        summary: 'Run completed message that should NOT trigger a second message pop'
      }
    })

    // Assert that the messages array length remains unchanged (deduplication succeeded)
    expect(store.messages.length).toBe(msgCountAfterFinal)
    expect(store.getRun('run-1').status).toBe('completed')

    disconnect()
    global.fetch = undefined
  })

  it('still processes lifecycle events for the active run when a different run is selected', async () => {
    const store = useAgentStore()
    store.setToken('test-token')
    store.setSession('sess-1')
    store.startRun('run-root')
    store.upsertRun({ id: 'run-root', sessionId: 'sess-1', status: 'running' })
    store.upsertRun({ id: 'run-child', sessionId: 'sess-1', parentRunId: 'run-root', status: 'running' })
    store.setSelectedRun('run-child')
    store.setMessages([])

    global.location = { protocol: 'http:', host: 'localhost' }

    let activeSocket = null
    class MockWebSocket {
      constructor(url, protocols) {
        this.url = url
        this.protocols = protocols
        this.readyState = 1
        activeSocket = this
        setTimeout(() => {
          this.onopen?.()
        }, 1)
      }
      send() {}
      close() { this.onclose?.() }
      triggerMessage(dataObj) {
        this.onmessage?.({ data: JSON.stringify(dataObj) })
      }
    }
    MockWebSocket.CONNECTING = 0
    MockWebSocket.OPEN = 1
    MockWebSocket.CLOSING = 2
    MockWebSocket.CLOSED = 3
    vi.stubGlobal('WebSocket', MockWebSocket)

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({}),
      text: async () => ''
    })

    const { connect, disconnect } = useWebSocket()
    await connect()

    activeSocket.triggerMessage({
      type: 'run_completed',
      runId: 'run-root',
      data: {
        summary: 'root finished'
      }
    })

    expect(store.isRunning).toBe(false)
    expect(store.activeRunId).toBe('')
    expect(store.getRun('run-root')?.status).toBe('completed')
    expect(store.messages.length).toBe(0)

    disconnect()
    global.fetch = undefined
  })
})
