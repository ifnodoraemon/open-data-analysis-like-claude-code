<template>
  <div class="app">
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
import { ref, onMounted } from 'vue'
import { useWebSocket } from './composables/useWebSocket.js'
import TopNav from './components/layout/TopNav.vue'
import AgentPanel from './components/agent/AgentPanel.vue'
import ReportPreview from './components/report/ReportPreview.vue'
import InputBar from './components/layout/InputBar.vue'

const { connect } = useWebSocket()
const leftWidth = ref(45)
const isDragging = ref(false)

onMounted(() => {
  connect()
})

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
