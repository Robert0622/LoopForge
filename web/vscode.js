(() => {
  const params = new URLSearchParams(location.search);
  const runId = String(params.get('run') || '').trim();
  const embedded = params.get('embed') === '1';
  const host = document.getElementById('workbench') || document.body;
  if (embedded) document.body.classList.add('embed');

  function vscodeWorkbenchURL(id, options = {}) {
    const next = new URLSearchParams();
    if (id) next.set('run', id);
    if (options.embed) next.set('embed', '1');
    return `/vscode.html?${next.toString()}`;
  }
  function vscodeWindowName(id) {
    return 'loopforge-vscode-' + String(id || '').replace(/[^A-Za-z0-9_-]/g, '');
  }
  function openVSCodeWindow(id) {
    const win = window.open(vscodeWorkbenchURL(id), vscodeWindowName(id), 'popup=yes,width=1440,height=900');
    if (!win) return false;
    try { win.focus(); } catch (e) {}
    return true;
  }
  function postParent(action, extra = {}) {
    if (!embedded || !window.parent || window.parent === window) return;
    window.parent.postMessage({ type: 'loopforge-vscode', runId, action, ...extra }, location.origin);
  }
  function escapeHTML(value) {
    return String(value || '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  }
  function showError(title, message) {
    host.innerHTML = `<div class="vscode-error"><b>${escapeHTML(title)}</b><p>${escapeHTML(message)}</p></div>`;
  }
  async function workspaceInfo() {
    const res = await fetch(`/api/runs/${encodeURIComponent(runId)}/workspace`, {
      credentials: 'same-origin',
      headers: { Accept: 'application/json' }
    });
    const body = await res.json().catch(() => ({}));
    if (res.status === 401) throw new Error('请先登录平台后再打开工作区');
    if (!res.ok) throw new Error(body.error || `HTTP ${res.status}`);
    return body;
  }
  function loadScript(src) {
    return new Promise((resolve, reject) => {
      const el = document.createElement('script');
      el.src = src;
      el.onload = () => resolve();
      el.onerror = () => reject(new Error('加载失败: ' + src));
      document.head.appendChild(el);
    });
  }
  function loadStyle(href) {
    const el = document.createElement('link');
    el.rel = 'stylesheet';
    el.href = href;
    document.head.appendChild(el);
  }

  window.vscodeWorkbenchURL = vscodeWorkbenchURL;
  window.vscodeWindowName = vscodeWindowName;
  window.openVSCodeWindow = openVSCodeWindow;

  if (!runId) {
    showError('缺少 Run ID', '请从 SOP 运行详情打开工作区。');
    return;
  }

  host.innerHTML = `<div class="vscode-boot"><span class="vscode-spinner"></span><b>正在启动 VS Code…</b></div>`;

  const baseUrl = `${location.origin}/vscode-web`;
  loadStyle(`${baseUrl}/out/vs/workbench/workbench.web.main.css`);

  (async () => {
    let repositories = [];
    let workspaceChanges = [];
    try {
      const info = await workspaceInfo();
      if (!info.available) {
        showError('当前无法浏览工作区', info.reason || '工作区不可用');
        return;
      }
      workspaceChanges = Array.isArray(info.changes) ? info.changes : [];
      const names = new Set();
      for (const name of info.repositories || []) {
        const repo = String(name || '').trim();
        if (repo) names.add(repo === '.' ? 'workspace' : repo);
      }
      if (!names.size) {
        for (const change of info.changes || []) {
          const repo = String(change.repository || '').trim();
          names.add(!repo || repo === '.' ? 'workspace' : repo);
        }
      }
      repositories = [...names];
    } catch (err) {
      showError('无法打开工作区', err?.message || String(err));
      return;
    }
    await loadScript(`${baseUrl}/out/vs/loader.js`);
    await loadScript(`${baseUrl}/out/vs/webPackagePaths.js`);
    const paths = {};
    Object.keys(self.webPackagePaths || {}).forEach(key => {
      paths[key] = `${baseUrl}/node_modules/${key}/${self.webPackagePaths[key]}`;
    });
    window.require.config({
      baseUrl: `${baseUrl}/out`,
      recordStats: true,
      urlArgs: 'v=native-scm-46',
      trustedTypesPolicy: window.trustedTypes?.createPolicy('amdLoader', {
        createScriptURL(value) {
          if (value.startsWith(location.origin) || value.startsWith('/')) return value;
          throw new Error('Invalid script url: ' + value);
        }
      }),
      paths
    });
    await loadScript(`${baseUrl}/out/vs/workbench/workbench.web.main.nls.js?v=native-scm-46`);
    await loadScript(`${baseUrl}/out/vs/workbench/workbench.web.main.js?v=native-scm-46`);
    window.require(['vs/workbench/workbench.web.main', 'vs/base/common/uri'], (workbench, uriMod) => {
      const URI = uriMod.URI;
      const folderUri = URI.from({
        scheme: 'loopforge',
        authority: runId,
        path: '/',
        query: (() => {
          const params = new URLSearchParams();
          if (repositories.length) params.set('repos', JSON.stringify(repositories));
          if (workspaceChanges.length) params.set('changes', JSON.stringify(workspaceChanges));
          const encoded = params.toString();
          return encoded || undefined;
        })()
      });
      const extensionUri = URI.parse(`${location.origin}/extensions/loopforge-workspace`);
      host.replaceChildren();
      workbench.create(host, {
        additionalBuiltinExtensions: [extensionUri],
        folderUri,
        workspaceProvider: {
          workspace: { folderUri },
          trusted: true,
          open: async () => true
        },
        configurationDefaults: {
          'workbench.colorTheme': 'Default Dark Modern',
          'workbench.startupEditor': 'none',
          'window.commandCenter': true,
          'workbench.layoutControl.enabled': true,
          'workbench.tips.enabled': false,
          'files.autoSave': 'off',
          'editor.readOnly': true,
          'git.enabled': false,
          'scm.alwaysShowRepositories': true,
          'scm.defaultViewMode': 'list',
          'scm.experimental.showHistoryGraph': false,
          'scm.showIncomingChanges': 'never',
          'scm.showOutgoingChanges': 'auto',
          'scm.showChangesSummary': false,
          'scm.repositories.visible': 10,
          'loopforge.repositories': repositories
        },
        productConfiguration: {
          nameShort: 'LoopForge VS Code',
          nameLong: 'LoopForge VS Code',
          applicationName: 'loopforge-vscode',
          dataFolderName: '.loopforge-vscode',
          version: '1.91.1',
          quality: 'stable',
          enableTelemetry: false,
          extensionEnabledApiProposals: {
            'loopforge.workspace': ['fileSearchProvider', 'textSearchProvider', 'scmHistoryProvider']
          }
        },
        windowIndicator: {
          label: `$(remote) SOP ${runId.slice(-12)}`,
          tooltip: embedded ? 'SOP run 工作区（只读）' : 'SOP run 工作区（只读，独立窗口）',
          command: embedded ? 'loopforge.popout' : undefined
        },
        commands: [
          {
            id: 'loopforge.popout',
            label: '在新窗口打开 VS Code',
            menu: 1,
            handler: () => {
              if (embedded) postParent('popout');
              else openVSCodeWindow(runId);
            }
          },
          {
            id: 'loopforge.maximize',
            label: '沉浸模式',
            menu: 1,
            handler: () => postParent('maximize')
          }
        ]
      });
    }, err => {
      showError('VS Code 启动失败', err?.message || String(err));
    });
  })().catch(err => {
    showError('VS Code 启动失败', err?.message || String(err));
  });
})();
