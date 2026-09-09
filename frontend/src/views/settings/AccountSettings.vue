<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { NCard, NForm, NFormItem, NInput, NButton, NSpace, NSelect, NDivider, useMessage, NDescriptions, NDescriptionsItem, NTag } from 'naive-ui'
import { getMe, updateMe, getMyQuota, type User, type MyQuota } from '@/api/users'
import { listStorageTargets } from '@/api/storage'
import { useAuthStore } from '@/stores/auth'

const message = useMessage()
const auth = useAuthStore()
const router = useRouter()

const me = ref<User | null>(null)
const quota = ref<MyQuota | null>(null)
const storageOptions = ref<{ label: string; value: string }[]>([])
const form = ref({ download_dir: '', default_storage: null as string | null, password: '', confirm: '' })
const saving = ref(false)

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
    storageOptions.value = (s.data.data || []).map((t) => ({ label: t.name, value: t.id }))
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
        下载的音乐会保存到「默认存储」中你设定的目录（本地路径指服务器上的目录）。不同用户各自可配置，互不影响。
      </p>
      <n-form label-placement="left" label-width="120">
        <n-form-item label="下载目录">
          <n-input v-model:value="form.download_dir" placeholder="如 /data/music/我的音乐，留空用系统默认" />
        </n-form-item>
        <n-form-item label="默认存储">
          <n-select
            v-model:value="form.default_storage"
            :options="storageOptions"
            clearable
            placeholder="选择你要上传到的网盘/存储（可在设置 → 存储目标中创建）"
          />
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
  </div>
</template>

<style scoped>
.form-tip {
  color: #999;
  font-size: 12px;
  margin: 0 0 16px;
  line-height: 1.7;
}
</style>