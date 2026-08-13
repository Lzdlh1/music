import { createRouter, createWebHistory } from 'vue-router'
import NProgress from 'nprogress'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      redirect: '/search',
    },
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/views/LoginView.vue'),
      meta: { public: true },
    },
    {
      path: '/search',
      name: 'Search',
      component: () => import('@/views/SearchView.vue'),
    },
    {
      path: '/queue',
      name: 'Queue',
      component: () => import('@/views/QueueView.vue'),
    },
    {
      path: '/library',
      name: 'Library',
      component: () => import('@/views/LibraryView.vue'),
    },
    {
      path: '/cloud',
      name: 'Cloud',
      component: () => import('@/views/CloudView.vue'),
    },
    {
      path: '/playlist',
      name: 'Playlist',
      component: () => import('@/views/PlaylistView.vue'),
    },
    {
      path: '/settings',
      name: 'Settings',
      component: () => import('@/views/settings/SettingsLayout.vue'),
      children: [
        { path: '', redirect: '/settings/download' },
        { path: 'download', component: () => import('@/views/settings/DownloadSettings.vue') },
        { path: 'sources', component: () => import('@/views/settings/SourceSettings.vue') },
        { path: 'storage', component: () => import('@/views/settings/StorageSettings.vue') },
        { path: 'telegram', component: () => import('@/views/settings/TelegramSettings.vue') },
        { path: 'proxy', component: () => import('@/views/settings/ProxySettings.vue') },
        { path: 'naming', component: () => import('@/views/settings/NamingSettings.vue') },
        { path: 'system', component: () => import('@/views/settings/SystemSettings.vue') },
      ],
    },
  ],
})

router.beforeEach((to) => {
  NProgress.start()
  const token = localStorage.getItem('mf_token')
  if (!to.meta.public && !token) {
    // 允许无 token 访问，后端 auth.enabled=false 时不需要 token
    // 仅在需要时拦截，由 axios 401 拦截器处理
  }
})

router.afterEach(() => {
  NProgress.done()
})

export default router
