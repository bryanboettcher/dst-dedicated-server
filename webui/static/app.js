// DST Server Web UI

const $ = (sel) => document.querySelector(sel);

// --- SSE status stream ---

let evtSource = null;

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
      updateStatus(JSON.parse(e.data));
    } catch (err) {
      console.error('failed to parse status:', err);
    }
  };

  evtSource.onerror = () => {
    badge.textContent = 'disconnected';
    badge.className = 'badge badge-error';
    evtSource.close();
    // Reconnect after 3s
    setTimeout(connectSSE, 3000);
  };

  evtSource.addEventListener('error', (e) => {
    if (typeof e.data === 'string') {
      console.warn('SSE error event:', e.data);
    }
  });
}

function updateStatus(status) {
  // State
  const stateEl = $('#state');
  stateEl.textContent = status.state || '—';
  stateEl.className = 'card-value state-' + (status.state || 'unknown');

  // Players
  if (status.players !== null && status.players !== undefined) {
    $('#players').textContent = status.players + ' / ' + (status.max_players || '?');
  } else {
    $('#players').textContent = '—';
  }

  // Server name
  $('#server-name').textContent = status.server_name || '—';

  // Map
  $('#map').textContent = status.map || '—';

  // Shard
  const shardText = [];
  if (status.cluster) shardText.push(status.cluster);
  if (status.shard) shardText.push(status.shard);
  $('#shard').textContent = shardText.join(' / ') || '—';

  // Uptime
  $('#uptime').textContent = status.uptime || '—';
}

// --- API actions ---

async function apiAction(action) {
  try {
    disableButtons(true);
    const resp = await fetch('/api/' + action, { method: 'POST' });
    const data = await resp.json();
    if (data.ok) {
      toast(data.message || action + ' OK', 'ok');
    } else {
      toast(data.error || 'failed', 'err');
    }
  } catch (err) {
    toast(err.message, 'err');
  } finally {
    disableButtons(false);
  }
}

function confirmAction(action) {
  if (confirm('Are you sure you want to ' + action + ' the server?')) {
    apiAction(action);
  }
}

async function rollback() {
  const days = $('#rollback-days').value;
  const path = days ? '/api/rollback/' + days : '/api/rollback/';
  try {
    disableButtons(true);
    const resp = await fetch(path, { method: 'POST' });
    const data = await resp.json();
    if (data.ok) {
      toast(data.message || 'rollback OK', 'ok');
    } else {
      toast(data.error || 'rollback failed', 'err');
    }
  } catch (err) {
    toast(err.message, 'err');
  } finally {
    disableButtons(false);
  }
}

async function sendConsole() {
  const input = $('#console-input');
  const cmd = input.value.trim();
  if (!cmd) return;

  logConsole('> ' + cmd, 'cmd');
  input.value = '';

  try {
    const resp = await fetch('/api/console', {
      method: 'POST',
      body: cmd,
    });
    const data = await resp.json();
    if (data.ok) {
      logConsole(data.message, 'ok');
    } else {
      logConsole(data.error || 'error', 'err');
    }
  } catch (err) {
    logConsole(err.message, 'err');
  }
}

// --- Console log ---

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

  // Keep last 50 entries
  while (log.children.length > 50) {
    log.removeChild(log.firstChild);
  }

  log.scrollTop = log.scrollHeight;
}

// --- Toast notifications ---

function toast(msg, type) {
  const container = $('#toast-container');
  const el = document.createElement('div');
  el.className = 'toast toast-' + type;
  el.textContent = msg;
  container.appendChild(el);
  setTimeout(() => el.remove(), 4000);
}

// --- Helpers ---

function disableButtons(disabled) {
  document.querySelectorAll('.btn').forEach(btn => {
    btn.disabled = disabled;
  });
}

// --- Init ---
connectSSE();
