let roomsPollTimer = 0;

function roomProjectId() {
  return state.projectFilter || '';
}

function roomWorkflowChoices() {
  return (state.data?.workflows || []).filter(w => catalogGeneric(w)
    ? w.status !== 'draft'
    : w.status === 'published' && (catalogOwned(w.projectId) || catalogShared(w.projectId, w.shared)));
}

function roomWorkflowLabel(w) {
  return (w.name || w.id) + (catalogGeneric(w) ? '（通用）' : '');
}

function activeRoom() {
  const rs = roomState();
  return (rs.items || []).find(item => item.id === rs.activeId) || null;
}

function roomGreeting() {
  const hour = new Date().getHours();
  const part = hour < 6 ? '凌晨好' : hour < 11 ? '早上好' : hour < 14 ? '中午好' : hour < 18 ? '下午好' : '晚上好';
  const user = state.currentUser;
  const name = user && !user.isGuest ? (user.chineseName || user.loginName || '') : '';
  return name ? part + '，' + name : part;
}

function roomState() {
  if (!state.rooms) state.rooms = {items: [], activeId: '', sops: [], active: false, mode: 'chat'};
  if (!Array.isArray(state.rooms.sops)) state.rooms.sops = [];
  if (state.rooms.mode !== 'issue') state.rooms.mode = 'chat';
  return state.rooms;
}

function setRoomMode(mode) {
  roomState().mode = mode === 'issue' ? 'issue' : 'chat';
  syncRoomComposer();
  $('#roomInput')?.focus();
}

function roomRunTerminal(status) {
  return status === 'succeeded' || status === 'failed' || status === 'cancelled';
}

function bindRooms() {
  const create = $('#newRoom');
  const form = $('#roomComposer');
  if (!create || create.dataset.bound) return;
  create.dataset.bound = '1';
  create.onclick = createRoom;
  $('#renameRoom').onclick = renameRoom;
  $('#roomModeChat').onclick = () => setRoomMode('chat');
  const shell = $('#roomsShell');
  const infoBtn = $('#toggleRoomInfo');
  const setInfo = open => {
    shell?.classList.toggle('info-open', open);
    infoBtn?.setAttribute('aria-pressed', String(open));
    localStorage.setItem('loopforge.roomInfo', open ? '1' : '0');
  };
  setInfo(localStorage.getItem('loopforge.roomInfo') === '1');
  if (infoBtn) infoBtn.onclick = () => {
    const open = !shell.classList.contains('info-open');
    if (open && roomState().threadRun) openRoomRunThread(roomState().threadRun);
    setInfo(open);
  };
  document.querySelectorAll('[data-room-tool]').forEach(button => {
    button.onclick = () => {
      const input = $('#roomInput');
      if (!input) return;
      if (button.dataset.roomTool === 'slash') {
        input.value = '/';
        input.focus();
        input.setSelectionRange(1, 1);
      } else {
        const at = input.selectionStart ?? input.value.length;
        const lead = at > 0 && !/\s$/.test(input.value.slice(0, at)) ? ' @' : '@';
        input.value = input.value.slice(0, at) + lead + input.value.slice(at);
        input.focus();
        input.setSelectionRange(at + lead.length, at + lead.length);
      }
      syncRoomComposer();
      updateRoomMention();
    };
  });
  $('#roomModeIssue').onclick = () => setRoomMode('issue');
  form.onsubmit = event => {
    event.preventDefault();
    submitRoomComposer();
  };
  $('#roomDispatch').onclick = () => {
    if (roomTargetPicked()) dispatchRoomIssue();
    else openRoomMention({targetsOnly: true});
  };
  const input = $('#roomInput');
  input.addEventListener('keydown', event => {
    if (roomMentionKey(event)) return;
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      submitRoomComposer();
    }
  });
  input.addEventListener('input', () => {
    roomAutosize(input);
    syncRoomComposer();
    updateRoomMention();
  });
  input.addEventListener('click', updateRoomMention);
  document.addEventListener('mousedown', event => {
    if (!event.target.closest?.('.room-composer-box')) closeRoomMention();
  });
  $('#roomSOP').addEventListener('change', event => {
    const id = event.target.value;
    if (id) toggleRoomSOP(id, true);
    event.target.value = '';
  });
  document.addEventListener('change', event => {
    if (event.target?.id === 'projectSwitcher' && state.currentView === 'rooms') loadRooms();
  });
}

function toggleRoomSOP(id, force) {
  const rs = roomState();
  const has = rs.sops.includes(id);
  const want = force === undefined ? !has : force;
  if (want && !has) {
    if (rs.sops.length >= 5) return toast('一次最多派给 5 条 SOP');
    rs.sops.push(id);
  }
  if (!want && has) rs.sops = rs.sops.filter(item => item !== id);
  if ($('#roomSOPList')) fillRoomSOPList();
  else syncRoomComposer();
}

function roomTargetPicked() {
  return roomState().sops.length > 0;
}

function syncRoomComposer() {
  const rs = roomState();
  if (rs.sops.length) rs.mode = 'issue';
  const issue = rs.mode === 'issue';
  const names = new Map(roomWorkflowChoices().map(w => [w.id, roomWorkflowLabel(w)]));
  rs.sops = rs.sops.filter(id => names.has(id));
  const picks = $('#roomSOPPicks');
  if (picks) {
    picks.innerHTML = (rs.reply ? `<button type="button" class="room-pick is-reply" data-unreply title="取消回复"><span>↩ 回复 ${escapeHTML(rs.reply.author)}：${escapeHTML(rs.reply.text)}</span><i>×</i></button>` : '') + (rs.crew ? '<button type="button" class="room-pick is-crew" data-unpick-crew title="移除">LoopForge · 组队来做<i>×</i></button>' : '')
      + rs.sops.map(id => `<button type="button" class="room-pick" data-unpick="${escapeHTML(id)}" title="移除">${escapeHTML(names.get(id))}<i>×</i></button>`).join('');
    picks.querySelectorAll('[data-unpick]').forEach(button => { button.onclick = () => toggleRoomSOP(button.dataset.unpick, false); });
    picks.querySelector('[data-unpick-crew]')?.addEventListener('click', () => { rs.crew = false; syncRoomComposer(); });
    picks.querySelector('[data-unreply]')?.addEventListener('click', () => { rs.reply = null; syncRoomComposer(); });
  }
  picks?.closest('.room-composer-picks')?.classList.toggle('has-picks', !!rs.reply || rs.crew || rs.sops.length > 0);
  $('#roomSOP')?.closest('.room-chip')?.classList.toggle('on', rs.sops.length > 0);
  $('#roomComposer')?.classList.toggle('is-issue', issue);
  $('#roomModeChat')?.classList.toggle('on', !issue);
  $('#roomModeIssue')?.classList.toggle('on', issue);
  const input = $('#roomInput');
  if (input) input.placeholder = rs.crew ? '说说要做成什么，队长会拉 Bot 分工（第一行是目标）' : issue ? (rs.sops.length ? '写下要交付什么（第一行是标题），回车就按 SOP 跑' : '先 @ 一条 SOP，再写要交付什么') : (rs.reply ? `回复 ${rs.reply.author}…` : '发消息… @LoopForge 提问或派活，@ SOP 按流程跑，/ 看命令');
  const send = $('#roomSend');
  if (send) {
    send.classList.toggle('is-dispatch', !!rs.crew || (issue && rs.sops.length > 0));
    send.classList.toggle('is-crew', !!rs.crew);
    send.title = rs.crew ? '组队' : issue && rs.sops.length ? '派活' : '发送';
  }
  const picked = roomTargetPicked();
  const dispatch = $('#roomDispatch');
  if (dispatch) {
    dispatch.textContent = !picked ? '建单并派发' : rs.sops.length > 1 ? `建单并派发 ${rs.sops.length} 条 SOP` : '建单并派发';
    dispatch.title = picked ? '第一行作为 Issue 标题，其余作为描述' : '先选一条 SOP';
  }
  const hint = $('#roomTargetHint');
  if (hint) {
    hint.textContent = !picked
      ? '选择一条或多条 SOP。第一行是标题，回车建单。'
      : (rs.sops.length > 1 ? `回车将并行派给 ${rs.sops.length} 条 SOP。` : `回车将建单并派给 SOP「${names.get(rs.sops[0])}」。`);
  }
  $('#roomSend')?.classList.toggle('ready', !!input?.value.trim());
  document.querySelectorAll('[data-room-pick]').forEach(button => {
    button.classList.toggle('picked', rs.sops.includes(button.dataset.roomPick));
  });
}

async function loadRooms() {
  bindRooms();
  const rs = roomState();
  const projectId = roomProjectId();
  fillRoomTargets();
  if (!rs.runsPrimed && typeof refreshRuns === 'function') {
    rs.runsPrimed = true;
    refreshRuns(true).then(() => { rs.threadSignature = ''; refreshRoomMessages(); }).catch(() => {});
  }
  if (!projectId) {
    renderRoomList([]);
    renderRoomThread(null, []);
    renderRoomResources(null);
    return;
  }
  try {
    const rooms = await api('/api/rooms?projectId=' + encodeURIComponent(projectId), {silent: true});
    rs.items = rooms || [];
    if (!rs.items.some(room => room.id === rs.activeId)) rs.activeId = rs.items[0]?.id || '';
    renderRoomList(rs.items);
    await openRoom(rs.activeId);
  } catch (error) {
    toast(error.message);
  }
}

// Poll faster while a dispatched run is still moving so progress feels live.
// Idle rooms back off to 15s; the server only reloads runs whose revision moved.
function scheduleRoomPoll() {
  clearTimeout(roomsPollTimer);
  const delay = roomState().active ? 4000 : 15000;
  roomsPollTimer = setTimeout(async () => {
    if (state.currentView === 'rooms' && roomState().activeId && !document.hidden) await refreshRoomMessages();
    if (state.currentView === 'rooms') scheduleRoomPoll();
  }, delay);
}

function fillRoomTargets() {
  const sop = $('#roomSOP');
  if (!sop) return;
  sop.innerHTML = '<option value="">添加 SOP…</option>' + roomWorkflowChoices().map(item => `<option value="${escapeHTML(item.id)}">${escapeHTML(roomWorkflowLabel(item))}</option>`).join('');
  syncRoomComposer();
}

function renderRoomList(rooms) {
  const host = $('#roomList');
  const count = $('#roomCount');
  if (count) count.textContent = String(rooms.length);
  if (!host) return;
  if (!rooms.length) {
    host.innerHTML = '<div class="room-empty"><b>还没有群</b><p>点右上角 ＋ 建一个群</p></div>';
    return;
  }
  host.innerHTML = rooms.map(room => `<button type="button" class="room-item ${room.id === roomState().activeId ? 'active' : ''}" data-room="${escapeHTML(room.id)}"><i>#</i><span>${escapeHTML(room.name)}</span><small title="群成员（即项目成员）">${(room.members || []).length} 人</small></button>`).join('');
  host.querySelectorAll('[data-room]').forEach(button => {
    button.onclick = () => openRoom(button.dataset.room);
  });
}

async function openRoom(id, keepScroll) {
  const rs = roomState();
  if (rs.activeId !== (id || '')) { rs.ephemeral = []; rs.reply = null; }
  rs.activeId = id || '';
  renderRoomList(rs.items || []);
  const room = (rs.items || []).find(item => item.id === id);
  renderRoomResources(room);
  const title = $('#roomTitle');
  const subtitle = $('#roomSubtitle');
  const rename = $('#renameRoom');
  if (title) title.textContent = room?.name || '选择一个群';
  if (subtitle) subtitle.textContent = room ? `${(room.members || []).length} 位项目成员` : '先在左侧选择或新建一个群';
  if (rename) rename.hidden = !room;
  if (!id) {
    renderRoomThread(null, []);
    return;
  }
  try {
    const messages = await api('/api/rooms/' + encodeURIComponent(id) + '/messages', {silent: true});
    renderRoomThread(room, messages || [], keepScroll);
  } catch (error) {
    toast(error.message);
  }
  scheduleRoomPoll();
}

if (!window.__roomsVisibilityBound) {
  window.__roomsVisibilityBound = true;
  document.addEventListener('visibilitychange', () => {
    if (!document.hidden && state.currentView === 'rooms' && roomState().activeId) {
      refreshRoomMessages().then(scheduleRoomPoll);
    }
  });
}

async function refreshRoomMessages(forceBottom) {
  const rs = roomState();
  const id = rs.activeId;
  const log = $('#roomLog');
  if (!id || !log) return;
  const near = log.scrollHeight - log.scrollTop - log.clientHeight < 80;
  try {
    const messages = await api('/api/rooms/' + encodeURIComponent(id) + '/messages', {silent: true});
    if (rs.activeId !== id) return;
    const room = (rs.items || []).find(item => item.id === id);
    renderRoomThread(room, messages || [], forceBottom === true ? false : !near);
    const open = rs.threadRun && rs.runDetail?.[rs.threadRun];
    if (open && !roomRunTerminal(open.status)) loadRoomRunDetail(rs.threadRun);
  } catch {}
}

function roomResGroup(title, count, body) {
  return `<section class="room-res-group"><div class="room-res-head"><span>${escapeHTML(title)}</span><em>${count}</em></div>${body || '<div class="room-res-more">暂无</div>'}</section>`;
}

const ROOM_MEMBER_ROLE_LABELS = {owner: '群主', maintainer: '维护者', member: '成员', viewer: '只读'};
function roomMemberRoleLabel(role) {
  const key = String(role || '').trim().toLowerCase();
  return ROOM_MEMBER_ROLE_LABELS[key] || role || '成员';
}
function roomResItem(icon, iconClass, name, sub, pick) {
  const tag = pick ? 'button' : 'div';
  const attrs = pick ? ` type="button" data-room-pick="${escapeHTML(pick)}" title="点击选为派发对象"` : '';
  return `<${tag} class="room-res-item ${pick ? '' : 'static'}"${attrs}><span class="room-res-icon ${iconClass}">${escapeHTML(icon)}</span><span class="room-res-text"><b>${escapeHTML(name)}</b>${sub ? `<small>${escapeHTML(sub)}</small>` : ''}</span></${tag}>`;
}

function renderRoomResources(room) {
  const host = $('#roomResources');
  if (!host) return;
  if (!roomProjectId()) {
    host.innerHTML = '<div class="room-empty"><b>先选择项目</b><p>选好项目后，这里会列出群成员，SOP 可以搜索后派发。</p></div>';
    return;
  }
  const members = room?.members || [];
  const workflows = roomWorkflowChoices();
  host.innerHTML = [
    roomResGroup('成员', members.length, members.length
      ? members.map(m => `<div class="room-res-item static"><span class="room-res-text"><b>${escapeHTML(m.name || ('用户 ' + m.userId))}<span class="room-member-tag">${escapeHTML(roomMemberRoleLabel(m.role))}</span></b></span></div>`).join('')
      : '<div class="room-res-more">这个项目还没有其他成员</div>'),
    roomResGroup('SOP', workflows.length, workflows.length
      ? `<input id="roomSOPSearch" class="room-sop-search" type="search" placeholder="搜索要派发的 SOP" value="${escapeHTML(roomState().sopQuery || '')}"><div id="roomSOPList"></div>`
      : roomNoSOPHint())
  ].join('');
  const search = $('#roomSOPSearch');
  if (search) {
    search.oninput = () => {
      roomState().sopQuery = search.value;
      fillRoomSOPList();
    };
  }
  fillRoomSOPList();
  bindRoomGoSOPs(host);
}

function fillRoomSOPList() {
  const list = $('#roomSOPList');
  if (!list) return;
  const rs = roomState();
  const query = String(rs.sopQuery || '').trim().toLowerCase();
  const picked = new Set(rs.sops);
  const all = roomWorkflowChoices();
  const rest = all.filter(w => !picked.has(w.id));
  const shown = query
    ? all.filter(w => roomWorkflowLabel(w).toLowerCase().includes(query) || String(w.id).toLowerCase().includes(query))
    : (rs.sopExpanded ? all : all.filter(w => picked.has(w.id)).concat(rest.slice(0, 4)));
  const hidden = query || rs.sopExpanded ? 0 : Math.max(0, rest.length - 4);
  const toggle = !query && rest.length > 4
    ? `<button type="button" class="room-res-more room-res-expand" data-room-sop-expand="${rs.sopExpanded ? 'collapse' : 'expand'}">${rs.sopExpanded ? '收起' : '展开其余 ' + hidden + ' 条'}</button>`
    : '';
  list.innerHTML = (shown.length
    ? shown.map(w => roomResItem('≡', 'sop', roomWorkflowLabel(w), (w.nodes || []).length ? (w.nodes.length + ' 个节点') : '点选后进入建单', w.id)).join('')
    : '<div class="room-res-more">没有匹配的 SOP</div>')
    + toggle;
  list.querySelectorAll('[data-room-pick]').forEach(button => {
    button.onclick = () => {
      setRoomMode('issue');
      toggleRoomSOP(button.dataset.roomPick);
    };
  });
  list.querySelector('[data-room-sop-expand]')?.addEventListener('click', event => {
    rs.sopExpanded = event.currentTarget.dataset.roomSopExpand === 'expand';
    fillRoomSOPList();
  });
  syncRoomComposer();
}

function roomTime(iso) {
  const d = new Date(iso);
  if (!iso || isNaN(d) || d.getFullYear() < 2000) return '';
  const hm = d.toTimeString().slice(0, 5);
  return d.toDateString() === new Date().toDateString() ? hm : `${d.getMonth() + 1}/${d.getDate()} ${hm}`;
}

function roomHue(name) {
  return [...String(name || '')].reduce((h, c) => (h * 31 + c.charCodeAt(0)) % 360, 7);
}

// "admin(Administrator)" → {name: 'Administrator', handle: 'admin'}
function roomPerson(author) {
  const match = String(author || '').match(/^(.*?)\((.*)\)$/);
  return match ? {name: match[2] || match[1], handle: match[1]} : {name: String(author || '群成员'), handle: ''};
}

function roomAgentName(id, fallback) {
  const agent = (state.data?.agents || []).find(a => a.id === id);
  return String(agent?.name || fallback || id || 'Agent').replace(/^演示\s*·\s*/, '');
}

const ROOM_ICON_HUMAN = '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="8" r="3.6"/><path d="M5 20c1.2-3.6 4-5.4 7-5.4s5.8 1.8 7 5.4"/></svg>';
const ROOM_ICON_TOOL = '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.7 6.3a4 4 0 0 0-5.4 5.4L4 17l3 3 5.3-5.3a4 4 0 0 0 5.4-5.4l-2.5 2.5-2.5-2.5z"/></svg>';

function roomFace(who, cls = '') {
  const kind = who?.kind || 'person';
  if (kind === 'lf') return `<span class="rf rf-lf ${cls}" title="LoopForge"><i></i></span>`;
  if (kind === 'human') return `<span class="rf rf-human ${cls}" title="${escapeHTML(who.name || '人工确认')}">${ROOM_ICON_HUMAN}</span>`;
  if (kind === 'tool') return `<span class="rf rf-tool ${cls}" title="${escapeHTML(who.name || '脚本')}">${ROOM_ICON_TOOL}</span>`;
  const name = String(who?.name || '?');
  const initial = escapeHTML([...name.replace(/^[^\p{L}\p{N}]+/u, '')][0] || '?').toUpperCase();
  return `<span class="rf ${kind === 'bot' ? 'rf-bot' : 'rf-person'} ${cls}" style="--h:${roomHue(name)}" title="${escapeHTML(name)}">${initial}</span>`;
}

function roomStepWho(step) {
  if (step.type === 'approval') return {kind: 'human', name: '人工确认'};
  if (step.type === 'script') return {kind: 'tool', name: '脚本'};
  if (step.type === 'sop') return {kind: 'lf', name: '子流程'};
  return {kind: 'bot', name: roomAgentName(step.agentId, step.name)};
}

function roomRunGroups(messages) {
  const groups = new Map();
  const order = [];
  messages.forEach(m => {
    if (m.kind === 'crew' && m.crew) {
      order.push({crew: m});
      return;
    }
    if (m.runId && ['dispatch', 'progress', 'approval', 'decision'].includes(m.kind)) {
      let g = groups.get(m.runId);
      if (!g) {
        g = {runId: m.runId, head: null, events: []};
        groups.set(m.runId, g);
        order.push({run: g});
      }
      if (m.kind === 'dispatch' && !g.head) g.head = m;
      else g.events.push(m);
      return;
    }
    order.push({msg: m});
  });
  return {order, groups};
}

const ROOM_STEP_DONE = ['succeeded', 'skipped'];

function roomRunModel(g) {
  const rs = roomState();
  const base = g.head || g.events[0] || {};
  const run = rs.runDetail?.[g.runId] || (state.data?.runs || []).find(r => r.id === g.runId) || null;
  const wf = (state.data?.workflows || []).find(w => w.id === (run?.workflowId || base.workflowId));
  const defs = run?.workflowSnapshot?.nodes || wf?.nodes || [];
  const pending = g.events.filter(e => e.kind === 'approval' && !e.resolved).pop() || null;
  const final = g.events.filter(e => e.kind === 'progress' && !e.nodeId).pop();
  let status = run?.status || base.runFinal || base.runStatus || final?.nodeStatus || (pending ? 'waiting' : 'queued');
  if (pending && !roomRunTerminal(status)) status = 'waiting';
  const stateOf = id => {
    const live = run?.nodes?.[id]?.status;
    if (live) return live;
    const ev = g.events.filter(e => e.nodeId === id).pop();
    if (ev) return ev.kind === 'approval' && !ev.resolved ? 'waiting_approval' : (ev.nodeStatus || 'running');
    return (base.runCurrent || []).includes(defs.find(n => n.id === id)?.name) ? 'running' : 'pending';
  };
  const steps = defs.map(n => ({id: n.id, name: n.name || n.id, type: n.type, agentId: n.agentId, status: stateOf(n.id), detail: run?.nodes?.[n.id] || null}));
  const done = steps.length ? steps.filter(s => ROOM_STEP_DONE.includes(s.status)).length : (base.runCompleted || 0);
  const total = steps.length || base.runTotal || 0;
  const current = steps.find(s => s.status === 'waiting_approval') || steps.find(s => s.status === 'running') || null;
  const updates = steps.filter(s => s.status !== 'pending').length + g.events.filter(e => e.kind === 'decision').length;
  return {g, base, run, steps, pending, status, done, total, current, updates, sop: base.workflowName || roomAuthorName(base.author) || 'SOP'};
}

function roomRunHeadline(m) {
  if (m.base.dispatchError && !m.g.runId) return {cls: 'failed', text: '派发没成功'};
  if (m.status === 'waiting') return {cls: 'waiting', text: `「${m.current?.name || m.pending?.nodeName || '确认'}」等你拍板`};
  if (m.status === 'running' || m.status === 'queued') {
    if (!m.current) return {cls: 'running', text: m.status === 'queued' ? '排队中，马上开始' : '正在推进'};
    return {cls: 'running', text: `${roomStepWho(m.current).name} 正在做「${m.current.name}」`};
  }
  if (m.status === 'succeeded') return {cls: 'succeeded', text: `交付完成 · ${m.total} 步全部通过`};
  if (m.status === 'failed') {
    const bad = m.steps.find(s => s.status === 'failed');
    return {cls: 'failed', text: bad ? `卡在「${bad.name}」` : '运行失败'};
  }
  if (m.status === 'cancelled') return {cls: 'cancelled', text: '已取消'};
  return {cls: m.status, text: roomRunStatusLabel(m.status)};
}

function roomChainHTML(steps, compact) {
  if (!steps.length) return '';
  return `<div class="rc-chain ${compact ? 'compact' : ''}">${steps.map((s, i) => {
    const cls = s.status === 'waiting_approval' ? 'waiting' : s.status;
    const link = i ? `<em class="rc-hand ${ROOM_STEP_DONE.includes(steps[i - 1].status) ? 'done' : ''}"></em>` : '';
    return `${link}<span class="rc-step is-${escapeHTML(cls)}" title="${escapeHTML(roomStepWho(s).name + ' · ' + s.name)}">${roomFace(roomStepWho(s))}<b>${escapeHTML(s.name)}</b></span>`;
  }).join('')}</div>`;
}

function roomAskText(m) {
  const at = m.steps.findIndex(s => s.id === m.pending?.nodeId);
  const prev = at > 0 ? m.steps[at - 1] : null;
  const next = at >= 0 ? m.steps[at + 1] : null;
  const body = String(m.pending?.body || '');
  if (body && !/等待审批|可以直接在群里/.test(body)) return body;
  const from = prev ? `${roomStepWho(prev).name} 已交付「${prev.name}」` : '上一步已完成';
  return next ? `${from}。批准后交给 ${roomStepWho(next).name} 继续「${next.name}」。` : `${from}，批准后这条需求就交付了。`;
}

function roomDecideButtons(e) {
  const attrs = `data-run="${escapeHTML(e.runId)}" data-node="${escapeHTML(e.nodeId)}" data-name="${escapeHTML(e.nodeName || e.nodeId)}"`;
  return `<button type="button" class="rc-btn primary" data-decide="approve" ${attrs}>批准</button>${e.canReject ? `<button type="button" class="rc-btn" data-decide="reject" ${attrs}>打回</button>` : ''}`;
}

function roomAskHTML(m, mine) {
  const b = m.base;
  if (!m.g.head || b.lane) return '';
  const who = roomPerson(b.author);
  const own = mine > 0 && Number(b.userId) === mine;
  return `<article class="rm ${own ? 'mine' : ''}">${roomFace({kind: 'person', name: who.name})}<div class="rm-body"><header><b>${escapeHTML(who.name)}</b>${own ? '<span class="rm-you">你</span>' : ''}<time>${roomTime(b.createdAt)}</time></header><p><span class="room-at bot">@${escapeHTML(m.sop)}</span> ${escapeHTML(b.issueTitle || '')}</p>${b.body ? `<p class="rm-sub">${escapeHTML(b.body)}</p>` : ''}</div></article>`;
}

function roomRunCardHTML(m, mine, bare) {
  const b = m.base;
  const head = roomRunHeadline(m);
  const open = roomState().threadRun === m.g.runId;
  const key = b.issueNumber ? '#' + b.issueNumber : '需求';
  const pct = m.total ? Math.round(m.done * 100 / m.total) : 0;
  const ask = m.pending ? `<div class="rc-ask"><p>${escapeHTML(roomAskText(m))}</p><div class="rc-actions">${roomDecideButtons(m.pending)}</div></div>` : '';
  const error = b.dispatchError ? `<p class="rc-error">${escapeHTML(b.dispatchError)}</p>` : '';
  const people = [...new Map(m.steps.filter(s => s.status !== 'pending').map(s => [roomStepWho(s).name, roomStepWho(s)])).values()].slice(0, 4);
  const foot = m.g.runId
    ? `<footer class="rc-foot"><span class="rc-faces">${people.map(w => roomFace(w, 'xs')).join('')}</span><span>${m.updates ? m.updates + ' 条更新' : '刚开始'}</span><span class="rc-open">${open ? '过程已展开' : '看过程'}<i>›</i></span></footer>`
    : '';
  const card = `<div class="rc is-${escapeHTML(head.cls)} ${open ? 'is-open' : ''}" ${m.g.runId ? `data-open-thread="${escapeHTML(m.g.runId)}" role="button" tabindex="0"` : ''}>
    <div class="rc-top"><span class="rc-key">${escapeHTML(key)}</span><b>${escapeHTML(b.issueTitle || m.sop)}</b>${m.total ? `<span class="rc-count">${m.done}/${m.total}</span>` : ''}</div>
    <div class="rc-now is-${escapeHTML(head.cls)}"><i></i><span>${escapeHTML(head.text)}</span></div>
    ${m.total ? `<div class="rc-bar"><i style="width:${pct}%"></i></div>` : ''}
    ${roomChainHTML(m.steps, true)}${error}${ask}${foot}
  </div>`;
  if (bare) return card;
  return roomAskHTML(m, mine) + roomBotHTML(card, false, m.sop, b.lane ? '' : roomTime(m.g.events[0]?.createdAt || b.createdAt));
}

function roomBotHTML(inner, ephemeral, sopName, time) {
  return `<article class="rm rm-bot ${ephemeral ? 'is-ephemeral' : ''}">${roomFace({kind: 'lf'})}<div class="rm-body"><header><b>${escapeHTML(sopName || 'LoopForge')}</b><span class="rm-tag">BOT</span>${ephemeral ? '<span class="rm-only">仅你可见</span>' : ''}${time ? `<time>${escapeHTML(time)}</time>` : ''}</header>${inner}</div></article>`;
}

function roomQuoteHTML(m) {
  if (!m.replyTo) return '';
  return `<button type="button" class="rm-quote" data-rm-jump="${escapeHTML(m.replyTo)}"><b>${escapeHTML(m.replyAuthor || '')}</b><span>${escapeHTML(m.replyText || '')}</span></button>`;
}

function roomReplyActHTML(id) {
  return `<div class="rm-acts"><button type="button" data-rm-reply="${escapeHTML(id)}" title="回复这条消息">↩ 回复</button></div>`;
}

function roomTextMessageHTML(message, mine) {
  const own = mine > 0 && Number(message.userId) === mine;
  if (message.kind === 'decision') return `<div class="rm-system"><b>${escapeHTML(roomPerson(message.author).name)}</b> ${escapeHTML(message.body || '')}</div>`;
  if (message.kind === 'dispatch') return roomRunCardHTML(roomRunModel({runId: '', head: message, events: []}), mine);
  if (message.kind === 'crew_post') return roomCrewPostHTML(message);
  const who = roomPerson(message.author);
  return `<article class="rm ${own ? 'mine' : ''}" data-mid="${escapeHTML(message.id)}">${roomFace({kind: 'person', name: who.name})}<div class="rm-body"><header><b>${escapeHTML(who.name)}</b>${own ? '<span class="rm-you">你</span>' : ''}<time>${roomTime(message.createdAt)}</time></header>${roomQuoteHTML(message)}<p>${roomTextHTML(message.body || '')}</p></div>${roomReplyActHTML(message.id)}</article>`;
}

function roomDayKey(iso) {
  const d = new Date(iso);
  return isNaN(d) ? '' : d.toDateString();
}

function roomDayLabel(iso) {
  const d = new Date(iso);
  const today = new Date();
  const yesterday = new Date(Date.now() - 864e5);
  if (d.toDateString() === today.toDateString()) return '今天';
  if (d.toDateString() === yesterday.toDateString()) return '昨天';
  return `${d.getMonth() + 1} 月 ${d.getDate()} 日`;
}

function renderRoomThread(room, messages, keepScroll) {
  const log = $('#roomLog');
  if (!log) return;
  const previous = log.scrollTop;
  const rs = roomState();
  rs.active = messages.some(m => (m.kind === 'dispatch' && m.runId && !m.runFinal && !roomRunTerminal(m.runStatus || '')) || (m.kind === 'crew' && ROOM_CREW_ACTIVE.includes(m.crew?.status)) || m.pending);
  rs.lastMessages = messages;
  const ephemeral = (rs.ephemeral || []).filter(e => e.roomId === undefined || e.roomId === room?.id);
  const runsSig = (state.data?.runs || []).map(r => r.id + ':' + (r.revision || r.updatedAt || r.status)).join(',');
  const signature = (room?.id || '') + '|' + JSON.stringify(messages) + '|' + ephemeral.map(e => e.id).join(',') + '|' + runsSig + '|' + (rs.threadRun || '');
  if (signature === rs.threadSignature && log.childElementCount) {
    renderRoomThreadPane();
    return;
  }
  rs.threadSignature = signature;
  renderRoomBots();
  if (!room) {
    log.innerHTML = `<div class="room-hero"><h2>${escapeHTML(roomGreeting())}</h2><p>${roomProjectId() ? '在左侧新建或选择一个群开始协作' : '先在顶栏选择项目'}</p></div>`;
    renderRoomThreadPane();
    return;
  }
  const mine = Number(state.currentUser?.id) || 0;
  const {order, groups} = roomRunGroups(messages);
  rs.runGroups = groups;
  let day = '';
  const rows = order.map(item => {
    const at = item.msg ? item.msg.createdAt : item.crew ? item.crew.createdAt : (item.run.head || item.run.events[0] || {}).createdAt;
    const key = roomDayKey(at);
    const divider = key && key !== day ? `<div class="rm-day"><span>${escapeHTML(roomDayLabel(at))}</span></div>` : '';
    if (key) day = key;
    return divider + (item.msg ? roomTextMessageHTML(item.msg, mine) : item.crew ? roomCrewHTML(item.crew, mine) : roomRunCardHTML(roomRunModel(item.run), mine));
  });
  const body = rows.join('') + ephemeral.map(e => roomBotHTML(e.html, true)).join('');
  log.innerHTML = body ? `<div class="rm-intro">${roomFace({kind: 'lf'}, 'lg')}<div><b>#${escapeHTML(room.name)}</b><p><b>@LoopForge</b> 提问或派活，Bot 们自己分工交接；<b>@</b> 一条 SOP 按固定流程跑。悬停消息点「回复」可以接着问，<b>/</b> 看命令。</p></div></div>` + body : roomGuideHTML();
  bindRoomLog(log);
  if (keepScroll) log.scrollTop = previous;
  else log.scrollTop = log.scrollHeight;
  renderRoomThreadPane();
}

function roomStartReply(id) {
  const m = (roomState().lastMessages || []).find(x => x.id === id);
  if (!m) return;
  const author = m.kind === 'crew' || (m.kind === 'crew_post' && !m.agentId) ? 'LoopForge' : m.agentId ? roomAgentName(m.agentId, m.author) : roomPerson(m.author).name;
  const text = m.kind === 'crew' ? (m.crew?.goal || '') : String(m.body || '');
  roomState().reply = {id, author, text: text.replace(/\s+/g, ' ').slice(0, 60)};
  syncRoomComposer();
  $('#roomInput')?.focus();
}

function bindRoomLog(log) {
  log.querySelectorAll('[data-rm-reply]').forEach(button => {
    button.onclick = event => { event.stopPropagation(); roomStartReply(button.dataset.rmReply); };
  });
  log.querySelectorAll('[data-rm-jump]').forEach(button => {
    button.onclick = event => {
      event.stopPropagation();
      const target = log.querySelector(`[data-mid="${CSS.escape(button.dataset.rmJump)}"]`);
      if (!target) return toast('原消息已经不在最近的记录里');
      target.scrollIntoView({block: 'center', behavior: 'smooth'});
      target.classList.add('is-flash');
      setTimeout(() => target.classList.remove('is-flash'), 1400);
    };
  });
  log.querySelectorAll('[data-room-approve]').forEach(button => {
    button.onclick = async event => {
      event.stopPropagation();
      button.disabled = true;
      if (typeof approveRun === 'function') await approveRun(button.dataset.roomApprove, button.dataset.node, button);
      button.textContent = '已通过';
    };
  });
  log.querySelectorAll('[data-room-suggest]').forEach(button => {
    button.onclick = () => runRoomCommand(button.dataset.roomSuggest);
  });
  log.querySelector('[data-room-try-chat]')?.addEventListener('click', () => setRoomMode('chat'));
  log.querySelector('[data-room-try-issue]')?.addEventListener('click', () => {
    setRoomMode('issue');
    if (!roomTargetPicked()) openRoomMention({targetsOnly: true});
  });
  bindRoomGoSOPs(log);
  log.querySelectorAll('[data-open-issue]').forEach(button => {
    button.onclick = event => { event.stopPropagation(); openIssueDetail(button.dataset.openIssue); };
  });
  log.querySelectorAll('[data-open-run]').forEach(button => {
    button.onclick = event => { event.stopPropagation(); location.hash = '#/runs/' + encodeURIComponent(button.dataset.openRun); };
  });
  log.querySelectorAll('[data-decide]').forEach(button => {
    button.onclick = event => {
      event.stopPropagation();
      decideRoomStep(button.dataset.decide, button.dataset.run, button.dataset.node, button.dataset.name);
    };
  });
  log.querySelectorAll('[data-rm-more]').forEach(button => {
    button.onclick = () => {
      const p = button.previousElementSibling;
      const open = p.classList.toggle('rm-clamp');
      button.textContent = open ? '展开' : '收起';
    };
  });
  log.querySelectorAll('[data-crew-act]').forEach(button => {
    button.onclick = event => {
      event.stopPropagation();
      roomCrewAct(button.dataset.crew, button.dataset.crewAct, button);
    };
  });
  log.querySelectorAll('[data-open-crew]').forEach(card => {
    const open = () => openRoomRunThread('crew:' + card.dataset.openCrew);
    card.onclick = open;
    card.onkeydown = event => { if (event.key === 'Enter') open(); };
  });
  log.querySelectorAll('[data-open-thread]').forEach(card => {
    const open = () => openRoomRunThread(card.dataset.openThread);
    card.onclick = open;
    card.onkeydown = event => { if (event.key === 'Enter') open(); };
  });
}

async function openRoomRunThread(runId) {
  const rs = roomState();
  if (runId && !runId.startsWith('crew:') && !rs.runGroups?.has(runId)) {
    location.hash = '#/runs/' + encodeURIComponent(runId);
    return;
  }
  rs.threadRun = rs.threadRun === runId ? '' : runId;
  rs.threadSignature = '';
  rs.crewEdit = '';
  $('#roomsShell')?.classList.remove('info-open');
  renderRoomThread(activeRoom(), rs.lastMessages || [], true);
  if (rs.threadRun && !runId.startsWith('crew:')) await loadRoomRunDetail(runId);
}

async function loadRoomRunDetail(runId) {
  const rs = roomState();
  try {
    const run = await api('/api/runs/' + encodeURIComponent(runId), {silent: true});
    rs.runDetail = Object.assign(rs.runDetail || {}, {[runId]: run});
    if (rs.threadRun === runId) {
      rs.threadSignature = '';
      renderRoomThread(activeRoom(), rs.lastMessages || [], true);
    }
  } catch {}
}

function roomOutputSummary(text) {
  const lines = String(text || '').split('\n')
    .map(line => line.replace(/^\s*(#{1,6}|>|[-*+]|\d+\.)\s*/, '').replace(/\*\*|`/g, '').trim())
    .filter(Boolean);
  return {title: lines[0] || '', body: lines.slice(1, 7).join('\n')};
}

function roomDuration(detail) {
  const a = Date.parse(detail?.startedAt);
  const b = Date.parse(detail?.finishedAt);
  if (!(a > 0) || !(b > a)) return '';
  const s = (b - a) / 1000;
  return s < 60 ? (s < 10 ? s.toFixed(1) : Math.round(s)) + ' 秒' : Math.round(s / 60) + ' 分钟';
}

const ROOM_STEP_VERB = {succeeded: '完成了', failed: '没做成', running: '正在做', waiting_approval: '等你确认', cancelled: '取消了', skipped: '跳过了'};

function roomThreadStepHTML(step, next, model) {
  const who = roomStepWho(step);
  const verb = ROOM_STEP_VERB[step.status] || '';
  const d = step.detail || {};
  const sum = roomOutputSummary(d.output || d.error || '');
  const meta = [roomDuration(d), Number(d.toolCalls) ? `${d.toolCalls} 次工具调用` : '', Number(d.attempt) > 1 ? `第 ${d.attempt} 次尝试` : '', d.gateResult?.decision === 'pass' ? '自检通过' : ''].filter(Boolean);
  const ask = step.status === 'waiting_approval' && model.pending && model.pending.nodeId === step.id
    ? `<div class="rc-ask"><p>${escapeHTML(roomAskText(model))}</p><div class="rc-actions">${roomDecideButtons(model.pending)}</div></div>`
    : '';
  const decisions = model.g.events.filter(e => e.kind === 'decision' && e.nodeId === step.id)
    .map(e => `<div class="rm-system"><b>${escapeHTML(roomPerson(e.author).name)}</b> ${escapeHTML(e.body || '')}</div>`).join('');
  const output = sum.title ? `<div class="rt-out"><b>${escapeHTML(sum.title)}</b>${sum.body ? `<p>${escapeHTML(sum.body)}</p>` : ''}</div>` : '';
  const hand = next && ROOM_STEP_DONE.includes(step.status)
    ? `<div class="rt-hand"><span>交给</span>${roomFace(roomStepWho(next), 'xs')}<b>${escapeHTML(roomStepWho(next).name)}</b><span>·「${escapeHTML(next.name)}」</span></div>`
    : '';
  return `<div class="rt-step is-${escapeHTML(step.status === 'waiting_approval' ? 'waiting' : step.status)}">
    <div class="rt-rail">${roomFace(who)}<i></i></div>
    <div class="rt-main">
      <header><b>${escapeHTML(who.name)}</b><span>${escapeHTML(verb)}「${escapeHTML(step.name)}」</span><time>${escapeHTML(roomTime(d.finishedAt && d.status !== 'running' ? d.finishedAt : d.startedAt))}</time></header>
      ${output}${meta.length ? `<small class="rt-meta">${escapeHTML(meta.join(' · '))}</small>` : ''}${ask}${decisions}${hand}
    </div>
  </div>`;
}

function renderRoomThreadPane() {
  const pane = $('#roomThread');
  const shell = $('#roomsShell');
  if (!pane) return;
  const rs = roomState();
  if (rs.threadRun?.startsWith('crew:')) {
    const crew = (rs.lastMessages || []).find(m => m.id === rs.threadRun.slice(5) && m.crew);
    shell?.classList.toggle('thread-open', !!crew);
    pane.hidden = !crew;
    if (crew) renderRoomCrewPane(pane, crew);
    return;
  }
  const group = rs.threadRun ? rs.runGroups?.get(rs.threadRun) : null;
  shell?.classList.toggle('thread-open', !!group);
  pane.hidden = !group;
  if (!group) return;
  const m = roomRunModel(group);
  const b = m.base;
  const head = roomRunHeadline(m);
  const started = m.steps.filter(s => s.status !== 'pending');
  const later = m.steps.filter(s => s.status === 'pending');
  const mine = Number(state.currentUser?.id) || 0;
  const who = roomPerson(b.author);
  const html = `<header class="rt-head"><div><small>执行过程 · ${escapeHTML(m.sop)}</small><b>${escapeHTML((b.issueNumber ? '#' + b.issueNumber + ' ' : '') + (b.issueTitle || m.sop))}</b></div><button type="button" class="rooms-icon-btn" data-close-thread title="关闭">×</button></header>
  <div class="rt-scroll">
    ${b.issueTitle ? `<article class="rm rt-origin ${mine && Number(b.userId) === mine ? 'mine' : ''}">${roomFace({kind: 'person', name: who.name})}<div class="rm-body"><header><b>${escapeHTML(who.name)}</b><time>${roomTime(b.createdAt)}</time></header><p><span class="room-at bot">@${escapeHTML(m.sop)}</span> ${escapeHTML(b.issueTitle)}</p>${b.body ? `<p class="rm-sub">${escapeHTML(b.body)}</p>` : ''}</div></article>` : ''}
    <div class="rt-status is-${escapeHTML(head.cls)}"><i></i><span>${escapeHTML(head.text)}</span>${m.total ? `<em>${m.done}/${m.total}</em>` : ''}</div>
    <div class="rt-steps">${started.map(s => roomThreadStepHTML(s, m.steps[m.steps.indexOf(s) + 1], m)).join('') || '<p class="rt-empty">Bot 正在接活，第一步开始后这里会实时更新。</p>'}</div>
    ${later.length ? `<div class="rt-later"><span>接下来</span>${later.map(s => `<span class="rt-later-step">${roomFace(roomStepWho(s), 'xs')}${escapeHTML(s.name)}</span>`).join('<em>→</em>')}</div>` : ''}
  </div>
  <footer class="rt-foot">${b.issueId ? `<button type="button" class="rc-btn" data-open-issue="${escapeHTML(b.issueId)}">打开需求</button>` : ''}<button type="button" class="rc-btn" data-open-run="${escapeHTML(m.g.runId)}">运行详情 ›</button></footer>`;
  if (pane.dataset.sig === html) return;
  const scroll = pane.querySelector('.rt-scroll')?.scrollTop || 0;
  pane.dataset.sig = html;
  pane.innerHTML = html;
  pane.querySelector('.rt-scroll').scrollTop = scroll;
  pane.querySelector('[data-close-thread]').onclick = () => openRoomRunThread(rs.threadRun);
  bindRoomLog(pane);
}

function roomBotsInProject() {
  const used = new Set();
  roomWorkflowChoices().forEach(w => (w.nodes || []).forEach(n => n.agentId && used.add(n.agentId)));
  const agents = (state.data?.agents || []).filter(a => used.has(a.id));
  return (agents.length ? agents : (state.data?.agents || [])).slice(0, 8);
}

function renderRoomBots() {
  const host = $('#roomBots');
  if (!host) return;
  const runs = typeof roomRunsInProject === 'function' ? roomRunsInProject() : [];
  const busy = new Map();
  let waiting = 0;
  runs.filter(r => ['running', 'waiting', 'queued'].includes(r.status)).forEach(r => {
    const defs = r.workflowSnapshot?.nodes || [];
    defs.forEach(n => {
      const st = r.nodes?.[n.id]?.status;
      if (st === 'running' && n.agentId) busy.set(n.agentId, n.name || n.id);
      if (st === 'waiting_approval') waiting++;
    });
  });
  const bots = roomBotsInProject();
  const row = (face, name, sub, live, attrs = '') => `<button type="button" class="rb ${live ? 'is-live' : ''}" ${attrs}>${face}<span><b>${escapeHTML(name)}</b><small>${escapeHTML(sub)}</small></span>${live ? '<i class="rb-dot"></i>' : ''}</button>`;
  host.innerHTML = `<div class="rooms-pane-sub">Bot 队伍 <em>${bots.length + 1}</em></div>`
    + row(roomFace({kind: 'lf'}), 'LoopForge', waiting ? `${waiting} 件事等你拍板` : '派活、交接、汇报', waiting > 0, 'data-room-bot-help')
    + bots.map(a => row(roomFace({kind: 'bot', name: roomAgentName(a.id, a.name)}), roomAgentName(a.id, a.name), busy.has(a.id) ? `正在做「${busy.get(a.id)}」` : (a.description || '空闲').split('\n')[0].slice(0, 24), busy.has(a.id), `data-room-bot="${escapeHTML(a.id)}"`)).join('');
  host.querySelector('[data-room-bot-help]')?.addEventListener('click', () => runRoomCommand('help'));
  host.querySelectorAll('[data-room-bot]').forEach(button => {
    button.onclick = () => {
      const agent = (state.data?.agents || []).find(a => a.id === button.dataset.roomBot);
      if (agent && typeof openAgentWizard === 'function') openAgentWizard(agent);
    };
  });
  const faces = $('#roomFaces');
  const room = activeRoom();
  if (faces) {
    const members = (room?.members || []).map(m => roomFace({kind: 'person', name: roomPerson(m.name).name}, 'sm'));
    const botFaces = [roomFace({kind: 'lf'}, 'sm')].concat(bots.slice(0, 3).map(a => roomFace({kind: 'bot', name: roomAgentName(a.id, a.name)}, 'sm')));
    faces.innerHTML = room ? `<span class="room-faces-stack">${members.slice(0, 3).join('')}${botFaces.join('')}</span><small>${(room.members || []).length} 人 · ${bots.length + 1} Bot</small>` : '';
  }
}

const ROOM_CREW_ACTIVE = ['planning', 'proposed', 'running', 'waiting'];

function roomCrewSteps(c) {
  return (c.tasks || []).map(t => ({id: t.id, name: t.title, type: 'agent', agentId: t.agentId, status: t.status, task: t}));
}

function roomCrewHeadline(c) {
  const tasks = c.tasks || [];
  const cur = tasks.find(t => t.status === 'running');
  const name = id => roomAgentName(id, (tasks.find(t => t.agentId === id) || {}).agentName);
  switch (c.status) {
    case 'planning': return {cls: 'running', text: '队长正在排分工…'};
    case 'proposed': return {cls: 'waiting', text: `分工排好了：${new Set(tasks.map(t => t.agentId)).size} 个 Bot、${tasks.length} 步，等你开工`};
    case 'running': return {cls: 'running', text: cur ? `${roomAgentName(cur.agentId, cur.agentName)} 正在做「${cur.title}」` : '交接中…'};
    case 'waiting': {
      if (c.question) return {cls: 'waiting', text: `${name(c.questionAgent)} 在等你回答`};
      const bad = [...tasks].reverse().find(t => t.status === 'failed');
      return {cls: 'waiting', text: bad ? `卡在「${bad.title}」，等你决定` : '等你决定'};
    }
    case 'done': return {cls: 'succeeded', text: `交付完成 · ${tasks.filter(t => t.status === 'succeeded').length} 步`};
    case 'stopped': return {cls: 'cancelled', text: '已停止'};
    default: return {cls: 'failed', text: c.error || '没能组队'};
  }
}

function roomCrewActionsHTML(msg, inPane) {
  const c = msg.crew;
  const at = `data-crew="${escapeHTML(msg.id)}"`;
  const btn = (act, label, primary) => `<button type="button" class="rc-btn ${primary ? 'primary' : ''}" data-crew-act="${act}" ${at}>${label}</button>`;
  const stuck = c.status === 'waiting' && !c.question;
  const parts = [];
  if (c.status === 'proposed') parts.push(btn('start', '开工', true), btn('edit', '改分工'));
  if (stuck) parts.push(btn('retry', '重试', true), btn('skip', '跳过'));
  if (c.status === 'running' || c.status === 'waiting') {
    if (inPane) parts.push(btn('edit', '改后面的分工'));
    parts.push(btn('stop', '停止'));
  }
  if (c.status === 'done') parts.push(c.savedWorkflowId ? btn('open-sop', '打开沉淀的 SOP') : btn('sop', '沉淀为 SOP', true));
  return parts.length ? `<div class="rc-actions">${parts.join('')}</div>` : '';
}

function roomCrewHTML(msg, mine) {
  const c = msg.crew;
  const who = roomPerson(msg.author);
  const own = mine > 0 && Number(msg.userId) === mine;
  const ask = c.source === 'chat' ? ''
    : c.source === 'home'
    ? `<div class="rm-system">${escapeHTML(who.name)} 在首页把这件事交给了群里的 Bot 团队</div>`
    : `<article class="rm ${own ? 'mine' : ''}">${roomFace({kind: 'person', name: who.name})}<div class="rm-body"><header><b>${escapeHTML(who.name)}</b>${own ? '<span class="rm-you">你</span>' : ''}<time>${roomTime(msg.createdAt)}</time></header><p><span class="room-at bot">@LoopForge</span> ${escapeHTML(c.goal || '')}</p>${c.detail ? `<p class="rm-sub">${escapeHTML(c.detail)}</p>` : ''}</div></article>`;
  const head = roomCrewHeadline(c);
  const tasks = c.tasks || [];
  const done = tasks.filter(t => ['succeeded', 'skipped'].includes(t.status)).length;
  const pct = tasks.length ? Math.round(done * 100 / tasks.length) : 0;
  const open = roomState().threadRun === 'crew:' + msg.id;
  const question = c.status === 'waiting'
    ? `<div class="rc-ask"><p>${escapeHTML(c.question ? c.question + '（直接在群里回复，队长会交回给它）' : '这一步没做成。可以重试、跳过，或者直接在群里说怎么改。')}</p>${roomCrewActionsHTML(msg)}</div>`
    : '';
  const actions = c.status === 'waiting' ? '' : roomCrewActionsHTML(msg);
  const faces = [...new Map(tasks.map(t => [t.agentId, {kind: 'bot', name: roomAgentName(t.agentId, t.agentName)}])).values()].slice(0, 5);
  const card = `<div class="rc is-crew is-${escapeHTML(head.cls)} ${open ? 'is-open' : ''}" data-open-crew="${escapeHTML(msg.id)}" role="button" tabindex="0">
    <div class="rc-top"><span class="rc-key">${msg.issueNumber ? '#' + msg.issueNumber : '协作'}</span><b>${escapeHTML(c.goal || '')}</b>${tasks.length ? `<span class="rc-count">${done}/${tasks.length}</span>` : ''}</div>
    <div class="rc-now is-${escapeHTML(head.cls)}"><i></i><span>${escapeHTML(head.text)}</span></div>
    ${tasks.length ? `<div class="rc-bar"><i style="width:${pct}%"></i></div>` : ''}
    ${roomChainHTML(roomCrewSteps(c), true)}${question}${actions ? `<div class="rc-row">${actions}</div>` : ''}
    <footer class="rc-foot"><span class="rc-faces">${faces.map(w => roomFace(w, 'xs')).join('')}</span><span>自由协作${c.planner === 'ai' ? ' · AI 排的分工' : ''}</span><span class="rc-open">${open ? '过程已展开' : '看过程'}<i>›</i></span></footer>
  </div>`;
  return ask + roomBotHTML(card, false, 'LoopForge', roomTime(msg.createdAt)).replace('<span class="rm-tag">BOT</span>', '<span class="rm-tag">BOT</span><span class="rm-only">队长</span>').replace('<article class="rm rm-bot ', `<article data-mid="${escapeHTML(msg.id)}" class="rm rm-bot `).replace(/<\/article>$/, roomReplyActHTML(msg.id) + '</article>');
}

function roomCrewPostHTML(m) {
  const lf = !m.agentId;
  const name = lf ? 'LoopForge' : roomAgentName(m.agentId, m.author);
  const face = lf ? roomFace({kind: 'lf'}) : roomFace({kind: 'bot', name});
  const body = String(m.body || '');
  const long = body.split('\n').length > 4 || body.length > 260;
  const hand = m.targetName ? `<div class="rt-hand"><span>交给</span>${roomFace({kind: 'bot', name: m.targetName}, 'xs')}<b>${escapeHTML(m.targetName)}</b><span>·「${escapeHTML(m.nodeName || '')}」</span></div>` : '';
  const stale = m.pending && Date.now() - Date.parse(m.createdAt || '') > 180000;
  const text = m.pending
    ? (stale ? '<p class="rm-sub">这次没答上来，可以再问一次。</p>' : '<p class="rm-typing" aria-label="正在思考"><i></i><i></i><i></i><span>正在思考…</span></p>')
    : `<p class="${long ? 'rm-clamp' : ''}">${roomTextHTML(body)}</p>${long ? '<button type="button" class="rm-more" data-rm-more>展开</button>' : ''}`;
  return `<article class="rm rm-bot rm-crew ${lf ? 'is-captain' : ''}" data-mid="${escapeHTML(m.id)}">${face}<div class="rm-body"><header><b>${escapeHTML(name)}</b><span class="rm-tag">BOT</span><time>${roomTime(m.createdAt)}</time>${lf || !m.crewId ? '' : `<button type="button" class="rm-thread-link" data-open-crew="${escapeHTML(m.crewId || '')}">看过程</button>`}</header>${roomQuoteHTML(m)}${text}${hand}</div>${m.pending ? '' : roomReplyActHTML(m.id)}</article>`;
}

async function roomCrewAct(id, act, button) {
  const rs = roomState();
  const msg = (rs.lastMessages || []).find(m => m.id === id);
  if (!msg) return;
  if (act === 'edit') {
    rs.threadRun = 'crew:' + id;
    rs.crewEdit = id;
    rs.threadSignature = '';
    renderRoomThread(activeRoom(), rs.lastMessages || [], true);
    return;
  }
  if (act === 'open-sop') return roomOpenSavedSOP(msg.crew.savedWorkflowId);
  if (act === 'stop' && typeof afConfirm === 'function' && !(await afConfirm({title: '让 Bot 们停下？', message: '正在做的那一步会被取消，做完的部分保留在需求单里。', confirmText: '停止'}))) return;
  if (button) button.disabled = true;
  try {
    const result = await api('/api/rooms/' + encodeURIComponent(rs.activeId) + '/crew/' + encodeURIComponent(id), {method: 'POST', body: JSON.stringify({action: act})});
    if (act === 'sop' && result.workflowId) {
      toast('已沉淀成 SOP 草稿，可以在流程设计里调整后发布');
      await roomRememberWorkflow(result.workflowId);
    }
    await refreshRoomMessages(true);
  } catch (error) {
    toast(error.message);
    if (button) button.disabled = false;
  }
}

async function roomRememberWorkflow(id) {
  try {
    const wf = await api('/api/workflows/' + encodeURIComponent(id), {silent: true});
    const list = state.data.workflows || (state.data.workflows = []);
    const at = list.findIndex(w => w.id === id);
    if (at >= 0) list[at] = wf;
    else list.unshift(wf);
    return wf;
  } catch {
    return null;
  }
}

async function roomOpenSavedSOP(id) {
  if (!id) return;
  if (!(state.data.workflows || []).some(w => w.id === id)) await roomRememberWorkflow(id);
  location.hash = '#/flow/' + encodeURIComponent(id);
}

function roomCrewEditorHTML(msg) {
  const c = msg.crew;
  const bots = roomBotsInProject();
  const pending = (c.tasks || []).filter(t => t.status === 'pending');
  const row = (t, i) => `<div class="rce-row" data-rce="${i}">
    <select data-rce-agent>${bots.map(a => `<option value="${escapeHTML(a.id)}" ${a.id === t.agentId ? 'selected' : ''}>${escapeHTML(roomAgentName(a.id, a.name))}</option>`).join('')}</select>
    <input data-rce-title value="${escapeHTML(t.title || '')}" maxlength="40" placeholder="这一步做什么">
    <button type="button" class="rooms-icon-btn" data-rce-del title="删掉这一步">×</button>
    <textarea data-rce-ins rows="2" maxlength="600" placeholder="给这个 Bot 的要求（可选）">${escapeHTML(t.instruction || '')}</textarea>
  </div>`;
  return `<div class="rce"><div class="rce-head"><b>改分工</b><small>${c.status === 'proposed' ? '开工前随便改' : '只改还没开始的步骤'}</small></div>
    <div class="rce-list">${pending.map(row).join('') || '<p class="rt-empty">后面没有排队的步骤了。</p>'}</div>
    <button type="button" class="rce-add" data-rce-add>＋ 加一步</button>
    <div class="rc-actions"><button type="button" class="rc-btn primary" data-rce-save>保存分工</button><button type="button" class="rc-btn" data-rce-cancel>取消</button></div></div>`;
}

function bindRoomCrewEditor(pane, msg) {
  const rs = roomState();
  const list = pane.querySelector('.rce-list');
  if (!list) return;
  pane.querySelectorAll('[data-rce-del]').forEach(b => { b.onclick = () => b.closest('.rce-row').remove(); });
  pane.querySelector('[data-rce-add]').onclick = () => {
    const bots = roomBotsInProject();
    const wrap = document.createElement('div');
    wrap.innerHTML = roomCrewEditorHTML({crew: {status: msg.crew.status, tasks: [{agentId: bots[0]?.id, title: '', status: 'pending'}]}});
    const row = wrap.querySelector('.rce-row');
    list.querySelector('.rt-empty')?.remove();
    list.appendChild(row);
    row.querySelector('[data-rce-del]').onclick = () => row.remove();
    row.querySelector('[data-rce-title]').focus();
  };
  pane.querySelector('[data-rce-cancel]').onclick = () => {
    rs.crewEdit = '';
    pane.dataset.sig = '';
    renderRoomThreadPane();
  };
  pane.querySelector('[data-rce-save]').onclick = async event => {
    const tasks = [...list.querySelectorAll('.rce-row')].map(r => ({agentId: r.querySelector('[data-rce-agent]').value, title: r.querySelector('[data-rce-title]').value.trim(), instruction: r.querySelector('[data-rce-ins]').value.trim()}));
    event.currentTarget.disabled = true;
    try {
      await api('/api/rooms/' + encodeURIComponent(rs.activeId) + '/crew/' + encodeURIComponent(msg.id), {method: 'POST', body: JSON.stringify({action: 'plan', tasks})});
      rs.crewEdit = '';
      toast('分工已更新');
      await refreshRoomMessages(true);
    } catch (error) {
      toast(error.message);
      event.currentTarget.disabled = false;
    }
  };
}

function renderRoomCrewPane(pane, msg) {
  const rs = roomState();
  const c = msg.crew;
  const tasks = c.tasks || [];
  const head = roomCrewHeadline(c);
  const steps = roomCrewSteps(c);
  const started = steps.filter(s => s.status !== 'pending');
  const later = steps.filter(s => s.status === 'pending');
  const editing = rs.crewEdit === msg.id && ROOM_CREW_ACTIVE.includes(c.status);
  const stepHTML = (s, i) => {
    const t = s.task;
    const who = roomStepWho(s);
    const sum = roomOutputSummary(t.output || t.summary || t.error || '');
    const verb = {succeeded: '完成了', failed: '没做成', running: '正在做', skipped: '跳过了'}[t.status] || '';
    const next = steps[steps.indexOf(s) + 1];
    const meta = [roomDuration({startedAt: t.startedAt, finishedAt: t.finishedAt}), t.addedBy && t.addedBy !== 'LoopForge' ? `${roomPerson(t.addedBy).name} 在群里加的` : ''].filter(Boolean);
    return `<div class="rt-step is-${escapeHTML(t.status)}">
      <div class="rt-rail">${roomFace(who)}<i></i></div>
      <div class="rt-main">
        <header><b>${escapeHTML(who.name)}</b><span>${escapeHTML(verb)}「${escapeHTML(t.title)}」</span><time>${escapeHTML(roomTime(t.finishedAt || t.startedAt))}</time></header>
        ${t.instruction ? `<small class="rt-brief">要求：${escapeHTML(t.instruction)}</small>` : ''}
        ${sum.title ? `<div class="rt-out ${t.status === 'failed' ? 'is-bad' : ''}"><b>${escapeHTML(sum.title)}</b>${sum.body ? `<p>${escapeHTML(sum.body)}</p>` : ''}</div>` : ''}
        ${meta.length ? `<small class="rt-meta">${escapeHTML(meta.join(' · '))}</small>` : ''}
        ${t.runId ? `<button type="button" class="room-link rt-run-link" data-open-run="${escapeHTML(t.runId)}">看这一步的运行 ›</button>` : ''}
        ${next && t.status === 'succeeded' ? `<div class="rt-hand"><span>交给</span>${roomFace(roomStepWho(next), 'xs')}<b>${escapeHTML(roomStepWho(next).name)}</b><span>·「${escapeHTML(next.name)}」</span></div>` : ''}
      </div></div>`;
  };
  const notes = (c.notes || []).length ? `<div class="rt-notes"><span>群里的要求 · 已带给 Bot</span>${c.notes.map(n => `<p><b>${escapeHTML(roomPerson(n.author).name)}</b>${escapeHTML(n.body)}</p>`).join('')}</div>` : '';
  const html = `<header class="rt-head"><div><small>执行过程 · 自由协作${c.planner === 'ai' ? ' · AI 排的分工' : ''}</small><b>${escapeHTML((msg.issueNumber ? '#' + msg.issueNumber + ' ' : '') + (c.goal || ''))}</b></div><button type="button" class="rooms-icon-btn" data-close-thread title="关闭">×</button></header>
  <div class="rt-scroll">
    <div class="rt-status is-${escapeHTML(head.cls)}"><i></i><span>${escapeHTML(head.text)}</span>${tasks.length ? `<em>${tasks.filter(t => t.status === 'succeeded').length}/${tasks.length}</em>` : ''}</div>
    ${c.question && c.status === 'waiting' ? `<div class="rc-ask rt-ask"><p>${escapeHTML(c.question)}</p><small>直接在群里回复，队长会交回给它继续。</small></div>` : ''}
    ${editing ? roomCrewEditorHTML(msg) : `<div class="rt-steps">${started.map(stepHTML).join('') || `<p class="rt-empty">${c.status === 'planning' ? '队长正在看有哪些 Bot 能干这个活…' : '还没开工。'}</p>`}</div>
    ${later.length ? `<div class="rt-later"><span>接下来</span>${later.map(s => `<span class="rt-later-step">${roomFace(roomStepWho(s), 'xs')}${escapeHTML(s.name)}</span>`).join('<em>→</em>')}</div>` : ''}`}
    ${notes}
  </div>
  <footer class="rt-foot">${editing ? '' : roomCrewActionsHTML(msg, true).replace('rc-actions', 'rc-actions rt-foot-actions')}${msg.issueId ? `<button type="button" class="rc-btn" data-open-issue="${escapeHTML(msg.issueId)}">打开需求</button>` : ''}</footer>`;
  if (pane.dataset.sig === html) return;
  const scroll = pane.querySelector('.rt-scroll')?.scrollTop || 0;
  pane.dataset.sig = html;
  pane.innerHTML = html;
  pane.querySelector('.rt-scroll').scrollTop = scroll;
  pane.querySelector('[data-close-thread]').onclick = () => openRoomRunThread(rs.threadRun);
  bindRoomLog(pane);
  if (editing) bindRoomCrewEditor(pane, msg);
}

function roomRunStatusLabel(status) {
  return {running: '运行中', waiting: '等待处理', succeeded: '已完成', failed: '失败', cancelled: '已取消', queued: '排队中'}[status] || status || '排队中';
}

function roomIsSOPAuthor(name) {
  return /^SOP「.*」$/.test(String(name || ''));
}

function roomAuthorName(name) {
  return roomIsSOPAuthor(name) ? String(name).slice(4, -1) : String(name || '');
}

// roomTextHTML escapes a chat line and highlights @mentions; mentions of the
// current user get an extra class so they stand out.
function roomTextHTML(text) {
  const me = String(state.currentUser?.loginName || '').toLowerCase();
  const names = (activeRoom()?.members || []).map(m => escapeHTML(m.name || '')).filter(Boolean)
    .sort((a, b) => b.length - a.length)
    .map(name => name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
  const pattern = new RegExp(`(^|\\s)@(${names.length ? names.join('|') + '|' : ''}[A-Za-z0-9_.-]+)`, 'g');
  return escapeHTML(text).replace(pattern, (all, lead, name) => {
    const mine = me && name.toLowerCase().startsWith(me);
    return `${lead}<span class="room-at${mine ? ' me' : ''}">@${name}</span>`;
  });
}

function roomNoSOPHint() {
  return '<div class="room-res-more">当前项目还没有可派发的 SOP。<button type="button" class="room-link" data-room-go-sops>去「流程模板」新建或复制一条</button></div>';
}

function bindRoomGoSOPs(host) {
  host?.querySelectorAll('[data-room-go-sops]').forEach(button => {
    button.onclick = () => switchView('sops');
  });
}

function roomGuideHTML() {
  const noSOP = !roomWorkflowChoices().length;
  return `<div class="room-guide">
    <span class="room-guide-orb"></span>
    <h2>${escapeHTML(roomGreeting())}</h2>
    <p class="room-guide-lead">在群里聊需求，<b>@LoopForge</b> 选一条 SOP 就开跑。进度、待确认和交付都会回到这个群。</p>
    <div class="room-guide-actions">
      <button type="button" class="room-suggest" data-room-try-issue><b>@LoopForge 派一条需求</b><span>选 SOP，第一行写标题</span></button>
      <button type="button" class="room-suggest" data-room-suggest="status"><b>/status</b><span>现在在跑什么、卡在哪</span></button>
      <button type="button" class="room-suggest" data-room-suggest="approve"><b>/approve</b><span>等我拍板的步骤</span></button>
      <button type="button" class="room-suggest" data-room-try-chat><b>发一条消息</b><span>@ 提醒同事</span></button>
    </div>
    ${noSOP ? '<button type="button" class="button ghost small" data-room-go-sops>当前项目还没有 SOP，去流程模板</button>' : ''}
  </div>`;
}

function roomMentionState() {
  const rs = roomState();
  if (!rs.mention) rs.mention = {open: false, items: [], index: 0, start: -1, targetsOnly: false};
  return rs.mention;
}

function roomMentionToken(input) {
  const before = input.value.slice(0, input.selectionStart);
  const match = before.match(/(^|\s)@([^\s@]{0,30})$/);
  return match ? {start: before.length - match[2].length - 1, query: match[2]} : null;
}

function roomMentionItems(query, targetsOnly) {
  const q = String(query || '').trim().toLowerCase();
  const hit = (...values) => !q || values.some(value => String(value || '').toLowerCase().includes(q));
  const items = [];
  if (!targetsOnly && hit('loopforge', '组队', '协作', '群聊', 'crew')) {
    const bots = roomBotsInProject();
    items.push({kind: 'crew', id: 'crew', name: 'LoopForge · 组队来做', sub: '没有固定流程的活：队长拉 Bot 分工，做完一步交接一步', bots});
  }
  const wantSOP = true;
  if (wantSOP) {
    roomWorkflowChoices().forEach(w => {
      if (hit(w.name, w.id)) items.push({kind: 'sop', id: w.id, name: roomWorkflowLabel(w), steps: w.nodes || [], sub: (w.nodes || []).length ? w.nodes.map(n => n.name || n.id).join(' → ') : '派发后立即运行'});
    });
  }
  if (!targetsOnly) {
    roomBotsInProject().forEach(a => {
      const name = roomAgentName(a.id, a.name);
      if (hit(name, a.id)) items.push({kind: 'bot', id: a.id, name, sub: '协作进行中 @ 它，就给它加一步'});
    });
    (activeRoom()?.members || []).forEach(m => {
      if (hit(m.name)) items.push({kind: 'member', id: String(m.userId), name: m.name || '用户 ' + m.userId, sub: roomMemberRoleLabel(m.role)});
    });
  }
  return items.slice(0, 40);
}

function openRoomMention({targetsOnly = false, token = null} = {}) {
  const ms = roomMentionState();
  ms.open = true;
  ms.targetsOnly = targetsOnly;
  ms.start = token ? token.start : -1;
  ms.items = roomMentionItems(token?.query || '', targetsOnly);
  ms.index = 0;
  renderRoomMention(token?.query || '');
}

function closeRoomMention() {
  const ms = roomMentionState();
  ms.targetsOnly = false;
  if (!ms.open) return;
  ms.open = false;
  const box = $('#roomMention');
  if (box) box.hidden = true;
}

const ROOM_COMMANDS = [
  {id: 'sop', name: '/sop', sub: '选一条 SOP，把这条需求派出去'},
  {id: 'status', name: '/status', sub: '现在有哪些在跑、卡在哪'},
  {id: 'approve', name: '/approve', sub: '列出等我拍板的步骤，一键通过'},
  {id: 'help', name: '/help', sub: '看看 LoopForge 在群里能做什么'},
];
function roomSlashQuery(input) {
  const match = String(input?.value || '').match(/^\/(\S*)$/);
  return match ? match[1].toLowerCase() : null;
}
function roomAutosize(input) {
  if (!input) return;
  input.style.height = 'auto';
  input.style.height = Math.min(200, input.scrollHeight) + 'px';
}

function updateRoomMention() {
  const input = $('#roomInput');
  if (!input) return;
  const slash = roomSlashQuery(input);
  if (slash !== null) {
    const ms = roomMentionState();
    ms.open = true;
    ms.targetsOnly = false;
    ms.start = -1;
    ms.slash = true;
    ms.items = ROOM_COMMANDS.filter(c => c.id.startsWith(slash) || !slash).map(c => ({...c, kind: 'cmd'}));
    ms.index = Math.min(ms.index || 0, Math.max(0, ms.items.length - 1));
    renderRoomMention(slash);
    return;
  }
  roomMentionState().slash = false;
  const token = roomMentionToken(input);
  if (token && (!token.query || roomMentionItems(token.query, false).length)) openRoomMention({token});
  else if (!roomMentionState().targetsOnly) closeRoomMention();
}

function renderRoomMention(query) {
  const box = $('#roomMention');
  const ms = roomMentionState();
  if (!box) return;
  const groups = [['cmd', '命令'], ['crew', '自由协作 · 交给群聊'], ['sop', '按 SOP 跑 · 固定流程'], ['bot', '角色 Bot'], ['member', '提醒群成员']];
  const icon = item => item.kind === 'crew'
    ? `<span class="rmn-team">${roomFace({kind: 'lf'}, 'sm')}${(item.bots || []).slice(0, 3).map(a => roomFace({kind: 'bot', name: roomAgentName(a.id, a.name)}, 'sm')).join('')}</span>`
    : item.kind === 'bot' ? `<span class="rmn-team">${roomFace({kind: 'bot', name: item.name}, 'sm')}</span>`
    : item.kind === 'sop'
    ? `<span class="rmn-team">${[...new Map((item.steps || []).map(n => [roomStepWho(n).name, roomStepWho(n)])).values()].slice(0, 4).map(w => roomFace(w, 'sm')).join('') || roomFace({kind: 'lf'}, 'sm')}</span>`
    : item.kind === 'cmd' ? `<span class="rmn-cmd">/</span>` : roomFace({kind: 'person', name: roomPerson(item.name).name}, 'sm');
  let html = ms.targetsOnly ? '<div class="room-mention-title">选这条需求要跑的 SOP</div>' : '';
  groups.forEach(([kind, label]) => {
    const rows = ms.items.map((item, index) => ({item, index})).filter(row => row.item.kind === kind);
    if (!rows.length && !(kind === 'sop' && !query && !roomWorkflowChoices().length)) return;
    html += `<div class="room-mention-group">${escapeHTML(label)}</div>`;
    if (!rows.length) html += roomNoSOPHint();
    html += rows.map(({item, index}) => `<button type="button" class="room-mention-item ${index === ms.index ? 'active' : ''}" data-mention="${index}">${icon(item)}<span><b>${escapeHTML(item.name)}</b><small>${escapeHTML(item.sub || '')}</small></span></button>`).join('');
  });
  if (!ms.items.length && !html.includes('room-mention-group')) html += `<div class="room-mention-empty">没有匹配「${escapeHTML(query)}」的${roomState().mode === 'issue' || ms.targetsOnly ? ' SOP 或成员' : '成员'}</div>`;
  box.innerHTML = html;
  box.hidden = false;
  box.querySelectorAll('[data-mention]').forEach(button => {
    button.onmousedown = event => event.preventDefault();
    button.onclick = () => pickRoomMention(ms.items[Number(button.dataset.mention)]);
  });
  bindRoomGoSOPs(box);
  box.querySelector('.room-mention-item.active')?.scrollIntoView({block: 'nearest'});
}

function roomMentionKey(event) {
  const ms = roomMentionState();
  if (!ms.open || event.isComposing) return false;
  if (event.key === 'Escape') {
    event.preventDefault();
    ms.targetsOnly = false;
    closeRoomMention();
    return true;
  }
  if (!ms.items.length) return false;
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault();
    ms.index = (ms.index + (event.key === 'ArrowDown' ? 1 : ms.items.length - 1)) % ms.items.length;
    renderRoomMention(roomMentionToken($('#roomInput'))?.query || '');
    return true;
  }
  if ((event.key === 'Enter' && !event.shiftKey) || event.key === 'Tab') {
    event.preventDefault();
    pickRoomMention(ms.items[ms.index]);
    return true;
  }
  return false;
}

function pickRoomMention(item) {
  const input = $('#roomInput');
  const ms = roomMentionState();
  if (!item || !input) return;
  if (item.kind === 'cmd') {
    input.value = '';
    ms.slash = false;
    closeRoomMention();
    runRoomCommand(item.id);
    return;
  }
  if (ms.start >= 0) {
    const caret = input.selectionStart;
    const insert = item.kind === 'member' || item.kind === 'bot' ? '@' + item.name + ' ' : '';
    input.value = input.value.slice(0, ms.start) + insert + input.value.slice(caret).replace(/^ /, insert ? '' : '');
    const next = ms.start + insert.length;
    input.setSelectionRange(next, next);
  }
  if (item.kind === 'sop') {
    roomState().crew = false;
    roomState().mode = 'issue';
    toggleRoomSOP(item.id, true);
  }
  if (item.kind === 'crew') {
    const rs = roomState();
    rs.sops = [];
    rs.crew = true;
    rs.mode = 'chat';
  }
  ms.targetsOnly = false;
  closeRoomMention();
  syncRoomComposer();
  input.focus();
}

async function decideRoomStep(action, runId, nodeId, name) {
  const id = roomState().activeId;
  if (!id) return;
  let reason = '';
  if (action === 'reject') {
    reason = await afPrompt({title: '打回「' + name + '」', message: '说明要改什么，会带给被打回的步骤。', label: '打回原因', placeholder: '例如：边界情况没覆盖', confirmText: '打回'});
    if (reason == null) return;
  }
  try {
    await api('/api/rooms/' + encodeURIComponent(id) + '/decisions', {method: 'POST', body: JSON.stringify({runId, nodeId, action, reason: String(reason || '').trim()})});
    toast(action === 'approve' ? '已批准' : '已打回');
    if (typeof refreshRuns === 'function') await refreshRuns(true).catch(() => {});
    if (roomState().threadRun === runId) await loadRoomRunDetail(runId);
    await refreshRoomMessages();
  } catch (error) {
    toast(error.message);
  }
}

async function renameRoom() {
  const room = activeRoom();
  if (!room) return toast('先选择一个群');
  const name = await afPrompt({title: '重命名协作群', message: '新名称会显示在左侧群列表和顶栏。', label: '群名称', placeholder: '例如：需求值班群', defaultValue: room.name, confirmText: '保存'});
  if (name == null) return;
  const next = String(name).trim();
  if (!next || next === room.name) return;
  try {
    const updated = await api('/api/rooms/' + encodeURIComponent(room.id), {method: 'PATCH', body: JSON.stringify({name: next})});
    room.name = updated.name || next;
    renderRoomList(roomState().items || []);
    const title = $('#roomTitle');
    if (title) title.textContent = room.name;
    toast('已重命名');
  } catch (error) {
    toast(error.message);
  }
}

async function createRoom() {
  const projectId = roomProjectId();
  if (!projectId) return toast('先在顶栏选择项目');
  const name = await afPrompt({title: '新建协作群', message: '群名称会显示在当前项目的群列表里。', label: '群名称', placeholder: '例如：需求值班群', confirmText: '创建'});
  if (!name || !String(name).trim()) return;
  try {
    const room = await api('/api/rooms', {method: 'POST', body: JSON.stringify({projectId, name: String(name).trim()})});
    roomState().activeId = room.id;
    await loadRooms();
    toast('已创建协作群');
  } catch (error) {
    toast(error.message);
  }
}

function submitRoomComposer() {
  const raw = String($('#roomInput')?.value || '').trim();
  const cmd = raw.match(/^\/(\w+)$/);
  if (cmd && ROOM_COMMANDS.some(c => c.id === cmd[1].toLowerCase())) {
    $('#roomInput').value = '';
    closeRoomMention();
    runRoomCommand(cmd[1].toLowerCase());
    return;
  }
  const rs = roomState();
  if (rs.crew) roomCreateCrew();
  else if (rs.sops.length) dispatchRoomIssue();
  else if (rs.mode === 'issue') openRoomMention({targetsOnly: true});
  else sendRoomMessage();
}

function roomBotReply(html) {
  const rs = roomState();
  if (!rs.activeId) return toast('先选择一个群');
  rs.ephemeral = (rs.ephemeral || []).concat({id: Date.now() + Math.random(), html});
  rs.threadSignature = '';
  refreshRoomMessages(true);
}

function roomRunsInProject() {
  const pid = roomProjectId();
  return (state.data?.runs || []).filter(r => !pid || r.executionContext?.projectId === pid || runContext(r)?.project?.id === pid);
}

function roomRunTitle(r) {
  return runContext(r)?.issue?.title || String(r.input || '').split('\n')[0] || r.workflowSnapshot?.name || r.id;
}

function roomRunLine(r) {
  const group = roomState().runGroups?.get(r.id) || {runId: r.id, head: {issueTitle: roomRunTitle(r), issueNumber: runContext(r)?.issue?.number, workflowId: r.workflowId, workflowName: r.workflowSnapshot?.name}, events: []};
  return roomRunCardHTML(roomRunModel(group), Number(state.currentUser?.id) || 0, true);
}

async function runRoomCommand(id) {
  if (id === 'sop') {
    setRoomMode('issue');
    openRoomMention({targetsOnly: true});
    return;
  }
  if (id === 'help') {
    roomBotReply(`<p>我是 LoopForge，在群里可以这样用我：</p><ul class="room-bot-list"><li><code>@LoopForge</code> 组队：没有固定流程的活交给群里的 Bot 团队，我来排分工，每个 Bot 做完在群里交接，你随时插话、@ 某个 Bot 加活</li><li><code>@</code> 选一条 SOP：有严格流程的活按 SOP 一步步跑，群里只出现一张运行卡片</li><li><code>/status</code> 看现在有哪些在跑、卡在哪一步</li><li><code>/approve</code> 列出等你拍板的步骤，直接在群里通过</li><li>SOP 跑到人工确认时，我会在群里 @ 负责人</li></ul>`);
    return;
  }
  if (typeof refreshRuns === 'function') { try { await refreshRuns(true); } catch {} }
  const runs = roomRunsInProject();
  if (id === 'status') {
    const live = runs.filter(r => ['running', 'queued', 'waiting', 'paused'].includes(r.status));
    if (!live.length) {
      const last = runs[0];
      roomBotReply(`<p>现在没有在跑的 SOP。${last ? `最近一次是「${escapeHTML(roomRunTitle(last))}」，${escapeHTML(roomRunStatusLabel(last.status))}。` : '要开一条吗？输入 <code>@</code> 选 SOP。'}</p>`);
      return;
    }
    const waiting = live.filter(r => r.status === 'waiting').length;
    roomBotReply(`<p>现在有 <b>${live.length}</b> 条在跑${waiting ? `，其中 <b class="is-wait">${waiting}</b> 条在等人拍板` : ''}：</p>${live.slice(0, 6).map(roomRunLine).join('')}`);
    return;
  }
  if (id === 'approve') {
    const items = typeof approvalItems === 'function' ? approvalItems() : runs.filter(r => r.status === 'waiting');
    roomBotReply(items.length
      ? `<p>有 <b class="is-wait">${items.length}</b> 个步骤在等你确认：</p>${items.slice(0, 6).map(roomRunLine).join('')}`
      : '<p>没有等你拍板的事，SOP 都在自己往前跑。</p>');
  }
}

async function sendRoomMessage() {
  const id = roomState().activeId;
  const input = $('#roomInput');
  const body = input?.value.trim();
  if (!id) return toast('先选择一个群');
  if (!body) return;
  try {
    await api('/api/rooms/' + encodeURIComponent(id) + '/messages', {method: 'POST', body: JSON.stringify({body, replyTo: roomState().reply?.id || ''})});
    input.value = '';
    roomState().reply = null;
    closeRoomMention();
    syncRoomComposer();
    await openRoom(id);
  } catch (error) {
    toast(error.message);
  }
}

async function roomCreateCrew() {
  const rs = roomState();
  const id = rs.activeId;
  const input = $('#roomInput');
  const {title, description} = splitRoomIssueText(input?.value);
  if (!id) return toast('先选择一个群');
  if (!title) return toast('第一行写要做成什么');
  const send = $('#roomSend');
  if (send) send.disabled = true;
  try {
    await api('/api/rooms/' + encodeURIComponent(id) + '/crew', {method: 'POST', body: JSON.stringify({goal: title, detail: description})});
    input.value = '';
    rs.crew = false;
    closeRoomMention();
    syncRoomComposer();
    await refreshRoomMessages(true);
  } catch (error) {
    toast(error.message);
  } finally {
    if (send) send.disabled = false;
  }
}

function splitRoomIssueText(text) {
  const lines = String(text || '').split('\n');
  const title = (lines.shift() || '').trim().slice(0, 200);
  return {title, description: lines.join('\n').trim()};
}

async function dispatchRoomIssue() {
  const rs = roomState();
  const id = rs.activeId;
  if (!id) return toast('先选择一个群');
  const {title, description} = splitRoomIssueText($('#roomInput')?.value);
  const workflowIds = rs.sops.slice();
  if (!title) return toast('第一行写 Issue 标题');
  if (!workflowIds.length) return toast('请选择要派发的 SOP');
  const button = $('#roomDispatch');
  button.disabled = true;
  try {
    const result = await api('/api/rooms/' + encodeURIComponent(id) + '/issues', {method: 'POST', body: JSON.stringify({title, description, workflowIds})});
    $('#roomInput').value = '';
    rs.sops = [];
    rs.mode = 'chat';
    closeRoomMention();
    syncRoomComposer();
    if (result.issue) {
      const items = state.data.issues || [];
      const index = items.findIndex(issue => issue.id === result.issue.id);
      if (index >= 0) items[index] = result.issue;
      else items.unshift(result.issue);
      state.data.issues = items;
    }
    await openRoom(id);
    const started = (result.tasks || []).length;
    toast(result.dispatchError ? 'Issue 已创建，部分派发未成功：' + result.dispatchError : (started ? `已建单并启动 ${started} 条 SOP，进度会显示在卡片上` : 'Issue 已创建，但没有启动 SOP'));
  } catch (error) {
    toast(error.message);
  } finally {
    button.disabled = false;
  }
}

bindRooms();
if ((state.currentView || activeViewName()) === 'rooms') loadRooms();
