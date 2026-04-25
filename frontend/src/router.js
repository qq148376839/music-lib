import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/', redirect: '/search' },
  { path: '/search', component: () => import('./views/SearchView.vue') },
  { path: '/downloads', component: () => import('./views/DownloadView.vue') },
  { path: '/monitors', component: () => import('./views/MonitorView.vue') },
  { path: '/settings', component: () => import('./views/SettingsView.vue') },
]

export default createRouter({
  history: createWebHistory(),
  routes,
})
