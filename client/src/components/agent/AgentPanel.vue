<template>
  <div class="agent-panel">
    <div class="panel-header">
      <span>🤖 Agent 执行</span>
      <span class="msg-count">{{ messages.length }} 条消息</span>
    </div>
    <div class="messages" ref="messagesEl">
      <div v-if="messages.length === 0" class="empty-state">
        <div class="empty-icon">🔍</div>
        <p>上传数据文件，输入分析需求</p>
        <p class="empty-hint">Agent 会自动分析数据并生成研报</p>
      </div>
      <TransitionGroup name="fade">
        <div v-for="msg in messages" :key="msg.id" class="message" :class="'msg-' + msg.type">
          <!-- 用户消息 -->
          <template v-if="msg.type === 'user'">
            <div class="msg-icon">👤</div>
            <div class="msg-body">
              <div class="msg-label">用户指令</div>
              <div class="msg-content">{{ msg.content }}</div>
            </div>
          </template>

          <!-- 思考中 -->
          <template v-else-if="msg.type === 'thinking'">
            <div class="msg-icon">🧠</div>
            <div class="msg-body">
              <div class="msg-label">思考中</div>
              <div class="msg-content thinking">{{ msg.content }}</div>
            </div>
          </template>

          <!-- 工具调用 -->
          <template v-else-if="msg.type === 'tool_call'">
            <div class="msg-icon">🔧</div>
            <div class="msg-body">
              <div class="msg-label">工具调用
                <span class="tool-name">{{ msg.name }}</span>
              </div>
              <details class="tool-details">
                <summary>查看参数</summary>
                <pre class="tool-args">{{ formatJSON(msg.arguments) }}</pre>
              </details>
            </div>
          </template>

          <!-- 工具结果 -->
          <template v-else-if="msg.type === 'tool_result'">
            <div class="msg-icon">{{ msg.success ? '✅' : '❌' }}</div>
            <div class="msg-body">
              <div class="msg-label">
                {{ msg.name }} 结果
                <span class="duration">{{ msg.duration }}ms</span>
              </div>
              <details class="tool-details">
                <summary>查看结果</summary>
                <pre class="tool-result">{{ truncate(msg.result, 2000) }}</pre>
              </details>
            </div>
          </template>

          <!-- 完成 -->
          <template v-else-if="msg.type === 'complete'">
            <div class="msg-icon">🎉</div>
            <div class="msg-body">
              <div class="msg-label complete-label">分析完成</div>
              <div class="msg-content">{{ msg.content }}</div>
            </div>
          </template>

          <template v-else-if="msg.type === 'cancelled'">
            <div class="msg-icon">⏹</div>
            <div class="msg-body">
              <div class="msg-label">任务已停止</div>
              <div class="msg-content">{{ msg.content }}</div>
            </div>
          </template>

          <!-- 错误 -->
          <template v-else-if="msg.type === 'error'">
            <div class="msg-icon">❌</div>
            <div class="msg-body">
              <div class="msg-label error-label">错误</div>
              <div class="msg-content error-content">{{ msg.content }}</div>
            </div>
          </template>

          <div class="msg-time">{{ msg.timestamp }}</div>
        </div>
      </TransitionGroup>

      <div v-if="isRunning" class="running-indicator">
        <span class="dot"></span>
        <span class="dot"></span>
        <span class="dot"></span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch, nextTick } from 'vue'
import { useAgentStore } from '../../stores/agent.js'

const store = useAgentStore()
const messages = computed(() => store.messages)
const isRunning = computed(() => store.isRunning)
const messagesEl = ref(null)

watch(messages, async () => {
  await nextTick()
  if (messagesEl.value) {
    messagesEl.value.scrollTop = messagesEl.value.scrollHeight
  }
}, { deep: true })

function formatJSON(obj) {
  try {
    return typeof obj === 'string' ? JSON.stringify(JSON.parse(obj), null, 2) : JSON.stringify(obj, null, 2)
  } catch { return String(obj) }
}

function truncate(str, max) {
  if (!str) return ''
  return str.length > max ? str.slice(0, max) + '\n... (已截断)' : str
}
</script>

<style scoped>
.agent-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--bg-primary);
}

.panel-header {
  padding: 10px 16px;
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border);
  display: flex;
  justify-content: space-between;
  flex-shrink: 0;
}

.msg-count { color: var(--text-muted); font-weight: 400; }

.messages {
  flex: 1;
  overflow-y: auto;
  padding: 12px;
}

.empty-state {
  text-align: center;
  padding: 4rem 2rem;
  color: var(--text-muted);
}

.empty-icon { font-size: 3rem; margin-bottom: 1rem; }
.empty-hint { font-size: 0.8rem; margin-top: 0.5rem; }

.message {
  display: flex;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 8px;
  margin-bottom: 6px;
  position: relative;
  animation: slideIn 0.3s ease;
}

.msg-icon { font-size: 1rem; flex-shrink: 0; margin-top: 2px; }

.msg-body { flex: 1; min-width: 0; }

.msg-label {
  font-size: 0.75rem;
  color: var(--text-secondary);
  margin-bottom: 4px;
  font-weight: 500;
}

.msg-content {
  font-size: 0.85rem;
  line-height: 1.5;
  color: var(--text-primary);
}

.msg-time {
  position: absolute;
  top: 10px;
  right: 12px;
  font-size: 0.65rem;
  color: var(--text-muted);
}

.msg-user { background: var(--bg-tertiary); }
.msg-thinking { background: rgba(88, 166, 255, 0.05); border-left: 2px solid var(--accent-blue); }
.msg-tool_call { background: rgba(188, 140, 255, 0.05); border-left: 2px solid var(--accent-purple); }
.msg-tool_result { background: rgba(63, 185, 80, 0.05); border-left: 2px solid var(--accent-green); }
.msg-complete { background: rgba(63, 185, 80, 0.08); border-left: 2px solid var(--accent-green); }
.msg-error { background: rgba(248, 81, 73, 0.08); border-left: 2px solid var(--accent-red); }

.tool-name {
  background: var(--accent-purple);
  color: white;
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 0.7rem;
  margin-left: 6px;
}

.duration {
  color: var(--text-muted);
  font-size: 0.7rem;
  margin-left: 8px;
}

.tool-details {
  margin-top: 6px;
}

.tool-details summary {
  font-size: 0.75rem;
  color: var(--accent-blue);
  cursor: pointer;
  user-select: none;
}

.tool-args, .tool-result {
  font-size: 0.75rem;
  font-family: 'SF Mono', 'Fira Code', monospace;
  background: var(--bg-secondary);
  padding: 8px 12px;
  border-radius: 6px;
  margin-top: 6px;
  overflow-x: auto;
  max-height: 300px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--text-secondary);
}

.complete-label { color: var(--accent-green); }
.error-label { color: var(--accent-red); }
.error-content { color: var(--accent-red); }

.thinking { color: var(--accent-blue); font-style: italic; }

.running-indicator {
  display: flex;
  gap: 4px;
  padding: 12px 16px;
  justify-content: center;
}

.running-indicator .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent-blue);
  animation: pulse 1.4s infinite ease-in-out;
}

.running-indicator .dot:nth-child(2) { animation-delay: 0.2s; }
.running-indicator .dot:nth-child(3) { animation-delay: 0.4s; }
</style>
