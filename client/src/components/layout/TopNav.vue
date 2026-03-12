<template>
  <nav class="top-nav">
    <div class="nav-left">
      <span class="logo">📊</span>
      <span class="title">数据分析智能体</span>
      <select
        v-if="workspaceOptions.length > 0"
        class="workspace-select"
        :value="workspaceId"
        @change="handleWorkspaceChange"
      >
        <option v-for="item in workspaceOptions" :key="item.id" :value="item.id">
          {{ item.name }}
        </option>
      </select>
    </div>
    <div class="nav-center">
      <span class="status-dot" :class="connected ? 'online' : 'offline'"></span>
      <span class="status-text">{{ statusText }}</span>
    </div>
    <div class="nav-right">
      <button class="nav-btn" @click="clearAll" title="新建分析">
        ✨ 新建
      </button>
      <button class="nav-btn" @click="logout">
        退出
      </button>
    </div>
  </nav>
</template>

<script setup>
import { computed } from 'vue'
import { useWebSocket } from '../../composables/useWebSocket.js'
import { useAgentStore } from '../../stores/agent.js'

const { connected, createNewSession, disconnect, switchWorkspace } = useWebSocket()
const store = useAgentStore()
const workspaceOptions = computed(() => store.workspaces || [])
const workspaceId = computed(() => store.workspace?.id || '')
const statusText = computed(() => {
  switch (store.connectionState) {
    case 'connected':
      return '已连接'
    case 'reconnecting':
      return '重连中...'
    case 'disconnected':
      return '未连接'
    default:
      return '连接中...'
  }
})

function clearAll() {
  createNewSession()
}

async function handleWorkspaceChange(event) {
  const nextWorkspaceId = event.target.value
  if (!nextWorkspaceId || nextWorkspaceId === workspaceId.value) return
  try {
    await switchWorkspace(nextWorkspaceId)
  } catch (err) {
    console.error('switch workspace failed:', err)
  }
}

function logout() {
  disconnect()
  store.logout()
}
</script>

<style scoped>
.top-nav {
  height: var(--nav-height);
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  backdrop-filter: blur(12px);
  flex-shrink: 0;
}

.nav-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.logo { font-size: 1.2rem; }

.title {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--text-primary);
}

.workspace-select {
  font-size: 0.72rem;
  color: var(--text-secondary);
  background: var(--bg-card);
  border: 1px solid var(--border);
  padding: 4px 8px;
  border-radius: 999px;
  outline: none;
}

.nav-center {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.75rem;
  color: var(--text-secondary);
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.status-dot.online { background: var(--accent-green); box-shadow: 0 0 4px var(--accent-green); }
.status-dot.offline { background: var(--accent-orange); animation: pulse 1.5s infinite; }

.nav-right {
  display: flex;
  gap: 8px;
}

.nav-btn {
  background: var(--bg-card);
  border: 1px solid var(--border);
  color: var(--text-primary);
  padding: 4px 12px;
  border-radius: 6px;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all var(--transition);
}

.nav-btn:hover {
  background: var(--bg-hover);
  border-color: var(--accent-blue);
}
</style>
