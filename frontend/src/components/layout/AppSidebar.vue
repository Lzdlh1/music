<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NLayout, NLayoutSider, NMenu, NIcon, NBadge } from 'naive-ui'
import { Icon } from '@iconify/vue'
import { useTaskStore } from '@/stores/task'
import { useBreakpoint } from '@/composables/useBreakpoint'
import { useAuthStore } from '@/stores/auth'
import AppTabBar from './AppTabBar.vue'

const route = useRoute()
const router = useRouter()
const taskStore = useTaskStore()
const auth = useAuthStore()
const { isMobile } = useBreakpoint()

const activeKey = computed(() => route.path)

const menuOptions = computed(() => {
  const items: any[] = [
    {
      label: '搜索',
      key: '/search',
      icon: () => renderIcon('material-symbols:search'),
    },
    {
      label: `下载队列 (${taskStore.activeCount()})`,
      key: '/queue',
      icon: () => renderIcon('material-symbols:download'),
    },
    {
      label: '音乐库',
      key: '/library',
      icon: () => renderIcon('material-symbols:library-music'),
    },
    {
      label: '网盘',
      key: '/cloud',
      icon: () => renderIcon('material-symbols:cloud'),
    },
    {
      label: '歌单导入',
      key: '/playlist',
      icon: () => renderIcon('material-symbols:playlist-add'),
    },
  ]
  // 管理员专属：用户管理
  if (auth.isAdmin) {
    items.push({
      label: '用户管理',
      key: '/users',
      icon: () => renderIcon('material-symbols:group'),
    })
  }
  items.push({
    label: '设置',
    key: '/settings',
    icon: () => renderIcon('material-symbols:settings'),
  })
  return items
})

function handleLogout() {
  auth.logout()
  router.replace('/login')
}

function renderIcon(icon: string) {
  return h(NIcon, { size: 20 }, { default: () => h(Icon, { icon }) })
}

function handleMenuUpdate(key: string) {
  router.push(key)
}

import { h } from 'vue'
</script>

<template>
  <n-layout has-sider v-if="!isMobile" style="height: 100vh">
    <n-layout-sider
      bordered
      :width="240"
      :collapsed-width="64"
      show-trigger
      collapse-mode="width"
    >
      <div class="logo">
        <Icon icon="material-symbols:music-note" :width="28" />
        <span class="logo-text">商角</span>
      </div>
      <n-menu
        :value="activeKey"
        :options="menuOptions"
        @update:value="handleMenuUpdate"
      />
      <div class="sidebar-footer">
        <div class="status-item">
          队列: {{ taskStore.stats.DOWNLOADING }}下载 {{ taskStore.stats.UPLOADING }}上传
        </div>
        <div class="user-box">
          <div class="user-info">
            <Icon icon="material-symbols:account-circle" :width="20" />
            <span class="user-name">{{ auth.username || '未登录' }}</span>
            <n-tag v-if="auth.isAdmin" size="tiny" type="warning" round>管理员</n-tag>
          </div>
          <n-button text size="tiny" type="error" @click="handleLogout">
            <template #icon><Icon icon="material-symbols:logout" :width="16" /></template>
            退出
          </n-button>
        </div>
      </div>
    </n-layout-sider>
    <n-layout>
      <div class="main-content">
        <router-view />
      </div>
    </n-layout>
  </n-layout>

  <div v-else class="mobile-layout">
    <div class="mobile-content">
      <router-view />
    </div>
    <AppTabBar />
  </div>
</template>

<style scoped>
.logo {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 20px 24px;
  font-size: 20px;
  font-weight: 700;
  color: #6366f1;
}

.sidebar-footer {
  position: absolute;
  bottom: 16px;
  left: 16px;
  right: 16px;
  font-size: 12px;
  color: #999;
}

.status-item {
  padding: 4px 0;
}

.user-box {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid rgba(128, 128, 128, 0.15);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 4px;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.user-name {
  font-size: 13px;
  color: #555;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.main-content {
  padding: 24px;
  height: 100vh;
  overflow-y: auto;
}

.mobile-layout {
  display: flex;
  flex-direction: column;
  height: 100vh;
}

.mobile-content {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
}
</style>
