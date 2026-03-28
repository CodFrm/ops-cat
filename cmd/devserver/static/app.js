// OpsKat Extension Dev Server - UI Shell
(function() {
  let manifest = null;
  let config = null;
  const logEntries = [];

  // Tab switching
  document.querySelectorAll('.tab').forEach(tab => {
    tab.addEventListener('click', () => {
      document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
      document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
      tab.classList.add('active');
      document.getElementById('panel-' + tab.dataset.tab).classList.add('active');
    });
  });

  // API helpers
  async function api(method, path, body) {
    const opts = { method, headers: { 'Content-Type': 'application/json' } };
    if (body) opts.body = JSON.stringify(body);
    const resp = await fetch(path, opts);
    return resp.json();
  }

  function escapeHtml(s) {
    return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }

  // ====== Info Panel ======
  async function loadInfo() {
    const data = await api('GET', '/api/manifest');
    manifest = data.manifest;
    document.getElementById('ext-badge').textContent = `${manifest.name} v${manifest.version}`;

    const panel = document.getElementById('panel-info');
    const assetTypes = (manifest.assetTypes || []).map(at =>
      `<div class="tool-item"><h4>${escapeHtml(at.type)}</h4><p>${escapeHtml(at.name)} | testConnection: ${at.testConnection ? 'Yes' : 'No'}</p></div>`
    ).join('');
    const tools = (manifest.tools || []).map(t =>
      `<div class="tool-item"><h4>${escapeHtml(t.name)}</h4><p>${escapeHtml(t.description)}</p></div>`
    ).join('');
    const pages = (manifest.frontend?.pages || []).map(p =>
      `<div class="tool-item"><h4>${escapeHtml(p.id)}</h4><p>${escapeHtml(p.name)} → ${escapeHtml(p.component)}</p></div>`
    ).join('');
    const policies = manifest.policies ? `type: ${manifest.policies.type}, actions: ${(manifest.policies.actions||[]).join(', ')}` : 'none';

    panel.innerHTML = `
      <h2>${escapeHtml(manifest.displayName || manifest.name)}</h2>
      <p style="color:#999;font-size:13px;margin-bottom:16px">${escapeHtml(manifest.description || '')}</p>
      <h3>Asset Types</h3><div class="tool-list">${assetTypes || '<p style="color:#666">None</p>'}</div>
      <h3>Tools (${(manifest.tools||[]).length})</h3><div class="tool-list">${tools}</div>
      <h3>Policies</h3><pre>${escapeHtml(policies)}</pre>
      <h3>Pages</h3><div class="tool-list">${pages || '<p style="color:#666">None</p>'}</div>
      ${data.prompt ? `<h3>Prompt</h3><pre>${escapeHtml(data.prompt)}</pre>` : ''}
    `;

    // Also populate tool select
    const toolSelect = document.getElementById('tool-select');
    if (toolSelect) {
      toolSelect.innerHTML = (manifest.tools || []).map(t =>
        `<option value="${escapeHtml(t.name)}">${escapeHtml(t.name)}</option>`
      ).join('');
    }
  }

  // ====== Config Panel ======
  async function loadConfig() {
    config = await api('GET', '/api/config');
    const panel = document.getElementById('panel-config');
    panel.innerHTML = `
      <h2>Asset Configuration</h2>
      <div class="form-group">
        <label>devconfig.json</label>
        <textarea id="config-editor">${JSON.stringify(config, null, 2)}</textarea>
      </div>
      <div style="display:flex;gap:8px">
        <button onclick="saveConfig()">Save</button>
        <button class="secondary" onclick="testConnection()">Test Connection</button>
      </div>
      <div id="config-result" class="result"></div>
    `;
  }

  window.saveConfig = async function() {
    try {
      const data = JSON.parse(document.getElementById('config-editor').value);
      const result = await api('PUT', '/api/config', data);
      document.getElementById('config-result').innerHTML = '<pre style="color:#34d399">Saved</pre>';
    } catch(e) {
      document.getElementById('config-result').innerHTML = `<pre style="color:#ef4444">${escapeHtml(e.message)}</pre>`;
    }
  };

  window.testConnection = async function() {
    try {
      const cfg = JSON.parse(document.getElementById('config-editor').value);
      const firstAsset = Object.values(cfg.assets || {})[0];
      if (!firstAsset) { alert('No assets configured'); return; }
      const result = await api('POST', '/api/test-connection', firstAsset.config);
      document.getElementById('config-result').innerHTML = `<pre style="color:#34d399">Connection OK</pre>`;
    } catch(e) {
      document.getElementById('config-result').innerHTML = `<pre style="color:#ef4444">${escapeHtml(e.message)}</pre>`;
    }
  };

  // ====== Tools Panel ======
  function initToolsPanel() {
    const panel = document.getElementById('panel-tools');
    panel.innerHTML = `
      <h2>Tool Testing</h2>
      <div class="flex-row">
        <div class="flex-1">
          <div class="form-group">
            <label>Tool</label>
            <select id="tool-select"></select>
          </div>
          <div class="form-group">
            <label>Arguments (JSON)</label>
            <textarea id="tool-args">{"asset_id": 1}</textarea>
          </div>
          <button onclick="executeTool()">Execute</button>
        </div>
        <div class="flex-1">
          <div id="tool-result" class="result"></div>
        </div>
      </div>
    `;
  }

  window.executeTool = async function() {
    const tool = document.getElementById('tool-select').value;
    const argsText = document.getElementById('tool-args').value;
    const resultDiv = document.getElementById('tool-result');
    try {
      const args = JSON.parse(argsText);
      const start = performance.now();
      const resp = await fetch(`/api/tool/${tool}`, {
        method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(args)
      });
      const duration = (performance.now() - start).toFixed(0);
      const data = await resp.json();
      resultDiv.innerHTML = `
        <h3>Result (${duration}ms)</h3>
        <pre>${escapeHtml(JSON.stringify(data, null, 2))}</pre>
      `;
    } catch(e) {
      resultDiv.innerHTML = `<pre style="color:#ef4444">${escapeHtml(e.message)}</pre>`;
    }
  };

  // ====== Extension Page Panel ======
  function initPagePanel() {
    const panel = document.getElementById('panel-page');
    panel.innerHTML = `
      <div style="margin-bottom:12px;display:flex;gap:8px;align-items:center">
        <label style="font-size:12px;color:#999">Asset ID:</label>
        <input id="page-asset-id" type="number" value="1" style="width:80px;padding:4px 8px;background:#1a1a1a;border:1px solid #333;border-radius:4px;color:#e5e5e5">
        <button onclick="loadExtPage()">Load Page</button>
      </div>
      <div id="ext-page-root" class="ext-page-container"></div>
    `;
  }

  window.loadExtPage = async function() {
    if (!manifest) return;
    const assetId = parseInt(document.getElementById('page-asset-id').value) || 1;
    const page = manifest.frontend?.pages?.[0];
    if (!page) { alert('No pages defined'); return; }

    // Load extension module
    const script = document.createElement('script');
    script.src = `/extension/index.js`;
    script.onload = () => {
      const globalName = `__OPSKAT_EXT_${manifest.name}__`;
      const mod = window[globalName];
      if (!mod || !mod[page.component]) {
        alert(`Component ${page.component} not found in ${globalName}`);
        return;
      }
      const Comp = mod[page.component];
      const container = document.getElementById('ext-page-root');
      const root = window.__OPSKAT_EXT__.ReactDOM.createRoot(container);
      root.render(window.__OPSKAT_EXT__.React.createElement(Comp, {
        assetId: assetId,
        assetType: manifest.assetTypes?.[0]?.type || manifest.name
      }));
    };
    document.head.appendChild(script);
  };

  // ====== Logs Panel ======
  function initLogsPanel() {
    const panel = document.getElementById('panel-logs');
    panel.innerHTML = `
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px">
        <h2>Real-time Logs</h2>
        <button class="secondary" onclick="document.getElementById('log-container').innerHTML=''">Clear</button>
      </div>
      <div id="log-container" style="font-family:monospace;font-size:12px"></div>
    `;

    // SSE connection
    const es = new EventSource('/api/logs');
    es.onmessage = (e) => {
      const entry = JSON.parse(e.data);
      const container = document.getElementById('log-container');
      if (!container) return;
      const time = new Date(entry.time).toLocaleTimeString();
      const type = entry.type || 'log';
      const detail = entry.detail ? JSON.stringify(entry.detail) : '';
      container.innerHTML += `<div class="log-entry"><span style="color:#666">${time}</span> <span class="log-type ${type}">[${type.toUpperCase()}]</span> ${escapeHtml(detail)}</div>`;
      container.scrollTop = container.scrollHeight;
    };
  }

  // ====== Setup __OPSKAT_EXT__ for extension page ======
  function setupExtRuntime() {
    // Load React from CDN for extension rendering
    const reactScript = document.createElement('script');
    reactScript.src = 'https://unpkg.com/react@19/umd/react.production.min.js';
    reactScript.crossOrigin = '';
    reactScript.onload = () => {
      const reactDomScript = document.createElement('script');
      reactDomScript.src = 'https://unpkg.com/react-dom@19/umd/react-dom.production.min.js';
      reactDomScript.crossOrigin = '';
      reactDomScript.onload = () => {
        window.__OPSKAT_EXT__ = {
          React: window.React,
          ReactDOM: window.ReactDOM,
          jsxRuntime: { jsx: window.React.createElement, jsxs: window.React.createElement, Fragment: window.React.Fragment },
          api: {
            callTool: async (name, args) => {
              // Parse "ext.tool" to just "tool" for our API
              const parts = name.split('.');
              const toolName = parts.length > 1 ? parts.slice(1).join('.') : name;
              const resp = await fetch(`/api/tool/${toolName}`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(args)
              });
              return resp.json();
            }
          }
        };
      };
      document.head.appendChild(reactDomScript);
    };
    document.head.appendChild(reactScript);
  }

  // ====== Init ======
  setupExtRuntime();
  initToolsPanel();
  initPagePanel();
  initLogsPanel();
  loadInfo().then(() => {
    // Populate tool select after manifest loads
    const toolSelect = document.getElementById('tool-select');
    if (toolSelect && manifest) {
      toolSelect.innerHTML = (manifest.tools || []).map(t =>
        `<option value="${t.name}">${t.name}</option>`
      ).join('');
    }
  });
  loadConfig();
})();
