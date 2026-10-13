// 首页对话：项目的 PM 助理。提需求（追问澄清 → 开跑卡片）、问进度、处理待确认、
// 画图出文档都在这里说；可以带图片和文件，可以 @ 已有需求。右侧列出本项目正在跑的 SOP。
const HOME_CHAT_DEMO = 'demo-delivery';
const HOME_CHAT_QUICK = 'quick-fix';
const HOME_CHAT_STORE = 'loopforge.homeChat.v3';
const HOME_CHAT_MAX_FILES = 10;
const HOME_CHAT_MAX_BYTES = 20 * 1024 * 1024;
const HOME_CHAT_SUGGESTIONS = [
  { label: '提一个新需求', text: '我想加个功能：' },
  { label: '现在有哪些在跑，卡在哪', text: '现在项目里有哪些 SOP 在跑？分别卡在哪一步，需要我做什么？' },
  { label: '@ 一条需求，把它跑起来', text: '@' },
  { label: '画一张交付流程图', text: '把这个项目从提需求到交付的流程画成一张图' },
];
const HOME_RUN_ACTIVE = new Set(['queued', 'pending', 'running', 'waiting', 'waiting_approval', 'paused', 'retrying']);
let homeRunsTimer = null;
const homeMermaidCache = new Map();
let homeMermaidLoading = null;

function homeChatHasModelKey() {
  return !!(userById(state.currentUser?.id) || {}).codeBuddyApiKeyConfigured;
}

function homeChatWorkflows() {
  const projectId = state.projectFilter || '';
  const all = state.data.workflows || [];
  const mine = all.filter(w => !catalogGeneric(w) && (!projectId || w.projectId === projectId) && w.status !== 'archived');
  const list = [];
  [HOME_CHAT_QUICK, HOME_CHAT_DEMO].forEach(id => { const w = all.find(x => x.id === id); if (w) list.push(w); });
  mine.forEach(w => { if (!list.some(x => x.id === w.id)) list.push(w); });
  return list;
}

function homeChatStoreKey() {
  return `${HOME_CHAT_STORE}:${state.currentUser?.id || 0}:${state.projectFilter || ''}`;
}

function ensureHomeChatState() {
  const key = homeChatStoreKey();
  if (!state.homeChat || state.homeChat.key !== key) {
    let saved = null;
    try { saved = JSON.parse(localStorage.getItem(key) || 'null'); } catch (e) {}
    state.homeChat = { messages: [], sessionId: '', workflowId: '', draft: '', notes: [], ...(saved || {}), key, busy: false, live: null, files: [], refs: [], _abort: null };
  }
  return state.homeChat;
}

function persistHomeChat() {
  const chat = state.homeChat;
  if (!chat) return;
  try {
    localStorage.setItem(chat.key, JSON.stringify({
      messages: chat.messages.slice(-80), sessionId: chat.sessionId, workflowId: chat.workflowId, draft: chat.draft, notes: chat.notes,
    }));
  } catch (e) {}
}

function homeChatSelectedWorkflow() {
  const chat = ensureHomeChatState();
  const workflows = homeChatWorkflows();
  if (!chat.workflowId || !workflows.some(w => w.id === chat.workflowId)) {
    const preferred = homeChatHasModelKey() ? [HOME_CHAT_QUICK, HOME_CHAT_DEMO] : [HOME_CHAT_DEMO, HOME_CHAT_QUICK];
    chat.workflowId = (preferred.map(id => workflows.find(w => w.id === id)).find(Boolean) || workflows[0])?.id || '';
  }
  return workflows.find(w => w.id === chat.workflowId) || null;
}

function homeChatWorkflowTag(w) {
  if (w.id === HOME_CHAT_DEMO) return '演示 · 不调模型';
  if (w.id === HOME_CHAT_QUICK) return '短链路';
  return w.status === 'published' ? '' : '草稿';
}

function homeChatLatestReply() {
  const msgs = ensureHomeChatState().messages;
  for (let i = msgs.length - 1; i >= 0; i--) {
    if (msgs[i].kind === 'reply') return msgs[i];
    if (msgs[i].role === 'user') return null;
  }
  return null;
}

function homeChatIssues() {
  const projectId = state.projectFilter || '';
  return (state.data.issues || []).filter(i => !projectId || i.projectId === projectId);
}

function homeIssueLabel(issue) {
  return `${issue.number ? '#' + issue.number + ' ' : ''}${issue.title || issue.id}`;
}

function homeFileURL(id, download) {
  return `/api/assistant/files/${encodeURIComponent(id)}${download ? '?download=1' : ''}`;
}

function homeFileIsImage(f) {
  return /^image\/(png|jpe?g|gif|webp|svg\+xml)$/i.test(f?.mime || f?.mimeType || '');
}

function homeFileSize(n) {
  n = Number(n) || 0;
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${Math.round(n / 1024)} KB`;
  return `${n} B`;
}

// ---- 渲染 ----

function renderHomeChat() {
  const host = $('#homeChat');
  if (!host) return;
  const user = state.currentUser;
  if (!user || user.isGuest) {
    host.innerHTML = '';
    return;
  }
  const chat = ensureHomeChatState();
  if (!host.querySelector('.hc-layout') || host.dataset.key !== chat.key) {
    host.dataset.key = chat.key;
    host.innerHTML = `<div class="hc-layout${localStorage.getItem('loopforge.homeRuns.collapsed') === '1' ? ' runs-collapsed' : ''}">
      <section class="hc-main" id="hcMain"></section>
      <aside class="hc-runs" id="hcRuns" aria-label="正在运行的 SOP"></aside>
    </div>`;
  }
  renderHomeChatMain();
  renderHomeRunsPanel();
  startHomeRunsPolling();
}

function renderHomeChatMain() {
  const main = $('#hcMain');
  if (!main) return;
  const chat = ensureHomeChatState();
  const empty = !chat.messages.length && !chat.busy;
  main.classList.toggle('is-empty', empty);
  main.innerHTML = empty ? homeChatHeroHTML() : `
    <header class="hc-head">
      <div><b>${escapeHTML(homeChatTitle())}</b><span>${escapeHTML(homeChatSubtitle())}</span></div>
      <div class="hc-head-actions">
        ${homeChatHistoryHTML()}
        <button type="button" class="button ghost small" id="homeChatNew">＋ 新对话</button>
      </div>
    </header>
    <div class="hc-thread" id="homeChatThread">${chat.messages.map(homeChatMessageHTML).join('')}${chat.busy ? homeChatLiveHTML() : ''}</div>
    <div class="hc-dock">${homeChatComposerHTML()}</div>`;
  bindHomeChat();
  renderHomeMermaid(main);
  const thread = $('#homeChatThread');
  if (thread) thread.scrollTop = thread.scrollHeight;
}

function homeChatHistoryHTML() {
  return '<div class="hc-history-wrap"><button type="button" class="button ghost small" id="homeChatHistory" aria-haspopup="menu">历史</button><div class="hc-history" id="homeChatHistoryMenu" hidden></div></div>';
}

function homeChatTitle() {
  const first = ensureHomeChatState().messages.find(m => m.role === 'user' && m.text);
  return first ? first.text.split('\n')[0].slice(0, 60) : '新对话';
}

function homeChatSubtitle() {
  const project = currentProject();
  return project ? `项目 · ${project.name || project.id}` : '未选择项目 · 只能问答';
}

function homeChatHeroHTML() {
  const workflows = homeChatWorkflows();
  const workflow = homeChatSelectedWorkflow();
  const keyHint = homeChatHasModelKey() ? '' : `<p class="hc-key-hint">还没配置 CodeBuddy Key，我暂时没法读代码、查数据、追问需求。<button type="button" class="link-button" data-chat-action="settings-models">去配置</button>，或<button type="button" class="link-button" data-chat-action="demo">先跑一遍演示</button>（不需要 Key）。</p>`;
  const cards = workflows.map(w => {
    const steps = (w.nodes || []).length;
    const tag = homeChatWorkflowTag(w);
    return `<button type="button" class="hc-sop${w.id === workflow?.id ? ' is-selected' : ''}" data-home-sop="${escapeHTML(w.id)}" aria-pressed="${w.id === workflow?.id}">
      <span class="hc-sop-flow" aria-hidden="true"><i></i><em></em><i class="is-main"></i><em></em><i></i></span>
      <b>${escapeHTML(w.name || w.id)}</b>
      <small>${escapeHTML(w.description || (steps ? `${steps} 个步骤` : '自定义 SOP'))}</small>
      ${tag ? `<span class="hc-sop-tag">${escapeHTML(tag)}</span>` : ''}
    </button>`;
  }).join('');
  return `<div class="hc-hero-tools">${homeChatHistoryHTML()}</div>
  <div class="hc-hero">
    <h1 class="hc-brand">LoopForge</h1>
    <p class="hc-sub">提需求、问进度、处理待确认、画图出文档，都在这里说。需求说清了，我按 SOP 帮你开跑。</p>
    ${homeChatComposerHTML()}
    ${keyHint}
    <div class="hc-suggest">${HOME_CHAT_SUGGESTIONS.map((s, i) => `<button type="button" class="hc-chip" data-home-suggest="${i}">${escapeHTML(s.label)}</button>`).join('')}</div>
    ${workflows.length ? `<div class="hc-sop-head">开跑默认用的 SOP</div><div class="hc-sop-rail" role="list">${cards}</div>` : '<p class="hc-key-hint">这个项目还没有 SOP。<button type="button" class="link-button" data-home-goto="templates">从模板创建一个</button></p>'}
  </div>`;
}

function homeChatComposerHTML() {
  const chat = ensureHomeChatState();
  const latest = homeChatLatestReply();
  const asking = latest && (latest.questions || []).length && !latest.answered;
  const placeholder = !chat.messages.length ? '说说要做什么，@ 可以引用需求，也可以粘贴截图' : asking ? '点选上面的选项，或直接打字回答' : '继续说，@ 引用需求，或粘贴图片和文件';
  const uploading = chat.files.some(f => f.uploading);
  const tray = chat.files.length || chat.refs.length ? `<div class="hc-tray">
      ${chat.refs.map((r, i) => `<span class="hc-ref-chip" title="${escapeHTML(r.title)}">@${escapeHTML(r.label || r.title)}<button type="button" data-home-unref="${i}" aria-label="移除引用">×</button></span>`).join('')}
      ${chat.files.map((f, i) => `<span class="hc-file-chip${f.uploading ? ' is-uploading' : ''}${f.error ? ' is-error' : ''}" title="${escapeHTML(f.error || f.name)}">
        ${homeFileIsImage(f) && f.id ? `<img src="${homeFileURL(f.id)}" alt="">` : `<i class="hc-file-ext">${escapeHTML((f.name.split('.').pop() || 'FILE').slice(0, 4).toUpperCase())}</i>`}
        <span>${escapeHTML(f.name)}</span><small>${f.uploading ? '上传中…' : f.error ? '失败' : homeFileSize(f.size)}</small>
        <button type="button" data-home-unfile="${i}" aria-label="移除附件">×</button></span>`).join('')}
    </div>` : '';
  return `<form class="hc-composer" id="homeChatForm">
    ${tray}
    <div class="hc-mention" id="homeMention" hidden role="listbox"></div>
    <textarea id="homeChatInput" rows="${chat.messages.length ? 2 : 3}" maxlength="8000" placeholder="${escapeHTML(placeholder)}" ${chat.busy ? 'disabled' : ''}>${escapeHTML(chat.draft || '')}</textarea>
    <div class="hc-composer-bar">
      <button type="button" class="hc-tool" id="homeChatAttach" title="添加图片或文件（也可以直接粘贴、拖进来）" aria-label="添加图片或文件" ${chat.busy ? 'disabled' : ''}>＋</button>
      <button type="button" class="hc-tool" id="homeChatAt" title="引用一条需求" aria-label="引用一条需求" ${chat.busy ? 'disabled' : ''}>@</button>
      <input type="file" id="homeChatFile" multiple hidden>
      ${typeof clarifyModelPickerHTML === 'function' ? clarifyModelPickerHTML(chat.busy) : ''}
      <span class="hc-composer-hint">${chat.busy ? '正在处理…' : uploading ? '附件上传中…' : 'Enter 发送 · Shift+Enter 换行'}</span>
      ${chat.busy
        ? '<button type="button" class="hc-send is-stop" id="homeChatStop" title="停止" aria-label="停止">■</button>'
        : `<button type="submit" class="hc-send" id="homeChatSend" title="发送" aria-label="发送" ${uploading ? 'disabled' : ''}>↑</button>`}
    </div>
  </form>`;
}

function homeChatMessageHTML(m, index) {
  if (m.role === 'user') return homeChatUserHTML(m);
  if (m.kind === 'system') return `<div class="hc-system">${escapeHTML(m.text)}</div>`;
  const body = m.kind === 'reply' ? homeChatReplyHTML(m, index)
    : m.kind === 'launched' ? homeChatLaunchedHTML(m)
    : `<p>${escapeHTML(m.text || '')}</p>${homeChatActionsHTML(m.actions)}`;
  return `<div class="hc-msg is-bot${m.kind === 'error' ? ' is-error' : ''}"><span class="hc-avatar" aria-hidden="true">LF</span><div class="hc-bubble">${body}</div></div>`;
}

function homeChatUserHTML(m) {
  const refs = (m.refs || []).map(r => `<button type="button" class="hc-ref-chip is-link" data-open-created="${escapeHTML(r.id)}">@${escapeHTML(r.title || r.id)}</button>`).join('');
  const files = homeChatFilesHTML(m.attachments || [], true);
  const text = m.text ? `<div class="hc-bubble">${escapeHTML(m.text).replace(/\n/g, '<br>')}</div>` : '';
  return `<div class="hc-msg is-user"><div class="hc-user-stack">${refs ? `<div class="hc-user-refs">${refs}</div>` : ''}${files}${text}</div></div>`;
}

function homeChatFilesHTML(files, compact) {
  if (!files.length) return '';
  const images = files.filter(homeFileIsImage);
  const others = files.filter(f => !homeFileIsImage(f));
  return `<div class="hc-files${compact ? ' is-compact' : ''}">
    ${images.map(f => `<a class="hc-image" href="${homeFileURL(f.id)}" target="_blank" rel="noopener" title="${escapeHTML(f.name)}"><img src="${homeFileURL(f.id)}" alt="${escapeHTML(f.name)}" loading="lazy"></a>`).join('')}
    ${others.map(f => `<a class="hc-file" href="${homeFileURL(f.id, true)}" download="${escapeHTML(f.name)}"><i class="hc-file-ext">${escapeHTML((f.name.split('.').pop() || 'FILE').slice(0, 4).toUpperCase())}</i><span><b>${escapeHTML(f.name)}</b><small>${homeFileSize(f.size)} · 下载</small></span></a>`).join('')}
  </div>`;
}

function homeChatActionsHTML(actions) {
  if (!actions || !actions.length) return '';
  return `<div class="hc-actions">${actions.map(a => `<button type="button" class="button ${a.primary ? 'primary' : 'ghost'} small" data-chat-action="${escapeHTML(a.id)}">${escapeHTML(a.label)}</button>`).join('')}</div>`;
}

// mermaid 代码块先占位，渲染完 DOM 后再异步画成图。
function homeMarkdownHTML(text) {
  const parts = String(text || '').split(/```mermaid[ \t]*\n([\s\S]*?)```/);
  return parts.map((part, i) => i % 2
    ? `<div class="hc-mermaid" data-mermaid="${escapeHTML(part.trim())}"><pre>${escapeHTML(part.trim())}</pre></div>`
    : (part.trim() ? renderMarkdown(part) : '')).join('');
}

function homeChatReplyHTML(m, index) {
  const latest = homeChatLatestReply() === m;
  const answers = m.answers || {};
  const locked = !latest || m.answered;
  const questions = (m.questions || []).map((q, qi) => {
    const qid = q.id || `q${qi + 1}`;
    const picked = answers[qid] || [];
    const options = typeof clarifyQuestionOptions === 'function' ? clarifyQuestionOptions(q) : (q.options || []);
    const chips = options.map(opt => `<button type="button" class="hc-opt${picked.includes(opt) ? ' is-picked' : ''}" data-q="${escapeHTML(qid)}" data-opt="${escapeHTML(opt)}" ${locked ? 'disabled' : ''}>${escapeHTML(opt)}</button>`).join('');
    return `<li class="hc-q"><b>${escapeHTML(q.question)}</b>${q.multiSelect ? '<small>可多选</small>' : ''}${q.freeText && !locked ? '<small>在下面输入框里回答</small>' : ''}${chips ? `<div class="hc-opts">${chips}</div>` : ''}</li>`;
  }).join('');
  const trace = (m.trace || []).length ? `<details class="hc-trace-box"><summary>查了 ${m.trace.length} 处</summary><ul class="hc-trace">${m.trace.map(t => `<li>${escapeHTML(t)}</li>`).join('')}</ul></details>` : '';
  const drafts = (m.drafts || []).map((d, di) => homeChatDraftHTML(d, index, di)).join('');
  const launch = m.launch ? homeChatLaunchCardHTML(m, index) : (latest && (m.questions || []).length && !m.answered
    ? '<div class="hc-actions"><button type="button" class="link-button" data-chat-action="force-launch">信息够了，按现在说的开跑</button></div>' : '');
  return `${trace}<div class="hc-md">${homeMarkdownHTML(m.text)}</div>${homeChatFilesHTML(m.files || [])}${questions ? `<ol class="hc-questions">${questions}</ol>` : ''}${drafts}${launch}`;
}

function homeChatLaunchCardHTML(m, index) {
  const l = m.launch;
  const workflows = homeChatWorkflows();
  const workflowId = l.workflowId && workflows.some(w => w.id === l.workflowId) ? l.workflowId : homeChatSelectedWorkflow()?.id;
  if (l.done) {
    return `<div class="hc-launch is-done"><div class="hc-launch-head"><b>已开跑</b><span>${escapeHTML(l.title || '')}，进度在右侧。</span></div></div>`;
  }
  const issue = l.issueId ? (state.data.issues || []).find(i => i.id === l.issueId) : null;
  return `<div class="hc-launch" data-launch="${index}">
    <div class="hc-launch-head"><b>${issue ? '跑这条已有需求' : '需求已经说清，可以开跑'}</b><span>${issue ? `按选中的 SOP 派发「${escapeHTML(homeIssueLabel(issue))}」` : '建一张需求单并立即派发'}，进度在右侧看。</span></div>
    ${issue ? '' : `<label>需求标题<input data-launch-title value="${escapeHTML(l.title || '')}" maxlength="120"></label>`}
    <label>SOP<select data-launch-workflow>${workflows.map(w => `<option value="${escapeHTML(w.id)}" ${w.id === workflowId ? 'selected' : ''}>${escapeHTML(w.name || w.id)}</option>`).join('')}</select></label>
    ${l.description ? `<details class="hc-launch-desc"><summary>${issue ? '补充说明' : '澄清结论（会写进需求描述）'}</summary><div class="hc-md">${renderMarkdown(l.description)}</div></details>` : ''}
    <div class="hc-actions"><button type="button" class="button primary" data-launch-go>${issue ? '开跑' : '建单并开跑'}</button></div>
  </div>`;
}

function homeDraftKind(tool) {
  const labels = {
    issues_save: '保存需求', issues_dispatch: '派发 SOP', runs_start: '启动运行', runs_approve: '审批通过', runs_pass: '放行', runs_reject: '驳回',
    runs_cancel: '取消运行', runs_retry: '重试运行', workflows_save: '保存 SOP', workflows_publish: '发布 SOP', agents_save: '保存角色',
    schedule_jobs_save: '保存定时任务', assistant_tasks_save: '保存助手定时任务', issues_comments_add: '添加评论',
  };
  const danger = /(_delete|_publish|_dispatch|_start|_cancel|_reject|_run|_restore|_pass|_retry|_pause|_resume|_reopen|_approve)$/.test(tool) || tool === 'sync_start';
  return { label: labels[tool] || tool, danger };
}

function homeChatDraftHTML(d, index, di) {
  const kind = homeDraftKind(d.tool);
  const targetId = d.path?.id || d.body?.id || '';
  const issue = targetId ? (state.data.issues || []).find(i => i.id === targetId) : null;
  const title = d.body?.name || d.body?.title || (issue ? homeIssueLabel(issue) : targetId);
  const preview = { tool: d.tool, ...(d.path ? { path: d.path } : {}), ...(d.query ? { query: d.query } : {}), ...(d.body !== undefined ? { body: d.body } : {}) };
  const status = d.saved ? (d.auto ? '已自动执行' : '已执行') : d.saveError ? `执行失败：${d.saveError}` : (kind.danger ? '确认后立即执行' : '确认后保存');
  return `<div class="hc-draft${d.saved ? ' is-done' : ''}${kind.danger ? ' is-danger' : ''}">
    <div class="hc-draft-head"><b>${escapeHTML(kind.label)}${title ? ' · ' + escapeHTML(String(title)) : ''}</b><small>${escapeHTML(status)}</small></div>
    <details><summary>查看参数</summary><pre>${escapeHTML(JSON.stringify(preview, null, 2))}</pre></details>
    ${d.saved ? '' : `<div class="hc-actions"><button type="button" class="button ${kind.danger ? 'danger-outline' : 'primary'} small" data-draft="${index}:${di}">确认${escapeHTML(kind.label)}</button></div>`}
  </div>`;
}

function homeChatLaunchedHTML(m) {
  return `<p>已按「${escapeHTML(m.workflowName || m.workflowId || 'SOP')}」开跑：<b>${escapeHTML(m.title || '')}</b>。进度在右侧，需要你确认时会停下来等你。</p>
    <div class="hc-actions">${m.issueId ? `<button type="button" class="button ghost small" data-open-created="${escapeHTML(m.issueId)}">打开需求</button>` : ''}</div>`;
}

function homeChatLiveHTML() {
  const live = ensureHomeChatState().live || {};
  const tools = (live.tools || []).slice(-4);
  return `<div class="hc-msg is-bot is-thinking"><span class="hc-avatar" aria-hidden="true">LF</span><div class="hc-bubble" id="hcThinking">
    ${tools.length ? `<ul class="hc-trace">${tools.map(t => `<li class="${t.done ? 'is-done' : ''}">${escapeHTML(t.text)}</li>`).join('')}</ul>` : ''}
    ${live.answer ? `<div class="hc-md">${renderMarkdown(live.answer)}</div>` : ''}
    <div class="hc-typing"><i></i><i></i><i></i><span>${escapeHTML(live.status || '正在思考…')}</span></div>
  </div></div>`;
}

function homeChatPaintLive() {
  const box = $('#hcThinking');
  if (!box) return;
  const thread = $('#homeChatThread');
  const stick = thread && thread.scrollHeight - thread.scrollTop - thread.clientHeight < 120;
  box.closest('.hc-msg').outerHTML = homeChatLiveHTML();
  if (thread && stick) thread.scrollTop = thread.scrollHeight;
}

async function renderHomeMermaid(root) {
  const blocks = [...root.querySelectorAll('.hc-mermaid:not(.is-rendered)')];
  if (!blocks.length) return;
  try {
    if (!window.mermaid) {
      homeMermaidLoading = homeMermaidLoading || new Promise((resolve, reject) => {
        const s = document.createElement('script');
        s.src = '/vendor/mermaid/mermaid.min.js';
        s.onload = resolve;
        s.onerror = () => { homeMermaidLoading = null; reject(new Error('mermaid 加载失败')); };
        document.head.append(s);
      });
      await homeMermaidLoading;
      const dark = document.documentElement.dataset.theme === 'dark';
      window.mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', theme: dark ? 'dark' : 'neutral', fontFamily: 'inherit' });
    }
    for (const el of blocks) {
      const src = el.dataset.mermaid || '';
      let svg = homeMermaidCache.get(src);
      if (!svg) {
        const out = await window.mermaid.render('hcm' + Math.random().toString(36).slice(2, 9), src);
        svg = out.svg;
        homeMermaidCache.set(src, svg);
      }
      el.innerHTML = svg;
      el.classList.add('is-rendered');
    }
  } catch (e) {
    blocks.forEach(el => el.classList.add('is-rendered', 'is-failed'));
  }
}

// ---- 交互 ----

function bindHomeChat() {
  const chat = ensureHomeChatState();
  const main = $('#hcMain');
  if (!main) return;
  const input = $('#homeChatInput');
  if (input) {
    input.oninput = () => { chat.draft = input.value; persistHomeChat(); homeMentionUpdate(); };
    input.onkeydown = e => {
      if (homeMentionKey(e)) return;
      if (e.key !== 'Enter' || e.shiftKey || e.isComposing || e.keyCode === 229) return;
      e.preventDefault();
      homeChatSend();
    };
    input.onpaste = e => {
      const files = [...(e.clipboardData?.files || [])];
      if (files.length) { e.preventDefault(); homeChatAddFiles(files); }
    };
    input.onblur = () => setTimeout(() => homeMentionClose(), 150);
  }
  const form = $('#homeChatForm');
  if (form) {
    form.onsubmit = e => { e.preventDefault(); homeChatSend(); };
    form.ondragover = e => { if (e.dataTransfer?.types?.includes('Files')) { e.preventDefault(); form.classList.add('is-drop'); } };
    form.ondragleave = () => form.classList.remove('is-drop');
    form.ondrop = e => {
      form.classList.remove('is-drop');
      const files = [...(e.dataTransfer?.files || [])];
      if (files.length) { e.preventDefault(); homeChatAddFiles(files); }
    };
  }
  const fileInput = $('#homeChatFile');
  $('#homeChatAttach') && ($('#homeChatAttach').onclick = () => fileInput?.click());
  if (fileInput) fileInput.onchange = () => { homeChatAddFiles([...fileInput.files]); fileInput.value = ''; };
  $('#homeChatAt') && ($('#homeChatAt').onclick = () => homeMentionInsertAt());
  main.querySelectorAll('[data-home-unfile]').forEach(b => b.onclick = () => { chat.files.splice(Number(b.dataset.homeUnfile), 1); renderHomeChatMain(); });
  main.querySelectorAll('[data-home-unref]').forEach(b => b.onclick = () => { chat.refs.splice(Number(b.dataset.homeUnref), 1); renderHomeChatMain(); });
  main.querySelectorAll('[data-home-suggest]').forEach(b => b.onclick = () => {
    const s = HOME_CHAT_SUGGESTIONS[Number(b.dataset.homeSuggest)];
    const el = $('#homeChatInput');
    if (!el || !s) return;
    el.value = s.text;
    chat.draft = s.text;
    el.focus();
    el.setSelectionRange(s.text.length, s.text.length);
    homeMentionUpdate();
  });
  const pick = id => { chat.workflowId = id; persistHomeChat(); renderHomeChatMain(); };
  main.querySelectorAll('[data-home-sop]').forEach(b => b.onclick = () => pick(b.dataset.homeSop));
  $('#homeChatStop') && ($('#homeChatStop').onclick = () => homeChatStop());
  $('#homeChatNew') && ($('#homeChatNew').onclick = () => homeChatReset());
  $('#homeChatHistory') && ($('#homeChatHistory').onclick = e => { e.stopPropagation(); homeChatToggleHistory(); });
  main.querySelectorAll('.hc-opt').forEach(b => b.onclick = () => homeChatPickOption(b.dataset.q, b.dataset.opt));
  main.querySelectorAll('[data-chat-action]').forEach(b => b.onclick = () => homeChatAction(b.dataset.chatAction));
  main.querySelectorAll('[data-home-goto]').forEach(b => b.onclick = () => switchView(b.dataset.homeGoto));
  main.querySelectorAll('[data-open-created]').forEach(b => b.onclick = () => openIssueDetail(b.dataset.openCreated, 'sop'));
  main.querySelectorAll('[data-launch-go]').forEach(b => b.onclick = () => homeChatLaunch(b.closest('[data-launch]')));
  main.querySelectorAll('[data-draft]').forEach(b => b.onclick = () => homeChatConfirmDraft(b));
  main.querySelectorAll('[data-clarify-model]').forEach(el => typeof bindClarifyModelPicker === 'function' && bindClarifyModelPicker(el));
  main.querySelectorAll('.md-code-copy').forEach(b => b.onclick = () => {
    const code = b.closest('.md-code-block')?.querySelector('code')?.textContent || '';
    navigator.clipboard?.writeText(code).then(() => { b.textContent = '已复制'; setTimeout(() => { b.textContent = '复制'; }, 1200); }).catch(() => {});
  });
}

async function homeChatAddFiles(list) {
  const chat = ensureHomeChatState();
  const room = HOME_CHAT_MAX_FILES - chat.files.length;
  if (room <= 0) return toast(`一条消息最多带 ${HOME_CHAT_MAX_FILES} 个附件`, 'err');
  const picked = list.slice(0, room);
  if (list.length > room) toast(`一条消息最多带 ${HOME_CHAT_MAX_FILES} 个附件，多出的没有添加`, 'err');
  const accepted = [];
  for (const file of picked) {
    if (file.size > HOME_CHAT_MAX_BYTES) { toast(`${file.name} 超过 20 MB`, 'err'); continue; }
    const entry = { name: file.name || 'pasted.png', size: file.size, mime: file.type, uploading: true };
    chat.files.push(entry);
    accepted.push([file, entry]);
  }
  if (!accepted.length) return;
  renderHomeChatMain();
  const form = new FormData();
  accepted.forEach(([file, entry]) => form.append('files', file, entry.name));
  try {
    const resp = await fetch('/api/assistant/files', { method: 'POST', body: form });
    const data = await resp.json().catch(() => ({}));
    if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
    (data.files || []).forEach((meta, i) => { const entry = accepted[i]?.[1]; if (entry) Object.assign(entry, meta, { uploading: false }); });
  } catch (err) {
    accepted.forEach(([, entry]) => { entry.uploading = false; entry.error = err.message; });
    toast('上传失败：' + err.message, 'err');
  }
  if (activeViewName() === 'chat') renderHomeChatMain();
  $('#homeChatInput')?.focus();
}

// ---- @ 引用需求 ----

function homeMentionQuery() {
  const input = $('#homeChatInput');
  if (!input) return null;
  const before = input.value.slice(0, input.selectionStart ?? input.value.length);
  const match = before.match(/(^|[^A-Za-z0-9_.@])@([^\s@]{0,30})$/);
  return match ? { query: match[2], start: before.length - match[2].length - 1 } : null;
}

function homeMentionUpdate() {
  const box = $('#homeMention');
  if (!box) return;
  const q = homeMentionQuery();
  if (!q) return homeMentionClose();
  const needle = q.query.toLowerCase().replace(/^#/, '');
  const items = homeChatIssues()
    .filter(i => !needle || String(i.number || '').startsWith(needle) || (i.title || '').toLowerCase().includes(needle))
    .slice(0, 8);
  box.hidden = false;
  box.dataset.active = '0';
  box.innerHTML = items.length
    ? `<div class="hc-mention-head">引用需求</div>${items.map((i, n) => `<button type="button" class="hc-mention-item${n === 0 ? ' is-active' : ''}" data-mention="${escapeHTML(i.id)}" role="option"><b>${escapeHTML(homeIssueLabel(i))}</b><small>${escapeHTML(typeof issueStatusLabel === 'function' ? issueStatusLabel(i.status) : (i.status || ''))}</small></button>`).join('')}`
    : `<div class="hc-mention-empty">${homeChatIssues().length ? '没有匹配的需求' : '这个项目还没有需求，直接说要做什么就行'}</div>`;
  box.querySelectorAll('[data-mention]').forEach(b => b.onmousedown = e => { e.preventDefault(); homeMentionPick(b.dataset.mention); });
}

function homeMentionClose() {
  const box = $('#homeMention');
  if (box) { box.hidden = true; box.innerHTML = ''; }
}

function homeMentionKey(e) {
  const box = $('#homeMention');
  if (!box || box.hidden) return false;
  const items = [...box.querySelectorAll('[data-mention]')];
  if (e.key === 'Escape') { e.preventDefault(); homeMentionClose(); return true; }
  if (!items.length) return false;
  let active = Number(box.dataset.active || 0);
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault();
    active = (active + (e.key === 'ArrowDown' ? 1 : items.length - 1)) % items.length;
    box.dataset.active = String(active);
    items.forEach((b, i) => b.classList.toggle('is-active', i === active));
    return true;
  }
  if ((e.key === 'Enter' || e.key === 'Tab') && !e.isComposing) {
    e.preventDefault();
    homeMentionPick(items[active].dataset.mention);
    return true;
  }
  return false;
}

function homeMentionPick(issueId) {
  const chat = ensureHomeChatState();
  const issue = (state.data.issues || []).find(i => i.id === issueId);
  const input = $('#homeChatInput');
  const q = homeMentionQuery();
  if (!issue || !input) return;
  const label = homeIssueLabel(issue).slice(0, 40);
  if (q) {
    const caret = input.selectionStart ?? input.value.length;
    input.value = input.value.slice(0, q.start) + input.value.slice(caret);
    input.setSelectionRange(q.start, q.start);
  }
  if (!chat.refs.some(r => r.id === issue.id)) chat.refs.push({ type: 'issue', id: issue.id, title: issue.title, label });
  chat.draft = input.value;
  homeMentionClose();
  renderHomeChatMain();
  const el = $('#homeChatInput');
  if (el) { el.focus(); el.setSelectionRange(el.value.length, el.value.length); }
}

// 从需求详情跳过来：带上这条需求的 @ 引用，光标停在输入框里。
function homeChatMentionIssue(issueId) {
  const issue = (state.data.issues || []).find(i => i.id === issueId);
  if (!issue) return switchView('chat');
  if (issue.projectId && issue.projectId !== state.projectFilter) {
    state.projectFilter = issue.projectId;
    localStorage.setItem('loopforge.activeProject', issue.projectId);
    if (typeof renderProjectSwitcher === 'function') renderProjectSwitcher();
  }
  const chat = ensureHomeChatState();
  if (!chat.refs.some(r => r.id === issue.id)) chat.refs.push({ type: 'issue', id: issue.id, title: issue.title, label: homeIssueLabel(issue).slice(0, 40) });
  switchView('chat');
  setTimeout(() => $('#homeChatInput')?.focus(), 50);
}

function homeMentionInsertAt() {
  const input = $('#homeChatInput');
  if (!input) return;
  const pos = input.selectionStart ?? input.value.length;
  const pad = pos && !/\s$/.test(input.value.slice(0, pos)) ? ' @' : '@';
  input.value = input.value.slice(0, pos) + pad + input.value.slice(pos);
  input.focus();
  input.setSelectionRange(pos + pad.length, pos + pad.length);
  ensureHomeChatState().draft = input.value;
  homeMentionUpdate();
}

// ---- 发送 ----

function homeChatPickOption(qid, opt) {
  const m = homeChatLatestReply();
  if (!m || m.answered) return;
  const q = (m.questions || []).find((x, i) => (x.id || `q${i + 1}`) === qid);
  m.answers = m.answers || {};
  const picked = m.answers[qid] || [];
  if (picked.includes(opt)) m.answers[qid] = picked.filter(x => x !== opt);
  else m.answers[qid] = q?.multiSelect ? [...picked, opt] : [opt];
  persistHomeChat();
  renderHomeChatMain();
  $('#homeChatInput')?.focus();
}

function homeChatAction(id) {
  if (id === 'settings-models') return openSettings('models');
  if (id === 'settings-git') return openSettings('git');
  if (id === 'demo') return homeChatRunDemo();
  if (id === 'retry') return homeChatRetry();
  if (id === 'force-launch') {
    const m = homeChatLatestReply();
    if (!m) return;
    const said = ensureHomeChatState().messages.filter(x => x.role === 'user' && x.text).map(x => x.text);
    m.launch = { title: (said[0] || '新需求').split('\n')[0].slice(0, 80), description: `${said.join('\n\n')}\n\n## 讨论结论\n\n${m.text || ''}`.trim() };
    persistHomeChat();
    renderHomeChatMain();
  }
}

function homeChatReset() {
  const chat = ensureHomeChatState();
  if (chat.busy) homeChatStop(true);
  Object.assign(chat, { messages: [], sessionId: '', draft: '', notes: [], files: [], refs: [] });
  persistHomeChat();
  renderHomeChatMain();
  $('#homeChatInput')?.focus();
}

function homeChatPush(message) {
  const chat = ensureHomeChatState();
  chat.messages.push({ at: Date.now(), ...message });
  persistHomeChat();
}

function homeChatNote(text) {
  const chat = ensureHomeChatState();
  chat.notes = [...(chat.notes || []), text].slice(-6);
  persistHomeChat();
}

// 选中的选项和输入框里的文字拼成一条回答。
function homeChatCollectText() {
  const typed = ($('#homeChatInput')?.value || '').trim();
  const m = homeChatLatestReply();
  const parts = [];
  if (m && !m.answered) {
    (m.questions || []).forEach((q, i) => {
      const picked = (m.answers || {})[q.id || `q${i + 1}`] || [];
      if (picked.length) parts.push(`${q.question}：${picked.join('、')}`);
    });
  }
  if (typed) parts.push(typed);
  return parts.join('\n');
}

// 发给助手的历史：用户消息带附件和引用，助手回合是 JSON（和 AI 助手的会话格式一致）。
function homeChatServerMessages() {
  return ensureHomeChatState().messages.filter(m => m.role === 'user' || m.kind === 'reply').map(m => m.role === 'user'
    ? { role: 'user', content: m.content || m.text || '（见附件）', ...(m.attachments?.length ? { attachments: m.attachments.map(a => ({ id: a.id, name: a.name, mime: a.mime, size: a.size })) } : {}), ...(m.refs?.length ? { refs: m.refs.map(r => ({ type: r.type || 'issue', id: r.id, title: r.title })) } : {}) }
    : { role: 'assistant', content: JSON.stringify({ message: m.text || '', drafts: m.drafts || [], ...(m.questions?.length ? { questions: m.questions } : {}), ...(m.launch ? { launch: m.launch } : {}), ...(m.files?.length ? { files: m.files } : {}) }) });
}

async function homeChatSend() {
  const chat = ensureHomeChatState();
  if (chat.busy || chat.files.some(f => f.uploading)) return;
  const text = homeChatCollectText();
  const files = chat.files.filter(f => f.id && !f.error);
  if (!text && !files.length) { $('#homeChatInput')?.focus(); return; }
  const latest = homeChatLatestReply();
  if (latest && (latest.questions || []).length) latest.answered = true;
  const notes = chat.notes || [];
  const message = {
    role: 'user', text,
    content: notes.length ? `（平台记录：${notes.join('；')}）\n\n${text || '（见附件）'}` : (text || '（见附件）'),
    attachments: files.map(f => ({ id: f.id, name: f.name, mime: f.mime, size: f.size })),
    refs: chat.refs.map(r => ({ type: 'issue', id: r.id, title: r.title })),
  };
  Object.assign(chat, { draft: '', files: [], refs: [], notes: [] });
  homeChatPush(message);
  if (!homeChatHasModelKey()) {
    homeChatPush({ role: 'assistant', kind: 'notice', text: '还没配置 CodeBuddy Key，我暂时没法读代码、查数据、追问需求。配好后再发一次就行；也可以先跑一遍演示 SOP 看完整流程，它不调模型。', actions: [{ id: 'settings-models', label: '去配置模型 Key', primary: true }, { id: 'demo', label: '先跑演示' }] });
    return renderHomeChatMain();
  }
  await homeChatAsk();
}

function homeChatRetry() {
  const chat = ensureHomeChatState();
  if (chat.messages[chat.messages.length - 1]?.kind === 'error') chat.messages.pop();
  if (chat.messages[chat.messages.length - 1]?.role !== 'user') return;
  homeChatAsk();
}

async function homeChatAsk() {
  const chat = ensureHomeChatState();
  const project = currentProject();
  chat.busy = true;
  chat.live = { status: '正在思考…', tools: [], answer: '' };
  renderHomeChatMain();
  const controller = new AbortController();
  chat._abort = controller;
  try {
    const resp = await fetch('/api/assistant/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
      body: JSON.stringify({
        projectId: project?.id || '',
        sessionId: chat.sessionId,
        model: typeof selectedClarifyModel === 'function' ? selectedClarifyModel() : undefined,
        approvalMode: 'confirm',
        scene: 'home',
        messages: homeChatServerMessages(),
      }),
      signal: controller.signal,
    });
    if (!resp.ok) {
      let em = `HTTP ${resp.status}`;
      try { const d = await resp.json(); em = d.error || em; } catch (e) {}
      throw new Error(em);
    }
    const reply = resp.headers.get('Content-Type')?.includes('text/event-stream') ? await homeChatReadStream(resp) : await resp.json();
    if (reply?.error) throw new Error(reply.error);
    chat.sessionId = reply.sessionId || chat.sessionId;
    homeChatPush({
      role: 'assistant', kind: 'reply', text: reply.message || '',
      questions: reply.questions || [], answers: {}, launch: reply.launch || null,
      files: (reply.files || []).map(f => ({ id: f.id, name: f.name, mime: f.mimeType || f.mime, size: f.size })),
      drafts: reply.drafts || [], trace: (chat.live?.tools || []).map(t => t.text).slice(-12),
    });
    if ((reply.drafts || []).some(d => d.saved)) refreshAfterHomeWrite();
  } catch (err) {
    if (err?.name !== 'AbortError') {
      const message = err?.message || '请求失败';
      const credential = /401|API Key|CodeBuddy|凭据|Unauthorized/i.test(message);
      homeChatPush({
        role: 'assistant', kind: 'error', text: `这一轮没回答成：${message}`,
        actions: credential ? [{ id: 'settings-models', label: '去个人设置检查 Key', primary: true }, { id: 'retry', label: '重试' }] : [{ id: 'retry', label: '重试', primary: true }],
      });
    }
  } finally {
    if (chat._abort === controller) {
      chat._abort = null;
      chat.busy = false;
      chat.live = null;
      persistHomeChat();
      if (activeViewName() === 'chat') {
        renderHomeChatMain();
        $('#homeChatInput')?.focus();
      }
    }
  }
}

async function homeChatReadStream(resp) {
  const chat = ensureHomeChatState();
  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '', result = null, error = null;
  for (;;) {
    const next = await reader.read();
    if (next.done) break;
    buffer += decoder.decode(next.value, { stream: true });
    let idx;
    while ((idx = buffer.indexOf('\n\n')) >= 0) {
      const chunk = buffer.slice(0, idx);
      buffer = buffer.slice(idx + 2);
      let event = 'message', data = '';
      chunk.split('\n').forEach(line => {
        if (line.startsWith('event:')) event = line.slice(6).trim();
        else if (line.startsWith('data:')) data += (data ? '\n' : '') + line.slice(5).trimStart();
      });
      if (!data) continue;
      let payload = {};
      try { payload = JSON.parse(data); } catch (e) { continue; }
      if (event === 'result') result = payload;
      else if (event === 'error') error = payload.error || '生成失败';
      else if (event === 'thought') homeChatThought(payload);
    }
  }
  if (error) throw new Error(error);
  if (!result) throw new Error('回答中断了，请重试');
  return result;
}

function homeChatThought(ev) {
  const live = ensureHomeChatState().live;
  if (!live) return;
  const tool = String(ev.tool || '').replace(/^mcp__loopforge_session__/, '');
  switch (ev.kind) {
    case 'answer':
      live.answer += ev.text || '';
      break;
    case 'tool':
      if (tool === 'submit_assistant_reply') { live.status = '正在整理回答…'; break; }
      live.tools.push({ id: ev.id || '', text: ev.text || (typeof clarifyToolLabel === 'function' ? clarifyToolLabel(tool) : tool) || '调用工具' });
      live.status = '正在查…';
      break;
    case 'toolDone':
      live.tools.forEach(t => { if (t.id && t.id === ev.id) t.done = true; });
      break;
    case 'thinking': case 'thinkingStart':
      live.status = '正在思考…';
      break;
    default:
      if (ev.text) live.status = ev.text;
  }
  homeChatPaintLive();
}

function homeChatStop(silent) {
  const chat = ensureHomeChatState();
  chat._abort?.abort();
  chat._abort = null;
  chat.busy = false;
  chat.live = null;
  if (!silent) {
    homeChatPush({ role: 'assistant', kind: 'notice', text: '已停止。改一下说法再发，或点重试。', actions: [{ id: 'retry', label: '重试' }] });
    renderHomeChatMain();
  }
}

// ---- 草稿、开跑、演示 ----

async function homeChatConfirmDraft(button) {
  const [mi, di] = button.dataset.draft.split(':').map(Number);
  const chat = ensureHomeChatState();
  const draft = chat.messages[mi]?.drafts?.[di];
  if (!draft || draft.saved) return;
  const kind = homeDraftKind(draft.tool);
  if (kind.danger && !await afConfirm({ title: kind.label, message: `将执行 ${draft.tool}。这会立即改动平台上的运行或资源。`, danger: true, confirmText: '确认执行' })) return;
  button.disabled = true;
  button.textContent = '正在执行…';
  try {
    const args = {};
    if (draft.path && Object.keys(draft.path).length) args.path = draft.path;
    if (draft.query && Object.keys(draft.query).length) args.query = draft.query;
    if (draft.body !== undefined && draft.body !== null) args.body = draft.body;
    const res = await api('/mcp', { method: 'POST', body: JSON.stringify({ jsonrpc: '2.0', id: Date.now(), method: 'tools/call', params: { name: draft.tool, arguments: args } }) });
    if (res.error || res.result?.isError) throw new Error(res.error?.message || res.result?.content?.[0]?.text || '执行失败');
    const saved = res.result?.structuredContent?.data || {};
    draft.saved = true;
    draft.saveError = '';
    homeChatNote(`用户已确认执行 ${draft.tool}${saved.id ? '（' + saved.id + '）' : ''}`);
    refreshAfterHomeWrite();
  } catch (err) {
    draft.saveError = String(err.message || '').slice(0, 200);
  }
  persistHomeChat();
  renderHomeChatMain();
}

async function refreshAfterHomeWrite() {
  try { await Promise.all([refreshRuns(true), refreshIssues()]); } catch (e) {}
  renderHomeRunsPanel();
}

async function homeChatLaunch(card) {
  if (!card) return;
  const chat = ensureHomeChatState();
  const m = chat.messages[Number(card.dataset.launch)];
  const l = m?.launch;
  const project = currentProject() || (state.data.projects || [])[0];
  if (!l || !project) return toast('请先选择项目');
  const workflowId = card.querySelector('[data-launch-workflow]')?.value || homeChatSelectedWorkflow()?.id;
  const workflow = homeChatWorkflows().find(w => w.id === workflowId);
  if (!workflowId) return toast('这个项目还没有可用的 SOP');
  const button = card.querySelector('[data-launch-go]');
  if (button) { button.disabled = true; button.textContent = l.issueId ? '正在派发…' : '正在建单…'; }
  let issue = l.issueId ? (state.data.issues || []).find(i => i.id === l.issueId) : null;
  try {
    if (!issue) {
      const title = (card.querySelector('[data-launch-title]')?.value || '').trim() || l.title || homeChatTitle();
      issue = await api('/api/issues', {
        method: 'POST',
        body: JSON.stringify({ projectId: project.id, title, description: l.description || title, kind: 'requirement', priority: 'medium', status: 'todo', syncTarget: 'local', workflowId, assigneeUserId: Number(state.currentUser?.id) || 0 }),
      });
      rememberIssue(issue);
    }
  } catch (err) {
    if (button) { button.disabled = false; button.textContent = '建单并开跑'; }
    return toast('建单失败：' + err.message, 'err');
  }
  try {
    const task = await api(`/api/issues/${issue.id}/dispatch`, { method: 'POST', body: JSON.stringify({ workflowId }) });
    rememberIssue({ ...issue, latestTaskId: task.id, status: 'in_progress' });
    (state.data.tasks || (state.data.tasks = [])).unshift(task);
    l.done = true;
    l.title = issue.title;
    l.issueId = issue.id;
    homeChatPush({ role: 'assistant', kind: 'launched', title: issue.title, issueId: issue.id, runId: task.runId || '', workflowId, workflowName: workflow?.name || workflowId });
    homeChatNote(`用户已开跑需求「${issue.title}」（${issue.id}），SOP ${workflowId}，运行 ${task.runId || task.id}`);
  } catch (err) {
    l.issueId = issue.id;
    const credential = /Token|API Key|个人设置/.test(err.message);
    homeChatPush({ role: 'assistant', kind: 'error', text: `需求已建好，但没能开跑：${err.message}`, actions: credential ? [{ id: /Token/.test(err.message) ? 'settings-git' : 'settings-models', label: '去个人设置', primary: true }] : [] });
  }
  persistHomeChat();
  renderHomeChatMain();
  await refreshAfterHomeWrite();
}

async function homeChatRunDemo() {
  homeChatPush({ role: 'assistant', kind: 'notice', text: '正在用演示 SOP 建单开跑，它不调模型，几秒后会停在「确认方案」等你确认。', actions: [] });
  renderHomeChatMain();
  try {
    const started = await startDemoRequirement(null, { stay: true });
    const issue = started?.issue;
    if (!issue) throw new Error('还没有项目，请先在项目管理里建一个');
    homeChatPush({ role: 'assistant', kind: 'launched', title: issue.title || '演示需求', issueId: issue.id, runId: started.task?.runId || '', workflowId: HOME_CHAT_DEMO, workflowName: '演示交付流程' });
  } catch (err) {
    homeChatPush({ role: 'assistant', kind: 'error', text: '演示没跑起来：' + err.message, actions: [] });
  }
  renderHomeChatMain();
  renderHomeRunsPanel();
}

// ---- 历史对话（和 AI 助手共用会话存储） ----

async function homeChatToggleHistory() {
  const menu = $('#homeChatHistoryMenu');
  if (!menu) return;
  if (!menu.hidden) { menu.hidden = true; return; }
  menu.hidden = false;
  menu.innerHTML = '<div class="hc-history-empty">正在读取…</div>';
  const close = e => { if (!menu.contains(e.target)) { menu.hidden = true; document.removeEventListener('click', close); } };
  document.addEventListener('click', close);
  try {
    const items = await api('/api/assistant/sessions', { silent: true });
    const chat = ensureHomeChatState();
    menu.innerHTML = items?.length
      ? items.slice(0, 30).map(it => `<button type="button" class="hc-history-item${it.id === chat.sessionId ? ' is-active' : ''}" data-home-session="${escapeHTML(it.id)}"><b>${escapeHTML(it.title || '未命名对话')}</b><small>${escapeHTML(homeRunAgo(it.updatedAt))}</small></button>`).join('')
      : '<div class="hc-history-empty">还没有历史对话</div>';
    menu.querySelectorAll('[data-home-session]').forEach(b => b.onclick = () => { menu.hidden = true; homeChatOpenSession(b.dataset.homeSession); });
  } catch (err) {
    menu.innerHTML = `<div class="hc-history-empty">${escapeHTML(err.message || '读取失败')}</div>`;
  }
}

async function homeChatOpenSession(id) {
  try {
    const item = await api('/api/assistant/sessions/' + encodeURIComponent(id), { silent: true });
    const chat = ensureHomeChatState();
    if (chat.busy) homeChatStop(true);
    const messages = [];
    for (const m of item.messages || []) {
      if (m.role === 'user') {
        if (/^(已通过 MCP 执行 |上一个草稿已确认保存成功|刚才的草稿保存失败：)/.test(m.content || '')) continue;
        const text = String(m.content || '').replace(/^（平台记录：[\s\S]*?）\n\n/, '');
        messages.push({ role: 'user', text: text === '（见附件）' ? '' : text, content: m.content, attachments: m.attachments || [], refs: m.refs || [] });
        continue;
      }
      let parsed = null;
      try { parsed = JSON.parse(m.content); } catch (e) {}
      if (!parsed || typeof parsed.message !== 'string') parsed = { message: m.content || '', drafts: [] };
      messages.push({ role: 'assistant', kind: 'reply', text: parsed.message, questions: parsed.questions || [], answers: {}, answered: true, launch: parsed.launch || null, files: (parsed.files || []).map(f => ({ id: f.id, name: f.name, mime: f.mimeType || f.mime, size: f.size })), drafts: parsed.drafts || [] });
    }
    Object.assign(chat, { messages, sessionId: item.id || id, draft: '', notes: [], files: [], refs: [] });
    const last = homeChatLatestReply();
    if (last) last.answered = false;
    persistHomeChat();
    renderHomeChatMain();
  } catch (err) {
    toast('打不开这段对话：' + err.message, 'err');
  }
}

// ---- 右侧：本项目正在跑的 SOP ----

function homeRunsForProject() {
  const projectId = state.projectFilter || '';
  return (state.data.runs || []).filter(r => !projectId || (r.executionContext?.projectId || '') === projectId);
}

function homeRunSteps(run) {
  const workflow = (state.data.workflows || []).find(w => w.id === run.workflowId);
  const nodes = (workflow?.nodes || run.workflowSnapshot?.nodes || []).filter(n => n && n.id);
  const states = run.nodes || {};
  let order = nodes.map(n => n.id);
  const edges = workflow?.edges || run.workflowSnapshot?.edges || [];
  if (order.length && edges.length) {
    const indeg = Object.fromEntries(order.map(id => [id, 0]));
    const forward = edges.filter(e => e && (!e.when || e.when === 'success') && e.source !== e.target && e.target in indeg && e.source in indeg);
    forward.forEach(e => { indeg[e.target]++; });
    const ready = order.filter(id => !indeg[id]);
    const sorted = [];
    while (ready.length) {
      const id = ready.shift();
      sorted.push(id);
      forward.forEach(e => {
        if (e.source === id && --indeg[e.target] === 0) ready.push(e.target);
      });
    }
    order.forEach(id => { if (!sorted.includes(id)) sorted.push(id); });
    order = sorted;
  }
  Object.keys(states).forEach(id => { if (!order.includes(id)) order.push(id); });
  return order.map(id => {
    const node = nodes.find(n => n.id === id);
    return { id, name: node?.name || id, status: homeRunStepStatus(states[id]?.status), error: states[id]?.error || '' };
  });
}

function homeRunStepStatus(status) {
  if (status === 'succeeded' || status === 'skipped') return 'done';
  if (status === 'waiting_approval' || status === 'waiting') return 'waiting';
  if (status === 'running' || status === 'queued' || status === 'retrying') return 'running';
  if (status === 'failed') return 'failed';
  if (status === 'cancelled') return 'cancelled';
  return 'pending';
}

function homeRunStatus(run) {
  const steps = homeRunSteps(run);
  if (steps.some(s => s.status === 'waiting')) return { key: 'waiting', label: '等你确认' };
  const map = { queued: '排队中', pending: '排队中', running: '运行中', retrying: '重试中', paused: '已暂停', waiting: '等你确认', succeeded: '已完成', failed: '失败', cancelled: '已取消' };
  return { key: run.status, label: map[run.status] || run.status };
}

function homeRunAgo(iso) {
  const t = Date.parse(iso || '');
  if (!t) return '';
  const s = Math.max(0, Math.round((Date.now() - t) / 1000));
  if (s < 60) return '刚刚';
  if (s < 3600) return `${Math.round(s / 60)} 分钟前`;
  if (s < 86400) return `${Math.round(s / 3600)} 小时前`;
  return `${Math.round(s / 86400)} 天前`;
}

function homeRunCardHTML(run) {
  const steps = homeRunSteps(run);
  const status = homeRunStatus(run);
  const current = steps.find(s => s.status === 'waiting') || steps.find(s => s.status === 'running') || steps.find(s => s.status === 'failed');
  const doneCount = steps.filter(s => s.status === 'done').length;
  const workflow = (state.data.workflows || []).find(w => w.id === run.workflowId);
  const issueId = run.executionContext?.issueId || '';
  const failed = steps.find(s => s.status === 'failed');
  const where = current ? `${status.key === 'waiting' ? '停在' : status.key === 'failed' ? '卡在' : '正在'}「${current.name}」` : (run.status === 'succeeded' ? '全部完成' : '等待开始');
  return `<article class="hc-run is-${escapeHTML(status.key)}" data-home-run="${escapeHTML(run.id)}" data-home-issue="${escapeHTML(issueId)}" tabindex="0">
    <div class="hc-run-top"><span class="hc-run-pill">${escapeHTML(status.label)}</span><small>${escapeHTML(homeRunAgo(run.createdAt))}</small></div>
    <b class="hc-run-title">${escapeHTML(runIssueTitle(run))}</b>
    <small class="hc-run-sop">${escapeHTML(workflow?.name || run.workflowSnapshot?.name || run.workflowId)}</small>
    ${steps.length ? `<div class="hc-steps" role="img" aria-label="第 ${doneCount}/${steps.length} 步">${steps.map(s => `<i class="is-${s.status}" title="${escapeHTML(s.name)}"></i>`).join('')}</div>` : ''}
    <div class="hc-run-where"><span>${escapeHTML(where)}</span><small>${doneCount}/${steps.length || 0}</small></div>
    ${failed?.error ? `<p class="hc-run-error">${escapeHTML(failed.error.split('\n')[0].slice(0, 120))}</p>` : ''}
    ${status.key === 'waiting' ? '<button type="button" class="button primary small hc-run-go">去确认</button>' : ''}
  </article>`;
}

function renderHomeRunsPanel() {
  const panel = $('#hcRuns');
  if (!panel) return;
  const runs = homeRunsForProject();
  const active = runs.filter(r => HOME_RUN_ACTIVE.has(r.status));
  const dayAgo = Date.now() - 3 * 86400 * 1000;
  const recent = runs.filter(r => !HOME_RUN_ACTIVE.has(r.status) && Date.parse(r.updatedAt || r.createdAt || '') > dayAgo).slice(0, 5);
  const waiting = active.filter(r => homeRunStatus(r).key === 'waiting').length;
  panel.innerHTML = `<header class="hc-runs-head">
      <button type="button" class="hc-runs-toggle" id="homeRunsToggle" title="收起 / 展开" aria-label="收起或展开运行面板">⟩</button>
      <div><b>正在运行</b><span>${active.length ? `${active.length} 条${waiting ? ` · ${waiting} 条等你确认` : ''}` : '暂无'}</span></div>
      <button type="button" class="button ghost small" id="homeRunsRefresh">刷新</button>
    </header>
    <div class="hc-runs-body">
      ${active.length ? active.map(homeRunCardHTML).join('') : '<p class="hc-runs-empty">还没有正在跑的 SOP。在左边说清需求开跑后，每一步的进度都会出现在这里。</p>'}
      ${recent.length ? `<div class="hc-runs-sub">最近结束</div>${recent.map(homeRunCardHTML).join('')}` : ''}
      <button type="button" class="link-button hc-runs-all" data-home-goto="runs">查看全部运行记录 →</button>
    </div>`;
  panel.querySelector('#homeRunsToggle').onclick = () => {
    const layout = panel.closest('.hc-layout');
    if (window.matchMedia('(max-width: 1100px)').matches) { layout.classList.toggle('runs-open'); return; }
    const collapsed = !layout.classList.contains('runs-collapsed');
    layout.classList.toggle('runs-collapsed', collapsed);
    localStorage.setItem('loopforge.homeRuns.collapsed', collapsed ? '1' : '0');
  };
  panel.querySelector('#homeRunsRefresh').onclick = async () => { await refreshRuns(true); renderHomeRunsPanel(); };
  panel.querySelectorAll('[data-home-goto]').forEach(b => b.onclick = () => switchView(b.dataset.homeGoto));
  panel.querySelectorAll('[data-home-run]').forEach(card => {
    const open = () => card.dataset.homeIssue ? openIssueDetail(card.dataset.homeIssue, 'sop') : (typeof openRunFromId === 'function' && openRunFromId(card.dataset.homeRun));
    card.onclick = open;
    card.onkeydown = e => { if (e.key === 'Enter') open(); };
  });
}

function startHomeRunsPolling() {
  if (homeRunsTimer) return;
  homeRunsTimer = setInterval(async () => {
    if (activeViewName() !== 'chat' || document.hidden) {
      if (activeViewName() !== 'chat') { clearInterval(homeRunsTimer); homeRunsTimer = null; }
      return;
    }
    try { await refreshRuns(true); } catch (e) {}
    renderHomeRunsPanel();
  }, 5000);
}
