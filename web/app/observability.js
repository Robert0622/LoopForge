// ================= 可观测与分析 =================
let refreshObsPending=null;
function showDirectRunButton(onFlow){
  const btn=$('#runButton');
  if(!btn)return;
  btn.style.display=onFlow&&isPreviewHost()?'':'none';
}
function isPreviewHost(){
  const host=String((typeof location!=='undefined'&&location.hostname)||'').toLowerCase();
  const domain=String((typeof state!=='undefined'&&state.appDomain)||(typeof window!=='undefined'&&window.__LOOPFORGE_APP_DOMAIN__)||'').toLowerCase();
  return host.includes('-test.')||domain.includes('-test.')||isLoopbackHost(host);
}
function obsNormalizeTab(tab){
  return tab==='delivery'?'delivery':'overview';
}
function obsScope(){return state.obsScope==='project'?'project':'user'}
function obsTrendGrain(){return ['day','week','month','all'].includes(state.obsTrendGrain)?state.obsTrendGrain:'all'}
function obsApiGrain(){return obsTrendGrain()==='all'?'month':obsTrendGrain()}
function obsTrendMetric(){return ['runs','issues','cost'].includes(state.obsTrendMetric)?state.obsTrendMetric:'tokens'}
function obsTrendWindow(points){
  const grain=obsTrendGrain();
  if(grain==='all'||grain==='month')return points||[];
  return (points||[]).slice(grain==='week'?-12:-30);
}
function obsWeekMonday(key){
  const m=String(key||'').match(/^(\d{4})-W(\d{2})$/);
  if(!m)return null;
  const year=Number(m[1]),week=Number(m[2]);
  const jan4=new Date(Date.UTC(year,0,4));
  const day=jan4.getUTCDay()||7;
  const monday=new Date(jan4);
  monday.setUTCDate(jan4.getUTCDate()-(day-1)+(week-1)*7);
  return monday;
}
function obsDateToWeekKey(at){
  const d=new Date(Date.UTC(at.getUTCFullYear(),at.getUTCMonth(),at.getUTCDate()));
  const day=d.getUTCDay()||7;
  d.setUTCDate(d.getUTCDate()+4-day);
  const year=d.getUTCFullYear();
  const jan4=new Date(Date.UTC(year,0,4));
  const week=1+Math.round((d-jan4)/86400000/7);
  return `${year}-W${String(week).padStart(2,'0')}`;
}
function obsWeekLabel(key){
  const monday=obsWeekMonday(key);
  if(!monday)return String(key||'');
  const sunday=new Date(monday);
  sunday.setUTCDate(monday.getUTCDate()+6);
  const fmt=x=>`${x.getUTCMonth()+1}/${x.getUTCDate()}`;
  return `${fmt(monday)}–${fmt(sunday)}`;
}
function obsDefaultPeriodKey(grain){
  grain=grain||obsTrendGrain();
  const now=new Date();
  if(grain==='month')return `${now.getUTCFullYear()}-${String(now.getUTCMonth()+1).padStart(2,'0')}`;
  if(grain==='week')return obsDateToWeekKey(now);
  return now.toISOString().slice(0,10);
}
function obsPeriodKey(){
  if(obsTrendGrain()==='all')return '';
  const key=String(state.obsPeriodKey||'').trim();
  if(key)return key;
  const trend=obsTrendWindow(state.analytics?.trend||[]);
  return trend.length?trend[trend.length-1].date:obsDefaultPeriodKey();
}
function obsPeriodRange(key,grain){
  grain=grain||obsTrendGrain();
  key=String(key||'').trim();
  if(!key||grain==='all')return null;
  if(grain==='month'&&/^\d{4}-\d{2}$/.test(key)){
    const y=Number(key.slice(0,4)),m=Number(key.slice(5,7));
    return {from:new Date(Date.UTC(y,m-1,1)),to:new Date(Date.UTC(y,m,1)),label:`${y}年${m}月`,shortLabel:`${m}月`};
  }
  if(grain==='week'){
    const monday=obsWeekMonday(key);
    if(!monday)return null;
    const to=new Date(monday);
    to.setUTCDate(monday.getUTCDate()+7);
    return {from:monday,to,label:obsWeekLabel(key),shortLabel:obsWeekLabel(key)};
  }
  if(/^\d{4}-\d{2}-\d{2}$/.test(key)){
    const from=new Date(key+'T00:00:00Z');
    const to=new Date(from);
    to.setUTCDate(from.getUTCDate()+1);
    return {from,to,label:key,shortLabel:`${Number(key.slice(5,7))}/${Number(key.slice(8,10))}`};
  }
  return null;
}
function obsStepPeriodKey(key,grain,delta){
  grain=grain||obsTrendGrain();
  key=String(key||obsDefaultPeriodKey(grain));
  if(grain==='month'){
    const y=Number(key.slice(0,4)),m=Number(key.slice(5,7));
    const next=new Date(Date.UTC(y,m-1+delta,1));
    return `${next.getUTCFullYear()}-${String(next.getUTCMonth()+1).padStart(2,'0')}`;
  }
  if(grain==='week'){
    const monday=obsWeekMonday(key)||new Date();
    monday.setUTCDate(monday.getUTCDate()+delta*7);
    return obsDateToWeekKey(monday);
  }
  const day=new Date((/^\d{4}-\d{2}-\d{2}$/.test(key)?key:obsDefaultPeriodKey('day'))+'T00:00:00Z');
  day.setUTCDate(day.getUTCDate()+delta);
  return day.toISOString().slice(0,10);
}
function obsTraceEventAt(t){
  const raw=t?.finishedAt||t?.createdAt;
  const at=raw?new Date(raw):null;
  return at&&!Number.isNaN(at.getTime())?at:null;
}
function obsTraceInRange(t,range){
  if(!range)return true;
  const at=obsTraceEventAt(t);
  return !!at&&at>=range.from&&at<range.to;
}
function obsPercentile(values,pct){
  const list=(values||[]).filter(n=>n>0).slice().sort((a,b)=>a-b);
  if(!list.length)return 0;
  return list[Math.min(list.length-1,Math.max(0,Math.ceil(list.length*pct/100)-1))];
}
function obsTrendLabel(p){
  if(p&&p.label)return p.label;
  const d=p&&p.date||'';
  if(/^\d{4}-W\d{2}$/.test(d))return obsWeekLabel(d);
  if(/^\d{4}-\d{2}$/.test(d))return `${Number(d.slice(5))}月`;
  if(d.length>=10)return d.slice(5).replace('-','/');
  return d;
}
function obsTrendValue(p,metric){
  if(metric==='runs')return Number(p?.runs||0);
  if(metric==='issues')return Number(p?.issues||0);
  if(metric==='cost')return Number(p?.costUsd||0);
  return Number(p?.tokens||0);
}
function obsFormatTrendValue(v,metric){
  if(metric==='tokens')return formatTokens(v);
  if(metric==='cost')return '$'+Number(v||0).toFixed(2);
  return String(Math.round(Number(v||0)));
}
function obsPeriodDelta(points,metric){
  const list=obsTrendWindow(points);
  if(list.length<2)return null;
  let idx=list.length-1;
  const selected=obsPeriodKey();
  if(selected){
    const found=list.findIndex(p=>p.date===selected);
    if(found>=0)idx=found;
  }
  if(idx<1)return null;
  const cur=list[idx],prev=list[idx-1];
  const current=obsTrendValue(cur,metric),previous=obsTrendValue(prev,metric);
  const delta=current-previous;
  const pct=previous?delta*100/previous:(current?100:0);
  return {current,previous,delta,pct,currentLabel:obsTrendLabel(cur),previousLabel:obsTrendLabel(prev)};
}
function obsDeltaChip(delta,invert){
  if(!delta)return '';
  const down=delta.delta<0,up=delta.delta>0;
  const tone=down?(invert?'good':'bad'):up?(invert?'bad':'good'):'flat';
  const arrow=down?'↓':up?'↑':'–';
  const pct=Math.abs(delta.pct);
  return `<em class="obs-delta ${tone}">${arrow} ${pct>=10?pct.toFixed(0):pct.toFixed(1)}% <span>较${escapeHTML(delta.previousLabel)}</span></em>`;
}
function obsGrainCaption(){return (obsTrendGrain()==='all'||obsTrendGrain()==='month')?'月':obsTrendGrain()==='week'?'周':'日'}
function obsRangeCaption(){
  if(obsTrendGrain()==='all')return '累计';
  const range=obsPeriodRange(obsPeriodKey(),obsTrendGrain());
  return range?range.shortLabel:('本'+obsGrainCaption());
}
function setObsTrendGrain(grain){
  grain=['day','week','month','all'].includes(grain)?grain:'all';
  if(obsTrendGrain()===grain)return;
  state.obsTrendGrain=grain;
  state.obsPeriodKey='';
  localStorage.setItem('loopforge.obsTrendGrain',grain);
  applyObsFilter();
}
function setObsPeriod(key,grain){
  key=String(key||'').trim();
  if(grain&&['day','week','month'].includes(grain)&&grain!==obsTrendGrain()){
    state.obsTrendGrain=grain;
    localStorage.setItem('loopforge.obsTrendGrain',grain);
    state.obsPeriodKey=key;
    applyObsFilter();
    return;
  }
  if(obsPeriodKey()===key)return;
  state.obsPeriodKey=key;
  renderObservability();
}
function obsShiftPeriod(step){
  const grain=obsTrendGrain();
  if(grain==='all')return;
  const next=obsStepPeriodKey(obsPeriodKey()||obsDefaultPeriodKey(grain),grain,step);
  if(step>0&&next>obsDefaultPeriodKey(grain))return;
  setObsPeriod(next);
}
function obsPeriodChoices(){
  const keys=new Set((state.analytics?.trend||[]).map(t=>t.date).filter(Boolean));
  keys.add(obsPeriodKey()||obsDefaultPeriodKey());
  keys.add(obsDefaultPeriodKey());
  return [...keys].sort();
}
function obsPeriodPicker(){
  const grain=obsTrendGrain();
  if(grain==='all')return '';
  const key=obsPeriodKey()||obsDefaultPeriodKey(grain);
  let control='';
  if(grain==='day')control=`<input type="date" id="obsPeriodInput" class="select obs-period-input" value="${escapeHTML(key)}">`;
  else if(grain==='month')control=`<input type="month" id="obsPeriodInput" class="select obs-period-input" value="${escapeHTML(key)}">`;
  else control=`<select id="obsPeriodInput" class="select obs-period-input">${obsPeriodChoices().map(w=>`<option value="${escapeHTML(w)}" ${w===key?'selected':''}>${escapeHTML(obsWeekLabel(w))}</option>`).join('')}</select>`;
  return `<div class="obs-period-picker" aria-label="选择时段"><button type="button" data-obs-period-step="-1" title="上一段">‹</button>${control}<button type="button" data-obs-period-step="1" title="下一段">›</button></div>`;
}
function setObsTrendMetric(metric){
  metric=['runs','issues','cost'].includes(metric)?metric:'tokens';
  if(obsTrendMetric()===metric)return;
  state.obsTrendMetric=metric;
  localStorage.setItem('loopforge.obsTrendMetric',metric);
  renderObservability();
}
function obsTrendChart(points,metric){
  const list=obsTrendWindow(points);
  if(!list.length)return obsEmpty('还没有趋势','至少完成一次运行后，这里按所选时段汇总。','chat','从首页发一条需求');
  const w=760,h=210,l=52,r=18,t=16,b=36;
  const values=list.map(p=>obsTrendValue(p,metric));
  const max=Math.max(1,...values);
  const span=Math.max(1,list.length-1);
  const xs=list.map((_,i)=>list.length===1?l+(w-l-r)/2:l+i*(w-l-r)/span);
  const ys=values.map(v=>t+(h-t-b)*(1-v/max));
  const line=xs.map((x,i)=>(i?'L':'M')+x.toFixed(1)+','+ys[i].toFixed(1)).join(' ');
  const area=`${line} L${xs[xs.length-1].toFixed(1)},${h-b} L${xs[0].toFixed(1)},${h-b} Z`;
  const step=Math.max(1,Math.ceil(list.length/7));
  const labels=list.map((p,i)=>{
    if(i!==0&&i!==list.length-1&&i%step)return '';
    return `<text x="${xs[i].toFixed(1)}" y="${h-12}" text-anchor="middle">${escapeHTML(obsTrendLabel(p))}</text>`;
  }).join('');
  const yTicks=[0,0.5,1].map(p=>{
    const y=t+(h-t-b)*(1-p),val=max*p;
    return `<line x1="${l}" y1="${y.toFixed(1)}" x2="${w-r}" y2="${y.toFixed(1)}"/><text class="obs-chart-y" x="${l-8}" y="${(y+4).toFixed(1)}" text-anchor="end">${escapeHTML(obsFormatTrendValue(val,metric))}</text>`;
  }).join('');
  const selected=obsPeriodKey();
  const dots=xs.map((x,i)=>{
    const active=selected&&list[i].date===selected;
    return `<circle class="${active?'obs-chart-dot-active':''}" data-obs-period="${escapeHTML(list[i].date||'')}" cx="${x.toFixed(1)}" cy="${ys[i].toFixed(1)}" r="${active?5:list.length>16?2.4:3.4}"><title>${escapeHTML(obsTrendLabel(list[i]))}：${obsFormatTrendValue(values[i],metric)}（点击查看该时段）</title></circle>`;
  }).join('');
  return `<svg class="obs-trend-svg" viewBox="0 0 ${w} ${h}" role="img" aria-label="用量趋势">${yTicks}<path class="obs-chart-area" d="${area}"></path><path class="obs-chart-line" d="${line}"></path>${dots}${labels}</svg>`;
}
function obsScopedView(){
  const summary=state.analytics?.summary||{};
  const demo=state.analytics?.demo||{};
  const all=state.analytics?.runTraces||[];
  if(obsTrendGrain()==='all'){
    return {
      period:false,traces:all,total:summary.totalRuns||0,succeeded:summary.succeededRuns||0,failed:summary.failedRuns||0,cancelled:summary.cancelledRuns||0,
      reworked:summary.reworkedRuns||0,uniqueIssues:summary.uniqueIssues||0,tokens:summary.totalTokens||0,inputTokens:summary.inputTokens||0,outputTokens:summary.outputTokens||0,
      cost:summary.totalCostUsd||0,costKnown:summary.costUnknownRuns===0,active:summary.activeRuns||0,p95:summary.executionP95DurationMs||0,
      avgExec:summary.executionAverageDurationMs||0,wait:summary.waitAverageDurationMs||0,runsPerIssue:summary.runsPerIssue,firstPass:summary.qualityRuns?summary.firstPassRate:null,
      successRate:summary.successRate,demo:demo||{}
    };
  }
  const range=obsPeriodRange(obsPeriodKey(),obsTrendGrain());
  const traces=all.filter(t=>obsTraceInRange(t,range));
  let succeeded=0,failed=0,cancelled=0,tokens=0,cost=0,human=0,toolOk=0,toolBad=0,auto=0;
  const issues=new Set(),durations=[];
  let costKnown=true,active=0;
  for(const t of traces){
    if(t.status==='succeeded')succeeded++;
    else if(t.status==='failed')failed++;
    else if(t.status==='cancelled')cancelled++;
    if(t.status==='running'||t.status==='waiting')active++;
    tokens+=Number(t.tokens||0);
    cost+=Number(t.costUsd||0);
    if(t.costKnown===false)costKnown=false;
    human+=Number(t.humanInterventions||0);
    toolOk+=Number(t.toolCallsSucceeded||0);
    toolBad+=Number(t.toolCallsFailed||0);
    if(t.origin!=='external'&&t.status==='succeeded'&&!Number(t.humanInterventions||0))auto++;
    if(t.issueId)issues.add(t.issueId);
    if(t.origin==='external'){if(t.executionDurationMs!=null)durations.push(Number(t.executionDurationMs))}
    else if(t.durationMs)durations.push(Number(t.durationMs));
  }
  const covered=traces.filter(t=>t.origin!=='external'),coveredSucceeded=covered.filter(t=>t.status==='succeeded').length;
  const total=traces.length,attempts=succeeded+failed,observed=toolOk+toolBad;
  return {
    period:true,traces,total,succeeded,failed,cancelled,reworked:null,uniqueIssues:issues.size,tokens,inputTokens:null,outputTokens:null,
    cost,costKnown,active,p95:obsPercentile(durations,95),avgExec:durations.length?Math.round(durations.reduce((a,b)=>a+b,0)/durations.length):0,
    wait:0,runsPerIssue:issues.size?traces.filter(t=>t.issueId).length/issues.size:null,firstPass:null,successRate:attempts?succeeded*100/attempts:0,
    demo:{
      coveredRuns:covered.length,coveredSucceededRuns:coveredSucceeded,averageTokensPerRun:total?tokens/total:0,toolCallsObserved:observed,toolCallsSucceeded:toolOk,toolCallsFailed:toolBad,
      toolSuccessRate:observed?toolOk*100/observed:0,humanInterventions:human,averageHumanInterventions:covered.length?human/covered.length:0,
      automatedSucceededRuns:auto,automationRate:coveredSucceeded?auto*100/coveredSucceeded:0
    }
  };
}
function ensureObsUserFilter(){
  if(state.obsUserFilterReady)return;
  state.obsUserFilterReady=true;
  if(state.obsScope==='project')return;
  const user=state.currentUser;
  if(user&&!user.isGuest&&user.id)state.obsUserFilter=String(user.id);
}
function setObsScope(scope){
  scope=scope==='project'?'project':'user';
  if(obsScope()===scope)return;
  if(scope==='project')state.obsUserSaved=state.obsUserFilter||'';
  state.obsScope=scope;
  localStorage.setItem('loopforge.obsScope',scope);
  if(scope==='user'&&!state.obsUserFilter){
    state.obsUserFilter=state.obsUserSaved||(state.currentUser&&!state.currentUser.isGuest&&state.currentUser.id?String(state.currentUser.id):'');
  }
  resetListPages('obs-');
  applyObsFilter();
}
async function switchObsTab(tab){
  tab=obsNormalizeTab(tab);
  state.obsTab=tab;
  $$('#obsTabs .obs-tab').forEach(x=>x.classList.toggle('active',x.dataset.obsTab===tab));
  if(tab==='delivery'&&!state.obsAnalyticsFull){
    const host=$('#obsBody');
    if(host)host.innerHTML=`<div class="obs-pane active"><div class="detail-empty">正在汇总交付数据…</div></div>`;
    try{
      state.analytics=await api('/api/analytics'+analyticsQuery({overview:false}),{silent:true});
      state.obsAnalyticsFull=true;
      state.obsError='';
    }catch(e){state.obsError=e.message||'交付数据没有加载出来';toast(state.obsError,'err')}
  }
  renderObservability();
}
async function refreshObs(){
  if(refreshObsPending)return refreshObsPending;
  refreshObsPending=(async()=>{
    ensureObsUserFilter();
    const host=$('#obsBody');
    if(host)host.innerHTML=`<div class="obs-pane active"><div class="detail-empty">正在汇总这段时间的用量…</div></div>`;
    state.obsAnalyticsFull=false;
    try{
      const opt={silent:true};
      const overview=state.obsTab!=='delivery';
      const analytics=await api('/api/analytics'+analyticsQuery({overview}),opt);
      state.analytics=analytics;
      state.obsError='';
      if(!overview)state.obsAnalyticsFull=true;
    }catch(e){state.obsError=e.message||'用量没有加载出来';toast(state.obsError,'err')}
    finally{refreshObsPending=null}
    renderObservability();
    refreshObsCrossProject().then(()=>{if(activeViewName()==='observability')renderObservability()});
  })();
  return refreshObsPending;
}
function runCost(r){return Number(r.usage?.costUsd||0)}
function runTokens(r){return Number(r.usage?.totalTokens||0)}
function runArtifactCount(r){return Number(r?.artifactCount??r?.artifacts?.length??0)}
function obsKpiRow(items){return `<div class="obs-kpi-grid">${items.map(c=>`<div class="obs-metric" style="--accent:${c.color}"><span>${c.label}</span><b>${c.value}</b><small>${c.foot||''}</small></div>`).join('')}</div>`}
function analyticsQuery(opts){
  opts=opts||{};
  const parts=[];
  if(!opts.crossProject){
    const base=projectQuery().replace(/^\?/,'');
    if(base)parts.push(base);
  }
  if(opts.overview)parts.push('scope=overview');
  if(obsScope()==='user'&&state.obsUserFilter)parts.push('userId='+encodeURIComponent(state.obsUserFilter));
  if(state.obsOriginFilter)parts.push('origin='+encodeURIComponent(state.obsOriginFilter));
  if(state.obsSourceFilter)parts.push('issueSource='+encodeURIComponent(state.obsSourceFilter));
  if(['week','month'].includes(obsApiGrain()))parts.push('grain='+obsApiGrain());
  return parts.length?'?'+parts.join('&'):'';
}
async function refreshObsCrossProject(){
  if(obsScope()!=='user'||!state.obsUserFilter){state.obsCrossProject=null;return}
  try{
    state.obsCrossProject=await api('/api/analytics'+analyticsQuery({overview:true,crossProject:true}),{silent:true});
  }catch(e){state.obsCrossProject=null}
}
async function applyObsFilter(){
  try{
    const overview=state.obsTab!=='delivery';
    const [analytics]=await Promise.all([
      api('/api/analytics'+analyticsQuery({overview}),{silent:true}),
    ]);
    state.analytics=analytics;
    state.obsAnalyticsFull=!overview;
  }catch(e){toast(e.message)}
  renderObservability();
  refreshObsCrossProject().then(()=>{if(activeViewName()==='observability')renderObservability()});
}
function obsUserName(id){
  if(!id||id==='unassigned')return '未分配';
  const user=userById(id);
  return user?userDisplay(user):String(id);
}
function obsLinkedIssue(run){
  const task=(state.data.tasks||[]).find(t=>t.runId===run?.id);
  const issueId=task?.issueId||run?.executionContext?.issueId;
  return issueId&&(state.data.issues||[]).find(i=>i.id===issueId);
}
function obsRunMatchesUser(run,uid){
  if(!uid)return true;
  const want=String(uid);
  if(String(run?.executionContext?.responsibleUserId||'')===want)return true;
  return String(obsLinkedIssue(run)?.assigneeUserId||'')===want;
}
function obsUserChoices(){
  const me=Number(state.currentUser?.id||0);
  const project=currentProject();
  const memberIds=new Set((project?.members||[]).map(m=>Number(m.userId)));
  if(project?.ownerId)memberIds.add(Number(project.ownerId));
  return [...(state.data.users||[])].sort((a,b)=>{
    const aid=Number(a.id),bid=Number(b.id);
    if(aid===me)return -1;
    if(bid===me)return 1;
    const am=memberIds.has(aid),bm=memberIds.has(bid);
    if(am!==bm)return am?-1:1;
    return userDisplay(a).localeCompare(userDisplay(b),'zh');
  });
}
function obsFilterBar(){
  ensureObsUserFilter();
  const project=currentProject();
  const projectName=project?.name||project?.id||'当前项目';
  const scope=obsScope();
  const selected=scope==='user'?(state.obsUserFilter||''):'';
  const current=userById(selected);
  const label=current?userDisplay(current):(selected?obsUserName(selected):'全部成员');
  const allOpt=`<button type="button" class="user-combobox-option${!selected?' selected':''}" data-user-option="" data-search="全部成员 all"><span>全部成员</span>${!selected?'<i>✓</i>':''}</button>`;
  const options=allOpt+obsUserChoices().map(u=>`<button type="button" class="user-combobox-option${String(u.id)===String(selected)?' selected':''}" data-user-option="${escapeHTML(String(u.id))}" data-search="${escapeHTML(`${u.loginName||''} ${u.displayName||''}`.toLowerCase())}"><span>${escapeHTML(userDisplay(u))}</span>${String(u.id)===String(selected)?'<i>✓</i>':''}</button>`).join('');
  const sources=[['','全部来源'],['local','本地'],['github','GitHub']].map(([v,l])=>`<option value="${v}" ${(state.obsSourceFilter||'')===v?'selected':''}>${l}</option>`).join('');
  const grain=obsTrendGrain();
  const grainBtns=[['all','全部'],['day','按日'],['week','按周'],['month','按月']].map(([id,l])=>`<button type="button" data-obs-grain="${id}" class="${grain===id?'active':''}">${l}</button>`).join('');
  const hint=scope==='project'
    ?`项目指标 = 顶栏当前项目「${projectName}」全部成员的运行、Token 和交付，不是全平台汇总。${grain==='all'?'当前看全部历史，趋势按月聚合，点一个月可下钻。':'上方数字和交付指标只含所选时段；点趋势图或用两侧箭头换一天 / 一周 / 一月。'}`
    :`个人指标 = 责任人在当前项目「${projectName}」里的执行。一人多项目时，下方「各项目」看他在其他项目的量，不会加进上面的 KPI。${grain==='all'?'当前看全部历史，趋势按月聚合，点一个月可下钻。':'上方数字和交付指标只含所选时段；点趋势图或用两侧箭头换一天 / 一周 / 一月。'}`;
  const userFilter=scope==='user'?`<label class="field obs-filter-user"><span>责任人</span><div class="user-combobox" data-user-combobox id="obsUserCombobox"><input type="hidden" name="obsUserFilter" value="${escapeHTML(selected)}"><button type="button" class="user-combobox-trigger" data-user-combobox-trigger aria-haspopup="listbox" aria-expanded="false"><span>${escapeHTML(label)}</span><i>⌄</i></button><div class="user-combobox-panel" data-user-combobox-panel hidden><input type="search" data-user-combobox-search placeholder="搜索显示名 / 登录名" autocomplete="off"><div class="user-combobox-options" role="listbox">${options}</div><p class="user-combobox-empty" hidden>没有匹配的用户</p></div></div></label>`:'';
  return `<div class="obs-toolbar"><div class="obs-toolbar-filters"><div class="mcp-scope-switch obs-scope-switch" aria-label="用量维度"><button type="button" data-obs-scope="user" class="${scope==='user'?'active':''}">个人</button><button type="button" data-obs-scope="project" class="${scope==='project'?'active':''}">项目</button></div>${userFilter}<label class="field obs-filter-source"><span>执行来源</span><select id="obsOriginFilter" class="select">${[['','全部执行'],['internal','平台内'],['external','外部工作流']].map(([v,l])=>`<option value="${v}" ${(state.obsOriginFilter||'')===v?'selected':''}>${l}</option>`).join('')}</select></label><label class="field obs-filter-source"><span>需求来源</span><select id="obsSourceFilter" class="select">${sources}</select></label><div class="obs-grain-switch" aria-label="趋势粒度">${grainBtns}</div>${obsPeriodPicker()}${obsCanManageSources()?'<button class="button ghost" id="obsManageSources" type="button">外部数据源</button>':''}</div><p class="obs-toolbar-note">${escapeHTML(hint)}</p></div>`;
}
function obsFilteredRuns(){
  let runs=scopedRuns(state.data.runs||[]);
  if(obsScope()==='user'&&state.obsUserFilter)runs=runs.filter(r=>obsRunMatchesUser(r,state.obsUserFilter));
  if(state.obsSourceFilter)runs=runs.filter(r=>String(obsLinkedIssue(r)?.source||'local').toLowerCase()===state.obsSourceFilter);
  return runs;
}
function obsProjectName(id){
  if(!id||id==='unassigned')return '未归属项目';
  const project=(state.data.projects||[]).find(p=>p.id===id);
  return project?.name||project?.key||id;
}
function obsCrossProjectCard(){
  if(obsScope()!=='user'||!state.obsUserFilter)return '';
  const rows=state.obsCrossProject?.projectUsage||[];
  if(!rows.length)return '';
  const current=state.projectFilter;
  const other=rows.filter(r=>r.projectId&&r.projectId!==current);
  const totalTokens=rows.reduce((n,r)=>n+(r.tokens||0),0);
  const here=rows.find(r=>r.projectId===current);
  const body=rows.map(r=>`<tr class="${r.projectId===current?'obs-current-project':''}"><td><b>${escapeHTML(obsProjectName(r.projectId))}</b>${r.projectId===current?'<span class="obs-pill">当前</span>':''}</td><td>${r.runs||0}</td><td>${r.uniqueIssues||0}</td><td>${formatTokens(r.tokens)}</td><td>${costLabel(r.costUsd,r.costKnown)}</td></tr>`).join('');
  const note=other.length
    ?`该责任人在 ${rows.length} 个项目有执行。上面 KPI 只含「${escapeHTML(obsProjectName(current))}」（${formatTokens(here?.tokens||0)}），跨项目合计 ${formatTokens(totalTokens)}。`
    :`该责任人目前只在当前项目有执行记录，个人与项目切换不会把其他项目加进来。`;
  return `<div class="obs-card obs-project-mix"><div class="obs-card-title">该责任人在各项目<span class="sub">${note}</span></div><table class="obs-table"><thead><tr><th>项目</th><th>运行</th><th>需求</th><th>Tokens</th><th>成本</th></tr></thead><tbody>${body}</tbody></table></div>`;
}
function obsUserTotals(){
  const bag={};
  for(const u of state.analytics?.userUsage||[]){
    const id=u.userId||'unassigned';
    const row=bag[id]||(bag[id]={userId:id,runs:0,success:0,failed:0,tokens:0,inputTokens:0,outputTokens:0,costUsd:0,costKnown:true,executionDurationMs:0});
    row.runs+=u.runs||0;
    row.success+=u.success||0;
    row.failed+=u.failed||0;
    row.tokens+=u.tokens||0;
    row.inputTokens+=u.inputTokens||0;
    row.outputTokens+=u.outputTokens||0;
    row.costUsd+=u.costUsd||0;
    if(u.costKnown===false)row.costKnown=false;
    row.executionDurationMs+=u.executionDurationMs||0;
  }
  return Object.values(bag).sort((a,b)=>(b.tokens-a.tokens)||(b.runs-a.runs));
}
function obsEmpty(title,hint,view,label){
  return `<div class="lf-callout"><b>${title}</b><p>${hint}</p><button type="button" class="button primary small" data-jump="${view}">${label}</button></div>`;
}
function obsTableCard(title,sub,headers,rows,colspan,pageKey){
  const list=Array.isArray(rows)?rows:(rows?[rows]:[]);
  const paged=pageKey?paginateList(pageKey,list):{items:list,total:list.length,size:list.length||1,page:1,pages:1,start:0,end:list.length};
  const body=paged.items.join('')||`<tr><td colspan="${colspan||headers.length}" class="detail-empty">${obsEmpty('这段时间还没有数据','跑一条 SOP 之后，成本和成功率会出现在这里。','issues','去 SOP 运行')}</td></tr>`;
  return `<div class="obs-card" style="margin-bottom:14px"><div class="obs-card-title">${title}${sub?`<span class="sub">${sub}</span>`:''}</div><table class="obs-table"><thead><tr>${headers.map(h=>`<th>${h}</th>`).join('')}</tr></thead><tbody>${body}</tbody></table>${pageKey?pagerHTML(pageKey,paged):''}</div>`;
}
// costLabel 区分"成本为 0"与"成本未识别"，未识别时展示占位而不是 ¥0。
function costLabel(value,known){
  const amount=Number(value||0);
  if(known===false&&amount<=0)return '<span class="muted">未识别</span>';
  return '$'+amount.toFixed(2)+(known===false?'（部分未识别）':'');
}
function renderObservability(){
  ensureObsUserFilter();
  const host=$('#obsBody');if(!host)return;
  if(state.obsError&&!state.analytics){
    host.innerHTML=`<div class="obs-pane active"><div class="lf-callout"><b>用量没有加载出来</b><p>${escapeHTML(state.obsError)}</p><button type="button" class="button primary" id="obsRetry">重试</button></div></div>`;
    host.querySelector('#obsRetry').onclick=()=>refreshObs();
    return;
  }
  state.obsTab=obsNormalizeTab(state.obsTab);
  $$('#obsTabs .obs-tab').forEach(b=>b.classList.toggle('active',b.dataset.obsTab===state.obsTab));
  const summary=state.analytics?.summary||{};
  $('#obsLiveState').classList.toggle('active',((summary.activeRuns||0)+(summary.waitingRuns||0))>0);
  const tab=state.obsTab;
  const html=tab==='delivery'?obsDelivery():obsOverview();
  const err=state.obsError?`<div class="lf-callout"><b>这次刷新没成功</b><p>${escapeHTML(state.obsError)}</p><button type="button" class="button primary small" id="obsRetry">重试</button></div>`:'';
  host.innerHTML=`<div class="obs-pane active">${err}${obsFilterBar()}${html}</div>`;
  host.querySelector('#obsRetry')?.addEventListener('click',()=>refreshObs());
  bindUserSearches(host);
  host.querySelectorAll('[data-obs-scope]').forEach(b=>b.onclick=()=>setObsScope(b.dataset.obsScope));
  host.querySelectorAll('[data-obs-grain]').forEach(b=>b.onclick=()=>setObsTrendGrain(b.dataset.obsGrain));
  host.querySelectorAll('[data-obs-metric]').forEach(b=>b.onclick=()=>setObsTrendMetric(b.dataset.obsMetric));
  host.querySelectorAll('[data-obs-period]').forEach(el=>el.onclick=e=>{e.stopPropagation();const key=el.dataset.obsPeriod;if(!key)return;if(obsTrendGrain()==='all')setObsPeriod(key,'month');else setObsPeriod(key)});
  host.querySelectorAll('[data-obs-period-step]').forEach(b=>b.onclick=()=>obsShiftPeriod(Number(b.dataset.obsPeriodStep)));
  const periodInput=host.querySelector('#obsPeriodInput');
  if(periodInput)periodInput.onchange=()=>setObsPeriod(periodInput.value);
  const obsUserInput=host.querySelector('#obsUserCombobox input[name="obsUserFilter"]');
  if(obsUserInput)obsUserInput.onchange=()=>{state.obsUserFilter=obsUserInput.value||'';resetListPages('obs-');applyObsFilter()};
  const originSelect=host.querySelector('#obsOriginFilter');if(originSelect)originSelect.onchange=()=>{state.obsOriginFilter=originSelect.value;resetListPages('obs-');applyObsFilter()};
  const sourceButton=host.querySelector('#obsManageSources');if(sourceButton)sourceButton.onclick=openObsSources;
  const obsSourceSel=host.querySelector('#obsSourceFilter');if(obsSourceSel)obsSourceSel.onchange=()=>{state.obsSourceFilter=obsSourceSel.value;resetListPages('obs-');applyObsFilter()};
  host.querySelectorAll('[data-open-run]').forEach(b=>b.onclick=e=>{if(e.target.closest&&e.target.closest('a'))return;openRunFromId(b.dataset.openRun)});
  host.querySelectorAll('[data-jump]').forEach(b=>b.onclick=()=>switchView(b.dataset.jump));
  host.querySelectorAll('[data-edit-issue]').forEach(b=>b.onclick=e=>{if(e.target.closest&&e.target.closest('a'))return;const issue=(state.data.issues||[]).find(i=>i.id===b.dataset.editIssue);if(issue)openIssue(issue);else openIssueDetail(b.dataset.editIssue)});
  host.querySelectorAll('[data-obs-delivery]').forEach(b=>b.onclick=()=>{state.obsDeliveryFilter=b.dataset.obsDelivery;resetListPage('obs-deliveries');renderObservability()});
  bindPager(host);
}
function obsSourceLabel(s){return s==='github'?'GitHub':s==='local'?'本地':s||'—'}
function obsCategoryLabel(k){return ({requirement:'需求开发',bugfix:'Bug 修复',refactor:'技术优化',other:'其他'}[k])||k||'未分类'}
function obsOverview(){
  const view=obsScopedView();
  const summary=state.analytics?.summary||{};
  const demo=view.demo||{};
  const traces=view.traces||[];
  const succeeded=view.succeeded||0;
  const failed=view.failed||0;
  const reworked=view.reworked;
  const cancelled=view.cancelled||0;
  const total=view.total||0;
  const totalTokens=view.tokens||0;
  const totalCost=view.cost||0;
  const stats=(state.analytics?.workflows||[]).map(w=>({...w,name:w.name||w.key}));
  const wfRows=stats.map(w=>`<tr><td><b>${escapeHTML(w.name)}</b></td><td>${w.runs}</td><td>${percent(w.successRate)}</td><td>${w.qualityUnknownRuns===w.runs?'—':percent(w.reworkRate)}</td><td>${percent(w.cancelRate)}</td><td>${formatTokens(w.tokens)}</td><td>${formatTokens(w.inputTokens)} / ${formatTokens(w.outputTokens)}</td><td>${w.tokenSharePct!=null?w.tokenSharePct.toFixed(1)+'%':'—'}</td><td>${costLabel(w.costUsd,!w.costUnknownRuns)}</td><td>${w.costSharePct!=null?w.costSharePct.toFixed(1)+'%':'—'}</td><td>${w.averageExecutionDurationMs?duration(w.averageExecutionDurationMs):'—'}</td></tr>`).join('');
  const modelRows=(state.analytics?.models||[]).map(m=>`<tr><td class="mono">${escapeHTML(m.name||m.key)}</td><td>${m.runs}</td><td>${formatTokens(m.tokens)}</td><td>${formatTokens(m.inputTokens)} / ${formatTokens(m.outputTokens)}</td><td>${m.tokenSharePct!=null?m.tokenSharePct.toFixed(1)+'%':'—'}</td><td>${costLabel(m.costUsd,!m.costUnknownRuns)}</td><td>${m.costSharePct!=null?m.costSharePct.toFixed(1)+'%':'—'}</td><td>${m.qualityUnknownRuns===m.runs?'—':percent(m.successRate)}</td><td>${m.averageExecutionDurationMs?duration(m.averageExecutionDurationMs):'—'}</td></tr>`).join('');
  const userRows=(state.analytics?.userUsage||[]).map(u=>`<tr><td>${escapeHTML(u.date)}</td><td><b>${escapeHTML(obsUserName(u.userId))}</b></td><td>${u.runs}</td><td>${formatTokens(u.tokens)}</td><td>${formatTokens(u.inputTokens)} / ${formatTokens(u.outputTokens)}</td><td>${costLabel(u.costUsd,u.costKnown)}</td><td>${u.executionDurationMs?duration(u.executionDurationMs):'—'}</td></tr>`);
  const memberRows=obsUserTotals().map(u=>`<tr><td><b>${escapeHTML(obsUserName(u.userId))}</b></td><td>${u.runs}</td><td>${u.success||0}</td><td>${u.failed||0}</td><td>${formatTokens(u.tokens)}</td><td>${formatTokens(u.inputTokens)} / ${formatTokens(u.outputTokens)}</td><td>${costLabel(u.costUsd,u.costKnown)}</td><td>${u.executionDurationMs?duration(u.executionDurationMs):'—'}</td></tr>`).join('');
  const srcRows=(state.analytics?.issueSources||[]).map(s=>`<tr><td><b>${obsSourceLabel(s.key)}</b></td><td>${s.runs}</td><td>${s.succeeded||0}</td><td>${s.completedIssues||0}</td><td>${formatTokens(s.tokens)}</td><td>${costLabel(s.costUsd,true)}</td><td>${s.averageExecutionDurationMs?duration(s.averageExecutionDurationMs):'—'}</td></tr>`).join('');
  const catRows=(state.analytics?.categories||[]).map(c=>`<tr><td><b>${escapeHTML(obsCategoryLabel(c.key))}</b></td><td>${c.runs}</td><td>${c.succeeded||0}</td><td>${c.completedIssues||0}</td><td>${formatTokens(c.tokens)}</td><td>${costLabel(c.costUsd,true)}</td></tr>`).join('');
  const versionRows=(state.analytics?.workflowVersions||[]).map(v=>`<tr><td><b>${escapeHTML(v.name||v.workflowId)}</b></td><td class="mono">v${v.version}</td><td>${v.runs}</td><td>${percent(v.successRate)}</td><td>${v.averageExecutionDurationMs?duration(v.averageExecutionDurationMs):'—'}</td><td>${formatTokens(v.tokens)}</td><td>${costLabel(v.costUsd,true)}</td><td>${v.insufficientSample?'<span class="muted">样本不足</span>':'—'}</td></tr>`).join('');
  const profileRows=stats.filter(w=>w.nodeCount>0).map(w=>`<tr><td><b>${escapeHTML(w.name)}</b></td><td>${w.nodeCount}</td><td>${Object.entries(w.nodeTypes||{}).map(([k,v])=>`${escapeHTML(k)}×${v}`).join('，')||'—'}</td><td>${escapeHTML(obsCategoryLabel(w.category))}</td><td>${w.runs}</td></tr>`).join('');
  const nodeStats=state.analytics?.nodes||[];
  const failReasonRows=(state.analytics?.failures||[]).slice(0,12).map(f=>`<tr><td>${escapeHTML(f.nodeId||'—')}</td><td title="${escapeHTML(f.code||'')}">${escapeHTML((f.code||'').slice(0,80))}</td><td>${f.count}</td></tr>`).join('');
  const nodeQualityRows=[...nodeStats].sort((a,b)=>(b.failedNodes||0)-(a.failedNodes||0)).slice(0,16).map(n=>`<tr><td>${escapeHTML(n.name||n.key)}</td><td>${n.runs}</td><td>${n.succeeded||0}</td><td>${n.failedNodes||0}</td><td>${n.reworkNodes||0}</td><td>${n.retryRate!=null?percent(n.retryRate):'—'}</td><td>${n.averageExecutionDurationMs?duration(n.averageExecutionDurationMs):'—'}</td><td>${formatTokens(n.tokens)}</td><td>${n.toolCalls||0}</td></tr>`).join('');
  const slowRows=[...nodeStats].filter(n=>n.averageExecutionDurationMs>0).sort((a,b)=>b.averageExecutionDurationMs-a.averageExecutionDurationMs).slice(0,10).map(n=>`<tr><td>${escapeHTML(n.name||n.key)}</td><td>${n.runs}</td><td>${duration(n.averageExecutionDurationMs)}</td><td>${n.p95ExecutionDurationMs?duration(n.p95ExecutionDurationMs):'—'}</td><td>${percent(n.successRate)}</td><td>${formatTokens(n.tokens)}</td></tr>`).join('');
  const trend=state.analytics?.trend||[];
  const visibleTrend=obsTrendWindow(trend);
  const grainName=obsGrainCaption();
  const rangeName=obsRangeCaption();
  const metric=obsTrendMetric();
  const metricBtns=[['tokens','Token'],['runs','运行'],['issues','需求量'],['cost','成本']].map(([id,l])=>`<button type="button" data-obs-metric="${id}" class="${metric===id?'active':''}">${l}</button>`).join('');
  const tokenDelta=obsPeriodDelta(trend,'tokens'),runDelta=obsPeriodDelta(trend,'runs'),issueDelta=obsPeriodDelta(trend,'issues'),costDelta=obsPeriodDelta(trend,'cost');
  const maxTrend=Math.max(1,...visibleTrend.map(t=>t.tokens||0));
  const selectedPeriod=obsPeriodKey();
  const trendRows=visibleTrend.slice().reverse().map(t=>`<tr class="run-row${t.date===selectedPeriod?' obs-period-active':''}" data-obs-period="${escapeHTML(t.date||'')}"><td>${escapeHTML(obsTrendLabel(t))}</td><td>${t.runs}</td><td>${t.issues||0}</td><td>${t.succeeded}</td><td>${t.failed||0}</td><td>${formatTokens(t.tokens)}</td><td>${formatTokens(t.inputTokens)} / ${formatTokens(t.outputTokens)}</td><td>${costLabel(t.costUsd,!t.costUnknownRuns)}</td><td>${t.executionDurationMs?duration(t.executionDurationMs):'—'}</td><td><span class="obs-mini-bar"><i style="width:${Math.round((t.tokens||0)*100/maxTrend)}%"></i></span></td></tr>`).join('');
  const failRows=traces.filter(t=>t.status==='failed').slice(0,5).map(t=>{
    const label=[t.origin==='external'?'外部 · '+t.workflowName:obsSourceLabel(t.issueSource||'local'),t.externalId||t.issueId||t.runId].filter(Boolean).join(' ');
    return `<tr class="run-row" ${t.origin==='external'?'':`data-open-run="${escapeHTML(t.runId||'')}"`}><td>${escapeHTML(label)}</td><td><span class="status ${escapeHTML(t.status||'')}">${runStatus(t.status)}</span></td><td>${relativeTime(t.createdAt)}</td></tr>`;
  }).join('');
  const recent=traces.slice(0,8).map(t=>{
    const label=[t.origin==='external'?'外部 · '+t.workflowName:obsSourceLabel(t.issueSource||'local'),t.externalId||t.issueId||t.runId].filter(Boolean).join(' ');
    return `<tr class="run-row" ${t.origin==='external'?'':`data-open-run="${escapeHTML(t.runId||'')}"`}><td>${escapeHTML(label)}</td><td><span class="status ${escapeHTML(t.status||'')}">${runStatus(t.status)}</span></td><td>${t.durationMs?duration(t.durationMs):'—'}</td><td>${formatTokens(t.tokens)}</td><td>${costLabel(t.costUsd,t.costKnown)}</td></tr>`;
  }).join('');
  const traceRows=traces.map(t=>{
    const source=t.origin==='external'?'外部 · '+(t.workflowName||''):obsSourceLabel(t.issueSource||'local');
    const issueRef=t.externalId||t.issueId||'—';
    const issue=t.externalUrl?`<a href="${escapeHTML(safeURL(t.externalUrl))}" target="_blank" rel="noopener noreferrer">${escapeHTML(source+' '+issueRef)}</a>`:`${escapeHTML(source+' '+issueRef)}`;
    const task=t.workflowTaskId?`<code title="${escapeHTML(t.workflowTaskId)}">${escapeHTML(t.workflowTaskId.slice(-12))}</code>`:'—';
    const runId=String(t.runId||''),traceId=String(t.traceId||t.runId||'');
    const relation=t.origin==='external'?`<code title="${escapeHTML(t.externalId||'')}">外部执行 ${escapeHTML(t.externalId||'')}</code>`:`<div class="obs-trace-ids"><code title="${escapeHTML(runId)}">Run ${escapeHTML(runId.slice(-12))}</code><code title="${escapeHTML(traceId)}">Trace ${escapeHTML(traceId.slice(-12))}</code></div>`;
    const observed=(t.toolCallsSucceeded||0)+(t.toolCallsFailed||0);
    const tools=observed?`${t.toolCallsSucceeded||0}/${observed} 成功`:(t.toolCalls?`${t.toolCalls} 次 · 结果未上报`:'—');
    const error=t.errorMessage?`<span class="obs-trace-error" title="${escapeHTML(t.errorMessage)}">${escapeHTML(t.errorNodeId||'Run')} · ${escapeHTML(t.errorMessage)}</span>`:'—';
    return `<tr class="run-row" ${t.origin==='external'?'':`data-open-run="${escapeHTML(runId)}"`}><td>${issue}</td><td>${task}</td><td>${relation}</td><td><span class="status ${escapeHTML(t.status||'')}">${runStatus(t.status)||escapeHTML(t.status||'—')}</span></td><td>${t.durationMs?duration(t.durationMs):'—'}</td><td>${formatTokens(t.tokens)}</td><td>${costLabel(t.costUsd,t.costKnown)}</td><td>${tools}</td><td>${t.origin==='external'?'未上报':t.humanInterventions||0}</td><td>${error}</td></tr>`;
  });
  const uniqueIssues=view.uniqueIssues||0;
  const tokenFoot=view.inputTokens!=null?`入 ${formatTokens(view.inputTokens)} / 出 ${formatTokens(view.outputTokens)}`:(view.period?`该${obsGrainCaption()} ${formatTokens(totalTokens)}`:`累计 ${formatTokens(totalTokens)}`);
  const successFoot=`${succeeded} 成功 / ${failed} 失败${view.firstPass!=null?` · 一次通过 ${total?percent(view.firstPass):'—'}`:''}`;
  const costFoot=reworked==null?`该${obsGrainCaption()} ${costLabel(totalCost,view.costKnown)}`:`返工 ${reworked}${view.period?'':''}`;
  return `<div class="obs-hero">
    <article><span>${rangeName}运行</span><b>${total}</b><small>${view.active||0} 进行中${view.period?` · 累计 ${summary.totalRuns||0}`:''}</small>${obsTrendGrain()==='all'?'':obsDeltaChip(runDelta)}</article>
    <article><span>${rangeName}需求</span><b>${uniqueIssues}</b><small>每单 ${view.runsPerIssue!=null?Number(view.runsPerIssue).toFixed(1):'—'} 次${view.period?` · 累计 ${summary.uniqueIssues||0}`:''}</small>${obsTrendGrain()==='all'?'':obsDeltaChip(issueDelta)}</article>
    <article><span>${rangeName} Token</span><b>${formatTokens(totalTokens)}</b><small>${tokenFoot}${view.period?` · 累计 ${formatTokens(summary.totalTokens)}`:''}</small>${obsTrendGrain()==='all'?'':obsDeltaChip(tokenDelta,true)}</article>
    <article><span>成功率</span><b>${total?percent(view.successRate):'—'}</b><small>${successFoot}</small></article>
    <article><span>${rangeName}成本</span><b>${costLabel(totalCost,view.costKnown)}</b><small>${costFoot}${view.period?` · 累计 ${costLabel(summary.totalCostUsd,summary.costUnknownRuns===0)}`:''}</small>${obsTrendGrain()==='all'?'':obsDeltaChip(costDelta,true)}</article>
    <article><span>P95 执行</span><b>${view.p95?duration(view.p95):'—'}</b><small>平均 ${view.avgExec?duration(view.avgExec):'—'}${view.wait?` · 等待 ${duration(view.wait)}`:''}</small></article>
  </div>
  <div class="obs-card obs-demo-card">
    <div class="obs-card-title"><div>交付指标<small>${view.period?`仅统计 ${escapeHTML(rangeName)} 内的 Run，可点趋势换时段`:'所有数字均可下钻到 workflow_task_id、run_id、trace_id 和原始事件'}</small></div><span class="sub">自动完成率仅统计有平台事件的执行；外部汇总不推断人工干预</span></div>
    ${obsKpiRow([
      {label:'平均 Token / Run',value:total?formatTokens(demo.averageTokensPerRun):'—',foot:`${formatTokens(totalTokens)} tokens ÷ ${total} runs`,color:'#5b8cff'},
      {label:'工具调用成功率',value:demo.toolCallsObserved?percent(demo.toolSuccessRate):'—',foot:`${demo.toolCallsSucceeded||0} 成功 / ${demo.toolCallsFailed||0} 失败 · 仅统计完成事件`,color:'#34d399'},
      {label:'平均人工干预',value:demo.coveredRuns?Number(demo.averageHumanInterventions||0).toFixed(2):'—',foot:`${demo.humanInterventions||0} 次人工决策 ÷ ${demo.coveredRuns||0} 平台内 runs · 不含暂停/恢复`,color:'#fbbf24'},
      {label:'自动完成率',value:demo.coveredSucceededRuns?percent(demo.automationRate):'—',foot:`${demo.automatedSucceededRuns||0} 个全自动成功 / ${demo.coveredSucceededRuns||0} 个平台内成功`,color:'#8b5cf6'},
    ])}
  </div>
  ${obsTableCard('关联与追溯','Issue / 需求 → workflow_task_id → run_id / trace_id；点击一行查看事件明细',['Issue / 需求','workflow_task_id','Run / Trace','状态','耗时','Tokens','成本','工具','人工','错误位置'],traceRows,10,'obs-traces')}
  <div class="obs-card obs-trend-card">
    <div class="obs-card-title">
      <div>趋势变化<small>按${grainName}对比 Token、需求量和运行量；点击某${grainName}查看该时段指标</small></div>
      <div class="obs-metric-switch">${metricBtns}</div>
    </div>
    <div class="obs-trend-compare">
      <span>最近一${grainName}（含未结束周期）Token ${obsFormatTrendValue(tokenDelta?tokenDelta.current:obsTrendValue(visibleTrend[visibleTrend.length-1]||{},'tokens'),'tokens')}</span>
      ${obsDeltaChip(tokenDelta,true)||'<em class="obs-delta flat">样本不足，暂无环比</em>'}
      <span>需求 ${obsFormatTrendValue(issueDelta?issueDelta.current:obsTrendValue(visibleTrend[visibleTrend.length-1]||{},'issues'),'issues')}</span>
      ${obsDeltaChip(issueDelta)||''}
    </div>
    ${obsTrendChart(trend,metric)}
    <table class="obs-table obs-trend-table"><thead><tr><th>${grainName}</th><th>运行</th><th>需求</th><th>成功</th><th>失败</th><th>Tokens</th><th>输入 / 输出</th><th>成本</th><th>执行耗时</th><th></th></tr></thead><tbody>${trendRows||`<tr><td colspan="10" class="detail-empty">${obsEmpty('还没有趋势','完成一次运行后，这里会按天汇总。','chat','从首页发一条需求')}</td></tr>`}</tbody></table>
  </div>
  ${obsCrossProjectCard()}
  <div class="obs-split">
    <div class="obs-card"><div class="obs-card-title">质量信号<span class="sub">失败优先处理</span></div><div class="obs-signal-row"><div><b>${failed}</b><span>失败</span></div><div><b>${reworked}</b><span>返工</span></div><div><b>${cancelled}</b><span>取消</span></div></div><table class="obs-table"><thead><tr><th>最近失败</th><th>状态</th><th>时间</th></tr></thead><tbody>${failRows||'<tr><td colspan="3" class="detail-empty">这段时间没有失败运行。</td></tr>'}</tbody></table></div>
    <div class="obs-card"><div class="obs-card-title">最近运行<span class="sub">点击打开对应需求</span></div><table class="obs-table"><thead><tr><th>需求 / 流程</th><th>状态</th><th>耗时</th><th>Tokens</th><th>成本</th></tr></thead><tbody>${recent||`<tr><td colspan="5" class="detail-empty">${obsEmpty('还没有运行','从首页说清楚要做什么，或直接去 SOP 运行开一条。','issues','去 SOP 运行')}</td></tr>`}</tbody></table></div>
  </div>
  ${obsTableCard('SOP 一览','成功率、返工、取消和各流程占了多少 Token / 成本',['SOP','运行','成功率','返工率','取消率','Tokens','输入 / 输出','Token 占比','成本','成本占比','均执行'],wfRows,11)}
  ${obsTableCard('各模型用量','每个模型花了多少 Token 和钱',['模型','调用','Tokens','输入 / 输出','Token 占比','成本','成本占比','成功率','均执行'],modelRows,9)}
  ${obsScope()==='project'?obsTableCard('成员汇总','当前项目全部责任人，按 Token 排序',['成员','运行','成功','失败','Tokens','输入 / 输出','成本','执行耗时'],memberRows,8):''}
  ${obsTableCard('每人每日用量','责任人 × 自然日',['日期','成员','运行','Tokens','输入 / 输出','成本','执行耗时'],userRows,7,'obs-users')}
  <div class="obs-split">
    ${obsTableCard('需求类型分布','按需求来源聚合，已完成数按需求去重',['来源','关联执行','SOP 成功','已交付需求','Tokens','成本','平均执行'],srcRows,7)}
    ${obsTableCard('编排类型分布','按 SOP 统计标签归类，完成需求数已按需求去重',['统计标签','执行','成功','完成需求','Tokens','成本'],catRows,6)}
  </div>
  ${obsTableCard('SOP 版本对比','同一 SOP 不同版本的执行效果；样本 < 5 标注样本不足',['SOP','版本','运行','成功率','均执行耗时','Tokens','成本','提示'],versionRows,8)}
  ${profileRows?obsTableCard('SOP 编排画像','节点数、节点类型构成与受控统计标签',['SOP','节点数','节点类型构成','统计标签','使用频次'],profileRows,5):''}
  ${obsTableCard('节点质量明细','按失败节点数排序；重试率 = 重试节点 ÷ 节点执行次数',['节点','执行','成功','失败','返工','重试率','均执行耗时','Tokens','工具调用'],nodeQualityRows,9)}
  <div class="obs-split">
    ${obsTableCard('失败原因','节点错误归一化后的高频原因',['节点','错误','次数'],failReasonRows,3)}
    ${obsTableCard('慢节点','按平均执行耗时排序',['节点','执行','均耗时','P95','成功率','Tokens'],slowRows,6)}
  </div>`;
}
function obsDelivery(){
  const d=state.analytics?.delivery||{},items=state.analytics?.deliveries||[];
  const filter=state.obsDeliveryFilter||'all';
  const sourceLabel=s=>s==='github'?'GitHub':s==='local'?'本地':s||'—';
  const mrLabel=s=>s==='merged'?'<span style="color:#34d399;font-weight:600">已合入</span>':s==='integrated'?'<span style="color:#fbbf24;font-weight:600">已合入基线</span>':({opened:'待合入',reopened:'重开',closed:'已关闭',none:'无 PR',unknown:'待查合入'}[s]||s||'—');
  const visible=items.filter(it=>{
    if(filter==='delivered')return it.delivered;
    if(filter==='sop')return it.sopSucceeded;
    if(filter==='mr')return it.mrState==='merged';
    if(filter==='baseline')return it.mrState==='integrated';
    if(filter==='github'||filter==='local')return (it.source||'local')===filter;
    return true;
  });
  const chips=[['all','全部'],['delivered','已交付'],['sop','SOP 成功'],['mr','PR 已合入'],['baseline','已合入基线'],['github','GitHub'],['local','本地']].map(([id,label])=>`<button class="obs-tab ${filter===id?'active':''}" data-obs-delivery="${id}">${label}</button>`).join('');
  const deliveryRows=visible.map(it=>{
    const issue=it.externalIid?`#${it.externalIid}`:(it.externalId||it.issueId);
    const mr=it.mrUrl?`<a href="${escapeHTML(safeURL(it.mrUrl))}" target="_blank" rel="noopener noreferrer">!${it.mrIid||''}</a>`:'—';
    const startedLanes=(it.lanes||[]).filter(l=>l.started);
    const sop=startedLanes.length?startedLanes.map(l=>`${l.label}${l.status==='succeeded'?'✓':''}`).join(' / '):(it.workflowName||it.workflowId);
    return `<tr class="run-row" data-edit-issue="${escapeHTML(it.issueId)}"><td>${sourceLabel(it.source)} ${escapeHTML(issue)}</td><td><b>${escapeHTML(it.title||it.issueId)}</b></td><td>${escapeHTML(sop)}</td><td><span class="status ${it.runStatus||''}">${it.sopSucceeded?'SOP 成功':(runStatus(it.runStatus)||it.runStatus||'—')}</span></td><td>${duration(it.durationMs)}</td><td>${it.executionDurationMs?duration(it.executionDurationMs):'—'}</td><td>${it.cycleDurationMs?duration(it.cycleDurationMs):'—'}</td><td>${it.runCount||0} 次</td><td>${escapeHTML(obsUserName(it.userId)||'—')}</td><td>${formatTokens(it.tokens)}</td><td>${costLabel(it.costUsd,it.costKnown)}</td><td>${mr} · ${mrLabel(it.mrState)}</td><td>${it.delivered?'已交付':'未交付'}</td></tr>`;
  });
  const pagedDeliveries=paginateList('obs-deliveries', deliveryRows);
  const rows=pagedDeliveries.items.join('');
  const summary=state.analytics?.summary||{};
  return obsKpiRow([
    {label:'已交付',value:d.delivered||0,foot:`SOP 成功且已在默认分支 · ${d.issues||0} 个有运行的单`,color:'#34d399'},
    {label:'完成需求 / Bug',value:`${d.requirementCount||0} / ${d.bugFixCount||0}`,foot:`按 SOP 统计标签归类，需求去重`,color:'#5b8cff'},
    {label:'PR 已合入',value:d.mrMerged||0,foot:`仅默认分支；${d.mrIntegrated||0} 个已合入基线`,color:'#38bdf8'},
    {label:'平均执行耗时',value:d.averageExecutionDurationMs?duration(d.averageExecutionDurationMs):'—',foot:`P50 ${d.p50ExecutionDurationMs?duration(d.p50ExecutionDurationMs):'—'} · P95 ${d.p95ExecutionDurationMs?duration(d.p95ExecutionDurationMs):'—'}`,color:'#8b5cf6'},
    {label:'平均端到端耗时',value:d.averageDurationMs?duration(d.averageDurationMs):'—',foot:`累计 ${formatTokens(d.totalTokens||0)} tokens`,color:'#38bdf8'},
    {label:'人均周吞吐',value:summary.weeklyActiveContributors?Number(summary.weeklyThroughputPerUser||0).toFixed(2):'—',foot:`近 7 天 ${summary.weeklyDeliveredIssues||0} 个交付 / ${summary.weeklyActiveContributors||0} 人`,color:'#34d399'},
    {label:'每需求平均执行次数',value:summary.runsPerIssue!=null?Number(summary.runsPerIssue).toFixed(2):'—',foot:`失败 ${summary.failuresPerIssue!=null?Number(summary.failuresPerIssue).toFixed(2):'—'} 次/需求 · 进行中 ${summary.inProgressIssues||0} 个`,color:'#fbbf24'},
  ])+`<div class="obs-card" style="margin-bottom:14px"><div class="obs-card-title">需求 / GitHub Issue<span class="sub">耗时为该需求最近一次执行的端到端时间；执行耗时 = 该需求全部执行按 SOP 节点求和；周期为首次执行到最后一次执行。「已合入 / 已交付」只统计默认分支。</span></div><div class="obs-tabs" style="margin:0 0 10px">${chips}</div><table class="obs-table"><thead><tr><th>单号</th><th>标题</th><th>SOP</th><th>流程</th><th>端到端</th><th>执行耗时</th><th>周期</th><th>执行次数</th><th>负责人</th><th>Tokens</th><th>成本</th><th>PR</th><th>交付</th></tr></thead><tbody>${rows||`<tr><td colspan="13" class="detail-empty">${obsEmpty('还没有带需求上下文的运行','先在 SOP 运行里开一条带需求的执行，交付和成本会记在这里。','issues','去 SOP 运行')}</td></tr>`}</tbody></table>${pagerHTML('obs-deliveries',pagedDeliveries)}</div>`;
}
function obsPlain(v){return String(v==null?'':v).replace(/<[^>]+>/g,'')}
function obsReportScope(){
  const project=currentProject();
  const dim=obsScope()==='project'?'项目':'个人';
  const who=obsScope()==='project'?(project?.name||project?.id||'当前项目'):(state.obsUserFilter?obsUserName(state.obsUserFilter):'全部成员');
  const source=(state.obsSourceFilter?obsSourceLabel(state.obsSourceFilter):'全部需求来源')+' · '+({internal:'平台内执行',external:'外部工作流'}[state.obsOriginFilter]||'全部执行');
  return {project:project?.name||project?.id||'全部项目', dim, who, source, at:new Date().toLocaleString()};
}
function obsReportTable(title,headers,rows){
  if(!rows)return `<section class="block"><h2>${title}</h2><p class="empty">暂无数据</p></section>`;
  return `<section class="block"><h2>${title}</h2><table><thead><tr>${headers.map(h=>`<th>${h}</th>`).join('')}</tr></thead><tbody>${rows}</tbody></table></section>`;
}
async function exportObsReport(){
  ensureObsUserFilter();
  toast('正在生成报告…');
  try{
    if(!state.obsAnalyticsFull){
      state.analytics=await api('/api/analytics'+analyticsQuery({overview:false}),{silent:true});
      state.obsAnalyticsFull=true;
    }
    const html=buildObsReportHTML();
    const blob=new Blob([html],{type:'text/html;charset=utf-8'});
    const a=document.createElement('a');
    const scope=obsReportScope();
    const stamp=new Date().toISOString().slice(0,10);
    a.href=URL.createObjectURL(blob);
    a.download=`LoopForge用量报告-${scope.project}-${scope.who}-${stamp}.html`;
    document.body.appendChild(a);a.click();a.remove();
    setTimeout(()=>URL.revokeObjectURL(a.href),1500);
    toast('已导出 HTML 报告','ok');
  }catch(e){toast(e.message||'导出失败','err')}
}
function buildObsReportHTML(){
  const scope=obsReportScope(),summary=state.analytics?.summary||{},d=state.analytics?.delivery||{};
  const workflows=state.data.workflows||[];
  const stats=(state.analytics?.workflows||[]).map(w=>({...w,name:w.name||workflows.find(x=>x.id===w.key)?.name||w.key}));
  const wfRows=stats.map(w=>`<tr><td>${escapeHTML(w.name)}</td><td>${w.runs}</td><td>${percent(w.successRate)}</td><td>${w.qualityUnknownRuns===w.runs?'—':percent(w.reworkRate)}</td><td>${formatTokens(w.tokens)}</td><td>${obsPlain(costLabel(w.costUsd,!w.costUnknownRuns))}</td><td>${w.averageExecutionDurationMs?duration(w.averageExecutionDurationMs):'—'}</td></tr>`).join('');
  const modelRows=(state.analytics?.models||[]).map(m=>`<tr><td>${escapeHTML(m.name||m.key)}</td><td>${m.runs}</td><td>${formatTokens(m.tokens)}</td><td>${formatTokens(m.inputTokens)} / ${formatTokens(m.outputTokens)}</td><td>${m.tokenSharePct!=null?m.tokenSharePct.toFixed(1)+'%':'—'}</td><td>${obsPlain(costLabel(m.costUsd,!m.costUnknownRuns))}</td></tr>`).join('');
  const userRows=(state.analytics?.userUsage||[]).slice(0,40).map(u=>`<tr><td>${escapeHTML(u.date)}</td><td>${escapeHTML(obsUserName(u.userId))}</td><td>${u.runs}</td><td>${formatTokens(u.tokens)}</td><td>${formatTokens(u.inputTokens)} / ${formatTokens(u.outputTokens)}</td><td>${obsPlain(costLabel(u.costUsd,u.costKnown))}</td></tr>`).join('');
  const memberRows=obsUserTotals().map(u=>`<tr><td>${escapeHTML(obsUserName(u.userId))}</td><td>${u.runs}</td><td>${u.success||0}</td><td>${u.failed||0}</td><td>${formatTokens(u.tokens)}</td><td>${formatTokens(u.inputTokens)} / ${formatTokens(u.outputTokens)}</td><td>${obsPlain(costLabel(u.costUsd,u.costKnown))}</td></tr>`).join('');
  const srcRows=(state.analytics?.issueSources||[]).map(s=>`<tr><td>${obsSourceLabel(s.key)}</td><td>${s.runs}</td><td>${s.succeeded||0}</td><td>${s.completedIssues||0}</td><td>${formatTokens(s.tokens)}</td><td>${obsPlain(costLabel(s.costUsd,true))}</td></tr>`).join('');
  const versionRows=(state.analytics?.workflowVersions||[]).map(v=>`<tr><td>${escapeHTML(v.name||v.workflowId)}</td><td>v${v.version}</td><td>${v.runs}</td><td>${percent(v.successRate)}</td><td>${formatTokens(v.tokens)}</td><td>${obsPlain(costLabel(v.costUsd,true))}</td></tr>`).join('');
  const trendRows=obsTrendWindow(state.analytics?.trend||[]).slice().reverse().map(t=>`<tr><td>${escapeHTML(obsTrendLabel(t))}</td><td>${t.runs}</td><td>${t.issues||0}</td><td>${t.succeeded}</td><td>${t.failed||0}</td><td>${formatTokens(t.tokens)}</td><td>${obsPlain(costLabel(t.costUsd,!t.costUnknownRuns))}</td><td>${t.executionDurationMs?duration(t.executionDurationMs):'—'}</td></tr>`).join('');
  const projectRows=(state.obsCrossProject?.projectUsage||state.analytics?.projectUsage||[]).map(r=>`<tr><td>${escapeHTML(obsProjectName(r.projectId))}</td><td>${r.runs||0}</td><td>${r.uniqueIssues||0}</td><td>${formatTokens(r.tokens)}</td><td>${obsPlain(costLabel(r.costUsd,r.costKnown))}</td></tr>`).join('');
  const deliveryRows=(state.analytics?.deliveries||[]).slice(0,80).map(it=>`<tr><td>${obsSourceLabel(it.source)} ${escapeHTML(String(it.externalIid?('#'+it.externalIid):(it.externalId||it.issueId)))}</td><td>${escapeHTML(it.title||it.issueId)}</td><td>${escapeHTML(it.workflowName||it.workflowId)}</td><td>${it.sopSucceeded?'SOP 成功':(it.runStatus||'—')}</td><td>${escapeHTML(obsUserName(it.userId))}</td><td>${formatTokens(it.tokens)}</td><td>${obsPlain(costLabel(it.costUsd,it.costKnown))}</td><td>${it.delivered?'已交付':'未交付'}</td></tr>`).join('');
  const kpis=[
    ['运行',summary.totalRuns||0,`${summary.activeRuns||0} 进行中`],
    ['成功率',summary.totalRuns?percent(summary.successRate):'—',`${summary.succeededRuns||0} 成功 / ${summary.failedRuns||0} 失败`],
    ['一次通过',summary.totalRuns?percent(summary.firstPassRate):'—',`${summary.reworkedRuns||0} 次返工`],
    ['Token',formatTokens(summary.totalTokens),`入 ${formatTokens(summary.inputTokens)} / 出 ${formatTokens(summary.outputTokens)}`],
    ['成本',obsPlain(costLabel(summary.totalCostUsd,summary.costUnknownRuns===0)),summary.costUnknownRuns?`${summary.costUnknownRuns} 次未识别`:'已识别'],
    ['P95 执行',summary.executionP95DurationMs?duration(summary.executionP95DurationMs):'—',summary.executionAverageDurationMs?`平均 ${duration(summary.executionAverageDurationMs)}`:'—'],
    ['已交付',d.delivered||0,`${d.issues||0} 个有运行的单`],
    ['PR 已合入',d.mrMerged||0,`${d.mrIntegrated||0} 已合入基线`],
  ].map(([l,v,f])=>`<article><span>${l}</span><b>${v}</b><small>${f}</small></article>`).join('');
  return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${escapeHTML(scope.project)} 用量报告</title>
<style>
:root{--text:#152033;--muted:#5b6b82;--line:#e6ebf3;--bg:#f5f7fb;--card:#fff;--blue:#3b6cff}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:14px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Noto Sans SC",sans-serif}
.wrap{max-width:1080px;margin:0 auto;padding:36px 28px 64px}header{margin-bottom:28px}
.eyebrow{margin:0;color:var(--blue);font-size:12px;font-weight:700;letter-spacing:.08em;text-transform:uppercase}
h1{margin:6px 0 8px;font-size:28px}header p{margin:0;color:var(--muted)}
.meta{display:flex;flex-wrap:wrap;gap:8px;margin-top:14px}
.meta span{padding:5px 10px;border:1px solid var(--line);border-radius:999px;background:var(--card);font-size:12px;color:var(--muted)}
.kpis{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px;margin:22px 0}
.kpis article{padding:16px 16px 14px;border:1px solid var(--line);border-radius:14px;background:var(--card)}
.kpis span,.kpis small{display:block;color:var(--muted);font-size:12px}.kpis b{display:block;margin:6px 0 4px;font-size:24px;letter-spacing:-.03em}
.block{margin:22px 0;padding:18px 18px 8px;border:1px solid var(--line);border-radius:14px;background:var(--card)}
.block h2{margin:0 0 12px;font-size:16px}table{width:100%;border-collapse:collapse;font-size:13px}
th{text-align:left;color:var(--muted);font-size:11px;padding:8px;border-bottom:1px solid var(--line)}td{padding:9px 8px;border-bottom:1px solid var(--line);vertical-align:top}
.empty{color:var(--muted);padding:12px 0}footer{margin-top:28px;color:var(--muted);font-size:12px}
@media print{body{background:#fff}.wrap{padding:0}.kpis article,.block{break-inside:avoid}}
@media(max-width:800px){.kpis{grid-template-columns:repeat(2,minmax(0,1fr))}}
</style></head><body><div class="wrap">
<header><p class="eyebrow">LoopForge</p><h1>用量分析报告</h1><p>效果看成功率和一次通过，投入看 Token、成本和执行耗时。交付看 SOP 是否跑完、PR 是否合入。</p>
<div class="meta"><span>项目 ${escapeHTML(scope.project)}</span><span>维度 ${escapeHTML(scope.dim)}</span><span>${obsScope()==='project'?'范围':'责任人'} ${escapeHTML(scope.who)}</span><span>来源 ${escapeHTML(scope.source)}</span><span>生成于 ${escapeHTML(scope.at)}</span></div></header>
<section class="kpis">${kpis}</section>
${obsReportTable('SOP 一览',['SOP','运行','成功率','返工率','Tokens','成本','均执行'],wfRows)}
${obsReportTable('各模型用量',['模型','调用','Tokens','输入 / 输出','Token 占比','成本'],modelRows)}
${obsScope()==='project'?obsReportTable('成员汇总',['成员','运行','成功','失败','Tokens','输入 / 输出','成本'],memberRows):''}
${obsReportTable('每人每日用量',['日期','成员','运行','Tokens','输入 / 输出','成本'],userRows)}
${obsReportTable('需求类型分布',['来源','关联执行','SOP 成功','已交付需求','Tokens','成本'],srcRows)}
${obsReportTable('SOP 版本对比',['SOP','版本','运行','成功率','Tokens','成本'],versionRows)}
${obsReportTable(obsGrainCaption()+'趋势',['周期','运行','需求','成功','失败','Tokens','成本','执行耗时'],trendRows)}
${projectRows?obsReportTable('各项目用量',['项目','运行','需求','Tokens','成本'],projectRows):''}
${obsReportTable('交付明细',['单号','标题','SOP','流程','责任人','Tokens','成本','交付'],deliveryRows)}
<footer>由 LoopForge 用量分析导出。日志与 Trace 请在运行记录中查看。</footer>
</div></body></html>`;
}


function obsCanManageSources(){
  const p=currentProject(),u=state.currentUser;
  return !!(p&&u&&!u.isGuest&&(u.isPlatformAdmin||Number(p.ownerId)===Number(u.id)||(p.members||[]).some(m=>Number(m.userId)===Number(u.id)&&m.role==='maintainer')));
}
async function openObsSources(){
  const project=currentProject();if(!project)return toast('请先选择项目');
  const dialog=document.createElement('dialog');dialog.className='obs-source-dialog';
  const memberIds=new Set([project.ownerId,...(project.members||[]).map(m=>m.userId)].map(Number));
  const users=(state.data.users||[]).filter(u=>memberIds.has(Number(u.id)));
  const options=users.map(u=>`<option value="${Number(u.id)}">${escapeHTML(userDisplay(u))}</option>`).join('');
  dialog.innerHTML=`<div class="obs-card-title">${escapeHTML(project.name)} · 外部数据源<button class="button ghost" data-close type="button">关闭</button></div><p>为外部工作流创建专用上报凭据。Token 仅在创建时显示，可随时撤销。</p><div data-sources>正在加载…</div><form data-create><label class="field">数据源名称<input name="name" required maxlength="100" placeholder="例如：团队代码评审流水线"></label><label class="field">有效期（天）<input name="days" type="number" min="1" max="365" value="90" required></label><p>责任人映射（可选）：外部账号对应本项目成员；不填责任人的记录归入“未分配”。</p><div data-mappings></div><button type="button" class="button ghost" data-add>添加责任人映射</button><button class="button primary" type="submit">创建数据源</button></form><div data-created hidden><p>请保存 Token，关闭后无法再次查看。</p><textarea data-token readonly aria-label="上报 Token"></textarea><button type="button" class="button ghost" data-copy>复制 Token</button><p>上报地址</p><code>${escapeHTML(location.origin+'/api/insights/v1/ingest')}</code><p>完成或失败时上报一次执行汇总；相同 ID 重试不会重复计量。协议及脚本见仓库 docs/external-workflow-usage.md。</p></div>`;
  document.body.appendChild(dialog);dialog.onclose=()=>dialog.remove();dialog.querySelector('[data-close]').onclick=()=>dialog.close();dialog.showModal();
  async function refresh(){
    try{
      const items=await api('/api/insights/v1/sources?projectId='+encodeURIComponent(project.id),{silent:true});
      if(!dialog.isConnected)return;
      dialog.querySelector('[data-sources]').innerHTML=items.length?items.map(s=>`<div class="obs-source-row"><span><b>${escapeHTML(s.name)}</b> · ${escapeHTML(s.tokenPrefix)} · ${s.revokedAt?'已撤销':new Date(s.expiresAt)<=new Date()?'已过期':'有效至 '+new Date(s.expiresAt).toLocaleDateString()}</span>${s.revokedAt?'':`<button type="button" class="button ghost" data-revoke="${escapeHTML(s.id)}">撤销</button>`}</div>`).join(''):'<p class="muted">尚无外部数据源</p>';
      dialog.querySelectorAll('[data-revoke]').forEach(b=>b.onclick=async()=>{b.disabled=true;try{await api('/api/insights/v1/sources/'+encodeURIComponent(b.dataset.revoke),{method:'DELETE'});await refresh()}catch(e){toast(e.message);b.disabled=false}});
    }catch(e){dialog.querySelector('[data-sources]').textContent=e.message}
  }
  dialog.querySelector('[data-add]').onclick=()=>{
    const row=document.createElement('div');row.className='obs-source-row';row.dataset.mapping='';
    row.innerHTML=`<input required maxlength="128" placeholder="外部账号" aria-label="外部账号"><select required aria-label="平台责任人"><option value="">选择责任人</option>${options}</select><button type="button" class="button ghost">移除</button>`;
    row.querySelector('button').onclick=()=>row.remove();dialog.querySelector('[data-mappings]').appendChild(row);
  };
  dialog.querySelector('[data-copy]').onclick=async()=>{try{await navigator.clipboard.writeText(dialog.querySelector('[data-token]').value);toast('Token 已复制')}catch(e){toast('请手动复制 Token')}};
  dialog.querySelector('[data-create]').onsubmit=async e=>{
    e.preventDefault();const form=e.currentTarget,button=form.querySelector('[type=submit]'),userMappings=Object.create(null);
    for(const row of form.querySelectorAll('[data-mapping]')){const key=row.querySelector('input').value.trim();if(!key||Object.hasOwn(userMappings,key))return toast('外部账号不能为空或重复');userMappings[key]=Number(row.querySelector('select').value)}
    button.disabled=true;
    try{
      const result=await api('/api/insights/v1/sources',{method:'POST',body:JSON.stringify({projectId:project.id,name:form.elements.name.value,expiresInDays:Number(form.elements.days.value),userMappings})});
      dialog.querySelector('[data-created]').hidden=false;dialog.querySelector('[data-token]').value=result.token;form.reset();await refresh();
    }catch(e){toast(e.message)}finally{button.disabled=false}
  };
  await refresh();
}
