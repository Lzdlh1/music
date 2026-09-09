<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NButton, NSpace, NSwitch, NTag, NTabs, NTabPane, NInputNumber, NModal, NForm, NFormItem, NInput, NSelect, useMessage, NDataTable, NPopconfirm } from 'naive-ui'
import { Icon } from '@iconify/vue'
import { h } from 'vue'
import {
  listUsers, updateUser, createInvite, listInvites, deleteInvite,
  type UserRow, type InviteKey,
} from '@/api/users'

const message = useMessage()

// ---------- 用户列表 ----------
const users = ref<UserRow[]>([])
const loading = ref(false)

async function loadUsers() {
  loading.value = true
  try {
    const res = await listUsers()
    users.value = res.data.data || []
  } catch {
    message.error('加载用户列表失败')
  } finally {
    loading.value = false
  }
}

// 编辑用户对话框
const editing = ref<UserRow | null>(null)
const editForm = ref({ role: 'user', qq_limit: 300, disabled: false, download_dir: '', default_storage: '' })
const showEdit = ref(false)

function openEdit(u: UserRow) {
  editing.value = u
  editForm.value = {
    role: u.role,
    qq_limit: u.qq_limit ?? 300,
    disabled: u.disabled,
    download_dir: u.download_dir || '',
    default_storage: u.default_storage || '',
  }
  showEdit.value = true
}

async function saveUser() {
  if (!editing.value) return
  try {
    await updateUser(editing.value.id, editForm.value)
    message.success('已保存')
    showEdit.value = false
    await loadUsers()
  } catch (e: any) {
    message.error(e?.response?.data?.message || '保存失败')
  }
}

async function toggleDisabled(u: UserRow) {
  try {
    await updateUser(u.id, { disabled: !u.disabled })
    await loadUsers()
  } catch {
    message.error('操作失败')
  }
}

function renderRole(role: string) {
  return h(
    NTag,
    { type: role === 'admin' ? 'warning' : 'default', size: 'small', round: true },
    { default: () => (role === 'admin' ? '管理员' : '用户') },
  )
}

function renderStatus(row: UserRow) {
  return h(
    NTag,
    { type: row.disabled ? 'error' : 'success', size: 'small', round: true },
    { default: () => (row.disabled ? '已停用' : '正常') },
  )
}

function renderActions(row: UserRow) {
  return h(NSpace, { size: 4 }, {
    default: () => [
      h(NButton, { size: 'tiny', onClick: () => openEdit(row) }, { default: () => '编辑' }),
      h(
        NPopconfirm,
        {
          onPositiveClick: () => toggleDisabled(row),
          positiveButtonProps: { type: 'error' },
        },
        {
          trigger: () => h(NButton, { size: 'tiny', type: row.disabled ? 'success' : 'error' }, { default: () => (row.disabled ? '启用' : '停用') }),
          default: () => (row.disabled ? '确认启用该账号？' : '确认停用该账号？'),
        },
      ),
    ],
  })
}

const columns = [
  { title: '用户名', key: 'username', width: 140 },
  { title: '角色', key: 'role', width: 90, render: renderRole },
  { title: '状态', key: 'disabled', width: 90, render: renderStatus },
  { title: 'QQ 配额', key: 'qq_used', width: 110, render: (row: UserRow) => `${row.qq_used ?? 0} / ${row.qq_limit ?? 300}` },
  { title: '下载目录', key: 'download_dir', ellipsis: { tooltip: true } },
  { title: '创建时间', key: 'created_at', width: 170, render: (row: UserRow) => row.created_at?.slice(0, 16).replace('T', ' ') || '-' },
  { title: '操作', key: 'actions', width: 140, render: renderActions },
]

// ---------- 邀请码 ----------
const invites = ref<InviteKey[]>([])
const inviting = ref(false)

async function loadInvites() {
  const res = await listInvites()
  invites.value = res.data.data || []
}

async function genInvite() {
  inviting.value = true
  try {
    const res = await createInvite()
    const key = res.data.data
    message.info(`邀请码已生成：${key.code}`)
    await loadInvites()
  } catch {
    message.error('生成失败')
  } finally {
    inviting.value = false
  }
}

async function removeInvite(id: string) {
  await deleteInvite(id)
  await loadInvites()
}

function copyCode(code: string) {
  navigator.clipboard?.writeText(code)
  message.success('邀请码已复制')
}

onMounted(() => {
  loadUsers()
  loadInvites()
})

// 当前用户只能看到自己；管理员管理全部
const me = ref<UserRow | null>(null)
</script>

<template>
  <div class="users-page">
    <h1 class="page-title">用户管理</h1>
    <n-tabs type="line">
      <!-- 用户列表 -->
      <n-tab-pane name="users" tab="用户列表">
        <n-data-table
          :columns="columns"
          :data="users"
          :loading="loading"
          :row-key="(row: UserRow) => row.id"
          :pagination="{ pageSize: 20 }"
        />
      </n-tab-pane>

      <!-- 邀请码 -->
      <n-tab-pane name="invites" tab="邀请码">
        <n-card>
          <div class="invite-header">
            <span class="invite-desc">邀请码一次性有效，派发后他人可用它注册普通账号。</span>
            <n-button type="primary" :loading="inviting" @click="genInvite">
              <template #icon><Icon icon="material-symbols:add" /></template>
              生成邀请码
            </n-button>
          </div>
          <n-space vertical style="margin-top: 16px">
            <n-card v-for="k in invites" :key="k.id" size="small">
              <div class="invite-row">
                <div class="invite-info">
                  <code class="invite-code">{{ k.code }}</code>
                  <n-tag size="tiny" :type="k.used_by ? 'default' : 'success'" round style="margin-left: 8px">
                    {{ k.used_by ? `已使用 · ${k.username || ''}` : '未使用' }}
                  </n-tag>
                  <span class="invite-time">{{ (k.created_at || '').slice(0, 16).replace('T', ' ') }}</span>
                </div>
                <n-space>
                  <n-button v-if="!k.used_by" size="tiny" @click="copyCode(k.code)">复制</n-button>
                  <n-button size="tiny" type="error" @click="removeInvite(k.id)">撤销</n-button>
                </n-space>
              </div>
            </n-card>
            <n-empty v-if="!invites.length" description="还没有邀请码" />
          </n-space>
        </n-card>
      </n-tab-pane>
    </n-tabs>

    <!-- 编辑用户 -->
    <n-modal v-model:show="showEdit" preset="card" title="编辑用户" style="width: 460px">
      <n-form label-placement="left" label-width="110">
        <n-form-item label="用户名">
          <n-input :value="editing?.username" disabled />
        </n-form-item>
        <n-form-item label="角色">
          <n-select
            v-model:value="editForm.role"
            :options="[
              { label: '管理员（可配置系统/源/邀请码）', value: 'admin' },
              { label: '普通用户', value: 'user' },
            ]"
          />
        </n-form-item>
        <n-form-item label="QQ 月配额">
          <n-input-number v-model:value="editForm.qq_limit" :min="0" style="width: 100%" />
        </n-form-item>
        <n-form-item label="下载目录">
          <n-input v-model:value="editForm.download_dir" placeholder="如 /data/music/我的目录（可留空用默认）" />
        </n-form-item>
        <n-form-item label="默认存储">
          <n-input v-model:value="editForm.default_storage" placeholder="存储目标 ID（可选）" />
        </n-form-item>
        <n-form-item label="状态">
          <n-switch v-model:value="editForm.disabled">
            <template #checked>停用</template>
            <template #unchecked>启用</template>
          </n-switch>
        </n-form-item>
        <n-space justify="end">
          <n-button @click="showEdit = false">取消</n-button>
          <n-button type="primary" @click="saveUser">保存</n-button>
        </n-space>
      </n-form>
    </n-modal>
  </div>
</template>

<style scoped>
.users-page {
  max-width: 1100px;
  margin: 0 auto;
}

.page-title {
  font-size: 24px;
  font-weight: 700;
  margin-bottom: 16px;
}

.invite-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.invite-desc {
  color: #999;
  font-size: 13px;
}

.invite-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.invite-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.invite-code {
  font-size: 15px;
  font-weight: 600;
  background: rgba(99, 102, 241, 0.1);
  padding: 4px 10px;
  border-radius: 6px;
  color: #6366f1;
}

.invite-time {
  color: #999;
  font-size: 12px;
}
</style>