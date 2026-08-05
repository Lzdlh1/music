<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  NCard, NButton, NSpace, NInput, NSwitch, NAlert, useMessage
} from 'naive-ui'
import api from '@/api'

const message = useMessage()

const proxyUrl = ref('')
const enabled = ref(false)
const loading = ref(false)
const testing = ref(false)

onMounted(loadConfig)

async function loadConfig() {
  loading.value = true
  try {
    const res = await api.get('/proxy')
    proxyUrl.value = res.data.url || ''
    enabled.value = res.data.enabled || false
  } catch {
    // no config yet
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  loading.value = true
  try {
    const res = await api.put('/proxy', { url: proxyUrl.value, enabled: enabled.value })
    if (res.data.success) {
      message.success('代理配置已保存')
    } else {
      message.error(res.data.message || '保存失败')
    }
  } catch {
    message.error('保存失败')
  } finally {
    loading.value = false
  }
}

async function handleTest() {
  if (!proxyUrl.value) {
    message.warning('请输入代理地址')
    return
  }
  testing.value = true
  try {
    const res = await api.post('/proxy/test', { url: proxyUrl.value })
    if (res.data.success) {
      message.success('代理连接成功')
    } else {
      message.error(res.data.message || '连接失败')
    }
  } catch {
    message.error('测试失败')
  } finally {
    testing.value = false
  }
}
</script>

<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px;">
      <h2 style="margin: 0;">代理配置</h2>
    </div>

    <n-card size="small">
      <n-alert type="info" :show-icon="false" style="margin-bottom: 16px;">
        直接输入代理链接，支持 HTTP / SOCKS5 代理。代理仅用于本项目的网络请求（Telegram API 等）。
      </n-alert>

      <div style="display: flex; gap: 8px; align-items: center; margin-bottom: 16px;">
        <n-input
          v-model:value="proxyUrl"
          placeholder="例如: http://127.0.0.1:7890 或 socks5://127.0.0.1:7891"
          style="flex: 1;"
        />
        <n-button @click="handleTest" :loading="testing" size="small">测试</n-button>
      </div>

      <div style="display: flex; justify-content: space-between; align-items: center;">
        <div style="display: flex; align-items: center; gap: 8px;">
          <span>启用代理</span>
          <n-switch v-model:value="enabled" />
        </div>
        <n-button type="primary" @click="handleSave" :loading="loading">保存</n-button>
      </div>
    </n-card>
  </div>
</template>
