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
      path: '/users',
      name: 'Users',
      component: () => import('@/views/UsersView.vue'),
      meta: { requireAdmin: true },
    },
    {
      path: '/settings',
      name: 'Settings',
      component: () => import('@/views/settings/SettingsLayout.vue'),
      children: [
        { path: '', redirect: '/settings/download' },
        { path: 'account', component: () => import('@/views/settings/AccountSettings.vue') },
        { path: 'download', component: () => import('@/views/settings/DownloadSettings.vue'), meta: { requireAdmin: true } },
        { path: 'sources', component: () => import('@/views/settings/SourceSettings.vue'), meta: { requireAdmin: true } },
        { path: 'storage', component: () => import('@/views/settings/StorageSettings.vue') },
        { path: 'telegram', component: () => import('@/views/settings/TelegramSettings.vue'), meta: { requireAdmin: true } },
        { path: 'proxy', component: () => import('@/views/settings/ProxySettings.vue'), meta: { requireAdmin: true } },
        { path: 'naming', component: () => import('@/views/settings/NamingSettings.vue'), meta: { requireAdmin: true } },
        { path: 'system', component: () => import('@/views/settings/SystemSettings.vue'), meta: { requireAdmin: true } },
      ],
    },
  ],
})

function isAdmin() {
  return localStorage.getItem('mf_role') === 'admin'
}

router.beforeEach((to) => {
  NProgress.start()
  const token = localStorage.getItem('mf_token')

  // 未登录：仅允许访问 public 页面
  if (!to.meta.public && !token) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  // 已登录访问登录页：直接进首页
  if (to.name === 'Login' && token) {
    return '/search'
  }
  // 普通用户访问管理员页面：退回首页
  if (to.meta.requireAdmin && !isAdmin()) {
    return '/search'
  }
})

router.afterEach(() => {
  NProgress.done()
})

export default router