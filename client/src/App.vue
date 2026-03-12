<template>
  <div v-if="isRestoring" class="app-loading">正在恢复工作区...</div>
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

const { bootstrap, connect } = useWebSocket()
const store = useAgentStore()
const leftWidth = ref(45)
const isDragging = ref(false)
const isRestoring = ref(false)
const isAuthenticated = computed(() => !!store.token && !!store.user)
let initPromise = null

onMounted(() => {
  if (store.token) {
    void initApp()
  }
})

watch(() => store.token, (nextToken, prevToken) => {
  if (nextToken && nextToken !== prevToken) {
    void initApp()
  } else if (!nextToken) {
    isRestoring.value = false
  }
})

function initApp() {
  if (initPromise) return initPromise
  isRestoring.value = !store.user
  initPromise = bootstrap()
    .then(() => connect())
    .catch((err) => {
      console.error('bootstrap failed:', err)
    })
    .finally(() => {
      isRestoring.value = false
      initPromise = null
    })
  return initPromise
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
