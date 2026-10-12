// Grasp-style home chatbot: one sentence on the dashboard → clarify → dispatch,
// with a live strip of running / waiting workflows next to the conversation.
const HOME_CHAT_DEMO = 'demo-delivery';
const HOME_CHAT_QUICK = 'quick-fix';

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
  if (demo) list.push(demo);
  if (quick) list.push(quick);
  mine.forEach(w => { if (!list.some(x => x.id === w.id)) list.push(w); });
  return list;
}

function homeChatActiveRuns() {
  const projectId = state.projectFilter || '';
  const runs = (state.data.runs || []).filter(r => {
    if (projectId && r.projectId && r.projectId !== projectId) return false;
    return ['running', 'waiting', 'queued'].includes(r.status);
  });
  return runs.slice(0, 8);
}

function ensureHomeChatState() {
  if (!state.homeChat) {
    state.homeChat = { messages: [], sessionId: '', jobId: '', busy: false, result: null, workflowId: '' };
  }
  return state.homeChat;
}

function renderHomeChat() {
  const host = $('#homeChat');
  if (!host) return;
  const user = state.currentUser;
  if (!user || user.isGuest) {
    host.innerHTML = `<div class="home-chat-hero"><p class="home-chat-kicker">Chat</p><h2>用对话启动研发</h2><p>登录后，在这里用一句话说清需求；平台会澄清、建单并开跑。</p><button type="button" class="button primary" id="homeChatLogin">登录开始</button></div>`;
    host.querySelector('#homeChatLogin').onclick = () => openLoginModal();
    return;
  }
  const chat = ensureHomeChatState();
  const workflows = homeChatWorkflows();
  if (!chat.workflowId || !workflows.some(w => w.id === chat.workflowId)) {
    // Prefer demo when present (no key); otherwise the short quick-fix SOP.
    chat.workflowId = (workflows.find(w => w.id === HOME_CHAT_DEMO)
      || workflows.find(w => w.id === HOME_CHAT_QUICK)
      || workflows[0])?.id || '';
  }
  const demoReady = workflows.some(w => w.id === HOME_CHAT_DEMO);
  const needsKey = !homeChatHasModelKey() && chat.workflowId !== HOME_CHAT_DEMO;
  const runs = homeChatActiveRuns();
  const thread = chat.messages.map(m => {
    if (m.role === 'user') return `<div class="home-chat-bubble is-user"><b>你</b><p>${escapeHTML(m.content)}</p></div>`;
    if (m.role === 'status') return `<div class="home-chat-bubble is-status">${escapeHTML(m.content)}</div>`;
    return `<div class="home-chat-bubble is-bot"><b>LoopForge</b><div class="home-chat-md">${m.html || escapeHTML(m.content)}</div>${m.actions || ''}</div>`;
  }).join('');
  const runCards = runs.length ? runs.map(r => {
    const issue = (state.data.issues || []).find(i => i.id === r.issueId);
    const title = issue?.title || r.workflowName || r.workflowId || r.id;
    const waiting = r.status === 'waiting';
    return `<button type="button" class="home-run-card ${waiting ? 'is-waiting' : ''}" data-home-run="${escapeHTML(r.id)}" data-home-issue="${escapeHTML(r.issueId || '')}"><span class="home-run-status">${waiting ? '待确认' : '运行中'}</span><b>${escapeHTML(title)}</b><small>${escapeHTML(r.workflowId || '')} · ${escapeHTML(statusText(r.status) || r.status)}</small></button>`;
  }).join('') : `<p class="home-runs-empty">还没有进行中的运行。发一条需求，或点快速开始里的「一键开跑」。</p>`;

  host.innerHTML = `
    <div class="home-chat-layout">
      <section class="home-chat-main">
        <div class="home-chat-hero">
          <p class="home-chat-kicker">Chat</p>
          <h2>一句话说清，剩下的交给流程</h2>
          <p>对话澄清 → 建单派发 → 在右侧看运行进度。演示模式不需要 API Key。</p>
        </div>
        <div class="home-chat-thread" id="homeChatThread">${thread || `<div class="home-chat-empty">试试：「给需求列表加导出 CSV」${demoReady ? '，或直接开跑演示 SOP' : ''}</div>`}</div>
        <form class="home-chat-composer" id="homeChatForm">
          <div class="home-chat-toolbar">
            <label>SOP<select id="homeChatWorkflow" class="select">${workflows.map(w => `<option value="${escapeHTML(w.id)}" ${w.id === chat.workflowId ? 'selected' : ''}>${escapeHTML(w.name || w.id)}${w.id === HOME_CHAT_DEMO ? '（演示）' : w.id === HOME_CHAT_QUICK ? '（短链路）' : ''}</option>`).join('')}</select></label>
            ${needsKey ? `<button type="button" class="button ghost small" id="homeChatGoSettings">先去配模型 Key</button>` : ''}
            ${demoReady ? `<button type="button" class="button ghost small" id="homeChatDemo" ${chat.busy ? 'disabled' : ''}>一键演示</button>` : ''}
          </div>
          <textarea id="homeChatInput" rows="3" maxlength="4000" placeholder="描述你想做的改动，例如：个人主页增加徽章，兼容现有成就体系" ${chat.busy ? 'disabled' : ''}></textarea>
          <div class="home-chat-actions">
            <span class="home-chat-hint" id="homeChatHint">${needsKey ? '当前 SOP 需要模型 API Key；演示 SOP 不用。' : 'Enter 发送，Shift+Enter 换行'}</span>
            <button type="button" class="button ghost" id="homeChatStop" ${chat.busy ? '' : 'hidden'}>停止</button>
            <button type="submit" class="button primary" id="homeChatSend" ${chat.busy || needsKey ? 'disabled' : ''}>${chat.messages.some(m => m.role === 'user') ? '继续澄清' : '开始澄清'}</button>
            <button type="button" class="button ghost" id="homeChatDispatch" ${chat.result?.ready ? '' : 'hidden'}>建单并开跑</button>
          </div>
        </form>
      </section>
      <aside class="home-chat-side">
        <header><b>正在运行</b><button type="button" class="button ghost small" id="homeChatRefreshRuns">刷新</button></header>
        <div class="home-run-list" id="homeChatRuns">${runCards}</div>
        <header class="home-side-second"><b>待我确认</b><button type="button" class="button ghost small" data-home-goto="approvals">全部</button></header>
        <div class="home-run-list" id="homeChatApprovals">${renderHomeChatApprovals()}</div>
      </aside>
    </div>`;
  bindHomeChat();
}

function renderHomeChatApprovals() {
  const items = (typeof approvalItems === 'function' ? approvalItems() : []).slice(0, 6);
  if (!items.length) return `<p class="home-runs-empty">没有等待你拍板的运行。</p>`;
  return items.map(run => {
    const issue = (state.data.issues || []).find(i => i.id === run.issueId);
    return `<button type="button" class="home-run-card is-waiting" data-home-run="${escapeHTML(run.id || '')}" data-home-issue="${escapeHTML(run.issueId || '')}"><span class="home-run-status">待确认</span><b>${escapeHTML(issue?.title || run.workflowId || run.id)}</b><small>批准后继续下游</small></button>`;
  }).join('');
}

function bindHomeChat() {
  const chat = ensureHomeChatState();
  const form = $('#homeChatForm');
  const input = $('#homeChatInput');
  const workflow = $('#homeChatWorkflow');
  if (workflow) workflow.onchange = () => { chat.workflowId = workflow.value; renderHomeChat(); };
  $('#homeChatGoSettings') && ($('#homeChatGoSettings').onclick = () => {
    if (typeof reopenSetupWizardFromGuide === 'function') reopenSetupWizardFromGuide();
    else switchView('settings');
  });
  $('#homeChatDemo') && ($('#homeChatDemo').onclick = () => homeChatRunDemo());
  $('#homeChatRefreshRuns') && ($('#homeChatRefreshRuns').onclick = async () => { try { await refreshRuns(true); await refreshIssues(); } catch (e) {} renderHomeChat(); });
  $('#homeChatStop') && ($('#homeChatStop').onclick = () => homeChatStop());
  $('#homeChatDispatch') && ($('#homeChatDispatch').onclick = () => homeChatCreateAndDispatch());
  $$('[data-home-goto]').forEach(b => b.onclick = () => switchView(b.dataset.homeGoto));
  $$('[data-home-run]').forEach(b => b.onclick = () => {
    if (b.dataset.homeIssue) openIssueDetail(b.dataset.homeIssue, 'sop');
    else if (b.dataset.homeRun && typeof openRunFromId === 'function') openRunFromId(b.dataset.homeRun);
  });
  if (!form) return;
  form.onsubmit = e => { e.preventDefault(); homeChatSend(); };
  if (input) {
    input.onkeydown = e => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); homeChatSend(); }
    };
  }
}

function homeChatPush(role, content, extra) {
  const chat = ensureHomeChatState();
  chat.messages.push({ role, content, ...(extra || {}) });
}

function homeChatSetBusy(busy) {
  ensureHomeChatState().busy = busy;
}

async function homeChatRunDemo() {
  const chat = ensureHomeChatState();
  chat.workflowId = HOME_CHAT_DEMO;
  const input = $('#homeChatInput');
  const text = (input?.value || '').trim() || '演示：需求列表支持导出 CSV';
  if (input) input.value = '';
  homeChatPush('user', text);
  homeChatPush('status', '正在用演示 SOP 建单并开跑…');
  homeChatSetBusy(true);
  renderHomeChat();
  try {
    await startDemoRequirement(null);
    homeChatPush('assistant', '演示需求已开跑。右侧「正在运行 / 待我确认」会更新；在需求详情里批准「确认方案」后看交付报告。', { html: '演示需求已开跑。右侧「正在运行 / 待我确认」会更新；在需求详情里批准「确认方案」后看交付报告。' });
  } catch (err) {
    homeChatPush('assistant', '开跑失败：' + err.message, { html: escapeHTML('开跑失败：' + err.message) });
  } finally {
    homeChatSetBusy(false);
    try { await refreshRuns(true); await refreshIssues(); } catch (e) {}
    renderHomeChat();
  }
}

async function homeChatSend() {
  const chat = ensureHomeChatState();
  if (chat.busy) return;
  const input = $('#homeChatInput');
  const msg = (input?.value || '').trim();
  if (!msg) return;
  if (chat.workflowId === HOME_CHAT_DEMO && !homeChatHasModelKey()) {
    if (input) input.value = msg;
    return homeChatRunDemo();
  }
  if (!homeChatHasModelKey()) {
    toast('请先在个人设置里配置模型 API Key');
    return switchView('settings');
  }
  const project = (typeof currentProject === 'function' ? currentProject() : null) || (state.data.projects || [])[0];
  if (!project) { toast('请先创建项目'); return switchView('projects'); }
  input.value = '';
  homeChatPush('user', msg);
  homeChatPush('status', '正在澄清…');
  homeChatSetBusy(true);
  chat.result = null;
  renderHomeChat();
  const controller = new AbortController();
  chat._abort = controller;
  chat.jobId = 'h' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
  try {
    const history = chat.messages.filter(m => m.role === 'user' || (m.role === 'assistant' && m.raw)).map(m => ({
      role: m.role === 'assistant' ? 'assistant' : 'user',
      content: m.raw || m.content,
    }));
    const resp = await fetch('/api/chat-clarify', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
      body: JSON.stringify({
        projectId: project.id,
        title: history.find(h => h.role === 'user')?.content?.slice(0, 80) || msg.slice(0, 80),
        description: history.filter(h => h.role === 'user').map(h => h.content).join('\n\n'),
        message: msg,
        sessionId: chat.sessionId,
        jobId: chat.jobId,
        history,
        workflowId: chat.workflowId,
      }),
      signal: controller.signal,
    });
    if (!resp.ok) {
      let em = `HTTP ${resp.status}`;
      try { const d = await resp.json(); em = d.error || em; } catch (e) {}
      throw new Error(em);
    }
    const data = await homeChatReadSSE(resp);
    chat.messages = chat.messages.filter(m => m.role !== 'status');
    if (data.sessionId) chat.sessionId = data.sessionId;
    const summary = data.summary || data.message || data.reply || '澄清完成';
    const questions = (data.questions || []).map((q, i) => `<li><b>Q${i + 1}.</b> ${escapeHTML(typeof q === 'string' ? q : (q.question || q.text || JSON.stringify(q)))}</li>`).join('');
    const score = data.assessment?.score ?? data.score;
    const ready = !!(data.passed || data.ready || (score != null && score >= 80) || data.status === 'ready');
    chat.result = { ready, data, title: data.suggestedTitle || history.find(h => h.role === 'user')?.content?.slice(0, 80) || msg.slice(0, 80) };
    const actions = ready
      ? `<div class="home-chat-cta"><button type="button" class="button primary small" id="homeChatDispatchInline">建单并开跑</button><span>澄清已达标${score != null ? `（${score} 分）` : ''}</span></div>`
      : `<div class="home-chat-cta"><span>再回答上面的问题，或改需求后继续澄清${score != null ? `（当前 ${score} 分）` : ''}</span></div>`;
    homeChatPush('assistant', summary, {
      raw: summary,
      html: `<p>${escapeHTML(summary)}</p>${questions ? `<ol class="home-chat-questions">${questions}</ol>` : ''}`,
      actions,
    });
  } catch (err) {
    chat.messages = chat.messages.filter(m => m.role !== 'status');
    if (err.name !== 'AbortError') {
      homeChatPush('assistant', '澄清失败：' + err.message, { html: escapeHTML('澄清失败：' + err.message) });
    } else {
      homeChatPush('status', '已停止');
    }
  } finally {
    chat._abort = null;
    chat.jobId = '';
    homeChatSetBusy(false);
    renderHomeChat();
    const inline = $('#homeChatDispatchInline');
    if (inline) inline.onclick = () => homeChatCreateAndDispatch();
  }
}

async function homeChatReadSSE(resp) {
  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buf = '', last = {};
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    const parts = buf.split('\n\n');
    buf = parts.pop() || '';
    for (const part of parts) {
      const lines = part.split('\n');
      let event = 'message', data = '';
      for (const line of lines) {
        if (line.startsWith('event:')) event = line.slice(6).trim();
        if (line.startsWith('data:')) data += line.slice(5).trim();
      }
      if (!data) continue;
      try {
        const payload = JSON.parse(data);
        if (event === 'result' || event === 'done' || payload.summary || payload.questions) last = { ...last, ...payload };
        if (payload.sessionId) last.sessionId = payload.sessionId;
        if (payload.message && event === 'status') {
          const chat = ensureHomeChatState();
          const status = chat.messages.find(m => m.role === 'status');
          if (status) status.content = payload.message;
          const hint = $('#homeChatHint');
          if (hint) hint.textContent = payload.message;
        }
      } catch (e) {}
    }
  }
  return last;
}

function homeChatStop() {
  const chat = ensureHomeChatState();
  chat._abort?.abort();
}

async function homeChatCreateAndDispatch() {
  const chat = ensureHomeChatState();
  const project = (typeof currentProject === 'function' ? currentProject() : null) || (state.data.projects || [])[0];
  if (!project) return toast('请先创建项目');
  const workflowId = chat.workflowId || HOME_CHAT_DEMO;
  const title = chat.result?.title || chat.messages.find(m => m.role === 'user')?.content?.slice(0, 80) || '新需求';
  const description = chat.messages.filter(m => m.role === 'user').map(m => m.content).join('\n\n');
  homeChatSetBusy(true);
  homeChatPush('status', '正在建单并开跑…');
  renderHomeChat();
  try {
    let issue = await api('/api/issues', {
      method: 'POST',
      body: JSON.stringify({
        projectId: project.id,
        title,
        description,
        kind: 'requirement',
        priority: 'medium',
        status: 'todo',
        syncTarget: 'local',
        workflowId,
        assigneeUserId: Number(state.currentUser?.id) || 0,
        clarification: chat.result?.data?.assessment ? {
          summary: chat.result.data.summary || '',
          dimensions: chat.result.data.assessment.dimensions,
          missing: chat.result.data.assessment.missing || [],
          questions: chat.result.data.questions || [],
          status: 'ready',
          passed: true,
          gated: true,
        } : undefined,
      }),
    });
    rememberIssue(issue);
    const task = await api(`/api/issues/${issue.id}/dispatch`, { method: 'POST', body: JSON.stringify({ workflowId }) });
    issue = { ...issue, latestTaskId: task.id, status: 'in_progress' };
    rememberIssue(issue);
    (state.data.tasks || (state.data.tasks = [])).unshift(task);
    if (task.runId && typeof startLiveRun === 'function') startLiveRun(task.runId);
    chat.messages = chat.messages.filter(m => m.role !== 'status');
    homeChatPush('assistant', '已建单并开跑', {
      html: `已创建需求并进入 SOP。<button type="button" class="button primary small" data-open-created="${escapeHTML(issue.id)}">打开需求</button>`,
    });
    chat.result = null;
    chat.sessionId = '';
    try { await refreshRuns(true); await refreshIssues(); } catch (e) {}
    renderHomeChat();
    const btn = document.querySelector(`[data-open-created="${issue.id}"]`);
    if (btn) btn.onclick = () => openIssueDetail(issue.id, 'sop');
    else openIssueDetail(issue.id, 'sop');
  } catch (err) {
    chat.messages = chat.messages.filter(m => m.role !== 'status');
    toast(err.message, 'err');
    homeChatPush('assistant', '建单失败：' + err.message, { html: escapeHTML('建单失败：' + err.message) });
    homeChatSetBusy(false);
    renderHomeChat();
  } finally {
    homeChatSetBusy(false);
  }
}
