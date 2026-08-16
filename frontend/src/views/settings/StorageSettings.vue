<script setup lang="ts">
import { ref, onMounted, computed, onBeforeUnmount } from 'vue'
import {
  NCard, NButton, NInput, NSpace, NTag, NModal,
  NForm, NFormItem, NSelect, NSwitch, NInputNumber, NTabs, NTabPane, NSpin,
  useMessage,
} from 'naive-ui'
import QRCode from 'qrcode'
import {
  listStorageTargets, createStorageTarget, updateStorageTarget,
  deleteStorageTarget, testStorageTarget,
  yun139SendSms, yun139SmsLogin, yun139PasswordLogin, yun139QrStart, yun139QrPoll,
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
      return '可用下方三种方式直接登录（短信验证码 / 账号密码 / 扫码），Token 自动写入；也可手动登录 yun.139.com 后从 F12 的 hcy/file/list 请求头复制 Authorization: Basic 后的内容。Token 有效期约 15 天。'
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

// ---------- 移动云盘（139）网页端登录 ----------

// 短信验证码登录
const smsAccount = ref('')
const smsCode = ref('')
const smsRandom = ref('')
const smsSending = ref(false)
const smsCountdown = ref(0)
let smsTimer: ReturnType<typeof setInterval> | null = null

async function sendSmsCode() {
  if (!smsAccount.value.trim()) {
    message.warning('请输入手机号')
    return
  }
  smsSending.value = true
  try {
    const res = await yun139SendSms(smsAccount.value.trim())
    const data = (res.data as any) || {}
    if (data.success) {
      smsRandom.value = data.data?.random || ''
      message.success('验证码已发送，请注意查收')
      smsCountdown.value = 60
      if (smsTimer) clearInterval(smsTimer)
      smsTimer = setInterval(() => {
        smsCountdown.value--
        if (smsCountdown.value <= 0 && smsTimer) clearInterval(smsTimer)
      }, 1000)
    } else {
      message.error(data.message || '验证码发送失败')
    }
  } catch {
    message.error('发送失败，请稍后重试')
  } finally {
    smsSending.value = false
  }
}

async function doSmsLogin() {
  if (!smsAccount.value.trim() || !smsCode.value.trim()) {
    message.warning('请输入手机号和验证码')
    return
  }
  try {
    const res = await yun139SmsLogin(smsAccount.value.trim(), smsCode.value.trim(), smsRandom.value, form.value.id || undefined)
    const data = (res.data as any) || {}
    if (data.success) {
      applyLoginToken(data.data)
      message.success(`登录成功（${data.data?.account || smsAccount.value}），Token 已写入`)
      if (!form.value.id) message.info('请点击"保存"完成存储创建')
    } else {
      message.error(data.message || '登录失败')
    }
  } catch {
    message.error('登录请求失败')
  }
}

// 账号密码登录
const pwdAccount = ref('')
const pwdPassword = ref('')
const pwdLogging = ref(false)

async function doPasswordLogin() {
  if (!pwdAccount.value.trim() || !pwdPassword.value) {
    message.warning('请输入手机号和密码')
    return
  }
  pwdLogging.value = true
  try {
    const res = await yun139PasswordLogin(pwdAccount.value.trim(), pwdPassword.value, form.value.id || undefined)
    const data = (res.data as any) || {}
    if (data.success) {
      applyLoginToken(data.data)
      message.success(`登录成功（${data.data?.account || pwdAccount.value}），Token 已写入`)
      if (!form.value.id) message.info('请点击"保存"完成存储创建')
    } else {
      message.error(data.message || '登录失败')
    }
  } catch {
    message.error('登录请求失败')
  } finally {
    pwdLogging.value = false
  }
}

// 二维码扫码登录
const qrImg = ref('')
const qrSid = ref('')
const qrPolling = ref(false) // 轮询进行中（仅用于状态提示，不遮挡二维码）
const qrStatus = ref('')
let qrTimer: ReturnType<typeof setInterval> | null = null

async function startQr() {
  qrImg.value = ''
  qrStatus.value = ''
  try {
    const res = await yun139QrStart()
    const data = (res.data as any) || {}
    if (!data.success || !data.data?.sid) {
      message.error(data.message || '二维码生成失败')
      return
    }
    qrSid.value = data.data.sid
    qrImg.value = await QRCode.toDataURL(data.data.content, { width: 200, margin: 1 })
    qrStatus.value = '请使用"中国移动云盘"APP 或微信扫码'
    qrPolling.value = true
    if (qrTimer) clearInterval(qrTimer)
    qrTimer = setInterval(pollQr, 2500)
  } catch {
    message.error('二维码生成失败')
  }
}

async function pollQr() {
  if (!qrSid.value) return
  try {
    const res = await yun139QrPoll(qrSid.value, form.value.id || undefined)
    const data = (res.data as any) || {}
    if (data.success && data.data?.authorization) {
      applyLoginToken(data.data)
      qrStatus.value = '登录成功！'
      qrPolling.value = false
      if (qrTimer) clearInterval(qrTimer)
      message.success(`登录成功（${data.data.account || ''}），Token 已写入`)
      if (!form.value.id) message.info('请点击"保存"完成存储创建')
    } else if (!data.success) {
      // 中间状态：等待扫码 / 已扫码待确认 / 失效等
      if (data.message) {
        qrStatus.value = data.message
        if (data.code === '200059542') {
          // 二维码失效，自动刷新
          qrPolling.value = false
          if (qrTimer) clearInterval(qrTimer)
          message.warning('二维码已失效，请重新生成')
        }
      }
    }
  } catch {
    // 轮询失败忽略，等待下次
  }
}

function applyLoginToken(data: any) {
  if (!data?.authorization) return
  const cfg = form.value.config as any
  cfg.token = data.authorization
  if (data.account) cfg.account = data.account
  if (data.user_domain_id) cfg.user_domain_id = data.user_domain_id
  if (data.personal_host) cfg.personal_host = data.personal_host
}

onBeforeUnmount(() => {
  if (smsTimer) clearInterval(smsTimer)
  if (qrTimer) clearInterval(qrTimer)
})
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

        <!-- 移动云盘网页端登录：短信验证码 / 账号密码 / 扫码 -->
        <n-card v-if="form.type === 'yun139'" size="small" class="yun139-login-card">
          <template #header>
            <span class="yun139-login-title">网页端登录（自动写入 Token）</span>
          </template>
          <n-tabs type="segment" size="small">
            <!-- 短信验证码 -->
            <n-tab-pane name="sms" tab="短信验证码">
              <n-form-item label="手机号">
                <n-input v-model:value="smsAccount" placeholder="请输入手机号" />
              </n-form-item>
              <n-form-item label="验证码">
                <div style="display: flex; gap: 8px; width: 100%">
                  <n-input v-model:value="smsCode" placeholder="短信验证码" />
                  <n-button :loading="smsSending" :disabled="smsCountdown > 0" @click="sendSmsCode" style="flex-shrink: 0">
                    {{ smsCountdown > 0 ? `${smsCountdown}s 后重发` : '获取验证码' }}
                  </n-button>
                </div>
              </n-form-item>
              <n-button type="primary" block @click="doSmsLogin">登录</n-button>
            </n-tab-pane>

            <!-- 账号密码 -->
            <n-tab-pane name="pwd" tab="账号密码">
              <n-form-item label="手机号">
                <n-input v-model:value="pwdAccount" placeholder="请输入手机号" />
              </n-form-item>
              <n-form-item label="密码">
                <n-input v-model:value="pwdPassword" type="password" show-password-on="click" placeholder="云盘账号密码" />
              </n-form-item>
              <n-button type="primary" block :loading="pwdLogging" @click="doPasswordLogin">登录</n-button>
            </n-tab-pane>

            <!-- 扫码登录 -->
            <n-tab-pane name="qr" tab="扫码登录">
              <div style="display: flex; flex-direction: column; align-items: center; gap: 10px; padding: 8px 0">
                <n-spin :show="!qrImg" size="small">
                  <img v-if="qrImg" :src="qrImg" alt="登录二维码" style="width: 200px; height: 200px; border-radius: 8px" />
                  <div v-else class="qr-placeholder">二维码将在此显示</div>
                </n-spin>
                <div v-if="qrStatus" class="qr-status">{{ qrStatus }}</div>
                <n-button type="primary" @click="startQr">生成二维码</n-button>
              </div>
            </n-tab-pane>
          </n-tabs>
        </n-card>

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
.yun139-login-card {
  margin-bottom: 16px;
}
.yun139-login-title {
  font-size: 13px;
  font-weight: 600;
}
.qr-placeholder {
  width: 200px;
  height: 200px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f5f5f7;
  border-radius: 8px;
  color: #999;
  font-size: 13px;
}
.qr-status {
  font-size: 12px;
  color: #666;
}
</style>
