'use strict';

const vscode = require('vscode');

const FileType = { Unknown: 0, File: 1, Directory: 2, SymbolicLink: 64 };
const BASE_SCHEME = 'loopforge-base';

function origin() {
  try { return self.location.origin; } catch (e) { return ''; }
}

async function fetchJSON(path) {
  const res = await fetch(`${origin()}${path}`, { credentials: 'same-origin', headers: { Accept: 'application/json' } });
  const body = await res.json().catch(() => ({}));
  if (res.status === 401) throw vscode.FileSystemError.Unavailable('请先登录平台后再打开工作区');
  if (res.status === 404) throw vscode.FileSystemError.FileNotFound();
  if (!res.ok) throw vscode.FileSystemError.Unavailable(body.error || `HTTP ${res.status}`);
  return body;
}

function uriQuery(uri) {
  const raw = String(uri && uri.query || '');
  if (!raw) return new URLSearchParams();
  try {
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      const params = new URLSearchParams();
      for (const [key, value] of Object.entries(parsed)) {
        if (value != null) params.set(key, String(value));
      }
      return params;
    }
  } catch (err) {}
  return new URLSearchParams(raw);
}

function gitRefQuery(ref) {
  return JSON.stringify({ ref: String(ref || '') });
}

function uriPath(uri) {
  return String(uri.path || '/').replace(/^\/+/, '').replace(/\/+$/, '');
}

function File(name, size, mtime) {
  return { type: FileType.File, ctime: mtime, mtime, size, name };
}

function Dir(name, mtime) {
  return { type: FileType.Directory, ctime: mtime, mtime, size: 0, name };
}

function changeCode(change) {
  const index = String(change?.index || ' ');
  const worktree = String(change?.worktree || ' ');
  if (index === 'R' || worktree === 'R' || index === 'C' || worktree === 'C') return 'R';
  if (index === 'D' || worktree === 'D') return 'D';
  if (index === 'A' || worktree === 'A') return 'A';
  if (index === '?' || worktree === '?') return 'U';
  return 'M';
}

function changeDecoration(code) {
  switch (code) {
    case 'D':
      return { strikeThrough: true, tooltip: 'Deleted', color: new vscode.ThemeColor('gitDecoration.deletedResourceForeground') };
    case 'A':
      return { tooltip: 'Added', color: new vscode.ThemeColor('gitDecoration.addedResourceForeground') };
    case 'U':
      return { tooltip: 'Untracked', color: new vscode.ThemeColor('gitDecoration.untrackedResourceForeground') };
    case 'R':
      return { tooltip: 'Renamed', color: new vscode.ThemeColor('gitDecoration.renamedResourceForeground') };
    default:
      return { tooltip: 'Modified', color: new vscode.ThemeColor('gitDecoration.modifiedResourceForeground') };
  }
}

function workspaceRepoName(name) {
  const key = String(name || '').trim();
  if (!key || key === '.') return 'workspace';
  return key;
}

function changeRepositoryName(change) {
  return workspaceRepoName(change && change.repository);
}

function sourceControlRootPath(name) {
  const key = workspaceRepoName(name);
  return key === 'workspace' ? '/' : '/' + key;
}

function repositoriesFromChanges(changes, tree, names) {
  const byRepo = new Map();
  const ensure = (path) => {
    const key = workspaceRepoName(path);
    if (!byRepo.has(key)) {
      byRepo.set(key, { path: key === 'workspace' ? '' : key, name: key, changes: [] });
    }
    return byRepo.get(key);
  };
  for (const name of names || []) ensure(name);
  for (const change of changes || []) {
    ensure(changeRepositoryName(change)).changes.push(change);
  }
  if (!byRepo.size) ensure('workspace');
  return [...byRepo.values()];
}

// workspaceGone marks an error for a workspace the platform reports as
// unavailable (cleaned after the run, runner uninstalled): a final state,
// not a failure to retry.
function workspaceGone(err) {
  err.workspaceGone = true;
  return err;
}

class LoopForgeFS {
  constructor(runId) {
    this.runId = runId;
    this._tree = null;
    this._changes = [];
    this._emitter = new vscode.EventEmitter();
    this.onDidChangeFile = this._emitter.event;
  }

  watch() {
    return new vscode.Disposable(() => {});
  }

  async _api(path) {
    return fetchJSON(path);
  }

  async _entries() {
    if (this._tree) return this._tree;
    if (this._pending) return this._pending;
    this._pending = (async () => {
      const info = await this._api(`/api/runs/${encodeURIComponent(this.runId)}/workspace`);
      if (!info.available) {
        throw workspaceGone(vscode.FileSystemError.Unavailable(info.reason || '工作区不可用'));
      }
      const tree = await this._api(`/api/runs/${encodeURIComponent(this.runId)}/workspace/tree`);
      this._tree = Array.isArray(tree.entries) ? tree.entries : [];
      this._changes = Array.isArray(info.changes) ? info.changes : [];
      return this._tree;
    })();
    try {
      return await this._pending;
    } catch (err) {
      this._pending = null;
      throw err;
    }
  }

  changeFor(path) {
    return (this._changes || []).find(item => item.path === path);
  }

  invalidate() {
    this._tree = null;
    this._pending = null;
  }

  async stat(uri) {
    const path = uriPath(uri);
    const ref = uriQuery(uri).get('ref');
    const now = Date.now();
    if (ref) {
      const content = await this.readFile(uri);
      return File(path.split('/').pop() || path, content.length, now);
    }
    if (!path) return Dir('', now);
    const entries = await this._entries();
    const exact = entries.find(item => item.path === path);
    if (exact) {
      const mtime = exact.modifiedAt ? Date.parse(exact.modifiedAt) || now : now;
      return exact.type === 'directory' ? Dir(exact.name, mtime) : File(exact.name, exact.size || 0, mtime);
    }
    if (entries.some(item => item.path.startsWith(path + '/'))) return Dir(path.split('/').pop(), now);
    throw vscode.FileSystemError.FileNotFound(uri);
  }

  async readDirectory(uri) {
    const path = uriPath(uri);
    const prefix = path ? path + '/' : '';
    const entries = await this._entries();
    const seen = new Map();
    for (const entry of entries) {
      if (prefix && !entry.path.startsWith(prefix)) continue;
      if (!prefix && entry.path === '') continue;
      const rest = prefix ? entry.path.slice(prefix.length) : entry.path;
      if (!rest) continue;
      const slash = rest.indexOf('/');
      const name = slash === -1 ? rest : rest.slice(0, slash);
      const type = slash === -1 && entry.type !== 'directory' ? FileType.File : FileType.Directory;
      if (!seen.has(name) || type === FileType.Directory) seen.set(name, type);
    }
    return [...seen.entries()];
  }

  async readFile(uri) {
    const path = uriPath(uri);
    if (!path) throw vscode.FileSystemError.FileIsADirectory(uri);
    const ref = uriQuery(uri).get('ref');
    if (ref === 'empty') return new TextEncoder().encode('');
    const q = ref ? `&ref=${encodeURIComponent(ref)}` : '';
    const file = await this._api(`/api/runs/${encodeURIComponent(this.runId)}/workspace/file?path=${encodeURIComponent(path)}${q}`);
    return new TextEncoder().encode(file.content || '');
  }

  writeFile() { throw vscode.FileSystemError.NoPermissions(); }
  createDirectory() { throw vscode.FileSystemError.NoPermissions(); }
  delete() { throw vscode.FileSystemError.NoPermissions(); }
  rename() { throw vscode.FileSystemError.NoPermissions(); }
}

class LoopForgeBaseFS {
  constructor(runId) {
    this.runId = runId;
    this._originals = new Map();
    this._emitter = new vscode.EventEmitter();
    this.onDidChangeFile = this._emitter.event;
  }

  watch() {
    return new vscode.Disposable(() => {});
  }

  async _load(path) {
    if (this._originals.has(path)) return this._originals.get(path);
    const body = await fetchJSON(`/api/runs/${encodeURIComponent(this.runId)}/workspace/diff?path=${encodeURIComponent(path)}`);
    const content = typeof body.original === 'string' ? body.original : '';
    this._originals.set(path, content);
    return content;
  }

  invalidate() {
    this._originals.clear();
  }

  async stat(uri) {
    const path = uriPath(uri);
    if (!path) return Dir('', Date.now());
    const content = await this._load(path);
    return File(path.split('/').pop(), new TextEncoder().encode(content).length, Date.now());
  }

  async readFile(uri) {
    const path = uriPath(uri);
    if (!path) throw vscode.FileSystemError.FileIsADirectory(uri);
    return new TextEncoder().encode(await this._load(path));
  }

  async readDirectory() { return []; }
  writeFile() { throw vscode.FileSystemError.NoPermissions(); }
  createDirectory() { throw vscode.FileSystemError.NoPermissions(); }
  delete() { throw vscode.FileSystemError.NoPermissions(); }
  rename() { throw vscode.FileSystemError.NoPermissions(); }
}

function workspaceUri(runId, path, scheme) {
  return vscode.Uri.from({
    scheme: scheme || 'loopforge',
    authority: runId,
    path: '/' + String(path || '').replace(/^\/+/, '')
  });
}

function changeResourceDecorations(code) {
  switch (code) {
    case 'D':
      return { strikeThrough: true, faded: true, tooltip: 'Deleted' };
    case 'A':
      return { tooltip: 'Added' };
    case 'U':
      return { tooltip: 'Untracked' };
    case 'R':
      return { tooltip: 'Renamed' };
    default:
      return { tooltip: 'Modified' };
  }
}

function shouldDiffChange(change) {
  return changeCode(change) !== 'U';
}

function changeResourceState(runId, change) {
  const code = changeCode(change);
  return {
    resourceUri: workspaceUri(runId, change.path, 'loopforge'),
    decorations: changeResourceDecorations(code),
    command: {
      command: 'loopforge.openChange',
      title: 'Open',
      arguments: [String(change.path || ''), code]
    }
  };
}

async function openChange(path, code) {
  const folder = vscode.workspace.workspaceFolders && vscode.workspace.workspaceFolders[0];
  const runId = folder && folder.uri.authority ? decodeURIComponent(folder.uri.authority) : '';
  if (!runId || !path) return;
  const uri = workspaceUri(runId, path, 'loopforge');
  try {
    if (code === 'U') {
      await vscode.commands.executeCommand('vscode.open', uri, { preview: true });
      return;
    }
    const original = workspaceUri(runId, path, BASE_SCHEME);
    const title = `${String(path).split('/').pop()} (Working Tree)`;
    await vscode.commands.executeCommand('vscode.diff', original, uri, title, { preview: true });
  } catch (err) {
    console.error('[loopforge] failed to open change', path, err);
    await vscode.commands.executeCommand('vscode.open', uri, { preview: true });
  }
}

function folderQueryValue(key) {
  const folder = vscode.workspace.workspaceFolders && vscode.workspace.workspaceFolders[0];
  const query = folder && folder.uri && folder.uri.query ? String(folder.uri.query) : '';
  if (!query) return null;
  try {
    const parsed = JSON.parse(new URLSearchParams(query).get(key) || 'null');
    return parsed;
  } catch (err) {
    return null;
  }
}

function repositoryNames(context) {
  const configured = vscode.workspace.getConfiguration('loopforge').get('repositories');
  if (Array.isArray(configured) && configured.length) {
    return configured.map(name => String(name || '').trim()).filter(Boolean);
  }
  const parsed = folderQueryValue('repos');
  if (Array.isArray(parsed) && parsed.length) {
    return parsed.map(name => String(name || '').trim()).filter(Boolean);
  }
  const uri = (context.extension && context.extension.extensionUri) || context.extensionUri;
  const fragment = uri && uri.fragment ? String(uri.fragment) : '';
  if (!fragment) return [];
  try {
    const parsed = JSON.parse(decodeURIComponent(fragment));
    return Array.isArray(parsed) ? parsed.map(name => String(name || '').trim()).filter(Boolean) : [];
  } catch (err) {
    return [];
  }
}

class LoopForgeHistoryProvider {
  constructor(runId, repoName) {
    this.runId = runId;
    this.repoName = repoName;
    this._log = null;
    this._emitter = new vscode.EventEmitter();
    this.onDidChangeCurrentHistoryItemGroup = this._emitter.event;
    this.currentHistoryItemGroup = {
      id: 'refs/heads/' + (repoName || 'HEAD'),
      name: repoName || 'HEAD'
    };
  }

  invalidate() {
    this._log = null;
    this._emitter.fire();
  }

  async load() {
    if (this._log) return this._log;
    const q = this.repoName && this.repoName !== 'workspace' ? `?repository=${encodeURIComponent(this.repoName)}` : '';
    try {
      this._log = await fetchJSON(`/api/runs/${encodeURIComponent(this.runId)}/workspace/log${q}`);
    } catch (err) {
      console.error('[loopforge] failed to load git log', this.repoName, err);
      this._log = { branch: this.repoName, commits: [] };
    }
    const branch = this._log.branch || this.repoName || 'HEAD';
    this.currentHistoryItemGroup = { id: 'refs/heads/' + branch, name: branch };
    return this._log;
  }

  toHistoryItem(commit) {
    return {
      id: commit.id,
      parentIds: Array.isArray(commit.parentIds) ? commit.parentIds : [],
      message: String(commit.message || ''),
      author: String(commit.author || ''),
      timestamp: Number(commit.timestamp) || 0
    };
  }

  async provideHistoryItems() {
    const log = await this.load();
    return (log.commits || []).map(commit => this.toHistoryItem(commit));
  }

  async provideHistoryItems2() {
    return this.provideHistoryItems();
  }

  async provideHistoryItemChanges(historyItemId, historyItemParentId) {
    const q = [
      this.repoName && this.repoName !== 'workspace' ? `repository=${encodeURIComponent(this.repoName)}` : '',
      `id=${encodeURIComponent(historyItemId)}`
    ].filter(Boolean).join('&');
    let files = [];
    try {
      const body = await fetchJSON(`/api/runs/${encodeURIComponent(this.runId)}/workspace/commit?${q}`);
      files = Array.isArray(body.files) ? body.files : [];
    } catch (err) {
      console.error('[loopforge] failed to load commit files', historyItemId, err);
    }
    return files.map(file => {
      const modified = workspaceUri(this.runId, file.path, 'loopforge').with({ query: gitRefQuery(historyItemId) });
      const original = workspaceUri(this.runId, file.path, 'loopforge').with({
        query: gitRefQuery(historyItemParentId || 'empty')
      });
      return { uri: modified, originalUri: original, modifiedUri: modified };
    });
  }

  async resolveHistoryItemGroupCommonAncestor() {
    const log = await this.load();
    const commits = Array.isArray(log.commits) ? log.commits : [];
    if (!commits.length) {
      return { id: 'HEAD', ahead: 0, behind: 0 };
    }
    return {
      id: commits[commits.length - 1].id,
      ahead: commits.length,
      behind: 0
    };
  }
}

function hideInput(scm) {
  if (!scm.inputBox) return;
  scm.inputBox.enabled = false;
  scm.inputBox.placeholder = '';
  if ('visible' in scm.inputBox) scm.inputBox.visible = false;
}

function createRepoSourceControl(context, runId, name) {
  const key = workspaceRepoName(name);
  const id = 'loopforge-' + key.replace(/[^\w]+/g, '-').replace(/^-+|-+$/g, '');
  const root = vscode.Uri.from({ scheme: 'loopforge', authority: runId, path: sourceControlRootPath(key) });
  const scm = vscode.scm.createSourceControl(id, 'Git', root);
  hideInput(scm);
  scm.quickDiffProvider = {
    provideOriginalResource(uri) {
      if (!uri || uri.scheme !== 'loopforge') return;
      return uri.with({ scheme: BASE_SCHEME, query: '' });
    }
  };
  let history = null;
  try {
    history = new LoopForgeHistoryProvider(runId, key);
    scm.historyProvider = history;
    history.invalidate();
  } catch (err) {
    console.error('[loopforge] history provider unavailable for', key, err);
  }
  context.subscriptions.push(scm);
  return { name: key, scm, history, index: null, workingTree: null, untracked: null };
}

function attachRepoGroups(control) {
  if (control.index) return control;
  control.index = control.scm.createResourceGroup('index', 'Staged Changes');
  control.workingTree = control.scm.createResourceGroup('workingTree', 'Changes');
  control.untracked = control.scm.createResourceGroup('untracked', 'Untracked Files');
  control.index.hideWhenEmpty = false;
  control.workingTree.hideWhenEmpty = false;
  control.untracked.hideWhenEmpty = false;
  return control;
}

function changesForRepo(changes, name) {
  const key = workspaceRepoName(name);
  return (changes || []).filter(item => changeRepositoryName(item) === key);
}

function changesSignature(changes) {
  return (changes || []).map(item => [
    String(item.path || ''),
    String(item.repository || ''),
    String(item.index || ' '),
    String(item.worktree || ' ')
  ].join('\t')).sort().join('\n');
}

function applyRepoChanges(control, runId, changes) {
  if (!control.index) attachRepoGroups(control);
  const indexStates = [];
  const workStates = [];
  const untrackedStates = [];
  for (const change of changes || []) {
    const state = changeResourceState(runId, change);
    const idx = String(change.index || ' ');
    const wt = String(change.worktree || ' ');
    if (idx === '?' || wt === '?') {
      untrackedStates.push(state);
      continue;
    }
    if (idx !== ' ') indexStates.push(state);
    if (wt !== ' ' || idx === ' ') workStates.push(state);
  }
  control.index.resourceStates = indexStates;
  control.workingTree.resourceStates = workStates;
  control.untracked.resourceStates = untrackedStates;
  control.index.hideWhenEmpty = true;
  control.workingTree.hideWhenEmpty = true;
  control.untracked.hideWhenEmpty = true;
  control.scm.count = indexStates.length + workStates.length + untrackedStates.length;
}

async function refreshWorkspace(runId, fs, baseFs, controls, decorations, options = {}) {
  const info = await fetchJSON(`/api/runs/${encodeURIComponent(runId)}/workspace`);
  if (!info.available) {
    throw workspaceGone(new Error(info.reason || '工作区不可用'));
  }
  const changes = Array.isArray(info.changes) ? info.changes : [];
  const force = !!options.force;
  const refreshHistory = force || options.history === true;
  if (!force && changesSignature(changes) === changesSignature(fs._changes)) {
    return info;
  }
  fs.invalidate();
  if (baseFs && typeof baseFs.invalidate === 'function') baseFs.invalidate();
  fs._changes = changes;
  for (const control of controls) {
    applyRepoChanges(control, runId, changesForRepo(changes, control.name));
    if (refreshHistory && control.history && typeof control.history.invalidate === 'function') {
      control.history.invalidate();
    }
  }
  decorations.fire(undefined);
  return info;
}

function startStatusAutoRefresh(context, runId, fs, baseFs, controls, decorations) {
  let inflight = null;
  let timer = 0;
  let disposed = false;
  const tick = (force) => {
    if (inflight) return inflight;
    inflight = refreshWorkspace(runId, fs, baseFs, controls, decorations, { force: !!force }).catch(err => {
      if (err && err.workspaceGone) {
        // The run finished and its workspace was cleaned; nothing will change.
        disposed = true;
        console.info('[loopforge] workspace no longer available:', err.message);
        return;
      }
      console.error('[loopforge] auto refresh failed', err);
    }).finally(() => {
      inflight = null;
    });
    return inflight;
  };
  const schedule = () => {
    if (disposed) return;
    timer = setTimeout(() => {
      Promise.resolve(tick(false)).finally(schedule);
    }, 3000);
  };
  tick(true).finally(schedule);
  context.subscriptions.push(new vscode.Disposable(() => {
    disposed = true;
    if (timer) clearTimeout(timer);
  }));
  if (vscode.window.onDidChangeWindowState) {
    context.subscriptions.push(vscode.window.onDidChangeWindowState(state => {
      if (state && state.focused) tick(false);
    }));
  }
}

function configuredChanges() {
  const parsed = folderQueryValue('changes');
  if (Array.isArray(parsed)) return parsed;
  const raw = vscode.workspace.getConfiguration('loopforge').get('changes');
  return Array.isArray(raw) ? raw : [];
}

function activate(context) {
  const folder = vscode.workspace.workspaceFolders && vscode.workspace.workspaceFolders[0];
  const runId = folder && folder.uri.authority ? decodeURIComponent(folder.uri.authority) : '';
  if (!runId) {
    vscode.window.showErrorMessage('缺少 Run ID，无法打开 SOP 工作区');
    return;
  }
  const names = repositoryNames(context);
  const initialChanges = configuredChanges();
  const controls = [];
  context.subscriptions.push(vscode.commands.registerCommand('loopforge.openChange', openChange));
  context.subscriptions.push(vscode.commands.registerCommand('loopforge.refresh', async () => {
    try {
      await refreshWorkspace(runId, fs, baseFs, controls, decorations, { force: true });
    } catch (err) {
      console.error('[loopforge] refresh failed', err);
      vscode.window.showErrorMessage('刷新工作区失败');
    }
  }));
  for (const name of (names.length ? names : ['workspace'])) {
    try {
      const control = attachRepoGroups(createRepoSourceControl(context, runId, name));
      applyRepoChanges(control, runId, changesForRepo(initialChanges, name));
      controls.push(control);
    } catch (err) {
      console.error('[loopforge] failed to register SCM for', name, err);
    }
  }
  const fs = new LoopForgeFS(runId);
  if (initialChanges.length) fs._changes = initialChanges;
  const baseFs = new LoopForgeBaseFS(runId);
  const decorations = new vscode.EventEmitter();
  context.subscriptions.push(decorations);
  context.subscriptions.push(vscode.workspace.registerFileSystemProvider('loopforge', fs, { isCaseSensitive: true, isReadonly: true }));
  context.subscriptions.push(vscode.workspace.registerFileSystemProvider(BASE_SCHEME, baseFs, { isCaseSensitive: true, isReadonly: true }));
  if (typeof vscode.workspace.registerFileSearchProvider === 'function') {
    context.subscriptions.push(vscode.workspace.registerFileSearchProvider('loopforge', {
      async provideFileSearchResults(query) {
        const needle = String(query.pattern || '').toLowerCase();
        const entries = await fs._entries();
        return entries.filter(item => item.type === 'file' && (!needle || item.path.toLowerCase().includes(needle))).map(item => vscode.Uri.from({ scheme: 'loopforge', authority: runId, path: '/' + item.path }));
      }
    }));
  }
  if (typeof vscode.workspace.registerTextSearchProvider === 'function') {
    context.subscriptions.push(vscode.workspace.registerTextSearchProvider('loopforge', {
      async provideTextSearchResults(query, options, progress) {
        const q = String(query.pattern || '').trim();
        if (q.length < 2) return { limitHit: false };
        const result = await fs._api(`/api/runs/${encodeURIComponent(runId)}/workspace/search?q=${encodeURIComponent(q)}`);
        for (const item of result.results || []) {
          progress.report({
            uri: vscode.Uri.from({ scheme: 'loopforge', authority: runId, path: '/' + item.path }),
            ranges: [new vscode.Range(Math.max(0, (item.line || 1) - 1), 0, Math.max(0, (item.line || 1) - 1), 1000)],
            preview: { text: item.preview || '', matches: [new vscode.Range(0, 0, 0, (item.preview || '').length)] }
          });
        }
        return { limitHit: !!result.truncated };
      }
    }));
  }
  context.subscriptions.push(vscode.window.registerFileDecorationProvider({
    onDidChangeFileDecorations: decorations.event,
    provideFileDecoration(uri) {
      if (uri.scheme !== 'loopforge') return;
      const path = uriPath(uri);
      const exact = fs.changeFor(path);
      if (exact) {
        const code = changeCode(exact);
        const deco = changeDecoration(code);
        return { badge: code, tooltip: deco.tooltip, color: deco.color, propagate: true };
      }
      if ((fs._changes || []).some(item => item.path.startsWith(path + '/'))) {
        return { badge: '•', tooltip: 'Contains Git changes', color: new vscode.ThemeColor('gitDecoration.modifiedResourceForeground'), propagate: false };
      }
    }
  }));
  decorations.fire(undefined);
  startStatusAutoRefresh(context, runId, fs, baseFs, controls, decorations);
  fs._entries().catch(err => {
    if (err && err.workspaceGone) {
      console.info('[loopforge] workspace no longer available:', err.message);
      return;
    }
    console.error('[loopforge] failed to load workspace tree', err);
  });
}

exports.activate = activate;
exports.deactivate = function () {};
exports.changeCode = changeCode;
exports.shouldDiffChange = shouldDiffChange;
exports.repositoriesFromChanges = repositoriesFromChanges;
exports.changesSignature = changesSignature;
