// ================= 工作台 =================
function jumpBind(host){if(!host)return;host.querySelectorAll('[data-jump],[data-issue-filter]').forEach(el=>el.onclick=()=>{
  if(el.dataset.issueFilter!=null){setIssueRunFilter(el.dataset.issueFilter,false);return}
  switchView(el.dataset.jump);
})}
function openRunFromId(id,opts={}){
  const issueId=issueIdForRun(id);
  if(!opts.forceRuns&&issueId){openIssueDetail(issueId,opts.tab||'sop');return}
  switchView('runs');
  showRunDetail(id);
}
function updateApprovalBadge(){
  const n=approvalItems().length;
  const badge=$('#approvalNavBadge');
  $$('[data-approval-count]').forEach(el=>{el.hidden=!n;el.textContent=n||''});
  const base='LoopForge · 你的 AI 交付团队';
  document.title=n?`(${n}) 待确认 · ${base}`:base;
  const prev=state.approvalCountSeen;
  state.approvalCountSeen=n;
  if(prev!=null&&n>prev)notifyApprovalWaiting(n);
  if(!badge)return;
  badge.hidden=!n;
  badge.textContent=n||'';
}
function notifyApprovalWaiting(n){
  if(!document.hidden||typeof Notification==='undefined')return;
  const show=()=>{try{const note=new Notification('LoopForge · 有流程等你确认',{body:`当前 ${n} 条待确认，点开处理`,tag:'loopforge-approval'});note.onclick=()=>{window.focus();switchView('approvals');note.close()}}catch(e){}};
  if(Notification.permission==='granted')show();
  else if(Notification.permission==='default')Notification.requestPermission().then(p=>{if(p==='granted')show()}).catch(()=>{});
}

function credentialClearAction(name,configured,label='API Key'){
  return `<div class="credential-actions"${configured?'':' hidden'}><button type="button" class="button small credential-clear" data-clear-credential="${name}" data-clear-label="${label}" data-pending-clear="false">清除 ${label}</button><span data-credential-status="${name}" data-configured="${configured?'true':'false'}" role="status">${configured?'当前已配置':'尚未配置'}</span></div>`;
}
function bindCredentialClearButtons(form){
  form.querySelectorAll('[data-clear-credential]').forEach(button=>{
    button.onclick=()=>{
      if(form.querySelector('button[type="submit"]').disabled)return;
      const name=button.dataset.clearCredential,input=form.elements[name],status=form.querySelector(`[data-credential-status="${name}"]`),pending=button.dataset.pendingClear!=='true';
      button.dataset.pendingClear=String(pending);
      button.textContent=pending?'撤销清除':`清除 ${button.dataset.clearLabel}`;
      input.disabled=pending;
      status.textContent=pending?'待清除，保存后生效':status.dataset.configured==='true'?'当前已配置':'尚未配置';
    };
  });
}
// 表单很长，保存栏吸底；一旦有改动就高亮，并在关闭或刷新页面前提醒。
function bindCredentialDirtyState(form){
  state.credentialsDirty=false;
  const bar=form.querySelector('.credential-savebar');
  const markDirty=()=>{
    if(state.credentialsDirty)return;
    state.credentialsDirty=true;
    bar.classList.add('is-dirty');
    bar.querySelector('b').textContent='有未保存的修改';
    bar.querySelector('span').textContent='点右边的按钮保存，保存后对下一次任务生效。';
  };
  form.addEventListener('input',markDirty);
  form.querySelectorAll('[data-clear-credential]').forEach(button=>button.addEventListener('click',markDirty));
}
if(typeof window!=='undefined'&&typeof window.addEventListener==='function')window.addEventListener('beforeunload',e=>{if(state.credentialsDirty){e.preventDefault();e.returnValue=''}});
function credentialClearPending(form,name){return form.querySelector(`[data-clear-credential="${name}"]`).dataset.pendingClear==='true'}
function credentialBadge(configured){return `<span class="credential-badge ${configured?'is-ready':'is-empty'}">${configured?'已配置':'未配置'}</span>`}
// 个人设置按用途分成子页，每页只放一类内容；凭证三页共用一个表单和保存栏。
const SETTINGS_TABS=[
  {id:'models',label:'模型 Key',hint:'Agent 执行任务用的模型 API Key'},
  {id:'git',label:'代码仓库',hint:'克隆、推送和创建 PR 用的 GitHub Token'},
  {id:'mcp',label:'MCP 令牌',hint:'「个人令牌」认证的 MCP 服务用'},
  {id:'account',label:'账号',hint:'登录信息与密码'},
];
function settingsTabValid(id){return SETTINGS_TABS.some(t=>t.id===id)}
// 没配模型 Key 时先去模型页，否则停在上次打开的子页。
function defaultSettingsTab(){
  const stored=userById(state.currentUser?.id)||{};
  if(!(stored.codeBuddyApiKeyConfigured||stored.codexApiKeyConfigured||stored.claudeCodeApiKeyConfigured))return 'models';
  return settingsTabValid(state.settingsTab)?state.settingsTab:'models';
}
function currentSettingsTab(){
  const routed=routePart(0)==='settings'?routePart(1):'';
  if(settingsTabValid(routed))return routed;
  return settingsTabValid(state.settingsTab)?state.settingsTab:defaultSettingsTab();
}
function openSettings(tab){
  state.settingsTab=settingsTabValid(tab)?tab:defaultSettingsTab();
  if(activeViewName()==='settings'&&showSettingsTab(state.settingsTab)){persistView('settings');return}
  switchView('settings');
}
// 只切换显示，不重绘：别的子页里还没保存的输入要留着。
function showSettingsTab(tab){
  const host=$('#personalSettingsContent');
  if(!host||!host.querySelector('[data-settings-tab]'))return false;
  host.querySelectorAll('[data-settings-tab]').forEach(b=>{const on=b.dataset.settingsTab===tab;b.classList.toggle('active',on);b.setAttribute('aria-selected',String(on))});
  host.querySelectorAll('[data-settings-panel]').forEach(p=>p.hidden=p.dataset.settingsPanel!==tab);
  const form=host.querySelector('#userCredentialForm');if(form)form.hidden=tab==='account';
  return true;
}
function settingsTabStatus(id,stored){
  if(id==='models')return stored.codeBuddyApiKeyConfigured||stored.codexApiKeyConfigured||stored.claudeCodeApiKeyConfigured;
  if(id==='git')return stored.gitTokenConfigured;
  if(id==='mcp')return stored.personalMcpTokenConfigured;
  return null;
}
async function renderPersonalSettings(){
  const host=$('#personalSettingsContent');if(!host)return;
  const user=state.currentUser;
  if(!user||user.isGuest){
    host.innerHTML='<section class="personal-settings-card"><h2>登录账号</h2><p>登录后即可管理自己的 Git Token 和 API Key。</p><button class="button primary" id="settingsLogin">登录</button></section>';
    host.querySelector('#settingsLogin').onclick=()=>openLoginModal();
    return;
  }
  const storedUser=userById(user.id)||{};
  const tab=currentSettingsTab();
  state.settingsTab=tab;
  const tabs=`<nav class="settings-tabs" role="tablist" aria-label="个人设置">${SETTINGS_TABS.map(t=>{const ok=settingsTabStatus(t.id,storedUser);return `<button type="button" role="tab" class="settings-tab${t.id===tab?' active':''}" data-settings-tab="${t.id}" aria-selected="${t.id===tab}"><span>${t.label}</span>${ok==null?'':`<i class="settings-tab-dot ${ok?'is-ready':'is-empty'}" title="${ok?'已配置':'未配置'}"></i>`}</button>`}).join('')}</nav>`;
  const panel=(id,body)=>`<div class="settings-panel" data-settings-panel="${id}"${id===tab?'':' hidden'}>${body}</div>`;
  const panelHead=id=>{const t=SETTINGS_TABS.find(x=>x.id===id);return `<header class="settings-panel-head"><h2>${t.label}</h2><p>${t.hint}</p></header>`};
  const models=`${panelHead('models')}<p class="settings-panel-note">至少配一个。任务用需求责任人（未指定时为项目 Owner）的 Key，只在运行时注入，不写进 Agent 资产。</p><div class="credential-grid">
        <fieldset class="provider-credentials"><legend><span class="credential-card-logo"><img src="/images/credentials/codebuddy.svg" alt=""></span><span>CodeBuddy<small>模型与执行</small></span>${credentialBadge(storedUser.codeBuddyApiKeyConfigured)}</legend>
          <p class="credential-description">用于调用 CodeBuddy Code 执行 Agent 任务；网络环境按账号所属站点配置。</p>
          <label>CodeBuddy API Key<input name="codeBuddyApiKey" type="password" autocomplete="new-password" placeholder="${storedUser.codeBuddyApiKeyConfigured?'已配置，留空则不改':'请输入 CodeBuddy API Key'}"></label>
          ${credentialClearAction('codeBuddyApiKey',storedUser.codeBuddyApiKeyConfigured)}
          <label>CodeBuddy 网络环境<input name="codeBuddyInternetEnvironment" value="${escapeHTML(storedUser.codeBuddyInternetEnvironment||'')}" placeholder="留空为国际站；internal 为中国站"></label>
          <a class="credential-help-link" href="https://www.codebuddy.ai/" target="_blank" rel="noopener noreferrer">前往 CodeBuddy 获取 API Key ↗</a>
        </fieldset>
        <fieldset class="provider-credentials"><legend><span class="credential-card-logo monochrome"><img src="/images/credentials/codex.svg" alt=""></span><span>Codex<small>Responses API</small></span>${credentialBadge(storedUser.codexApiKeyConfigured)}</legend>
          <p class="credential-description">用于调用 OpenAI Responses API 兼容服务；具体模型名称在角色定义中设置。</p>
          <label>Codex API Key<input name="codexApiKey" type="password" autocomplete="new-password" placeholder="${storedUser.codexApiKeyConfigured?'已配置，留空则不改':'请输入 Codex API Key'}"></label>
          ${credentialClearAction('codexApiKey',storedUser.codexApiKeyConfigured)}
          <label>Codex Base URL<input name="codexBaseUrl" type="url" value="${escapeHTML(storedUser.codexBaseUrl||'')}" placeholder="https://api.openai.com/v1"></label>
          <small>填写提供商地址；留空使用官方地址。</small>
        </fieldset>
        <fieldset class="provider-credentials"><legend><span class="credential-card-logo monochrome"><img src="/images/credentials/claude.svg" alt=""></span><span>Claude Code<small>Messages API</small></span>${credentialBadge(storedUser.claudeCodeApiKeyConfigured)}</legend>
          <p class="credential-description">用于调用 Anthropic Messages API 兼容服务，支持自定义服务地址。</p>
          <label>Claude Code API Key<input name="claudeCodeApiKey" type="password" autocomplete="new-password" placeholder="${storedUser.claudeCodeApiKeyConfigured?'已配置，留空则不改':'请输入 Claude Code API Key'}"></label>
          ${credentialClearAction('claudeCodeApiKey',storedUser.claudeCodeApiKeyConfigured)}
          <label>Claude Code Base URL<input name="claudeCodeBaseUrl" type="url" value="${escapeHTML(storedUser.claudeCodeBaseUrl||'')}" placeholder="https://api.anthropic.com"></label>
          <small>填写提供商地址；留空使用官方地址。</small>
        </fieldset></div>`;
  const git=`${panelHead('git')}<div class="credential-grid credential-grid-single">
        <fieldset class="provider-credentials"><legend><span class="credential-card-logo"><img src="/images/credentials/github.svg" alt=""></span><span>GitHub<small>代码仓库访问</small></span>${credentialBadge(storedUser.gitTokenConfigured)}</legend>
          <div class="upp-alert-warn git-token-pat-warning"${storedUser.gitTokenConfigured?' hidden':''}>请填 GitHub <b>个人访问令牌</b>。Classic token 勾选 <b>repo</b>；Fine-grained token 需要目标仓库的 <b>Contents</b>、<b>Pull requests</b>、<b>Issues</b> 读写权限，否则克隆 / 推送 / 建 PR 会失败。</div>
          <p class="credential-description">用于克隆私有仓库、创建任务分支及推送交付产物。</p>
          <label>GitHub Git Token<input name="gitToken" type="password" autocomplete="new-password" placeholder="${storedUser.gitTokenConfigured?'已配置，留空则不改':'ghp_xxx / github_pat_xxx'}"></label>
          ${credentialClearAction('gitToken',storedUser.gitTokenConfigured,'Git Token')}
          <a class="credential-help-link" href="https://github.com/settings/tokens" target="_blank" rel="noopener noreferrer">前往 GitHub 创建个人访问令牌 ↗</a>
          <div class="git-token-verify-row"><button type="button" class="button ghost small" id="verifyGitToken">校验 Token 有效性</button><span id="gitTokenVerifyStatus" class="git-token-verify-status"></span></div>
        </fieldset></div>`;
  const mcp=`${panelHead('mcp')}<div class="credential-grid credential-grid-single">
        <fieldset class="provider-credentials"><legend><span class="credential-card-logo"><img src="/images/credentials/github.svg" alt=""></span><span>个人 MCP 令牌<small>GitHub MCP 等</small></span>${credentialBadge(storedUser.personalMcpTokenConfigured)}</legend>
          <p class="credential-description">用于认证方式为「个人令牌」的 HTTP MCP（如 GitHub MCP）。令牌会自动拼成 Bearer 请求头，只在你负责的任务里注入。</p>
          <label>个人 MCP 令牌<input name="personalMcpToken" type="password" autocomplete="new-password" placeholder="${storedUser.personalMcpTokenConfigured?'已配置，留空则不改':'留空则 GitHub MCP 复用「代码仓库」里的 GitHub Token'}"></label>
          ${credentialClearAction('personalMcpToken',storedUser.personalMcpTokenConfigured,'MCP 令牌')}
          <a class="credential-help-link" href="https://github.com/settings/tokens" target="_blank" rel="noopener noreferrer">前往 GitHub 创建个人访问令牌 ↗</a>
        </fieldset></div>`;
  const account=`${panelHead('account')}<div class="settings-account">
      <section class="personal-settings-card personal-account-card"><div class="personal-account-avatar">${userAvatarHTML(user, 150)}</div><div><span class="credential-eyebrow">当前账号</span><h2>${escapeHTML(userDisplay(user))}</h2><p>登录名 <code>${escapeHTML(user.loginName||'')}</code>${isPlatformAdmin()?' · 平台管理员':''}</p></div>${state.proxyAuth?'':'<button type="button" class="button ghost upp-btn-danger" id="settingsLogout">退出登录</button>'}</section>
      ${state.proxyAuth?'<p class="settings-panel-note">当前通过反向代理登录，密码由你们的身份系统管理。</p>':`<form id="userPasswordForm" class="personal-settings-card settings-password-form"><h3>修改密码</h3><p>至少 8 位，修改后当前登录保持有效。忘记密码请联系平台管理员重置。</p><label>当前密码<input name="currentPassword" type="password" autocomplete="current-password" required></label><label>新密码<input name="newPassword" type="password" autocomplete="new-password" required minlength="8"></label><div class="settings-form-actions"><button class="button primary" type="submit">修改密码</button></div></form>`}
    </div>`;
  host.innerHTML=`${tabs}<form id="userCredentialForm" class="upp-credentials settings-credential-form"${tab==='account'?' hidden':''}>${panel('models',models)}${panel('git',git)}${panel('mcp',mcp)}
        <footer class="credential-savebar"><div><b>保存后对下一次任务生效</b><span>留空不会覆盖已经配置的密钥。</span></div><button class="button primary" type="submit">保存</button></footer>
      </form>${panel('account',account)}`;
  host.querySelectorAll('[data-settings-tab]').forEach(b=>b.onclick=()=>openSettings(b.dataset.settingsTab));
  const logout=host.querySelector('#settingsLogout');
  if(logout)logout.onclick=async()=>{try{await api('/api/auth/logout',{method:'POST'})}catch(_){}location.reload()};
  const passwordForm=host.querySelector('#userPasswordForm');
  if(passwordForm)passwordForm.onsubmit=async e=>{
    e.preventDefault();
    try{
      await api('/api/auth/password',{method:'POST',body:JSON.stringify({currentPassword:passwordForm.currentPassword.value,newPassword:passwordForm.newPassword.value})});
      passwordForm.reset();
      toast('密码已修改','ok');
    }catch(err){toast(err.message,'err')}
  };
  bindCredentialClearButtons(host.querySelector('#userCredentialForm'));
  bindCredentialDirtyState(host.querySelector('#userCredentialForm'));
  const verifyBtn=host.querySelector('#verifyGitToken');
  // 状态文案节点每次重渲染后都会重建，统一现查现用，避免写入游离 DOM。
  const setStatus=text=>{const el=$('#gitTokenVerifyStatus');if(el)el.textContent=text};
  // The token-type warning only matters while no working token is stored.
  const showPatWarning=show=>{const el=$('.git-token-pat-warning');if(el)el.hidden=!show};
  const renderVerifyStatus=r=>{
    state.gitTokenVerify={userId:user.id,result:r};
    if(r.error){setStatus(`✗ ${r.error}`);showPatWarning(true);if(state.gitTokenAnnounceVerify){state.gitTokenAnnounceVerify=false;toast(r.error)}return}
    if(!r.valid){setStatus(`✗ 令牌有效但缺少权限：${(r.missing||[]).join('、')}，请在 GitHub 补充勾选`);showPatWarning(true);if(state.gitTokenAnnounceVerify){state.gitTokenAnnounceVerify=false;toast(r.error||'Git Token 校验未通过，请检查权限范围')}return}
    showPatWarning(false);
    setStatus(r.scopes&&r.scopes.length?`✓ 校验通过（${(r.scopes||[]).join(', ')}）`:'✓ 令牌有效');
    if(state.gitTokenAnnounceVerify){state.gitTokenAnnounceVerify=false}
  };
  async function verifyGitToken(){
    if(verifyBtn)verifyBtn.disabled=true;
    setStatus('正在校验…');
    const typed=(host.querySelector('input[name=gitToken]')?.value||'').trim();
    try{renderVerifyStatus(await api(`/api/users/${user.id}/git-token/verify`,typed?{method:'POST',body:JSON.stringify({token:typed}),silent:true}:{silent:true}))}
    catch(err){renderVerifyStatus({error:err.message})}
    finally{if(verifyBtn)verifyBtn.disabled=false}
  }
  if(verifyBtn)verifyBtn.onclick=verifyGitToken;
  if(storedUser.gitTokenConfigured){
    if(state.gitTokenVerify?.userId===user.id)renderVerifyStatus(state.gitTokenVerify.result);
    else verifyGitToken();
  }
  host.querySelector('#userCredentialForm').onsubmit=async e=>{
    e.preventDefault();const f=e.currentTarget,button=f.querySelector('button[type="submit"]');button.disabled=true;
    const payload={personalMcpToken:f.elements.personalMcpToken.value.trim(),personalMcpTokenClear:credentialClearPending(f,'personalMcpToken'),gitToken:f.elements.gitToken.value.trim(),gitTokenClear:credentialClearPending(f,'gitToken'),codeBuddyApiKey:f.elements.codeBuddyApiKey.value.trim(),codeBuddyApiKeyClear:credentialClearPending(f,'codeBuddyApiKey'),codeBuddyInternetEnvironment:f.elements.codeBuddyInternetEnvironment.value.trim(),codexApiKey:f.elements.codexApiKey.value.trim(),codexApiKeyClear:credentialClearPending(f,'codexApiKey'),codexBaseUrl:f.elements.codexBaseUrl.value.trim(),claudeCodeApiKey:f.elements.claudeCodeApiKey.value.trim(),claudeCodeApiKeyClear:credentialClearPending(f,'claudeCodeApiKey'),claudeCodeBaseUrl:f.elements.claudeCodeBaseUrl.value.trim()};
    try{if(payload.gitToken||payload.gitTokenClear)state.gitTokenVerify=null;if(payload.gitToken)state.gitTokenAnnounceVerify=true;const saved=await api(`/api/users/${user.id}/credentials`,{method:'PUT',body:JSON.stringify(payload)}),index=state.data.users.findIndex(u=>Number(u.id)===Number(saved.id));if(index>=0)state.data.users[index]=saved;else state.data.users.push(saved);renderPersonalSettings();toast('个人凭据已保存');if(payload.codeBuddyApiKey)verifyCodeBuddyKey(user.id)}
    catch(err){toast(err.message);button.disabled=false}
  };
}
// 谁能改通用模板是平台级权限，不属于任何一个项目，也不属于个人凭据，
// 平台管理也容纳业务域管理；按各自授权展示入口与 tab。
function renderPlatformNav(){
  const group=$('#platformNavGroup');if(!group)return;
  group.hidden=!canAccessPlatformManagement();
  const platformItem=group.querySelector('[data-view="platform"]');
  if(platformItem)platformItem.hidden=!isPlatformAdmin();
}
function renderPlatformAdminPanel(){
  const host=$('#platformAdminPanel');if(!host)return;
  if(!isPlatformAdmin()){host.innerHTML='';return}
  const tab=state.platformAdminTab||'admins';
  host.innerHTML=`
    <div class="platform-metric-grid pa-stats">
      <div class="platform-metric-card"><span class="platform-metric-label">平台管理员</span><b class="platform-metric-num" id="platformMetricTotal">--</b></div>
      <div class="platform-metric-card"><span class="platform-metric-label">系统内置</span><b class="platform-metric-num" id="platformMetricLocked">--</b></div>
      <div class="platform-metric-card"><span class="platform-metric-label">界面授权</span><b class="platform-metric-num" id="platformMetricGranted">--</b></div>
      <div class="platform-metric-card is-scope"><span class="platform-metric-label">生效范围</span><b class="platform-metric-num">全平台</b><small>通用模板 · 共享角色 · 门禁</small></div>
    </div>

    <nav class="pa-tabs" role="tablist">
      <button type="button" role="tab" data-pa-tab="admins" class="${tab==='admins'?'active':''}"><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>平台管理员<em id="platformAdminCountBadge" class="platform-admin-count-pill">--</em></button>
      <button type="button" role="tab" data-pa-tab="accounts" class="${tab==='accounts'?'active':''}"><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M19 8v6M22 11h-6"/></svg>成员账号</button>
      <button type="button" role="tab" data-pa-tab="security" class="${tab==='security'?'active':''}"><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>注册与安全</button>
    </nav>

    <div class="platform-admin-layout pa-layout">
      <div class="platform-admin-main-card pa-pane" data-pa-pane="admins" ${tab==='admins'?'':'hidden'}>
        <div class="pa-pane-head">
          <div><h2>谁可以改通用模板</h2><p>平台管理员能编辑通用 SOP 模板和它打包的角色，改完所有项目下次运行生效。<code>LOOPFORGE_PLATFORM_ADMINS</code> 里的名单不能在这里撤销。</p></div>
        </div>
        <form id="platformAdminForm" class="platform-admin-form pa-inline-form">
          <div class="platform-input-wrap">
            <span class="platform-input-icon"><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg></span>
            <input name="loginName" placeholder="输入登录名授予管理员，如 alice" autocomplete="off">
          </div>
          <button class="button primary platform-add-btn" type="submit">授予</button>
        </form>
        <div class="platform-list-toolbar">
          <div class="platform-search-wrap">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
            <input id="platformAdminSearch" placeholder="搜索姓名或登录名" autocomplete="off">
          </div>
          <div class="platform-filter-tabs" id="platformFilterTabs">
            <button type="button" class="platform-filter-btn active" data-source-filter="all">全部</button>
            <button type="button" class="platform-filter-btn" data-source-filter="builtin">环境变量</button>
            <button type="button" class="platform-filter-btn" data-source-filter="granted">已授权</button>
          </div>
        </div>
        <div id="platformAdminList" class="platform-admin-list"></div>
      </div>

      <div class="platform-admin-main-card pa-pane" data-pa-pane="accounts" ${tab==='accounts'?'':'hidden'}>
        <div class="pa-pane-head"><div><h2>创建或重置账号</h2><p>本地账号用 bcrypt 存储密码。登录名已存在时会重置为新密码；接入 SSO 时改用受信反向代理身份头，无需在这里建号。</p></div></div>
        <form id="platformAccountForm" class="platform-admin-form platform-account-form pa-grid-form">
          <label><span>登录名</span><input name="loginName" placeholder="alice" autocomplete="off" required></label>
          <label><span>显示名</span><input name="displayName" placeholder="可选" autocomplete="off"></label>
          <label><span>初始密码</span><input name="password" type="password" placeholder="至少 8 位" autocomplete="new-password" required></label>
          <div class="pa-form-foot"><label class="toggle-row"><input name="admin" type="checkbox"> 同时设为平台管理员</label><button class="button primary platform-add-btn" type="submit">创建账号 / 重置密码</button></div>
        </form>
      </div>

      <div class="platform-admin-main-card pa-pane" data-pa-pane="security" ${tab==='security'?'':'hidden'}>
        <div class="pa-pane-head"><div><h2>注册与安全</h2><p>控制谁能进入平台，以及平台级权限的边界。</p></div></div>
        <label class="pa-switch-row platform-signup-box">
          <span><b>允许新用户在登录页自行注册</b><small>注册的账号是普通成员：能看项目、提需求、被指派为负责人。只在可信网络里打开；关闭后只能由管理员创建账号。</small></span>
          <input type="checkbox" id="platformSignupOpen" class="pa-switch">
        </label>
        <ul class="platform-side-list pa-rules">
          <li><i><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 6h16M4 12h10M4 18h7"/></svg></i><div><b>通用 SOP 模板全局生效</b><p>通用模板由平台管理员统一维护，修改后各项目下次运行即生效。</p></div></li>
          <li><i><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="9" cy="7" r="4"/><path d="M2 21v-2a4 4 0 0 1 4-4h6"/><path d="m16 19 2 2 4-4"/></svg></i><div><b>公共角色与门禁统一下发</b><p>通用模板引用的 Agent 角色、执行门禁在项目内默认只读。</p></div></li>
          <li><i><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg></i><div><b>内置名单防锁死</b><p>环境变量里的管理员无法从页面撤销，保证总有人能管理平台。</p></div></li>
        </ul>
      </div>

      <aside class="platform-admin-side pa-side">
        <div class="platform-side-card">
          <b class="pa-side-title">通用资产</b>
          <p class="platform-side-desc">在流程设计或角色定义中修改通用模板后，所有关联项目下次运行自动用最新版本。</p>
          <div class="platform-side-quick-links">
            <button type="button" class="button ghost small platform-quick-link-btn" data-platform-nav="sops">流程模板 →</button>
            <button type="button" class="button ghost small platform-quick-link-btn" data-platform-nav="agents">角色定义 →</button>
          </div>
          <div class="platform-audit-note">授予与撤销实时生效，并记录平台审计。</div>
        </div>
      </aside>
    </div>
  `;
  host.querySelectorAll('[data-pa-tab]').forEach(btn=>btn.onclick=()=>{
    state.platformAdminTab=btn.dataset.paTab;
    host.querySelectorAll('[data-pa-tab]').forEach(b=>b.classList.toggle('active',b===btn));
    host.querySelectorAll('[data-pa-pane]').forEach(p=>p.hidden=p.dataset.paPane!==btn.dataset.paTab);
  });
  loadPlatformAdmins(host);
}
async function loadPlatformAdmins(host){
  const list=host.querySelector('#platformAdminList');
  const form=host.querySelector('#platformAdminForm');
  const searchInput=host.querySelector('#platformAdminSearch');
  const filterTabs=host.querySelectorAll('#platformFilterTabs button');
  if(!list||!form)return;

  host.querySelectorAll('[data-platform-nav]').forEach(btn=>{
    btn.onclick=()=>{
      const target=btn.dataset.platformNav;
      if(target&&typeof switchView==='function'){
        switchView(target);
      }
    };
  });

  const sourceMeta={
    builtin:{label:'内置',className:'source-builtin',icon:'<svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>'},
    granted:{label:'已授权',className:'source-granted',icon:'<svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>'}
  };

  let allRows=[];
  let activeFilter='all';
  let searchQuery='';

  const updateMetrics=rows=>{
    const listArr=rows||[];
    const total=listArr.length;
    const locked=listArr.filter(r=>!r.revocable).length;
    const granted=listArr.filter(r=>r.revocable).length;
    const elTotal=host.querySelector('#platformMetricTotal');
    const elLocked=host.querySelector('#platformMetricLocked');
    const elGranted=host.querySelector('#platformMetricGranted');
    const elBadge=host.querySelector('#platformAdminCountBadge');
    if(elTotal)elTotal.textContent=total;
    if(elLocked)elLocked.textContent=locked;
    if(elGranted)elGranted.textContent=granted;
    if(elBadge)elBadge.textContent=String(total);
  };

  const render=()=>{
    let filtered=allRows||[];
    if(activeFilter==='builtin'){
      filtered=filtered.filter(r=>!r.revocable||r.source==='builtin'||r.source==='env');
    }else if(activeFilter==='granted'){
      filtered=filtered.filter(r=>r.revocable||r.source==='granted');
    }
    if(searchQuery){
      const q=searchQuery.toLowerCase();
      const knownDisplayNames = {};
      filtered=filtered.filter(r=>
        (r.displayName&&String(r.displayName).toLowerCase().includes(q))||
        (r.loginName&&String(r.loginName).toLowerCase().includes(q))||
        (knownDisplayNames[String(r.loginName).toLowerCase()]&&knownDisplayNames[String(r.loginName).toLowerCase()].toLowerCase().includes(q))
      );
    }

    if(!filtered.length){
      list.innerHTML=`
        <div class="platform-empty-state">
          <div class="platform-empty-icon">🔍</div>
          <b>未找到符合条件的管理员</b>
          <p>${(allRows&&allRows.length)?'请尝试更换搜索关键字或重置筛选条件。':'还没有配置平台管理员。'}</p>
        </div>
      `;
      return;
    }

    list.innerHTML=filtered.map(row=>{
      const isBuiltin=!row.revocable||row.source==='builtin'||row.source==='env';
      const meta=isBuiltin?sourceMeta.builtin:sourceMeta.granted;
      const userObj=(state.data?.users||[]).find(u=>u.loginName===row.loginName||(row.userId&&Number(u.id)===Number(row.userId)));
      const knownDisplayNames = {};
      const lName=row.loginName||'';
      let cName=row.displayName||userObj?.displayName||'';
      if(!cName||cName.toLowerCase()===lName.toLowerCase()){
        if(knownDisplayNames[lName.toLowerCase()]){
          cName=knownDisplayNames[lName.toLowerCase()];
        }
      }
      const displayName=(cName&&cName.toLowerCase()!==lName.toLowerCase())
        ? `${lName}(${cName})`
        : lName;
      return `
        <div class="platform-admin-row ${row.revocable?'is-granted':'is-locked'}">
          <div class="platform-admin-user">
            <div class="platform-admin-avatar">
              ${userAvatarHTML(row.loginName, 100)}
            </div>
            <div class="platform-admin-info">
              <div class="platform-admin-name-row">
                <span class="platform-admin-name">${escapeHTML(displayName)}</span>
              </div>
              <div class="platform-admin-meta">
                <span class="platform-source-badge ${meta.className}">
                  ${meta.icon}<span>${escapeHTML(meta.label)}</span>
                </span>
              </div>
            </div>
          </div>
          <div class="platform-admin-actions">
            ${row.revocable?`
              <button type="button" class="button ghost small platform-revoke-btn" data-revoke-admin="${escapeHTML(row.loginName)}" title="撤销该账号的平台管理员权限">
                <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                <span>撤销</span>
              </button>
            `:`
              <span class="platform-locked-tag" title="系统内置名单无法通过界面撤销，保障平台管理连续性">
                <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
                <span>不可撤销</span>
              </span>
            `}
          </div>
        </div>
      `;
    }).join('');

    list.querySelectorAll('[data-revoke-admin]').forEach(btn=>btn.onclick=async()=>{
      const target=btn.dataset.revokeAdmin;
      if(!await afConfirm({
        title:'撤销平台管理员',
        message:`确定撤销 ${target} 的平台管理员权限？撤销后该账号将不再具备通用模板与角色编辑能力。`,
        danger:true,
        confirmText:'撤销权限'
      }))return;
      try{
        const rows=await api('/api/platform/admins',{method:'PUT',body:JSON.stringify({loginName:target,role:''})});
        allRows=rows||[];
        updateMetrics(allRows);
        render();
        toast('已撤销管理员权限','ok');
      }catch(e){toast(e.message,'err')}
    });
  };

  if(searchInput){
    searchInput.oninput=()=>{
      searchQuery=searchInput.value.trim();
      render();
    };
  }

  filterTabs.forEach(btn=>{
    btn.onclick=()=>{
      filterTabs.forEach(b=>b.classList.remove('active'));
      btn.classList.add('active');
      activeFilter=btn.dataset.sourceFilter||'all';
      render();
    };
  });

  try{
    allRows=await api('/api/platform/admins')||[];
    updateMetrics(allRows);
    render();
  }catch(e){
    list.innerHTML=`<p class="prop-hint">${escapeHTML(e.message||'加载管理员列表失败')}</p>`;
  }

  form.onsubmit=async e=>{
    e.preventDefault();
    const login=form.elements.loginName.value.trim();
    if(!login)return toast('请填写登录名');
    const submitBtn=form.querySelector('button[type="submit"]');
    if(submitBtn)submitBtn.disabled=true;
    try{
      const rows=await api('/api/platform/admins',{method:'PUT',body:JSON.stringify({loginName:login,role:'admin'})});
      form.reset();
      allRows=rows||[];
      updateMetrics(allRows);
      render();
      toast('已成功添加平台管理员','ok');
    }catch(err){
      toast(err.message,'err');
    }finally{
      if(submitBtn)submitBtn.disabled=false;
    }
  };
  const signup=host.querySelector('#platformSignupOpen');
  if(signup){
    api('/api/auth/settings',{silent:true}).then(v=>{signup.checked=!!v.signupOpen}).catch(()=>{});
    signup.onchange=async()=>{
      signup.disabled=true;
      try{const v=await api('/api/auth/settings',{method:'PUT',body:JSON.stringify({signupOpen:signup.checked})});signup.checked=!!v.signupOpen;state.signupOpen=signup.checked;toast(signup.checked?'已开放注册':'已关闭注册','ok')}
      catch(err){signup.checked=!signup.checked;toast(err.message,'err')}
      finally{signup.disabled=false}
    };
  }
  const accountForm=$('#platformAccountForm');
  if(accountForm)accountForm.onsubmit=async e=>{
    e.preventDefault();
    const el=accountForm.elements,button=accountForm.querySelector('button[type="submit"]');
    button.disabled=true;
    try{
      const admin=el.admin.checked;
      const created=await api('/api/users',{method:'POST',body:JSON.stringify({loginName:el.loginName.value.trim(),displayName:el.displayName.value.trim(),password:el.password.value,admin})});
      accountForm.reset();
      state.data.users=await api('/api/users');
      if(admin){allRows=await api('/api/platform/admins')||[];updateMetrics(allRows);render()}
      toast(`账号 ${created.loginName} 已就绪`,'ok');
    }catch(err){toast(err.message,'err')}
    finally{button.disabled=false}
  };
}


function renderUserProfilePopover() {
  const popover = $('#userProfilePopover');
  if (!popover) return;
  const user = state.currentUser;
  const isGuest = !user || user.isGuest;
  const displayName = user?.displayName || user?.loginName || '访客';
  const initial = (displayName[0] || '访').toUpperCase();
  if (isGuest) {
    popover.innerHTML = `
      <div class="upp-header">
        <div class="upp-avatar-lg is-guest">${escapeHTML(initial)}</div>
        <div class="upp-user-meta">
          <div class="upp-name-row">
            <span class="upp-name">访客身份</span>
            <span class="upp-badge-tag is-guest">未登录</span>
          </div>
          <div class="upp-login-sub">Guest Session</div>
        </div>
      </div>
      <div class="upp-divider"></div>
      <div class="upp-guest-body">
        <p class="upp-guest-desc">当前以访客身份访问平台。登录后可在左侧「个人设置」配置 GitHub Token 和模型 API Key。</p>
      </div>
      <div class="upp-footer">
        <button id="uppLoginBtn" class="button small primary upp-full-btn" type="button">登录</button>
      </div>
    `;
    popover.querySelector('#uppLoginBtn').onclick = () => { $('#userProfileWrap')?.classList.remove('is-open'); openLoginModal(); };
  } else {
    popover.innerHTML = `
      <div class="upp-header">
        <div class="upp-avatar-lg">${userAvatarHTML(user, 150)}</div>
        <div class="upp-user-meta">
          <div class="upp-name-row">
            <span class="upp-name">${escapeHTML(displayName)}</span>
            ${isPlatformAdmin()?'<span class="upp-badge-tag">平台管理员</span>':''}
          </div>
          <div class="upp-login-sub">${escapeHTML(user.loginName || '')}</div>
        </div>
      </div>
      <div class="upp-divider"></div>
      <div class="upp-info-list">
        <div class="upp-info-item">
          <span class="upp-label">显示名</span>
          <span class="upp-value">${escapeHTML(user.displayName || displayName)}</span>
        </div>
        <div class="upp-info-item">
          <span class="upp-label">登录账号</span>
          <span class="upp-value">${escapeHTML(user.loginName || '-')}</span>
        </div>
        <div class="upp-info-item">
          <span class="upp-label">认证方式</span>
          <span class="upp-value">${state.proxyAuth?'反向代理身份头':'本地账号'}</span>
        </div>
      </div>
      <div class="upp-divider"></div>
      <button type="button" id="openOnboarding" class="button ghost">新手引导</button>
      <button type="button" id="openPersonalSettings" class="button ghost">个人设置 · 执行凭据</button>
 <div class="upp-divider"></div>
      <div class="upp-footer">
        <button id="uppReAuthBtn" class="button small ghost" type="button" title="退出后用其他账号登录"${state.proxyAuth?' hidden':''}>切换账号</button>
        <button id="uppLogoutBtn" class="button small ghost upp-btn-danger" type="button" title="退出当前登录"${state.proxyAuth?' hidden':''}>退出登录</button>
      </div>
    `;
    popover.querySelector('#openPersonalSettings').onclick=()=>{ $('#userProfileWrap').classList.remove('is-open'); openSettings() };
    popover.querySelector('#openOnboarding').onclick=()=>{ $('#userProfileWrap').classList.remove('is-open'); startOnboarding(); };
    const reAuthBtn = popover.querySelector('#uppReAuthBtn');
    if (reAuthBtn) {
      reAuthBtn.onclick = async () => {
        try { await api('/api/auth/logout', {method: 'POST'}); } catch (_) {}
        $('#userProfileWrap')?.classList.remove('is-open');
        openLoginModal({forced:state.authRequired});
      };
    }
    const logoutBtn = popover.querySelector('#uppLogoutBtn');
    if (logoutBtn) {
      logoutBtn.onclick = async () => {
        try {
          await api('/api/auth/logout', {method: 'POST'});
          if(state.authRequired){location.reload();return}
          state.currentUser = { isGuest: true, loginName: 'guest', displayName: '访客' };
          $('#userProfileWrap')?.classList.remove('is-open');
          renderUserBadge();
          if(activeViewName()==='settings')renderPersonalSettings();
          toast('已退出登录，当前为访客身份');
        } catch (err) {
          toast('退出登录失败: ' + err.message, 'err');
        }
      };
    }
  }
}

function userAvatarHTML(userOrId, spec=100, extraClass=''){
  let user = null;
  if(userOrId && typeof userOrId === 'object'){
    user = userOrId;
  } else if(userOrId != null && userOrId !== '') {
    if(typeof userById === 'function'){
      user = userById(userOrId);
    }
    if(!user){
      user = {loginName: String(userOrId)};
    }
  }
  const isGuest = !user || user.isGuest || user.loginName === 'guest' || !user.loginName;
  const initial = (user?.displayName || user?.loginName || '用')[0]?.toUpperCase() || '访';
  if(isGuest){
    return `<span class="user-avatar-text">${escapeHTML(initial)}</span>`;
  }
  return `<span class="user-avatar-text ${extraClass}" title="${escapeHTML(user.displayName || user.loginName)}">${escapeHTML(initial)}</span>`;
}

function renderUserBadge(){
  const badge=$('#userBadge'), nameEl=$('#userName'), avatarEl=$('#userAvatarText');
  if(!badge||!nameEl||!avatarEl) return;
  const user=state.currentUser;
  const isGuest = !user || user.isGuest;
  const displayName=user?.displayName||user?.loginName||'访客';
  nameEl.textContent=displayName;
  avatarEl.innerHTML=userAvatarHTML(user, 100);
  if(isGuest){
    badge.classList.add('is-guest');
    let authErr = null;
    try { authErr = sessionStorage.getItem('loopforge_auth_err'); } catch (_) {}
    badge.title=authErr ? '认证换票未通过，点击或悬停查看个人信息' : '当前为访客身份，点击或悬停查看个人信息';
  }else{
    badge.classList.remove('is-guest');
    badge.title=`${displayName} (${user.loginName||user.staffId}) - 点击或悬停查看个人信息`;
  }
  renderUserProfilePopover();
}


// Checks a saved CodeBuddy key against both network environments; the server
// stores the one that works (China-site keys fail on the international default).
async function verifyCodeBuddyKey(userId){
  toast('正在验证 CodeBuddy Key（约 10 秒）…');
  try{
    const result=await api(`/api/users/${userId}/codebuddy/verify`,{method:'POST',silent:true});
    if(result.ok){
      if(result.switched){
        const index=(state.data.users||[]).findIndex(u=>Number(u.id)===Number(userId));
        if(index>=0)state.data.users[index]={...state.data.users[index],codeBuddyInternetEnvironment:result.environment};
        if(typeof renderPersonalSettings==='function'&&activeViewName()==='settings')renderPersonalSettings();
      }
      toast(result.switched?`CodeBuddy Key 可用：这是${result.environment==='internal'?'中国站':'国际站'}的 Key，已自动切换网络环境`:'CodeBuddy Key 验证通过','ok');
    }else if(!result.skipped){
      toast(result.error||'CodeBuddy Key 验证失败','err');
    }
    return result;
  }catch(err){toast(err.message||'CodeBuddy Key 验证失败','err');return null}
}
