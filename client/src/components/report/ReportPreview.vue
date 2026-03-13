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
      <iframe v-else-if="mode === 'preview'" :src="reportURL" class="report-iframe" sandbox="allow-scripts allow-same-origin"></iframe>
      <pre v-else class="source-code">{{ reportHTML }}</pre>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useWebSocket } from '../../composables/useWebSocket.js'
import { useAgentStore } from '../../stores/agent.js'

const store = useAgentStore()
const { downloadRunReport } = useWebSocket()
const reportHTML = computed(() => store.reportHTML)
const selectedRun = computed(() => store.runs.find(run => run.id === store.selectedRunId) || null)
const activeRun = computed(() => store.runs.find(run => run.id === store.activeRunId) || null)
const selectedReport = computed(() => selectedRun.value?.report || null)
const reportURL = ref('')
const runMetaLabel = computed(() => {
  if (selectedReport.value?.title) {
    const suffix = selectedReport.value.author ? ` · ${selectedReport.value.author}` : ''
    return `${selectedReport.value.title}${suffix}`
  }
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

watch(reportHTML, (html) => {
  if (reportURL.value) {
    URL.revokeObjectURL(reportURL.value)
    reportURL.value = ''
  }
  if (!html) return
  reportURL.value = URL.createObjectURL(new Blob([html], { type: 'text/html' }))
}, { immediate: true })

onBeforeUnmount(() => {
  if (reportURL.value) {
    URL.revokeObjectURL(reportURL.value)
  }
})

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
  background: var(--bg-primary);
  border-left: 1px solid var(--border);
}

.panel-header {
  padding: 16px;
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--text-primary);
  border-bottom: 1px solid var(--border);
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-shrink: 0;
  background: var(--bg-primary);
}

.header-main {
  display: flex;
  align-items: center;
  gap: 10px;
}

.run-meta {
  font-size: 0.75rem;
  color: var(--text-muted);
  font-weight: 400;
}

.toolbar { display: flex; gap: 8px; }

.toolbar-btn {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  color: var(--text-secondary);
  padding: 4px 12px;
  border-radius: 6px;
  font-size: 0.75rem;
  cursor: pointer;
  transition: all var(--transition);
}

.toolbar-btn:hover { border-color: var(--border-light); color: var(--text-primary); background: var(--bg-hover); }
.toolbar-btn.active { background: var(--text-primary); color: var(--bg-primary); border-color: var(--text-primary); }
.toolbar-btn.export { background: var(--accent-blue); color: white; border-color: var(--accent-blue); }
.toolbar-btn.export:hover { opacity: 0.9; background: var(--accent-blue); }

.preview-area {
  flex: 1;
  overflow: hidden;
  position: relative;
  background: var(--bg-secondary);
}

.empty-state {
  text-align: center;
  padding: 4rem 2rem;
  color: var(--text-muted);
  height: 100%;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
}

.empty-icon { font-size: 3rem; margin-bottom: 1rem; opacity: 0.5; }

.report-iframe {
  width: 100%;
  height: 100%;
  border: none;
  background: var(--bg-primary);
  box-shadow: -4px 0 16px rgba(0, 0, 0, 0.02);
}

.source-code {
  font-size: 0.8rem;
  font-family: 'SF Mono', 'Fira Code', monospace;
  padding: 24px;
  overflow: auto;
  height: 100%;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-all;
  background: var(--bg-primary);
  margin: 0;
}
</style>
