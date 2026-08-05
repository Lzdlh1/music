<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import {
  NCard, NButton, NSpace, NInput, NFormItem, NForm,
  NTabs, NTabPane, NTag, NSwitch, NPagination, NEmpty,
  NInputGroup, NModal, useMessage
} from 'naive-ui'
import {
  listBots, createBot, deleteBot, testBotToken,
  listAccounts, createAccount, deleteAccount, startAccount, submitCode, submitPassword,
  listChannels, addChannel, removeChannel, toggleChannel, scanChannelHistory,
  listAllChannelFiles,
  getFileDownloadURL, saveFileToLibrary,
  type TGBot, type TGAccount, type TGChannel, type TGChannelFile,
} from '@/api/telegram'

const message = useMessage()

const bots = ref<TGBot[]>([])
const accounts = ref<TGAccount[]>([])
const channels = ref<TGChannel[]>([])
const channelFiles = ref<TGChannelFile[]>([])
const privateFiles = ref<TGChannelFile[]>([])
const filesTotalCount = ref(0)
const privateFilesTotalCount = ref(0)
const filesPage = ref(1)
const privateFilesPage = ref(1)
const filesKeyword = ref('')
const privateFilesKeyword = ref('')
const filesLoading = ref(false)
const privateFilesLoading = ref(false)

const botForm = ref({ name: '', username: '', token: '' })
const accountForm = ref({ phone: '' })
const channelForm = ref({ chatId: '' })

// 登录验证码/密码相关
const authCodeForm = ref({ id: '', code: '' })
const authPwdForm = ref({ id: '', password: '' })
const showCodeModal = ref(false)
const showPwdModal = ref(false)

onMounted(async () => {
  await Promise.all([loadBots(), loadAccounts(), loadChannels()])
})

// === Bot 管理 ===
async function loadBots() {
  try {
    const res = await listBots()
    bots.value = res.data.data || []
  } catch { /* empty */ }
}

async function handleSaveBot() {
  if (!botForm.value.token) {
    message.warning('请输入 Bot Token')
    return
  }
  try {
    const testRes = await testBotToken(botForm.value.token)
    const result = testRes.data.data || testRes.data
    const testData = result as any
    if (!testData?.success) {
      message.error('Token 无效: ' + (testData?.message || ''))
      return
    }

    const username = testData.data?.username || botForm.value.username
    await createBot({
      name: botForm.value.name || username,
      username,
      config: { token: botForm.value.token, chat_ids: [] },
    })
    message.success(`Bot @${username} 已添加`)
    botForm.value = { name: '', username: '', token: '' }
    await loadBots()
  } catch {
    message.error('保存失败')
  }
}

async function handleDeleteBot(id: string) {
  await deleteBot(id)
  message.success('已删除')
  await loadBots()
}

// === 账号管理 ===
async function loadAccounts() {
  try {
    const res = await listAccounts()
    accounts.value = res.data.data || []
  } catch { /* empty */ }
}

async function handleSaveAccount() {
  if (!accountForm.value.phone) {
    message.warning('请输入手机号')
    return
  }
  try {
    await createAccount({ phone: accountForm.value.phone })
    message.success('账号已添加')
    accountForm.value = { phone: '' }
    await loadAccounts()
  } catch {
    message.error('添加失败')
  }
}

async function handleDeleteAccount(id: string) {
  await deleteAccount(id)
  message.success('已删除')
  await loadAccounts()
}

async function handleStartAccount(id: string) {
  try {
    await startAccount(id)
    message.success('登录流程已启动，请等待发送验证码')
    await loadAccounts()
  } catch {
    message.error('启动失败')
  }
}

async function handleShowCode(id: string) {
  authCodeForm.value.id = id
  authCodeForm.value.code = ''
  showCodeModal.value = true
}

async function handleSubmitCode() {
  if (!authCodeForm.value.code) return
  try {
    await submitCode(authCodeForm.value.id, authCodeForm.value.code)
    message.success('验证码已提交，请刷新状态')
    showCodeModal.value = false
    await loadAccounts()
  } catch {
    message.error('提交失败')
  }
}

async function handleShowPassword(id: string) {
  authPwdForm.value.id = id
  authPwdForm.value.password = ''
  showPwdModal.value = true
}

async function handleSubmitPassword() {
  if (!authPwdForm.value.password) return
  try {
    await submitPassword(authPwdForm.value.id, authPwdForm.value.password)
    message.success('密码已提交，请刷新状态')
    showPwdModal.value = false
    await loadAccounts()
  } catch {
    message.error('提交失败')
  }
}

// === 频道管理 ===
async function loadChannels() {
  try {
    const res = await listChannels()
    channels.value = res.data.data || []
  } catch { /* empty */ }
}

async function handleAddChannel() {
  const id = channelForm.value.chatId.trim()
  if (!id) {
    message.warning('请输入频道用户名或 Chat ID')
    return
  }
  try {
    const res = await addChannel(id)
    const result = (res.data as any)
    if (result.success === false) {
      message.error(result.message || '添加失败')
      return
    }
    message.success('频道已添加')
    channelForm.value = { chatId: '' }
    await loadChannels()
  } catch {
    message.error('添加失败')
  }
}

async function handleRemoveChannel(id: string) {
  await removeChannel(id)
  message.success('已删除')
  await loadChannels()
}

async function handleToggleChannel(id: string, enabled: boolean) {
  await toggleChannel(id, enabled)
}

async function handleScanHistory(id: string) {
  try {
    await scanChannelHistory(id)
    message.success('已开始后台扫描历史记录')
  } catch {
    message.error('启动扫描失败')
  }
}

// === 频道文件浏览 ===
async function loadFiles() {
  filesLoading.value = true
  try {
    const res = await listAllChannelFiles({
      page: filesPage.value,
      page_size: 50,
      keyword: filesKeyword.value || undefined,
    })
    const result = res.data as any
    channelFiles.value = result.data || []
    filesTotalCount.value = result.total || 0
  } catch { /* empty */ }
  filesLoading.value = false
}

function handleFilesSearch() {
  filesPage.value = 1
  loadFiles()
}

function handlePageChange(page: number) {
  filesPage.value = page
  loadFiles()
}

async function handleDownloadFile(file: TGChannelFile) {
  try {
    const res = await getFileDownloadURL(file.id)
    const result = (res.data as any)
    if (result.success) {
      window.open(result.download_url, '_blank')
      message.success('开始下载')
    } else {
      message.error(result.message || '获取下载链接失败')
    }
  } catch {
    message.error('下载失败')
  }
}

async function handleSaveFile(file: TGChannelFile) {
  try {
    const res = await saveFileToLibrary(file.id)
    const result = (res.data as any)
    if (result.success) {
      message.success('已加入下载队列')
      file.downloaded = true
    } else {
      message.error(result.message || '保存失败')
    }
  } catch {
    message.error('操作失败')
  }
}

// === 转发资源（私聊文件）===
async function loadPrivateFiles() {
  privateFilesLoading.value = true
  try {
    const res = await listAllChannelFiles({
      page: privateFilesPage.value,
      page_size: 50,
      keyword: privateFilesKeyword.value || undefined,
      channel_id: 'private',
    })
    const result = res.data as any
    privateFiles.value = result.data || []
    privateFilesTotalCount.value = result.total || 0
  } catch { /* empty */ }
  privateFilesLoading.value = false
}

function handlePrivateFilesSearch() {
  privateFilesPage.value = 1
  loadPrivateFiles()
}

function handlePrivatePageChange(page: number) {
  privateFilesPage.value = page
  loadPrivateFiles()
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1024 / 1024).toFixed(1) + ' MB'
}

function formatDuration(seconds: number): string {
  if (!seconds) return ''
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}:${s.toString().padStart(2, '0')}`
}

function formatTime(dateStr: string): string {
  if (!dateStr) return ''
  return new Date(dateStr).toLocaleDateString('zh-CN')
}

// 需要有bot才能使用频道功能
const hasBots = computed(() => bots.value.length > 0)
</script>

<template>
  <div>
    <h2>Telegram 配置</h2>

    <n-tabs type="segment">
      <!-- Bot 管理 -->
      <n-tab-pane name="bots" tab="Bot 管理">
        <n-card title="添加 Telegram Bot" size="small" style="margin-top: 12px;">
          <n-form label-placement="left" label-width="100">
            <n-form-item label="名称">
              <n-input v-model:value="botForm.name" placeholder="Bot 名称 (可选)" />
            </n-form-item>
            <n-form-item label="Bot Token">
              <n-input v-model:value="botForm.token" type="password" show-password-on="click" placeholder="从 @BotFather 获取" />
            </n-form-item>
            <n-form-item>
              <n-button type="primary" @click="handleSaveBot">验证并保存</n-button>
            </n-form-item>
          </n-form>
        </n-card>

        <n-space vertical style="margin-top: 16px;">
          <n-card v-for="bot in bots" :key="bot.id" size="small">
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <div>
                <strong>{{ bot.name }}</strong>
                <n-tag size="tiny" round style="margin-left: 8px;" :type="bot.enabled ? 'success' : 'default'">
                  @{{ bot.username }}
                </n-tag>
                <span v-if="bot.last_tested" style="margin-left: 8px; color: #999; font-size: 12px;">
                  成功率: {{ (bot.success_rate * 100).toFixed(0) }}%
                </span>
              </div>
              <n-button size="tiny" type="error" @click="handleDeleteBot(bot.id)">删除</n-button>
            </div>
          </n-card>
          <div v-if="bots.length === 0" style="text-align: center; color: #999; padding: 20px 0;">
            暂无已配置的 Bot
          </div>
        </n-space>
      </n-tab-pane>

      <!-- 频道资源 -->
      <n-tab-pane name="channels" tab="频道资源" @update:value="loadFiles">
        <div v-if="!hasBots" style="padding: 40px 0; text-align: center; color: #999;">
          <p>请先在"Bot 管理"中添加一个 Telegram Bot</p>
          <p style="font-size: 12px;">Bot 需要是频道管理员才能获取资源</p>
        </div>

        <template v-else>
          <!-- 添加频道 -->
          <n-card title="添加资源频道" size="small" style="margin-top: 12px;">
            <n-form label-placement="left" label-width="120">
              <n-form-item label="频道用户名/ID">
                <n-input-group>
                  <n-input v-model:value="channelForm.chatId" placeholder="@channel_name 或 -100xxxxxxxxxx" />
                  <n-button type="primary" @click="handleAddChannel">添加</n-button>
                </n-input-group>
              </n-form-item>
            </n-form>
            <div style="font-size: 12px; color: #999; margin-top: -8px;">
              Bot 必须是频道管理员。添加后系统会自动抓取频道中的音频文件。
            </div>
          </n-card>

          <!-- 已订阅频道列表 -->
          <n-space vertical style="margin-top: 16px;">
            <n-card v-for="ch in channels" :key="ch.id" size="small">
              <div style="display: flex; justify-content: space-between; align-items: center;">
                <div>
                  <strong>{{ ch.title }}</strong>
                  <n-tag v-if="ch.username" size="tiny" round style="margin-left: 8px;">
                    @{{ ch.username }}
                  </n-tag>
                  <span style="margin-left: 8px; color: #999; font-size: 12px;">
                    {{ ch.file_count }} 个文件
                  </span>
                </div>
                <n-space>
                  <n-switch :value="ch.enabled" size="small" @update:value="(v: boolean) => handleToggleChannel(ch.id, v)" />
                  <n-button size="tiny" @click="handleScanHistory(ch.id)">扫描历史</n-button>
                  <n-button size="tiny" type="error" @click="handleRemoveChannel(ch.id)">删除</n-button>
                </n-space>
              </div>
            </n-card>
            <div v-if="channels.length === 0" style="text-align: center; color: #999; padding: 16px 0;">
              暂未订阅任何频道
            </div>
          </n-space>

          <!-- 频道文件浏览 -->
          <n-card title="频道音频资源" size="small" style="margin-top: 16px;" v-if="channels.length > 0">
            <template #header-extra>
              <n-input-group style="width: 300px;">
                <n-input v-model:value="filesKeyword" placeholder="搜索文件名/标题/艺术家" size="small" clearable @keyup.enter="handleFilesSearch" />
                <n-button size="small" type="primary" @click="handleFilesSearch">搜索</n-button>
              </n-input-group>
            </template>

            <n-button size="small" style="margin-bottom: 12px;" @click="loadFiles" :loading="filesLoading">
              刷新列表
            </n-button>

            <div v-if="channelFiles.length === 0 && !filesLoading" style="text-align: center; color: #999; padding: 20px 0;">
              <n-empty description="暂无音频文件，频道新消息会自动抓取" />
            </div>

            <div v-for="file in channelFiles" :key="file.id" class="file-item">
              <div class="file-info">
                <div class="file-title">
                  {{ file.title || file.file_name || '未知标题' }}
                  <n-tag v-if="file.downloaded" size="tiny" type="success" round style="margin-left: 6px;">已下载</n-tag>
                </div>
                <div class="file-meta">
                  <span v-if="file.artist">{{ file.artist }}</span>
                  <span v-if="file.duration">{{ formatDuration(file.duration) }}</span>
                  <span>{{ formatSize(file.file_size) }}</span>
                  <span>{{ file.mime_type }}</span>
                  <span>{{ formatTime(file.posted_at) }}</span>
                </div>
                <div v-if="file.caption" class="file-caption">{{ file.caption }}</div>
              </div>
              <n-space>
                <n-button size="tiny" @click="handleDownloadFile(file)">下载</n-button>
                <n-button size="tiny" type="primary" :disabled="file.downloaded" @click="handleSaveFile(file)">
                  保存到库
                </n-button>
              </n-space>
            </div>

            <n-pagination
              v-if="filesTotalCount > 50"
              :page="filesPage"
              :page-size="50"
              :item-count="filesTotalCount"
              style="margin-top: 16px; justify-content: center;"
              @update:page="handlePageChange"
            />
          </n-card>
        </template>
      </n-tab-pane>

      <!-- 转发资源 -->
      <n-tab-pane name="forwarded" tab="转发资源">
        <div v-if="!hasBots" style="padding: 40px 0; text-align: center; color: #999;">
          <p>请先在"Bot 管理"中添加一个 Telegram Bot</p>
        </div>

        <template v-else>
          <n-card size="small" style="margin-top: 12px;">
            <template #header>
              <div style="display: flex; align-items: center; gap: 8px;">
                <span>收到的音频</span>
                <n-tag size="tiny" round type="info">{{ privateFilesTotalCount }} 个</n-tag>
              </div>
            </template>
            <template #header-extra>
              <n-input-group style="width: 300px;">
                <n-input v-model:value="privateFilesKeyword" placeholder="搜索文件名/标题/艺术家" size="small" clearable @keyup.enter="handlePrivateFilesSearch" />
                <n-button size="small" type="primary" @click="handlePrivateFilesSearch">搜索</n-button>
              </n-input-group>
            </template>

            <div style="margin-bottom: 12px; font-size: 12px; color: #999;">
              💡 在 Telegram 中，把资源频道机器人发给你的音频文件<b>转发</b>给本 Bot，即可自动收录。
            </div>

            <n-button size="small" style="margin-bottom: 12px;" @click="loadPrivateFiles" :loading="privateFilesLoading">
              刷新列表
            </n-button>

            <div v-if="privateFiles.length === 0 && !privateFilesLoading" style="text-align: center; color: #999; padding: 20px 0;">
              <n-empty description="还没有收到转发的音频文件" />
            </div>

            <div v-for="file in privateFiles" :key="file.id" class="file-item">
              <div class="file-info">
                <div class="file-title">
                  {{ file.title || file.file_name || '未知标题' }}
                  <n-tag v-if="file.downloaded" size="tiny" type="success" round style="margin-left: 6px;">已下载</n-tag>
                </div>
                <div class="file-meta">
                  <span v-if="file.artist">{{ file.artist }}</span>
                  <span v-if="file.duration">{{ formatDuration(file.duration) }}</span>
                  <span>{{ formatSize(file.file_size) }}</span>
                  <span>{{ file.mime_type }}</span>
                  <span>{{ formatTime(file.posted_at) }}</span>
                </div>
                <div v-if="file.caption" class="file-caption">{{ file.caption }}</div>
              </div>
              <n-space>
                <n-button size="tiny" @click="handleDownloadFile(file)">下载</n-button>
                <n-button size="tiny" type="primary" :disabled="file.downloaded" @click="handleSaveFile(file)">
                  保存到库
                </n-button>
              </n-space>
            </div>

            <n-pagination
              v-if="privateFilesTotalCount > 50"
              :page="privateFilesPage"
              :page-size="50"
              :item-count="privateFilesTotalCount"
              style="margin-top: 16px; justify-content: center;"
              @update:page="handlePrivatePageChange"
            />
          </n-card>
        </template>
      </n-tab-pane>

      <!-- 账号登录 -->
      <n-tab-pane name="accounts" tab="账号登录 (MTProto)">
        <div style="margin-top: 12px; font-size: 13px; color: #666; margin-bottom: 16px;">
          提示：登录真实 Telegram 账号后，可以通过 MTProto 高速下载频道大文件，并扫描频道历史记录。
        </div>
        <n-card title="添加 Telegram 账号" size="small">
          <n-form label-placement="left" label-width="100">
            <n-form-item label="手机号">
              <n-input v-model:value="accountForm.phone" placeholder="+86xxxxxxxxx" />
            </n-form-item>
            <n-form-item>
              <n-button type="primary" @click="handleSaveAccount">添加账号</n-button>
            </n-form-item>
          </n-form>
        </n-card>

        <n-button size="small" style="margin-top: 16px; margin-bottom: 8px;" @click="loadAccounts">刷新状态</n-button>

        <n-space vertical>
          <n-card v-for="account in accounts" :key="account.id" size="small">
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <div>
                <strong>{{ account.phone }}</strong>
                <n-tag size="tiny" round style="margin-left: 8px;" :type="account.status === 'active' ? 'success' : (account.status === 'code_required' || account.status === 'password_required' ? 'warning' : 'default')">
                  {{ account.status }}
                </n-tag>
              </div>
              <n-space>
                <n-button v-if="account.status === 'offline'" size="tiny" type="primary" @click="handleStartAccount(account.id)">登录</n-button>
                <n-button v-if="account.status === 'code_required'" size="tiny" type="info" @click="handleShowCode(account.id)">输入验证码</n-button>
                <n-button v-if="account.status === 'password_required'" size="tiny" type="info" @click="handleShowPassword(account.id)">输入两步验证密码</n-button>
                <n-button size="tiny" type="error" @click="handleDeleteAccount(account.id)">删除</n-button>
              </n-space>
            </div>
          </n-card>
          <div v-if="accounts.length === 0" style="text-align: center; color: #999; padding: 20px 0;">
            暂无 Telegram 账号
          </div>
        </n-space>
      </n-tab-pane>
    </n-tabs>

    <!-- 验证码弹窗 -->
    <n-modal v-model:show="showCodeModal" preset="dialog" title="输入验证码">
      <div style="margin-top: 16px;">
        <n-input v-model:value="authCodeForm.code" placeholder="输入从 Telegram 收到的验证码" />
      </div>
      <template #action>
        <n-button @click="showCodeModal = false">取消</n-button>
        <n-button type="primary" @click="handleSubmitCode">提交</n-button>
      </template>
    </n-modal>

    <!-- 两步验证密码弹窗 -->
    <n-modal v-model:show="showPwdModal" preset="dialog" title="输入两步验证密码">
      <div style="margin-top: 16px;">
        <n-input v-model:value="authPwdForm.password" type="password" show-password-on="click" placeholder="输入您的两步验证密码" />
      </div>
      <template #action>
        <n-button @click="showPwdModal = false">取消</n-button>
        <n-button type="primary" @click="handleSubmitPassword">提交</n-button>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.file-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 12px;
  border-bottom: 1px solid #f0f0f0;
}
.file-item:last-child {
  border-bottom: none;
}
.file-info {
  flex: 1;
  min-width: 0;
}
.file-title {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-meta {
  font-size: 12px;
  color: #999;
  margin-top: 2px;
  display: flex;
  gap: 12px;
}
.file-caption {
  font-size: 12px;
  color: #666;
  margin-top: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 500px;
}
</style>
