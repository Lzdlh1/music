<script setup lang="ts">
import { NMenu, NLayout, NLayoutSider } from 'naive-ui'
import { computed, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Icon } from '@iconify/vue'
import { useBreakpoint } from '@/composables/useBreakpoint'

const route = useRoute()
const router = useRouter()
const { isMobile } = useBreakpoint()

const activeKey = computed(() => route.path)

const menuOptions = [
  { label: '下载偏好', key: '/settings/download', icon: 'material-symbols:download' },
  { label: '音乐来源', key: '/settings/sources', icon: 'material-symbols:music-note' },
  { label: 'Telegram', key: '/settings/telegram', icon: 'material-symbols:send' },
  { label: '存储目标', key: '/settings/storage', icon: 'material-symbols:cloud-upload' },
  { label: '代理配置', key: '/settings/proxy', icon: 'material-symbols:vpn-key' },
  { label: '文件命名', key: '/settings/naming', icon: 'material-symbols:folder' },
  { label: '系统', key: '/settings/system', icon: 'material-symbols:info' },
].map((item) => ({
  ...item,
  icon: () => h(Icon, { icon: item.icon, width: 18 }),
}))
</script>

<template>
  <div class="settings-layout">
    <h1 class="page-title">设置</h1>
    <n-layout has-sider v-if="!isMobile">
      <n-layout-sider :width="200" bordered>
        <n-menu
          :value="activeKey"
          :options="menuOptions"
          @update:value="(key: string) => router.push(key)"
        />
      </n-layout-sider>
      <n-layout class="settings-content">
        <router-view />
      </n-layout>
    </n-layout>
    <div v-else>
      <n-menu
        :value="activeKey"
        :options="menuOptions"
        @update:value="(key: string) => router.push(key)"
      />
      <router-view />
    </div>
  </div>
</template>

<style scoped>
.settings-layout {
  max-width: 1000px;
  margin: 0 auto;
}

.page-title {
  font-size: 24px;
  font-weight: 700;
  margin-bottom: 16px;
}

.settings-content {
  padding: 16px 24px;
}
</style>
