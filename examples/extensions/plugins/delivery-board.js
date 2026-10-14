// Example web plugin: a "Delivery board" page that lays the current project's
// runs out as a kanban. Load it with LOOPFORGE_WEB_PLUGINS=examples/extensions/plugins.
(function () {
  const { registerView, api, escapeHTML } = window.LoopForge;
  const en = () => document.documentElement.dataset.lang === 'en';
  const L = (zh, enText) => (en() ? enText : zh);
  const columns = [
    { id: 'active', zh: '进行中', en: 'In progress', statuses: ['queued', 'running', 'paused'], tone: 'run' },
    { id: 'waiting', zh: '等你确认', en: 'Needs you', statuses: ['waiting', 'waiting_approval'], tone: 'wait' },
    { id: 'done', zh: '已交付', en: 'Delivered', statuses: ['succeeded'], tone: 'ok' },
    { id: 'failed', zh: '失败', en: 'Failed', statuses: ['failed'], tone: 'bad' },
    { id: 'cancelled', zh: '已取消', en: 'Cancelled', statuses: ['cancelled'], tone: 'mute' },
  ];
  const DAY = 86400000;

  function runTitle(run) {
    const line = String(run.input || '').split('\n').map(s => s.trim()).find(Boolean);
    return line || run.workflowSnapshot?.name || run.workflowId;
  }
  function fmtDuration(ms) {
    if (!ms || ms < 0) return '—';
    const s = Math.round(ms / 1000);
    if (s < 60) return `${s}s`;
    const m = Math.round(s / 60);
    if (m < 60) return `${m}m`;
    const h = Math.floor(m / 60);
    if (h < 24) return `${h}h ${m % 60}m`;
    return `${Math.floor(h / 24)}d ${h % 24}h`;
  }
  function ago(ts) {
    const diff = Date.now() - new Date(ts).getTime();
    if (!isFinite(diff)) return '';
    if (diff < 60000) return L('刚刚', 'just now');
    return L(`${fmtDuration(diff)}前`, `${fmtDuration(diff)} ago`);
  }
  let workflowNodes = new Map();
  function steps(run) {
    const defs = run.workflowSnapshot?.nodes || workflowNodes.get(run.workflowId) || [];
    const states = run.nodes || {};
    return defs.map(n => ({ name: n.name || n.id, status: states[n.id]?.status || 'pending' }));
  }
  function stepStrip(list) {
    return `<div class="db-steps">${list.map(s => `<i class="is-${escapeHTML(s.status)}" title="${escapeHTML(s.name)} · ${escapeHTML(s.status)}"></i>`).join('')}</div>`;
  }
  function card(run) {
    const list = steps(run);
    const done = list.filter(s => s.status === 'succeeded').length;
    const current = list.find(s => ['running', 'waiting', 'failed'].includes(s.status));
    const num = run.executionContext?.issueNumber;
    const wall = run.timing?.wallMs || (new Date(run.updatedAt) - new Date(run.createdAt));
    return `<button class="db-card is-${escapeHTML(run.status)}" data-run="${escapeHTML(run.id)}">
      <div class="db-card-top"><span class="db-key">${num ? `LF-${num}` : 'RUN'}</span><span class="db-sop">${escapeHTML(run.workflowSnapshot?.name || run.workflowId)}</span></div>
      <b class="db-title">${escapeHTML(runTitle(run))}</b>
      ${stepStrip(list)}
      <div class="db-card-foot">
        <span>${current ? `<em>${escapeHTML(current.name)}</em>` : `${done}/${list.length} ${L('步', 'steps')}`}</span>
        <span>${fmtDuration(wall)} · ${ago(run.updatedAt)}</span>
      </div>
    </button>`;
  }
  function trend(runs) {
    const today = new Date(); today.setHours(0, 0, 0, 0);
    const days = Array.from({ length: 14 }, (_, i) => ({ t: today.getTime() - (13 - i) * DAY, ok: 0, other: 0 }));
    for (const run of runs) {
      const t = new Date(run.createdAt).getTime();
      const d = days.find(x => t >= x.t && t < x.t + DAY);
      if (d) run.status === 'succeeded' ? d.ok++ : d.other++;
    }
    const max = Math.max(1, ...days.map(d => d.ok + d.other));
    const bars = days.map((d, i) => {
      const x = i * 14 + 2, hOk = (d.ok / max) * 44, hOther = (d.other / max) * 44;
      return `<rect x="${x}" y="${48 - hOk}" width="9" height="${hOk}" rx="2" class="ok"/><rect x="${x}" y="${48 - hOk - hOther}" width="9" height="${hOther}" rx="2" class="other"/><rect x="${x}" y="46" width="9" height="2" rx="1" class="base"/>`;
    }).join('');
    return `<svg class="db-trend" viewBox="0 0 196 50" preserveAspectRatio="none">${bars}</svg>`;
  }

  registerView({
    id: 'delivery-board',
    title: '大盘',
    icon: '▦',
    async render(el, ctx) {
      el.innerHTML = `<div class="db"><div class="db-loading"><span></span><span></span><span></span></div></div>`;
      const query = ctx.project ? `?projectId=${encodeURIComponent(ctx.project.id)}` : '';
      let runs;
      try {
        const [list, workflows] = await Promise.all([
          api(`/api/runs${query}`, { silent: true }),
          api('/api/workflows', { silent: true }).catch(() => []),
        ]);
        runs = list;
        workflowNodes = new Map((workflows || []).map(w => [w.id, w.nodes || []]));
      } catch (e) {
        el.innerHTML = `<div class="db"><div class="db-empty">${L('加载失败', 'Failed to load')}：${escapeHTML(e.message)}</div></div>`;
        return;
      }
      const week = runs.filter(r => Date.now() - new Date(r.createdAt) < 7 * DAY);
      const delivered = week.filter(r => r.status === 'succeeded');
      const finished = runs.filter(r => ['succeeded', 'failed'].includes(r.status));
      const rate = finished.length ? Math.round((finished.filter(r => r.status === 'succeeded').length / finished.length) * 100) : null;
      const lead = delivered.length ? delivered.reduce((s, r) => s + (r.timing?.wallMs || 0), 0) / delivered.length : 0;
      const waiting = runs.filter(r => r.status === 'waiting');
      const waitMs = waiting.reduce((s, r) => s + (r.timing?.waitingMs || 0), 0);
      const stat = (label, value, sub, tone = '') => `<div class="db-stat ${tone}"><span>${label}</span><b>${value}</b><small>${sub}</small></div>`;
      const cols = columns.map(col => {
        const items = runs.filter(r => col.statuses.includes(r.status));
        return `<section class="db-col tone-${col.tone}">
          <header><i></i><span>${L(col.zh, col.en)}</span><em>${items.length}</em></header>
          <div class="db-col-body">${items.map(card).join('') || `<div class="db-col-empty">${col.id === 'waiting' ? L('没有卡在你这里的事', 'Nothing waiting on you') : '—'}</div>`}</div>
        </section>`;
      }).join('');
      el.innerHTML = `<div class="db">
        <header class="db-head">
          <div><p class="db-eyebrow">${escapeHTML(ctx.project?.name || L('全部项目', 'All projects'))}</p><h1>${L('交付看板', 'Delivery board')}</h1><p class="db-sub">${L('每一次 SOP 运行都是一张卡：从开跑、等你拍板，到交付。', 'Every SOP run is a card — from kickoff, through your sign-off, to delivery.')}</p></div>
          <div class="db-trend-box"><span>${L('近 14 天运行', 'Runs · last 14 days')}</span>${trend(runs)}</div>
        </header>
        <div class="db-stats">
          ${stat(L('本周交付', 'Delivered this week'), delivered.length, L(`共 ${week.length} 次运行`, `${week.length} runs total`), 'is-ok')}
          ${stat(L('成功率', 'Success rate'), rate === null ? '—' : `${rate}%`, L(`${finished.length} 次已结束`, `${finished.length} finished`))}
          ${stat(L('平均交付时长', 'Avg lead time'), fmtDuration(lead), L('开跑 → 交付', 'kickoff → delivered'))}
          ${stat(L('等你确认', 'Waiting on you'), waiting.length, waiting.length ? L(`累计等待 ${fmtDuration(waitMs)}`, `${fmtDuration(waitMs)} waiting`) : L('全部清空', 'All clear'), waiting.length ? 'is-wait' : '')}
        </div>
        <div class="db-board">${cols}</div>
      </div>`;
      el.querySelectorAll('[data-run]').forEach(btn => btn.addEventListener('click', () => { location.hash = `#/runs/${btn.dataset.run}`; }));
    },
  });
})();
