<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { NCard, NForm, NFormItem, NInput, NButton, NAlert, NTabs, NTabPane, useMessage } from 'naive-ui'
import { Icon } from '@iconify/vue'
import { login, getAuthStatus, setup, register } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const message = useMessage()
const auth = useAuthStore()

const needSetup = ref(false) // 系统未初始化（users 表为空）
const loading = ref(false)
const mode = ref<'login' | 'register'>('login')

const loginForm = ref({ username: '', password: '' })
const regForm = ref({ code: '', username: '', password: '', confirm: '' })
const setupForm = ref({ username: '', password: '', confirm: '' })

const isInitializing = computed(() => needSetup.value)

onMounted(async () => {
  // 已有 token 直接进首页（后端会二次校验）
  if (auth.loggedIn) {
    router.replace('/search')
    return
  }
  // 检测是否需要首次初始化（创建管理员）
  try {
    const res = await getAuthStatus()
    needSetup.value = !!res.data?.need_setup
  } catch {
    /* 接口异常时按普通登录处理 */
  }
})

function afterLogin(res: any) {
  const d = res.data?.data || res.data
  auth.setSession({
    token: d.token,
    user_id: d.user_id,
    username: d.username,
    role: d.role,
  })
  message.success(d.role === 'admin' ? '欢迎回来，管理员' : '登录成功')
  router.replace('/search')
}

async function handleLogin() {
  if (!loginForm.value.username || !loginForm.value.password) {
    message.warning('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    const res = await login(loginForm.value.username, loginForm.value.password)
    afterLogin(res)
  } catch (e: any) {
    message.error(e?.response?.data?.message || '用户名或密码错误')
  } finally {
    loading.value = false
  }
}

async function handleRegister() {
  const f = regForm.value
  if (!f.code || !f.username || !f.password) {
    message.warning('请完整填写邀请码、用户名和密码')
    return
  }
  if (f.username.length < 2) {
    message.warning('用户名至少 2 位')
    return
  }
  if (f.password.length < 6) {
    message.warning('密码至少 6 位')
    return
  }
  if (f.password !== f.confirm) {
    message.warning('两次输入的密码不一致')
    return
  }
  loading.value = true
  try {
    const res = await register(f.code, f.username, f.password)
    afterLogin(res)
  } catch (e: any) {
    message.error(e?.response?.data?.message || '注册失败')
  } finally {
    loading.value = false
  }
}

async function handleSetup() {
  const f = setupForm.value
  if (!f.username || !f.password) {
    message.warning('请设置管理员用户名和密码')
    return
  }
  if (f.username.length < 2) {
    message.warning('用户名至少 2 位')
    return
  }
  if (f.password.length < 6) {
    message.warning('密码至少 6 位')
    return
  }
  if (f.password !== f.confirm) {
    message.warning('两次输入的密码不一致')
    return
  }
  loading.value = true
  try {
    const res = await setup(f.username, f.password)
    afterLogin(res)
  } catch (e: any) {
    message.error(e?.response?.data?.message || '初始化失败')
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
        <h1>商角</h1>
      </div>

      <!-- 首次使用：初始化管理员 -->
      <template v-if="isInitializing">
        <n-alert type="info" :show-icon="false" style="margin-bottom: 16px">
          首次使用，请先创建超级管理员账号。
        </n-alert>
        <n-form @submit.prevent="handleSetup">
          <n-form-item label="管理员用户名">
            <n-input v-model:value="setupForm.username" placeholder="admin" />
          </n-form-item>
          <n-form-item label="密码">
            <n-input v-model:value="setupForm.password" type="password" show-password-on="click" placeholder="至少 6 位" />
          </n-form-item>
          <n-form-item label="确认密码">
            <n-input v-model:value="setupForm.confirm" type="password" show-password-on="click" placeholder="再次输入密码" @keyup.enter="handleSetup" />
          </n-form-item>
          <n-button type="primary" block :loading="loading" @click="handleSetup">
            创建管理员并进入
          </n-button>
        </n-form>
      </template>

      <!-- 登录 / 邀请码注册 -->
      <template v-else>
        <n-tabs v-model:value="mode" type="segment" style="margin-bottom: 20px">
          <n-tab-pane name="login" tab="登录">
            <n-form @submit.prevent="handleLogin">
              <n-form-item label="用户名">
                <n-input v-model:value="loginForm.username" placeholder="请输入用户名" />
              </n-form-item>
              <n-form-item label="密码">
                <n-input v-model:value="loginForm.password" type="password" show-password-on="click" placeholder="请输入密码" @keyup.enter="handleLogin" />
              </n-form-item>
              <n-button type="primary" block :loading="loading" @click="handleLogin">
                登录
              </n-button>
            </n-form>
          </n-tab-pane>
          <n-tab-pane name="register" tab="邀请码注册">
            <n-form @submit.prevent="handleRegister">
              <n-form-item label="邀请码">
                <n-input v-model:value="regForm.code" placeholder="向管理员索取一次性邀请码" />
              </n-form-item>
              <n-form-item label="用户名">
                <n-input v-model:value="regForm.username" placeholder="至少 2 位" />
              </n-form-item>
              <n-form-item label="密码">
                <n-input v-model:value="regForm.password" type="password" show-password-on="click" placeholder="至少 6 位" />
              </n-form-item>
              <n-form-item label="确认密码">
                <n-input v-model:value="regForm.confirm" type="password" show-password-on="click" placeholder="再次输入密码" @keyup.enter="handleRegister" />
              </n-form-item>
              <n-button type="primary" block :loading="loading" @click="handleRegister">
                使用邀请码注册
              </n-button>
            </n-form>
          </n-tab-pane>
        </n-tabs>
        <div class="login-tip">
          还没有账号？让管理员在「设置 → 用户管理」派发邀请码后，使用邀请码注册。
        </div>
      </template>
    </n-card>
  </div>
</template>

<style scoped>
.login-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: linear-gradient(135deg, #eef2ff 0%, #faf5ff 100%);
}

.login-card {
  width: 400px;
  border-radius: 16px;
  box-shadow: 0 12px 40px rgba(99, 102, 241, 0.15);
}

.login-logo {
  text-align: center;
  margin-bottom: 8px;
}

.login-logo h1 {
  margin: 8px 0 20px;
  font-size: 26px;
  font-weight: 700;
  color: #6366f1;
}

.login-tip {
  margin-top: 16px;
  font-size: 12px;
  color: #999;
  text-align: center;
  line-height: 1.7;
}
</style>