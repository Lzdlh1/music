<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  NCard, NButton, NSpace, NTag, NModal, NForm, NFormItem,
  NInput, NInputNumber, NSwitch, NSelect, useMessage, NPopconfirm
} from 'naive-ui'
import { listSources, createSource, updateSource, deleteSource, testSource } from '@/api/source'
import type { MusicSourceConfig } from '@/types'

const message = useMessage()
const sources = ref<MusicSourceConfig[]>([])
const showModal = ref(false)
const editing = ref<MusicSourceConfig | null>(null)

const form = ref({
  name: '',
  type: 'meting',
  priority: 0,
  enabled: true,
  config: {
    base_url: '',
    api_key: '',
    timeout: 30,
    music_source: 'netease',
    cookie: '',
  },
})

const typeOptions = [
  { label: 'Meting 采集站', value: 'meting' },
  { label: '自定义 API', value: 'custom_api' },
  { label: '网易云 (NeteaseCloudMusicApi)', value: 'netease' },
]

const metingSourceOptions = [
  { label: '网易云音乐', value: 'netease' },
  { label: '酷我音乐', value: 'kuwo' },
  { label: 'Joox', value: 'joox' },
  { label: 'Bilibili', value: 'bilibili' },
  { label: '腾讯音乐', value: 'tencent' },
  { label: '酷狗音乐', value: 'kugou' },
  { label: '咪咕音乐', value: 'migu' },
]

async function loadSources() {
  const res = await listSources()
  sources.value = res.data.data || []
}

onMounted(loadSources)

function openCreate() {
  editing.value = null
  form.value = { name: '', type: 'meting', priority: 0, enabled: true, config: { base_url: '', api_key: '', timeout: 30, music_source: 'netease', cookie: '' } }
  showModal.value = true
}

function openEdit(src: MusicSourceConfig) {
  editing.value = src
  const cfg = typeof src.config === 'string' ? JSON.parse(src.config) : src.config
  form.value = {
    name: src.name,
    type: src.type,
    priority: src.priority,
    enabled: src.enabled,
    config: {
      base_url: cfg.base_url || '',
      api_key: cfg.api_key || '',
      timeout: cfg.timeout || 30,
      music_source: cfg.music_source || 'netease',
      cookie: cfg.cookie || '',
    },
  }
  showModal.value = true
}

async function handleSave() {
  const payload = { ...form.value, config: form.value.config }
  if (editing.value) {
    await updateSource(editing.value.id, payload)
    message.success('已更新')
  } else {
    await createSource(payload)
    message.success('已添加')
  }
  showModal.value = false
  await loadSources()
}

async function handleDelete(id: string) {
  await deleteSource(id)
  message.success('已删除')
  await loadSources()
}

async function handleTest(id: string) {
  try {
    const res = await testSource(id)
    const result = (res.data.data || res.data) as any
    if (result?.success) {
      message.success(result.message || '连接成功')
    } else {
      message.error(result?.message || '连接失败')
    }
  } catch {
    message.error('测试失败')
  }
}
</script>

<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px;">
      <h2 style="margin: 0;">音乐来源配置</h2>
      <n-button type="primary" @click="openCreate">+ 添加来源</n-button>
    </div>

    <n-space vertical>
      <n-card v-for="src in sources" :key="src.id" size="small">
        <div style="display: flex; justify-content: space-between; align-items: center;">
          <div>
            <strong>{{ src.name }}</strong>
            <n-tag size="tiny" :type="src.enabled ? 'success' : 'default'" round style="margin-left: 8px;">
              {{ src.type }}
            </n-tag>
            <n-tag v-if="!src.enabled" size="tiny" type="warning" round style="margin-left: 4px;">已禁用</n-tag>
            <span style="margin-left: 8px; color: #999; font-size: 12px;">优先级: {{ src.priority }}</span>
          </div>
          <n-space>
            <n-button size="tiny" @click="handleTest(src.id)">测试</n-button>
            <n-button size="tiny" @click="openEdit(src)">编辑</n-button>
            <n-popconfirm @positive-click="handleDelete(src.id)">
              <template #trigger>
                <n-button size="tiny" type="error">删除</n-button>
              </template>
              确定删除此音乐源？
            </n-popconfirm>
          </n-space>
        </div>
      </n-card>

      <div v-if="sources.length === 0" style="text-align: center; color: #999; padding: 40px 0;">
        暂无配置的音乐源，点击上方按钮添加
      </div>
    </n-space>

    <n-modal v-model:show="showModal" preset="dialog" :title="editing ? '编辑音乐源' : '添加音乐源'" style="width: 520px;">
      <n-form label-placement="left" label-width="90">
        <n-form-item label="名称">
          <n-input v-model:value="form.name" placeholder="例如: GD音乐台" />
        </n-form-item>
        <n-form-item label="类型">
          <n-select v-model:value="form.type" :options="typeOptions" />
        </n-form-item>
        <n-form-item label="API 地址">
          <n-input v-model:value="form.config.base_url" :placeholder="form.type === 'meting' ? 'https://music-api.gdstudio.xyz/api.php' : 'https://api.example.com'" />
        </n-form-item>
        <n-form-item v-if="form.type === 'meting'" label="音乐平台">
          <n-select v-model:value="form.config.music_source" :options="metingSourceOptions" />
        </n-form-item>
        <n-form-item v-if="form.type === 'custom_api'" label="API Key">
          <n-input v-model:value="form.config.api_key" placeholder="可选" />
        </n-form-item>
        <n-form-item v-if="form.type === 'netease'" label="Cookie">
          <n-input v-model:value="form.config.cookie" type="textarea" :rows="2" placeholder="可选，登录后可获取更高音质" />
        </n-form-item>
        <n-form-item label="超时(秒)">
          <n-input-number v-model:value="form.config.timeout" :min="5" :max="120" />
        </n-form-item>
        <n-form-item label="优先级">
          <n-input-number v-model:value="form.priority" :min="0" :max="100" />
        </n-form-item>
        <n-form-item label="启用">
          <n-switch v-model:value="form.enabled" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-button @click="showModal = false">取消</n-button>
        <n-button type="primary" @click="handleSave">保存</n-button>
      </template>
    </n-modal>
  </div>
</template>
