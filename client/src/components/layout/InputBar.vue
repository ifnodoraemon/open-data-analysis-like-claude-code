<template>
  <div class="input-bar">
    <div class="upload-area" v-if="uploadedFiles.length > 0">
      <span v-for="file in uploadedFiles" :key="file.fileId" class="file-tag">
        📎 {{ file.name }}
        <span class="file-size">({{ formatSize(file.size) }})</span>
      </span>
    </div>
    <div class="input-row">
      <label
        class="upload-btn"
        :class="{ disabled: isUploading }"
        title="上传数据文件"
        aria-label="上传数据文件"
      >
        📁
        <input
          type="file"
          accept=".csv,.xlsx,.xls"
          @change="handleFile"
          :disabled="isUploading"
          hidden
        />
      </label>
      <button
        class="sources-btn"
        @click="showSourcesDrawer = true"
        title="数据源管理"
        aria-label="数据源管理"
      >
        🔗
      </button>
      <textarea
        v-model="input"
        class="input-field"
        placeholder="输入你的目标、问题或约束..."
        aria-label="消息输入框"
        @keydown.enter.exact="handleSend"
        rows="1"
        :disabled="isRunning"
      ></textarea>
      <button
        v-if="!isRunning"
        class="send-btn"
        @click="handleSend"
        :disabled="!input.trim()"
      >
        发送 ⏎
      </button>
      <button v-else class="stop-btn" @click="handleStop">■ 停止</button>
    </div>
    <DataSourceDrawer
      :open="showSourcesDrawer"
      :sessionSources="dataSourceStore.sessionSources"
      :workspaceDataSources="dataSourceStore.workspaceDataSources"
      :pendingProfiles="pendingProfiles"
      @close="showSourcesDrawer = false"
    />
  </div>
</template>

<script setup>
import { ref, computed } from "vue";
import { useWebSocket } from "../../composables/useWebSocket.js";
import { useAgentStore } from "../../stores/agent.js";
import { useDataSourceStore } from "../../stores/datasource.js";
import DataSourceDrawer from "../datasource/DataSourceDrawer.vue";

const { sendMessage, stop, ensureSession } = useWebSocket();
const store = useAgentStore();
const dataSourceStore = useDataSourceStore();
const input = ref("");
const isRunning = computed(() => store.isRunning);
const uploadedFiles = computed(() => store.uploadedFiles);
const isUploading = ref(false);
const showSourcesDrawer = ref(false);

const pendingProfiles = computed(() =>
  dataSourceStore.sessionSources.filter(s => s.semantic_status !== 'confirmed' && s.semantic_status !== 'rejected' && s.semantic_status !== '')
);

const MAX_FILE_SIZE = 50 * 1024 * 1024;

async function handleFile(e) {
  const file = e.target.files[0];
  if (!file) return;

  if (file.size > MAX_FILE_SIZE) {
    store.addMessage({
      type: "error",
      content: `文件过大（${formatSize(file.size)}），最大支持 ${formatSize(MAX_FILE_SIZE)}`,
    });
    e.target.value = "";
    return;
  }

  const formData = new FormData();
  formData.append("file", file);

  try {
    isUploading.value = true;
    const sessionId = await ensureSession();
    const res = await fetch(
      `/api/upload?session_id=${encodeURIComponent(sessionId)}`,
      {
        method: "POST",
        headers: store.token ? { Authorization: `Bearer ${store.token}` } : {},
        body: formData,
      },
    );
    if (!res.ok) {
      throw new Error(await res.text());
    }
    const data = await res.json();
    store.addFile({ fileId: data.file_id, name: file.name, size: file.size });
    store.addMessage({
      type: "user",
      content: `📎 已上传文件: ${file.name} (${formatSize(file.size)})`,
    });
  } catch (err) {
    store.addMessage({
      type: "error",
      content: `文件上传失败: ${err.message}`,
    });
  } finally {
    isUploading.value = false;
    e.target.value = "";
  }
}

async function handleSend() {
  if (!input.value.trim() || isRunning.value) return;
  await sendMessage(input.value.trim());
  input.value = "";
}

function handleStop() {
  stop();
}

function formatSize(bytes) {
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
  return (bytes / (1024 * 1024)).toFixed(1) + " MB";
}
</script>

<style scoped>
.input-bar {
  background: transparent;
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
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  padding: 2px 8px;
  border-radius: 4px;
  color: var(--text-secondary);
}

.file-size {
  color: var(--text-muted);
}

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

.sources-btn {
  font-size: 1.2rem;
  cursor: pointer;
  padding: 4px;
  border-radius: 6px;
  transition: background var(--transition);
  color: var(--text-secondary);
  background: none;
  border: none;
}

.sources-btn:hover {
  background: var(--border-light);
}

.upload-btn:hover {
  background: var(--border-light);
}
.upload-btn.disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

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

.input-field::placeholder {
  color: var(--text-muted);
}

.send-btn,
.stop-btn {
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

.send-btn:hover:not(:disabled) {
  opacity: 0.85;
}
.send-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
  background: var(--text-muted);
}

.stop-btn {
  background: var(--bg-primary);
  color: var(--accent-red);
  border: 1px solid var(--border);
}

.stop-btn:hover {
  background: rgba(220, 38, 38, 0.05);
}
</style>
