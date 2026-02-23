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

  // ---------------------------------------------------------------------------
  // DOM references (cached once on load)
  // ---------------------------------------------------------------------------

  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

  let dom; // populated in init()

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

  // ---------------------------------------------------------------------------
  // API helpers
  // ---------------------------------------------------------------------------

  /**
   * Generic fetch wrapper.
   * - Automatically shows / hides the loading spinner.
   * - Parses JSON and checks the `code` field.
   * - Returns the `data` payload on success.
   * - Throws with a user-friendly message on failure.
   *
   * @param {string}        url
   * @param {RequestInit=}  options
   * @param {boolean=}      silent  If true, don't toggle loading indicator (used for parallel calls).
   * @returns {Promise<any>}
   */
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
      // Network errors or JSON parse failures
      if (err instanceof SyntaxError) {
        throw new Error('服务器返回了无效的数据');
      }
      throw err;
    } finally {
      if (!silent) hideLoading();
    }
  }

  /**
   * When the selected source is "all", fan-out requests to every provider
   * in parallel and merge the results into a single array.
   *
   * @param {(sourceId: string) => Promise<any[]>} fetcher  Receives a single source id, returns an array.
   * @returns {Promise<any[]>}  Merged results from all providers that succeeded.
   */
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

  /** Create an element with optional className and text */
  function el(tag, className, text) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  /**
   * Build a song card DOM element.
   *
   * Layout:
   *   div.song-card
   *     div.song-info
   *       div.song-name   — song name
   *       div.song-meta   — artist / album / source / duration / size
   *     div.song-actions
   *       button.btn-download  下载链接
   *       button.btn-lyrics    歌词
   */
  function renderSongCard(song) {
    const card = el('div', 'song-card');

    // -- info section --
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
      // The source name gets a special tag span
      if (part === getSourceName(song.source)) {
        const tag = el('span', 'source-tag', part);
        meta.appendChild(tag);
      } else {
        meta.appendChild(document.createTextNode(part));
      }
      if (idx < metaParts.length - 1) {
        meta.appendChild(document.createTextNode(' \u00b7 '));  // middle dot separator
      }
    });

    info.appendChild(meta);
    card.appendChild(info);

    // -- actions section --
    const actions = el('div', 'song-actions');

    const btnDownload = el('button', 'btn-download', '下载链接');
    btnDownload.addEventListener('click', () => handleDownload(song));
    actions.appendChild(btnDownload);

    const btnLyrics = el('button', 'btn-lyrics', '歌词');
    btnLyrics.addEventListener('click', () => handleLyrics(song));
    actions.appendChild(btnLyrics);

    card.appendChild(actions);
    return card;
  }

  /**
   * Build a playlist card DOM element.
   *
   * Layout:
   *   div.playlist-card
   *     div.playlist-cover > img   (or fallback icon)
   *     div.playlist-info
   *       div.playlist-name
   *       div.playlist-meta   — creator / track count
   */
  function renderPlaylistCard(playlist) {
    const card = el('div', 'playlist-card');
    card.addEventListener('click', () => handlePlaylistDetail(playlist));

    // -- cover --
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

    // -- info --
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

  /** SVG music note icon used as cover fallback */
  function createMusicNoteIcon() {
    const wrapper = el('div', 'music-note-icon');
    wrapper.innerHTML =
      '<svg viewBox="0 0 24 24" width="48" height="48" fill="currentColor">' +
      '<path d="M12 3v10.55A4 4 0 1 0 14 17V7h4V3h-6z"/>' +
      '</svg>';
    return wrapper;
  }

  /** Empty the container and append an array of child elements */
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
    const tabPanels = [dom.tabSearch, dom.tabPlaylist, dom.tabParse];

    tabBtns.forEach((btn) => {
      btn.addEventListener('click', () => {
        // Activate the clicked button
        tabBtns.forEach((b) => b.classList.remove('active'));
        btn.classList.add('active');

        // Show the matching panel
        const targetId = `tab-${btn.dataset.tab}`;
        tabPanels.forEach((panel) => {
          panel.classList.toggle('active', panel.id === targetId);
        });
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
  // Download
  // ---------------------------------------------------------------------------

  async function handleDownload(song) {
    const source = song.source || getSelectedSource();
    if (!source || source === 'all') {
      showToast('无法确定歌曲来源');
      return;
    }

    try {
      const data = await apiFetch(
        `/api/download?source=${encodeURIComponent(source)}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(song),
        }
      );

      const url = data && data.url;
      if (!url) {
        showToast('未获取到下载链接');
        return;
      }

      // Open in new tab
      window.open(url, '_blank');

      // Copy to clipboard
      try {
        await navigator.clipboard.writeText(url);
        showToast('已复制下载链接');
      } catch {
        // Clipboard API may be blocked; fall back to legacy method
        const ta = document.createElement('textarea');
        ta.value = url;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
        showToast('已复制下载链接');
      }
    } catch (err) {
      showToast(err.message || '获取下载链接失败');
    }
  }

  // ---------------------------------------------------------------------------
  // Lyrics
  // ---------------------------------------------------------------------------

  async function handleLyrics(song) {
    const source = song.source || getSelectedSource();
    if (!source || source === 'all') {
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

      // Ensure list view is visible
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

      const cards = (songs || []).map(renderSongCard);
      renderInto(dom.playlistSongs, cards);

      // Update detail title
      dom.playlistDetailTitle.textContent = playlist.name || '歌单详情';

      // Switch to detail view
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
        // playlist parse
        const data = await apiFetch(
          `/api/playlist/parse?source=${encodeURIComponent(source)}&link=${encodeURIComponent(link)}`
        );

        dom.parseResult.innerHTML = '';

        if (data && data.playlist) {
          // Render playlist info header
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
      }
    });
  }

  // ---------------------------------------------------------------------------
  // Modal events
  // ---------------------------------------------------------------------------

  function initModal() {
    // Close button(s) inside the modal
    $$('.modal-close', dom.lyricsModal).forEach((btn) => {
      btn.addEventListener('click', closeLyricsModal);
    });

    // Click on the overlay background (the modal element itself, not its children)
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
    // Cache all DOM references
    dom = {
      providerSelect:     $('#provider-select'),
      tabSearch:          $('#tab-search'),
      tabPlaylist:        $('#tab-playlist'),
      tabParse:           $('#tab-parse'),
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
      parseInput:         $('#parse-input'),
      parseBtn:           $('#parse-btn'),
      parseResult:        $('#parse-result'),
      parseTypeRadios:    $$('input[name=parse-type]'),
      lyricsModal:        $('#lyrics-modal'),
      lyricsTitle:        $('#lyrics-title'),
      lyricsText:         $('#lyrics-text'),
      toast:              $('#toast'),
      loading:            $('#loading'),
    };

    // --- Tabs ---
    initTabs();

    // --- Song search ---
    dom.searchBtn.addEventListener('click', doSongSearch);

    // --- Playlist ---
    dom.playlistSearchBtn.addEventListener('click', doPlaylistSearch);
    dom.playlistRecommendBtn.addEventListener('click', doPlaylistRecommend);
    dom.playlistBackBtn.addEventListener('click', handlePlaylistBack);

    // --- Parse ---
    dom.parseBtn.addEventListener('click', doParse);

    // --- Modal ---
    initModal();

    // --- Keyboard ---
    initKeyboard();
  }

  // Boot
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
