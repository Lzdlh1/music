<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { NCard, NForm, NFormItem, NInput, NButton, useMessage } from 'naive-ui'
import { Icon } from '@iconify/vue'
import { login, getAuthStatus } from '@/api/auth'

const router = useRouter()
const message = useMessage()

const form = ref({ username: '', password: '' })
const loading = ref(false)

onMounted(async () => {
  // 若认证未启用，直接跳转
  try {
    const res = await getAuthStatus()
    if (!res.data.auth_enabled) {
      router.replace('/search')
      return
    }
  } catch { /* proceed to login */ }

  // 已有 token 则跳转
  if (localStorage.getItem('mf_token')) {
    router.replace('/search')
  }
})

async function handleLogin() {
  if (!form.value.username || !form.value.password) {
    message.warning('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    const res = await login(form.value.username, form.value.password)
    localStorage.setItem('mf_token', res.data.token)
    message.success('登录成功')
    router.replace('/search')
  } catch {
    message.error('用户名或密码错误')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <n-card class="login-card" size="large">
      <div class="login-logo">
        <Icon icon="material-symbols:music-note" :width="48" color="#6366f1" />
        <h1>MusicFlow</h1>
      </div>
      <n-form @submit.prevent="handleLogin">
        <n-form-item label="用户名">
          <n-input v-model:value="form.username" placeholder="admin" />
        </n-form-item>
        <n-form-item label="密码">
          <n-input v-model:value="form.password" type="password" show-password-on="click" placeholder="密码" @keyup.enter="handleLogin" />
        </n-form-item>
        <n-button type="primary" block :loading="loading" @click="handleLogin">
          登录
        </n-button>
      </n-form>
    </n-card>
  </div>
</template>

<style scoped>
.login-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: #f5f5f5;
}

.login-card {
  width: 380px;
}

.login-logo {
  text-align: center;
  margin-bottom: 24px;
}

.login-logo h1 {
  margin-top: 8px;
  font-size: 24px;
  font-weight: 700;
  color: #6366f1;
}
</style>
