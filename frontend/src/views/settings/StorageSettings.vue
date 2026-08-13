<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import {
  NCard, NButton, NInput, NSpace, NTag, NModal,
  NForm, NFormItem, NSelect, NSwitch, NInputNumber,
  useMessage,
} from 'naive-ui'
import {
  listStorageTargets, createStorageTarget, updateStorageTarget,
  deleteStorageTarget, testStorageTarget,
} from '@/api/storage'
import type { StorageTarget } from '@/types'

const message = useMessage()
const targets = ref<StorageTarget[]>([])
const showDialog = ref(false)
const editing = ref(false)

const form = ref({
  id: '',
  name: '',
  type: 'local',
  enabled: true,
  config: {} as Record<string, unknown>,
})

const typeOptions = [
  { label: '本地存储', value: 'local' },
  { label: 'WebDAV（坚果云/Nextcloud等）', value: 'webdav' },
  { label: 'SFTP', value: 'sftp' },
  { label: 'S3 / MinIO', value: 's3' },
  { label: '阿里云盘', value: 'alipan' },
  { label: 'OneDrive', value: 'onedrive' },
  { label: '中国移动云盘（139）', value: 'yun139' },
  { label: '天翼云盘', value: 'tianyi' },
]

const configFields = computed(() => {
  switch (form.value.type) {
    case 'local':
      return [{ key: 'base_path', label: '存储路径', type: 'text' }]
    case 'webdav':
      return [
        { key: 'endpoint', label: 'WebDAV 地址', type: 'text' },
        { key: 'username', label: '用户名', type: 'text' },
        { key: 'password', label: '密码', type: 'password' },
        { key: 'base_path', label: '基础路径', type: 'text' },
      ]
    case 'sftp':
      return [
        { key: 'host', label: '主机', type: 'text' },
        { key: 'port', label: '端口', type: 'number' },
        { key: 'username', label: '用户名', type: 'text' },
        { key: 'password', label: '密码', type: 'password' },
        { key: 'private_key', label: '私钥 (可选)', type: 'textarea' },
        { key: 'base_path', label: '基础路径', type: 'text' },
      ]
    case 's3':
      return [
        { key: 'endpoint', label: '端点 (MinIO/自定义)', type: 'text' },
        { key: 'region', label: '区域', type: 'text' },
        { key: 'bucket', label: 'Bucket', type: 'text' },
        { key: 'access_key_id', label: 'Access Key ID', type: 'text' },
        { key: 'secret_access_key', label: 'Secret Access Key', type: 'password' },
        { key: 'base_path', label: '基础路径 (可选)', type: 'text' },
      ]
    case 'alipan':
      return [
        { key: 'refresh_token', label: 'Refresh Token', type: 'password' },
        { key: 'client_id', label: 'Client ID', type: 'text' },
        { key: 'client_secret', label: 'Client Secret (可选)', type: 'password' },
        { key: 'root_folder_id', label: '根文件夹ID (可选, 默认root)', type: 'text' },
        { key: 'drive_id', label: 'Drive ID (可选, 自动获取)', type: 'text' },
      ]
    case 'onedrive':
      return [
        { key: 'refresh_token', label: 'Refresh Token', type: 'password' },
        { key: 'client_id', label: 'Client ID', type: 'text' },
        { key: 'client_secret', label: 'Client Secret (可选)', type: 'password' },
        { key: 'root_path', label: '挂载子目录 (可选)', type: 'text' },
      ]
    case 'yun139':
      return [
        { key: 'token', label: 'Token', type: 'textarea' },
      ]
    case 'tianyi':
      return [
        { key: 'cookie', label: 'Cookie', type: 'textarea' },
      ]
    default:
      return []
  }
})

const typeHints = computed(() => {
  switch (form.value.type) {
    case 'alipan':
      return '在 https://www.alipan.com/developer/ 创建应用获得 Client ID/Secret；Refresh Token 可通过第三方扫码工具获取。'
    case 'onedrive':
      return '需在 Azure 门户注册应用并授予 files.readwrite 权限，通过 OAuth 流程获取 Refresh Token。'
    case 'yun139':
      return '登录 yun.139.com 后按 F12 打开开发者工具，在 Network 中找到 hcy/file/list 请求，复制请求头 Authorization: Basic 后面的内容填入。有效期约 15 天。'
    case 'tianyi':
      return '登录 cloud.189.cn 后，在浏览器开发者工具中复制 Cookie 请求头内容填入。'
    default:
      return ''
  }
})

onMounted(loadTargets)

async function loadTargets() {
  const res = await listStorageTargets()
  targets.value = res.data.data || []
}

function openAdd() {
  editing.value = false
  form.value = { id: '', name: '', type: 'local', enabled: true, config: {} }
  showDialog.value = true
}

function openEdit(target: StorageTarget) {
  editing.value = true
  form.value = {
    id: target.id,
    name: target.name,
    type: target.type,
    enabled: target.enabled,
    config: (typeof target.config === 'object' ? { ...target.config as Record<string, unknown> } : {}),
  }
  showDialog.value = true
}

async function handleSave() {
  const payload = {
    name: form.value.name,
    type: form.value.type,
    enabled: form.value.enabled,
    config: form.value.config,
  } as any
  try {
    if (editing.value) {
      await updateStorageTarget(form.value.id, payload)
      message.success('已更新')
    } else {
      await createStorageTarget(payload)
      message.success('已创建')
    }
    showDialog.value = false
    await loadTargets()
  } catch {
    message.error('保存失败')
  }
}

async function handleDelete(id: string) {
  try {
    await deleteStorageTarget(id)
    message.success('已删除')
    await loadTargets()
  } catch {
    message.error('删除失败')
  }
}

async function handleTest(id: string) {
  try {
    const res = await testStorageTarget(id)
    const result = (res.data.data || res.data) as any
    if (result?.success) {
      message.success(result.message || '连接成功')
    } else {
      message.error(result?.message || '连接失败')
    }
  } catch {
    message.error('测试请求失败')
  }
}
</script>

<template>
  <div>
    <h2>存储目标</h2>
    <n-space vertical>
      <n-card v-for="target in targets" :key="target.id" size="small">
        <div class="storage-row">
          <div>
            <strong>{{ target.name }}</strong>
            <n-tag size="tiny" :type="target.enabled ? 'success' : 'default'" round style="margin-left: 8px">
              {{ target.type }}
            </n-tag>
          </div>
          <n-space>
            <n-button size="tiny" @click="handleTest(target.id)">测试</n-button>
            <n-button size="tiny" @click="openEdit(target)">编辑</n-button>
            <n-button size="tiny" type="error" @click="handleDelete(target.id)">删除</n-button>
          </n-space>
        </div>
      </n-card>
      <n-button type="primary" @click="openAdd">+ 添加存储目标</n-button>
    </n-space>

    <n-modal v-model:show="showDialog" preset="card" :title="editing ? '编辑存储目标' : '添加存储目标'" style="width: 500px">
      <n-form label-placement="left" label-width="120">
        <n-form-item label="名称">
          <n-input v-model:value="form.name" placeholder="存储名称" />
        </n-form-item>
        <n-form-item label="类型">
          <n-select v-model:value="form.type" :options="typeOptions" :disabled="editing" />
        </n-form-item>
        <div v-if="typeHints" class="type-hint">💡 {{ typeHints }}</div>
        <n-form-item label="启用">
          <n-switch v-model:value="form.enabled" />
        </n-form-item>

        <template v-for="field in configFields" :key="field.key">
          <n-form-item :label="field.label">
            <n-input
              v-if="field.type === 'text' || field.type === 'password'"
              v-model:value="(form.config as any)[field.key]"
              :type="field.type"
              :placeholder="field.label"
            />
            <n-input
              v-else-if="field.type === 'textarea'"
              v-model:value="(form.config as any)[field.key]"
              type="textarea"
              :rows="3"
              :placeholder="field.label"
            />
            <n-input-number
              v-else-if="field.type === 'number'"
              v-model:value="(form.config as any)[field.key]"
              :placeholder="field.label"
              style="width: 100%"
            />
          </n-form-item>
        </template>

        <n-form-item>
          <n-space>
            <n-button type="primary" @click="handleSave">保存</n-button>
            <n-button @click="showDialog = false">取消</n-button>
          </n-space>
        </n-form-item>
      </n-form>
    </n-modal>
  </div>
</template>

<style scoped>
h2 { margin-bottom: 16px; }
.storage-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.type-hint {
  font-size: 12px;
  color: #999;
  background: rgba(99, 102, 241, 0.08);
  border-radius: 6px;
  padding: 8px 10px;
  margin-bottom: 12px;
  line-height: 1.6;
}
</style>
