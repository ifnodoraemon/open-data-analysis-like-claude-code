<template>
  <div v-if="isRestoring" class="app-loading">正在恢复工作区...</div>
  <div v-else-if="hasRestoreError" class="app-loading error-state">
    <div class="error-card">
      <p>工作区恢复失败</p>
      <p class="error-message">{{ restoreError }}</p>
      <button class="retry-btn" type="button" @click="retryInit">重试</button>
    </div>
  </div>
  <LoginScreen v-else-if="!isAuthenticated" />
  <div v-else class="app">
    <TopNav />
    <div class="main-content">
      <AgentPanel class="panel-left" :style="{ width: leftWidth + '%' }" />
      <div class="splitter" @mousedown="startDrag" :class="{ dragging: isDragging }"></div>
      <ReportPreview class="panel-right" :style="{ width: (100 - leftWidth) + '%' }" />
    </div>
    <InputBar />
  </div>
</template>

<script setup>
import { computed, ref, onMounted, watch } from 'vue'
import { useWebSocket } from './composables/useWebSocket.js'
import { useAgentStore } from './stores/agent.js'
import TopNav from './components/layout/TopNav.vue'
import AgentPanel from './components/agent/AgentPanel.vue'
import ReportPreview from './components/report/ReportPreview.vue'
import InputBar from './components/layout/InputBar.vue'
import LoginScreen from './components/auth/LoginScreen.vue'

const { initializeApp } = useWebSocket()
const store = useAgentStore()
const leftWidth = ref(45)
const isDragging = ref(false)
const isAuthenticated = computed(() => !!store.token && !!store.user)
const isRestoring = computed(() => store.bootstrapState === 'loading')
const hasRestoreError = computed(() => store.bootstrapState === 'error' && !!store.token)
const restoreError = computed(() => store.bootstrapError || '请稍后重试。')

onMounted(() => {
  if (store.token) {
    void initApp()
  }
})

watch(() => store.token, (nextToken, prevToken) => {
  if (nextToken && nextToken !== prevToken) {
    void initApp()
  } else if (!nextToken) {
    store.setBootstrapState('idle')
  }
})

function initApp() {
  return initializeApp().catch((err) => {
    console.error('bootstrap failed:', err)
  })
}

function retryInit() {
  void initApp()
}

function startDrag(e) {
  isDragging.value = true
  const startX = e.clientX
  const startWidth = leftWidth.value

  function onMove(e) {
    const dx = e.clientX - startX
    const containerWidth = document.querySelector('.main-content').offsetWidth
    const newWidth = startWidth + (dx / containerWidth) * 100
    leftWidth.value = Math.max(25, Math.min(75, newWidth))
  }

  function onUp() {
    isDragging.value = false
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
  }

  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}
</script>

<style scoped>
.app-loading {
  height: 100vh;
  display: grid;
  place-items: center;
  background: var(--bg-primary);
  color: var(--text-secondary);
  letter-spacing: 0.04em;
}

.error-state {
  padding: 24px;
}

.error-card {
  max-width: 420px;
  padding: 20px 24px;
  border: 1px solid var(--border);
  border-radius: 16px;
  background: var(--bg-secondary);
  text-align: center;
}

.error-message {
  margin: 12px 0 0;
  color: var(--accent-red);
  line-height: 1.5;
}

.retry-btn {
  margin-top: 16px;
  padding: 8px 16px;
  border: 1px solid var(--accent-blue);
  border-radius: 8px;
  background: var(--accent-blue);
  color: white;
  cursor: pointer;
}

.app {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--bg-primary);
}

.main-content {
  flex: 1;
  display: flex;
  overflow: hidden;
}

.panel-left, .panel-right {
  overflow: hidden;
}

.splitter {
  width: 4px;
  background: var(--border);
  cursor: col-resize;
  transition: background var(--transition);
  flex-shrink: 0;
}

.splitter:hover, .splitter.dragging {
  background: var(--accent-blue);
}
</style>
