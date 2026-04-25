<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import {
  NCard, NTag, NProgress, NEmpty, NSpace, NTabs, NTabPane, NSpin,
  NCollapse, NCollapseItem, NButton, useMessage,
} from 'naive-ui'
import { api } from '../api.js'
import { store } from '../store.js'
import { getSourceName } from '../store.js'
import { formatSize, formatTime } from '../utils.js'

const message = useMessage()
const tasks = ref([])
const batches = ref([])
const loading = ref(false)
const upgradingAll = ref(false)
const upgradingSingle = ref({}) // taskId -> bool

const activeTasks = computed(() =>
  tasks.value.filter((t) => t.status === 'pending' || t.status === 'running')
)
const historyTasks = computed(() =>
  tasks.value.filter((t) => t.status === 'done' || t.status === 'failed')
)
const upgradeableCount = computed(() =>
  tasks.value.filter(
    (t) => t.status === 'done' && !t.skipped && !t.upgraded
  ).length
)

async function refresh() {
  try {
    const [t, b] = await Promise.all([
      api.getTasks().catch(() => []),
      api.getBatches().catch(() => []),
    ])
    tasks.value = (t || []).reverse()
    batches.value = (b || []).reverse()
  } catch { /* ignore */ }
}

function statusType(status) {
  switch (status) {
    case 'done': return 'success'
    case 'failed': return 'error'
    case 'running': return 'info'
    default: return 'default'
  }
}

function statusLabel(task) {
  if (task.status === 'done' && task.skipped) return '已存在'
  const map = { pending: '等待中', running: '下载中', done: '已完成', failed: '失败' }
  return map[task.status] || task.status
}

function taskProgress(task) {
  if (task.status !== 'running' || !task.total_size) return 0
  return Math.min(100, Math.round((task.progress / task.total_size) * 100))
}

function batchProgress(b) {
  if (!b.total) return 0
  return Math.round(((b.done + b.failed) / b.total) * 100)
}

function isDegraded(task) {
  if (!task.actual_quality || !task.requested_quality) return false
  const req = task.requested_quality
  const actual = (task.actual_quality || '').toUpperCase()
  if (req === 'lossless' && actual.indexOf('FLAC') === -1) return true
  if (req === 'high' && (actual.indexOf('128') !== -1 || actual.indexOf('M4A') !== -1)) return true
  return false
}

function canUpgrade(task) {
  return task.status === 'done' && !task.skipped && !task.upgraded
}

async function handleUpgradeSingle(taskId) {
  upgradingSingle.value[taskId] = true
  try {
    const result = await api.upgradeDownloads(store.quality, [taskId])
    if (result.queued > 0) {
      message.success('已入队升级')
    } else {
      message.warning('无法升级')
    }
  } catch (e) {
    message.error('升级失败: ' + e.message)
  } finally {
    upgradingSingle.value[taskId] = false
  }
}

async function handleUpgradeAll() {
  upgradingAll.value = true
  try {
    const result = await api.upgradeDownloads(store.quality, [])
    message.success(`已入队 ${result.queued} 个升级任务`)
  } catch (e) {
    message.error('升级失败: ' + e.message)
  } finally {
    upgradingAll.value = false
  }
}

let pollTimer = null
onMounted(() => {
  loading.value = true
  refresh().finally(() => (loading.value = false))
  pollTimer = setInterval(refresh, 3000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div>
    <!-- NAS Status -->
    <n-space align="center" style="margin-bottom: 16px">
      <n-tag v-if="store.nasEnabled" type="success" size="small">NAS已启用 ({{ store.nasMusicDir }})</n-tag>
      <n-tag v-else type="default" size="small">NAS未启用</n-tag>
    </n-space>

    <n-spin :show="loading">
      <!-- Batch Summary -->
      <n-collapse v-if="batches.length > 0" style="margin-bottom: 16px">
        <n-collapse-item title="批量任务" :name="1">
          <template #header-extra>{{ batches.length }} 批</template>
          <n-card v-for="(b, i) in batches" :key="i" size="small" style="margin-bottom: 8px">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px">
              <span style="font-weight: 500">{{ b.name || b.playlist_name || b.id }}</span>
              <span style="font-size: 12px; color: #8b949e">
                {{ b.done }}/{{ b.total }} 完成
                <template v-if="b.running"> · {{ b.running }}下载中</template>
                <template v-if="b.failed"> · {{ b.failed }}失败</template>
              </span>
            </div>
            <n-progress
              type="line"
              :percentage="batchProgress(b)"
              :status="b.failed > 0 && b.done === 0 ? 'error' : (b.done === b.total ? 'success' : 'info')"
              :show-indicator="false"
              style="height: 4px"
            />
          </n-card>
        </n-collapse-item>
      </n-collapse>

      <!-- Upgrade Bar -->
      <div v-if="upgradeableCount > 0" class="upgrade-bar">
        <n-button
          size="small"
          type="primary"
          :loading="upgradingAll"
          @click="handleUpgradeAll"
        >
          全部升级
        </n-button>
        <span class="upgrade-hint">{{ upgradeableCount }} 个任务可能可升级</span>
      </div>

      <n-tabs type="line" animated>
        <!-- Active Tasks -->
        <n-tab-pane name="active" :tab="`进行中 (${activeTasks.length})`">
          <n-empty v-if="activeTasks.length === 0" description="暂无进行中的任务" />
          <n-card v-for="task in activeTasks" :key="task.id" size="small" style="margin-bottom: 8px">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px">
              <span class="task-name">{{ task.song?.name || '未知' }}</span>
              <n-space :size="4" align="center">
                <n-tag v-if="task.actual_quality" size="tiny" :bordered="false">{{ task.actual_quality }}</n-tag>
                <n-tag :type="statusType(task.status)" size="tiny">{{ statusLabel(task) }}</n-tag>
              </n-space>
            </div>
            <div class="task-meta">
              {{ task.song?.artist || '' }}
              <n-tag v-if="task.source" size="tiny" :bordered="false" style="margin-left: 4px">{{ getSourceName(task.source) }}</n-tag>
              <template v-if="task.fallback_source">
                → <n-tag size="tiny" :bordered="false" type="warning">{{ getSourceName(task.fallback_source) }}</n-tag>
              </template>
            </div>
            <template v-if="task.status === 'running'">
              <n-progress type="line" :percentage="taskProgress(task)" :show-indicator="false" style="margin-top: 6px" />
              <div class="task-meta" style="margin-top: 2px">
                {{ task.progress > 0 ? formatSize(task.progress) : '下载中...' }}{{ task.total_size > 0 ? ` / ${formatSize(task.total_size)}` : '' }}
              </div>
            </template>
          </n-card>
        </n-tab-pane>

        <!-- History -->
        <n-tab-pane name="history" :tab="`历史 (${historyTasks.length})`">
          <n-empty v-if="historyTasks.length === 0" description="暂无历史记录" />
          <n-card v-for="task in historyTasks" :key="task.id" size="small" style="margin-bottom: 8px">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px">
              <span class="task-name">{{ task.song?.name || '未知' }}</span>
              <n-space :size="4" align="center">
                <!-- Upgrade result -->
                <n-tag v-if="task.upgraded" size="tiny" type="success">
                  已升级 {{ task.previous_quality }} → {{ task.actual_quality }}
                </n-tag>
                <!-- Quality tag -->
                <n-tag v-else-if="task.actual_quality" size="tiny" :bordered="false">{{ task.actual_quality }}</n-tag>
                <!-- Degraded indicator -->
                <n-tag v-if="isDegraded(task)" size="tiny" type="warning">已降级</n-tag>
                <!-- Status -->
                <n-tag :type="task.status === 'done' && task.skipped ? 'warning' : statusType(task.status)" size="tiny">
                  {{ statusLabel(task) }}
                </n-tag>
              </n-space>
            </div>
            <div class="task-meta">
              {{ task.song?.artist || '' }}
              <n-tag v-if="task.source" size="tiny" :bordered="false" style="margin-left: 4px">{{ getSourceName(task.source) }}</n-tag>
            </div>
            <div v-if="task.file_path && task.status === 'done'" class="task-meta" style="margin-top: 4px; word-break: break-all">
              {{ task.file_path }}
            </div>
            <div v-if="task.error && task.status === 'failed'" style="color: #f85149; font-size: 12px; margin-top: 4px">
              {{ task.error }}
            </div>
            <!-- Per-song upgrade button -->
            <div v-if="canUpgrade(task)" style="text-align: right; margin-top: 6px">
              <n-button
                text
                size="tiny"
                type="primary"
                :loading="upgradingSingle[task.id]"
                @click="handleUpgradeSingle(task.id)"
              >
                ↑ 升级
              </n-button>
            </div>
          </n-card>
        </n-tab-pane>
      </n-tabs>
    </n-spin>
  </div>
</template>

<style scoped>
.task-name {
  font-weight: 500;
  white-space: nowrap;
  text-overflow: ellipsis;
  overflow: hidden;
  flex: 1;
  min-width: 0;
}
.task-meta {
  font-size: 12px;
  color: #8b949e;
}
.upgrade-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  margin-bottom: 12px;
  border: 1px solid #30363d;
  border-radius: 6px;
  background: #161b22;
}
.upgrade-hint {
  font-size: 13px;
  color: #8b949e;
}
</style>
