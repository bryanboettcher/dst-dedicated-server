// DST Server Web UI — Multi-shard

const $ = (sel) => document.querySelector(sel);

let shardNames = [];
let masterShard = null;
let evtSource = null;
let logStream = null; // SSE for log tailing
let currentLogShard = null;

// SVG icons (inline, no external deps)
const ICON_START = '<svg viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg>';
const ICON_STOP = '<svg viewBox="0 0 24 24"><path d="M6 6h12v12H6z"/></svg>';
const ICON_RESTART = '<svg viewBox="0 0 24 24"><path d="M17.65 6.35A7.96 7.96 0 0 0 12 4a8 8 0 1 0 8 8h-2a6 6 0 1 1-1.76-4.24L14 10h7V3l-3.35 3.35z"/></svg>';

// --- Init ---

async function init() {
  try {
    const resp = await fetch('/shards');
    shardNames = await resp.json();
  } catch {
    shardNames = ['default'];
  }

  buildShardUI();
  populateShardSelect();
  populateLogSelect();
  connectSSE();
}

function buildShardUI() {
  const container = $('#shards');
  container.innerHTML = '';

  for (const name of shardNames) {
    const section = document.createElement('section');
    section.className = 'shard-section';
    section.id = 'shard-' + name;
    section.innerHTML = `
      <div class="shard-header">
        <div class="shard-header-left" onclick="toggleShard('${name}')">
          <h2>
            ${name}
            <span class="master-badge" id="master-badge-${name}" style="display:none">master</span>
          </h2>
          <span class="shard-state" id="shard-state-${name}">—</span>
        </div>
        <div class="shard-controls">
          <button class="icon-btn icon-btn-start" id="start-btn-${name}"
                  onclick="event.stopPropagation(); shardAction('${name}', 'restart')"
                  title="Start shard" disabled>
            ${ICON_START}
          </button>
          <button class="icon-btn icon-btn-restart" id="restart-btn-${name}"
                  onclick="event.stopPropagation(); shardAction('${name}', 'restart')"
                  title="Restart shard" disabled>
            ${ICON_RESTART}
          </button>
          <button class="icon-btn icon-btn-stop" id="stop-btn-${name}"
                  onclick="event.stopPropagation(); confirmShardAction('${name}', 'shutdown')"
                  title="Stop shard" disabled>
            ${ICON_STOP}
          </button>
        </div>
      </div>
      <div class="shard-body">
        <div class="status-grid">
          <div class="card">
            <div class="card-label">Players</div>
            <div class="card-value" id="players-${name}">—</div>
          </div>
          <div class="card">
            <div class="card-label">Server</div>
            <div class="card-value small" id="server-name-${name}">—</div>
          </div>
          <div class="card">
            <div class="card-label">Map</div>
            <div class="card-value small" id="map-${name}">—</div>
          </div>
          <div class="card">
            <div class="card-label">Uptime</div>
            <div class="card-value small" id="uptime-${name}">—</div>
          </div>
        </div>
      </div>
    `;
    container.appendChild(section);
  }
}

function populateShardSelect() {
  const select = $('#console-shard');
  select.innerHTML = '';
  for (const name of shardNames) {
    const opt = document.createElement('option');
    opt.value = name;
    opt.textContent = name;
    select.appendChild(opt);
  }
}

function toggleShard(name) {
  const body = $(`#shard-${name} .shard-body`);
  body.style.display = body.style.display === 'none' ? '' : 'none';
}

// --- SSE ---

function connectSSE() {
  if (evtSource) evtSource.close();

  const badge = $('#connection-status');
  badge.textContent = 'connecting';
  badge.className = 'badge badge-connecting';

  evtSource = new EventSource('/events');

  evtSource.onmessage = (e) => {
    badge.textContent = 'connected';
    badge.className = 'badge badge-connected';
    try {
      const data = JSON.parse(e.data);
      for (const [name, status] of Object.entries(data)) {
        updateShard(name, status);
      }
    } catch (err) {
      console.error('parse error:', err);
    }
  };

  evtSource.onerror = () => {
    badge.textContent = 'disconnected';
    badge.className = 'badge badge-error';
    evtSource.close();
    setTimeout(connectSSE, 3000);
  };
}

function updateShard(name, status) {
  const stateEl = $(`#shard-state-${name}`);
  if (!stateEl) return;

  // Discover master
  if (status.is_master) {
    masterShard = name;
    const badge = $(`#master-badge-${name}`);
    if (badge) badge.style.display = '';
  } else {
    const badge = $(`#master-badge-${name}`);
    if (badge) badge.style.display = 'none';
  }

  const state = status.state || 'unknown';
  stateEl.textContent = state;
  stateEl.className = 'shard-state state-' + state;

  // Update icon button states
  const startBtn = $(`#start-btn-${name}`);
  const restartBtn = $(`#restart-btn-${name}`);
  const stopBtn = $(`#stop-btn-${name}`);

  const isRunning = state === 'running' || state === 'starting';
  const isStopped = state === 'stopped';

  startBtn.disabled = !isStopped;
  restartBtn.disabled = !isRunning;
  stopBtn.disabled = !isRunning;

  // Players
  const playersEl = $(`#players-${name}`);
  if (status.player_count !== undefined) {
    const names = (status.players || []).map(p => p.name).join(', ');
    playersEl.textContent = status.player_count.toString();
    playersEl.title = names || 'no players';
  } else {
    playersEl.textContent = '—';
    playersEl.title = '';
  }

  $(`#server-name-${name}`).textContent = status.server_name || '—';
  $(`#map-${name}`).textContent = status.map || '—';
  $(`#uptime-${name}`).textContent = status.uptime || '—';
}

// --- Cluster actions (routed to master shard) ---

function getMaster() {
  return masterShard || shardNames[0];
}

async function clusterAction(action) {
  const name = getMaster();
  try {
    const resp = await fetch(`/shard/${name}/api/${action}`, { method: 'POST' });
    const data = await resp.json();
    if (data.ok) {
      toast(data.message || action + ' OK', 'ok');
    } else {
      toast(data.error || 'failed', 'err');
    }
  } catch (err) {
    toast(err.message, 'err');
  }
}

function confirmClusterAction(action) {
  if (confirm(`Are you sure you want to ${action} the cluster?`)) {
    clusterAction(action);
  }
}

async function clusterRollback() {
  const name = getMaster();
  const days = $('#rollback-days').value;
  const path = days ? `/shard/${name}/api/rollback/${days}` : `/shard/${name}/api/rollback/`;
  try {
    const resp = await fetch(path, { method: 'POST' });
    const data = await resp.json();
    if (data.ok) {
      toast(data.message, 'ok');
    } else {
      toast(data.error, 'err');
    }
  } catch (err) {
    toast(err.message, 'err');
  }
}

async function clusterCommand(cmd) {
  const name = getMaster();
  try {
    const resp = await fetch(`/shard/${name}/api/console`, {
      method: 'POST',
      body: cmd,
    });
    const data = await resp.json();
    if (data.ok) {
      toast(data.message, 'ok');
      logConsole(`[${name}] > ${cmd}`, 'ok');
    } else {
      toast(data.error, 'err');
      logConsole(`[${name}] ${data.error}`, 'err');
    }
  } catch (err) {
    toast(err.message, 'err');
  }
}

function confirmClusterCommand(cmd, description) {
  if (confirm(`Are you sure you want to ${description}?`)) {
    clusterCommand(cmd);
  }
}

// --- Announce (all shards) ---

async function announceAll() {
  const input = $('#announce-input');
  const msg = input.value.trim();
  if (!msg) return;

  const escaped = msg.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
  const cmd = `c_announce("${escaped}")`;
  input.value = '';

  for (const name of shardNames) {
    try {
      await fetch(`/shard/${name}/api/console`, {
        method: 'POST',
        body: cmd,
      });
      logConsole(`[${name}] > ${cmd}`, 'ok');
    } catch (err) {
      logConsole(`[${name}] ${err.message}`, 'err');
    }
  }
  toast('announced to all shards', 'ok');
}

// --- Per-shard actions ---

async function shardAction(name, action) {
  try {
    const resp = await fetch(`/shard/${name}/api/${action}`, { method: 'POST' });
    const data = await resp.json();
    if (data.ok) {
      toast(`[${name}] ${data.message || action + ' OK'}`, 'ok');
    } else {
      toast(`[${name}] ${data.error || 'failed'}`, 'err');
    }
  } catch (err) {
    toast(`[${name}] ${err.message}`, 'err');
  }
}

function confirmShardAction(name, action) {
  if (confirm(`Are you sure you want to ${action} the ${name} shard?`)) {
    shardAction(name, action);
  }
}

// --- Console ---

async function sendConsole() {
  const input = $('#console-input');
  const cmd = input.value.trim();
  if (!cmd) return;

  const name = $('#console-shard').value;
  logConsole(`[${name}] > ${cmd}`, 'cmd');
  input.value = '';

  try {
    const resp = await fetch(`/shard/${name}/api/console`, {
      method: 'POST',
      body: cmd,
    });
    const data = await resp.json();
    if (data.ok) {
      logConsole(`[${name}] ${data.message}`, 'ok');
    } else {
      logConsole(`[${name}] ${data.error}`, 'err');
    }
  } catch (err) {
    logConsole(`[${name}] ${err.message}`, 'err');
  }
}

function logConsole(msg, type) {
  const log = $('#console-log');
  const entry = document.createElement('div');
  entry.className = 'log-entry';

  const time = document.createElement('span');
  time.className = 'log-time';
  time.textContent = new Date().toLocaleTimeString();

  const text = document.createElement('span');
  text.className = 'log-' + (type === 'cmd' ? 'ok' : type);
  text.textContent = msg;

  entry.appendChild(time);
  entry.appendChild(text);
  log.appendChild(entry);

  while (log.children.length > 50) {
    log.removeChild(log.firstChild);
  }
  log.scrollTop = log.scrollHeight;
}

// --- Toast ---

function toast(msg, type) {
  const container = $('#toast-container');
  const el = document.createElement('div');
  el.className = 'toast toast-' + type;
  el.textContent = msg;
  container.appendChild(el);
  setTimeout(() => el.remove(), 4000);
}

// --- Log viewer ---

function populateLogSelect() {
  const select = $('#log-shard');
  select.innerHTML = '';
  for (const name of shardNames) {
    const opt = document.createElement('option');
    opt.value = name;
    opt.textContent = name;
    select.appendChild(opt);
  }
  if (shardNames.length > 0) {
    switchLogShard();
  }
}

async function switchLogShard() {
  const name = $('#log-shard').value;
  if (name === currentLogShard) return;
  currentLogShard = name;

  // Disconnect existing stream
  if (logStream) {
    logStream.close();
    logStream = null;
  }

  const output = $('#log-output');
  output.innerHTML = '';

  // Load initial backlog
  try {
    const resp = await fetch(`/shard/${name}/api/logs?lines=200`);
    const lines = await resp.json();
    if (lines && lines.length) {
      for (const line of lines) {
        appendLogLine(line);
      }
    }
  } catch (err) {
    appendLogLine(`[error loading logs: ${err.message}]`);
  }

  // Connect live stream
  logStream = new EventSource(`/shard/${name}/api/logs/stream`);
  logStream.onmessage = (e) => {
    appendLogLine(e.data);
  };
  logStream.onerror = () => {
    // Will auto-reconnect via EventSource spec
  };
}

function appendLogLine(text) {
  const output = $('#log-output');
  const line = document.createElement('div');
  line.className = 'log-line';
  line.textContent = text;
  output.appendChild(line);

  // Cap at 500 lines
  while (output.children.length > 500) {
    output.removeChild(output.firstChild);
  }

  // Auto-scroll if enabled
  if ($('#log-autoscroll').checked) {
    output.scrollTop = output.scrollHeight;
  }
}

// --- Init ---
init();
