// First-run credential wizard (Grasp-style): pick a CLI → apply links → save → done.
const SETUP_WIZARD_KEY = 'loopforge.setupWizard.v1';

const SETUP_CLI_OPTIONS = [
  {
    id: 'claude',
    name: 'Claude Code',
    field: 'claudeCodeApiKey',
    configured: u => !!u?.claudeCodeApiKeyConfigured,
    applyURL: 'https://console.anthropic.com/settings/keys',
    hint: 'Anthropic API Key，写入 Messages API。',
  },
  {
    id: 'codex',
    name: 'Codex',
    field: 'codexApiKey',
    configured: u => !!u?.codexApiKeyConfigured,
    applyURL: 'https://platform.openai.com/api-keys',
    hint: 'OpenAI API Key，写入 Responses API。',
  },
  {
    id: 'codebuddy',
    name: 'CodeBuddy',
    field: 'codeBuddyApiKey',
    configured: u => !!u?.codeBuddyApiKeyConfigured,
    applyURL: 'https://www.codebuddy.ai/',
    hint: 'CodeBuddy 控制台申请的模型 Key。',
  },
];

function setupWizardSeen() {
  try { return localStorage.getItem(SETUP_WIZARD_KEY) === '1'; } catch (_) { return false; }
}
function setupWizardMarkDone() {
  try { localStorage.setItem(SETUP_WIZARD_KEY, '1'); } catch (_) {}
}
function setupWizardHasModelKey() {
  const stored = typeof userById === 'function' ? userById(state.currentUser?.id) : null;
  return SETUP_CLI_OPTIONS.some(o => o.configured(stored || {}));
}
function setupWizardDemo() {
  return typeof quickstartDemoAvailable === 'function' && quickstartDemoAvailable();
}

function maybeStartSetupWizard() {
  const user = state.currentUser;
  if (!user || user.isGuest || setupWizardSeen() || setupWizardDemo() || setupWizardHasModelKey()) return;
  // Let onboarding finish first when both would show.
  if (typeof onboardingSeen === 'function' && !onboardingSeen()) return;
  openSetupWizard();
}

function openSetupWizard() {
  let root = document.getElementById('setupWizard');
  if (!root) {
    root = document.createElement('div');
    root.id = 'setupWizard';
    root.className = 'setup-wizard';
    document.body.appendChild(root);
  }
  root.hidden = false;
  document.body.classList.add('setup-wizard-open');
  paintSetupWizard(root, { step: 0, cli: 'claude', key: '', gitToken: '', busy: false, error: '' });
}

function closeSetupWizard(done) {
  if (done) setupWizardMarkDone();
  const root = document.getElementById('setupWizard');
  if (root) root.hidden = true;
  document.body.classList.remove('setup-wizard-open');
}

function paintSetupWizard(root, ctx) {
  const opt = SETUP_CLI_OPTIONS.find(o => o.id === ctx.cli) || SETUP_CLI_OPTIONS[0];
  if (ctx.step === 0) {
    root.innerHTML = `<div class="setup-wizard-backdrop"></div><div class="setup-wizard-card" role="dialog" aria-modal="true">
      <p class="setup-wizard-kicker">首次配置</p>
      <h2>选一个编码 CLI，填上 API Key</h2>
      <p>真实 SOP 需要模型 Key；演示 SOP 不用。可随时从接入指南重开本向导。</p>
      <div class="setup-wizard-cli">${SETUP_CLI_OPTIONS.map(o => `<button type="button" class="setup-cli-card ${o.id === ctx.cli ? 'is-active' : ''}" data-cli="${o.id}"><b>${escapeHTML(o.name)}</b><small>${escapeHTML(o.hint)}</small></button>`).join('')}</div>
      <div class="setup-wizard-actions">
        <button type="button" class="button ghost" data-skip>稍后再说</button>
        <button type="button" class="button primary" data-next>下一步：申请与粘贴</button>
      </div>
    </div>`;
    root.querySelectorAll('[data-cli]').forEach(b => b.onclick = () => paintSetupWizard(root, { ...ctx, cli: b.dataset.cli }));
    root.querySelector('[data-skip]').onclick = () => closeSetupWizard(true);
    root.querySelector('[data-next]').onclick = () => paintSetupWizard(root, { ...ctx, step: 1 });
    return;
  }
  root.innerHTML = `<div class="setup-wizard-backdrop"></div><div class="setup-wizard-card" role="dialog" aria-modal="true">
    <p class="setup-wizard-kicker">${escapeHTML(opt.name)}</p>
    <h2>粘贴 Key，立刻开跑短链路 SOP</h2>
    <p><a href="${escapeHTML(opt.applyURL)}" target="_blank" rel="noopener">打开申请页</a> → 复制 Key → 粘贴下方。可选填 GitHub Token（读写仓库）。</p>
    <label>模型 API Key<input type="password" autocomplete="off" id="setupWizardKey" placeholder="sk-…" value="${escapeHTML(ctx.key || '')}"></label>
    <label>GitHub Token（可选）<input type="password" autocomplete="off" id="setupWizardGit" placeholder="ghp_… 或 fine-grained" value="${escapeHTML(ctx.gitToken || '')}"></label>
    ${ctx.error ? `<p class="setup-wizard-error">${escapeHTML(ctx.error)}</p>` : ''}
    <div class="setup-wizard-actions">
      <button type="button" class="button ghost" data-back>上一步</button>
      <button type="button" class="button ghost" data-skip>跳过</button>
      <button type="button" class="button primary" data-save ${ctx.busy ? 'disabled' : ''}>${ctx.busy ? '保存中…' : '保存并完成'}</button>
    </div>
  </div>`;
  root.querySelector('[data-back]').onclick = () => paintSetupWizard(root, { ...ctx, step: 0, key: root.querySelector('#setupWizardKey')?.value || '', gitToken: root.querySelector('#setupWizardGit')?.value || '' });
  root.querySelector('[data-skip]').onclick = () => closeSetupWizard(true);
  root.querySelector('[data-save]').onclick = async () => {
    const key = String(root.querySelector('#setupWizardKey')?.value || '').trim();
    const gitToken = String(root.querySelector('#setupWizardGit')?.value || '').trim();
    if (!key) return paintSetupWizard(root, { ...ctx, error: '请先粘贴模型 API Key' });
    const user = state.currentUser;
    if (!user?.id) return toast('请先登录', 'err');
    paintSetupWizard(root, { ...ctx, key, gitToken, busy: true, error: '' });
    const payload = { [opt.field]: key };
    if (gitToken) payload.gitToken = gitToken;
    try {
      const saved = await api(`/api/users/${user.id}/credentials`, { method: 'PUT', body: JSON.stringify(payload) });
      const index = (state.data.users || []).findIndex(u => Number(u.id) === Number(saved.id));
      if (index >= 0) state.data.users[index] = saved;
      else (state.data.users || (state.data.users = [])).push(saved);
      toast('凭据已保存，可以用「快速修复」SOP 开跑了', 'ok');
      closeSetupWizard(true);
      if (typeof renderPersonalSettings === 'function' && state.view === 'settings') renderPersonalSettings();
      if (typeof renderHomeChat === 'function') renderHomeChat();
      if (opt.field === 'codeBuddyApiKey' && typeof verifyCodeBuddyKey === 'function') verifyCodeBuddyKey(user.id);
    } catch (e) {
      paintSetupWizard(root, { ...ctx, key, gitToken, busy: false, error: e.message || '保存失败' });
    }
  };
}

function reopenSetupWizardFromGuide() {
  try { localStorage.removeItem(SETUP_WIZARD_KEY); } catch (_) {}
  openSetupWizard();
}
