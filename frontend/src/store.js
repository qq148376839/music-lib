import { reactive } from 'vue'

export const store = reactive({
  nasEnabled: false,
  nasMusicDir: '',
  quality: localStorage.getItem('quality') || 'lossless',
  loginState: {
    netease: { loggedIn: false, nickname: '' },
    qq: { loggedIn: false, nickname: '' },
  },
  providers: [
    { id: 'netease', name: '网易云' },
    { id: 'qq', name: 'QQ音乐' },
    { id: 'kugou', name: '酷狗' },
    { id: 'kuwo', name: '酷我' },
    { id: 'migu', name: '咪咕' },
    { id: 'qianqian', name: '千千' },
    { id: 'soda', name: '汽水' },
    { id: 'fivesing', name: '5sing' },
    { id: 'jamendo', name: 'Jamendo' },
    { id: 'joox', name: 'JOOX' },
    { id: 'bilibili', name: 'B站' },
  ],
})

export function setQuality(q) {
  store.quality = q
  localStorage.setItem('quality', q)
}

export function getSourceName(id) {
  const p = store.providers.find((v) => v.id === id)
  return p ? p.name : id || '未知'
}
