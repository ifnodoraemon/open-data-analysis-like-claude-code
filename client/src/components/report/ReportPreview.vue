<template>
  <div class="report-preview">
    <div class="panel-header">
      <span>📄 研报预览</span>
      <div class="toolbar">
        <button class="toolbar-btn" :class="{ active: mode === 'preview' }" @click="mode = 'preview'">预览</button>
        <button class="toolbar-btn" :class="{ active: mode === 'source' }" @click="mode = 'source'">源码</button>
        <button v-if="reportHTML" class="toolbar-btn export" @click="exportHTML">⬇ 导出</button>
      </div>
    </div>
    <div class="preview-area">
      <div v-if="!reportHTML" class="empty-state">
        <div class="empty-icon">📊</div>
        <p>研究报告将在这里实时渲染</p>
      </div>
      <iframe v-else-if="mode === 'preview'" :srcdoc="reportHTML" class="report-iframe" sandbox="allow-scripts"></iframe>
      <pre v-else class="source-code">{{ reportHTML }}</pre>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useAgentStore } from '../../stores/agent.js'

const store = useAgentStore()
const reportHTML = computed(() => store.reportHTML)
const mode = ref('preview')

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
