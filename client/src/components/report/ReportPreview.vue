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
        <div v-if="reportHTML" class="export-menu">
          <button class="toolbar-btn export" @click="toggleExportMenu">
            ⬇ 导出报告
          </button>
          <div v-if="showExportMenu" class="export-dropdown">
            <button class="export-option" @click="exportReport('pdf')">导出 PDF</button>
            <button class="export-option" @click="exportReport('word')">导出 Word</button>
            <button class="export-option" @click="exportReport('html')">导出 HTML</button>
          </div>
        </div>
      </div>
    </div>
    <div class="preview-area">
      <div v-if="!reportHTML" class="empty-state">
        <div class="empty-icon">📊</div>
        <p>研究报告将在这里实时渲染</p>
      </div>
      <iframe
        v-else-if="mode === 'preview'"
        ref="reportFrame"
        :src="reportURL"
        class="report-iframe"
        sandbox="allow-scripts allow-same-origin"
      ></iframe>
      <pre v-else class="source-code">{{ reportHTML }}</pre>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useAgentStore } from '../../stores/agent.js'

const store = useAgentStore()
const reportHTML = computed(() => store.reportHTML)
const selectedRun = computed(() => store.runs.find(run => run.id === store.selectedRunId) || null)
const activeRun = computed(() => store.runs.find(run => run.id === store.activeRunId) || null)
const selectedReport = computed(() => selectedRun.value?.report || null)
const reportURL = ref('')
const reportFrame = ref(null)
const showExportMenu = ref(false)
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
  showExportMenu.value = false
  if (!html) return
  reportURL.value = URL.createObjectURL(new Blob([html], { type: 'text/html' }))
}, { immediate: true })

onBeforeUnmount(() => {
  if (reportURL.value) {
    URL.revokeObjectURL(reportURL.value)
  }
})

function toggleExportMenu() {
  showExportMenu.value = !showExportMenu.value
}

async function exportReport(format) {
  showExportMenu.value = false
  if (!reportHTML.value) return
  try {
    if (format === 'pdf') {
      await exportPDF()
      return
    }
    if (format === 'word') {
      await exportWord()
      return
    }
    exportHTML()
  } catch (error) {
    const message = error instanceof Error ? error.message : '报告导出失败'
    store.addMessage({ type: 'error', content: message })
  }
}

function exportHTML() {
  downloadBlob(new Blob([reportHTML.value], { type: 'text/html;charset=utf-8' }), `${buildFilename()}.html`)
}

async function exportWord() {
  const snapshotHTML = await buildRenderedSnapshotHTML({ forWord: true })
  const res = await fetch('/api/report-exports/docx', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(store.token ? { Authorization: `Bearer ${store.token}` } : {}),
    },
    body: JSON.stringify({
      title: buildFilename(),
      html: snapshotHTML,
    }),
  })
  if (!res.ok) {
    throw new Error(await res.text())
  }
  const blob = await res.blob()
  downloadBlob(blob, `${buildFilename()}.docx`)
}

async function exportPDF() {
  const snapshotHTML = await buildRenderedSnapshotHTML()
  const url = URL.createObjectURL(new Blob([snapshotHTML], { type: 'text/html;charset=utf-8' }))
  const printWindow = window.open(url, '_blank', 'width=1200,height=900')
  if (!printWindow) return
  await waitForPrintWindow(printWindow)
  printWindow.focus()
  printWindow.print()
  window.setTimeout(() => {
    URL.revokeObjectURL(url)
  }, 60 * 1000)
}

function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

function buildFilename() {
  const title = selectedReport.value?.title || '研究报告'
  const safeTitle = title.replace(/[\\/:*?"<>|]/g, '-').trim() || '研究报告'
  const date = new Date().toISOString().slice(0, 10)
  return `${safeTitle}_${date}`
}

function waitForPrintWindow(targetWindow) {
  return new Promise((resolve) => {
    const checkReady = () => {
      try {
        if (targetWindow.document.readyState === 'complete') {
          resolve()
          return
        }
      } catch (_) {
        resolve()
        return
      }
      window.setTimeout(checkReady, 60)
    }
    checkReady()
  })
}

async function buildRenderedSnapshotHTML(options = {}) {
  const { forWord = false } = options
  const frameWindow = reportFrame.value?.contentWindow
  const frameDocument = frameWindow?.document
  if (!frameDocument?.documentElement) {
    return reportHTML.value
  }

  await waitForFrameReady(frameDocument)
  const clonedDocument = frameDocument.documentElement.cloneNode(true)
  const sourceCanvases = Array.from(frameDocument.querySelectorAll('canvas'))
  clonedDocument.querySelectorAll('canvas').forEach((canvasNode, index) => {
    const sourceCanvas = sourceCanvases[index]
    if (!sourceCanvas?.toDataURL) return
    const image = clonedDocument.ownerDocument.createElement('img')
    image.src = sourceCanvas.toDataURL('image/png')
    image.alt = sourceCanvas.getAttribute('aria-label') || 'chart'
    const rect = sourceCanvas.getBoundingClientRect()
    const width = Math.max(Math.round(rect.width || sourceCanvas.clientWidth || sourceCanvas.width || 0), 1)
    const height = Math.max(Math.round(rect.height || sourceCanvas.clientHeight || sourceCanvas.height || 0), 1)
    image.width = width
    image.height = height
    image.style.display = 'block'
    image.style.width = `${width}px`
    image.style.maxWidth = '100%'
    image.style.height = `${height}px`
    image.style.objectFit = 'contain'
    image.style.margin = '0 auto'
    canvasNode.replaceWith(image)
  })
  if (forWord) {
    optimizeSnapshotForWord(clonedDocument)
  }
  clonedDocument.querySelectorAll('script').forEach((node) => node.remove())
  return `<!DOCTYPE html>\n${clonedDocument.outerHTML}`
}

function optimizeSnapshotForWord(documentNode) {
  const doc = documentNode.ownerDocument
  const head = documentNode.querySelector('head')
  const body = documentNode.querySelector('body')
  if (!head || !body) return

  head.querySelectorAll('script, link[rel="preconnect"], link[rel="stylesheet"]').forEach((node) => node.remove())
  body.querySelectorAll('.cover').forEach((node) => {
    node.style.minHeight = 'auto'
    node.style.background = '#ffffff'
    node.style.color = '#111827'
    node.style.padding = '0 0 18pt 0'
    node.style.margin = '0 0 18pt 0'
    node.style.pageBreakAfter = 'avoid'
  })
  body.querySelectorAll('.toc, .section, .footer').forEach((node) => {
    node.style.maxWidth = 'none'
    node.style.margin = '0 0 16pt 0'
    node.style.padding = node.classList.contains('footer') ? '8pt 0 0 0' : '0'
    node.style.background = '#ffffff'
    node.style.boxShadow = 'none'
    node.style.border = 'none'
    node.style.borderRadius = '0'
    node.style.pageBreakInside = 'avoid'
  })
  body.querySelectorAll('.summary-box, .conclusion-box').forEach((node) => {
    node.style.background = '#ffffff'
    node.style.border = '1px solid #d1d5db'
    node.style.borderLeft = '4px solid #0f2b46'
    node.style.borderRadius = '0'
    node.style.boxShadow = 'none'
    node.style.padding = '12pt'
  })
  body.querySelectorAll('.chart-box').forEach((node) => {
    node.style.height = 'auto'
    node.style.minHeight = '0'
    node.style.padding = '8pt'
    node.style.border = '1px solid #d1d5db'
    node.style.boxShadow = 'none'
    node.style.background = '#ffffff'
    node.style.pageBreakInside = 'avoid'
  })
  body.querySelectorAll('img').forEach((node) => {
    node.style.display = 'block'
    node.style.maxWidth = '100%'
    node.style.height = 'auto'
    node.style.pageBreakInside = 'avoid'
  })
  body.querySelectorAll('table').forEach((node) => {
    node.style.width = '100%'
    node.style.borderCollapse = 'collapse'
    node.style.fontSize = '10.5pt'
  })
  body.querySelectorAll('th, td').forEach((node) => {
    node.style.border = '1px solid #d1d5db'
    node.style.padding = '6pt 8pt'
  })

  const exportStyle = doc.createElement('style')
  exportStyle.textContent = `
    @page { size: A4; margin: 22mm 18mm; }
    html, body {
      background: #ffffff !important;
    }
    body {
      font-family: "Microsoft YaHei", "PingFang SC", "Noto Sans CJK SC", "SimSun", sans-serif !important;
      color: #111827 !important;
      font-size: 11pt !important;
      line-height: 1.7 !important;
      margin: 0 !important;
      padding: 0 !important;
    }
    * {
      animation: none !important;
      transition: none !important;
      text-shadow: none !important;
    }
    .cover::before,
    .toc h2::before,
    .section h2::after,
    .footer::before {
      content: none !important;
      display: none !important;
    }
    .cover h1 {
      font-size: 22pt !important;
      margin-bottom: 8pt !important;
    }
    .cover .meta {
      display: block !important;
      font-size: 10.5pt !important;
    }
    .toc h2,
    .section h2,
    .content h3,
    .content h4,
    .content h5 {
      color: #0f2b46 !important;
      border: none !important;
      padding: 0 !important;
      margin-top: 0 !important;
    }
    .content p {
      text-indent: 0 !important;
      font-size: 11pt !important;
      margin: 0 0 8pt 0 !important;
    }
    .toc ol,
    .content ul {
      padding-left: 18pt !important;
      margin: 0 0 10pt 0 !important;
    }
  `
  head.appendChild(exportStyle)
}

function waitForFrameReady(frameDocument) {
  return new Promise((resolve) => {
    const checkReady = () => {
      if (frameDocument.readyState === 'complete') {
        window.setTimeout(resolve, 120)
        return
      }
      window.setTimeout(checkReady, 80)
    }
    checkReady()
  })
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

.export-menu {
  position: relative;
}

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

.export-dropdown {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  min-width: 140px;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: 10px;
  box-shadow: 0 12px 30px rgba(15, 23, 42, 0.12);
  padding: 6px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  z-index: 10;
}

.export-option {
  border: none;
  background: transparent;
  color: var(--text-primary);
  text-align: left;
  padding: 8px 10px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 0.8rem;
}

.export-option:hover {
  background: var(--bg-secondary);
}

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
