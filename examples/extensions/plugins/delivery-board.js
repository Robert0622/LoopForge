// Example web plugin: a "Delivery board" page that summarizes the current
// project's runs. Load it with LOOPFORGE_WEB_PLUGINS=examples/extensions/plugins.
(function () {
  const { registerView, api, escapeHTML } = window.LoopForge;
  const labels = { running: '运行中', waiting_approval: '等待确认', succeeded: '成功', failed: '失败', cancelled: '已取消' };

  registerView({
    id: 'delivery-board',
    title: '交付看板',
    icon: '▦',
    async render(el, ctx) {
      el.innerHTML = '<div class="empty">加载中…</div>';
      const query = ctx.project ? `?projectId=${encodeURIComponent(ctx.project.id)}` : '';
      let runs;
      try {
        runs = await api(`/api/runs${query}`, { silent: true });
      } catch (e) {
        el.innerHTML = `<div class="empty">加载失败：${escapeHTML(e.message)}</div>`;
        return;
      }
      const counts = {};
      for (const run of runs) counts[run.status] = (counts[run.status] || 0) + 1;
      const tiles = Object.entries(labels).map(([status, label]) =>
        `<div class="delivery-board-tile" data-status="${status}"><b>${counts[status] || 0}</b><span>${label}</span></div>`).join('');
      const recent = runs.slice(0, 10).map(run =>
        `<li><code>${escapeHTML(run.id)}</code> ${escapeHTML(run.workflowSnapshot?.name || run.workflowId)} · ${escapeHTML(labels[run.status] || run.status)}</li>`).join('');
      el.innerHTML = `
        <div class="delivery-board">
          <h2>${escapeHTML(ctx.project?.name || '全部项目')} · 共 ${runs.length} 次运行</h2>
          <div class="delivery-board-tiles">${tiles}</div>
          <ul class="delivery-board-recent">${recent || '<li>还没有运行记录</li>'}</ul>
        </div>`;
    },
  });
})();
