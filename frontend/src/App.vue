<script setup lang="ts">
import { onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { NConfigProvider, NMessageProvider, darkTheme, zhCN, dateZhCN } from 'naive-ui'
import { useWSStore } from '@/stores/ws'
import { useTaskStore } from '@/stores/task'
import AppSidebar from '@/components/layout/AppSidebar.vue'
import PlayerBar from '@/components/player/PlayerBar.vue'

const route = useRoute()
const wsStore = useWSStore()
const taskStore = useTaskStore()

const isLoginPage = computed(() => route.name === 'Login')

onMounted(() => {
  wsStore.connect()
  taskStore.fetchStats()
})
</script>

<template>
  <n-config-provider :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
      <router-view v-if="isLoginPage" />
      <template v-else>
        <AppSidebar />
        <PlayerBar />
      </template>
    </n-message-provider>
  </n-config-provider>
</template>

<style>
body {
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

#app {
  height: 100vh;
}
</style>
