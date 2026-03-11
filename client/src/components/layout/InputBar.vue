<template>
  <div class="input-bar">
    <div class="upload-area" v-if="uploadedFiles.length > 0">
      <span v-for="file in uploadedFiles" :key="file.name" class="file-tag">
        📎 {{ file.name }}
        <span class="file-size">({{ formatSize(file.size) }})</span>
      </span>
    </div>
    <div class="input-row">
      <label class="upload-btn" title="上传数据文件">
        📁
        <input type="file" accept=".csv,.xlsx,.xls,.json" @change="handleFile" hidden />
      </label>
      <textarea
        v-model="input"
        class="input-field"
        placeholder="输入数据分析需求，例如：分析销售数据的趋势和构成..."
        @keydown.enter.exact="handleSend"
        rows="1"
        :disabled="isRunning"
      ></textarea>
      <button v-if="!isRunning" class="send-btn" @click="handleSend" :disabled="!input.trim()">
        发送 ⏎
      </button>
      <button v-else class="stop-btn" @click="handleStop">
        ■ 停止
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useWebSocket } from '../../composables/useWebSocket.js'
import { useAgentStore } from '../../stores/agent.js'

const { sendMessage, stop } = useWebSocket()
const store = useAgentStore()
const input = ref('')
const isRunning = computed(() => store.isRunning)
const uploadedFiles = computed(() => store.uploadedFiles)

async function handleFile(e) {
  const file = e.target.files[0]
  if (!file) return

  const formData = new FormData()
  formData.append('file', file)

  try {
    const res = await fetch('/api/upload', { method: 'POST', body: formData })
    const data = await res.json()
    store.addFile({ name: file.name, size: file.size, path: data.path })
    store.addMessage({ type: 'user', content: `📎 已上传文件: ${file.name} (${formatSize(file.size)})` })
  } catch (err) {
    store.addMessage({ type: 'error', content: `文件上传失败: ${err.message}` })
  }
}

function handleSend() {
  if (!input.value.trim() || isRunning.value) return
  sendMessage(input.value.trim())
  input.value = ''
}

function handleStop() {
  stop()
}

function formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}
</script>

<style scoped>
.input-bar {
  border-top: 1px solid var(--border);
  background: var(--bg-secondary);
  padding: 8px 16px;
  flex-shrink: 0;
}

.upload-area {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  margin-bottom: 6px;
}

.file-tag {
  font-size: 0.75rem;
  background: var(--bg-card);
  border: 1px solid var(--border);
  padding: 2px 8px;
  border-radius: 4px;
  color: var(--text-secondary);
}

.file-size { color: var(--text-muted); }

.input-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.upload-btn {
  font-size: 1.2rem;
  cursor: pointer;
  padding: 4px;
  border-radius: 6px;
  transition: background var(--transition);
}

.upload-btn:hover { background: var(--bg-hover); }

.input-field {
  flex: 1;
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 10px 14px;
  color: var(--text-primary);
  font-size: 0.85rem;
  font-family: inherit;
  resize: none;
  outline: none;
  transition: border-color var(--transition);
}

.input-field:focus { border-color: var(--accent-blue); }
.input-field::placeholder { color: var(--text-muted); }

.send-btn, .stop-btn {
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 0.8rem;
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: all var(--transition);
}

.send-btn {
  background: var(--accent-blue);
  color: white;
}

.send-btn:hover:not(:disabled) { opacity: 0.9; }
.send-btn:disabled { opacity: 0.5; cursor: not-allowed; }

.stop-btn {
  background: var(--accent-red);
  color: white;
}

.stop-btn:hover { opacity: 0.9; }
</style>
