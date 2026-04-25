<script setup>
import { ref, onUnmounted } from 'vue'
import {
  NCard, NButton, NTag, NSelect, NSpace, NDescriptions, NDescriptionsItem,
  NModal, NSpin, NInput, NCollapse, NCollapseItem, useMessage,
} from 'naive-ui'
import { api } from '../api.js'
import { store, setQuality } from '../store.js'

const message = useMessage()

const qualityOptions = [
  { label: '无损', value: 'lossless' },
  { label: '极高', value: 'high' },
  { label: '标准', value: 'standard' },
]

// --- QR Login ---
const qrVisible = ref(false)
const qrPlatform = ref('')
const qrImage = ref('')
const qrStatus = ref('')
const qrState = ref('') // starting, waiting_scan, scanned, success, expired, error
let qrTimer = null

const PLATFORM_NAMES = { netease: '网易云音乐', qq: 'QQ音乐' }

// Platforms that support QR login (netease blocked by anti-bot).
const QR_PLATFORMS = new Set(['qq'])

function openLogin(platform) {
  qrPlatform.value = platform
  qrVisible.value = true
  cookieText.value = ''
  if (store.loginState[platform]?.loggedIn) {
    qrState.value = 'logged_in'
    qrStatus.value = ''
  } else if (QR_PLATFORMS.has(platform)) {
    startQR(platform)
  } else {
    // Cookie-only platform — show cookie input directly.
    qrState.value = 'cookie_only'
    qrStatus.value = ''
  }
}

async function startQR(platform) {
  qrImage.value = ''
  qrState.value = 'starting'
  qrStatus.value = '正在启动...'
  try {
    await api.startQRLogin(platform)
    startPolling(platform)
  } catch (e) {
    qrState.value = 'error'
    qrStatus.value = e.message || '启动失败'
  }
}

function startPolling(platform) {
  stopPolling()
  qrTimer = setInterval(async () => {
    try {
      const data = await api.pollQRLogin(platform)
      if (!data) return
      qrState.value = data.state
      switch (data.state) {
        case 'starting':
          qrStatus.value = '正在启动...'
          break
        case 'waiting_scan':
          if (data.qr_image) qrImage.value = 'data:image/png;base64,' + data.qr_image
          qrStatus.value = `请使用${PLATFORM_NAMES[platform] || platform}App扫码`
          break
        case 'scanned':
          qrStatus.value = '已扫码，请在手机上确认'
          break
        case 'success':
          qrStatus.value = '登录成功！'
          stopPolling()
          store.loginState[platform].loggedIn = true
          store.loginState[platform].nickname = data.nickname || ''
          setTimeout(() => { qrVisible.value = false }, 1200)
          break
        case 'expired':
          qrStatus.value = '二维码已过期，正在刷新...'
          stopPolling()
          startQR(platform)
          break
        case 'error':
          qrStatus.value = data.error || '登录出错'
          stopPolling()
          break
      }
    } catch { /* ignore */ }
  }, 2000)
}

function stopPolling() {
  if (qrTimer) { clearInterval(qrTimer); qrTimer = null }
}

async function doLogout(platform) {
  try {
    await api.logout(platform)
    store.loginState[platform].loggedIn = false
    store.loginState[platform].nickname = ''
    qrVisible.value = false
    message.success(`已退出${PLATFORM_NAMES[platform] || platform}`)
  } catch (e) {
    message.error(e.message || '退出失败')
  }
}

function onModalClose() {
  stopPolling()
}

// --- Manual Cookie ---
const cookieText = ref('')
const cookieLoading = ref(false)

async function submitCookie(platform) {
  const cookie = cookieText.value.trim()
  if (!cookie) {
    message.warning('请粘贴 Cookie')
    return
  }
  cookieLoading.value = true
  try {
    await api.loginWithCookie(platform, cookie, '')
    store.loginState[platform].loggedIn = true
    store.loginState[platform].nickname = ''
    message.success('Cookie 设置成功')
    cookieText.value = ''
    setTimeout(() => { qrVisible.value = false }, 800)
    // Refresh status to get nickname.
    const status = await api.getLoginStatus(platform)
    if (status) {
      store.loginState[platform].loggedIn = status.logged_in
      store.loginState[platform].nickname = status.nickname || ''
    }
  } catch (e) {
    message.error(e.message || 'Cookie 设置失败')
  } finally {
    cookieLoading.value = false
  }
}

onUnmounted(stopPolling)
</script>

<template>
  <div>
    <div style="font-size: 16px; font-weight: 600; margin-bottom: 16px">设置</div>

    <!-- Login Status -->
    <n-card title="登录管理" size="small" style="margin-bottom: 16px">
      <n-space vertical :size="12">
        <div v-for="platform in ['netease', 'qq']" :key="platform" style="display: flex; align-items: center; justify-content: space-between">
          <n-space align="center">
            <span style="font-weight: 500; min-width: 80px">{{ PLATFORM_NAMES[platform] }}</span>
            <n-tag v-if="store.loginState[platform]?.loggedIn" type="success" size="small">
              {{ store.loginState[platform].nickname || '已登录' }}
            </n-tag>
            <n-tag v-else type="default" size="small">未登录</n-tag>
          </n-space>
          <n-button size="small" @click="openLogin(platform)">
            {{ store.loginState[platform]?.loggedIn ? '管理' : '登录' }}
          </n-button>
        </div>
      </n-space>
    </n-card>

    <!-- Quality -->
    <n-card title="全局音质" size="small" style="margin-bottom: 16px">
      <n-select
        :value="store.quality"
        :options="qualityOptions"
        @update:value="setQuality"
        style="max-width: 200px"
      />
    </n-card>

    <!-- NAS Status -->
    <n-card title="NAS 存储" size="small">
      <n-descriptions label-placement="left" :column="1" size="small">
        <n-descriptions-item label="状态">
          <n-tag :type="store.nasEnabled ? 'success' : 'default'" size="small">
            {{ store.nasEnabled ? '已启用' : '未启用' }}
          </n-tag>
        </n-descriptions-item>
        <n-descriptions-item v-if="store.nasEnabled" label="目录">
          {{ store.nasMusicDir }}
        </n-descriptions-item>
      </n-descriptions>
    </n-card>

    <!-- Login Modal -->
    <n-modal v-model:show="qrVisible" preset="card"
      :title="`${PLATFORM_NAMES[qrPlatform] || ''} ${qrState === 'logged_in' ? '账号管理' : QR_PLATFORMS.has(qrPlatform) ? '扫码登录' : '登录'}`"
      style="max-width: 380px; text-align: center" @after-leave="onModalClose">
      <!-- Logged in state -->
      <template v-if="qrState === 'logged_in'">
        <div style="margin: 20px 0">
          <div style="font-size: 15px; margin-bottom: 16px">
            已登录: <span style="color: #58a6ff; font-weight: 600">{{ store.loginState[qrPlatform]?.nickname }}</span>
          </div>
          <n-button type="error" ghost @click="doLogout(qrPlatform)">退出登录</n-button>
        </div>
      </template>

      <!-- Cookie-only login (netease) -->
      <template v-else-if="qrState === 'cookie_only'">
        <div style="text-align: left; padding: 8px 0">
          <div style="font-size: 13px; color: #8b949e; margin-bottom: 12px; line-height: 1.6">
            1. 浏览器打开 <a href="https://music.163.com" target="_blank" style="color: #58a6ff">music.163.com</a> 并登录<br>
            2. 按 F12 → 切换到 Network（网络）面板 → 刷新页面<br>
            3. 点击任意一个请求 → 在 Headers 中找到 <b>Cookie</b> → 右键复制值
          </div>
          <n-input v-model:value="cookieText" type="textarea" :rows="3"
            placeholder="粘贴 Cookie 字符串..." />
          <n-button size="small" type="primary" :loading="cookieLoading" :disabled="!cookieText.trim()"
            style="margin-top: 8px" @click="submitCookie(qrPlatform)">
            设置 Cookie
          </n-button>
        </div>
      </template>

      <!-- QR login flow (qq) -->
      <template v-else>
        <div v-if="qrImage" style="display: flex; justify-content: center; margin: 16px 0">
          <div style="background: #fff; border-radius: 12px; padding: 16px">
            <img :src="qrImage" alt="QR Code" style="width: 200px; height: 200px" />
          </div>
        </div>
        <div v-else-if="qrState === 'starting'" style="padding: 40px 0">
          <n-spin />
        </div>
        <div :style="{
          color: qrState === 'success' ? '#3fb950' : qrState === 'scanned' ? '#58a6ff' : (qrState === 'error' || qrState === 'expired') ? '#f85149' : '#8b949e',
          margin: '12px 0',
          fontSize: '14px'
        }">
          {{ qrStatus }}
        </div>

        <!-- Manual cookie input (fallback for QR platforms) -->
        <n-collapse style="margin-top: 12px; text-align: left">
          <n-collapse-item title="手动输入 Cookie" name="cookie">
            <div style="font-size: 12px; color: #8b949e; margin-bottom: 8px">
              从浏览器开发者工具复制 Cookie 后粘贴到此处
            </div>
            <n-input v-model:value="cookieText" type="textarea" :rows="3"
              placeholder="粘贴 Cookie 字符串..." />
            <n-button size="small" type="primary" :loading="cookieLoading" :disabled="!cookieText.trim()"
              style="margin-top: 8px" @click="submitCookie(qrPlatform)">
              设置 Cookie
            </n-button>
          </n-collapse-item>
        </n-collapse>
      </template>
    </n-modal>
  </div>
</template>
