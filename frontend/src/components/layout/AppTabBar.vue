<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Icon } from '@iconify/vue'
import { useTaskStore } from '@/stores/task'

const route = useRoute()
const router = useRouter()
const taskStore = useTaskStore()

const tabs = [
  { path: '/search', icon: 'material-symbols:search', label: '搜索' },
  { path: '/queue', icon: 'material-symbols:download', label: '队列' },
  { path: '/library', icon: 'material-symbols:library-music', label: '音乐库' },
  { path: '/cloud', icon: 'material-symbols:cloud', label: '网盘' },
  { path: '/settings', icon: 'material-symbols:settings', label: '设置' },
]

const activeTab = computed(() => {
  return tabs.find((t) => route.path.startsWith(t.path))?.path || '/search'
})
</script>

<template>
  <div class="tab-bar">
    <div
      v-for="tab in tabs"
      :key="tab.path"
      class="tab-item"
      :class="{ active: activeTab === tab.path }"
      @click="router.push(tab.path)"
    >
      <div class="tab-icon-wrap">
        <Icon :icon="tab.icon" :width="24" />
        <span
          v-if="tab.path === '/queue' && taskStore.activeCount() > 0"
          class="badge"
        >
          {{ taskStore.activeCount() }}
        </span>
      </div>
      <span class="tab-label">{{ tab.label }}</span>
    </div>
  </div>
</template>

<style scoped>
.tab-bar {
  display: flex;
  justify-content: space-around;
  align-items: center;
  height: 56px;
  border-top: 1px solid var(--n-border-color, #e0e0e0);
  background: var(--n-card-color, #fff);
  padding-bottom: env(safe-area-inset-bottom);
}

.tab-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  cursor: pointer;
  color: #999;
  transition: color 0.2s;
}

.tab-item.active {
  color: #6366f1;
}

.tab-icon-wrap {
  position: relative;
}

.badge {
  position: absolute;
  top: -4px;
  right: -8px;
  background: #f43f5e;
  color: #fff;
  font-size: 10px;
  min-width: 16px;
  height: 16px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 4px;
}

.tab-label {
  font-size: 11px;
}
</style>
