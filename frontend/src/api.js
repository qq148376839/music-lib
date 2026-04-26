async function request(url, options = {}) {
  const resp = await fetch(url, options)
  if (!resp.ok) {
    let msg = `HTTP ${resp.status}`
    try {
      const j = await resp.json()
      if (j.message) msg = j.message
      if (j.error) msg = j.error
    } catch { /* ignore */ }
    throw new Error(msg)
  }
  const json = await resp.json()
  if (json.code !== undefined && json.code !== 0) {
    throw new Error(json.message || json.error || '请求失败')
  }
  return json.data
}

function post(url, body) {
  return request(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

function put(url, body) {
  return request(url, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

function del(url) {
  return request(url, { method: 'DELETE' })
}

export const api = {
  // Providers
  getProviders: () => request('/providers'),

  // Search
  searchSongs: (source, keyword) =>
    request(`/api/search?source=${encodeURIComponent(source)}&keyword=${encodeURIComponent(keyword)}`),
  getLyrics: (source, song) =>
    post(`/api/lyrics?source=${encodeURIComponent(source)}`, song),
  parseSong: (source, link) =>
    request(`/api/parse?source=${encodeURIComponent(source)}&link=${encodeURIComponent(link)}`),

  // Playlist
  searchPlaylists: (source, keyword) =>
    request(`/api/playlist/search?source=${encodeURIComponent(source)}&keyword=${encodeURIComponent(keyword)}`),
  getPlaylistSongs: (source, id) =>
    request(`/api/playlist/songs?source=${encodeURIComponent(source)}&id=${encodeURIComponent(id)}`),
  parsePlaylist: (source, link) =>
    request(`/api/playlist/parse?source=${encodeURIComponent(source)}&link=${encodeURIComponent(link)}`),
  getRecommended: (source) =>
    request(`/api/playlist/recommended?source=${encodeURIComponent(source)}`),

  // Download
  proxyDownload: (source, quality, song) =>
    fetch(`/api/download/file?source=${encodeURIComponent(source)}&quality=${encodeURIComponent(quality)}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(song),
    }),
  nasDownload: (source, quality, song) =>
    post(`/api/nas/download?source=${encodeURIComponent(source)}&quality=${encodeURIComponent(quality)}`, song),
  nasBatchDownload: (source, quality, name, songs) =>
    post(`/api/nas/download/batch?source=${encodeURIComponent(source)}&quality=${encodeURIComponent(quality)}`, {
      playlist_name: name,
      songs,
    }),
  getNASStatus: () => request('/api/nas/status'),
  getTasks: () => request('/api/nas/tasks'),
  getBatches: () => request('/api/nas/batches'),
  upgradeDownloads: (quality, taskIds) =>
    post(`/api/nas/download/upgrade?quality=${encodeURIComponent(quality)}`, {
      task_ids: taskIds || [],
    }),

  // Login
  getLoginStatus: (platform) => request(`/api/login/status?platform=${encodeURIComponent(platform)}`),
  startQRLogin: (platform) => post(`/api/login/qr/start?platform=${encodeURIComponent(platform)}`),
  pollQRLogin: (platform) => request(`/api/login/qr/poll?platform=${encodeURIComponent(platform)}`),
  loginWithCookie: (platform, cookie, nickname) =>
    post(`/api/login/cookie?platform=${encodeURIComponent(platform)}`, { cookie, nickname }),
  logout: (platform) => post(`/api/login/logout?platform=${encodeURIComponent(platform)}`),

  // Charts & Monitors
  getCharts: (source) => request(`/api/charts?source=${encodeURIComponent(source)}`),
  getMonitors: () => request('/api/monitors'),
  createMonitor: (data) => post('/api/monitors', data),
  updateMonitor: (id, data) => put(`/api/monitors/${id}`, data),
  deleteMonitor: (id) => del(`/api/monitors/${id}`),
  getMonitorRuns: (id) => request(`/api/monitors/${id}/runs`),
  triggerMonitor: (id) => post(`/api/monitors/${id}/trigger`),
  resolvePlaylist: (url) => post('/api/monitors/resolve', { url }),
}
