<template>
  <div class="agent-panel">
    <div v-if="activeRun || selectedRun" class="run-summary">
      <span v-if="activeRun" class="summary-pill live">
        正在执行 {{ truncate(activeRun.summary || activeRun.inputMessage || activeRun.id, 36) }}
      </span>
      <span v-if="selectedRun && selectedRun.id !== activeRun?.id" class="summary-pill history">
        当前查看历史任务 {{ truncate(selectedRun.summary || selectedRun.inputMessage || selectedRun.id, 36) }}
      </span>
    </div>
    <RunTree />
    <WorkingMemoryPanel />
    <SubgoalTree />
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
              <div class="msg-content markdown-body" v-html="renderMarkdown(msg.content)"></div>
            </div>
          </template>

          <!-- 思考中 -->
          <template v-else-if="msg.type === 'thinking'">
            <div class="msg-icon">🧠</div>
            <div class="msg-body">
              <div class="msg-label">思考中</div>
              <div class="msg-content markdown-body thinking" v-html="renderMarkdown(msg.content)"></div>
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

          <!-- 提问用户 (Human in loop) -->
          <template v-else-if="msg.type === 'user_request_input'">
            <div class="msg-icon">🙋</div>
            <div class="msg-body">
              <div class="msg-label ask-user-label">需要您确认</div>
              <div class="msg-content markdown-body ask-user-question" v-html="renderMarkdown(msg.question)"></div>
              <div v-if="msg.options && msg.options.length > 0" class="ask-options">
                <button 
                  v-for="(opt, idx) in msg.options" 
                  :key="idx" 
                  class="ask-option-btn"
                  @click="handleOptionClick(opt)"
                >
                  {{ opt }}
                </button>
              </div>
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
              <div v-if="toolResultSummary(msg)" class="msg-content tool-result-summary">
                {{ toolResultSummary(msg) }}
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
              <div class="msg-content markdown-body" v-html="renderMarkdown(msg.content)"></div>
            </div>
          </template>

          <template v-else-if="msg.type === 'cancelled'">
            <div class="msg-icon">⏹</div>
            <div class="msg-body">
              <div class="msg-label">任务已停止</div>
              <div class="msg-content markdown-body" v-html="renderMarkdown(msg.content)"></div>
            </div>
          </template>

          <!-- 错误 -->
          <template v-else-if="msg.type === 'error'">
            <div class="msg-icon">❌</div>
            <div class="msg-body">
              <div class="msg-label error-label">错误</div>
              <div class="msg-content markdown-body error-content" v-html="renderMarkdown(msg.content)"></div>
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
import { marked } from 'marked'
import hljs from 'highlight.js'
import { useWebSocket } from '../../composables/useWebSocket.js'
import { useAgentStore } from '../../stores/agent.js'
import RunTree from './RunTree.vue'
import SubgoalTree from './SubgoalTree.vue'
import WorkingMemoryPanel from './WorkingMemoryPanel.vue'

const store = useAgentStore()
const { openRun } = useWebSocket()
const messages = computed(() => store.messages)
const isRunning = computed(() => store.isRunning)
const selectedRunId = computed(() => store.selectedRunId)
const activeRunId = computed(() => store.activeRunId)
const selectedRun = computed(() => store.getRun(selectedRunId.value))
const activeRun = computed(() => store.getRun(activeRunId.value))
const messagesEl = ref(null)

marked.setOptions({
  gfm: true,
  breaks: true,
  highlight(code, language) {
    if (language && hljs.getLanguage(language)) {
      return hljs.highlight(code, { language }).value
    }
    return hljs.highlightAuto(code).value
  },
})

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

function toolResultSummary(msg) {
  const payload = msg?.parsedResult
  if (!payload || typeof payload !== 'object') return ''
  if (typeof payload.summary_text === 'string' && payload.summary_text.trim()) return payload.summary_text
  if (typeof payload.delegate_summary === 'string' && payload.delegate_summary.trim()) return payload.delegate_summary
  if (typeof payload.message === 'string' && payload.message.trim()) return payload.message
  return ''
}

function renderMarkdown(content) {
  return marked.parse(String(content || ''))
}

async function handleRunClick(runId) {
  if (!runId || runId === selectedRunId.value) return
  try {
    await openRun(runId)
  } catch (err) {
    console.error('open run failed:', err)
  }
}

function handleOptionClick(opt) {
  const { sendMessage } = useWebSocket()
  sendMessage(opt)
}
</script>

<style scoped>
.agent-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--bg-primary);
}

.run-summary {
  display: flex;
  gap: 8px;
  padding: 10px 12px 0;
  flex-wrap: wrap;
}

.summary-pill {
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 0.7rem;
}

.summary-pill.live {
  color: #d2e9ff;
  background: rgba(47, 129, 247, 0.18);
  border: 1px solid rgba(47, 129, 247, 0.35);
}

.summary-pill.history {
  color: var(--text-secondary);
  background: rgba(139, 148, 158, 0.12);
  border: 1px solid rgba(139, 148, 158, 0.2);
}

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

.tool-result-summary {
  margin-bottom: 8px;
}

.markdown-body :deep(p),
.markdown-body :deep(ul),
.markdown-body :deep(ol),
.markdown-body :deep(blockquote),
.markdown-body :deep(pre) {
  margin: 0 0 0.75rem;
}

.markdown-body :deep(p:last-child),
.markdown-body :deep(ul:last-child),
.markdown-body :deep(ol:last-child),
.markdown-body :deep(blockquote:last-child),
.markdown-body :deep(pre:last-child) {
  margin-bottom: 0;
}

.markdown-body :deep(ul),
.markdown-body :deep(ol) {
  padding-left: 1.25rem;
}

.markdown-body :deep(li + li) {
  margin-top: 0.2rem;
}

.markdown-body :deep(code) {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 0.8em;
  background: rgba(139, 148, 158, 0.14);
  padding: 0.1rem 0.35rem;
  border-radius: 4px;
}

.markdown-body :deep(pre) {
  overflow-x: auto;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 0.85rem 1rem;
}

.markdown-body :deep(pre code) {
  background: transparent;
  padding: 0;
}

.markdown-body :deep(blockquote) {
  padding-left: 0.85rem;
  border-left: 3px solid var(--border);
  color: var(--text-secondary);
}

.markdown-body :deep(a) {
  color: var(--accent-blue);
}

.markdown-body :deep(strong) {
  font-weight: 700;
}

.msg-time {
  position: absolute;
  top: 10px;
  right: 12px;
  font-size: 0.65rem;
  color: var(--text-muted);
}

.msg-user {
  background: var(--bg-secondary);
  border-radius: 12px;
  align-self: flex-end;
  max-width: 85%;
}

.msg-thinking, .msg-tool_call, .msg-tool_result, .msg-complete, .msg-error {
  background: transparent;
  border-left: none;
}

.msg-icon { font-size: 1.2rem; flex-shrink: 0; margin-top: 2px; }

.thinking { color: var(--text-muted); font-style: italic; }

.tool-name {
  background: var(--bg-hover);
  color: var(--text-secondary);
  padding: 2px 8px;
  border-radius: 6px;
  font-size: 0.7rem;
  margin-left: 6px;
  border: 1px solid var(--border);
}

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

.msg-user_request_input {
  background: rgba(255, 152, 0, 0.08);
  border: 1px solid rgba(255, 152, 0, 0.3);
}

.ask-user-label {
  color: var(--accent-orange);
  font-weight: 600;
}

.ask-user-question {
  margin-top: 4px;
}

.ask-options {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}

.ask-option-btn {
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 6px 12px;
  font-size: 0.8rem;
  color: var(--text-primary);
  cursor: pointer;
  transition: all 0.2s ease;
}

.ask-option-btn:hover {
  background: var(--bg-hover);
  border-color: var(--accent-blue);
  color: var(--accent-blue);
}
</style>
