<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NLayout, NLayoutSider, NMenu, NIcon, NBadge } from 'naive-ui'
import { Icon } from '@iconify/vue'
import { useTaskStore } from '@/stores/task'
import { useBreakpoint } from '@/composables/useBreakpoint'
import AppTabBar from './AppTabBar.vue'

const route = useRoute()
const router = useRouter()
const taskStore = useTaskStore()
const { isMobile } = useBreakpoint()

const activeKey = computed(() => route.path)

const menuOptions = computed(() => [
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
  {
    label: '设置',
    key: '/settings',
    icon: () => renderIcon('material-symbols:settings'),
  },
])

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
        <span class="logo-text">MusicFlow</span>
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
