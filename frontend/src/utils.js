export function formatDuration(seconds) {
  if (!seconds || seconds <= 0) return '-'
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

export function formatSize(bytes) {
  if (!bytes || bytes <= 0) return '-'
  const mb = bytes / 1024 / 1024
  return `${mb.toFixed(2)} MB`
}

export function formatTime(ts) {
  if (!ts) return '-'
  const d = new Date(ts)
  if (isNaN(d.getTime())) return '-'
  return d.toLocaleString('zh-CN', { hour12: false })
}
