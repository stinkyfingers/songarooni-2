const POLL_MS = 750;
const DEFAULT_SONG_ID = 0;
const MAX_TITLE_LENGTH = 20; // truncate long titles in the "Now" display

let songs = [];
let sortBy = ''; // '' = catalog order, 'title', 'tempo', or an extra field name
let lastScrolledSelectedId = null; // only auto-scroll when the selection actually changes, not on every poll
let state = {
  hasSelection: false, selectedSongId: 0, selectedTitle: '', selectedTempo: 0,
  playingSongId: 0, playingTitle: '', playingTempo: 0,
  running: false, muted: false, logoActive: false, liveActive: false,
};
let configuration = {};

const el = {
  search: document.getElementById('search'),
  sortBy: document.getElementById('sort-by'),
  list: document.getElementById('song-list'),
  defaultTempoBox: document.getElementById('default-tempo'),
  tempoInput: document.getElementById('tempo-input'),
  tempoSetBtn: document.getElementById('tempo-set-btn'),
  nowTitle: document.getElementById('now-title'),
  nowTempo: document.getElementById('now-tempo'),
  upNext: document.getElementById('up-next'),
  startBtn: document.getElementById('start-btn'),
  stopBtn: document.getElementById('stop-btn'),
  muteBtn: document.getElementById('mute-btn'),
  logoBtn: document.getElementById('logo-btn'),
  liveBtn: document.getElementById('live-btn'),
  defaultBtn: document.getElementById('default-btn'),
  candidBtn: document.getElementById('candid-btn'),
  candidInput: document.getElementById('candid-input'),
  cameraIpInput: document.getElementById('camera-ip-input'),
  cameraPortInput: document.getElementById('camera-port-input'),
  cameraSaveBtn: document.getElementById('camera-save-btn'),
  cameraFindBtn: document.getElementById('camera-find-btn'),
  error: document.getElementById('error'),
};

async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `${method} ${path} failed (${res.status})`);
  return data;
}

function showError(message) {
  el.error.textContent = message || '';
}

function clearSearch() {
  el.search.value = '';
  renderList();
}

function populateSortOptions() {
  const options = [
    { value: 'title', label: 'TITLE' },
    { value: 'tempo', label: 'TEMPO' },
  ];
  el.sortBy.innerHTML = '';
  for (const opt of options) {
    const option = document.createElement('option');
    option.value = opt.value;
    option.textContent = opt.label;
    el.sortBy.appendChild(option);
  }
}

function sortedSongs() {
  const query = el.search.value.trim().toLowerCase();
  let list = songs
    .filter((s) => !query || s.title.toLowerCase().includes(query) || s.artist?.toLowerCase().includes(query))
    .sort((a, b) => a.title.localeCompare(b.title)); // always sort by ID first, so that the catalog order is stable

  switch (sortBy) {
    case 'title':
      return list;
    case 'tempo':
      return [...list].sort((a, b) => a.tempo - b.tempo);
    default:
      return [...list].sort((a, b) =>
        ((a.extra && a.extra[sortBy]) || '').localeCompare((b.extra && b.extra[sortBy]) || ''));
  }
}

function renderList() {
  el.list.innerHTML = '';
  let i = 0;
  for (const song of sortedSongs()) {
    const li = document.createElement('li');
    li.tabIndex = i; // make the whole row focusable for keyboard navigation
    i++;

    const title = document.createElement('div');
    const info = document.createElement('div');
    title.className = 'song-title';
    info.className = 'song-info';
    // Default's tempo is user-adjustable (see the tempo box below), so a
    // static number in the list would just go stale — only show BPM for
    // real songs, whose tempo is fixed from songs.json.
    title.textContent =
      ((song.id === DEFAULT_SONG_ID) || configuration.no_click) ?
        song.title :
        `${song.title} — ${song.tempo} BPM`.trim();

    info.textContent = song.id === DEFAULT_SONG_ID || (!song.artist && !song.genre) ?
      '' :
      `${song?.artist} - ${song?.genre}${song.has_images ? '' : ' (no images)'}`.trim();
    li.appendChild(title);
    li.appendChild(info);

    li.dataset.id = song.id;
    li.classList.toggle('selected', song.id === state.selectedSongId);
    li.classList.toggle('playing', song.id === state.playingSongId);
    li.addEventListener('click', () => selectSong(song.id));
    el.list.appendChild(li);
  }
}

function renderState() {
  if (state.running) {
    const playingTitle = state.playingTitle.length > MAX_TITLE_LENGTH ? state.playingTitle.slice(0, MAX_TITLE_LENGTH - 3) + '...' : state.playingTitle;
    el.nowTitle.textContent = `Playing: ${playingTitle}`;
    el.nowTempo.textContent = `${state.playingTempo} BPM`;
  } else if (state.hasSelection) {
    const selectedTitle = state.selectedTitle.length > MAX_TITLE_LENGTH ? state.selectedTitle.slice(0, MAX_TITLE_LENGTH - 3) + '...' : state.selectedTitle;
    el.nowTitle.textContent = `Selected: ${selectedTitle}`;
    el.nowTempo.textContent = `${state.selectedTempo} BPM`;
  } else {
    el.nowTitle.textContent = 'No song selected';
    el.nowTempo.textContent = '';
  }
  if (state.no_click || !state.playingTempo) {
    el.nowTempo.style.display = 'none';
  } else {
    el.nowTempo.style.display = '';
  }

  el.upNext.textContent =
    state.running && state.selectedSongId !== state.playingSongId
      ? `Up next: ${state.selectedTitle}`
      : '';

  const defaultSelected = state.hasSelection && state.selectedSongId === DEFAULT_SONG_ID;
  el.defaultTempoBox.classList.toggle('visible', defaultSelected);
  if (defaultSelected && document.activeElement !== el.tempoInput) {
    el.tempoInput.value = state.selectedTempo;
  }

  el.muteBtn.classList.toggle('active', state.muted);
  el.logoBtn.classList.toggle('active', state.logoActive);
  el.liveBtn.classList.toggle('active', state.liveActive);
  el.startBtn.disabled = !state.hasSelection || (state.running && state.selectedSongId === state.playingSongId);
  el.stopBtn.disabled = !state.running;
  el.defaultBtn.classList.toggle('active', state.hasSelection && state.selectedSongId === DEFAULT_SONG_ID);
  showError(state.error);

  for (const li of el.list.children) {
    const id = Number(li.dataset.id);
    li.classList.toggle('selected', id === state.selectedSongId);
    li.classList.toggle('playing', id === state.playingSongId);
  }

  // Keep the selected row on-screen as the pedal's Page Up/Down moves it —
  // only when the selection actually changed, so this doesn't fight a user
  // manually scrolling the list on every 750ms poll tick.
  if (state.hasSelection && state.selectedSongId !== lastScrolledSelectedId) {
    el.list.querySelector(`[data-id="${state.selectedSongId}"]`)
      ?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
  }
  lastScrolledSelectedId = state.selectedSongId;
}

function adjustConfigurationUI(config) {
  if (config.no_click) {
    el.muteBtn.style.display = 'none';
    el.stopBtn.style.display = 'none';
    el.defaultTempoBox.style.display = 'none';
  } else {
    el.muteBtn.style.display = '';
    el.stopBtn.style.display = '';
    el.defaultTempoBox.style.display = '';
  }
}

async function selectSong(id) {
  try {
    state = await api('POST', '/api/select', { id });
    renderState();
    clearSearch();
  } catch (err) {
    showError(err.message);
  }
}

async function refreshState() {
  try {
    state = await api('GET', '/api/state');
    renderState();
  } catch (err) {
    showError(err.message);
  }
}

el.search.addEventListener('input', renderList);
el.sortBy.addEventListener('change', () => {
  sortBy = el.sortBy.value;
  renderList();
});

el.startBtn.addEventListener('click', async () => {
  try {
    state = await api('POST', '/api/start');
    renderState();
  } catch (err) {
    showError(err.message);
  }
});

el.stopBtn.addEventListener('click', async () => {
  try {
    state = await api('POST', '/api/stop');
    renderState();
  } catch (err) {
    showError(err.message);
  }
});

el.muteBtn.addEventListener('click', async () => {
  try {
    state = await api('POST', '/api/mute', { muted: !state.muted });
    renderState();
  } catch (err) {
    showError(err.message);
  }
});

el.logoBtn.addEventListener('click', async () => {
  try {
    state = await api('POST', '/api/logo');
    renderState();
  } catch (err) {
    showError(err.message);
  }
});

el.liveBtn.addEventListener('click', async () => {
  try {
    state = await api('POST', '/api/live');
    renderState();
  } catch (err) {
    showError(err.message);
  }
});

el.cameraSaveBtn.addEventListener('click', async () => {
  const ip = el.cameraIpInput.value.trim();
  const port = parseInt(el.cameraPortInput.value, 10);
  if (!port || port <= 0) {
    showError('Enter a port greater than 0');
    return;
  }
  try {
    await api('POST', '/api/camera-config', { ip, port });
    showError('');
  } catch (err) {
    showError(err.message);
  }
});

el.cameraFindBtn.addEventListener('click', async () => {
  showError('Scanning the network for the camera…');
  try {
    const { found } = await api('POST', '/api/camera/discover');
    if (found.length === 0) {
      showError('No camera found on the local network');
    } else {
      el.cameraIpInput.value = found[0];
      showError(found.length > 1 ? `Found ${found.length} cameras — using ${found[0]}` : '');
    }
  } catch (err) {
    showError(err.message);
  }
});

el.defaultBtn.addEventListener('click', async () => {
  try {
    state = await api('POST', '/api/select', { id: DEFAULT_SONG_ID });
    renderState();
  } catch (err) {
    showError(err.message);
  }
  try {
    state = await api('POST', '/api/start');
    renderState();
  } catch (err) {
    showError(err.message);
  }
});

// CANDID_MAX_DIMENSION/CANDID_JPEG_QUALITY: the venue display (a Chromium
// kiosk browser) can't render HEIC in an <img> tag at all, so every
// upload gets re-encoded to JPEG client-side first — Safari, running on
// whatever phone is actually doing the upload, can decode HEIC into a
// canvas even though Chromium can't display it directly. The same step
// also caps resolution: there's no reason to ship a full-resolution
// phone photo when nothing downstream can show more than a fraction of
// that detail on a TV-sized screen.
const CANDID_MAX_DIMENSION = 1920;
const CANDID_JPEG_QUALITY = 0.85;

function resizeToJPEG(file) {
  return new Promise((resolve, reject) => {
    const img = new Image();
    const url = URL.createObjectURL(file);
    img.onload = () => {
      URL.revokeObjectURL(url);
      let { width, height } = img;
      const longEdge = Math.max(width, height);
      if (longEdge > CANDID_MAX_DIMENSION) {
        const scale = CANDID_MAX_DIMENSION / longEdge;
        width = Math.round(width * scale);
        height = Math.round(height * scale);
      }
      const canvas = document.createElement('canvas');
      canvas.width = width;
      canvas.height = height;
      canvas.getContext('2d').drawImage(img, 0, 0, width, height);
      canvas.toBlob((blob) => {
        if (blob) resolve(blob);
        else reject(new Error(`could not process ${file.name}`));
      }, 'image/jpeg', CANDID_JPEG_QUALITY);
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error(`could not load ${file.name}`));
    };
    img.src = url;
  });
}

el.candidBtn.addEventListener('click', () => el.candidInput.click());

el.candidInput.addEventListener('change', async () => {
  const files = Array.from(el.candidInput.files || []);
  el.candidInput.value = ''; // allow re-selecting the same file(s) next time
  if (files.length === 0) return;

  try {
    const jpegs = await Promise.all(files.map(resizeToJPEG));
    const formData = new FormData();
    jpegs.forEach((blob, i) => formData.append('photos', blob, `candid-${Date.now()}-${i}.jpg`));

    // Not using the shared api() helper — it always JSON-encodes the
    // body, but a file upload needs a multipart FormData body instead
    // (the browser sets the boundary itself; no Content-Type override).
    const res = await fetch('/api/candid/upload', { method: 'POST', body: formData });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || `upload failed (${res.status})`);
    showError(data.saved ? '' : 'No photos were saved');
  } catch (err) {
    showError(err.message);
  }
});

el.tempoSetBtn.addEventListener('click', async () => {
  const bpm = parseInt(el.tempoInput.value, 10);
  if (!bpm || bpm <= 0) {
    showError('Enter a tempo greater than 0');
    return;
  }
  try {
    state = await api('POST', '/api/tempo', { bpm });
    renderList(); // the Default row's displayed tempo changed too
    renderState();
  } catch (err) {
    showError(err.message);
  }
});

document.addEventListener('keydown', (e) => {
  if (e.code === 'PageDown' || e.code === 'ArrowDown') {
    e.preventDefault();
    const selected = el.list.querySelector('.selected');
    const next = selected?.nextElementSibling || el.list.firstElementChild;
    next?.click();
    next?.focus();
  }
  if (e.code === 'PageUp' || e.code === 'ArrowUp') {
    e.preventDefault();
    const selected = el.list.querySelector('.selected');
    const prev = selected?.previousElementSibling || el.list.lastElementChild;
    prev?.click();
    prev?.focus();
  }
  if (e.code === 'Space') {
    e.preventDefault();
    el.defaultBtn.click();
  }
  if (e.code === 'Enter') {
    e.preventDefault();
    el.startBtn.click();
  }
});

(async function init() {
  try {
    let cameraConfig;
    [songs, state, configuration, cameraConfig] = await Promise.all([
      api('GET', '/api/songs'),
      api('GET', '/api/state'),
      api('GET', '/api/config'),
      api('GET', '/api/camera-config'),
    ]);
    populateSortOptions();
    renderList();
    renderState();
    adjustConfigurationUI(configuration);
    el.cameraIpInput.value = cameraConfig.ip || '';
    el.cameraPortInput.value = cameraConfig.port || '';
    // Some iOS/WebKit builds don't reliably deliver hardware-keyboard
    // keydown events to the page at all unless *something* is focused —
    // desktop browsers fire the document-level listener below regardless.
    // Focusing the first row on load, rather than waiting for a keypress to
    // focus something reactively, is a minimal test of that theory without
    // changing how navigation itself works.
    el.list.firstElementChild?.focus();
  } catch (err) {
    showError(err.message);
  }
  setInterval(refreshState, POLL_MS);
})();
