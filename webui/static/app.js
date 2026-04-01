// DST Server Web UI — Multi-shard

const $ = (sel) => document.querySelector(sel);

let shardNames = [];
let masterShard = null; // discovered from status data
let evtSource = null;

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
      <div class="shard-header" onclick="toggleShard('${name}')">
        <h2>
          ${name}
          <span class="master-badge" id="master-badge-${name}" style="display:none">master</span>
        </h2>
        <span class="shard-state" id="shard-state-${name}">—</span>
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
        <div class="shard-actions">
          <div class="action-row">
            <button class="btn btn-warning" onclick="shardAction('${name}', 'restart')">Restart Shard</button>
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

  // Discover master from status data
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

  const players = $(`#players-${name}`);
  if (status.players !== null && status.players !== undefined) {
    players.textContent = status.players + ' / ' + (status.max_players || '?');
  } else {
    players.textContent = '—';
  }

  $(`#server-name-${name}`).textContent = status.server_name || '—';
  $(`#map-${name}`).textContent = status.map || '—';
  $(`#uptime-${name}`).textContent = status.uptime || '—';
}

// --- Cluster actions (routed to master shard) ---

function getMaster() {
  if (masterShard) return masterShard;
  // Fallback to first shard if master not yet discovered
  return shardNames[0];
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

// --- Announce (sent to all shards) ---

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

// --- Init ---
init();
