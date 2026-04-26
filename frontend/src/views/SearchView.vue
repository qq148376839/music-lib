<script setup>
import { ref, computed } from 'vue'
import {
  NTabs, NTabPane, NInput, NButton, NSelect, NSpace, NSpin, NEmpty,
  NCard, NTag, NModal, NGrid, NGi, NDropdown, useMessage,
} from 'naive-ui'
import { api } from '../api.js'
import { store } from '../store.js'
import { getSourceName } from '../store.js'
import { formatDuration, formatSize } from '../utils.js'

const message = useMessage()

// Source selector
const source = ref('all')
const sourceOptions = computed(() => [
  { label: '全部平台', value: 'all' },
  ...store.providers.map((p) => ({ label: p.name, value: p.id })),
])

// Platform capability check
const canRecommend = computed(() => {
  if (source.value === 'all') return store.providers.some((p) => p.capabilities?.playlist_recommended)
  const p = store.providers.find((v) => v.id === source.value)
  return p?.capabilities?.playlist_recommended ?? false
})

// --- Song Search ---
const songKeyword = ref('')
const songLoading = ref(false)
const songs = ref([])

async function searchSongs() {
  const kw = songKeyword.value.trim()
  if (!kw) return message.warning('请输入关键词')
  songLoading.value = true
  songs.value = []
  try {
    if (source.value === 'all') {
      const results = await Promise.allSettled(
        store.providers.map((p) => api.searchSongs(p.id, kw).catch(() => []))
      )
      songs.value = results.flatMap((r) => (r.status === 'fulfilled' && Array.isArray(r.value) ? r.value : []))
    } else {
      songs.value = (await api.searchSongs(source.value, kw)) || []
    }
    if (songs.value.length === 0) message.info('未找到结果')
  } catch (e) {
    message.error(e.message || '搜索失败')
  } finally {
    songLoading.value = false
  }
}

// --- Playlist Search ---
const plKeyword = ref('')
const plLoading = ref(false)
const playlists = ref([])
const plDetail = ref(null) // { playlist, songs }
const plDetailLoading = ref(false)

async function searchPlaylists() {
  const kw = plKeyword.value.trim()
  if (!kw) return message.warning('请输入关键词')
  plLoading.value = true
  playlists.value = []
  plDetail.value = null
  try {
    if (source.value === 'all') {
      const results = await Promise.allSettled(
        store.providers.map((p) => api.searchPlaylists(p.id, kw).catch(() => []))
      )
      playlists.value = results.flatMap((r) => (r.status === 'fulfilled' && Array.isArray(r.value) ? r.value : []))
    } else {
      playlists.value = (await api.searchPlaylists(source.value, kw)) || []
    }
  } catch (e) {
    message.error(e.message || '搜索失败')
  } finally {
    plLoading.value = false
  }
}

async function loadRecommended() {
  plLoading.value = true
  playlists.value = []
  plDetail.value = null
  try {
    if (source.value === 'all') {
      const supported = store.providers.filter((p) => p.capabilities?.playlist_recommended)
      const results = await Promise.allSettled(
        supported.map((p) => api.getRecommended(p.id).catch(() => []))
      )
      playlists.value = results.flatMap((r) => (r.status === 'fulfilled' && Array.isArray(r.value) ? r.value : []))
    } else {
      playlists.value = (await api.getRecommended(source.value)) || []
    }
  } catch (e) {
    message.error(e.message || '获取推荐失败')
  } finally {
    plLoading.value = false
  }
}

async function openPlaylist(pl) {
  const src = pl.source || source.value
  if (!src || src === 'all') return message.warning('无法确定来源')
  plDetailLoading.value = true
  try {
    const plSongs = (await api.getPlaylistSongs(src, pl.id)) || []
    plDetail.value = { playlist: pl, songs: plSongs, source: src }
  } catch (e) {
    message.error(e.message || '获取歌单失败')
  } finally {
    plDetailLoading.value = false
  }
}

async function batchNASDownload() {
  if (!plDetail.value) return
  const { songs: plSongs, playlist, source: src } = plDetail.value
  try {
    const data = await api.nasBatchDownload(src, store.quality, playlist.name, plSongs)
    message.success(`已加入下载队列 (${data?.task_count || plSongs.length}首)`)
  } catch (e) {
    message.error(e.message || '批量下载失败')
  }
}

// --- Parse ---
const parseLink = ref('')
const parseType = ref('song')
const parseLoading = ref(false)
const parseResult = ref(null) // { type: 'song'|'playlist', song?, playlist?, songs? }

async function doParse() {
  const link = parseLink.value.trim()
  if (!link) return message.warning('请输入链接')
  if (source.value === 'all') return message.warning('解析需选择具体平台')
  parseLoading.value = true
  parseResult.value = null
  try {
    if (parseType.value === 'song') {
      const song = await api.parseSong(source.value, link)
      parseResult.value = song ? { type: 'song', song } : null
    } else {
      const data = await api.parsePlaylist(source.value, link)
      parseResult.value = data ? { type: 'playlist', playlist: data.playlist, songs: data.songs || [] } : null
    }
    if (!parseResult.value) message.info('未解析到内容')
  } catch (e) {
    message.error(e.message || '解析失败')
  } finally {
    parseLoading.value = false
  }
}

// --- Lyrics Modal ---
const lyricsVisible = ref(false)
const lyricsTitle = ref('')
const lyricsText = ref('')

async function showLyrics(song) {
  const src = song.source || (source.value !== 'all' ? source.value : '')
  if (!src) return message.warning('无法确定来源')
  try {
    const data = await api.getLyrics(src, song)
    lyricsTitle.value = song.name || '歌词'
    lyricsText.value = data?.lyrics || '暂无歌词'
    lyricsVisible.value = true
  } catch (e) {
    message.error(e.message || '获取歌词失败')
  }
}

// --- Download ---
function resolveSource(song) {
  if (song.source && song.source !== 'all') return song.source
  if (plDetail.value?.source) return plDetail.value.source
  if (source.value !== 'all') return source.value
  return ''
}

async function browserDownload(song) {
  const src = resolveSource(song)
  if (!src) return message.warning('无法确定来源')
  try {
    const resp = await api.proxyDownload(src, store.quality, song)
    if (!resp.ok) throw new Error(`下载失败 (HTTP ${resp.status})`)
    const blob = await resp.blob()
    const ext = song.ext || 'mp3'
    const filename = `${song.artist || '未知'} - ${song.name || '未知'}.${ext}`
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url; a.download = filename
    document.body.appendChild(a); a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
    message.success('下载已开始')
  } catch (e) {
    message.error(e.message || '下载失败')
  }
}

async function nasDownload(song) {
  const src = resolveSource(song)
  if (!src) return message.warning('无法确定来源')
  try {
    await api.nasDownload(src, store.quality, song)
    message.success('已加入NAS下载队列')
  } catch (e) {
    message.error(e.message || 'NAS下载失败')
  }
}

function downloadActions(song) {
  const opts = [{ label: '浏览器下载', key: 'browser' }]
  if (store.nasEnabled) opts.push({ label: '下载到NAS', key: 'nas' })
  return opts
}

function onDownloadSelect(key, song) {
  if (key === 'browser') browserDownload(song)
  else if (key === 'nas') nasDownload(song)
}
</script>

<template>
  <div>
    <!-- Source Selector -->
    <n-space align="center" style="margin-bottom: 16px" :wrap="true">
      <span style="color: #8b949e; font-size: 14px">平台</span>
      <n-select v-model:value="source" :options="sourceOptions" size="small" style="width: 140px" />
    </n-space>

    <n-tabs type="line" animated>
      <!-- Songs Tab -->
      <n-tab-pane name="songs" tab="搜索歌曲">
        <n-space style="margin-bottom: 16px">
          <n-input v-model:value="songKeyword" placeholder="歌曲名、歌手..." clearable
            @keydown.enter="searchSongs" style="min-width: 240px" />
          <n-button type="primary" :loading="songLoading" @click="searchSongs">搜索</n-button>
        </n-space>
        <n-spin :show="songLoading">
          <n-empty v-if="!songLoading && songs.length === 0" description="搜索歌曲开始探索" />
          <div v-else class="song-list">
            <n-card v-for="(song, i) in songs" :key="i" size="small" style="margin-bottom: 8px">
              <div class="song-row">
                <div class="song-info">
                  <div class="song-name">{{ song.name || '未知' }}</div>
                  <div class="song-meta">
                    {{ song.artist || '未知' }}
                    <n-tag v-if="song.source" size="tiny" :bordered="false" style="margin-left: 6px">{{ getSourceName(song.source) }}</n-tag>
                    <span v-if="song.album"> · {{ song.album }}</span>
                    <span v-if="song.duration"> · {{ formatDuration(song.duration) }}</span>
                    <span v-if="song.size"> · {{ formatSize(song.size) }}</span>
                  </div>
                </div>
                <n-space :size="6" style="flex-shrink: 0">
                  <n-dropdown :options="downloadActions(song)" @select="(k) => onDownloadSelect(k, song)" trigger="click">
                    <n-button size="tiny" type="warning" secondary>下载</n-button>
                  </n-dropdown>
                  <n-button size="tiny" type="info" secondary @click="showLyrics(song)">歌词</n-button>
                </n-space>
              </div>
            </n-card>
          </div>
        </n-spin>
      </n-tab-pane>

      <!-- Playlists Tab -->
      <n-tab-pane name="playlists" tab="歌单">
        <template v-if="!plDetail">
          <n-space style="margin-bottom: 16px">
            <n-input v-model:value="plKeyword" placeholder="搜索歌单..." clearable
              @keydown.enter="searchPlaylists" style="min-width: 200px" />
            <n-button type="primary" :loading="plLoading" @click="searchPlaylists">搜索</n-button>
            <n-button v-if="canRecommend" :loading="plLoading" @click="loadRecommended">推荐</n-button>
          </n-space>
          <n-spin :show="plLoading">
            <n-empty v-if="!plLoading && playlists.length === 0" description="搜索或浏览推荐歌单" />
            <n-grid v-else :cols="2" :x-gap="12" :y-gap="12" responsive="screen" :item-responsive="true">
              <n-gi v-for="(pl, i) in playlists" :key="i" span="2 m:1">
                <n-card hoverable size="small" @click="openPlaylist(pl)" style="cursor: pointer">
                  <div style="display: flex; gap: 12px; align-items: center">
                    <img v-if="pl.cover" :src="pl.cover" style="width: 60px; height: 60px; border-radius: 8px; object-fit: cover" loading="lazy" @error="(e) => e.target.style.display='none'" />
                    <div style="min-width: 0; flex: 1">
                      <div style="font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">{{ pl.name || '未知' }}</div>
                      <div style="font-size: 12px; color: #8b949e; margin-top: 4px">
                        {{ pl.creator || '' }}{{ pl.track_count != null ? ` · ${pl.track_count}首` : '' }}
                        <n-tag v-if="pl.source" size="tiny" :bordered="false" style="margin-left: 4px">{{ getSourceName(pl.source) }}</n-tag>
                      </div>
                    </div>
                  </div>
                </n-card>
              </n-gi>
            </n-grid>
          </n-spin>
        </template>

        <!-- Playlist Detail -->
        <template v-else>
          <n-space align="center" style="margin-bottom: 16px">
            <n-button text @click="plDetail = null">← 返回</n-button>
            <span style="font-weight: 600">{{ plDetail.playlist?.name || '歌单详情' }}</span>
            <n-button v-if="store.nasEnabled" size="small" type="success" @click="batchNASDownload">批量下载到NAS</n-button>
          </n-space>
          <n-spin :show="plDetailLoading">
            <div class="song-list">
              <n-card v-for="(song, i) in plDetail.songs" :key="i" size="small" style="margin-bottom: 8px">
                <div class="song-row">
                  <div class="song-info">
                    <div class="song-name">{{ song.name || '未知' }}</div>
                    <div class="song-meta">
                      {{ song.artist || '未知' }}
                      <span v-if="song.album"> · {{ song.album }}</span>
                      <span v-if="song.duration"> · {{ formatDuration(song.duration) }}</span>
                    </div>
                  </div>
                  <n-space :size="6" style="flex-shrink: 0">
                    <n-dropdown :options="downloadActions(song)" @select="(k) => onDownloadSelect(k, song)" trigger="click">
                      <n-button size="tiny" type="warning" secondary>下载</n-button>
                    </n-dropdown>
                    <n-button size="tiny" type="info" secondary @click="showLyrics(song)">歌词</n-button>
                  </n-space>
                </div>
              </n-card>
            </div>
          </n-spin>
        </template>
      </n-tab-pane>

      <!-- Parse Tab -->
      <n-tab-pane name="parse" tab="链接解析">
        <n-space vertical style="margin-bottom: 16px">
          <n-input v-model:value="parseLink" type="textarea" placeholder="粘贴歌曲或歌单链接..." :rows="2" />
          <n-space>
            <n-select v-model:value="parseType" :options="[{label:'歌曲',value:'song'},{label:'歌单',value:'playlist'}]" size="small" style="width: 100px" />
            <n-button type="primary" :loading="parseLoading" @click="doParse">解析</n-button>
          </n-space>
        </n-space>
        <n-spin :show="parseLoading">
          <n-empty v-if="!parseResult" description="输入链接进行解析" />
          <template v-else>
            <template v-if="parseResult.type === 'song'">
              <n-card size="small">
                <div class="song-row">
                  <div class="song-info">
                    <div class="song-name">{{ parseResult.song.name }}</div>
                    <div class="song-meta">{{ parseResult.song.artist }} · {{ parseResult.song.album || '' }}</div>
                  </div>
                  <n-space :size="6" style="flex-shrink: 0">
                    <n-dropdown :options="downloadActions(parseResult.song)" @select="(k) => onDownloadSelect(k, parseResult.song)" trigger="click">
                      <n-button size="tiny" type="warning" secondary>下载</n-button>
                    </n-dropdown>
                  </n-space>
                </div>
              </n-card>
            </template>
            <template v-else>
              <div v-if="parseResult.playlist" style="margin-bottom: 12px; font-weight: 500">
                {{ parseResult.playlist.name }} ({{ parseResult.songs.length }}首)
              </div>
              <n-card v-for="(song, i) in parseResult.songs" :key="i" size="small" style="margin-bottom: 8px">
                <div class="song-row">
                  <div class="song-info">
                    <div class="song-name">{{ song.name }}</div>
                    <div class="song-meta">{{ song.artist }}</div>
                  </div>
                  <n-space :size="6" style="flex-shrink: 0">
                    <n-dropdown :options="downloadActions(song)" @select="(k) => onDownloadSelect(k, song)" trigger="click">
                      <n-button size="tiny" type="warning" secondary>下载</n-button>
                    </n-dropdown>
                  </n-space>
                </div>
              </n-card>
            </template>
          </template>
        </n-spin>
      </n-tab-pane>
    </n-tabs>

    <!-- Lyrics Modal -->
    <n-modal v-model:show="lyricsVisible" preset="card" :title="lyricsTitle" style="max-width: 500px; max-height: 80vh">
      <pre style="white-space: pre-wrap; color: #8b949e; line-height: 2; font-family: inherit; font-size: 13px">{{ lyricsText }}</pre>
    </n-modal>
  </div>
</template>

<style scoped>
.song-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.song-info {
  flex: 1;
  min-width: 0;
  overflow: hidden;
}
.song-name {
  font-weight: 500;
  white-space: nowrap;
  text-overflow: ellipsis;
  overflow: hidden;
}
.song-meta {
  font-size: 12px;
  color: #8b949e;
  white-space: nowrap;
  text-overflow: ellipsis;
  overflow: hidden;
  margin-top: 2px;
}
@media (max-width: 640px) {
  .song-row { flex-wrap: wrap; }
  .song-row > .n-space { width: 100%; margin-top: 8px; }
}
</style>
