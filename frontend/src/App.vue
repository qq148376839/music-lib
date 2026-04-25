<script setup>
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { NConfigProvider, NLayout, NLayoutHeader, NLayoutContent, NMenu, NMessageProvider, darkTheme } from 'naive-ui'
import { SearchOutline, DownloadOutline, PulseOutline, SettingsOutline } from '@vicons/ionicons5'
import { api } from './api.js'
import { store } from './store.js'

const router = useRouter()
const route = useRoute()

const menuOptions = [
  { label: '搜索', key: '/search', icon: SearchOutline },
  { label: '下载', key: '/downloads', icon: DownloadOutline },
  { label: '监控', key: '/monitors', icon: PulseOutline },
  { label: '设置', key: '/settings', icon: SettingsOutline },
]

const activeKey = ref('/search')

router.afterEach((to) => {
  activeKey.value = to.path
})

function onMenuUpdate(key) {
  router.push(key)
}

// Theme override to match existing dark color scheme.
const themeOverrides = {
  common: {
    primaryColor: '#58a6ff',
    primaryColorHover: '#79c0ff',
    primaryColorPressed: '#388bfd',
    bodyColor: '#0d1117',
    cardColor: '#161b22',
    modalColor: '#161b22',
    popoverColor: '#161b22',
    tableColor: '#161b22',
    inputColor: '#161b22',
    actionColor: '#161b22',
    borderColor: '#30363d',
    dividerColor: '#30363d',
    textColorBase: '#f0f6fc',
    textColor1: '#f0f6fc',
    textColor2: '#c9d1d9',
    textColor3: '#8b949e',
    placeholderColor: '#484f58',
  },
}

const activeTasks = ref(0)

async function initApp() {
  try {
    const status = await api.getNASStatus()
    store.nasEnabled = status?.enabled || false
    store.nasMusicDir = status?.music_dir || ''
  } catch { /* NAS not configured */ }

  for (const platform of ['netease', 'qq']) {
    try {
      const data = await api.getLoginStatus(platform)
      store.loginState[platform].loggedIn = data?.logged_in || false
      store.loginState[platform].nickname = data?.nickname || ''
    } catch { /* ignore */ }
  }

  pollActiveTasks()
}

async function pollActiveTasks() {
  try {
    const tasks = await api.getTasks()
    activeTasks.value = (tasks || []).filter(
      (t) => t.status === 'pending' || t.status === 'running'
    ).length
  } catch { /* ignore */ }
}

// Poll active tasks every 5s for badge.
let badgeTimer = null
onMounted(() => {
  initApp()
  activeKey.value = route.path
  badgeTimer = setInterval(pollActiveTasks, 5000)
})
</script>

<template>
  <n-config-provider :theme="darkTheme" :theme-overrides="themeOverrides">
    <n-message-provider>
      <n-layout style="min-height: 100vh">
        <n-layout-header bordered style="padding: 12px 20px; display: flex; align-items: center; justify-content: space-between">
          <div style="font-size: 18px; font-weight: 700; color: #f0f6fc">Music Lib</div>
          <n-menu
            mode="horizontal"
            :value="activeKey"
            :options="menuOptions.map(o => ({
              label: o.label + (o.key === '/downloads' && activeTasks > 0 ? ` (${activeTasks})` : ''),
              key: o.key,
              icon: () => h(NIcon, { size: 18 }, { default: () => h(o.icon) }),
            }))"
            @update:value="onMenuUpdate"
            style="flex: 1; justify-content: center"
          />
        </n-layout-header>
        <n-layout-content style="padding: 16px; max-width: 960px; margin: 0 auto; width: 100%">
          <router-view />
        </n-layout-content>
      </n-layout>
    </n-message-provider>
  </n-config-provider>
</template>

<script>
import { h } from 'vue'
import { NIcon } from 'naive-ui'
export default { components: { NIcon } }
</script>

<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
html, body { height: 100%; }
body {
  background: #0d1117;
  color: #f0f6fc;
  font-family: system-ui, -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif;
  -webkit-font-smoothing: antialiased;
}
::-webkit-scrollbar { width: 6px; }
::-webkit-scrollbar-track { background: #0d1117; }
::-webkit-scrollbar-thumb { background: #30363d; border-radius: 3px; }
@media (max-width: 640px) {
  .n-layout-header { flex-direction: column !important; gap: 8px; }
}
</style>
