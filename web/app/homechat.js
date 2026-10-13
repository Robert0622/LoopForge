// 「发起需求」：和澄清 Agent 对话，说清后按 SOP 开跑；右侧列出本项目正在跑的 SOP 和每一步的进度。
const HOME_CHAT_DEMO = 'demo-delivery';
const HOME_CHAT_QUICK = 'quick-fix';
const HOME_CHAT_STORE = 'loopforge.homeChat.v2';
const HOME_CHAT_PLACEHOLDERS = [
  '给需求列表加导出 CSV，导出当前筛选结果',
  '登录失败时提示具体原因，别只显示「登录失败」',
  '个人主页增加徽章，兼容现有成就体系',
];
const HOME_RUN_ACTIVE = new Set(['queued', 'pending', 'running', 'waiting', 'waiting_approval', 'paused', 'retrying']);
let homeRunsTimer = null;

function homeChatHasModelKey() {
  const stored = userById(state.currentUser?.id) || {};
  return !!(stored.claudeCodeApiKeyConfigured || stored.codexApiKeyConfigured || stored.codeBuddyApiKeyConfigured);
}

function homeChatWorkflows() {
  const projectId = state.projectFilter || '';
  const all = state.data.workflows || [];
  const mine = all.filter(w => !catalogGeneric(w) && (!projectId || w.projectId === projectId) && w.status !== 'archived');
  const demo = all.find(w => w.id === HOME_CHAT_DEMO);
  const quick = all.find(w => w.id === HOME_CHAT_QUICK);
  const list = [];
  if (quick) list.push(quick);
  if (demo) list.push(demo);
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
    state.homeChat = { messages: [], sessionId: '', jobId: '', workflowId: '', draft: '', ...(saved || {}), key, busy: false, thinking: [] };
  }
  return state.homeChat;
}

function persistHomeChat() {
  const chat = state.homeChat;
  if (!chat) return;
  try {
    localStorage.setItem(chat.key, JSON.stringify({
      messages: chat.messages.slice(-80), sessionId: chat.sessionId, jobId: chat.jobId, workflowId: chat.workflowId, draft: chat.draft,
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

// 一次对话从最近一次开跑之后算起：开跑之后再说话就是新需求。
function homeChatTurns() {
  const chat = ensureHomeChatState();
  const last = chat.messages.map(m => m.kind).lastIndexOf('launched');
  return chat.messages.slice(last + 1);
}

function homeChatLatestClarify() {
  const turns = homeChatTurns();
  for (let i = turns.length - 1; i >= 0; i--) {
    if (turns[i].kind === 'clarify') return turns[i];
    if (turns[i].role === 'user') return null;
  }
  return null;
}

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
  if (chat.jobId && !chat._abort) homeChatAttach(chat.jobId);
}

function renderHomeChatMain() {
  const main = $('#hcMain');
  if (!main) return;
  const chat = ensureHomeChatState();
  const workflow = homeChatSelectedWorkflow();
  const empty = !chat.messages.length && !chat.busy;
  main.classList.toggle('is-empty', empty);
  main.innerHTML = empty ? homeChatHeroHTML(workflow) : `
    <header class="hc-head">
      <div><b>${escapeHTML(homeChatTitle())}</b><span>${workflow ? `说清后按「${escapeHTML(workflow.name || workflow.id)}」开跑` : '先在项目里发布一条 SOP'}</span></div>
      <button type="button" class="button ghost small" id="homeChatNew">＋ 新对话</button>
    </header>
    <div class="hc-thread" id="homeChatThread">${chat.messages.map(homeChatMessageHTML).join('')}${chat.busy ? homeChatThinkingHTML() : ''}</div>
    <div class="hc-dock">${homeChatComposerHTML(workflow)}</div>`;
  bindHomeChat();
  const thread = $('#homeChatThread');
  if (thread) thread.scrollTop = thread.scrollHeight;
}

function homeChatTitle() {
  const first = homeChatTurns().find(m => m.role === 'user') || ensureHomeChatState().messages.find(m => m.role === 'user');
  return first ? first.text.split('\n')[0].slice(0, 60) : '新需求';
}

function homeChatHeroHTML(workflow) {
  const workflows = homeChatWorkflows();
  const keyHint = homeChatHasModelKey() ? '' : `<p class="hc-key-hint">还没配置模型 Key，我暂时没法读代码、追问需求。<button type="button" class="link-button" data-chat-action="settings-models">去配置</button>，或<button type="button" class="link-button" data-chat-action="demo">先跑一遍演示</button>（不需要 Key）。</p>`;
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
  return `<div class="hc-hero">
    <h1 class="hc-brand">LoopForge</h1>
    <p class="hc-sub">一句话说清要改什么。我会读代码、追问到说清楚，再按你选的 SOP 开跑。</p>
    ${homeChatComposerHTML(workflow)}
    ${keyHint}
    ${workflows.length ? `<div class="hc-sop-head">选择 SOP</div><div class="hc-sop-rail" role="list">${cards}</div>` : '<p class="hc-key-hint">这个项目还没有 SOP。<button type="button" class="link-button" data-home-goto="templates">从模板创建一个</button></p>'}
  </div>`;
}

function homeChatComposerHTML(workflow) {
  const chat = ensureHomeChatState();
  const workflows = homeChatWorkflows();
  const latest = homeChatLatestClarify();
  const asking = latest && !latest.ready && (latest.questions || []).length;
  const placeholder = !chat.messages.length ? HOME_CHAT_PLACEHOLDERS[0] : asking ? '点选上面的选项，或直接打字回答' : '继续补充，或说一个新需求';
  return `<form class="hc-composer" id="homeChatForm">
    <textarea id="homeChatInput" rows="${chat.messages.length ? 2 : 3}" maxlength="4000" placeholder="${escapeHTML(placeholder)}" ${chat.busy ? 'disabled' : ''}>${escapeHTML(chat.draft || '')}</textarea>
    <div class="hc-composer-bar">
      <label class="hc-sop-pick" title="说清后按这条 SOP 开跑"><span>SOP</span><select id="homeChatWorkflow" ${chat.busy ? 'disabled' : ''}>${workflows.map(w => `<option value="${escapeHTML(w.id)}" ${w.id === workflow?.id ? 'selected' : ''}>${escapeHTML(w.name || w.id)}</option>`).join('')}</select></label>
      ${typeof clarifyModelPickerHTML === 'function' ? clarifyModelPickerHTML(chat.busy) : ''}
      <span class="hc-composer-hint">${chat.busy ? '分析中，关掉页面也会继续' : 'Enter 发送 · Shift+Enter 换行'}</span>
      ${chat.busy
        ? '<button type="button" class="hc-send is-stop" id="homeChatStop" title="停止" aria-label="停止">■</button>'
        : '<button type="submit" class="hc-send" id="homeChatSend" title="发送" aria-label="发送">↑</button>'}
    </div>
  </form>`;
}

function homeChatMessageHTML(m, index) {
  if (m.role === 'user') return `<div class="hc-msg is-user"><div class="hc-bubble">${escapeHTML(m.text).replace(/\n/g, '<br>')}</div></div>`;
  const body = m.kind === 'clarify' ? homeChatClarifyHTML(m, index)
    : m.kind === 'launched' ? homeChatLaunchedHTML(m)
    : `<p>${escapeHTML(m.text || '')}</p>${homeChatActionsHTML(m.actions)}`;
  return `<div class="hc-msg is-bot${m.kind === 'error' ? ' is-error' : ''}"><span class="hc-avatar" aria-hidden="true">LF</span><div class="hc-bubble">${body}</div></div>`;
}

function homeChatActionsHTML(actions) {
  if (!actions || !actions.length) return '';
  return `<div class="hc-actions">${actions.map(a => `<button type="button" class="button ${a.primary ? 'primary' : 'ghost'} small" data-chat-action="${escapeHTML(a.id)}">${escapeHTML(a.label)}</button>`).join('')}</div>`;
}

function homeChatClarifyHTML(m, index) {
  const latest = homeChatLatestClarify() === m;
  const answers = m.answers || {};
  const questions = (m.questions || []).map((q, qi) => {
    const qid = q.id || `q${qi + 1}`;
    const picked = answers[qid] || [];
    const options = clarifyQuestionOptions(q);
    const chips = options.map(opt => `<button type="button" class="hc-opt${picked.includes(opt) ? ' is-picked' : ''}" data-q="${escapeHTML(qid)}" data-opt="${escapeHTML(opt)}" ${latest && !m.answered ? '' : 'disabled'}>${escapeHTML(opt)}</button>`).join('');
    return `<li class="hc-q"><b>${escapeHTML(q.question)}</b>${q.multiSelect ? '<small>可多选</small>' : ''}${chips ? `<div class="hc-opts">${chips}</div>` : ''}</li>`;
  }).join('');
  const view = typeof composerAssessmentView === 'function' ? composerAssessmentView(m) : null;
  const score = view && Object.keys(view.ratings || {}).length ? view.score : (m.assessment?.score ?? null);
  const threshold = view?.threshold || 80;
  const meter = score == null ? '' : `<div class="hc-meter${m.ready ? ' is-ready' : ''}"><div class="hc-meter-bar"><i style="width:${Math.max(4, Math.min(100, score))}%"></i><em style="left:${threshold}%"></em></div><span>${m.ready ? '已说清' : '完整度'} ${score} / ${threshold}</span></div>`;
  const summary = m.summary ? `<div class="hc-md">${renderMarkdown(m.summary)}</div>` : '';
  const launchable = latest && (m.ready || m.forceLaunch);
  const tail = launchable ? homeChatLaunchCardHTML(m)
    : latest && !m.answered ? `<div class="hc-actions"><button type="button" class="link-button" data-chat-action="force-launch">信息够了，按当前结论开跑</button></div>` : '';
  return `${summary}${questions ? `<ol class="hc-questions">${questions}</ol>` : ''}${meter}${tail}`;
}

function homeChatLaunchCardHTML(m) {
  const workflows = homeChatWorkflows();
  const workflow = homeChatSelectedWorkflow();
  const title = m.suggestedTitle || homeChatTitle();
  return `<div class="hc-launch" id="homeLaunchCard">
    <div class="hc-launch-head"><b>${m.ready ? '需求已经说清，可以开跑' : '按当前结论开跑'}</b><span>建一张需求单并立即派发，进度在右侧看。</span></div>
    <label>需求标题<input id="homeLaunchTitle" value="${escapeHTML(title)}" maxlength="120"></label>
    <label>SOP<select id="homeLaunchWorkflow">${workflows.map(w => `<option value="${escapeHTML(w.id)}" ${w.id === workflow?.id ? 'selected' : ''}>${escapeHTML(w.name || w.id)}</option>`).join('')}</select></label>
    <div class="hc-actions"><button type="button" class="button primary" id="homeChatDispatch">建单并开跑</button></div>
  </div>`;
}

function homeChatLaunchedHTML(m) {
  return `<p>已按「${escapeHTML(m.workflowName || m.workflowId || 'SOP')}」开跑：<b>${escapeHTML(m.title || '')}</b>。进度在右侧，需要你确认时会停下来等你。</p>
    <div class="hc-actions">${m.issueId ? `<button type="button" class="button ghost small" data-open-created="${escapeHTML(m.issueId)}">打开需求</button>` : ''}</div>`;
}

function homeChatThinkingHTML() {
  const chat = ensureHomeChatState();
  const steps = chat.thinking.slice(-4);
  const current = steps[steps.length - 1];
  return `<div class="hc-msg is-bot is-thinking"><span class="hc-avatar" aria-hidden="true">LF</span><div class="hc-bubble" id="hcThinking">
    <div class="hc-typing"><i></i><i></i><i></i><span>${escapeHTML(homeChatThoughtText(current) || '正在分析…')}</span></div>
    ${steps.length > 1 ? `<ul class="hc-trace">${steps.slice(0, -1).map(s => `<li>${escapeHTML(homeChatThoughtText(s))}</li>`).join('')}</ul>` : ''}
  </div></div>`;
}

function homeChatThoughtText(ev) {
  if (!ev) return '';
  if (ev.kind === 'tool' && !ev.tool && ev.text) return ev.text;
  if (ev.kind === 'tool') return `${typeof clarifyToolLabel === 'function' ? clarifyToolLabel(ev.tool) : ev.tool}${ev.detail ? ' · ' + ev.detail : ''}`;
  return ev.text || '';
}

function homeChatPaintThinking() {
  const box = $('#hcThinking');
  if (!box) return;
  box.closest('.hc-msg').outerHTML = homeChatThinkingHTML();
  const thread = $('#homeChatThread');
  if (thread) thread.scrollTop = thread.scrollHeight;
}

function bindHomeChat() {
  const chat = ensureHomeChatState();
  const main = $('#hcMain');
  if (!main) return;
  const input = $('#homeChatInput');
  if (input) {
    input.oninput = () => { chat.draft = input.value; persistHomeChat(); };
    input.onkeydown = e => {
      if (e.key !== 'Enter' || e.shiftKey || e.isComposing || e.keyCode === 229) return;
      e.preventDefault();
      homeChatSend();
    };
  }
  const form = $('#homeChatForm');
  if (form) form.onsubmit = e => { e.preventDefault(); homeChatSend(); };
  const pick = id => { chat.workflowId = id; persistHomeChat(); renderHomeChatMain(); };
  const select = $('#homeChatWorkflow');
  if (select) select.onchange = () => pick(select.value);
  main.querySelectorAll('[data-home-sop]').forEach(b => b.onclick = () => pick(b.dataset.homeSop));
  const launchSelect = $('#homeLaunchWorkflow');
  if (launchSelect) launchSelect.onchange = () => { chat.workflowId = launchSelect.value; persistHomeChat(); };
  $('#homeChatStop') && ($('#homeChatStop').onclick = () => homeChatStop());
  $('#homeChatNew') && ($('#homeChatNew').onclick = () => homeChatReset());
  $('#homeChatDispatch') && ($('#homeChatDispatch').onclick = () => homeChatCreateAndDispatch());
  main.querySelectorAll('.hc-opt').forEach(b => b.onclick = () => homeChatPickOption(b.dataset.q, b.dataset.opt));
  main.querySelectorAll('[data-chat-action]').forEach(b => b.onclick = () => homeChatAction(b.dataset.chatAction));
  main.querySelectorAll('[data-home-goto]').forEach(b => b.onclick = () => switchView(b.dataset.homeGoto));
  main.querySelectorAll('[data-open-created]').forEach(b => b.onclick = () => openIssueDetail(b.dataset.openCreated, 'sop'));
  main.querySelectorAll('[data-clarify-model]').forEach(el => typeof bindClarifyModelPicker === 'function' && bindClarifyModelPicker(el));
}

function homeChatPickOption(qid, opt) {
  const m = homeChatLatestClarify();
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
    const m = homeChatLatestClarify();
    if (m) { m.forceLaunch = true; persistHomeChat(); renderHomeChatMain(); }
  }
}

function homeChatReset() {
  const chat = ensureHomeChatState();
  if (chat.busy) homeChatStop();
  chat.messages = [];
  chat.sessionId = '';
  chat.jobId = '';
  chat.draft = '';
  persistHomeChat();
  renderHomeChatMain();
  $('#homeChatInput')?.focus();
}

function homeChatPush(message) {
  const chat = ensureHomeChatState();
  chat.messages.push({ at: Date.now(), ...message });
  persistHomeChat();
}

// 把当前这轮选中的选项和输入框里的文字拼成一条回答。
function homeChatCollectReply() {
  const input = $('#homeChatInput');
  const typed = (input?.value || '').trim();
  const m = homeChatLatestClarify();
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

function homeChatHistory() {
  return homeChatTurns().filter(m => m.role === 'user' || m.kind === 'clarify').map(m => m.role === 'user'
    ? { role: 'user', content: m.text }
    : { role: 'assistant', content: JSON.stringify({ summary: m.summary, questions: m.questions, ready: m.ready, assessment: m.assessment, suggestedTitle: m.suggestedTitle }) });
}

async function homeChatSend() {
  const chat = ensureHomeChatState();
  if (chat.busy) return;
  const text = homeChatCollectReply();
  if (!text) { $('#homeChatInput')?.focus(); return; }
  const latest = homeChatLatestClarify();
  if (latest) latest.answered = true;
  chat.draft = '';
  homeChatPush({ role: 'user', text });
  if (!homeChatHasModelKey()) {
    homeChatPush({ role: 'assistant', kind: 'notice', text: '还没配置模型 Key，我暂时没法读代码、追问需求。配好后再发一次就行；也可以先跑一遍演示 SOP 看完整流程，它不调模型。', actions: [{ id: 'settings-models', label: '去配置模型 Key', primary: true }, { id: 'demo', label: '先跑演示' }] });
    return renderHomeChatMain();
  }
  const project = currentProject() || (state.data.projects || [])[0];
  if (!project) {
    homeChatPush({ role: 'assistant', kind: 'notice', text: '还没有项目。先建一个项目并绑定代码仓库，我才知道要改哪里。', actions: [] });
    return renderHomeChatMain();
  }
  await homeChatClarify(text, project);
}

function homeChatRetry() {
  const last = homeChatTurns().filter(m => m.role === 'user').pop();
  const project = currentProject() || (state.data.projects || [])[0];
  if (!last || !project) return;
  const chat = ensureHomeChatState();
  if (chat.messages[chat.messages.length - 1]?.kind === 'error') chat.messages.pop();
  homeChatClarify(last.text, project);
}

async function homeChatClarify(text, project) {
  const chat = ensureHomeChatState();
  const history = homeChatHistory();
  const users = history.filter(h => h.role === 'user').map(h => h.content);
  chat.busy = true;
  chat.thinking = [{ kind: 'status', text: '正在准备分析…' }];
  chat.jobId = 'h' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
  persistHomeChat();
  renderHomeChatMain();
  const controller = new AbortController();
  chat._abort = controller;
  try {
    const resp = await fetch('/api/chat-clarify', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
      body: JSON.stringify({
        projectId: project.id,
        repositoryId: project.defaultRepositoryId || '',
        title: (users[0] || text).slice(0, 80),
        description: users.join('\n\n'),
        message: text,
        model: typeof selectedClarifyModel === 'function' ? selectedClarifyModel() : undefined,
        sessionId: chat.sessionId,
        jobId: chat.jobId,
        history,
      }),
      signal: controller.signal,
    });
    if (!resp.ok) {
      let em = `HTTP ${resp.status}`;
      try { const d = await resp.json(); em = d.error || em; } catch (e) {}
      throw new Error(em);
    }
    homeChatApply(await homeChatReadStream(resp));
  } catch (err) {
    homeChatFail(err);
  } finally {
    homeChatSettle(controller);
  }
}

async function homeChatAttach(jobId) {
  const chat = ensureHomeChatState();
  const controller = new AbortController();
  chat._abort = controller;
  chat.busy = true;
  chat.thinking = [{ kind: 'status', text: '正在接上刚才的分析…' }];
  renderHomeChatMain();
  try {
    const resp = await fetch('/api/chat-clarify?jobId=' + encodeURIComponent(jobId), { headers: { Accept: 'text/event-stream' }, signal: controller.signal });
    if (resp.status === 404) { chat.jobId = ''; return; }
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    homeChatApply(await homeChatReadStream(resp));
  } catch (err) {
    homeChatFail(err);
  } finally {
    homeChatSettle(controller);
  }
}

function homeChatApply(data) {
  const chat = ensureHomeChatState();
  if (data?.error) throw new Error(data.error);
  const result = parseClarifyPayload(data);
  chat.sessionId = data?.sessionId || data?.repository?.sessionId || chat.sessionId;
  if (!result.ready && !(result.questions || []).length && !String(result.summary || '').trim()) throw new Error('这一轮没有拿到澄清结论，请再试一次');
  homeChatPush({
    role: 'assistant', kind: 'clarify',
    summary: result.summary || '', questions: result.questions || [], ready: !!result.ready,
    assessment: result.assessment || null, suggestedTitle: result.suggestedTitle || '', answers: {},
  });
}

function homeChatFail(err) {
  if (err?.name === 'AbortError') return;
  const message = err?.message || '澄清失败';
  const credential = /401|API Key|网络环境|Unauthorized/i.test(message);
  homeChatPush({
    role: 'assistant', kind: 'error', text: `这一轮没分析成：${message}`,
    actions: credential ? [{ id: 'settings-models', label: '去个人设置检查 Key', primary: true }, { id: 'retry', label: '重试' }] : [{ id: 'retry', label: '重试', primary: true }],
  });
}

function homeChatSettle(controller) {
  const chat = ensureHomeChatState();
  if (chat._abort !== controller) return;
  chat._abort = null;
  chat.busy = false;
  chat.jobId = '';
  chat.thinking = [];
  persistHomeChat();
  if (activeViewName() === 'chat') {
    renderHomeChatMain();
    $('#homeChatInput')?.focus();
  }
}

async function homeChatReadStream(resp) {
  const chat = ensureHomeChatState();
  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '', done = null;
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
        else if (line.startsWith('data:')) data += line.slice(5).trim();
      });
      if (!data) continue;
      let payload = {};
      try { payload = JSON.parse(data); } catch (e) { continue; }
      if (event === 'job' && payload.jobId) { chat.jobId = payload.jobId; persistHomeChat(); }
      else if (event === 'thought') {
        if (payload.kind !== 'tool' && payload.kind !== 'status' && typeof clarifyThoughtShouldHide === 'function' && clarifyThoughtShouldHide(payload.text)) continue;
        chat.thinking.push(payload);
        homeChatPaintThinking();
      } else if (event === 'done') done = payload;
      else if (event === 'error') done = { error: payload.error || '分析失败' };
    }
  }
  return done || { error: '分析没有返回结果' };
}

function homeChatStop() {
  const chat = ensureHomeChatState();
  const jobId = chat.jobId;
  if (jobId) api('/api/chat-clarify/cancel', { method: 'POST', silent: true, body: JSON.stringify({ jobId }) }).catch(() => {});
  chat._abort?.abort();
  chat._abort = null;
  chat.busy = false;
  chat.jobId = '';
  chat.thinking = [];
  homeChatPush({ role: 'assistant', kind: 'notice', text: '已停止。改一下说法再发，或点重试。', actions: [{ id: 'retry', label: '重试' }] });
  renderHomeChatMain();
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

async function homeChatCreateAndDispatch() {
  const chat = ensureHomeChatState();
  const project = currentProject() || (state.data.projects || [])[0];
  if (!project) return toast('请先创建项目');
  const latest = homeChatLatestClarify();
  const workflowId = $('#homeLaunchWorkflow')?.value || chat.workflowId;
  const workflow = homeChatWorkflows().find(w => w.id === workflowId);
  const title = ($('#homeLaunchTitle')?.value || '').trim() || homeChatTitle();
  const said = homeChatTurns().filter(m => m.role === 'user').map(m => m.text).join('\n\n');
  const description = latest?.summary ? `${said}\n\n## 澄清结论\n\n${latest.summary}` : said;
  const button = $('#homeChatDispatch');
  if (button) { button.disabled = true; button.textContent = '正在建单…'; }
  let issue;
  try {
    issue = await api('/api/issues', {
      method: 'POST',
      body: JSON.stringify({
        projectId: project.id, title, description, kind: 'requirement', priority: 'medium', status: 'todo', syncTarget: 'local', workflowId,
        assigneeUserId: Number(state.currentUser?.id) || 0,
        clarification: latest?.assessment ? {
          summary: latest.summary || '', dimensions: latest.assessment.dimensions, missing: latest.assessment.missing || [],
          questions: latest.questions || [], status: 'ready', passed: true, gated: true,
        } : undefined,
      }),
    });
    rememberIssue(issue);
  } catch (err) {
    if (button) { button.disabled = false; button.textContent = '建单并开跑'; }
    return toast('建单失败：' + err.message, 'err');
  }
  try {
    const task = await api(`/api/issues/${issue.id}/dispatch`, { method: 'POST', body: JSON.stringify({ workflowId }) });
    rememberIssue({ ...issue, latestTaskId: task.id, status: 'in_progress' });
    (state.data.tasks || (state.data.tasks = [])).unshift(task);
    chat.sessionId = '';
    homeChatPush({ role: 'assistant', kind: 'launched', title, issueId: issue.id, runId: task.runId || '', workflowId, workflowName: workflow?.name || workflowId });
  } catch (err) {
    const credential = /Token|API Key|个人设置/.test(err.message);
    homeChatPush({ role: 'assistant', kind: 'error', text: `需求已建好，但没能开跑：${err.message}`, actions: credential ? [{ id: /Token/.test(err.message) ? 'settings-git' : 'settings-models', label: '去个人设置', primary: true }] : [] });
  }
  renderHomeChatMain();
  try { await refreshRuns(true); await refreshIssues(); } catch (e) {}
  renderHomeRunsPanel();
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
