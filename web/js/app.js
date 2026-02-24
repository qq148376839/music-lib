/**
 * Music Library SPA - Main Application
 * Talks to the Go API server at the same origin.
 */
(function () {
  'use strict';

  // ---------------------------------------------------------------------------
  // Constants
  // ---------------------------------------------------------------------------

  const PROVIDERS = [
    { id: 'netease',  name: '网易云' },
    { id: 'qq',       name: 'QQ音乐' },
    { id: 'kugou',    name: '酷狗' },
    { id: 'kuwo',     name: '酷我' },
    { id: 'migu',     name: '咪咕' },
    { id: 'qianqian', name: '千千' },
    { id: 'soda',     name: '汽水' },
    { id: 'fivesing', name: '5sing' },
    { id: 'jamendo',  name: 'Jamendo' },
    { id: 'joox',     name: 'JOOX' },
    { id: 'bilibili', name: 'B站' },
  ];

  const TOAST_DURATION = 2500;
  const NAS_POLL_INTERVAL = 3000;
  const QR_POLL_INTERVAL = 2000;

  const PLATFORM_NAMES = {
    netease: '网易云音乐',
    qq: 'QQ音乐',
  };

  // ---------------------------------------------------------------------------
  // DOM references (cached once on load)
  // ---------------------------------------------------------------------------

  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

  let dom; // populated in init()
  let nasEnabled = false;
  let nasTaskPollingTimer = null;
  let nasBadgePollingTimer = null;
  let currentPlaylistSongs = [];
  let currentPlaylistName = '';
  let currentPlaylistSource = '';
  let qrPollingTimer = null;
  let currentQRPlatform = null;

  // Login state per platform
  let loginState = {
    netease: { loggedIn: false, nickname: '' },
    qq:      { loggedIn: false, nickname: '' },
  };

  // ---------------------------------------------------------------------------
  // Utility functions
  // ---------------------------------------------------------------------------

  /** Format seconds into "MM:SS" */
  function formatDuration(seconds) {
    if (!seconds || seconds <= 0) return '-';
    const m = Math.floor(seconds / 60);
    const s = Math.floor(seconds % 60);
    return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  }

  /** Format byte count into "X.XX MB" */
  function formatSize(bytes) {
    if (!bytes || bytes <= 0) return '-';
    const mb = bytes / 1024 / 1024;
    return `${mb.toFixed(2)} MB`;
  }

  /** Show a toast notification for TOAST_DURATION ms */
  function showToast(msg) {
    const el = dom.toast;
    el.textContent = msg;
    el.classList.add('show');
    clearTimeout(el._timer);
    el._timer = setTimeout(() => el.classList.remove('show'), TOAST_DURATION);
  }

  /** Show the loading overlay */
  function showLoading() {
    dom.loading.classList.add('show');
  }

  /** Hide the loading overlay */
  function hideLoading() {
    dom.loading.classList.remove('show');
  }

  /** Look up the Chinese display name for a provider id */
  function getSourceName(sourceId) {
    const p = PROVIDERS.find((v) => v.id === sourceId);
    return p ? p.name : sourceId || '未知';
  }

  /** Get the currently selected source from the dropdown */
  function getSelectedSource() {
    return dom.providerSelect.value;
  }

  /** Get the currently selected quality from the dropdown */
  function getQuality() {
    return dom.qualitySelect ? dom.qualitySelect.value : 'lossless';
  }

  /**
   * Resolve the effective source for a song using a three-level fallback:
   *   song.source → currentPlaylistSource → getSelectedSource()
   * Returns '' if none of the levels yield a usable (non-"all") value.
   */
  function resolveSource(song) {
    const candidates = [
      song && song.source,
      currentPlaylistSource,
      getSelectedSource(),
    ];
    for (const s of candidates) {
      if (s && s !== 'all') return s;
    }
    return '';
  }

  // ---------------------------------------------------------------------------
  // API helpers
  // ---------------------------------------------------------------------------

  async function apiFetch(url, options = {}, silent = false) {
    if (!silent) showLoading();
    try {
      const resp = await fetch(url, options);
      const json = await resp.json();
      if (json.code !== 0) {
        throw new Error(json.message || `请求失败 (code: ${json.code})`);
      }
      return json.data;
    } catch (err) {
      if (err instanceof SyntaxError) {
        throw new Error('服务器返回了无效的数据');
      }
      throw err;
    } finally {
      if (!silent) hideLoading();
    }
  }

  async function fetchAllProviders(fetcher) {
    showLoading();
    try {
      const promises = PROVIDERS.map((p) =>
        fetcher(p.id).catch(() => [])
      );
      const results = await Promise.allSettled(promises);
      const merged = [];
      for (const r of results) {
        if (r.status === 'fulfilled' && Array.isArray(r.value)) {
          merged.push(...r.value);
        }
      }
      return merged;
    } finally {
      hideLoading();
    }
  }

  // ---------------------------------------------------------------------------
  // Rendering helpers
  // ---------------------------------------------------------------------------

  function el(tag, className, text) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  function renderSongCard(song) {
    const card = el('div', 'song-card');

    const info = el('div', 'song-info');
    info.appendChild(el('div', 'song-name', song.name || '未知歌曲'));

    const metaParts = [
      song.artist || '未知歌手',
      song.album || '',
      getSourceName(song.source),
      formatDuration(song.duration),
      formatSize(song.size),
    ].filter(Boolean);

    const meta = el('div', 'song-meta');

    metaParts.forEach((part, idx) => {
      if (part === getSourceName(song.source)) {
        const tag = el('span', 'source-tag', part);
        meta.appendChild(tag);
      } else {
        meta.appendChild(document.createTextNode(part));
      }
      if (idx < metaParts.length - 1) {
        meta.appendChild(document.createTextNode(' \u00b7 '));
      }
    });

    info.appendChild(meta);
    card.appendChild(info);

    const actions = el('div', 'song-actions');

    const dlWrap = el('div', 'download-dropdown');
    const btnDl = el('button', 'btn-download');
    btnDl.innerHTML = '<svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor" style="vertical-align:-1px;margin-right:4px"><path d="M2.75 14A1.75 1.75 0 0 1 1 12.25v-2.5a.75.75 0 0 1 1.5 0v2.5c0 .138.112.25.25.25h10.5a.25.25 0 0 0 .25-.25v-2.5a.75.75 0 0 1 1.5 0v2.5A1.75 1.75 0 0 1 13.25 14ZM7.25 7.689V2a.75.75 0 0 1 1.5 0v5.689l1.97-1.969a.749.749 0 1 1 1.06 1.06l-3.25 3.25a.749.749 0 0 1-1.06 0L4.22 6.78a.749.749 0 1 1 1.06-1.06Z"/></svg>下载';
    btnDl.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      closeAllMenus();
      dlMenu.classList.toggle('show');
    });
    dlWrap.appendChild(btnDl);

    const dlMenu = el('div', 'download-menu');

    const browserItem = el('button', 'download-menu-item');
    browserItem.innerHTML = '<span class="menu-icon">&#8615;</span>浏览器下载';
    browserItem.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      dlMenu.classList.remove('show');
      handleBrowserDownload(song);
    });
    dlMenu.appendChild(browserItem);

    if (nasEnabled) {
      const nasItem = el('button', 'download-menu-item');
      nasItem.innerHTML = '<span class="menu-icon">&#9776;</span>下载到NAS';
      nasItem.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        dlMenu.classList.remove('show');
        handleNASDownload(song);
      });
      dlMenu.appendChild(nasItem);
    }

    dlWrap.appendChild(dlMenu);
    actions.appendChild(dlWrap);

    const btnLyrics = el('button', 'btn-lyrics', '歌词');
    btnLyrics.addEventListener('click', () => handleLyrics(song));
    actions.appendChild(btnLyrics);

    card.appendChild(actions);
    return card;
  }

  function closeAllMenus() {
    $$('.download-menu.show').forEach((m) => m.classList.remove('show'));
  }

  function renderPlaylistCard(playlist) {
    const card = el('div', 'playlist-card');
    card.addEventListener('click', () => handlePlaylistDetail(playlist));

    const coverWrap = el('div', 'playlist-cover');
    if (playlist.cover) {
      const img = document.createElement('img');
      img.src = playlist.cover;
      img.alt = playlist.name || '';
      img.loading = 'lazy';
      img.onerror = function () {
        this.replaceWith(createMusicNoteIcon());
      };
      coverWrap.appendChild(img);
    } else {
      coverWrap.appendChild(createMusicNoteIcon());
    }
    card.appendChild(coverWrap);

    const info = el('div', 'playlist-info');
    info.appendChild(el('div', 'playlist-name', playlist.name || '未知歌单'));

    const metaParts = [
      playlist.creator || '',
      playlist.track_count != null ? `${playlist.track_count}首` : '',
    ].filter(Boolean);
    info.appendChild(el('div', 'playlist-meta', metaParts.join(' \u00b7 ')));

    card.appendChild(info);
    return card;
  }

  function createMusicNoteIcon() {
    const wrapper = el('div', 'music-note-icon');
    wrapper.innerHTML =
      '<svg viewBox="0 0 24 24" width="48" height="48" fill="currentColor">' +
      '<path d="M12 3v10.55A4 4 0 1 0 14 17V7h4V3h-6z"/>' +
      '</svg>';
    return wrapper;
  }

  function renderInto(container, children) {
    container.innerHTML = '';
    if (!children || children.length === 0) {
      container.appendChild(el('div', 'empty-state', '暂无结果'));
      return;
    }
    const frag = document.createDocumentFragment();
    children.forEach((c) => frag.appendChild(c));
    container.appendChild(frag);
  }

  // ---------------------------------------------------------------------------
  // Tab switching
  // ---------------------------------------------------------------------------

  function initTabs() {
    const tabBtns = $$('.tab-btn[data-tab]');
    const tabPanels = [dom.tabSearch, dom.tabPlaylist, dom.tabParse, dom.tabNasTasks];

    tabBtns.forEach((btn) => {
      btn.addEventListener('click', () => {
        tabBtns.forEach((b) => b.classList.remove('active'));
        btn.classList.add('active');

        const targetId = `tab-${btn.dataset.tab}`;
        tabPanels.forEach((panel) => {
          panel.classList.toggle('active', panel.id === targetId);
        });

        if (btn.dataset.tab === 'nas-tasks') {
          stopBadgePolling();
          startTaskPolling();
        } else {
          stopTaskPolling();
          if (nasEnabled) startBadgePolling();
        }
      });
    });
  }

  // ---------------------------------------------------------------------------
  // Song search
  // ---------------------------------------------------------------------------

  async function doSongSearch() {
    const keyword = dom.searchInput.value.trim();
    if (!keyword) {
      showToast('请输入搜索关键词');
      return;
    }

    const source = getSelectedSource();

    try {
      let songs;
      if (source === 'all') {
        songs = await fetchAllProviders((srcId) =>
          apiFetch(
            `/api/search?source=${encodeURIComponent(srcId)}&keyword=${encodeURIComponent(keyword)}`,
            {},
            true
          )
        );
      } else {
        songs = await apiFetch(
          `/api/search?source=${encodeURIComponent(source)}&keyword=${encodeURIComponent(keyword)}`
        );
      }

      const cards = (songs || []).map(renderSongCard);
      renderInto(dom.searchResults, cards);
    } catch (err) {
      showToast(err.message || '搜索失败');
    }
  }

  // ---------------------------------------------------------------------------
  // Browser Download (proxy through server)
  // ---------------------------------------------------------------------------

  async function handleBrowserDownload(song) {
    const source = resolveSource(song);
    if (!source) {
      showToast('无法确定歌曲来源');
      return;
    }

    showLoading();
    try {
      const resp = await fetch(
        `/api/download/file?source=${encodeURIComponent(source)}&quality=${encodeURIComponent(getQuality())}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(song),
        }
      );

      if (!resp.ok) {
        let errMsg = `下载失败 (HTTP ${resp.status})`;
        try {
          const errData = await resp.json();
          if (errData.message) errMsg = errData.message;
        } catch {
          // keep default message
        }
        throw new Error(errMsg);
      }

      const blob = await resp.blob();
      const ext = song.ext || 'mp3';
      const artist = song.artist || '未知歌手';
      const name = song.name || '未知歌曲';
      const filename = `${artist} - ${name}.${ext}`;

      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);

      showToast('下载已开始');
    } catch (err) {
      showToast(err.message || '下载失败');
    } finally {
      hideLoading();
    }
  }

  // ---------------------------------------------------------------------------
  // NAS Download
  // ---------------------------------------------------------------------------

  async function handleNASDownload(song) {
    const source = resolveSource(song);
    if (!source) {
      showToast('无法确定歌曲来源');
      return;
    }

    try {
      await apiFetch(
        `/api/nas/download?source=${encodeURIComponent(source)}&quality=${encodeURIComponent(getQuality())}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(song),
        }
      );
      showToast('已加入NAS下载队列');
      startBadgePolling();
    } catch (err) {
      showToast(err.message || 'NAS下载失败');
    }
  }

  async function handleNASBatchDownload() {
    if (!currentPlaylistSongs || currentPlaylistSongs.length === 0) {
      showToast('歌单无歌曲');
      return;
    }

    const source = currentPlaylistSource || getSelectedSource();
    if (!source || source === 'all') {
      showToast('无法确定歌曲来源');
      return;
    }

    try {
      const data = await apiFetch(
        `/api/nas/download/batch?source=${encodeURIComponent(source)}&quality=${encodeURIComponent(getQuality())}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            playlist_name: currentPlaylistName,
            songs: currentPlaylistSongs,
          }),
        }
      );
      showToast(`已加入NAS下载队列 (${data.task_count}首)`);
      startBadgePolling();
    } catch (err) {
      showToast(err.message || '批量下载失败');
    }
  }

  async function handleBrowserBatchDownload() {
    if (!currentPlaylistSongs || currentPlaylistSongs.length === 0) {
      showToast('歌单无歌曲');
      return;
    }

    const songs = currentPlaylistSongs;
    const total = songs.length;
    showToast(`开始批量下载 ${total} 首歌曲...`);

    let success = 0;
    let fail = 0;
    for (let i = 0; i < songs.length; i++) {
      try {
        await handleBrowserDownload(songs[i]);
        success++;
      } catch {
        fail++;
      }
      if (i < songs.length - 1) {
        await new Promise((r) => setTimeout(r, 1500));
      }
    }
    showToast(`批量下载完成: ${success}成功, ${fail}失败`);
  }

  // ---------------------------------------------------------------------------
  // Lyrics
  // ---------------------------------------------------------------------------

  async function handleLyrics(song) {
    const source = resolveSource(song);
    if (!source) {
      showToast('无法确定歌曲来源');
      return;
    }

    try {
      const data = await apiFetch(
        `/api/lyrics?source=${encodeURIComponent(source)}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(song),
        }
      );

      showLyricsModal(song.name || '歌词', (data && data.lyrics) || '暂无歌词');
    } catch (err) {
      showToast(err.message || '获取歌词失败');
    }
  }

  function showLyricsModal(title, text) {
    dom.lyricsTitle.textContent = title;
    dom.lyricsText.textContent = text;
    dom.lyricsModal.classList.add('show');
  }

  function closeLyricsModal() {
    dom.lyricsModal.classList.remove('show');
  }

  // ---------------------------------------------------------------------------
  // Playlist search & recommend
  // ---------------------------------------------------------------------------

  async function doPlaylistSearch() {
    const keyword = dom.playlistKeyword.value.trim();
    if (!keyword) {
      showToast('请输入搜索关键词');
      return;
    }

    const source = getSelectedSource();

    try {
      let playlists;
      if (source === 'all') {
        playlists = await fetchAllProviders((srcId) =>
          apiFetch(
            `/api/playlist/search?source=${encodeURIComponent(srcId)}&keyword=${encodeURIComponent(keyword)}`,
            {},
            true
          )
        );
      } else {
        playlists = await apiFetch(
          `/api/playlist/search?source=${encodeURIComponent(source)}&keyword=${encodeURIComponent(keyword)}`
        );
      }

      const cards = (playlists || []).map(renderPlaylistCard);
      renderInto(dom.playlistList, cards);

      dom.playlistList.style.display = '';
      dom.playlistDetail.style.display = 'none';
    } catch (err) {
      showToast(err.message || '搜索歌单失败');
    }
  }

  async function doPlaylistRecommend() {
    const source = getSelectedSource();

    try {
      let playlists;
      if (source === 'all') {
        playlists = await fetchAllProviders((srcId) =>
          apiFetch(
            `/api/playlist/recommended?source=${encodeURIComponent(srcId)}`,
            {},
            true
          )
        );
      } else {
        playlists = await apiFetch(
          `/api/playlist/recommended?source=${encodeURIComponent(source)}`
        );
      }

      const cards = (playlists || []).map(renderPlaylistCard);
      renderInto(dom.playlistList, cards);

      dom.playlistList.style.display = '';
      dom.playlistDetail.style.display = 'none';
    } catch (err) {
      showToast(err.message || '获取推荐歌单失败');
    }
  }

  // ---------------------------------------------------------------------------
  // Playlist detail (songs)
  // ---------------------------------------------------------------------------

  async function handlePlaylistDetail(playlist) {
    const source = playlist.source || getSelectedSource();
    if (!source || source === 'all') {
      showToast('无法确定歌单来源');
      return;
    }

    try {
      const songs = await apiFetch(
        `/api/playlist/songs?source=${encodeURIComponent(source)}&id=${encodeURIComponent(playlist.id)}`
      );

      currentPlaylistSongs = songs || [];
      currentPlaylistName = playlist.name || '';
      currentPlaylistSource = source;

      const cards = (songs || []).map(renderSongCard);
      renderInto(dom.playlistSongs, cards);

      dom.playlistDetailTitle.textContent = playlist.name || '歌单详情';
      dom.batchActions.style.display = '';

      dom.playlistList.style.display = 'none';
      dom.playlistDetail.style.display = 'block';
    } catch (err) {
      showToast(err.message || '获取歌单歌曲失败');
    }
  }

  function handlePlaylistBack() {
    dom.playlistDetail.style.display = 'none';
    dom.playlistList.style.display = '';
  }

  // ---------------------------------------------------------------------------
  // Parse tab
  // ---------------------------------------------------------------------------

  async function doParse() {
    const link = dom.parseInput.value.trim();
    if (!link) {
      showToast('请输入链接');
      return;
    }

    const source = getSelectedSource();
    if (source === 'all') {
      showToast('解析功能需要选择具体的音乐源，不支持"全部"');
      return;
    }

    const parseType = (dom.parseTypeRadios.find((r) => r.checked) || {}).value || 'song';

    try {
      if (parseType === 'song') {
        const song = await apiFetch(
          `/api/parse?source=${encodeURIComponent(source)}&link=${encodeURIComponent(link)}`
        );
        dom.parseResult.innerHTML = '';
        if (song) {
          dom.parseResult.appendChild(renderSongCard(song));
        } else {
          dom.parseResult.appendChild(el('div', 'empty-state', '未解析到歌曲'));
        }
      } else {
        const data = await apiFetch(
          `/api/playlist/parse?source=${encodeURIComponent(source)}&link=${encodeURIComponent(link)}`
        );

        dom.parseResult.innerHTML = '';

        if (data && data.playlist) {
          const header = el('div', 'parse-playlist-header');
          header.appendChild(renderPlaylistCard(data.playlist));
          dom.parseResult.appendChild(header);
        }

        if (data && data.songs && data.songs.length > 0) {
          const songsContainer = el('div', 'parse-playlist-songs');
          data.songs.forEach((song) => {
            songsContainer.appendChild(renderSongCard(song));
          });
          dom.parseResult.appendChild(songsContainer);
        } else {
          dom.parseResult.appendChild(el('div', 'empty-state', '未解析到歌曲'));
        }
      }
    } catch (err) {
      showToast(err.message || '解析失败');
    }
  }

  // ---------------------------------------------------------------------------
  // NAS Status & Task Polling
  // ---------------------------------------------------------------------------

  async function checkNASStatus() {
    try {
      const data = await apiFetch('/api/nas/status', {}, true);
      nasEnabled = data && data.enabled;
      if (dom.nasStatusBadge) {
        if (nasEnabled) {
          dom.nasStatusBadge.textContent = `NAS已启用 (${data.music_dir})`;
          dom.nasStatusBadge.className = 'nas-status-badge enabled';
        } else {
          dom.nasStatusBadge.textContent = 'NAS未启用';
          dom.nasStatusBadge.className = 'nas-status-badge disabled';
        }
      }
      syncNASUI();
    } catch {
      nasEnabled = false;
      syncNASUI();
    }
  }

  function syncNASUI() {
    $$('.nas-only').forEach((el) => {
      el.style.display = nasEnabled ? '' : 'none';
    });
  }

  function startTaskPolling() {
    stopTaskPolling();
    refreshNASTasks();
    nasTaskPollingTimer = setInterval(refreshNASTasks, NAS_POLL_INTERVAL);
  }

  function stopTaskPolling() {
    if (nasTaskPollingTimer) {
      clearInterval(nasTaskPollingTimer);
      nasTaskPollingTimer = null;
    }
  }

  function updateNASBadge(tasks) {
    if (!dom.nasTaskBadge) return;
    const active = (tasks || []).filter(
      (t) => t.status === 'pending' || t.status === 'running'
    ).length;
    if (active > 0) {
      dom.nasTaskBadge.textContent = active > 99 ? '99+' : String(active);
      dom.nasTaskBadge.classList.add('show');
    } else {
      dom.nasTaskBadge.classList.remove('show');
      dom.nasTaskBadge.textContent = '';
      stopBadgePolling();
    }
  }

  function startBadgePolling() {
    if (nasBadgePollingTimer) return;
    refreshBadge();
    nasBadgePollingTimer = setInterval(refreshBadge, NAS_POLL_INTERVAL);
  }

  function stopBadgePolling() {
    if (nasBadgePollingTimer) {
      clearInterval(nasBadgePollingTimer);
      nasBadgePollingTimer = null;
    }
  }

  async function refreshBadge() {
    try {
      const tasks = await apiFetch('/api/nas/tasks', {}, true);
      updateNASBadge(tasks || []);
    } catch {
      // Silent fail.
    }
  }

  async function refreshNASTasks() {
    try {
      const [tasks, batches] = await Promise.all([
        apiFetch('/api/nas/tasks', {}, true),
        apiFetch('/api/nas/batches', {}, true),
      ]);
      renderBatchSummary(batches || []);
      renderNASTaskPanel(tasks || []);
      updateNASBadge(tasks || []);
    } catch {
      // Silent fail for polling.
    }
  }

  function renderNASTaskPanel(tasks) {
    const container = dom.nasTaskList;
    if (!container) return;
    container.innerHTML = '';

    if (tasks.length === 0) {
      container.appendChild(el('div', 'empty-state', '暂无下载任务'));
      return;
    }

    const reversed = [...tasks].reverse();
    const frag = document.createDocumentFragment();
    for (const task of reversed) {
      frag.appendChild(renderTaskCard(task));
    }
    container.appendChild(frag);
  }

  function renderTaskCard(task) {
    const card = el('div', 'task-card');

    const header = el('div', 'task-card-header');
    const songName = el('div', 'song-name', task.song ? (task.song.name || '未知') : '未知');
    header.appendChild(songName);

    const statusLabels = {
      pending: '等待中',
      running: '下载中',
      completed: '已完成',
      failed: '失败',
    };
    const isSkipped = task.status === 'completed' && task.skipped;
    const statusText = isSkipped ? '已存在' : (statusLabels[task.status] || task.status);
    const statusClass = isSkipped ? 'skipped' : task.status;
    const badge = el('span', `task-status ${statusClass}`, statusText);
    header.appendChild(badge);
    card.appendChild(header);

    const meta = el('div', 'song-meta');
    const parts = [];
    if (task.song && task.song.artist) parts.push(task.song.artist);
    if (task.source) parts.push(getSourceName(task.source));
    meta.textContent = parts.join(' \u00b7 ');

    if (task.fallback_source) {
      const fbTag = el('span', 'source-tag fallback-tag',
        getSourceName(task.source) + ' \u2192 ' + getSourceName(task.fallback_source));
      meta.appendChild(document.createTextNode(' \u00b7 '));
      meta.appendChild(fbTag);
    }
    card.appendChild(meta);

    if (task.status === 'running') {
      const track = el('div', 'progress-bar-track');
      const fill = el('div', 'progress-bar-fill');
      const pct = task.total_size > 0 ? Math.min(100, Math.round(task.progress / task.total_size * 100)) : 0;
      fill.style.width = pct + '%';
      track.appendChild(fill);
      card.appendChild(track);

      const progressText = el('div', 'progress-text');
      if (task.progress > 0) {
        progressText.textContent = formatSize(task.progress) + (task.total_size > 0 ? ` / ${formatSize(task.total_size)}` : '');
      } else {
        progressText.textContent = '下载中...';
      }
      card.appendChild(progressText);
    }

    if (task.status === 'failed' && task.error) {
      const errEl = el('div', 'task-error', task.error);
      card.appendChild(errEl);
    }

    if (task.status === 'completed' && task.file_path) {
      const pathEl = el('div', 'progress-text', task.file_path);
      card.appendChild(pathEl);
    }

    return card;
  }

  function renderBatchSummary(batches) {
    const container = dom.nasBatches;
    if (!container) return;
    container.innerHTML = '';

    if (batches.length === 0) return;

    const frag = document.createDocumentFragment();
    const reversed = [...batches].reverse();
    for (const batch of reversed) {
      const card = el('div', 'batch-card');

      const header = el('div', 'batch-card-header');
      header.appendChild(el('span', 'batch-name', batch.playlist_name || batch.id));
      const stats = `${batch.completed}/${batch.total} 完成`;
      const extra = [];
      if (batch.running > 0) extra.push(`${batch.running}下载中`);
      if (batch.failed > 0) extra.push(`${batch.failed}失败`);
      const statsText = stats + (extra.length ? ` (${extra.join(', ')})` : '');
      header.appendChild(el('span', 'batch-stats', statsText));
      card.appendChild(header);

      const track = el('div', 'progress-bar-track');
      const fill = el('div', 'progress-bar-fill');
      const pct = batch.total > 0 ? Math.round((batch.completed + batch.failed) / batch.total * 100) : 0;
      fill.style.width = pct + '%';
      if (batch.failed > 0 && batch.completed === 0) {
        fill.style.backgroundColor = '#f85149';
      } else if (batch.completed === batch.total) {
        fill.style.backgroundColor = '#3fb950';
      }
      track.appendChild(fill);
      card.appendChild(track);

      frag.appendChild(card);
    }
    container.appendChild(frag);
  }

  // ---------------------------------------------------------------------------
  // Dual-Platform QR Login
  // ---------------------------------------------------------------------------

  async function checkLoginStatus() {
    const platforms = ['netease', 'qq'];
    for (const platform of platforms) {
      try {
        const data = await apiFetch(`/api/login/status?platform=${platform}`, {}, true);
        loginState[platform].loggedIn = data && data.logged_in;
        loginState[platform].nickname = (data && data.nickname) || '';
      } catch {
        loginState[platform].loggedIn = false;
        loginState[platform].nickname = '';
      }
    }
    updateLoginButtons();
  }

  function updateLoginButtons() {
    const buttons = $$('.btn-login[data-platform]');
    buttons.forEach((btn) => {
      const platform = btn.dataset.platform;
      const textEl = btn.querySelector('.login-text');
      if (!textEl) return;

      const state = loginState[platform];
      if (state && state.loggedIn) {
        textEl.textContent = state.nickname || '已登录';
        btn.classList.add('logged-in');
        btn.title = `点击管理${PLATFORM_NAMES[platform] || platform}登录`;
      } else {
        textEl.textContent = PLATFORM_NAMES[platform] ? PLATFORM_NAMES[platform].replace('音乐', '') + '登录' : platform;
        btn.classList.remove('logged-in');
        btn.title = `${PLATFORM_NAMES[platform] || platform}登录`;
      }
    });
  }

  function openQRModal(platform) {
    currentQRPlatform = platform;
    dom.qrModalTitle.textContent = `${PLATFORM_NAMES[platform] || platform} 扫码登录`;
    dom.qrModal.classList.add('show');

    if (loginState[platform] && loginState[platform].loggedIn) {
      showLoggedInState(platform);
    } else {
      startQRLogin(platform);
    }
  }

  function closeQRModal() {
    dom.qrModal.classList.remove('show');
    stopQRPolling();
    currentQRPlatform = null;
  }

  function showLoggedInState(platform) {
    dom.qrImage.innerHTML = '';
    dom.qrImage.style.display = 'none';
    dom.qrStatus.style.display = 'none';
    dom.qrLoggedIn.style.display = '';
    dom.qrNickname.textContent = loginState[platform].nickname;
  }

  async function startQRLogin(platform) {
    dom.qrLoggedIn.style.display = 'none';
    dom.qrImage.style.display = '';
    dom.qrImage.innerHTML = '<div class="spinner"></div>';
    dom.qrStatus.style.display = '';
    dom.qrStatus.textContent = '正在启动...';
    dom.qrStatus.className = 'qr-status';

    try {
      await apiFetch(`/api/login/qr/start?platform=${platform}`, { method: 'POST' }, true);
      startQRPolling(platform);
    } catch (err) {
      dom.qrStatus.textContent = err.message || '启动登录失败';
      dom.qrStatus.className = 'qr-status expired';
    }
  }

  function startQRPolling(platform) {
    stopQRPolling();
    qrPollingTimer = setInterval(async () => {
      try {
        const data = await apiFetch(
          `/api/login/qr/poll?platform=${encodeURIComponent(platform)}`,
          {},
          true
        );
        if (!data) return;

        switch (data.state) {
          case 'starting':
            dom.qrStatus.textContent = '正在启动...';
            dom.qrStatus.className = 'qr-status';
            break;

          case 'waiting_scan':
            if (data.qr_image) {
              dom.qrImage.innerHTML = '';
              const img = document.createElement('img');
              img.src = 'data:image/png;base64,' + data.qr_image;
              img.alt = 'QR Code';
              img.style.maxWidth = '200px';
              img.style.maxHeight = '200px';
              dom.qrImage.appendChild(img);
            }
            dom.qrStatus.textContent = `请使用${PLATFORM_NAMES[platform] || platform}App扫码`;
            dom.qrStatus.className = 'qr-status';
            break;

          case 'scanned':
            dom.qrStatus.textContent = '已扫码，请在手机上确认';
            dom.qrStatus.className = 'qr-status scanned';
            break;

          case 'success':
            dom.qrStatus.textContent = '登录成功！';
            dom.qrStatus.className = 'qr-status success';
            stopQRPolling();
            loginState[platform].loggedIn = true;
            loginState[platform].nickname = data.nickname || '';
            updateLoginButtons();
            setTimeout(() => closeQRModal(), 1200);
            break;

          case 'expired':
            dom.qrStatus.textContent = '二维码已过期，正在刷新...';
            dom.qrStatus.className = 'qr-status expired';
            // Script auto-refreshes; next poll with qr_ready will update the image
            break;

          case 'error':
            dom.qrStatus.textContent = data.error || '登录出错';
            dom.qrStatus.className = 'qr-status expired';
            stopQRPolling();
            break;
        }
      } catch {
        // Silent fail for polling
      }
    }, QR_POLL_INTERVAL);
  }

  function stopQRPolling() {
    if (qrPollingTimer) {
      clearInterval(qrPollingTimer);
      qrPollingTimer = null;
    }
  }

  async function handleLogout(platform) {
    if (!platform) platform = currentQRPlatform;
    if (!platform) return;

    try {
      await apiFetch(`/api/login/logout?platform=${platform}`, { method: 'POST' });
      loginState[platform].loggedIn = false;
      loginState[platform].nickname = '';
      updateLoginButtons();
      closeQRModal();
      showToast(`已退出${PLATFORM_NAMES[platform] || platform}登录`);
    } catch (err) {
      showToast(err.message || '退出登录失败');
    }
  }

  // ---------------------------------------------------------------------------
  // Keyboard shortcuts
  // ---------------------------------------------------------------------------

  function initKeyboard() {
    dom.searchInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        doSongSearch();
      }
    });

    dom.playlistKeyword.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        doPlaylistSearch();
      }
    });

    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') {
        closeLyricsModal();
        closeQRModal();
      }
    });
  }

  // ---------------------------------------------------------------------------
  // Modal events
  // ---------------------------------------------------------------------------

  function initModal() {
    $$('.modal-close', dom.lyricsModal).forEach((btn) => {
      btn.addEventListener('click', closeLyricsModal);
    });

    dom.lyricsModal.addEventListener('click', (e) => {
      if (e.target === dom.lyricsModal) {
        closeLyricsModal();
      }
    });
  }

  // ---------------------------------------------------------------------------
  // Initialization
  // ---------------------------------------------------------------------------

  function init() {
    dom = {
      qualitySelect:      $('#quality-select'),
      providerSelect:     $('#provider-select'),
      tabSearch:          $('#tab-search'),
      tabPlaylist:        $('#tab-playlist'),
      tabParse:           $('#tab-parse'),
      tabNasTasks:        $('#tab-nas-tasks'),
      searchInput:        $('#search-input'),
      searchBtn:          $('#search-btn'),
      searchResults:      $('#search-results'),
      playlistKeyword:    $('#playlist-keyword'),
      playlistSearchBtn:  $('#playlist-search-btn'),
      playlistRecommendBtn: $('#playlist-recommend-btn'),
      playlistList:       $('#playlist-list'),
      playlistDetail:     $('#playlist-detail'),
      playlistSongs:      $('#playlist-songs'),
      playlistDetailTitle: $('#playlist-detail-title'),
      playlistBackBtn:    $('#playlist-back-btn'),
      batchActions:       $('#playlist-batch-actions'),
      batchDownloadBtn:   $('#playlist-batch-download-btn'),
      batchDownloadMenu:  $('#batch-download-menu'),
      parseInput:         $('#parse-input'),
      parseBtn:           $('#parse-btn'),
      parseResult:        $('#parse-result'),
      parseTypeRadios:    $$('input[name=parse-type]'),
      lyricsModal:        $('#lyrics-modal'),
      lyricsTitle:        $('#lyrics-title'),
      lyricsText:         $('#lyrics-text'),
      toast:              $('#toast'),
      loading:            $('#loading'),
      nasStatusBadge:     $('#nas-status-badge'),
      nasBatches:         $('#nas-batches'),
      nasTaskList:        $('#nas-task-list'),
      nasTaskBadge:       $('#nas-task-badge'),
      qrModal:            $('#qr-modal'),
      qrModalTitle:       $('#qr-modal-title'),
      qrModalClose:       $('#qr-modal-close'),
      qrImage:            $('#qr-image'),
      qrStatus:           $('#qr-status'),
      qrLoggedIn:         $('#qr-logged-in'),
      qrNickname:         $('#qr-nickname'),
      logoutBtn:          $('#logout-btn'),
    };

    // --- Quality setting (localStorage persistence) ---
    const savedQuality = localStorage.getItem('quality');
    if (savedQuality && dom.qualitySelect) {
      dom.qualitySelect.value = savedQuality;
    }
    if (dom.qualitySelect) {
      dom.qualitySelect.addEventListener('change', () => {
        localStorage.setItem('quality', dom.qualitySelect.value);
      });
    }

    // --- Tabs ---
    initTabs();

    // --- Song search ---
    dom.searchBtn.addEventListener('click', doSongSearch);

    // --- Playlist ---
    dom.playlistSearchBtn.addEventListener('click', doPlaylistSearch);
    dom.playlistRecommendBtn.addEventListener('click', doPlaylistRecommend);
    dom.playlistBackBtn.addEventListener('click', handlePlaylistBack);

    // --- Batch download dropdown ---
    if (dom.batchDownloadBtn) {
      dom.batchDownloadBtn.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        closeAllMenus();
        dom.batchDownloadMenu.classList.toggle('show');
      });
    }
    if (dom.batchDownloadMenu) {
      $$('.download-menu-item', dom.batchDownloadMenu).forEach((item) => {
        item.addEventListener('click', (e) => {
          e.preventDefault();
          e.stopPropagation();
          dom.batchDownloadMenu.classList.remove('show');
          const action = item.dataset.action;
          if (action === 'nas') {
            handleNASBatchDownload();
          } else {
            handleBrowserBatchDownload();
          }
        });
      });
    }

    // --- Parse ---
    dom.parseBtn.addEventListener('click', doParse);

    // --- Modal ---
    initModal();

    // --- Keyboard ---
    initKeyboard();

    // --- Close dropdown menus on outside click ---
    document.addEventListener('click', () => closeAllMenus());

    // --- NAS status ---
    checkNASStatus();

    // --- Dual-platform Login ---
    $$('.btn-login[data-platform]').forEach((btn) => {
      btn.addEventListener('click', () => {
        const platform = btn.dataset.platform;
        openQRModal(platform);
      });
    });

    if (dom.qrModalClose) {
      dom.qrModalClose.addEventListener('click', closeQRModal);
    }
    if (dom.qrModal) {
      dom.qrModal.addEventListener('click', (e) => {
        if (e.target === dom.qrModal) closeQRModal();
      });
    }
    if (dom.logoutBtn) {
      dom.logoutBtn.addEventListener('click', () => handleLogout(currentQRPlatform));
    }

    checkLoginStatus();
  }

  // Boot
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
