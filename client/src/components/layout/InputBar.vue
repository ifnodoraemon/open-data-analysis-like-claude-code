<template>
  <div class="input-bar">
    <!-- 内置提示词模板 -->
    <div class="prompt-templates" v-if="!isRunning && messages.length === 0">
      <span class="template-label">💡 快捷分析:</span>
      <button v-for="tpl in templates" :key="tpl.text" class="template-btn" @click="useTemplate(tpl.text)">
        {{ tpl.icon }} {{ tpl.label }}
      </button>
    </div>

    <div class="upload-area" v-if="uploadedFiles.length > 0">
      <span v-for="file in uploadedFiles" :key="file.fileId" class="file-tag">
        📎 {{ file.name }}
        <span class="file-size">({{ formatSize(file.size) }})</span>
      </span>
    </div>
    <div class="input-row">
      <label class="upload-btn" :class="{ disabled: isUploading }" title="上传数据文件">
        📁
        <input type="file" accept=".csv,.xlsx,.xls,.json" @change="handleFile" :disabled="isUploading" hidden />
      </label>
      <textarea
        v-model="input"
        class="input-field"
        placeholder="上传数据文件后，输入分析需求..."
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

const { sendMessage, stop, ensureSession } = useWebSocket()
const store = useAgentStore()
const input = ref('')
const isRunning = computed(() => store.isRunning)
const uploadedFiles = computed(() => store.uploadedFiles)
const messages = computed(() => store.messages)
const isUploading = ref(false)

const templates = [
  { icon: '📊', label: '全面分析', text: '请对数据进行全面分析，包括：数据概览、各维度分布、趋势变化、关键发现和建议，并生成完整的研究报告。' },
  { icon: '📈', label: '趋势分析', text: '请分析数据的时间趋势变化，找出增长/下降的关键转折点和驱动因素。' },
  { icon: '🔍', label: '对比分析', text: '请按不同维度（如地区、产品、客户类型等）拆解对比分析，找出表现最好和最差的类别及原因。' },
  { icon: '⚠️', label: '异常检测', text: '请检查数据质量，识别异常值、缺失值和潜在的数据问题，并给出处理建议。' },
  { icon: '💰', label: '营收分析', text: '请重点分析营收/销售相关指标，包括总量、构成、趋势和同环比变化。' },
]

function useTemplate(text) {
  input.value = text
}

async function handleFile(e) {
  const file = e.target.files[0]
  if (!file) return

  const formData = new FormData()
  formData.append('file', file)

  try {
    isUploading.value = true
    const sessionId = await ensureSession()
    const res = await fetch(`/api/upload?session_id=${encodeURIComponent(sessionId)}`, {
      method: 'POST',
      headers: store.token ? { Authorization: `Bearer ${store.token}` } : {},
      body: formData,
    })
    if (!res.ok) {
      throw new Error(await res.text())
    }
    const data = await res.json()
    store.addFile({ fileId: data.file_id, name: file.name, size: file.size })
    store.addMessage({ type: 'user', content: `📎 已上传文件: ${file.name} (${formatSize(file.size)})` })
  } catch (err) {
    store.addMessage({ type: 'error', content: `文件上传失败: ${err.message}` })
  } finally {
    isUploading.value = false
    e.target.value = ''
  }
}

async function handleSend() {
  if (!input.value.trim() || isRunning.value) return
  await sendMessage(input.value.trim())
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
  background: transparent;
  padding: 8px 16px;
  flex-shrink: 0;
}

.prompt-templates {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  margin-bottom: 8px;
  align-items: center;
}

.template-label {
  font-size: 0.75rem;
  color: var(--text-muted);
  margin-right: 4px;
}

.template-btn {
  font-size: 0.72rem;
  background: var(--bg-card);
  border: 1px solid var(--border);
  color: var(--text-secondary);
  padding: 4px 10px;
  border-radius: 12px;
  cursor: pointer;
  transition: all var(--transition);
  white-space: nowrap;
}

.template-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.upload-area {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  margin-bottom: 6px;
}

.file-tag {
  font-size: 0.75rem;
  background: var(--bg-secondary);
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
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 16px;
  padding: 8px 12px;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.05);
}

.upload-btn {
  font-size: 1.2rem;
  cursor: pointer;
  padding: 4px;
  border-radius: 6px;
  transition: background var(--transition);
  color: var(--text-secondary);
}

.upload-btn:hover { background: var(--border-light); }
.upload-btn.disabled { opacity: 0.5; cursor: not-allowed; }

.input-field {
  flex: 1;
  background: transparent;
  border: none;
  padding: 4px 8px;
  color: var(--text-primary);
  font-size: 0.9rem;
  font-family: inherit;
  resize: none;
  outline: none;
}

.input-field::placeholder { color: var(--text-muted); }

.send-btn, .stop-btn {
  padding: 6px 14px;
  border-radius: 12px;
  font-size: 0.8rem;
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: all var(--transition);
}

.send-btn {
  background: var(--text-primary);
  color: var(--bg-primary);
}

.send-btn:hover:not(:disabled) { opacity: 0.85; }
.send-btn:disabled { opacity: 0.3; cursor: not-allowed; background: var(--text-muted); }

.stop-btn {
  background: var(--bg-primary);
  color: var(--accent-red);
  border: 1px solid var(--border);
}

.stop-btn:hover { background: rgba(220, 38, 38, 0.05); }
</style>
