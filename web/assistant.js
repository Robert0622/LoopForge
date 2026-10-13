/* Conversation transcripts are stored per-user on the server, never with API keys. */
(() => {
  const shell=document.querySelector('.app-shell')||document.body;
  const root=document.createElement('aside');
  root.id='aiAssistant';root.className='ai-assistant';root.hidden=true;
  root.setAttribute('role','complementary');root.setAttribute('aria-label','AI 助手');
  root.innerHTML=`<button type="button" class="ai-resize" data-resize title="拖动调节宽度，双击恢复默认" aria-label="拖动调节助手宽度"></button>
    <header class="ai-head">
      <div><b>AI 助手</b><span data-project-hint>跟随顶栏项目</span></div>
      <div class="ai-head-actions">
        <button type="button" class="button ghost small" data-history aria-haspopup="true" aria-expanded="false">历史</button>
        <button type="button" class="button ghost small" data-new>新对话</button>
        <button type="button" class="ai-icon-btn" data-close aria-label="关闭 AI 助手">×</button>
      </div>
    </header>
    <div class="ai-history-menu" hidden role="menu"></div>
    <div class="ai-messages" role="log" aria-live="polite"></div>
    <form class="ai-compose">
      <div class="ai-compose-box">
        <textarea class="ai-compose-input" aria-label="发送给 AI 助手的消息" placeholder="快来与我对话吧（Shift+Enter 换行）" rows="2" maxlength="8000"></textarea>
        <div class="ai-compose-bar">
          <div class="ai-compose-model">
            <button type="button" class="ai-model-toggle" data-model-toggle aria-haspopup="listbox" aria-expanded="false">
              <span class="ai-model-icon" data-family="glm">智</span>
              <span data-model-label>GLM-5.3-Flash</span>
              <span class="ai-model-caret" aria-hidden="true"></span>
            </button>
            <div class="ai-model-menu" hidden role="listbox"></div>
          </div>
          <div class="ai-compose-mode">
            <button type="button" class="ai-model-toggle ai-mode-toggle" data-mode-toggle aria-haspopup="listbox" aria-expanded="false" title="审批模式">
              <span class="ai-mode-icon" aria-hidden="true">
                <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M9 12l2 2 4-5"/><path d="M12 3l7 4v5c0 4.4-3 8.1-7 9-4-.9-7-4.6-7-9V7z"/></svg>
              </span>
              <span data-mode-label>每次确认</span>
              <span class="ai-model-caret" aria-hidden="true"></span>
            </button>
            <div class="ai-model-menu ai-mode-menu" hidden role="listbox"></div>
          </div>
          <small data-compose-hint hidden></small>
          <div class="ai-compose-tools">
            <button type="button" class="ai-tool-btn" data-stop hidden aria-label="停止">
              <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><rect x="6" y="6" width="12" height="12" rx="2" fill="currentColor"/></svg>
            </button>
            <button type="submit" class="ai-send-btn" aria-label="发送">
              <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M22 2 11 13"/><path d="M22 2 15 22l-4-9-9-4 20-7z"/></svg>
            </button>
          </div>
        </div>
      </div>
    </form>`;
  shell.append(root);

  const launch=document.createElement('button');
  launch.className='button ghost small ai-launch';launch.type='button';
  launch.innerHTML='<span aria-hidden="true">✦</span><b>AI 助手</b>';
  launch.setAttribute('aria-controls','aiAssistant');
  launch.setAttribute('aria-expanded','false');
  launch.title='到首页和 AI 助手对话';
  const actions=document.querySelector('.top-actions');
  if(actions){
    const theme=actions.querySelector('#themeToggle');
    actions.insertBefore(launch, theme||actions.firstChild);
  }else{
    document.body.append(launch);
  }

  const log=root.querySelector('.ai-messages');
  log.addEventListener('click',e=>{
    const b=e.target.closest('.md-code-copy');
    if(!b||!log.contains(b))return;
    copyText(b.closest('.md-code-block')?.querySelector('pre code')?.textContent||'','已复制');
  });
  const input=root.querySelector('.ai-compose textarea');
  const historyMenu=root.querySelector('.ai-history-menu');
  const modelMenu=root.querySelector('.ai-compose .ai-model-menu');
  const modelToggle=root.querySelector('.ai-compose [data-model-toggle]');
  const modeMenu=root.querySelector('.ai-compose .ai-mode-menu');
  const modeToggle=root.querySelector('.ai-compose [data-mode-toggle]');
  const DEFAULT_MODEL='glm-5.3-flash';
  const PANEL_MIN=280, PANEL_DEFAULT=380, PANEL_KEY='loopforge.assistantWidth';
  const MODE_KEY='loopforge.assistantApprovalMode';
  const APPROVAL_MODES=[
    {id:'confirm',name:'每次确认',desc:'写入转为草稿，逐个审批后执行'},
    {id:'assisted',name:'帮我批准',desc:'保存类自动执行，删除/发布等危险操作需确认'},
    {id:'auto',name:'自动执行',desc:'AI 直接执行全部操作，一次完成任务'}
  ];
  function panelMax(){
    const nav=window.innerWidth<=900?76:232;
    return Math.max(PANEL_MIN, window.innerWidth-nav-240);
  }
  function currentPanelWidth(){
    return parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--ai-panel-w'))||PANEL_DEFAULT;
  }
  function applyPanelWidth(px){
    const w=Math.round(Math.min(panelMax(), Math.max(PANEL_MIN, Number(px)||PANEL_DEFAULT)));
    document.documentElement.style.setProperty('--ai-panel-w', w+'px');
    return w;
  }
  function persistPanelWidth(){
    localStorage.setItem(PANEL_KEY, String(currentPanelWidth()));
  }
  applyPanelWidth(Number(localStorage.getItem(PANEL_KEY))||PANEL_DEFAULT);
  window.addEventListener('resize',()=>applyPanelWidth(currentPanelWidth()));
  (function bindAssistantResize(){
    const handle=root.querySelector('[data-resize]');
    handle.onpointerdown=ev=>{
      if(ev.button!=null&&ev.button!==0)return;
      ev.preventDefault();
      const startX=ev.clientX;
      const startW=root.getBoundingClientRect().width||currentPanelWidth();
      handle.setPointerCapture(ev.pointerId);
      handle.classList.add('dragging');
      document.body.classList.add('ai-assistant-resizing');
      handle.onpointermove=move=>{
        applyPanelWidth(startW+(startX-move.clientX));
      };
      const stop=()=>{
        handle.classList.remove('dragging');
        document.body.classList.remove('ai-assistant-resizing');
        handle.onpointermove=null;
        handle.onpointerup=null;
        handle.onpointercancel=null;
        persistPanelWidth();
      };
      handle.onpointerup=stop;
      handle.onpointercancel=stop;
    };
    handle.ondblclick=()=>{applyPanelWidth(PANEL_DEFAULT);persistPanelWidth()};
    handle.onkeydown=ev=>{
      const cur=root.getBoundingClientRect().width||currentPanelWidth();
      if(ev.key==='ArrowLeft'){ev.preventDefault();applyPanelWidth(cur+16);persistPanelWidth()}
      if(ev.key==='ArrowRight'){ev.preventDefault();applyPanelWidth(cur-16);persistPanelWidth()}
    };
  })();
  const FAMILY_MARK={claude:'',gpt:'',glm:'智',gemini:'G',kimi:'K',deepseek:'D',minimax:'M',hunyuan:'混',echo:'E',other:'AI'};
  const FAMILY_ICON={
    claude:'/images/credentials/claude.svg',
    gpt:'/images/credentials/codex.svg',
    glm:'/images/credentials/glm.svg',
    gemini:'/images/credentials/gemini.svg',
    kimi:'/images/credentials/kimi.svg',
    deepseek:'/images/credentials/deepseek.svg',
    minimax:'/images/credentials/minimax.svg',
    hunyuan:'/images/credentials/hunyuan.svg'
  };
  let messages=[],controller=null,owner='',saving=false,sessionId='',model=DEFAULT_MODEL,models={default:DEFAULT_MODEL,models:[{id:DEFAULT_MODEL,name:'GLM-5.3-Flash',family:'glm',credits:'x0.06'}]};
  let approvalMode=APPROVAL_MODES.some(m=>m.id===localStorage.getItem(MODE_KEY))?localStorage.getItem(MODE_KEY):'confirm';

  function projectId(){return state.projectFilter||''}
  function projectName(){
    const id=projectId();
    return ((state.data?.projects||[]).find(p=>p.id===id)||{}).name||(id?'未命名项目':'未选择项目（仅问答）');
  }
  function hasKey(){return !!(userById(state.currentUser?.id)||{}).codeBuddyApiKeyConfigured}
  function modelMeta(id){return (models.models||[]).find(m=>m.id===id)||{id:id||DEFAULT_MODEL,family:familyOf(id),name:id||DEFAULT_MODEL}}
  function familyOf(id){
    const v=String(id||'').toLowerCase();
    if(v.startsWith('claude'))return 'claude';
    if(v.startsWith('gpt')||v.includes('codex'))return 'gpt';
    if(v.startsWith('glm'))return 'glm';
    if(v.startsWith('gemini'))return 'gemini';
    if(v.startsWith('kimi'))return 'kimi';
    if(v.startsWith('deepseek'))return 'deepseek';
    if(v.startsWith('minimax'))return 'minimax';
    if(v.startsWith('hy')||v.includes('hunyuan'))return 'hunyuan';
    if(v==='echo')return 'echo';
    return 'other';
  }
  function fillModelIcon(el, family, id){
    if(!el)return;
    el.dataset.family=family||'other';
    const src=FAMILY_ICON[family];
    if(src){el.innerHTML=`<img src="${src}" alt="">`;return}
    el.textContent=FAMILY_MARK[family]||String(id||'AI').slice(0,1).toUpperCase();
  }
  function creditLabel(item){
    const credits=String(item?.credits||'').trim();
    if(!credits)return '';
    return credits.endsWith('credits')?credits:credits+' credits';
  }
  function paintModelToggle(toggle, selectedId){
    if(!toggle)return;
    const meta=modelMeta(selectedId);
    const icon=toggle.querySelector('.ai-model-icon');
    if(icon) fillModelIcon(icon, meta.family||familyOf(meta.id), meta.id);
    const label=toggle.querySelector('[data-model-label]');
    if(label)label.textContent=meta.name||meta.id;
    const credits=creditLabel(meta);
    toggle.title=credits?`${meta.name||meta.id} ${credits}`:(meta.id||'');
  }
  function renderModelList(menu, selectedId, onPick){
    renderOptionList(menu, models.models||[], selectedId, onPick);
  }
  // renderOptionList 渲染任意 {id,name,family,credits,icon} 选项列表，供
  // 角色向导等场景复用同一套 ai-model-* 下拉样式。icon:false 跳过图标位。
  function renderOptionList(menu, items, selectedId, onPick){
    if(!menu)return;
    menu.replaceChildren();
    for(const item of items||[]){
      const opt=document.createElement('button');
      opt.type='button';opt.setAttribute('role','option');opt.className='ai-model-option'+(item.id===selectedId?' is-active':'');
      opt.dataset.id=item.id;
      if(item.icon!==false){
        const icon=document.createElement('span');icon.className='ai-model-icon';
        fillModelIcon(icon, item.family||familyOf(item.id), item.id);
        opt.append(icon);
      }
      const label=document.createElement('span');label.className='ai-model-name';label.textContent=item.name||item.id;label.title=item.name?`${item.name} (${item.id})`:item.id;
      const credits=document.createElement('span');credits.className='ai-model-credits';credits.textContent=creditLabel(item);
      opt.append(label,credits);
      opt.onclick=()=>{
        try{
          onPick(item.id);
        }finally{
          menu.hidden=true;
          const t=menu.parentElement?.querySelector('[data-model-toggle]');
          if(t) t.setAttribute('aria-expanded','false');
          closeMenus();
        }
      };
      menu.append(opt);
    }
  }
  // paintItem 按给定的 {id,name,family,credits} 绘制一个 ai-model-toggle。
  function paintItem(toggle, item){
    if(!toggle)return;
    const id=item?.id||'';
    const icon=toggle.querySelector('.ai-model-icon');
    if(icon) fillModelIcon(icon, item?.family||familyOf(id), id);
    const label=toggle.querySelector('[data-model-label]');
    if(label)label.textContent=item?.name||id||'选择模型';
    const credits=creditLabel(item);
    toggle.title=credits?`${item?.name||id} ${credits}`:(item?.name||id||'');
  }
  function renderModelToggle(){paintModelToggle(modelToggle, model)}
  function renderModelMenu(){renderModelList(modelMenu, model, id=>{model=id;renderModelToggle()})}
  function renderModeToggle(){
    const meta=APPROVAL_MODES.find(m=>m.id===approvalMode)||APPROVAL_MODES[0];
    const label=modeToggle.querySelector('[data-mode-label]');
    if(label)label.textContent=meta.name;
    modeToggle.title=meta.desc;
  }
  function renderModeMenu(){
    if(!modeMenu)return;
    modeMenu.replaceChildren();
    for(const item of APPROVAL_MODES){
      const opt=document.createElement('button');
      opt.type='button';opt.setAttribute('role','option');opt.className='ai-model-option ai-mode-option'+(item.id===approvalMode?' is-active':'');
      opt.dataset.id=item.id;
      const label=document.createElement('span');label.className='ai-model-name';label.textContent=item.name;
      const desc=document.createElement('span');desc.className='ai-mode-desc';desc.textContent=item.desc;
      opt.append(label,desc);
      opt.onclick=()=>{
        approvalMode=item.id;
        localStorage.setItem(MODE_KEY,approvalMode);
        renderModeToggle();
        modeMenu.hidden=true;
        modeToggle.setAttribute('aria-expanded','false');
      };
      modeMenu.append(opt);
    }
  }
  function closeMenus(){
    historyMenu.hidden=true;
    modelMenu.hidden=true;
    modeMenu.hidden=true;
    root.querySelector('[data-history]').setAttribute('aria-expanded','false');
    modelToggle.setAttribute('aria-expanded','false');
    modeToggle.setAttribute('aria-expanded','false');
  }
  function add(role,text){
    const el=document.createElement('div');
    el.className=`ai-message ai-role-${role}`;
    el.textContent=text;
    log.append(el);
    log.scrollTop=log.scrollHeight;
    return el;
  }
  function setOpen(open){
    root.hidden=!open;
    launch.classList.toggle('is-open',open);
    launch.setAttribute('aria-expanded',String(open));
    document.body.classList.toggle('ai-assistant-open',open);
    if(!open)closeMenus()
  }
  function shortcuts(){
    const box=document.createElement('div');box.className='ai-shortcuts';
    for(const q of ['角色和 SOP 有什么区别？','根据代码解释 SOP 保存时如何检查版本冲突','帮我构建一个代码评审角色','帮我设计开发、评审、测试的 SOP','用当前项目的 GitHub MCP 看一下未关闭的 Issue']){
      const b=document.createElement('button');b.type='button';b.textContent=q;
      b.onclick=()=>{if(input.disabled)return;input.value=q;input.focus();root.querySelector('.ai-compose').requestSubmit()};
      box.append(b);
    }
    return box;
  }
  function welcome(){
    log.replaceChildren();
    add('assistant','你好！我可以解答平台使用问题，帮你把工作流搭起来。当前项目里已安装的 GitHub 等 HTTP MCP 会由服务端代调。平台写入按下方审批模式处理：默认转为草稿逐个确认；切换到「帮我批准」或「自动执行」后，AI 会直接调用平台写入工具，自动执行模式下一次性完成全部步骤。');
    log.append(shortcuts());
  }
  function reset(){controller?.abort();controller=null;messages=[];sessionId='';welcome();input.value='';syncChrome()}
  function syncChrome(){
    root.querySelector('[data-project-hint]').textContent=projectId()?('项目 · '+projectName()):'未选择项目 · 仅问答';
    const keyed=hasKey();
    const busy=!!controller||saving;
    input.disabled=!keyed||busy;
    input.placeholder=keyed?'快来与我对话吧（Shift+Enter 换行）':'请先在个人设置中配置 CodeBuddy API Key';
    root.querySelector('.ai-compose [type=submit]').disabled=!keyed||busy||!input.value.trim();
    root.querySelector('[data-stop]').hidden=!controller;
    root.querySelector('[data-new]').disabled=busy;
    root.querySelector('[data-history]').disabled=busy;
    modelToggle.disabled=busy;
    modeToggle.disabled=busy;
    const hint=root.querySelector('[data-compose-hint]');
    hint.hidden=true;hint.textContent='';
  }
  function setBusy(busy){
    if(busy){/* controller already set */}
    syncChrome();
  }
  async function loadModels(){
    try{
      const catalog=await api('/api/assistant/models',{silent:true});
      if(catalog?.models?.length){
        models=catalog;
        if(!(models.models||[]).some(m=>m.id===model))model=catalog.default||DEFAULT_MODEL;
      }
    }catch{}
    renderModelToggle();
    renderModelMenu();
    return models;
  }
  window.LoopForgeAssistantModels={
    DEFAULT:DEFAULT_MODEL,
    catalog(){return models},
    load:loadModels,
    paint:paintModelToggle,
    paintItem,
    renderList:renderModelList,
    renderOptions:renderOptionList
  };
  async function loadHistoryMenu(){
    historyMenu.replaceChildren();
    const fresh=document.createElement('button');fresh.type='button';fresh.className='ai-history-new';fresh.textContent='新对话';
    fresh.onclick=()=>{reset();closeMenus()};
    historyMenu.append(fresh);
    try{
      const items=await api('/api/assistant/sessions',{silent:true});
      if(!items?.length){
        const empty=document.createElement('div');empty.className='ai-history-empty';empty.textContent='还没有历史对话';historyMenu.append(empty);return;
      }
      for(const item of items){
        const row=document.createElement('div');row.className='ai-history-item'+(item.id===sessionId?' is-active':'');
        const open=document.createElement('button');open.type='button';open.className='ai-history-open';
        open.textContent=item.title||'未命名对话';
        open.title=item.updatedAt?new Date(item.updatedAt).toLocaleString():'';
        open.onclick=()=>openSession(item.id);
        const del=document.createElement('button');del.type='button';del.className='ai-history-del';del.setAttribute('aria-label','删除对话');del.textContent='×';
        del.onclick=async e=>{
          e.stopPropagation();
          try{
            await api('/api/assistant/sessions/'+encodeURIComponent(item.id),{method:'DELETE',silent:true});
            if(sessionId===item.id)reset();
            await loadHistoryMenu();
          }catch(err){add('error',err.message)}
        };
        row.append(open,del);historyMenu.append(row);
      }
    }catch(err){
      const empty=document.createElement('div');empty.className='ai-history-empty';empty.textContent=err.message||'读取历史失败';historyMenu.append(empty);
    }
  }
  function intervalLabel(sec){
    const n=Number(sec)||0;
    if(n>=86400&&n%86400===0)return n===86400?'每天':`每 ${n/86400} 天`;
    if(n>=3600&&n%3600===0)return n===3600?'每小时':`每 ${n/3600} 小时`;
    if(n>=60&&n%60===0)return `每 ${n/60} 分钟`;
    return n?`每 ${n} 秒`:'';
  }
  function scheduleLabel(task){
    if(task?.cron)return task.cron;
    return intervalLabel(task?.intervalSec||3600);
  }
  function parseAssistantTurn(content){
    try{
      const parsed=JSON.parse(content);
      if(parsed&&typeof parsed.message==='string')return parsed;
    }catch{}
    return {message:content,drafts:[]};
  }
  function isSaveReceipt(text){
    return /^(已通过 MCP 执行 |已保存助手定时任务：|上一个草稿已确认保存成功|刚才的草稿保存失败：)/.test(String(text||''));
  }
  function renderTurn(message){
    if(message.role==='user'){
      if(isSaveReceipt(message.content))return;
      add('user',message.content);return;
    }
    const parsed=parseAssistantTurn(message.content);
    const el=add('assistant','');
    el.innerHTML=renderMarkdown(parsed.message||'');
    el.classList.add('markdown-body');
    for(const d of parsed.drafts||[])showDraft(d);
  }
  async function openSession(id){
    closeMenus();
    try{
      const item=await api('/api/assistant/sessions/'+encodeURIComponent(id),{silent:true});
      controller?.abort();controller=null;
      sessionId=item.id||id;
      messages=Array.isArray(item.messages)?item.messages:[];
      if(item.model)model=item.model;
      renderModelToggle();renderModelMenu();
      log.replaceChildren();
      if(!messages.length)welcome();
      else for(const m of messages)renderTurn(m);
      syncChrome();
      log.scrollTop=log.scrollHeight;
    }catch(err){add('error',err.message||'无法打开对话')}
  }
  function openPanel(){
    setOpen(true);
    const who=String(state.currentUser?.id||state.currentUser?.loginName||'');
    if(owner!==who){reset();owner=who}
    loadModels();
    loadHistoryMenu();
    syncChrome();
    if(hasKey())input.focus();
  }
  // 平台只保留首页这一个对话入口；面板留着只为复用模型目录等能力。
  launch.onclick=()=>{setOpen(false);if(typeof switchView==='function')switchView('chat');setTimeout(()=>document.getElementById('homeChatInput')?.focus(),50)};
  root.querySelector('[data-close]').onclick=()=>{setOpen(false);launch.focus()};
  root.querySelector('[data-new]').onclick=reset;
  root.querySelector('[data-stop]').onclick=()=>controller?.abort();
  root.querySelector('[data-history]').onclick=async e=>{
    e.stopPropagation();
    const open=historyMenu.hidden;
    closeMenus();
    if(open){await loadHistoryMenu();historyMenu.hidden=false;root.querySelector('[data-history]').setAttribute('aria-expanded','true')}
  };
  modelToggle.onclick=e=>{
    e.stopPropagation();
    const open=modelMenu.hidden;
    closeMenus();
    if(open){renderModelMenu();modelMenu.hidden=false;modelToggle.setAttribute('aria-expanded','true')}
  };
  modeToggle.onclick=e=>{
    e.stopPropagation();
    const open=modeMenu.hidden;
    closeMenus();
    if(open){renderModeMenu();modeMenu.hidden=false;modeToggle.setAttribute('aria-expanded','true')}
  };
  document.addEventListener('click',e=>{if(!root.contains(e.target))closeMenus()});
  document.addEventListener('change',e=>{if(e.target?.id==='projectSwitcher')syncChrome()});
  root.addEventListener('keydown',e=>{if(e.key==='Escape'){if(!historyMenu.hidden||!modelMenu.hidden){closeMenus();e.stopPropagation();return}root.querySelector('[data-close]').click()}});
  input.onkeydown=e=>{if(e.key==='Enter'&&!e.shiftKey&&!e.isComposing){e.preventDefault();root.querySelector('.ai-compose').requestSubmit()}};
  input.oninput=()=>{
    input.style.height='auto';
    input.style.height=Math.min(input.scrollHeight,140)+'px';
    syncChrome();
  };
  input.onfocus=syncChrome;

  async function refresh(){const data=await api('/api/bootstrap',{silent:true});Object.assign(state.data,data);renderAll()}
  function draftKind(tool){
    const labels={
      agents_save:{label:'角色',save:'保存角色',view:'agents'},
      workflows_save:{label:'SOP',save:'保存 SOP',view:'flow'},
      workflows_publish:{label:'发布 SOP',save:'发布',view:'flow',danger:true},
      workflows_delete:{label:'删除 SOP',save:'删除',view:'sops',danger:true},
      workflows_clone:{label:'克隆 SOP',save:'克隆',view:'flow'},
      dockerfiles_save:{label:'Dockerfile',save:'保存 Dockerfile',view:'dockerfiles'},
      dockerfiles_delete:{label:'删除 Dockerfile',save:'删除',view:'dockerfiles',danger:true},
      environments_save:{label:'环境安装脚本',save:'保存环境',view:'environments'},
      environments_delete:{label:'删除环境安装脚本',save:'删除',view:'environments',danger:true},
      sandboxes_save:{label:'沙箱',save:'保存沙箱',view:'sandboxes'},
      sandboxes_delete:{label:'删除沙箱',save:'删除',view:'sandboxes',danger:true},
      rules_save:{label:'规则',save:'保存规则',view:'rules'},
      rules_delete:{label:'删除规则',save:'删除',view:'rules',danger:true},
      skills_save:{label:'技能',save:'保存技能',view:'skills'},
      skills_delete:{label:'删除技能',save:'删除',view:'skills',danger:true},
      mcp_servers_save:{label:'MCP',save:'保存 MCP',view:'mcp'},
      mcp_servers_import:{label:'导入 MCP',save:'导入',view:'mcp'},
      schedule_jobs_save:{label:'平台定时任务',save:'保存定时任务',view:'issues'},
      schedule_jobs_delete:{label:'删除平台定时任务',save:'删除',view:'issues',danger:true},
      schedule_jobs_run:{label:'立即执行平台定时任务',save:'立即执行',view:'issues',danger:true},
      assistant_tasks_save:{label:'助手定时任务',save:'保存定时任务'},
      runs_start:{label:'启动 Run',save:'启动',view:'runs',danger:true},
      issues_save:{label:'需求',save:'保存需求',view:'issues'},
      issues_dispatch:{label:'派发 SOP',save:'派发',view:'runs',danger:true},
      projects_save:{label:'项目',save:'保存项目',view:'projects'},
      repositories_save:{label:'仓库',save:'保存仓库',view:'projects'},
      repositories_delete:{label:'删除仓库',save:'删除',view:'projects',danger:true}
    };
    if(labels[tool])return labels[tool];
    const danger=/(_delete|_publish|_dispatch|_start|_cancel|_reject|_run|_restore|_pass|_retry|_pause|_resume|_reopen)$/.test(tool)||tool==='sync_start';
    let save='确认执行';
    if(tool.endsWith('_save'))save='保存';
    else if(tool.endsWith('_delete'))save='删除';
    else if(tool.endsWith('_publish'))save='发布';
    else if(tool.endsWith('_start')||tool.endsWith('_run'))save='执行';
    return {label:tool,save,danger};
  }
  function draftTitle(draft){
    return draft.body?.name||draft.body?.id||draft.path?.id||draft.body?.workflowId||'未命名';
  }
  function sameDraft(a,b){
    if(!a||!b||a.tool!==b.tool)return false;
    const aid=a.body?.id||a.path?.id||a.savedId||'';
    const bid=b.body?.id||b.path?.id||b.savedId||'';
    if(aid&&bid)return String(aid)===String(bid);
    return JSON.stringify(a.path||{})===JSON.stringify(b.path||{})&&JSON.stringify(a.body||{})===JSON.stringify(b.body||{});
  }
  function markDraftSaved(draft,saved){
    const id=saved?.id||draft.body?.id||draft.path?.id||'';
    draft.saved=true;
    draft.savedId=id;
    if(saved&&typeof saved==='object')draft.savedResult=saved;
    for(const m of messages){
      if(m.role!=='assistant')continue;
      const parsed=parseAssistantTurn(m.content);
      if(!parsed.drafts?.length)continue;
      let changed=false;
      for(const d of parsed.drafts){
        if(!sameDraft(d,draft))continue;
        d.saved=true;d.savedId=id;
        if(saved&&typeof saved==='object')d.savedResult=saved;
        changed=true;
      }
      if(changed){
        const out={message:parsed.message,drafts:parsed.drafts};
        if(parsed.lookups)out.lookups=parsed.lookups;
        if(parsed.sources)out.sources=parsed.sources;
        m.content=JSON.stringify(out);
      }
    }
  }
  async function persistSession(){
    if(!sessionId||!messages.length)return;
    try{
      await api('/api/assistant/sessions/'+encodeURIComponent(sessionId),{method:'PUT',silent:true,body:JSON.stringify({id:sessionId,projectId:projectId(),model,messages})});
    }catch{}
  }
  function bindSavedDraft(save,result,draft,saved,kind){
    const data=saved&&typeof saved==='object'?saved:(draft.savedResult||draft.body||{});
    save.disabled=false;save.className='button ghost small';
    result.textContent=kind.danger?'已执行':'已保存';
    save.textContent=kind.view?'打开':'完成';
    save.onclick=async()=>{
      if(draft.tool==='workflows_save'&&state.workflowDirty&&!await afConfirm({title:'未保存更改',message:'当前流程有未保存修改，打开生成的 SOP 将放弃这些修改。继续？',danger:true}))return;
      const project=data.projectId||draft.body?.projectId;
      if(project){state.projectFilter=project;localStorage.setItem('loopforge.activeProject',state.projectFilter)}
      setOpen(false);
      openDraftTarget(draft.tool,data,kind.view);
    };
  }
  function showDraft(draft){
    const kind=draftKind(draft.tool);
    const card=document.createElement('section');card.className='ai-draft';
    if(draft.auto){
      // 自动执行的草稿只渲染执行回执，不再弹出审批卡片。
      card.classList.add('ai-draft-auto');
      const title=document.createElement('strong');title.textContent=(draft.saved?'已自动执行':'自动执行失败')+' · '+kind.label+' · '+draftTitle(draft);card.append(title);
      const result=document.createElement('small');
      const hb=draft.saved?null:humanizeDraftError(draft.saveError||'');
      result.textContent=draft.saved?('已通过 MCP 自动执行'+(draft.savedResult?.id?' · '+draft.savedResult.id:'')):('自动执行失败'+(hb.display?'：'+hb.display:''));
      card.append(result);
      if(!draft.saved)setTimeout(()=>queueDraftFailureFeedback(draft,hb.text),60);
      log.append(card);return;
    }
    const title=document.createElement('strong');title.textContent=kind.label+' · '+draftTitle(draft);card.append(title);
    const description=document.createElement('p');
    description.textContent=draft.body?.taskPrompt?.objective||draft.body?.description||(draft.tool==='assistant_tasks_save'?`${scheduleLabel(draft.body||{})} · ${draft.body?.model||'默认模型'}`:(draft.tool==='schedule_jobs_save'?`${draft.body?.skipDispatch?'AI 定时任务':'触发任务'} · ${scheduleLabel(draft.body||{})}`:(draft.body?.driver||(kind.danger?'确认后将立即调用平台接口':''))));
    if(description.textContent)card.append(description);
    if(draft.warnings?.length){
      const warn=document.createElement('ul');warn.className='ai-draft-warn';
      for(const w of draft.warnings){const li=document.createElement('li');li.textContent=w;warn.append(li)}
      card.append(warn);
    }
    const steps=draft.tool==='agents_save'?(draft.body?.taskPrompt?.actions||[]):draft.tool==='workflows_save'?(draft.body?.nodes||[]).map(n=>n.name||n.type):[];
    if(steps.length){const list=document.createElement('ol');for(const text of steps.slice(0,8)){const item=document.createElement('li');item.textContent=text;list.append(item)}card.append(list)}
    const preview={tool:draft.tool};if(draft.path)preview.path=draft.path;if(draft.query)preview.query=draft.query;if(draft.body!==undefined)preview.body=draft.body;
    const detail=document.createElement('details'),summary=document.createElement('summary');summary.textContent='预览完整定义';const pre=document.createElement('pre');pre.textContent=JSON.stringify(preview,null,2);detail.append(summary,pre);card.append(detail);
    const result=document.createElement('small');result.textContent=(kind.danger?'待确认':'待保存')+' · '+(draft.body?.projectId||projectId()||'未选择项目');card.append(result);
    const save=document.createElement('button');save.type='button';save.className=kind.danger?'button danger-outline small':'button primary small';save.textContent=kind.save;card.append(save);
    if(draft.saved){
      bindSavedDraft(save,result,draft,draft.savedResult,kind);
      log.append(card);return;
    }
    save.onclick=async()=>{
      if(saving)return;
      if(kind.danger&&!await afConfirm({title:kind.label,message:'将通过 MCP 执行 '+draft.tool+'（'+draftTitle(draft)+'）。此操作可能发布、启动或删除平台资源。',danger:true,confirmText:kind.save}))return;
      saving=true;save.disabled=true;syncChrome();result.textContent='正在提交…';
      try{
        const args={};
        if(draft.tool==='assistant_tasks_save'){
          args.body={
            id:draft.body?.id||'',
            name:draft.body?.name||'',
            prompt:draft.body?.prompt||'',
            intervalSec:draft.body?.cron?0:Number(draft.body?.intervalSec)||3600,
            cron:draft.body?.cron||'',
            enabled:draft.body?.enabled!==false,
            model:draft.body?.model||model,
            projectId:draft.body?.projectId||projectId()
          };
        }else{
          if(draft.path&&Object.keys(draft.path).length)args.path=draft.path;
          if(draft.query&&Object.keys(draft.query).length)args.query=draft.query;
          if(draft.body!==undefined&&draft.body!==null)args.body=draft.body;
        }
        const res=await api('/mcp',{method:'POST',body:JSON.stringify({jsonrpc:'2.0',id:Date.now(),method:'tools/call',params:{name:draft.tool,arguments:args}})});
        if(res.error||res.result?.isError)throw new Error(res.error?.message||res.result?.content?.[0]?.text||'提交失败');
        if(!res.result)throw new Error('保存响应无效');
        const saved=res.result.structuredContent?.data||draft.body||{};
        bindSavedDraft(save,result,draft,saved,kind);
        markDraftSaved(draft,saved);
        messages.push({role:'user',content:`已通过 MCP 执行 ${draft.tool}：${saved.id||saved.name||draftTitle(draft)}。后续修改请使用最新定义。`});
        await persistSession();
        try{await refresh()}catch{result.textContent=(kind.danger?'已执行':'已保存')+'；页面刷新失败，可稍后刷新页面查看'}
        // 保存成功后自动续跑下一步（延迟到 finally 释放 saving 后）。
        setTimeout(queueAutoContinue,60);
      }catch(e){
        const hb=humanizeDraftError(e.message);
        result.textContent='保存失败：'+hb.display+(hb.fromServer?'；已把失败原因发回给 AI 自动修正':'');
        save.disabled=false;
        if(hb.fromServer)setTimeout(()=>queueDraftFailureFeedback(draft,hb.text),60);
      }
      finally{saving=false;syncChrome()}
    };
    log.append(card);
  }
  function openDraftTarget(tool,saved,view){
    const id=saved.id||saved.workflowId;
    if(tool==='agents_save'){state.selectedAgent=id;switchView('agents');openAgentWizard((state.data.agents||[]).find(a=>a.id===id)||saved);return}
    if(tool==='workflows_save'||tool==='workflows_clone'||tool==='workflows_publish'){
      const wf=(state.data.workflows||[]).find(w=>w.id===id)||saved;
      if(wf&&wf.nodes){state.workflow=clone(wf);state.selectedNode=null;markWorkflowClean();renderAll();switchView('flow');return}
    }
    if(tool==='runs_start'||tool==='issues_dispatch'){switchView('runs');if(id&&typeof showRunDetail==='function')showRunDetail(id);return}
    if(view)switchView(view);
  }
  // 时间线：思考折叠框（完成后收起为"已完成思考"）、回答分段（Markdown）、
  // 工具调用行（保留不消除，转圈→完成态）。timeline 记录当前追加目标。
  let timeline=null;
  function resetTimeline(){timeline=null}
  // 仅当用户本来就停在底部附近时才自动跟随滚动，避免流式输出把
  // 向上翻阅的视口强拉回底部。
  function nearBottom(){return log.scrollHeight-log.scrollTop-log.clientHeight<120}
  function timelineScroll(){if(nearBottom())log.scrollTop=log.scrollHeight}
  // 收起当前展开的思考框，并把标题改为"已完成思考"。
  function collapseThinking(pending){
    pending.querySelectorAll('.ai-think[open]').forEach(box=>{
      box.open=false;
      const s=box.querySelector('summary');
      if(s)s.textContent='已完成思考';
    });
  }
  // 把仍在转圈的工具行标记为完成（保留展示，不移除）。
  function settleToolLine(line){
    if(!line||line.classList.contains('is-done'))return;
    line.classList.add('is-done');
    const sp=line.querySelector('.ai-tool-spinner');
    if(sp){sp.classList.remove('ai-tool-spinner');sp.classList.add('ai-tool-check')}
  }
  function newThinkingSeg(pending){
    collapseThinking(pending);
    const box=document.createElement('details');box.className='ai-think';box.open=true;
    box.innerHTML='<summary>正在思考…</summary><div class="ai-think-body"></div>';
    pending.append(box);timelineScroll();
    return box.querySelector('.ai-think-body');
  }
  function newAnswerPart(pending){
    collapseThinking(pending);
    const el=document.createElement('div');el.className='ai-answer ai-answer-part markdown-body';el.__raw='';
    pending.append(el);timelineScroll();return el;
  }
  function paintAnswerPart(el){
    if(el.__raf)return;
    el.__raf=requestAnimationFrame(()=>{el.__raf=0;el.innerHTML=renderMarkdown(el.__raw||'');timelineScroll()});
  }
  function newToolLine(pending,ev){
    collapseThinking(pending);
    const line=document.createElement('div');line.className='ai-tool-line';line.dataset.toolId=ev.id||'';
    line.innerHTML='<span class="ai-tool-spinner" aria-hidden="true"></span><span class="ai-tool-line-text"></span>';
    line.querySelector('.ai-tool-line-text').textContent=ev.text||('调用工具 '+(ev.tool||''));
    pending.append(line);timelineScroll();return line;
  }
  function finishToolLine(pending,id){
    if(!id)return;
    pending.querySelectorAll('.ai-tool-line').forEach(line=>{if(line.dataset.toolId===id)settleToolLine(line)});
  }
  function setStatusLine(pending,text){
    let line=pending.querySelector('.ai-status-line');
    if(!line){line=document.createElement('div');line.className='ai-status-line';pending.append(line)}
    line.textContent=text;timelineScroll();
  }
  function pushThought(pending,ev){
    switch(ev.kind||''){
      case 'thinkingStart':
        collapseThinking(pending);
        timeline={kind:'thinking',el:null};
        return;
      case 'thinking':
        if(!ev.text)return;
        if(!timeline||timeline.kind!=='thinking'||!timeline.el)timeline={kind:'thinking',el:newThinkingSeg(pending)};
        timeline.el.textContent+=ev.text;
        timelineScroll();
        return;
      case 'answer':
        if(!ev.text)return;
        if(!timeline||timeline.kind!=='answer')timeline={kind:'answer',el:newAnswerPart(pending)};
        timeline.el.__raw+=ev.text;
        paintAnswerPart(timeline.el);
        return;
      case 'tool':
        if(String(ev.tool||'').replace(/^mcp__loopforge_session__/,'')==='submit_assistant_reply')return;
        timeline={kind:'tool',el:newToolLine(pending,ev)};
        return;
      case 'toolDone':
        finishToolLine(pending,ev.id);
        return;
    }
    if(ev.text)setStatusLine(pending,ev.text);
  }
  async function consumeAssistantStream(resp){
    const reader=resp.body.getReader();
    const decoder=new TextDecoder();
    let buffer='',result=null,error=null;
    const dispatch=(event,dataStr)=>{
      let payload={};try{payload=JSON.parse(dataStr)}catch{return}
      if(event==='thought')pushThought(consumeAssistantStream.pending,payload);
      else if(event==='result')result=payload;
      else if(event==='error')error=payload.error||'生成失败';
    };
    for(;;){
      const {value,done}=await reader.read();
      if(done)break;
      buffer+=decoder.decode(value,{stream:true});
      let idx;
      while((idx=buffer.indexOf('\n\n'))>=0){
        const chunk=buffer.slice(0,idx);buffer=buffer.slice(idx+2);
        let event='message',dataStr='';
        chunk.split('\n').forEach(line=>{
          if(line.startsWith('event:'))event=line.slice(6).trim();
          else if(line.startsWith('data:'))dataStr+=(dataStr?'\n':'')+line.slice(5).trimStart();
        });
        if(dataStr)dispatch(event,dataStr);
      }
    }
    if(error)throw new Error(error);
    if(!result)throw new Error('生成中断，未返回完整回答');
    return result;
  }

  // confirm 模式下草稿保存成功后自动续跑下一步，直到计划完成；
  // 手动发送会重置计数，连续续跑有上限防失控。
  let autoContinues=0;
  const AUTO_CONTINUE_MAX=30;
  function queueAutoContinue(){
    if(controller||saving)return;
    if(approvalMode==='auto')return;
    if(autoContinues>=AUTO_CONTINUE_MAX){
      add('assistant','已连续自动续跑 '+AUTO_CONTINUE_MAX+' 轮，先停一下。需要继续时发送"继续"即可。');
      return;
    }
    autoContinues++;
    sendAssistantTurn('上一个草稿已确认保存成功，请继续执行计划中的下一步；如果计划已全部完成，请直接给出总结，不要再提交新草稿。',false,true,'已保存，正在让 AI 继续下一步…');
  }
  // 把服务端错误整理成给用户看的文案 + 给 AI 的原始信息。
  function humanizeDraftError(msg){
    let text=String(msg||'').trim();
    let fromServer=false;
    try{
      const p=JSON.parse(text);
      if(p&&typeof p==='object'&&p.error){text=String(p.error);fromServer=true}
    }catch{}
    text=text.trim().slice(0,300);
    const display=text
      .replace(/(?:workflow|rule|rules|agent|agents|sandbox|skill|environment|dockerfile)\s+(\w+)(\s+is\s+required)/gi,'缺少必填字段 "$1"')
      .replace(/(\w+)\s+is\s+required/gi,'缺少必填字段 "$1"')
      .replace(/is required/gi,'缺少必填字段');
    return{text,display,fromServer};
  }
  // 草稿执行失败时自动打回给 AI：带原始错误让它修正后重新提交。
  function queueDraftFailureFeedback(draft,errText){
    if(controller||saving)return;
    if(autoContinues>=AUTO_CONTINUE_MAX)return;
    autoContinues++;
    sendAssistantTurn('刚才的草稿保存失败：'+draft.tool+'（'+draftTitle(draft)+'）。服务端返回错误：'+(errText||'执行失败')+'。请分析原因并修正：若是缺少必填字段或字段不符合平台 schema（例如 sandboxProfileId、instructions、revision），先用只读工具读取现有定义与要求，修正后重新提交草稿；若是依赖未满足，先提交能解除依赖的草稿，或向我说明需要先做什么。',false,true,'保存失败，正在让 AI 重新生成…');
  }
  // 系统状态行：自动续跑/失败打回等内部动作以轻量提示展示，不出用户气泡。
  function addSystemNote(text){
    const line=document.createElement('div');line.className='ai-system-note';
    const tx=document.createElement('span');tx.textContent=text;
    line.append(tx);log.append(line);log.scrollTop=log.scrollHeight;
    return line;
  }
  async function sendAssistantTurn(text,reuseUser,isAuto,note){
    if(!isAuto)autoContinues=0;
    if(controller||saving||!String(text||'').trim())return;
    if(!hasKey()){syncChrome();return}
    text=String(text).trim();
    let noteEl=null;
    if(!reuseUser){
      messages.push({role:'user',content:text});
      if(isAuto)noteEl=addSystemNote(note||'正在继续…');
      else{add('user',text);input.value=''}
    }
    controller=new AbortController();const current=controller;setBusy(true);const pending=add('assistant','正在整理方案…');
    pending.textContent='';
    consumeAssistantStream.pending=pending;
    resetTimeline();
    try{
      const res=await fetch('/api/assistant/chat',{
        method:'POST',
        headers:{'Content-Type':'application/json','Accept':'text/event-stream'},
        signal:current.signal,
        body:JSON.stringify({projectId:projectId(),sessionId,model,approvalMode,messages})
      });
      const ctype=res.headers.get('Content-Type')||'';
      let reply;
      if(!res.ok){
        let em=`HTTP ${res.status}`;try{const d=await res.json();em=d.error||em}catch{}
        throw new Error(em);
      }
      if(ctype.includes('text/event-stream'))reply=await consumeAssistantStream(res);
      else reply=await res.json();
      if(reply.error)throw new Error(reply.error);
      sessionId=reply.sessionId||sessionId;
      pending.querySelector('.ai-status-line')?.remove();
      // 思考框收起、工具行标记完成（保留过程展示，不移除）。
      collapseThinking(pending);
      pending.querySelectorAll('.ai-tool-line:not(.is-done)').forEach(line=>settleToolLine(line));
      // 最后一个回答分段即模型的最终回答，用 result 的权威文本替换；
      // 之前的思考框与回答分段保留为过程记录。
      const parts=pending.querySelectorAll('.ai-answer-part');
      const body=document.createElement('div');body.className='ai-answer markdown-body';body.innerHTML=renderMarkdown(reply.message||'');
      if(parts.length)parts[parts.length-1].replaceWith(body);
      else pending.append(body);
      pending.classList.add('markdown-body');
      messages.push({role:'assistant',content:JSON.stringify({message:reply.message,drafts:reply.drafts})});
      if(reply.sources?.length){
        const details=document.createElement('details');details.className='ai-sources';
        const title=document.createElement('summary');title.textContent='代码依据 · '+new Set(reply.sources.map(s=>s.path)).size+' 个文件';details.append(title);
        const pre=document.createElement('pre');pre.textContent=reply.sources.map(s=>s.path+':'+s.line+'  '+s.text).join('\n');details.append(pre);log.append(details);
      }
      for(const d of reply.drafts||[])showDraft(d);
      loadHistoryMenu();
    }catch(err){
      pending.classList.remove('markdown-body');
      pending.classList.add('ai-role-error');
      const msg=err.name==='AbortError'?'已停止生成。':err.message;
      pending.querySelector('.ai-status-line')?.remove();
      const textEl=document.createElement('div');
      textEl.textContent=msg;
      pending.append(textEl);
      if(err.name!=='AbortError'){
        const retry=document.createElement('button');
        retry.type='button';
        retry.className='button primary small ai-retry';
        retry.textContent='重试';
        retry.onclick=()=>{
          pending.remove();
          sendAssistantTurn(text,true);
        };
        pending.append(retry);
      }else{
        messages.pop();
        input.value=text;
      }
    }finally{
      if(noteEl){noteEl.classList.add('is-done')}
      if(controller===current)controller=null;setBusy(false);if(nearBottom())log.scrollTop=log.scrollHeight;if(hasKey())input.focus();
    }
  }
  root.querySelector('.ai-compose').onsubmit=async e=>{
    e.preventDefault();
    await sendAssistantTurn(input.value,false);
  };
  welcome();
  renderModeToggle();
  syncChrome();
})();
