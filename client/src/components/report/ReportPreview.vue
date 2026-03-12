<template>
  <div class="report-preview">
    <div class="panel-header">
      <div class="header-main">
        <span>📄 研报预览</span>
        <span v-if="runMetaLabel" class="run-meta">{{ runMetaLabel }}</span>
      </div>
      <div class="toolbar">
        <button class="toolbar-btn" :class="{ active: mode === 'preview' }" @click="mode = 'preview'">预览</button>
        <button class="toolbar-btn" :class="{ active: mode === 'source' }" @click="mode = 'source'">源码</button>
        <button v-if="reportHTML" class="toolbar-btn export" @click="handleExport">
          {{ selectedRun?.reportFileId ? '⬇ 下载报告' : '⬇ 导出快照' }}
        </button>
      </div>
    </div>
    <div class="preview-area">
      <div v-if="!reportHTML" class="empty-state">
        <div class="empty-icon">📊</div>
        <p>研究报告将在这里实时渲染</p>
      </div>
      <iframe v-else-if="mode === 'preview'" :srcdoc="reportHTML" class="report-iframe" sandbox="allow-scripts allow-same-origin"></iframe>
      <pre v-else class="source-code">{{ reportHTML }}</pre>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useWebSocket } from '../../composables/useWebSocket.js'
import { useAgentStore } from '../../stores/agent.js'

const store = useAgentStore()
const { downloadRunReport } = useWebSocket()
const reportHTML = computed(() => store.reportHTML)
const selectedRun = computed(() => store.runs.find(run => run.id === store.selectedRunId) || null)
const activeRun = computed(() => store.runs.find(run => run.id === store.activeRunId) || null)
const runMetaLabel = computed(() => {
  if (selectedRun.value?.id && activeRun.value?.id && selectedRun.value.id !== activeRun.value.id) {
    return `当前查看历史任务 ${selectedRun.value.id}`
  }
  if (selectedRun.value?.id) {
    return `当前报告 ${selectedRun.value.id}`
  }
  if (activeRun.value?.id) {
    return `正在执行 ${activeRun.value.id}`
  }
  return ''
})
const mode = ref('preview')

async function handleExport() {
  if (selectedRun.value?.reportFileId) {
    await downloadRunReport(selectedRun.value.id)
    return
  }
  exportHTML()
}

function exportHTML() {
  const blob = new Blob([reportHTML.value], { type: 'text/html' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `研究报告_${new Date().toISOString().slice(0, 10)}.html`
  a.click()
  URL.revokeObjectURL(url)
}
</script>

<style scoped>
.report-preview {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--bg-secondary);
}

.panel-header {
  padding: 10px 16px;
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border);
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-shrink: 0;
}

.header-main {
  display: flex;
  align-items: center;
  gap: 10px;
}

.run-meta {
  font-size: 0.7rem;
  color: var(--text-muted);
  font-weight: 400;
}

.toolbar { display: flex; gap: 4px; }

.toolbar-btn {
  background: var(--bg-card);
  border: 1px solid var(--border);
  color: var(--text-secondary);
  padding: 2px 10px;
  border-radius: 4px;
  font-size: 0.7rem;
  cursor: pointer;
  transition: all var(--transition);
}

.toolbar-btn:hover { border-color: var(--accent-blue); color: var(--text-primary); }
.toolbar-btn.active { background: var(--accent-blue); color: white; border-color: var(--accent-blue); }
.toolbar-btn.export { background: var(--accent-green); color: white; border-color: var(--accent-green); }
.toolbar-btn.export:hover { opacity: 0.9; }

.preview-area {
  flex: 1;
  overflow: hidden;
  position: relative;
}

.empty-state {
  text-align: center;
  padding: 4rem 2rem;
  color: var(--text-muted);
}

.empty-icon { font-size: 3rem; margin-bottom: 1rem; }

.report-iframe {
  width: 100%;
  height: 100%;
  border: none;
  background: white;
}

.source-code {
  font-size: 0.75rem;
  font-family: 'SF Mono', 'Fira Code', monospace;
  padding: 16px;
  overflow: auto;
  height: 100%;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
