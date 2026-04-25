<script setup>
import { ref, onMounted, watch } from 'vue'
import {
  NCard, NButton, NTag, NSwitch, NModal, NForm, NFormItem,
  NInput, NInputNumber, NSelect, NSpace, NSpin, NEmpty,
  NDescriptions, NDescriptionsItem, NPopconfirm, useMessage,
} from 'naive-ui'
import { api } from '../api.js'
import { getSourceName } from '../store.js'
import { formatTime } from '../utils.js'

const message = useMessage()
const monitors = ref([])
const loading = ref(false)

// Chart platforms
const chartPlatforms = [
  { label: '网易云', value: 'netease' },
  { label: 'QQ音乐', value: 'qq' },
  { label: '酷狗', value: 'kugou' },
]

const intervalOptions = [
  { label: '每6小时', value: 6 },
  { label: '每12小时', value: 12 },
  { label: '每24小时', value: 24 },
]

// Create/Edit modal
const showModal = ref(false)
const editing = ref(null) // null = create, object = edit
const form = ref({ name: '', platform: '', chart_id: '', top_n: 20, interval: 12 })
const charts = ref([])
const chartsLoading = ref(false)
const saving = ref(false)

// Runs modal
const showRuns = ref(false)
const runsMonitor = ref(null)
const runs = ref([])
const runsLoading = ref(false)

async function loadMonitors() {
  loading.value = true
  try {
    monitors.value = (await api.getMonitors()) || []
  } catch (e) {
    message.error(e.message || '加载监控失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  form.value = { name: '', platform: '', chart_id: '', top_n: 20, interval: 12 }
  charts.value = []
  showModal.value = true
}

function openEdit(m) {
  editing.value = m
  form.value = { name: m.name, platform: m.platform, chart_id: m.chart_id, top_n: m.top_n, interval: m.interval }
  loadCharts(m.platform)
  showModal.value = true
}

async function loadCharts(platform) {
  if (!platform) { charts.value = []; return }
  chartsLoading.value = true
  try {
    const data = (await api.getCharts(platform)) || []
    charts.value = data.map((c) => ({ label: c.name, value: c.id }))
  } catch {
    charts.value = []
  } finally {
    chartsLoading.value = false
  }
}

watch(() => form.value.platform, (v) => {
  form.value.chart_id = ''
  loadCharts(v)
})

async function saveMonitor() {
  if (!form.value.name || !form.value.platform || !form.value.chart_id) {
    return message.warning('请填写完整信息')
  }
  saving.value = true
  try {
    if (editing.value) {
      await api.updateMonitor(editing.value.id, form.value)
      message.success('已更新')
    } else {
      await api.createMonitor(form.value)
      message.success('已创建')
    }
    showModal.value = false
    await loadMonitors()
  } catch (e) {
    message.error(e.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(m) {
  try {
    await api.updateMonitor(m.id, { enabled: !m.enabled })
    m.enabled = !m.enabled
  } catch (e) {
    message.error(e.message || '更新失败')
  }
}

async function deleteMonitor(m) {
  try {
    await api.deleteMonitor(m.id)
    message.success('已删除')
    await loadMonitors()
  } catch (e) {
    message.error(e.message || '删除失败')
  }
}

async function triggerMonitor(m) {
  try {
    await api.triggerMonitor(m.id)
    message.success('已触发执行')
  } catch (e) {
    message.error(e.message || '触发失败')
  }
}

async function openRuns(m) {
  runsMonitor.value = m
  showRuns.value = true
  runsLoading.value = true
  try {
    runs.value = (await api.getMonitorRuns(m.id)) || []
  } catch (e) {
    message.error(e.message || '加载历史失败')
  } finally {
    runsLoading.value = false
  }
}

function runStatusType(s) {
  switch (s) {
    case 'done': return 'success'
    case 'failed': return 'error'
    case 'running': return 'info'
    default: return 'default'
  }
}

onMounted(loadMonitors)
</script>

<template>
  <div>
    <n-space justify="space-between" align="center" style="margin-bottom: 16px">
      <span style="font-size: 16px; font-weight: 600">榜单监控</span>
      <n-button type="primary" size="small" @click="openCreate">新建监控</n-button>
    </n-space>

    <n-spin :show="loading">
      <n-empty v-if="!loading && monitors.length === 0" description="暂无监控规则，点击新建开始" />
      <n-card v-for="m in monitors" :key="m.id" size="small" style="margin-bottom: 10px">
        <div style="display: flex; justify-content: space-between; align-items: flex-start; gap: 12px">
          <div style="flex: 1; min-width: 0">
            <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 4px">
              <span style="font-weight: 500">{{ m.name }}</span>
              <n-tag size="tiny" :bordered="false">{{ getSourceName(m.platform) }}</n-tag>
              <n-tag v-if="!m.enabled" size="tiny" type="default">已暂停</n-tag>
            </div>
            <div style="font-size: 12px; color: #8b949e">
              Top {{ m.top_n }} · 每{{ m.interval }}小时
              <template v-if="m.last_run_at"> · 上次执行: {{ formatTime(m.last_run_at) }}</template>
            </div>
          </div>
          <n-space :size="6" align="center" style="flex-shrink: 0">
            <n-switch :value="m.enabled" size="small" @update:value="() => toggleEnabled(m)" />
            <n-button size="tiny" secondary @click="openRuns(m)">历史</n-button>
            <n-button size="tiny" type="success" secondary @click="triggerMonitor(m)">执行</n-button>
            <n-button size="tiny" secondary @click="openEdit(m)">编辑</n-button>
            <n-popconfirm @positive-click="() => deleteMonitor(m)">
              <template #trigger>
                <n-button size="tiny" type="error" secondary>删除</n-button>
              </template>
              确定删除 "{{ m.name }}" ？
            </n-popconfirm>
          </n-space>
        </div>
      </n-card>
    </n-spin>

    <!-- Create/Edit Modal -->
    <n-modal v-model:show="showModal" preset="card" :title="editing ? '编辑监控' : '新建监控'" style="max-width: 480px">
      <n-form label-placement="left" label-width="80px">
        <n-form-item label="名称">
          <n-input v-model:value="form.name" placeholder="如: 网易热歌榜" />
        </n-form-item>
        <n-form-item label="平台">
          <n-select v-model:value="form.platform" :options="chartPlatforms" placeholder="选择平台" :disabled="!!editing" />
        </n-form-item>
        <n-form-item label="榜单">
          <n-select v-model:value="form.chart_id" :options="charts" :loading="chartsLoading"
            placeholder="先选平台" :disabled="!form.platform || !!editing" />
        </n-form-item>
        <n-form-item label="Top N">
          <n-input-number v-model:value="form.top_n" :min="1" :max="100" style="width: 100%" />
        </n-form-item>
        <n-form-item label="间隔">
          <n-select v-model:value="form.interval" :options="intervalOptions" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space justify="end">
          <n-button @click="showModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="saveMonitor">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Runs Modal -->
    <n-modal v-model:show="showRuns" preset="card" :title="`执行历史 — ${runsMonitor?.name || ''}`" style="max-width: 600px; max-height: 80vh">
      <n-spin :show="runsLoading">
        <n-empty v-if="!runsLoading && runs.length === 0" description="暂无执行记录" />
        <n-card v-for="run in runs.slice(0, 10)" :key="run.id" size="small" style="margin-bottom: 8px">
          <n-space justify="space-between" align="center">
            <div>
              <n-tag :type="runStatusType(run.status)" size="tiny" style="margin-right: 8px">{{ run.status }}</n-tag>
              <span style="font-size: 12px; color: #8b949e">{{ formatTime(run.started_at) }}</span>
            </div>
            <div style="font-size: 12px; color: #8b949e">
              抓取 {{ run.total_fetched }} · 新增 {{ run.new_queued }} · 跳过 {{ run.skipped }}
            </div>
          </n-space>
          <div v-if="run.error" style="color: #f85149; font-size: 12px; margin-top: 4px">{{ run.error }}</div>
        </n-card>
      </n-spin>
    </n-modal>
  </div>
</template>
