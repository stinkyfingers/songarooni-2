const POLL_MS = 750;
const DEFAULT_SONG_ID = 0;
const MAX_TITLE_LENGTH = 20; // truncate long titles in the "Now" display

let songs = [];
let fields = { extra: [], displayed: [] };
let sortBy = ''; // '' = catalog order, 'title', 'tempo', or an extra field name
let lastScrolledSelectedId = null; // only auto-scroll when the selection actually changes, not on every poll
let state = {
  hasSelection: false, selectedSongId: 0, selectedTitle: '', selectedTempo: 0,
  playingSongId: 0, playingTitle: '', playingTempo: 0,
  running: false, muted: false, logoActive: false,
};

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
  defaultBtn: document.getElementById('default-btn'),
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
    ...fields.extra.map((name) => ({ value: name, label: name.toUpperCase() })),
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
    .filter((s) => !query || s.title.toLowerCase().includes(query))
    .filter((s) => s.id !== DEFAULT_SONG_ID) // hide Default from song list
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
    // real songs, whose tempo is fixed from songs.csv.
    title.textContent =
      song.id === DEFAULT_SONG_ID ?
        song.title :
        `${song.title} — ${song.tempo} BPM`.trim();

    info.textContent = song.id === DEFAULT_SONG_ID ?
      '' :
      `${song?.extra?.artist} - ${song?.extra?.genre}`;
    li.appendChild(title);
    li.appendChild(info);

    const shown = fields.displayed
      .map((name) => song.extra && song.extra[name])
      .filter(Boolean);
    if (shown.length) {
      const meta = document.createElement('div');
      meta.className = 'song-meta';
      meta.textContent = shown.join(' · ');
      li.appendChild(meta);
    }

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

  el.upNext.textContent =
    state.running && state.selectedSongId !== state.playingSongId
      ? `Up next: ${state.selectedTitle} — ${state.selectedTempo} BPM`
      : '';

  const defaultSelected = state.hasSelection && state.selectedSongId === DEFAULT_SONG_ID;
  el.defaultTempoBox.classList.toggle('visible', defaultSelected);
  if (defaultSelected && document.activeElement !== el.tempoInput) {
    el.tempoInput.value = state.selectedTempo;
  }

  el.muteBtn.classList.toggle('active', state.muted);
  el.logoBtn.classList.toggle('active', state.logoActive);
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
  console.log(`Key: ${e.key} | Code: ${e.code}`)
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
    [songs, fields, state] = await Promise.all([
      api('GET', '/api/songs'),
      api('GET', '/api/fields'),
      api('GET', '/api/state'),
    ]);
    // Defensive: an API field that should always be an array must not be
    // allowed to take the whole page down if it's ever null instead.
    fields.extra = fields.extra || [];
    fields.displayed = fields.displayed || [];
    populateSortOptions();
    renderList();
    renderState();
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
