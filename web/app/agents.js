function renderAgentsLegacy(){
  const list=$('#agentList');list.innerHTML=state.data.agents.map(a=>`<div class="agent-card ${state.selectedAgent===a.id?'active':''}" data-id="${a.id}"><div class="agent-card-top"><h3>${escapeHTML(a.name)}</h3><span class="pill">v${a.version}</span></div><p>${escapeHTML(a.description||'暂无说明')}</p><div class="chips"><span class="chip">${a.rules?.length||0} rules</span><span class="chip">${a.skills?.length||0} skills</span><span class="chip">${a.mcpServers?.length||0} MCP</span></div></div>`).join('');
  list.querySelectorAll('.agent-card').forEach(el=>el.onclick=()=>selectAgent(state.data.agents.find(a=>a.id===el.dataset.id)));
  if(!state.selectedAgent&&state.data.agents[0])selectAgent(state.data.agents[0],false);
}
function renderAgents(){
  const list=$('#agentList');if(!list)return;renderCatalogScopes();
  const query=($('#agentSearch')?.value||'').trim().toLowerCase(),agents=scopedAgents().filter(a=>!query||`${a.name} ${a.id} ${a.description} ${(a.rules||[]).join(' ')} ${(a.skills||[]).join(' ')}`.toLowerCase().includes(query));
  const used=new Map();(state.data.workflows||[]).forEach(w=>(w.nodes||[]).forEach(n=>{if(n.agentId)used.set(n.agentId,(used.get(n.agentId)||0)+1)}));
  $('#agentCatalogStats').innerHTML=`<span><b>${agents.length}</b> 个${state.agentScope==='shared'?'共享':'项目'}角色</span><span><b>${agents.reduce((n,a)=>n+(a.skills?.length||0),0)}</b> 项技能绑定</span><span><b>${agents.reduce((n,a)=>n+(a.mcpServers?.length||0),0)}</b> 个 MCP 连接</span>`;
  const empty=state.agentScope==='shared'?'还没有共享给当前项目的角色。':'当前项目还没有角色。可以新建，或从共享角色复制后改。';
  const hue=t=>{let h=0;for(const c of String(t||''))h=(h*31+c.charCodeAt(0))%360;return h};
  list.innerHTML=agents.map(a=>{const initials=(a.name||'AI').replace(/[^A-Za-z\u4e00-\u9fa5]/g,'').slice(0,2)||'AI';const exec=(state.data.executors||[]).find(x=>x.id===a.executorId);const n=used.get(a.id)||0;
    const meta=[`${a.skills?.length||0} 技能`,`${a.mcpServers?.length||0} MCP`,n?`用在 ${n} 个步骤`:'还没用在流程里'].join(' · ');
    return`<article class="definition-card role-card" data-agent-id="${escapeHTML(a.id)}" style="--h:${hue(a.id)}"><header><span class="role-avatar">${escapeHTML(initials)}</span><div><h3>${escapeHTML(a.name)} ${typeof lfStatusBadge==='function'?lfStatusBadge(a):''}</h3><small>${escapeHTML(exec?.name||a.executorId||'未选 CLI')}${a.model?` · ${escapeHTML(a.model)}`:''}${a.loop?.enabled?` · ${a.loop.maxIterations||1} 轮`:''}</small></div>${a.shared?'<span class="role-shared">共享</span>':''}</header><p>${escapeHTML(a.description||'尚未填写职责说明')}</p><footer><span class="role-meta">${escapeHTML(meta)}</span><span class="role-actions">${shareBadge(a,'agent')}${catalogOwned(a.projectId)?`<button type="button" class="scope-share delete-action" data-delete-agent="${escapeHTML(a.id)}" ${a.shared?'disabled title="共享状态的角色不能删除，请先取消共享"':(n?'disabled title="角色仍被流程引用，请先解除引用"':'')}>删除</button>`:''}</span><span class="role-open">编辑 ›</span></footer></article>`}).join('')+(state.agentScope==='shared'?'':`<button type="button" class="role-card role-new" id="agentNewCard"><span class="role-new-orb" aria-hidden="true"></span><b>新建 Bot</b><small>说清它干什么，剩下的帮你填好</small></button>`)||`<div class="catalog-empty"><span>AI</span><b>${empty}</b><p>调整搜索，或新建一个角色。</p></div>`;
  $('#agentNewCard')&&($('#agentNewCard').onclick=()=>openAgentWizard());
  list.querySelectorAll('[data-share-agent]').forEach(btn=>btn.onclick=e=>{e.stopPropagation();toggleShare('agent',btn.dataset.shareAgent)});
  list.querySelectorAll('[data-clone-agent]').forEach(btn=>btn.onclick=e=>{e.stopPropagation();cloneAgent(btn.dataset.cloneAgent)});
  list.querySelectorAll('[data-delete-agent]').forEach(btn=>btn.onclick=e=>{e.stopPropagation();if(btn.disabled)return;deleteAgent(btn.dataset.deleteAgent)});
  list.querySelectorAll('[data-agent-id]').forEach(card=>card.onclick=()=>openAgentWizard(state.data.agents.find(a=>a.id===card.dataset.agentId)));
}
function pickerHTML(items,selected,prefix){return `<input class="input picker-filter" data-picker-filter="${prefix}" placeholder="筛选名称或说明…">${(items||[]).map(x=>`<label class="asset-check"><input type="checkbox" name="${prefix}" value="${x.id}" ${(selected||[]).includes(x.id)?'checked':''}><span>${escapeHTML(x.name)}<small>${escapeHTML(x.description||x.id)}</small></span></label>`).join('')||'<span class="type-label">暂无可用资产，请先到对应的资产管理页面添加</span>'}`}
function bindPickerFilters(root){(root||document).querySelectorAll('[data-picker-filter]').forEach(input=>input.oninput=()=>{const q=input.value.trim().toLowerCase();input.parentElement.querySelectorAll('.asset-check').forEach(row=>row.hidden=!!q&&!row.textContent.toLowerCase().includes(q))})}
function executorOptions(selected){const allowed=new Set(['codebuddy','codex','claude']),items=(state.data.executors||[]).filter(x=>allowed.has(x.id)),legacy=selected&&!allowed.has(selected)?`<option value="${escapeHTML(selected)}" selected disabled>历史执行器 ${escapeHTML(selected)}（请重新选择 CLI）</option>`:'';return legacy+items.map(x=>`<option value="${escapeHTML(x.id)}" ${x.id===selected?'selected':''}>${escapeHTML(x.name||x.id)}</option>`).join('')}
const AGENT_CUSTOM_MODEL='__custom__';
function executorModelCatalogs(){return window.LoopForgeExecutorModels||{}}
async function loadExecutorModelCatalogs(){
  // 未登录时接口 401，失败结果不缓存：下次打开向导会重试。
  const cached=window.LoopForgeExecutorModels;
  if(cached&&Object.keys(cached).length)return cached;
  try{
    const data=await api('/api/executor-models',{silent:true});
    if(data&&data.codebuddy&&data.codex&&data.claude)window.LoopForgeExecutorModels=data;
  }catch{}
  return executorModelCatalogs();
}
function agentModelAPI(){return window.LoopForgeAssistantModels}
function currentAgentExecutorId(){return $('#agentForm')?.elements?.executorId?.value||'codebuddy'}
function agentModelItems(executorId,current){
  const cat=executorModelCatalogs()[executorId]||{},items=(cat.models||[]).slice();
  if(current&&current!==AGENT_CUSTOM_MODEL&&!items.some(m=>m.id===current))items.push({id:current,name:`${current}（历史模型）`,family:'other'});
  if(cat.customModel)items.push({id:AGENT_CUSTOM_MODEL,name:'手动输入模型 ID…',icon:false});
  return items;
}
function agentEffortItems(executorId){
  const cat=executorModelCatalogs()[executorId]||{};
  return [{id:'',name:'默认',icon:false}].concat((cat.efforts||[]).map(e=>({id:e.value,name:e.label,icon:false})));
}
function paintAgentPicker(pickerId,item){
  const picker=$(pickerId);if(!picker)return;
  agentModelAPI()?.paintItem?.(picker.querySelector('[data-model-toggle]'),item);
}
function renderAgentModelPicker(){
  const picker=$('#agentModelPicker');if(!picker)return;
  const items=agentModelItems(currentAgentExecutorId(),picker.querySelector('[name="modelChoice"]').value);
  const current=picker.querySelector('[name="modelChoice"]').value;
  const menu=picker.querySelector('.ai-model-menu');
  agentModelAPI()?.renderOptions?.(menu,items,current,id=>{
    picker.querySelector('[name="modelChoice"]').value=id;
    syncAgentModelCustomField();
    paintAgentPicker('#agentModelPicker',items.find(x=>x.id===id));
    menu.hidden=true;picker.querySelector('[data-model-toggle]').setAttribute('aria-expanded','false');
  });
  syncAgentModelCustomField();
  paintAgentPicker('#agentModelPicker',items.find(x=>x.id===current)||(current?{id:current,name:current}:null));
}
function syncAgentModelCustomField(){
  const picker=$('#agentModelPicker'),field=$('#agentModelCustomField');
  if(!picker||!field)return;
  field.hidden=picker.querySelector('[name="modelChoice"]').value!==AGENT_CUSTOM_MODEL;
}
function renderAgentEffortPicker(){
  const picker=$('#agentEffortPicker');if(!picker)return;
  const items=agentEffortItems(currentAgentExecutorId());
  const current=picker.querySelector('[name="effort"]').value;
  const menu=picker.querySelector('.ai-model-menu');
  agentModelAPI()?.renderOptions?.(menu,items,current,id=>{
    picker.querySelector('[name="effort"]').value=id;
    paintAgentPicker('#agentEffortPicker',items.find(x=>x.id===id));
    menu.hidden=true;picker.querySelector('[data-model-toggle]').setAttribute('aria-expanded','false');
  });
  paintAgentPicker('#agentEffortPicker',items.find(x=>x.id===current)||items[0]);
}
function applyAgentModelUI(executorId,model,effort){
  const picker=$('#agentModelPicker');if(!picker||!$('#agentEffortPicker'))return;
  const cat=executorModelCatalogs()[executorId]||{};
  let choice=model||'';
  if(choice&&!(cat.models||[]).some(m=>m.id===choice)&&cat.customModel)choice=AGENT_CUSTOM_MODEL;
  picker.querySelector('[name="modelChoice"]').value=choice;
  const customInput=$('#agentModelCustomField')?.querySelector('input[name="model"]');
  if(customInput&&choice===AGENT_CUSTOM_MODEL)customInput.value=model||'';
  renderAgentModelPicker();
  const efforts=cat.efforts||[];
  $('#agentEffortPicker').querySelector('[name="effort"]').value=efforts.some(e=>e.value===effort)?(effort||''):'';
  renderAgentEffortPicker();
}
function bindAgentPicker(pickerId,render){
  const picker=$(pickerId);if(!picker||picker.dataset.bound)return;
  picker.dataset.bound='1';
  picker.querySelector('[data-model-toggle]').onclick=e=>{
    e.stopPropagation();
    const menu=picker.querySelector('.ai-model-menu'),toggle=picker.querySelector('[data-model-toggle]');
    const open=menu.hidden;menu.hidden=!open;
    toggle.setAttribute('aria-expanded',open?'true':'false');
    if(open)render();
  };
}
function agentEffectiveModel(f){
  const choice=f?.elements?.modelChoice?.value;
  if(choice==null)return (f?.elements?.model?.value||'').trim();
  return choice===AGENT_CUSTOM_MODEL?(f?.elements?.model?.value||'').trim():choice;
}
function selectAgent(a,rerender=true){
  state.selectedAgent=a.id;state.agentDraft=null;
  const f=$('#agentForm');
  ['id','name','description','instructions'].forEach(k=>{if(f.elements[k])f.elements[k].value=a[k]||''});
  if(f.elements.shared)f.elements.shared.checked=!!a.shared;
  if(f.elements.executorId)f.elements.executorId.innerHTML=executorOptions(a.executorId||'codebuddy');
  const paintAgentModel=()=>applyAgentModelUI(a.executorId||f.elements.executorId?.value||'codebuddy',a.model||'',a.effort||'');
  paintAgentModel();loadExecutorModelCatalogs().then(paintAgentModel).catch(()=>{});
  if(f.elements.id)f.elements.id.readOnly=!!a.id;
  renderAgentPresets();renderAgentTaskBook(a);renderAgentGovernance(a);renderAgentAssetsPanel(a);renderAgentSummary();
  $('#agentVersion').textContent=`v${a.version||1}`;
  $('#agentWizardTitle').textContent=agentWizardForeign(a)?`查看 ${a.name}`:(a.id?`编辑 ${a.name}`:'新建角色');
  syncAgentOwnershipUI(a);
  if(rerender)renderAgents();
}
function syncAgentOwnershipUI(a){
  const generic=catalogGeneric(state.workflow);
  const foreign=agentWizardForeign(a);
  const save=$('#agentSave'),cloneBtn=$('#agentClone');
  if(save)save.hidden=foreign;
  if(cloneBtn)cloneBtn.hidden=generic||!foreign;
  const share=$('#agentForm')?.elements?.shared;
  if(share)share.disabled=foreign;
}
const agentPresets=[{id:'developer',name:'开发实现',title:'研发工程师',desc:'根据任务书完成代码变更、验证和产物交付',instructions:'你是生产研发工程师。先读取规则、任务输入、上游冻结产物和相关源码；实施最小且完整的变更；运行构建、静态检查和测试；按节点产物契约提交真实证据。不得用自述代替磁盘产物和命令退码。'},{id:'reviewer',name:'独立评审',title:'独立代码审查员',desc:'独立检查正确性、安全性、兼容性和回归风险',instructions:'你是独立审查员，不是实现者。基于代码差异、上游产物和真实验证证据进行审查，明确阻断项、风险和结论；发现问题必须给出具体文件、原因与可执行修复建议。'},{id:'tester',name:'独立测试',title:'独立测试工程师',desc:'覆盖正常、边界、错误和回归路径',instructions:'你是独立测试工程师。根据需求与验收标准设计正常、边界、错误和回归用例，运行真实测试并记录命令、结果、覆盖率与数据清理证据。'},{id:'coordinator',name:'交付协调',title:'交付协调员',desc:'核对 PR、Checks、Pipeline、验收与关闭证据',instructions:'你是交付协调员。只根据可核验的外部状态和冻结产物推进交付，确保 PR、Checks、Pipeline、验收和关闭证据完整；任何关键事实缺失时停止并报告。'}];
function openAgentWizard(agent=null,initialStep=1){
  const open=a=>{
    const presetHost=$('#agentPresetHost');if(presetHost)presetHost.dataset.ready='';
    selectAgent(a||{id:'',name:'',version:1,projectId:state.projectFilter||'',shared:false,description:'',instructions:'',taskPrompt:{},rules:[],skills:[],mcpServers:[],hooks:[],loop:{enabled:false,maxIterations:3,onExhausted:'fail'},interventions:[],artifactOutputs:[],verification:[]},false);
    bindAgentStudio();$('#agentForm').classList.remove('is-dirty');$('#agentForm').classList.toggle('is-creating',!agent);setAgentStudioMode(initialStep>1?'full':(localStorage.getItem('loopforge.agentMode')||'simple'),false);if($('#agentAIInput'))$('#agentAIInput').value='';$('#agentModal').classList.add('open');setAgentWizardStep(initialStep,initialStep>1);
    if(catalogGeneric(state.workflow))loadMCPMarket().then(()=>{const current=workflowAgent(state.selectedAgent)||state.agentDraft||a;if(current)renderAgentAssetsPanel(current)});
  };
  if(agent?.id && !agent.instructions){
    api(`/api/agents/${encodeURIComponent(agent.id)}`).then(full=>{const i=state.data.agents.findIndex(x=>x.id===full.id);if(i>=0)state.data.agents[i]=full;open(full)}).catch(e=>{toast(e.message,'err');open(agent)});
    return;
  }
  open(agent);
}
function applyAgentPreset(id){const p=agentPresets.find(x=>x.id===id),f=$('#agentForm');if(!p)return;f.elements.id.value=f.elements.id.value||`${id}-${Math.random().toString(36).slice(2,6)}`;f.elements.name.value=p.title;f.elements.description.value=p.desc;f.elements.instructions.value=p.instructions;$$('[data-agent-preset]').forEach(b=>b.classList.toggle('active',b.dataset.agentPreset===id));renderAgentSummary();toast(`已套用「${p.name}」模板，可直接保存或继续调整`)}
function setAgentWizardStep(step,scroll=true){
  const f=$('#agentForm');
  state.agentWizardStep=Math.max(1,Math.min(4,step));
  f.querySelectorAll('.wizard-pane').forEach(p=>p.classList.add('active'));
  f.querySelectorAll('[data-wizard-steps="agent"] [data-step-target]').forEach(b=>b.classList.toggle('active',Number(b.dataset.stepTarget)===state.agentWizardStep));
  const main=f.querySelector('.as-main'),pane=f.querySelector(`.wizard-pane[data-step="${state.agentWizardStep}"]`);
  if(scroll&&main&&pane){state.agentScrollLock=Date.now();main.scrollTo({top:state.agentWizardStep===1?0:pane.offsetTop-main.offsetTop-8,behavior:'smooth'})}
  const save=$('#agentSave');
  const foreign=agentWizardForeign(agentWizardSubject());
  save.hidden=foreign;
  save.textContent='保存';
  save.classList.add('primary');save.classList.remove('ghost');
  const cloneBtn=$('#agentClone');
  if(cloneBtn)cloneBtn.hidden=catalogGeneric(state.workflow)||!foreign;
  if(state.agentWizardStep===3&&state.selectedAgent&&scroll){const a=agentWizardSubject();renderAgentGovernance(a)}
  renderAgentSummary();
}
const AGENT_DRAFT_RULES=[
  [/安全|漏洞|权限|security/i,'reviewer','安全审查员'],
  [/评审|审查|review|\bcr\b/i,'reviewer','代码审查员'],
  [/测试|用例|回归|\bqa\b|test/i,'tester','测试工程师'],
  [/发布|上线|合并|交付|checks?|pipeline/i,'coordinator','交付协调员'],
  [/分析|根因|调研|方案|拆解|需求/i,'developer','需求分析师'],
  [/文档|readme|说明书|doc/i,'developer','文档工程师'],
  [/前端|页面|样式|\bui\b|组件/i,'developer','前端工程师'],
  [/后端|接口|\bapi\b|数据库|服务端/i,'developer','后端工程师'],
];
function agentSlug(text){const ascii=String(text||'').toLowerCase().replace(/[^a-z0-9]+/g,'-').replace(/^-+|-+$/g,'');return ascii.length>=3?ascii.slice(0,40):''}
function draftAgentFromText(text){
  const t=String(text||'').trim();if(!t)return toast('先用一句话说说这个角色要干什么');
  const rule=AGENT_DRAFT_RULES.find(r=>r[0].test(t))||[null,'developer','研发工程师'];
  const preset=agentPresets.find(p=>p.id===rule[1])||agentPresets[0];
  const f=$('#agentForm'),el=f.elements;
  const readonlyHint=/不改|只读|只提|不修改|不动代码/.test(t)?'\n不得修改业务代码，只输出结论与建议。':'';
  el.name.value=rule[2];
  if(!el.id.readOnly)el.id.value=`${agentSlug(preset.id)||'role'}-${Math.random().toString(36).slice(2,6)}`;
  el.description.value=t;
  el.instructions.value=`${preset.instructions}\n\n本角色的具体职责：${t}${readonlyHint}`;
  f.classList.add('is-dirty','has-draft');
  renderAgentSummary();
  f.querySelector('.as-draft-flash')?.classList.remove('as-draft-flash');
  requestAnimationFrame(()=>f.querySelector('.wizard-pane[data-step="1"]')?.classList.add('as-draft-flash'));
  el.name.focus();el.name.select();
}
function setAgentStudioMode(mode,remember=true){
  const f=$('#agentForm');if(!f)return;
  const full=mode==='full';
  f.classList.toggle('as-simple',!full);
  $$('[data-agent-mode]').forEach(b=>b.classList.toggle('active',b.dataset.agentMode===(full?'full':'simple')));
  if(remember)localStorage.setItem('loopforge.agentMode',full?'full':'simple');
  if(full&&state.selectedAgent)renderAgentGovernance(agentWizardSubject());
}
function bindAgentStudio(){
  const f=$('#agentForm');if(!f||f.dataset.studio)return;f.dataset.studio='1';
  const main=f.querySelector('.as-main');
  main?.addEventListener('scroll',()=>{
    if(Date.now()-(state.agentScrollLock||0)<600)return;
    const top=main.scrollTop+main.offsetTop+80;let cur=1;
    f.querySelectorAll('.wizard-pane').forEach(p=>{if(p.offsetTop<=top)cur=Number(p.dataset.step)});
    if(main.scrollTop+main.clientHeight>=main.scrollHeight-4)cur=4;
    if(cur!==state.agentWizardStep){state.agentWizardStep=cur;f.querySelectorAll('[data-wizard-steps="agent"] [data-step-target]').forEach(b=>b.classList.toggle('active',Number(b.dataset.stepTarget)===cur))}
  },{passive:true});
  $('#agentCloseAlt')&&($('#agentCloseAlt').onclick=()=>f.querySelector('.agent-close')?.click());
  $$('[data-agent-mode]').forEach(b=>b.onclick=()=>setAgentStudioMode(b.dataset.agentMode));
  $('#agentAIGo')&&($('#agentAIGo').onclick=()=>draftAgentFromText($('#agentAIInput').value));
  $('#agentAIInput')?.addEventListener('keydown',e=>{if(e.key==='Enter'){e.preventDefault();draftAgentFromText(e.target.value)}});
  $$('[data-agent-example]').forEach(b=>b.onclick=()=>{$('#agentAIInput').value=b.dataset.agentExample;draftAgentFromText(b.dataset.agentExample)});
  f.elements.name?.addEventListener('input',()=>{if(!f.elements.id.readOnly&&!f.elements.id.dataset.touched){f.elements.id.value=agentSlug(f.elements.name.value)||f.elements.id.value||`role-${Math.random().toString(36).slice(2,6)}`}});
  f.elements.id?.addEventListener('input',()=>{f.elements.id.dataset.touched='1'});
  const markDirty=()=>{f.classList.add('is-dirty')};
  f.addEventListener('input',markDirty);f.addEventListener('change',markDirty);
  f.addEventListener('keydown',e=>{if((e.metaKey||e.ctrlKey)&&e.key.toLowerCase()==='s'){e.preventDefault();if(!$('#agentSave').hidden)f.requestSubmit()}});
}
async function saveAgent(e){e.preventDefault();const f=e.currentTarget;const existing=state.data.agents.find(a=>a.id===f.elements.id.value);if(!catalogGeneric(state.workflow)&&existing&&catalogForeign(existing.projectId))return toast('这是其他项目的共享角色，请先复制到本项目再改');const checked=name=>[...f.querySelectorAll(`input[name="${name}"]:checked`)].map(x=>x.value);const draft=readAgentDraft(f);const artifacts=draft.artifactOutputs.map((x,i)=>({name:x.name||`artifact-${i+1}`,path:x.path||'result.md',mediaType:x.path?.endsWith('.json')?'application/json':(x.mediaType||'text/markdown'),required:!!x.required})),verification=draft.verification.map((gate,i)=>({id:gate.id||`gate-${i+1}`,name:gate.name||`门禁 ${i+1}`,mode:'command',stage:'after_execution',commands:(gate.commands||[]).map(c=>String(c||'').trim()).filter(Boolean),required:true,onFailure:['loop','reject','fail','human'].includes(gate.onFailure)?gate.onFailure:'loop',executionScope:gate.executionScope==='repositories'?'repositories':'run'})).filter(g=>g.commands.length),hooks=(draft.hooks||[]).map((h,i)=>({id:String(h.id||`hook-${i+1}`).trim(),name:String(h.name||`Hook ${i+1}`).trim(),event:h.event||'PreToolUse',matcher:String(h.matcher||'').trim(),language:h.language==='python'?'python':'shell',script:String(h.script||'').trim(),timeoutSec:Number(h.timeoutSec)||10,executionScope:h.executionScope==='repositories'?'repositories':'run'})).filter(h=>h.script),interventions=[];if(f.elements.interventionAfter?.checked)interventions.push({id:'human-review',name:'人工结果复核',stage:'after_execution',required:true});if(f.elements.interventionPublish?.checked)interventions.push({id:'publish-approval',name:'产物发布确认',stage:'before_publish',required:true});const a={id:f.elements.id.value.trim(),name:f.elements.name.value.trim(),version:(existing?.version||0)+1,projectId:existing?(existing.projectId||state.projectFilter||''):(state.projectFilter||''),shared:!!f.elements.shared?.checked,description:f.elements.description.value,instructions:f.elements.instructions.value,taskPrompt:draft.taskPrompt,executorId:f.elements.executorId?.value||'',model:agentEffectiveModel(f),effort:f.elements.effort?.value||'',rules:checked('rules'),skills:checked('skills'),mcpServers:checked('mcpServers'),platformMcp:!!f.elements.platformMcp?.checked,hooks,loop:{enabled:!!f.elements.loopEnabled?.checked,maxIterations:Number(f.elements.loopMaxIterations?.value)||1,exitCondition:'all_gates_pass',onExhausted:f.elements.loopOnExhausted?.value||'fail',feedbackPrompt:f.elements.loopFeedbackPrompt?.value||''},interventions,artifactOutputs:artifacts,verification};if(!a.id||!a.name)return toast('标识和名称不能为空');if(catalogGeneric(state.workflow)){if(!isPlatformAdmin())return toast('只有平台管理员可以编辑通用模板角色');const bundled=workflowAgent(a.id);if(!bundled)return toast('该角色不属于当前通用模板');a.projectId='';a.shared=false;a.rules=bundled.rules||[];a.skills=bundled.skills||[];a.mcpServers=f.querySelectorAll('input[name="mcpServers"]').length?checked('mcpServers'):(bundled.mcpServers||[]);a.hooks=Array.isArray(bundled.hooks)?bundled.hooks:a.hooks;a.version=bundled.version||0;try{const saved=await api(`/api/workflows/${encodeURIComponent(state.workflow.id)}/agents`,{method:'PUT',body:JSON.stringify(a)});const list=state.genericTemplateAgents||[];const gi=list.findIndex(x=>x.id===saved.id);if(gi>=0)list[gi]=saved;else list.push(saved);state.genericTemplateAgents=list;state.selectedAgent=saved.id;state.agentDraft=null;$('#agentModal').classList.remove('open');renderCanvas();renderProperties();($('#agentForm')?.classList.remove('is-dirty'),toast('通用模板角色已保存，所有项目下次运行生效'));}catch(err){toast(err.message)}return;}try{await api('/api/agents',{method:'PUT',body:JSON.stringify(a)});const i=state.data.agents.findIndex(x=>x.id===a.id);if(i>=0)state.data.agents[i]=a;else state.data.agents.push(a);if(a.shared){(state.data.rules||[]).forEach(item=>{if(a.rules.includes(item.id)&&item.projectId===a.projectId)item.shared=true});(state.data.skills||[]).forEach(item=>{if(a.skills.includes(item.id)&&item.projectId===a.projectId)item.shared=true})}state.selectedAgent=a.id;state.agentDraft=null;$('#agentModal').classList.remove('open');renderAgents();renderCanvas();($('#agentForm')?.classList.remove('is-dirty'),toast('Agent 新版本已保存'))  }catch(err){toast(err.message)}}
async function deleteAgent(id){
  const a=(state.data.agents||[]).find(x=>x.id===id);
  if(!a)return toast('角色不存在或已删除');
  if(!await afConfirm({title:`删除 Bot「${a.name}」？`,message:'此操作不可撤销。引用它的流程步骤会失去绑定。',confirmText:'删除 Bot',danger:true,confirmName:a.name}))return;
  try{
    await api(`/api/agents/${encodeURIComponent(id)}`,{method:'DELETE'});
    state.data.agents=state.data.agents.filter(x=>x.id!==id);
    if(state.selectedAgent===id)state.selectedAgent=null;
    if(state.agentDraft&&state.agentDraft.id===id)state.agentDraft=null;
    renderAgents();renderCanvas();
    toast('角色已删除')
  }catch(e){toast(e.message||'删除失败')}
}
async function cloneAgent(id){
  const src=id?(state.data.agents||[]).find(a=>a.id===id):state.data.agents.find(a=>a.id===state.selectedAgent);
  if(!src?.id)return toast('先选择一个角色');
  const name=await afPrompt({title:'复制角色',message:'请输入复制后的角色名称：',defaultValue:`${src.name} - 副本`});
  if(!name)return;
  try{
    const copied=await api(`/api/agents/${encodeURIComponent(src.id)}/clone`,{method:'POST',body:JSON.stringify({id:uid('agent'),name,projectId:state.projectFilter||''})});
    const [mcps,rules,skills]=await Promise.all([api('/api/mcp-servers',{silent:true}),api('/api/rules',{silent:true}),api('/api/skills',{silent:true})]);state.data.mcpServers=mcps;state.data.rules=rules;state.data.skills=skills;
    const i=state.data.agents.findIndex(a=>a.id===copied.id);
    if(i>=0)state.data.agents[i]=copied;else state.data.agents.unshift(copied);
    state.agentScope='project';
    renderAgents();
    openAgentWizard(copied);
    toast('已复制到本项目，可继续改');
  }catch(e){toast(e.message)}
}

