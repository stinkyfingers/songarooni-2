const POLL_MS = 750;
const DEFAULT_SONG_ID = 0;

let songs = [];
let fields = { extra: [], displayed: [] };
let sortBy = ''; // '' = catalog order, 'title', 'tempo', or an extra field name
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
  indicator: document.getElementById('click-indicator'),
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

function populateSortOptions() {
  const options = [
    { value: '', label: 'Setlist order' },
    { value: 'title', label: 'Title' },
    { value: 'tempo', label: 'Tempo' },
    ...fields.extra.map((name) => ({ value: name, label: name })),
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
  let list = songs.filter((s) => !query || s.title.toLowerCase().includes(query));

  if (sortBy === 'title') {
    list = [...list].sort((a, b) => a.title.localeCompare(b.title));
  } else if (sortBy === 'tempo') {
    list = [...list].sort((a, b) => a.tempo - b.tempo);
  } else if (sortBy) {
    list = [...list].sort((a, b) =>
      ((a.extra && a.extra[sortBy]) || '').localeCompare((b.extra && b.extra[sortBy]) || ''));
  }
  return list;
}

function renderList() {
  el.list.innerHTML = '';
  for (const song of sortedSongs()) {
    const li = document.createElement('li');

    const title = document.createElement('div');
    title.className = 'song-title';
    // Default's tempo is user-adjustable (see the tempo box below), so a
    // static number in the list would just go stale — only show BPM for
    // real songs, whose tempo is fixed from songs.csv.
    title.textContent =
      song.id === DEFAULT_SONG_ID ? song.title : `${song.title} — ${song.tempo} BPM`;
    li.appendChild(title);

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
    el.nowTitle.textContent = `Now playing: ${state.playingTitle}`;
    el.nowTempo.textContent = `${state.playingTempo} BPM`;
  } else if (state.hasSelection) {
    el.nowTitle.textContent = `Selected: ${state.selectedTitle}`;
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

  el.indicator.classList.toggle('on', state.running && !state.muted);
  el.muteBtn.classList.toggle('active', state.muted);
  el.logoBtn.classList.toggle('active', state.logoActive);
  el.startBtn.disabled = !state.hasSelection || (state.running && state.selectedSongId === state.playingSongId);
  el.stopBtn.disabled = !state.running;
  showError(state.error);

  for (const li of el.list.children) {
    const id = Number(li.dataset.id);
    li.classList.toggle('selected', id === state.selectedSongId);
    li.classList.toggle('playing', id === state.playingSongId);
  }
}

async function selectSong(id) {
  try {
    state = await api('POST', '/api/select', { id });
    renderState();
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
  } catch (err) {
    showError(err.message);
  }
  setInterval(refreshState, POLL_MS);
})();
