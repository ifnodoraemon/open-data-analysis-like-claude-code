<template>
  <div class="login-shell">
    <div class="login-card">
      <div class="brand">数据分析智能体</div>
      <h1>登录</h1>
      <p class="hint">使用管理员账号或已配置账号进入工作区。</p>
      <form class="form" @submit.prevent="handleLogin">
        <label class="field">
          <span>账号</span>
          <input v-model.trim="email" type="text" autocomplete="username" placeholder="请输入账号" />
        </label>
        <label class="field">
          <span>密码</span>
          <input v-model="password" type="password" autocomplete="current-password" placeholder="请输入密码" />
        </label>
        <button class="submit" :disabled="loading || !email || !password">
          {{ loading ? '登录中...' : '登录' }}
        </button>
      </form>
      <p v-if="error" class="error">{{ error }}</p>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useWebSocket } from '../../composables/useWebSocket.js'

const emit = defineEmits(['success'])
const { login } = useWebSocket()
const email = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function handleLogin() {
  if (loading.value) return
  loading.value = true
  error.value = ''
  try {
    await login(email.value, password.value)
    emit('success')
  } catch (err) {
    error.value = err.message || '登录失败'
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-shell {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 24px;
  background: var(--bg-secondary);
}

.login-card {
  width: min(420px, 100%);
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 18px;
  padding: 28px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.08);
}

.brand {
  display: inline-block;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(37, 99, 235, 0.1);
  color: var(--accent-blue);
  font-size: 0.75rem;
  margin-bottom: 14px;
}

h1 {
  font-size: 1.6rem;
  margin-bottom: 6px;
  color: var(--text-primary);
}

.hint {
  color: var(--text-secondary);
  font-size: 0.9rem;
  margin-bottom: 20px;
}

.form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  color: var(--text-secondary);
  font-size: 0.8rem;
}

.field input {
  border: 1px solid var(--border);
  background: var(--bg-primary);
  color: var(--text-primary);
  border-radius: 10px;
  padding: 12px 14px;
  outline: none;
  transition: border-color 0.2s;
}

.field input:focus {
  border-color: var(--accent-blue);
}

.submit {
  margin-top: 8px;
  border: none;
  border-radius: 10px;
  background: var(--accent-blue);
  color: white;
  padding: 12px 16px;
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.2s;
}

.submit:hover:not(:disabled) {
  opacity: 0.9;
}

.submit:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.error {
  margin-top: 12px;
  color: #ff7b72;
  font-size: 0.82rem;
}
</style>
