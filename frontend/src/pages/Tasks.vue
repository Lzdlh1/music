<template>
  <div>
    <h2>任务</h2>
    <div>
      <input v-model="title" placeholder="标题" />
      <input v-model="url" placeholder="下载 URL" />
      <button @click="create">创建任务</button>
    </div>

    <h3>我的任务</h3>
    <ul>
      <li v-for="t in tasks" :key="t.id">
        <div style="display:flex; gap:8px; align-items:center">
          <strong>{{ t.title }}</strong>
          <span style="color: gray">— {{ t.status }}</span>
        </div>
        <div v-if="t.status === 'failed'" style="color: #b91c1c; margin-top:4px">错误: {{ t.error_message }}</div>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'
import { useAuthStore } from '../stores/auth'
import { useSettingsStore } from '../stores/settings'

const auth = useAuthStore()
const settings = useSettingsStore()

const title = ref('')
const url = ref('')
const tasks = ref([] as any[])

function authHeader() {
  return auth.token ? { Authorization: `Bearer ${auth.token}` } : {}
}

async function load() {
  const res = await api.get('/tasks', { headers: authHeader() })
  tasks.value = res.data
}

async function create() {
  try {
    const payload: any = { title: title.value, url: url.value }
    if (settings.cmcookies) payload.cookie = settings.cmcookies
    await api.post('/tasks', payload)
    title.value = ''
    url.value = ''
    alert('任务已创建，后台处理中')
    await load()
  } catch (e) {
    alert('创建失败：' + (e?.response?.data?.error || '请确认已登录'))
  }
}

onMounted(load)
</script>
