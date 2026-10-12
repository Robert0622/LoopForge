// Environment self-check from /api/doctor: what this deployment still lacks,
// each item with a one-click fix where the platform can do it.
let doctorPollTimer = null;

function loadDoctor(force = false) {
  const user = state.currentUser;
  if (!user || user.isGuest) return Promise.resolve(null);
  if (!force && (state.doctorStatus === 'loading' || state.doctorStatus === 'ready')) return Promise.resolve(state.doctor);
  state.doctorStatus = 'loading';
  return api('/api/doctor', { silent: true })
    .then(report => { state.doctor = report; state.doctorStatus = 'ready'; return report; })
    .catch(() => { state.doctor = null; state.doctorStatus = 'error'; return null; })
    .finally(() => {
      scheduleDoctorPoll();
      if (typeof accessViewActive === 'function' && accessViewActive() && state.accessTab === 'setup') renderGuideSetup();
      if (typeof renderQuickstart === 'function') renderQuickstart();
    });
}

function scheduleDoctorPoll() {
  clearTimeout(doctorPollTimer);
  if (state.doctor?.prewarm?.state === 'building') doctorPollTimer = setTimeout(() => loadDoctor(true), 5000);
}

function doctorPendingChecks() {
  return (state.doctor?.checks || []).filter(c => c.status === 'warn' || c.status === 'error');
}

const DOCTOR_ACTION_LABELS = {
  'setup-wizard': '打开配 Key 向导',
  'git-token': '去填写',
  'prewarm-sandbox': '现在预构建',
};

function doctorCheckHTML(check) {
  const icon = { ok: '✓', warn: '!', error: '×', info: 'i' }[check.status] || 'i';
  const building = check.action === 'prewarm-sandbox' && state.doctor?.prewarm?.state === 'building';
  const label = DOCTOR_ACTION_LABELS[check.action];
  const action = label && !building ? `<button type="button" class="setup-mini" data-doctor-action="${escapeHTML(check.action)}">${label} →</button>` : '';
  return `<li class="doctor-item is-${escapeHTML(check.status)}" data-doctor-check="${escapeHTML(check.id)}"><span class="doctor-icon" aria-hidden="true">${icon}</span><div><b>${escapeHTML(check.label)}</b><small>${escapeHTML(check.detail)}</small></div>${action}</li>`;
}

function doctorCardHTML() {
  if (state.doctorStatus !== 'ready' || !state.doctor) {
    const text = state.doctorStatus === 'error' ? '环境自检加载失败，稍后点「重新检测」。' : '正在检测运行环境…';
    return `<div class="doctor-card"><div class="setup-card-head"><b>环境自检</b><small>部署是否能跑真实需求</small><button type="button" class="setup-mini" data-doctor-action="refresh">重新检测</button></div><p class="doctor-loading">${text}</p></div>`;
  }
  const pending = doctorPendingChecks();
  const summary = !state.doctor.ready
    ? '<span class="setup-status missing">有阻塞项</span>'
    : pending.length ? `<span class="setup-status missing">${pending.length} 项建议处理</span>` : '<span class="setup-status ready">✓ 全部就绪</span>';
  return `<div class="doctor-card"><div class="setup-card-head"><b>环境自检</b><small>部署是否能跑真实需求</small>${summary}<button type="button" class="setup-mini" data-doctor-action="refresh">重新检测</button></div><ul class="doctor-list">${state.doctor.checks.map(doctorCheckHTML).join('')}</ul></div>`;
}

function doctorQuickstartLine() {
  if (state.doctorStatus !== 'ready' || !state.doctor) return '';
  const pending = doctorPendingChecks();
  if (!pending.length) return '';
  return `<div class="doctor-strip ${state.doctor.ready ? '' : 'is-error'}"><span>环境自检：${pending.map(c => escapeHTML(c.label)).join('、')} 需要处理</span><button type="button" class="setup-mini" data-doctor-action="open-guide">查看 →</button></div>`;
}

async function runDoctorAction(action, button) {
  if (action === 'refresh') return loadDoctor(true);
  if (action === 'open-guide') { switchView('guide'); return setAccessTab('setup'); }
  if (action === 'setup-wizard') return openSetupWizard();
  if (action === 'git-token') return gotoCredential('gitToken');
  if (action === 'prewarm-sandbox') {
    if (button) button.disabled = true;
    try {
      await api('/api/doctor/prewarm-sandbox', { method: 'POST' });
      toast('沙箱镜像开始在后台构建，通常需要几分钟', 'ok');
    } catch (err) {
      toast(err.message || '预构建失败', 'err');
    }
    return loadDoctor(true);
  }
}

if (typeof document !== 'undefined' && typeof document.addEventListener === 'function') {
  document.addEventListener('click', event => {
    const button = event.target.closest?.('[data-doctor-action]');
    if (!button) return;
    event.preventDefault();
    runDoctorAction(button.dataset.doctorAction, button);
  });
}
