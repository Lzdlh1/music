<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { NCard, NForm, NFormItem, NInput, NInputGroup, NButton, NSpace, NSelect, NDivider, NModal, NSpin, NIcon, NAlert, useMessage, NDescriptions, NDescriptionsItem, NTag } from 'naive-ui'
import { Icon } from '@iconify/vue'
import { getMe, updateMe, getMyQuota, type User, type MyQuota } from '@/api/users'
import { listStorageTargets, browseStorage } from '@/api/storage'
import type { StorageTarget, FileInfo } from '@/types'
import { useAuthStore } from '@/stores/auth'

const message = useMessage()
const auth = useAuthStore()
const router = useRouter()

const me = ref<User | null>(null)
const quota = ref<MyQuota | null>(null)
const targets = ref<StorageTarget[]>([])
const storageOptions = computed(() =>
  targets.value.map((t) => ({ label: t.name, value: t.id })),
)
const form = ref({ download_dir: '', default_storage: null as string | null, password: '', confirm: '' })
const saving = ref(false)

// ---------- 下载目录选择（按所选存储目标浏览） ----------
const showDirPicker = ref(false)
const dirPath = ref('/')
const dirList = ref<FileInfo[]>([])
const dirLoading = ref(false)
const dirError = ref('')

const currentStorageName = computed(() => {
  const t = targets.value.find((x) => x.id === form.value.default_storage)
  return t ? t.name : ''
})

function joinPath(parent: string, name: string) {
  if (parent === '/' || parent === '') return '/' + name
  return parent.replace(/\/+$/, '') + '/' + name
}

async function openDirPicker() {
  if (!form.value.default_storage) {
    message.warning('请先选择「默认存储」，下载目录是相对该存储根目录的路径')
    return
  }
  dirPath.value = form.value.download_dir?.startsWith('/') ? form.value.download_dir : '/'
  dirError.value = ''
  showDirPicker.value = true
  await loadDir()
}

async function loadDir() {
  const targetId = form.value.default_storage
  if (!targetId) return
  dirLoading.value = true
  dirError.value = ''
  try {
    const res = await browseStorage(targetId, dirPath.value)
    dirList.value = (res.data.data || []).filter((f: FileInfo) => f.is_dir)
  } catch (e: any) {
    dirList.value = []
    // 存储侧报错（如 139 未登录/无权限）原样展示，便于用户定位
    dirError.value = e?.response?.data?.message || e?.message || '读取目录失败'
  } finally {
    dirLoading.value = false
  }
}

function enterDir(dir: FileInfo) {
  dirPath.value = dir.path || joinPath(dirPath.value, dir.name)
  loadDir()
}

function goParent() {
  if (dirPath.value === '/' || dirPath.value === '') return
  const idx = dirPath.value.lastIndexOf('/')
  dirPath.value = idx <= 0 ? '/' : dirPath.value.slice(0, idx)
  loadDir()
}

function pickDir() {
  form.value.download_dir = dirPath.value === '/' ? '' : dirPath.value
  showDirPicker.value = false
  message.success(
    dirPath.value === '/'
      ? `已设为存储根目录（${currentStorageName.value}）`
      : `已选择：${dirPath.value}`,
  )
}

onMounted(async () => {
  try {
    const res = await getMe()
    me.value = res.data.data
    form.value.download_dir = res.data.data.download_dir || ''
    form.value.default_storage = res.data.data.default_storage || null
  } catch { /* ignore */ }
  try {
    const q = await getMyQuota()
    quota.value = q.data.data
  } catch { /* ignore */ }
  try {
    const s = await listStorageTargets()
    targets.value = s.data.data || []
  } catch { /* ignore */ }
})

async function handleSave() {
  if (form.value.password && form.value.password !== form.value.confirm) {
    message.warning('两次输入的密码不一致')
    return
  }
  if (form.value.password && form.value.password.length < 6) {
    message.warning('密码至少 6 位')
    return
  }
  saving.value = true
  try {
    await updateMe({
      download_dir: form.value.download_dir,
      default_storage: form.value.default_storage || undefined,
      password: form.value.password || undefined,
    })
    message.success('已保存')
    form.value.password = ''
    form.value.confirm = ''
    // 若本账号被降权，刷新角色
    const res = await getMe()
    me.value = res.data.data
  } catch (e: any) {
    message.error(e?.response?.data?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

function handleLogout() {
  auth.logout()
  router.replace('/login')
}

function fmtQuota() {
  const q = quota.value?.qq
  if (!q) return '-'
  return `${(q as any).used ?? 0} / ${q.limit ?? 300}`
}
</script>

<template>
  <div>
    <h2>我的账户</h2>

    <n-card title="我的信息" size="small" style="margin-bottom: 16px">
      <n-descriptions :column="2" size="small" bordered>
        <n-descriptions-item label="用户名">
          {{ me?.username || auth.username }}
        </n-descriptions-item>
        <n-descriptions-item label="角色">
          <n-tag size="small" :type="me?.role === 'admin' || auth.isAdmin ? 'warning' : 'default'" round>
            {{ me?.role === 'admin' || auth.isAdmin ? '管理员' : '普通用户' }}
          </n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="QQ 下载次数（本月）">
          <b>{{ fmtQuota() }}</b>
        </n-descriptions-item>
        <n-descriptions-item label="来源">
          共用平台音乐源（QQ 音乐）
        </n-descriptions-item>
      </n-descriptions>
    </n-card>

    <n-card title="下载路径与存储" size="small">
      <p class="form-tip">
        下载的音乐会保存到「默认存储」下的下载目录中。下载目录是<b>相对该存储根目录</b>的路径，
        例如填 <code>/音乐</code> 表示存到该存储的「音乐」文件夹；留空则存在存储根目录。
        不同用户各自可配置，互不影响。
      </p>
      <n-form label-placement="left" label-width="120">
        <n-form-item label="默认存储">
          <n-select
            v-model:value="form.default_storage"
            :options="storageOptions"
            clearable
            placeholder="选择你要上传到的网盘/存储（可在设置 → 存储目标中创建）"
          />
        </n-form-item>
        <n-form-item label="下载目录">
          <n-input-group>
            <n-input
              v-model:value="form.download_dir"
              :placeholder="form.default_storage ? '留空为存储根目录，点右侧「浏览」从目录树中选' : '请先选择默认存储'"
            />
            <n-button :disabled="!form.default_storage" @click="openDirPicker">
              <template #icon>
                <n-icon><Icon icon="material-symbols:folder-open" /></n-icon>
              </template>
              浏览
            </n-button>
          </n-input-group>
        </n-form-item>
        <n-divider />
        <n-form-item label="新密码">
          <n-input v-model:value="form.password" type="password" show-password-on="click" placeholder="留空不修改" />
        </n-form-item>
        <n-form-item label="确认密码">
          <n-input v-model:value="form.confirm" type="password" show-password-on="click" placeholder="再次输入新密码" />
        </n-form-item>
        <n-space>
          <n-button type="primary" :loading="saving" @click="handleSave">保存</n-button>
          <n-button type="error" secondary @click="handleLogout">
            退出登录
          </n-button>
        </n-space>
      </n-form>
    </n-card>

    <!-- 下载目录选择：浏览所选存储目标的目录树 -->
    <n-modal
      v-model:show="showDirPicker"
      preset="card"
      :title="`选择下载目录 · ${currentStorageName || '未选择存储'}`"
      style="width: 520px; max-width: 92vw"
    >
      <div class="dir-picker">
        <div class="dir-bar">
          <n-button size="tiny" @click="goParent" :disabled="dirPath === '/' || dirPath === ''">上一级</n-button>
          <span class="dir-path">{{ dirPath }}</span>
        </div>
        <n-alert v-if="dirError" type="error" :show-icon="true" style="margin-bottom: 4px">
          {{ dirError }}
        </n-alert>
        <div v-if="dirError" style="text-align: right">
          <n-button size="tiny" @click="loadDir">重试</n-button>
        </div>
        <n-spin :show="dirLoading">
          <div v-if="dirList.length" class="dir-list">
            <div v-for="dir in dirList" :key="dir.path || dir.name" class="dir-item" @click="enterDir(dir)">
              <n-icon><Icon icon="material-symbols:folder" :width="20" /></n-icon>
              <span class="dir-name">{{ dir.name }}</span>
            </div>
          </div>
          <div v-else-if="!dirError" class="dir-empty">该目录下没有子文件夹</div>
        </n-spin>
        <p class="dir-hint">选中的目录会写入「下载目录」，保存后生效。</p>
      </div>
      <template #footer>
        <n-button @click="showDirPicker = false">取消</n-button>
        <n-button type="primary" @click="pickDir">
          {{ dirPath === '/' ? '使用存储根目录' : '使用此文件夹' }}
        </n-button>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.form-tip {
  color: #999;
  font-size: 12px;
  margin: 0 0 16px;
  line-height: 1.7;
}
.form-tip code {
  background: #f2f2f5;
  padding: 1px 5px;
  border-radius: 4px;
  color: #555;
}

.dir-picker {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 220px;
}

.dir-bar {
  display: flex;
  align-items: center;
  gap: 10px;
}

.dir-path {
  font-size: 13px;
  color: #666;
  word-break: break-all;
}

.dir-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  /* 目录多时弹窗内滚动，避免撑破视口遮挡底部按钮 */
  max-height: 46vh;
  overflow-y: auto;
}

.dir-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 10px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
  /* 触屏下保证足够的手指点击高度 */
  min-height: 40px;
  box-sizing: border-box;
}

.dir-item:hover {
  background: #f5f6fa;
}

.dir-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dir-empty {
  color: #999;
  font-size: 13px;
  text-align: center;
  padding: 32px 0;
}

.dir-hint {
  margin: 0;
  font-size: 12px;
  color: #999;
}
</style>