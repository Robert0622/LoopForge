const $ = s => document.querySelector(s);
const $$ = s => [...document.querySelectorAll(s)];
window.__LOOPFORGE_APP_DOMAIN__ = document.querySelector('meta[name="loopforge-app-domain"]')?.content || window.__LOOPFORGE_APP_DOMAIN__ || '';
// Go serialises an unset time as 0001-01-01; treat that like no time at all.
const knownTime = v => { const d = v ? new Date(v) : null; return d && d.getFullYear() > 1970 ? d : null; };
const clone = v => v==null ? v : JSON.parse(JSON.stringify(v));
const state = {
  data:null, currentUser:null, authRequired:false, currentView:'', analytics:null, workflow:null, workflowSaved:null, workflowDirty:false, selectedNode:null, connectFrom:null, connectCursor:null, activeRun:null,
  selectedAgent:null, selectedRun:null, selectedRunNode:null, runFilter:'all', runQuery:'', runWorkflowFilter:'',
  selectedIssue:null, projectFilter:'', projectDraft:null, requirementIssueId:null, sourceFilter:'',
  sopScope:'project', agentScope:'project', ruleScope:'project', skillScope:'project', dockerfileScope:'project', environmentScope:'project', sandboxScope:'project',
  issueRunFilter:'', issueStatusFilter:'', issueAssigneeFilter:'', issueQuery:'', issueView:localStorage.getItem('loopforge.issueView')||'kanban',
  assetType:'dockerfiles', selectedAsset:null, mcpTab:'market', mcpScope:'project', mcpMarket:[], mcpQuery:'', editingMCPId:'', poll:null, runStream:null,
  obsTab:'overview', obsDeliveryFilter:'all', obsTraceRunId:null, obsTraceAdapter:'live', obsScope:localStorage.getItem('loopforge.obsScope')==='project'?'project':'user', obsUserFilter:'', obsSourceFilter:'', obsUserFilterReady:false, obsTrendGrain:['day','week','month','all'].includes(localStorage.getItem('loopforge.obsTrendGrain'))?localStorage.getItem('loopforge.obsTrendGrain'):'all', obsPeriodKey:'', obsTrendMetric:['runs','issues','cost'].includes(localStorage.getItem('loopforge.obsTrendMetric'))?localStorage.getItem('loopforge.obsTrendMetric'):'tokens', obsCrossProject:null, drawerOpen:false, issueDetailTab:'overview', issueDetailData:null, propTab:'basic',
  uiScroll:{runId:'', nodeId:'', graphLeft:0, feedTop:0, telTop:0, feedStick:true, telStick:true},
  runLogs:null, runTelemetry:null, runTelemetryAdapter:'live', runWorkspace:null,
  accessTab:localStorage.getItem('loopforge.accessTab')||'setup', accessClient:'cursor',
  accessCapabilities:null, accessCapabilitiesStatus:'idle', accessCapabilitiesError:'',
  accessTokens:[], accessTokensStatus:'idle', accessTokensError:'', revealedToken:'',
  metricsRange:['7d','30d','90d','365d','custom'].includes(localStorage.getItem('loopforge.metricsRange'))?localStorage.getItem('loopforge.metricsRange'):'30d', metricsDateFrom:localStorage.getItem('loopforge.metricsDateFrom')||'', metricsDateTo:localStorage.getItem('loopforge.metricsDateTo')||'', metricsData:null, metricsIssues:[], metricsLoading:false, metricsError:'', metricsTrendMetric:localStorage.getItem('loopforge.metricsTrendMetric')||'deliveries',
  listPages:{}
};

const LIST_PAGE_SIZE={issues:20,runs:20,mcp:24,obs:20};
function listPageSize(key){return String(key||'').startsWith('obs-')?LIST_PAGE_SIZE.obs:(LIST_PAGE_SIZE[key]||20)}
function resetListPage(key){if(!state.listPages)state.listPages={};if(key)state.listPages[key]=1}
function resetListPages(prefix){
  if(!prefix){state.listPages={};return}
  Object.keys(state.listPages||{}).forEach(k=>{if(k===prefix||k.startsWith(prefix))state.listPages[k]=1});
}
function paginateList(key, items, size){
  const list=Array.isArray(items)?items:[];
  const pageSize=Math.max(1, Number(size)||listPageSize(key));
  const total=list.length;
  const pages=Math.max(1, Math.ceil(total/pageSize)||1);
  if(!state.listPages)state.listPages={};
  let page=Number(state.listPages[key]||1);
  if(!Number.isFinite(page)||page<1)page=1;
  if(page>pages)page=pages;
  state.listPages[key]=page;
  const start=(page-1)*pageSize;
  const end=Math.min(start+pageSize, total);
  return {key, page, pages, size:pageSize, start, end, total, items:list.slice(start, end)};
}
function pagerHTML(key, p){
  if(!p||p.total<=p.size)return '';
  const page=p.page, pages=p.pages, nums=[];
  const push=n=>{if(n>=1&&n<=pages&&!nums.includes(n))nums.push(n)};
  push(1);push(page-1);push(page);push(page+1);push(pages);
  nums.sort((a,b)=>a-b);
  let buttons='', prev=0;
  for(const n of nums){
    if(prev&&n>prev+1)buttons+=`<span class="pager-ellipsis">…</span>`;
    buttons+=`<button type="button" class="pager-page${n===page?' active':''}" data-page-key="${escapeHTML(key)}" data-page="${n}">${n}</button>`;
    prev=n;
  }
  return `<nav class="pager" data-pager="${escapeHTML(key)}" aria-label="分页"><span class="pager-meta">第 ${p.start+1}–${p.end} 条 / 共 ${p.total} 条</span><div class="pager-nav"><button type="button" class="pager-btn" data-page-key="${escapeHTML(key)}" data-page="${page-1}" ${page<=1?'disabled':''}>上一页</button>${buttons}<button type="button" class="pager-btn" data-page-key="${escapeHTML(key)}" data-page="${page+1}" ${page>=pages?'disabled':''}>下一页</button></div></nav>`;
}
function goListPage(key, page){
  if(!state.listPages)state.listPages={};
  state.listPages[key]=Math.max(1, Number(page)||1);
  if(String(key).startsWith('obs-'))renderObservability();
  else if(key==='issues')renderIssues();
  else if(key==='runs')renderRuns();
  else if(key==='mcp')renderMCPManagement();
  const el=document.querySelector(`[data-page-anchor="${key}"]`)||document.querySelector(`[data-pager="${key}"]`);
  if(el)el.scrollIntoView({block:'nearest'});
}
function bindPager(root){
  (root||document).querySelectorAll('[data-page-key]').forEach(btn=>{
    btn.onclick=e=>{
      e.preventDefault();
      e.stopPropagation();
      if(btn.disabled)return;
      goListPage(btn.dataset.pageKey, btn.dataset.page);
    };
  });
}

let apiInflight=0, toastTimer=null;
async function api(path, options={}) {
  const {silent, ...fetchOpts}=options;
  const bar=$('#globalProgress');
  if(!silent){
    apiInflight++;
    if(bar){bar.classList.add('active');bar.setAttribute('aria-hidden','false')}
  }
  try{
    const request={...fetchOpts};
    if(!request.headers&&!(request.body instanceof FormData))request.headers={'Content-Type':'application/json'};
    const res = await fetch(path, request);
    const body = await res.json().catch(()=>({}));
    if (res.status===401 && state.authRequired && !state.proxyAuth && !String(path).startsWith('/api/auth/')) openLoginModal({forced:true});
    if (!res.ok) throw new Error(body.error || `HTTP ${res.status}`);
    return body;
  } finally {
    if(!silent){
      apiInflight=Math.max(0, apiInflight-1);
      if(bar&&apiInflight===0){bar.classList.remove('active');bar.setAttribute('aria-hidden','true')}
    }
  }
}
let toastUndoHandler=null;
function clearToastUndo(){
  if(toastUndoHandler){document.removeEventListener('keydown',toastUndoHandler);toastUndoHandler=null}
}
function armToastUndo(fn,remove){
  clearToastUndo();
  toastUndoHandler=e=>{
    if(!(e.metaKey||e.ctrlKey)||e.key.toLowerCase()!=='z'||e.shiftKey)return;
    if(e.target?.closest?.('input,textarea,select,[contenteditable="true"]'))return;
    e.preventDefault();
    clearToastUndo();
    remove();
    try{fn()}catch(err){console.error(err)}
  };
  document.addEventListener('keydown',toastUndoHandler);
}
function toast(message, type='info', duration=type==='err'?6000:2600, extra){
  const container=$('#toastContainer');
  const legacy=$('#toast');
  if(!container&&legacy){
    const kind=type==='ok'||type==='err'?type:'info';
    legacy.textContent=message;
    legacy.className=`toast show toast-${kind}`;
    clearTimeout(toastTimer);
    toastTimer=setTimeout(()=>legacy.classList.remove('show'), 2200);
    return;
  }
  if(!container)return;
  const kind=['ok','err','warn','info'].includes(type)?type:'info';
  const icons={ok:'✓',err:'✕',warn:'⚠️',info:'ℹ️'};
  const item=document.createElement('div');
  item.className=`toast-item toast-${kind}`;
  item.setAttribute('role','status');
  item.innerHTML=`<span class="toast-item-icon">${icons[kind]}</span><span class="toast-item-msg">${escapeHTML(message)}</span>${extra?.undo?'<button type="button" class="toast-undo">撤销 ⌘Z</button>':''}<button type="button" class="toast-item-close" title="关闭">×</button>`;
  container.appendChild(item);
  let timer=null;
  const remove=()=>{
    if(item.classList.contains('leaving'))return;
    item.classList.add('leaving');
    setTimeout(()=>{try{item.remove()}catch{}}, 200);
  };
  timer=setTimeout(remove, duration);
  item.onmouseenter=()=>clearTimeout(timer);
  item.onmouseleave=()=>{timer=setTimeout(remove, 1400)};
  item.querySelector('.toast-item-close').onclick=()=>{clearToastUndo();remove()};
  if(extra?.undo){
    const undo=item.querySelector('.toast-undo');
    const run=()=>{clearToastUndo();remove();extra.undo()};
    if(undo)undo.onclick=run;
    armToastUndo(extra.undo,remove);
  }
}

let confirmPromiseResolve = null;
function afConfirm(options){
  return new Promise((resolve)=>{
    const modal=$('#confirmModal');
    if(!modal){
      const msg=typeof options==='string'?options:(options?.message||'');
      return resolve(window.confirm(msg));
    }
    const opt=typeof options==='string'?{message:options}:(options||{});
    const title=opt.title||(opt.danger?'危险操作确认':'操作确认');
    const message=opt.message||'确定要继续吗？';
    const confirmText=opt.confirmText||(opt.danger?'确定执行':'确定');
    const cancelText=opt.cancelText||'取消';
    const danger=!!(opt.danger||opt.isDanger);
    const confirmName=String(opt.confirmName||'').trim();

    $('#confirmTitle').textContent=title;
    $('#confirmMessage').textContent=message;
    const okBtn=$('#confirmOkBtn');
    const cancelBtn=$('#confirmCancelBtn');
    const iconWrap=$('#confirmIconWrap');
    const iconEl=$('#confirmIcon');
    const promptWrap=$('#confirmPromptWrap');
    const rerunFields=$('#rerunFromNodeFields');

    promptWrap.hidden=true;
    if(rerunFields)rerunFields.hidden=true;
    const nameWrap=$('#confirmNameWrap');
    const nameInput=$('#confirmNameInput');
    const nameHint=$('#confirmNameHint');
    if(nameWrap)nameWrap.hidden=!confirmName;
    if(nameInput){nameInput.value='';nameInput.placeholder=confirmName}
    if(nameHint&&confirmName)nameHint.textContent=`输入 ${confirmName} 以确认`;
    okBtn.textContent=confirmText;
    okBtn.className=`button ${danger?'danger-outline':'primary'}`;
    okBtn.disabled=!!confirmName;
    if(nameInput&&confirmName)nameInput.oninput=()=>{okBtn.disabled=nameInput.value.trim()!==confirmName};
    cancelBtn.textContent=cancelText;
    cancelBtn.hidden=false;

    iconWrap.className=`confirm-icon-wrap ${danger?'danger':opt.info?'info':'warn'}`;
    iconEl.textContent=danger?'🗑️':(opt.info?'ℹ️':'⚠️');

    modal.classList.add('open');
    if(confirmName&&nameInput)nameInput.focus();else okBtn.focus();

    confirmPromiseResolve=(val)=>{
      if(val&&confirmName&&nameInput&&nameInput.value.trim()!==confirmName)return;
      modal.classList.remove('open');
      okBtn.disabled=false;
      if(nameWrap)nameWrap.hidden=true;
      confirmPromiseResolve=null;
      resolve(val);
    };
  });
}

function afPrompt(options, defaultVal=''){
  return new Promise((resolve)=>{
    const modal=$('#confirmModal');
    const opt=typeof options==='string'?{message:options, defaultValue:defaultVal}:(options||{});
    if(!modal){
      return resolve(window.prompt(opt.message||'', opt.defaultValue||''));
    }
    const title=opt.title||'请输入内容';
    const message=opt.message||'';
    const confirmText=opt.confirmText||'确定';
    const cancelText=opt.cancelText||'取消';
    const placeholder=opt.placeholder||'';
    const defaultValue=opt.defaultValue||'';

    $('#confirmTitle').textContent=title;
    $('#confirmMessage').textContent=message;
    const okBtn=$('#confirmOkBtn');
    const cancelBtn=$('#confirmCancelBtn');
    const iconWrap=$('#confirmIconWrap');
    const iconEl=$('#confirmIcon');
    const promptWrap=$('#confirmPromptWrap');
    const promptInput=$('#confirmPromptInput');
    const promptLabel=$('#confirmPromptLabel');
    const rerunFields=$('#rerunFromNodeFields');

    promptWrap.hidden=false;
    if(rerunFields)rerunFields.hidden=true;
    const nameWrap=$('#confirmNameWrap');if(nameWrap)nameWrap.hidden=true;
    okBtn.disabled=false;
    promptLabel.textContent=opt.label||title;
    promptInput.value=defaultValue;
    promptInput.placeholder=placeholder;

    okBtn.textContent=confirmText;
    okBtn.className='button primary';
    cancelBtn.textContent=cancelText;
    cancelBtn.hidden=false;

    iconWrap.className='confirm-icon-wrap info';
    iconEl.textContent='✎';

    modal.classList.add('open');
    promptInput.focus();
    promptInput.select();

    confirmPromiseResolve=(isOk)=>{
      const val=isOk?promptInput.value:null;
      modal.classList.remove('open');
      confirmPromiseResolve=null;
      resolve(val);
    };
  });
}

function afAlert(options){
  return new Promise((resolve)=>{
    const modal=$('#confirmModal');
    const opt=typeof options==='string'?{message:options}:(options||{});
    if(!modal){
      window.alert(opt.message||'');
      return resolve();
    }
    $('#confirmTitle').textContent=opt.title||'提示';
    $('#confirmMessage').textContent=opt.message||'';
    $('#confirmPromptWrap').hidden=true;
    const nameWrap=$('#confirmNameWrap');if(nameWrap)nameWrap.hidden=true;
    const rerunFields=$('#rerunFromNodeFields');
    if(rerunFields)rerunFields.hidden=true;
    const okBtn=$('#confirmOkBtn');
    const cancelBtn=$('#confirmCancelBtn');
    okBtn.textContent='我知道了';
    okBtn.className='button primary';
    cancelBtn.hidden=true;
    const iconWrap=$('#confirmIconWrap');
    const iconEl=$('#confirmIcon');
    iconWrap.className=`confirm-icon-wrap ${opt.type==='err'?'danger':opt.type==='ok'?'info':'warn'}`;
    iconEl.textContent=opt.type==='err'?'✕':(opt.type==='ok'?'✓':'ℹ️');

    modal.classList.add('open');
    okBtn.focus();

    confirmPromiseResolve=()=>{
      modal.classList.remove('open');
      confirmPromiseResolve=null;
      resolve();
    };
  });
}

function highlightCode(code, lang){
  const s=String(code||'');
  const language=String(lang||'').toLowerCase();
  if(language==='shell'||language==='bash'||language==='sh'){
    return highlightShell(s);
  } else if(language==='python'||language==='py'){
    return highlightPython(s);
  } else if(language==='json'){
    return highlightJSON(s);
  }
  return escapeHTML(s);
}

function highlightShell(src){
  const lines=src.split('\n');
  return lines.map(line=>{
    let commentIdx=-1;
    let inSingle=false, inDouble=false;
    for(let i=0;i<line.length;i++){
      const ch=line[i];
      if(ch==="'"&&!inDouble) inSingle=!inSingle;
      else if(ch==='"'&&!inSingle&&(i===0||line[i-1]!=='\\')) inDouble=!inDouble;
      else if(ch==='#'&&!inSingle&&!inDouble){
        commentIdx=i;
        break;
      }
    }
    let codePart=commentIdx>=0?line.slice(0, commentIdx):line;
    let comment=commentIdx>=0?line.slice(commentIdx):'';

    let html=escapeHTML(codePart);
    html=html.replace(/(&quot;[\s\S]*?&quot;|&#39;[\s\S]*?&#39;)/g, '<span class="tok-str">$1</span>');
    html=html.replace(/(\$(?:[a-zA-Z_0-9]+|\{[^}]+\}|[?@*#0-9]))/g, '<span class="tok-var">$1</span>');
    html=html.replace(/\b(if|then|elif|else|fi|for|in|do|done|while|until|case|esac|function|return|exit|export|local|set|unset|shift|trap)\b/g, '<span class="tok-kw">$1</span>');
    html=html.replace(/\b(echo|cd|ls|grep|cat|rm|mkdir|cp|mv|chmod|chown|touch|test|git|go|python|node|npm|pnpm|yarn|pip|curl|wget|tar|zip|unzip|sed|awk|printf|tee|find|kill|ps|docker|source|read)\b/g, '<span class="tok-cmd">$1</span>');
    html=html.replace(/(\s)(-[a-zA-Z0-9_-]+)/g, '$1<span class="tok-op">$2</span>');

    if(comment){
      html+=`<span class="tok-comment">${escapeHTML(comment)}</span>`;
    }
    return html;
  }).join('\n');
}

function highlightPython(src){
  const lines=src.split('\n');
  return lines.map(line=>{
    let commentIdx=-1;
    let inSingle=false, inDouble=false;
    for(let i=0;i<line.length;i++){
      const ch=line[i];
      if(ch==="'"&&!inDouble) inSingle=!inSingle;
      else if(ch==='"'&&!inSingle&&(i===0||line[i-1]!=='\\')) inDouble=!inDouble;
      else if(ch==='#'&&!inSingle&&!inDouble){
        commentIdx=i;
        break;
      }
    }
    let codePart=commentIdx>=0?line.slice(0, commentIdx):line;
    let comment=commentIdx>=0?line.slice(commentIdx):'';

    let html=escapeHTML(codePart);
    html=html.replace(/(&quot;[\s\S]*?&quot;|&#39;[\s\S]*?&#39;)/g, '<span class="tok-str">$1</span>');
    html=html.replace(/\b(def|class|import|from|return|if|elif|else|for|while|try|except|finally|with|as|lambda|pass|raise|yield|async|await|assert|break|continue|global|nonlocal|del|in|is|not|and|or)\b/g, '<span class="tok-kw">$1</span>');
    html=html.replace(/\b(print|len|range|str|int|float|dict|list|set|tuple|bool|open|isinstance|issubclass|type|enumerate|zip|map|filter|sum|min|max|any|all|abs|round|input)\b/g, '<span class="tok-built">$1</span>');
    html=html.replace(/\b(True|False|None)\b/g, '<span class="tok-cmd">$1</span>');
    html=html.replace(/(@[a-zA-Z_0-9.]+)/g, '<span class="tok-var">$1</span>');
    html=html.replace(/\b(\d+(?:\.\d+)?)\b/g, '<span class="tok-num">$1</span>');

    if(comment){
      html+=`<span class="tok-comment">${escapeHTML(comment)}</span>`;
    }
    return html;
  }).join('\n');
}

function highlightJSON(src){
  let html=escapeHTML(src);
  html=html.replace(/(&quot;[\s\S]*?&quot;)(\s*:)?/g, (match, p1, p2)=>{
    if(p2) return `<span class="tok-kw">${p1}</span>${p2}`;
    return `<span class="tok-str">${p1}</span>`;
  });
  html=html.replace(/\b(true|false|null)\b/g, '<span class="tok-cmd">$1</span>');
  html=html.replace(/\b(\d+(?:\.\d+)?)\b/g, '<span class="tok-num">$1</span>');
  return html;
}

function renderMarkdown(md){
  if(!md)return '';
  const text=String(md).replace(/\r\n/g, '\n').replace(/\r/g, '\n');
  const lines=text.split('\n');
  const out=[];
  let inCode=false, codeLang='', codeLines=[];
  let inList=false, listType='', listItems=[];
  let inTable=false, tableRows=[];

  function flushList(){
    if(!inList)return;
    const tag=listType==='ol'?'ol':'ul';
    out.push(`<${tag}>${listItems.join('')}</${tag}>`);
    inList=false;listType='';listItems=[];
  }

  function flushTable(){
    if(!inTable||!tableRows.length)return;
    const parseRow=(r)=>r.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map(c=>c.trim());
    const headers=parseRow(tableRows[0]);
    const bodyRows=tableRows.slice(2);
    const thead=`<thead><tr>${headers.map(h=>`<th>${parseInlineMarkdown(h)}</th>`).join('')}</tr></thead>`;
    const tbody=`<tbody>${bodyRows.map(row=>{
      const cells=parseRow(row);
      return `<tr>${headers.map((_,idx)=>`<td>${parseInlineMarkdown(cells[idx]||'')}</td>`).join('')}</tr>`;
    }).join('')}</tbody>`;
    out.push(`<table>${thead}${tbody}</table>`);
    inTable=false;tableRows=[];
  }

  for(let i=0;i<lines.length;i++){
    const line=lines[i];

    if(line.trim().startsWith('```')){
      if(inCode){
        const rawCode=codeLines.join('\n');
        const highlighted=highlightCode(rawCode, codeLang);
        out.push(`<div class="md-code-block"><div class="md-code-header"><span class="md-code-lang">${escapeHTML(codeLang||'TEXT')}</span><button type="button" class="md-code-copy">复制</button></div><pre><code>${highlighted}</code></pre></div>`);
        inCode=false;codeLang='';codeLines=[];
      } else {
        flushList();
        flushTable();
        inCode=true;
        codeLang=line.trim().slice(3).trim();
        codeLines=[];
      }
      continue;
    }
    if(inCode){
      codeLines.push(line);
      continue;
    }

    if(line.trim().startsWith('|') && line.trim().endsWith('|')){
      flushList();
      if(!inTable){
        const next=lines[i+1]||'';
        if(next.trim().startsWith('|') && /\|[-:| ]+\|/.test(next)){
          inTable=true;
          tableRows=[line];
          continue;
        }
      } else {
        tableRows.push(line);
        continue;
      }
    } else if(inTable){
      flushTable();
    }

    if(!line.trim()){
      flushList();
      continue;
    }

    const alertMatch=line.match(/^>\s*\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]/i);
    if(alertMatch){
      flushList();
      const kind=alertMatch[1].toLowerCase();
      const alertLines=[];
      while(i+1<lines.length && lines[i+1].trim().startsWith('>')){
        i++;
        alertLines.push(lines[i].replace(/^>\s?/, ''));
      }
      const titleIcons={note:'ℹ️ NOTE', tip:'💡 TIP', important:'🟣 IMPORTANT', warning:'⚠️ WARNING', caution:'🛑 CAUTION'};
      const titleText=titleIcons[kind]||kind.toUpperCase();
      out.push(`<div class="md-alert md-alert-${kind}"><div class="md-alert-title">${titleText}</div><div class="md-alert-content">${renderMarkdown(alertLines.join('\n'))}</div></div>`);
      continue;
    }

    if(line.startsWith('>')){
      flushList();
      const bqLines=[line.replace(/^>\s?/, '')];
      while(i+1<lines.length && lines[i+1].trim().startsWith('>')){
        i++;
        bqLines.push(lines[i].replace(/^>\s?/, ''));
      }
      out.push(`<blockquote>${renderMarkdown(bqLines.join('\n'))}</blockquote>`);
      continue;
    }

    if(/^(?:---|\*\*\*|___)\s*$/.test(line.trim())){
      flushList();
      out.push('<hr>');
      continue;
    }

    const hMatch=line.match(/^(#{1,6})\s+(.+)$/);
    if(hMatch){
      flushList();
      const level=hMatch[1].length;
      out.push(`<h${level}>${parseInlineMarkdown(hMatch[2])}</h${level}>`);
      continue;
    }

    const taskMatch=line.match(/^[-*]\s+\[([ xX])\]\s+(.+)$/);
    if(taskMatch){
      if(!inList||listType!=='ul'){
        flushList();
        inList=true;listType='ul';
      }
      const checked=taskMatch[1].toLowerCase()==='x';
      listItems.push(`<li class="task-list-item"><input type="checkbox" class="task-checkbox" disabled ${checked?'checked':''}><span>${parseInlineMarkdown(taskMatch[2])}</span></li>`);
      continue;
    }

    const ulMatch=line.match(/^[-*+]\s+(.+)$/);
    if(ulMatch){
      if(!inList||listType!=='ul'){
        flushList();
        inList=true;listType='ul';
      }
      listItems.push(`<li>${parseInlineMarkdown(ulMatch[1])}</li>`);
      continue;
    }

    const olMatch=line.match(/^\d+\.\s+(.+)$/);
    if(olMatch){
      if(!inList||listType!=='ol'){
        flushList();
        inList=true;listType='ol';
      }
      listItems.push(`<li>${parseInlineMarkdown(olMatch[1])}</li>`);
      continue;
    }

    flushList();
    out.push(`<p>${parseInlineMarkdown(line)}</p>`);
  }

  flushList();
  flushTable();
  if(inCode){
    const rawCode=codeLines.join('\n');
    out.push(`<div class="md-code-block"><pre><code>${highlightCode(rawCode, codeLang)}</code></pre></div>`);
  }
  return out.join('');
}

function parseInlineMarkdown(text){
  let s=escapeHTML(text);
  s=s.replace(/`([^`]+)`/g, '<code>$1</code>');
  s=s.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  s=s.replace(/__([^_]+)__/g, '<strong>$1</strong>');
  s=s.replace(/\*([^*]+)\*/g, '<em>$1</em>');
  s=s.replace(/_([^_]+)_/g, '<em>$1</em>');
  s=s.replace(/~~([^~]+)~~/g, '<del>$1</del>');
  s=s.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (match, label, url)=>{
    const cleanUrl=safeURL(url.trim());
    return `<a href="${escapeHTML(cleanUrl)}" target="_blank" rel="noopener noreferrer">${label}</a>`;
  });
  return s;
}

function isMarkdownArtifact(artifact, content){
  const name=String(artifact?.name||'').toLowerCase();
  const path=String(artifact?.path||'').toLowerCase();
  const type=String(artifact?.mediaType||'').toLowerCase();
  if(name.endsWith('.md') || name.endsWith('.markdown') || path.endsWith('.md') || path.endsWith('.markdown') || type==='text/markdown'){
    return true;
  }
  if(typeof content==='string'){
    if(/^\s*#+\s+/m.test(content) || /\|[^\n]+\|\n\|[-:| ]+\|/.test(content)){
      return true;
    }
  }
  return false;
}

let currentCodeEditorSave = null;
function openCodeEditorModal({ title, subtitle, language='shell', value='', onSave }){
  const modal=$('#codeEditorModal');
  if(!modal)return;
  currentCodeEditorSave=onSave;

  $('#codeEditorTitle').textContent=title||'代码编辑';
  $('#codeEditorSubtitle').textContent=subtitle||'编辑脚本与配置';
  const langSelect=$('#codeEditorLang');
  langSelect.value=language==='python'?'python':language==='plaintext'?'plaintext':'shell';
  $('#codeEditorBadge').textContent=langSelect.value.toUpperCase();

  const textarea=$('#codeEditorTextarea');
  const highlight=$('#codeEditorHighlight');
  const gutter=$('#codeEditorGutter');
  const stats=$('#codeEditorStats');
  const card=$('.code-editor-card');
  card?.classList.remove('fullscreen');

  textarea.value=value;

  function updateView(){
    const text=textarea.value;
    const lines=text.split('\n');
    const lineCount=lines.length;

    gutter.innerHTML=Array.from({length:lineCount}, (_,i)=>i+1).join('\n');
    const lang=langSelect.value;
    highlight.innerHTML=highlightCode(text, lang)+'\n';

    const selStart=textarea.selectionStart||0;
    const beforeCursor=text.slice(0, selStart);
    const curLine=beforeCursor.split('\n').length;
    const curCol=selStart - beforeCursor.lastIndexOf('\n');
    stats.textContent=`行 ${curLine}，列 ${curCol} · 共 ${lineCount} 行 · ${text.length} 字符`;
  }

  function syncScroll(){
    highlight.scrollTop=textarea.scrollTop;
    highlight.scrollLeft=textarea.scrollLeft;
    gutter.scrollTop=textarea.scrollTop;
  }

  textarea.oninput=updateView;
  textarea.onscroll=syncScroll;
  textarea.onclick=textarea.onkeyup=updateView;

  langSelect.onchange=()=>{
    $('#codeEditorBadge').textContent=langSelect.value.toUpperCase();
    updateView();
  };

  updateView();
  modal.classList.add('open');
  setTimeout(()=>{
    textarea.focus();
    syncScroll();
  }, 50);
}
function lines(value){return String(value||'').split('\n').map(v=>v.trim()).filter(Boolean)}
function escapeHTML(v){return String(v??'').replace(/[&<>'"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c]))}
function clarifyOptionLabel(opt){
  if(opt==null)return '';
  if(typeof opt==='object'){
    if(Array.isArray(opt))return '';
    return clarifyOptionLabel(opt.label||opt.text||opt.value||opt.title||opt.question||opt.name||'');
  }
  const text=String(opt).trim();
  if(!text)return '';
  if(/^[\[{]/.test(text)){
    try{const inner=JSON.parse(text);const label=clarifyOptionLabel(inner);if(label)return label;}catch{}
    return '';
  }
  return text.length>120?`${text.slice(0,120)}…`:text;
}
function clarifyQuestionOptions(q){
  const raw=q&&q.options;
  let list=[];
  if(Array.isArray(raw))list=raw;
  else if(typeof raw==='string'&&raw.trim()){
    try{const parsed=JSON.parse(raw);list=Array.isArray(parsed)?parsed:[raw];}catch{list=[raw];}
  }
  return list.map(clarifyOptionLabel).filter(Boolean);
}
function safeURL(value){try{const url=new URL(String(value||''),location.origin);return ['http:','https:'].includes(url.protocol)?url.href:'#'}catch{return '#'}}
function safeColor(value){return /^#[0-9a-f]{3,8}$/i.test(String(value||''))?value:'#74829a'}
function uid(prefix){return `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2,6)}`}
const PRODUCTION_SOPS=new Set(['issue-fix-production','requirement-development-production']);
function catalogOwned(projectId){return !!state.projectFilter&&projectId===state.projectFilter}
function catalogForeign(projectId){return !!projectId&&projectId!==state.projectFilter}
function catalogShared(projectId,shared){return !projectId||(!!shared&&projectId!==state.projectFilter)}
function catalogVisible(projectId,shared){return catalogOwned(projectId)||catalogShared(projectId,shared)}
// Generic templates are platform-provided and project independent: every
// project sees them, but they are read-only blueprints until adopted.
function isPlatformAdmin(){return !!(state.currentUser&&!state.currentUser.isGuest&&state.currentUser.isPlatformAdmin)}
function canEditGeneric(wf){return catalogGeneric(wf)&&isPlatformAdmin()}
function catalogGeneric(item){return !!item?.generic}
// Generic roles live in generic_templates, but the first project that
// adopted a template often reused the same role id. Looking the wizard
// subject up in the project catalog then marks that role as foreign and
// hides Save in favour of「复制到本项目」.
function agentWizardSubject(){
  if(catalogGeneric(state.workflow))return (state.genericTemplateAgents||[]).find(x=>x.id===state.selectedAgent)||state.agentDraft||{};
  return (state.data.agents||[]).find(x=>x.id===state.selectedAgent)||state.agentDraft||{};
}
function agentWizardForeign(a){
  if(catalogGeneric(state.workflow))return !isPlatformAdmin();
  return catalogForeign(a?.projectId);
}
function genericTemplateMCPOptions(selected){
  const wanted=['github'];
  const out=[];
  const seen=new Set();
  const add=(id,name,description)=>{
    if(!id||seen.has(id))return;
    seen.add(id);
    out.push({id,name:name||id,description:description||'通用模板按市场别名绑定，运行时解析到当前项目已安装的 MCP'});
  };
  for(const item of state.mcpMarket||[]){
    const alias=(item.aliases||[]).find(key=>wanted.includes(key));
    if(alias||wanted.includes(item.id))add(alias||item.id,item.name,item.description);
  }
  for(const id of selected||[]){
    if(seen.has(id))continue;
    const item=(state.mcpMarket||[]).find(m=>m.id===id||(m.aliases||[]).includes(id));
    add(id,item?.name,item?.description);
  }
  return out;
}
function genericMCPChecked(id,selected){
  if((selected||[]).includes(id))return true;
  const item=(state.mcpMarket||[]).find(m=>m.id===id||(m.aliases||[]).includes(id));
  if(!item)return false;
  return (selected||[]).some(s=>s===item.id||(item.aliases||[]).includes(s));
}
function assetVisible(item){return !item?.projectId||item.projectId===state.projectFilter}
function catalogWorkflows(){return (state.data?.workflows||[]).filter(w=>!catalogGeneric(w)&&catalogVisible(w.projectId,w.shared))}
// Runnable options = stored SOPs + platform generic templates (run by reference).
function runnableWorkflowOptions(){return catalogWorkflows().concat((state.data?.workflows||[]).filter(catalogGeneric).map(w=>({...w,name:`通用 · ${w.name}`})))}
function scopedWorkflows(){
  const all=state.data?.workflows||[];
  if(state.sopScope==='generic')return all.filter(w=>catalogGeneric(w));
  if(state.sopScope==='shared')return all.filter(w=>!catalogGeneric(w)&&catalogShared(w.projectId,w.shared));
  return all.filter(w=>!catalogGeneric(w)&&catalogOwned(w.projectId));
}
function catalogAgents(){return (state.data?.agents||[]).filter(a=>catalogVisible(a.projectId,a.shared))}
function workflowAgents(){
  if(catalogGeneric(state.workflow))return state.genericTemplateAgents||[];
  const projectId=state.workflow?.projectId||state.projectFilter||'';
  return (state.data?.agents||[]).filter(a=>!!a.projectId&&(a.projectId===projectId||(a.shared&&a.projectId!==projectId)));
}
function workflowAgent(id){return workflowAgents().find(a=>a.id===id)}
function clearGenericTemplateAgents(){state.genericTemplateAgents=[];state.genericTemplateAgentWorkflowId=''}
async function hydrateGenericWorkflowRoles(wf=state.workflow){
  if(!catalogGeneric(wf)){
    if(state.genericTemplateAgentWorkflowId)clearGenericTemplateAgents();
    return false;
  }
  if(state.genericTemplateAgentWorkflowId===wf.id)return false;
  const id=wf.id;
  try{
    const agents=await api(`/api/workflows/${encodeURIComponent(id)}/agents`);
    if(state.workflow?.id!==id)return false;
    state.genericTemplateAgents=Array.isArray(agents)?agents:[];
    state.genericTemplateAgentWorkflowId=id;
    return true;
  }catch(e){
    // 不要把失败记成「已加载」：那个标记是用来跳过重复请求的，
    // 一旦写上，这个模板就再也不会重拉，门禁会一直显示成没有配置。
    if(state.workflow?.id===id)clearGenericTemplateAgents();
    toast(e.message||'加载通用模板角色失败');
    return false;
  }
}
async function refreshGenericWorkflowRoles(){
  if(await hydrateGenericWorkflowRoles()){
    renderCanvas();
    renderProperties();
  }
}
function scopedAgents(){const all=state.data?.agents||[];return state.agentScope==='shared'?all.filter(a=>catalogShared(a.projectId,a.shared)):all.filter(a=>catalogOwned(a.projectId))}
function visibleAssets(items){return (items||[]).filter(assetVisible)}
function visibleMCPAssets(items){const uid=state.currentUser&&!state.currentUser.isGuest?Number(state.currentUser.id):0;return (items||[]).filter(item=>item.scope==='project'?item.projectId===state.projectFilter:item.scope==='personal'&&!item.marketId&&uid>0&&Number(item.ownerUserId)===uid)}
function projectQuery(){return state.projectFilter?`?projectId=${encodeURIComponent(state.projectFilter)}`:''}
function projectQueryWith(extra){const q=projectQuery();return q?q+`&${extra}`:`?${extra}`}
function runProjectId(r){
  if(r?.executionContext?.projectId)return r.executionContext.projectId;
  const issue=runContext(r).issue;
  if(issue?.projectId)return issue.projectId;
  const rid=r?.repository?.repositoryId;
  if(rid){const repo=(state.data.repositories||[]).find(x=>x.id===rid);if(repo?.projectId)return repo.projectId}
  return '';
}
function scopedRuns(runs){const pid=state.projectFilter,list=runs||[];return pid?list.filter(r=>runProjectId(r)===pid):list}
function preferredWorkflow(){
  const usable=catalogWorkflows(), mine=usable.filter(w=>catalogOwned(w.projectId));
  return mine.find(w=>w.id==='issue-fix-production')||mine[0]||usable.find(w=>w.id==='issue-fix-production')||usable[0]||null;
}
function syncWorkflowToProject(){
  if(state.workflow&&!catalogGeneric(state.workflow)&&catalogVisible(state.workflow.projectId,state.workflow.shared))return;
  state.workflow=clone(preferredWorkflow());
  state.workflowSaved=clone(state.workflow);
  state.workflowDirty=false;
  rememberOpenWorkflow();
}
function isStaleRun(r){if(r?.status!=='running')return false;const t=new Date(r.updatedAt||r.createdAt||0).getTime();return !t||(Date.now()-t)>4*60*1000}
function isGhostRun(r){if(!['failed','cancelled'].includes(r?.status))return false;return Object.values(r?.nodes||{}).some(n=>n?.status==='running'||n?.status==='pending_input')}
function liveRuns(){return scopedRuns((state.data?.runs||[]).filter(r=>r.status==='waiting'||(r.status==='running'&&!isStaleRun(r))||isGhostRun(r)))}
function staleRuns(){return scopedRuns((state.data?.runs||[]).filter(isStaleRun))}
function currentProject(){return (state.data.projects||[]).find(p=>p.id===state.projectFilter)||(state.data.projects||[])[0]}
function currentRepository(project=currentProject()){const id=project?.defaultRepositoryId;return (state.data.repositories||[]).find(r=>r.id===id)}
function userById(id){return (state.data.users||[]).find(u=>Number(u.id)===Number(id))}
function userDisplay(user){return user?(user.displayName&&user.loginName?`${user.loginName}(${user.displayName})`:user.loginName||user.displayName||`用户 ${user.id}`):''}
function projectMembers(projectId){return ((state.data.projects||[]).find(p=>p.id===projectId)||{}).members||[]}
function issueAssigneeName(issue){
	if(issue?.assigneeUserId)return userDisplay(userById(issue.assigneeUserId));
	if(!issue?.assigneeId)return '';
  if(issue.assigneeType==='agent')return (state.data.agents||[]).find(a=>a.id===issue.assigneeId)?.name||'';
  return '';
}
function runIssueTitle(r){const task=(state.data.tasks||[]).find(t=>t.runId===r.id);const issue=task&&(state.data.issues||[]).find(i=>i.id===task.issueId);if(issue?.title)return issue.title;const line=String(r.input||'').split('\n')[0].trim();return line||r.workflowSnapshot?.name||r.workflowId||r.id}

function isLoopbackHost(hostname){
  return hostname==='localhost'||hostname==='127.0.0.1'||hostname==='[::1]'||hostname==='0.0.0.0';
}
// 登录框有两种模式：登录，以及开放注册时的注册。首次启动（还没有任何账号）
// 则是创建管理员，见 openSetupScreen。
function openLoginModal({forced=false,mode='login'}={}){
  let modal=$('#loginModal');
  if(!modal){
    modal=document.createElement('div');
    modal.id='loginModal';modal.className='modal';
    modal.innerHTML=`<form id="loginForm" class="modal-card login-modal-card"><button type="button" class="modal-close login-close">×</button><p class="eyebrow">LOOPFORGE</p><h2 class="login-title">登录</h2><p class="modal-hint login-hint"></p><label>登录名</label><input name="loginName" class="input" autocomplete="username" required><div class="login-register-only"><label>显示名（可选）</label><input name="displayName" class="input" autocomplete="name"></div><label>密码</label><input name="password" class="input" type="password" autocomplete="current-password" required><p class="login-error" hidden></p><div class="modal-actions"><button type="button" class="button ghost login-close">取消</button><button type="submit" class="button primary login-submit">登录</button></div><p class="login-switch"></p></form>`;
    document.body.appendChild(modal);
    modal.querySelectorAll('.login-close').forEach(b=>b.onclick=()=>modal.classList.remove('open'));
    modal.querySelector('#loginForm').onsubmit=async e=>{
      e.preventDefault();
      const form=e.currentTarget,err=form.querySelector('.login-error'),button=form.querySelector('[type=submit]');
      const register=modal.dataset.mode==='register';
      err.hidden=true;button.disabled=true;
      try{
        const body={loginName:form.loginName.value.trim(),password:form.password.value};
        if(register)body.displayName=form.displayName.value.trim();
        const result=await api(register?'/api/auth/register':'/api/auth/login',{method:'POST',silent:true,body:JSON.stringify(body)});
        state.currentUser=result.user||null;
        location.reload();
      }catch(ex){err.textContent=loginErrorText(ex.message);err.hidden=false}
      finally{button.disabled=false}
    };
  }
  if(modal.classList.contains('open')&&modal.dataset.forced==='true'&&modal.dataset.mode===mode)return;
  const register=mode==='register'&&state.signupOpen;
  modal.dataset.forced=String(forced);
  modal.dataset.mode=register?'register':'login';
  modal.querySelectorAll('.login-close').forEach(b=>b.hidden=forced);
  modal.querySelector('#loginForm').reset();
  modal.querySelector('.login-error').hidden=true;
  modal.querySelector('.login-title').textContent=register?'注册账号':'登录';
  modal.querySelector('.login-hint').textContent=register?'注册后可以在项目里提需求、被指派为负责人，并配置自己的模型 Key。':'使用你的 LoopForge 账号登录。';
  modal.querySelector('.login-register-only').hidden=!register;
  modal.querySelector('[name=password]').autocomplete=register?'new-password':'current-password';
  modal.querySelector('[name=password]').minLength=register?8:0;
  modal.querySelector('.login-submit').textContent=register?'注册并登录':'登录';
  const sw=modal.querySelector('.login-switch');
  sw.innerHTML=register?'已有账号？<button type="button" class="link-button" data-login-mode="login">去登录</button>':state.signupOpen?'还没有账号？<button type="button" class="link-button" data-login-mode="register">注册</button>':'没有账号？请联系平台管理员创建。';
  sw.querySelectorAll('[data-login-mode]').forEach(b=>b.onclick=()=>openLoginModal({forced,mode:b.dataset.loginMode}));
  modal.classList.add('open');
  setTimeout(()=>modal.querySelector('[name=loginName]')?.focus(),50);
}
function loginErrorText(message){
  const known={'invalid login name or password':'登录名或密码不对','too many failed attempts; try again in a minute':'失败次数太多，请一分钟后再试','loginName and password are required':'请填写登录名和密码'};
  if(known[message])return known[message];
  if(/password must be at least/i.test(message||''))return '密码至少 8 位';
  if(/loginName must be/i.test(message||''))return '登录名为 1-64 个字符，不能含空格或斜杠';
  return message||'操作失败';
}
// 全新实例还没有任何账号：第一个打开页面的人创建管理员。
function openSetupScreen(){
  let modal=$('#setupModal');
  if(!modal){
    modal=document.createElement('div');
    modal.id='setupModal';modal.className='modal open';
    modal.innerHTML=`<form id="setupForm" class="modal-card login-modal-card"><p class="eyebrow">欢迎使用 LOOPFORGE</p><h2>创建管理员账号</h2><p class="modal-hint">这是这个实例的第一个账号，拥有全部管理权限，默认项目也归它。之后队友可以自己注册，开关在「平台管理」里。</p><label>登录名</label><input name="loginName" class="input" autocomplete="username" required placeholder="例如 alice"><label>显示名（可选）</label><input name="displayName" class="input" autocomplete="name"><label>密码</label><input name="password" class="input" type="password" autocomplete="new-password" required minlength="8" placeholder="至少 8 位"><label>确认密码</label><input name="confirm" class="input" type="password" autocomplete="new-password" required minlength="8"><p class="login-error" hidden></p><div class="modal-actions"><button type="submit" class="button primary">创建并进入</button></div></form>`;
    document.body.appendChild(modal);
    modal.querySelector('#setupForm').onsubmit=async e=>{
      e.preventDefault();
      const form=e.currentTarget,err=form.querySelector('.login-error'),button=form.querySelector('[type=submit]');
      err.hidden=true;
      if(form.password.value!==form.confirm.value){err.textContent='两次输入的密码不一致';err.hidden=false;return}
      button.disabled=true;
      try{
        await api('/api/auth/setup',{method:'POST',silent:true,body:JSON.stringify({loginName:form.loginName.value.trim(),displayName:form.displayName.value.trim(),password:form.password.value})});
        location.reload();
      }catch(ex){
        if(/管理员已创建/.test(ex.message||'')){location.reload();return}
        err.textContent=loginErrorText(ex.message);err.hidden=false;
      }finally{button.disabled=false}
    };
  }
  setTimeout(()=>modal.querySelector('[name=loginName]')?.focus(),50);
}

async function boot(){
  state.data = {runs:[],issues:[],tasks:[],workflowMetrics:[],syncLogs:[],agents:[],workflows:[],projects:[],repositories:[],users:[],rules:[],skills:[],mcpServers:[],executors:[],dockerfiles:[],environments:[],sandboxes:[]};
  bind(); renderProjectSwitcher(); updateApprovalBadge();

  // 1. 获取当前登录身份（本地账号 Cookie 或受信反向代理身份头）
  if (!state.currentUser) {
    try {
      const curRes = await api('/api/auth/current', {silent: true});
      state.currentUser = curRes.user || null;
      state.authRequired = !!curRes.authRequired;
      state.proxyAuth = !!curRes.proxyAuth;
      state.needsSetup = !!curRes.needsSetup;
      state.signupOpen = !!curRes.signupOpen;
    } catch (err) {
      console.warn('[auth] get current user failed:', err);
    }
  }

  // 2. 强制登录的部署里，访客直接看到登录框；全新实例先创建管理员
  if ((!state.currentUser || state.currentUser.isGuest) && state.authRequired && !state.proxyAuth) {
    if(state.needsSetup)openSetupScreen();
    else openLoginModal({forced:true});
    return;
  }
  renderUserBadge();

  const view=restoredView();
  const shell = await api('/api/bootstrap', {silent: view==='runs'});
  state.data = Object.assign(state.data, shell);
  const savedProject=localStorage.getItem('loopforge.activeProject');
  state.projectFilter=(state.data.projects||[]).some(p=>p.id===savedProject)?savedProject:(state.data.projects||[])[0]?.id||'';
  state.workflow = clone(restoreOpenWorkflow());
  state.workflowSaved = clone(state.workflow);
  state.workflowDirty = false;
  if(state.workflow?.id)localStorage.setItem('loopforge.activeWorkflow',state.workflow.id);
  const storage=state.data.storage;
  if(storage){
    const el=$('#sidebarSyncState');
    if(el){
      el.textContent=storage.driver==='mysql'?'已接 MySQL':storage.driver==='sqlite'?'本地 SQLite':`已接 ${storage.driver||'数据库'}`;
      el.title=storage.note||'';
    }
  }
  renderProjectSwitcher(); updateApprovalBadge();
  window.addEventListener('hashchange',async()=>{
    const name=restoredView();
    if(activeViewName()!==name){
      if(name==='flow')await applyFlowRoute();
      if(restoredView()===name)switchView(name);
      return;
    }
    if(name==='runs'){
      const id=runRouteId();
      if(id){if(state.selectedRun!==id)showRunDetail(id)}
      else if(state.selectedRun)showRunList();
      return;
    }
    if(name==='flow')applyFlowRoute();
    if(name==='settings'&&typeof openSettings==='function'&&routePart(1)&&routePart(1)!==state.settingsTab)openSettings(routePart(1));
  });
  renderAll();
  switchView(view);
  restoreRequirementComposer();
  if(typeof maybeStartOnboarding==='function')maybeStartOnboarding();
}
const savedTheme=localStorage.getItem('loopforge.theme.v3')||'dark';document.documentElement.dataset.theme=savedTheme;

function bind(){
	prepareMCPFormHints();
  $$('.nav[data-view]').forEach(btn=>btn.onclick=()=>switchView(btn.dataset.view));
  document.addEventListener('click',e=>{
    const wizard=e.target.closest('[data-open-setup-wizard]');
    if(wizard){e.stopPropagation();e.preventDefault();if(typeof reopenSetupWizardFromGuide==='function')reopenSetupWizardFromGuide();return}
    const jump=e.target.closest('[data-view]:not(.nav)');
    if(jump&&jump.dataset.view&&!jump.closest('.platform-sidebar')){e.stopPropagation();closeIssueDrawer();switchView(jump.dataset.view);return}
    const gotoSettings=e.target.closest('[data-goto-settings]');if(!gotoSettings)return;e.stopPropagation();closeIssueDrawer();openSettings(gotoSettings.dataset.gotoSettings);
  });
  const capabilityToggle=$('#capabilityNavToggle');if(capabilityToggle)capabilityToggle.onclick=()=>setCapabilityNavExpanded(capabilityToggle.getAttribute('aria-expanded')!=='true');
  setCapabilityNavExpanded(localStorage.getItem('loopforge.capabilityNavExpanded')==='true');
  const environmentToggle=$('#environmentNavToggle');if(environmentToggle)environmentToggle.onclick=()=>setEnvironmentNavExpanded(environmentToggle.getAttribute('aria-expanded')!=='true');
  setEnvironmentNavExpanded(localStorage.getItem('loopforge.environmentNavExpanded')==='true');
  $$('[data-nav-group]').forEach(toggle=>{
    const group=toggle.dataset.navGroup;
    toggle.onclick=()=>{
      const open=toggle.getAttribute('aria-expanded')!=='true';
      if(open&&navIsCompact())$$('[data-nav-group]').forEach(t=>t!==toggle&&setNavGroupExpanded(t.dataset.navGroup,false));
      setNavGroupExpanded(group,open);
    };
    setNavGroupExpanded(group,!navIsCompact()&&localStorage.getItem(`loopforge.navGroup.${group}`)==='true');
  });
  document.addEventListener('click',e=>{
    if(!navIsCompact()||e.target.closest('[data-nav-group]'))return;
    const child=e.target.closest('[data-nav-group-body] .nav');
    if(child||!e.target.closest('[data-nav-group-body]'))$$('[data-nav-group-body]:not([hidden])').forEach(body=>{body.hidden=true;$(`[data-nav-group="${body.dataset.navGroupBody}"]`)?.setAttribute('aria-expanded','false')});
  });
  $$('#accessTabs [data-access-tab]').forEach(btn=>btn.onclick=()=>setAccessTab(btn.dataset.accessTab));
  $('#guideCreateToken')&&($('#guideCreateToken').onclick=()=>setAccessTab('tokens',true));
  $$('[data-add]').forEach(btn=>btn.onclick=()=>addNode(btn.dataset.add));
  $('#workflowSelect').onchange=async e=>{
    if(state.workflowDirty&&!(await afConfirm({title:'未保存更改',message:'当前流程有未保存修改，切换将放弃这些修改。继续？',danger:true}))){if(state.workflow)e.target.value=state.workflow.id;return}
    state.workflow=clone((state.data.workflows||[]).find(w=>w.id===e.target.value)||null);state.selectedNode=null;markWorkflowClean();rememberOpenWorkflow();renderAll();refreshGenericWorkflowRoles();
  };
  $('#workflowName').oninput=e=>{if(!state.workflow)return;state.workflow.name=e.target.value;dirty()};
  $('#workflowOwner').oninput=e=>{if(!state.workflow)return;state.workflow.owner=e.target.value;dirty()};
  $('#workflowSandbox').onchange=e=>{if(!state.workflow)return;state.workflow.sandboxProfileId=e.target.value;dirty()};
  $('#workflowStatus').onchange=e=>{if(!state.workflow)return;state.workflow.status=e.target.value;dirty()};
  $('#workflowTags').oninput=e=>{if(!state.workflow)return;state.workflow.tags=e.target.value.split(',').map(x=>x.trim()).filter(Boolean);dirty()};
  $('#workflowMetricCategory').onchange=e=>{if(!state.workflow)return;state.workflow.metricCategory=e.target.value;dirty()};
  $('#workflowSOPLane').onchange=e=>{if(!state.workflow)return;state.workflow.sopLane=e.target.value;dirty()};
  $('#workflowDescription').oninput=e=>{if(!state.workflow)return;state.workflow.description=e.target.value;dirty()};
  $('#saveWorkflow').onclick=saveWorkflow; $('#discardWorkflow').onclick=discardWorkflow; $('#publishWorkflow').onclick=publishWorkflow;
  $('#flowHistory')&&($('#flowHistory').onclick=()=>openRevisionHistory('workflow', state.workflow?.id, state.workflow?.name));
  $('#deleteNode').onclick=deleteSelected;{const wb=$('.flow-workbench'),tg=$('#flowPropsToggle');const setProps=on=>{wb?.classList.toggle('props-hidden',!on);tg?.classList.toggle('on',on);localStorage.setItem('loopforge.flowProps',on?'1':'0');renderEdges()};setProps(localStorage.getItem('loopforge.flowProps')!=='0');tg&&(tg.onclick=()=>setProps(wb.classList.contains('props-hidden')));$('#nodes')?.addEventListener('dblclick',e=>{if(e.target.closest('.flow-node'))setProps(true)})} $('#fitButton').onclick=arrange;
  bindFlowResize();
  $('#runButton').onclick=()=>openRunModal(state.workflow?.id);
  $('#flowSettingsToggle').onclick=()=>{const panel=$('#flowSettingsPanel');panel.hidden=!panel.hidden;$('#flowSettingsToggle').classList.toggle('active',!panel.hidden)};
  const closeFlowSettings=()=>{const panel=$('#flowSettingsPanel');if(panel){panel.hidden=true;$('#flowSettingsToggle')?.classList.remove('active')}};
  $('#flowSettingsClose')?.addEventListener('click',closeFlowSettings);
  $('#flowSettingsCollapse')?.addEventListener('click',closeFlowSettings);
  $('#flowSettingsQuickSave')?.addEventListener('click',saveWorkflow);
  const sandboxChip=$('#sandboxChip');
  if(sandboxChip)sandboxChip.onclick=()=>{
    const panel=$('#flowSettingsPanel');
    if(!panel)return;
    panel.hidden=false;
    $('#flowSettingsToggle')?.classList.add('active');
    const field=$('#workflowSandboxField');
    if(field){field.classList.add('highlight');setTimeout(()=>field.classList.remove('highlight'),1600)}
    const sel=$('#workflowSandbox');
    if(sel)sel.focus();
  };
  $$('.canvas-view-toggle [data-flow-view]').forEach(btn=>btn.onclick=()=>setFlowView(btn.dataset.flowView));bindCanvasPan();
  const refreshRunnersBtn=$('#refreshRunners');if(refreshRunnersBtn)refreshRunnersBtn.onclick=()=>refreshRunners();
  $('#themeToggle').onclick=toggleTheme;renderThemeToggle();
  $('#langToggle')&&($('#langToggle').onclick=()=>window.lfSetLang&&window.lfSetLang(window.lfLang&&window.lfLang()==='en'?'zh':'en'));
  {const spotSel='.ap-card,.db-card,.db-stat,.kanban-card,.run-card,.sop-card,.hc-sop,.room-suggest,.room-bot-run,.pa-rules li,.metric-card,.catalog-card,.agent-card,.skill-card,.mcp-card';let spotEl=null;
  document.addEventListener('pointermove',e=>{const el=e.target.closest?.(spotSel)||null;if(el!==spotEl){spotEl?.classList.remove('lf-spot-on');spotEl=el;el?.classList.add('lf-spot-on')}if(el){const r=el.getBoundingClientRect();el.style.setProperty('--mx',`${e.clientX-r.left}px`);el.style.setProperty('--my',`${e.clientY-r.top}px`)}},{passive:true});
  document.addEventListener('pointerleave',()=>{spotEl?.classList.remove('lf-spot-on');spotEl=null});}
  const profileWrap = $('#userProfileWrap');
  const userBadge = $('#userBadge');
  const credentialsButton = $('#userCredentialsButton');
  if (userBadge && profileWrap) {
    const toggleProfile=()=>{profileWrap.classList.toggle('is-open');userBadge.setAttribute('aria-expanded',String(profileWrap.classList.contains('is-open')))};
    userBadge.onclick = (e) => {
      e.stopPropagation();
      toggleProfile();
    };
    if(credentialsButton)credentialsButton.onclick=e=>{e.stopPropagation();profileWrap.classList.add('is-open');userBadge.setAttribute('aria-expanded','true');setTimeout(()=>$('#userCredentialForm input')?.focus(),0)};
    userBadge.onkeydown = (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        toggleProfile();
      }
    };
    const popover = $('#userProfilePopover');
    if (popover) {
      popover.onclick = (e) => e.stopPropagation();
    }
    document.addEventListener('click', (e) => {
      if (!profileWrap.contains(e.target)) {
        profileWrap.classList.remove('is-open');
        userBadge.setAttribute('aria-expanded', 'false');
      }
    });
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') {
        profileWrap.classList.remove('is-open');
        userBadge.setAttribute('aria-expanded', 'false');
      }
    });
  }
  $('#commandPaletteTrigger').onclick=openCommandPalette;
  $('#commandPalette').onclick=e=>{if(e.target===$('#commandPalette'))closeCommandPalette()};
  $('#commandSearch').oninput=renderCommandResults;
  $('#commandResults').onclick=e=>{const item=e.target.closest('[data-command-index]');if(item)executeCommand(Number(item.dataset.commandIndex))};
  $('#commandResults').onmousemove=e=>{const item=e.target.closest('[data-command-index]');if(!item)return;const idx=Number(item.dataset.commandIndex);if(idx!==commandSelectedIndex){commandSelectedIndex=idx;highlightCommandSelection()}};
  $$('[data-issue-view]').forEach(btn=>btn.onclick=()=>setIssueView(btn.dataset.issueView));
  setIssueView(state.issueView,false);
  $$('#runModal .modal-close').forEach(b=>b.onclick=()=>$('#runModal').classList.remove('open'));
  $$('.generic-template-close').forEach(b=>b.onclick=()=>$('#genericTemplateModal').classList.remove('open'));
  $('#confirmGenericTemplate')&&($('#confirmGenericTemplate').onclick=adoptGenericTemplate);
  $('#runGenericTemplateDirect')&&($('#runGenericTemplateDirect').onclick=()=>{const id=state.genericTemplateId;$('#genericTemplateModal').classList.remove('open');if(!id)return toast('请先选择通用模板');openRunModal(id)});
  $('#confirmRun').onclick=startRun; $('#newAgent').onclick=()=>openAgentWizard();
  $('#agentForm').onsubmit=saveAgent;
  $('#agentForm').elements.executorId?.addEventListener('change',e=>applyAgentModelUI(e.target.value,'',''));
  bindAgentPicker('#agentModelPicker',renderAgentModelPicker);
  bindAgentPicker('#agentEffortPicker',renderAgentEffortPicker);
  $('#agentClone')&&($('#agentClone').onclick=()=>cloneAgent(state.selectedAgent));
  $('#agentForm').addEventListener('input',()=>renderAgentPreview(state.agentDraft));
  $('#agentForm').addEventListener('change',()=>renderAgentPreview(state.agentDraft)); $('#refreshRuns').onclick=()=>refreshRuns(false);$('#runBackBtn')&&($('#runBackBtn').onclick=()=>showRunList());
	$('#refreshIssues').onclick=refreshIssues;
	$('#refreshSOPs').onclick=refreshWorkflowMetrics;
  $('#agentSearch').oninput=renderAgents;$('#agentPrev').onclick=()=>setAgentWizardStep(state.agentWizardStep-1);$('#agentNext').onclick=()=>setAgentWizardStep(state.agentWizardStep+1);$$('[data-wizard-steps="agent"] [data-step-target]').forEach(b=>b.onclick=()=>setAgentWizardStep(Number(b.dataset.stepTarget)));$$('.agent-close').forEach(b=>b.onclick=()=>$('#agentModal').classList.remove('open'));
  $('#newRunFromRuns')&&($('#newRunFromRuns').onclick=()=>switchView('issues'));
  $('#runSearch').oninput=e=>{state.runQuery=e.target.value.trim().toLowerCase();resetListPage('runs');renderRuns()};
  $('#runWorkflowFilter').onchange=e=>{state.runWorkflowFilter=e.target.value;resetListPage('runs');renderRuns()};
  $('#runFilters').onclick=e=>{const button=e.target.closest('[data-run-filter]');if(!button)return;state.runFilter=button.dataset.runFilter;resetListPage('runs');$$('[data-run-filter]').forEach(x=>x.classList.toggle('active',x===button));renderRuns()};
  $$('[data-asset-scope]').forEach(btn=>btn.onclick=()=>{state[assetScopeKey(state.assetType)]=btn.dataset.assetScope;state.selectedAsset=null;renderAssets()});
  $('#newAsset').onclick=()=>{state.selectedAsset='__new__';renderAssets()};
  $('#assetForm').onsubmit=saveAsset;
  // 技能文件上传：事件委托绑定在持久表单节点上，表单 innerHTML 重建不影响。
  // 每次渲染直接挂 onchange 会因轮询重建表单时 input 被替换而丢事件。
  $('#assetForm').addEventListener('change',e=>{
    if(e.target.id==='skillFolderPick'||e.target.id==='skillZipPick')uploadSkillFiles(e.target);
    if(e.target.id==='skillMdPick')uploadSkillMd(e.target);
  });
  $('#assetForm').addEventListener('click',e=>{
    const previewBtn=e.target.closest('[data-skill-file-preview]');
    if(previewBtn){
      e.preventDefault();
      if(previewBtn.disabled)return;
      openSkillFilePreview(selectedSkill(), previewBtn.getAttribute('data-skill-file-preview'));
      return;
    }
    const delBtn=e.target.closest('[data-skill-file-del]');
    if(delBtn){
      e.preventDefault();
      if(skillWriteBlocked())return toast('共享资产只读');
      deleteSkillFile(delBtn.getAttribute('data-skill-file-del'));
      return;
    }
    const tree=e.target.closest('[data-tree-toggle]');
    if(tree){
      const key=tree.getAttribute('data-tree-toggle');
      if(!(state.skillTreeCollapsed instanceof Set))state.skillTreeCollapsed=new Set();
      if(state.skillTreeCollapsed.has(key))state.skillTreeCollapsed.delete(key);else state.skillTreeCollapsed.add(key);
      refreshSkillFileTree();
      return;
    }
    const folderBtn=e.target.closest('#skillFolderBtn');
    const zipBtn=e.target.closest('#skillZipBtn');
    const mdBtn=e.target.closest('#skillMdBtn');
    if(folderBtn||zipBtn||mdBtn){
      if(state.selectedAsset==='__new__'||!state.selectedAsset)return toast('先保存技能，再上传文件');
      if(skillWriteBlocked())return toast('共享资产只读');
    }
    if(folderBtn){
      const inp=$('#skillFolderPick');if(!inp)return;
      inp.click();
      // 兜底：个别场景 change 不触发，轮询 files 变化
      let seen=0;
      const poll=setInterval(()=>{
        const n=inp.files?inp.files.length:0;
        if(n>0&&n!==seen){seen=n;clearInterval(poll);uploadSkillFiles(inp)}
      },300);
      setTimeout(()=>clearInterval(poll),180000);
    }
    if(zipBtn){
      const inp=$('#skillZipPick');if(!inp)return;
      inp.click();
      let seen=0;
      const poll=setInterval(()=>{
        const n=inp.files?inp.files.length:0;
        if(n>0&&n!==seen){seen=n;clearInterval(poll);uploadSkillFiles(inp)}
      },300);
      setTimeout(()=>clearInterval(poll),180000);
    }
    if(mdBtn){
      const inp=$('#skillMdPick');if(!inp)return;
      inp.click();
      let seen=0;
      const poll=setInterval(()=>{
        const n=inp.files?inp.files.length:0;
        if(n>0&&n!==seen){seen=n;clearInterval(poll);uploadSkillMd(inp)}
      },300);
      setTimeout(()=>clearInterval(poll),180000);
    }
  });
  $$('[data-mcp-tab]').forEach(btn=>btn.onclick=()=>{state.mcpTab=btn.dataset.mcpTab;resetListPage('mcp');renderMCPManagement()});
  $$('[data-mcp-scope]').forEach(btn=>btn.onclick=()=>{state.mcpScope=btn.dataset.mcpScope;if(state.mcpScope==='personal')state.mcpTab='installed';resetListPage('mcp');renderMCPManagement()});
  let mcpSearchTimer=0;
  $('#mcpMarketSearch')&&($('#mcpMarketSearch').oninput=e=>{state.mcpQuery=e.target.value.trim().toLowerCase();resetListPage('mcp');clearTimeout(mcpSearchTimer);mcpSearchTimer=setTimeout(()=>renderMCPManagement(),120)});
  $('#newCustomMcp')&&($('#newCustomMcp').onclick=()=>openCustomMCPModal());
  $$('.custom-mcp-close').forEach(btn=>btn.onclick=()=>$('#customMcpModal').classList.remove('open'));
  $('#customMcpForm')&&($('#customMcpForm').onsubmit=saveCustomMCP);
  $('#customMcpForm')?.addEventListener('change',e=>{
    if(e.target.name==='transport')mcpSetTransport(e.target.value);
    if(e.target.id==='mcpFolderPick'||e.target.id==='mcpZipPick'){
      const other=$(e.target.id==='mcpFolderPick'?'#mcpZipPick':'#mcpFolderPick');if(other)other.value='';mcpUpdateFileSummary(e.target.files);
    }
  });
  $('#customMcpForm')?.addEventListener('click',async e=>{
    const add=e.target.closest('[data-mcp-kv-add]');if(add){e.preventDefault();mcpAddKVRow(add.dataset.mcpKvAdd);return}
    const del=e.target.closest('[data-mcp-kv-del]');if(del){e.preventDefault();del.closest('.kv-editor-row')?.remove();return}
    if(e.target.closest('#mcpJsonParseBtn')){e.preventDefault();try{await importMCPJSONText($('#mcpJsonInput')?.value||'')}catch(err){toast(err.message,'err')}return}
    if(e.target.closest('#mcpFolderBtn')){e.preventDefault();$('#mcpFolderPick')?.click()}
    if(e.target.closest('#mcpZipBtn')){e.preventDefault();$('#mcpZipPick')?.click()}
  });
  $('#agentHistory')&&($('#agentHistory').onclick=()=>openRevisionHistory('agent', state.selectedAgent, $('#agentForm')?.elements?.name?.value));
  $('#revisionClose')&&($('#revisionClose').onclick=closeRevisionHistory);
  $('#revisionClose2')&&($('#revisionClose2').onclick=closeRevisionHistory);
  $('#revisionRestore')&&($('#revisionRestore').onclick=restoreSelectedRevision);
  $('#newIssue').onclick=openRequirementWizard;
  $('#importIssueButton')&&($('#importIssueButton').onclick=openImportIssueModal);
  $$('.import-issue-close').forEach(b=>b.onclick=()=>$('#importIssueModal').classList.remove('open'));
  $('#importIssueForm')&&($('#importIssueForm').onsubmit=importIssueFromURL);
  $('#newScheduleJob').onclick=openScheduleJobModal;
  $('#newProject').onclick=()=>{state.projectDraft={id:'',key:'',name:'',description:'',color:'#536dfe',ownerId:state.currentUser?.id||'',workflowId:'generic-backend-delivery',defaultRepositoryId:'',members:[],clarification:defaultClarification(),runEval:defaultRunEval()};renderProjectAdmin()};
  $('#projectAdminForm').onsubmit=saveProjectAdmin;
  $$('.requirement-close').forEach(b=>b.onclick=()=>closeRequirementComposer());
  $('#chatSend').onclick=()=>sendChatClarify(!!state.composerClarifyRetry);
  $('#chatStop').onclick=()=>abortChatClarify();
  $('#chatCreateDirect')&&($('#chatCreateDirect').onclick=()=>createIssueDirect());
  $('#chatCreateLocal')&&($('#chatCreateLocal').onclick=()=>createIssueFromChat('local'));
  $('#chatCreateGitHub').onclick=()=>createIssueFromChat('github');
  $('#chatInput').addEventListener('keydown',e=>{if(e.key==='Enter'&&!e.shiftKey){if(chatHistory.length>0){e.preventDefault();sendChatClarify()}}});
  $('#reqWorkflow')&&($('#reqWorkflow').onchange=()=>persistRequirementComposer());
  $('#chatInput').addEventListener('input',()=>{syncComposerReplyState();persistRequirementComposer()});
  window.addEventListener('beforeunload',()=>persistRequirementComposer());
  if($('#sopCreateForm')){$('#sopCreateForm').onsubmit=createSOPFromWizard;$('#sopPrev').onclick=()=>setSOPWizardStep(state.sopWizardStep-1);$('#sopNext').onclick=()=>setSOPWizardStep(state.sopWizardStep+1);$$('[data-wizard-steps="sop"] [data-step-target]').forEach(b=>b.onclick=()=>setSOPWizardStep(Number(b.dataset.stepTarget)));$$('.sop-create-close').forEach(b=>b.onclick=()=>$('#sopCreateModal').classList.remove('open'))}
  $('#newSOP')&&($('#newSOP').onclick=openSOPWizard);
  $('#newSOPFromFlow')&&($('#newSOPFromFlow').onclick=openSOPWizard);
  $('#sopHealthFilter').onchange=renderSOPCatalog;$('#sopSort').onchange=renderSOPCatalog;
  $('#manageRepositories').onclick=openRepositoryModal;
  $$('.repository-close').forEach(b=>b.onclick=()=>$('#repositoryModal').classList.remove('open'));
  $('#repositoryForm').onsubmit=saveRepository;
  $('#sourceFilter').onchange=e=>{state.sourceFilter=e.target.value;resetListPage('issues');renderIssues()};
  $('#issueStatusFilter')&&($('#issueStatusFilter').onchange=e=>{state.issueStatusFilter=e.target.value;resetListPage('issues');renderIssues()});
  $('#issueAssigneeFilter')&&($('#issueAssigneeFilter').onchange=e=>{state.issueAssigneeFilter=e.target.value;resetListPage('issues');renderIssues()});
  $('#issueRunFilter').onchange=e=>{state.issueRunFilter=e.target.value;resetListPage('issues');renderIssues()};
  $('#issueSearch').oninput=e=>{state.issueQuery=e.target.value.trim().toLowerCase();resetListPage('issues');renderIssues()};
  $$('.issue-close').forEach(b=>b.onclick=()=>$('#issueModal').classList.remove('open'));
  $('#issueForm').onsubmit=saveIssue;
  $('#dispatchIssue').onclick=dispatchIssue;
  $('#syncNowBtn').onclick=syncNow;
  $('#saveSyncConfig').onclick=saveSyncConfig;
  $('#scheduleJobForm').onsubmit=saveScheduleJob;
  $('#scheduleJobReset').onclick=()=>{resetScheduleJobForm();$('#scheduleJobForm').elements.name.focus()};
  $('#scheduleKindTabs')?.addEventListener('click',e=>{
    const tab=e.target.closest('[data-job-kind]');
    if(!tab)return;
    $('#scheduleJobForm').elements.jobKind.value=tab.dataset.jobKind;
    syncScheduleJobKind();
  });
  $('#scheduleTimingTabs')?.addEventListener('click',e=>{
    const tab=e.target.closest('[data-schedule-tab]');
    if(!tab)return;
    const f=$('#scheduleJobForm');
    f.elements.scheduleMode.value=tab.dataset.scheduleTab;
    syncScheduleJobTiming();
    if(f.elements.scheduleMode.value==='cron')f.elements.cron.focus();
    else f.elements.intervalSec.focus();
  });
  bindScheduleModelPicker();
  bindClarifyModelPicker($('#reqClarifyModelPicker'));
  $('#pushToForge').onclick=()=>pushIssue('github');
  $('#openSyncModal').onclick=()=>{$('#syncModal').classList.add('open');refreshSyncStatus()};
  $$('.sync-close').forEach(b=>b.onclick=()=>$('#syncModal').classList.remove('open'));
  $$('.schedule-close').forEach(b=>b.onclick=()=>$('#scheduleModal').classList.remove('open'));
  $('#syncAutoEnabledInline').onchange=e=>{$('#syncAutoEnabled').checked=e.target.checked;saveSyncConfig()};
  $$('.artifact-close').forEach(b=>b.onclick=()=>$('#artifactModal').classList.remove('open'));
  $$('.modal').forEach(modal=>modal.addEventListener('pointerdown',e=>{
    if(e.target!==modal)return;
    if(modal.id==='requirementModal')return;
    modal.classList.remove('open');
  }));
  document.addEventListener('keydown',e=>{
    if((e.metaKey||e.ctrlKey)&&e.key.toLowerCase()==='k'){e.preventDefault();openCommandPalette();return}
    if(!$('#commandPalette').hidden){
      if(e.key==='Escape'){closeCommandPalette();return}
      if(e.key==='ArrowDown'){e.preventDefault();moveCommandSelection(1);return}
      if(e.key==='ArrowUp'){e.preventDefault();moveCommandSelection(-1);return}
      if(e.key==='Tab'){e.preventDefault();if(typeof cycleCommandFilter==='function')cycleCommandFilter();return}
      if(e.key==='Enter'){e.preventDefault();executeCommand(commandSelectedIndex);return}
    }
    if(e.key==='Escape'){
      const login=$('#loginModal');
      if(login?.classList.contains('open')){if(login.dataset.forced!=='true')login.classList.remove('open');return}
      if(state.connectFrom||state.connectCursor){state.connectFrom=null;state.connectCursor=null;renderCanvas();renderProperties();return}
      if(state.drawerOpen)return closeIssueDrawer();
      if($('#requirementModal')?.classList.contains('open')){closeRequirementComposer();return}
      const open=[...$$('.modal.open')].pop();
      if(open)return open.classList.remove('open');
      const panel=$('#flowSettingsPanel');
      if(panel&&!panel.hidden){panel.hidden=true;$('#flowSettingsToggle')?.classList.remove('active');return}
    }
    if(e.metaKey||e.ctrlKey||e.altKey||e.target.closest?.('input,textarea,select,[contenteditable="true"]')||$('.modal.open'))return;
    if(e.key.toLowerCase()==='a'){
      const scope=state.drawerOpen?$('#issueDetailBody'):activeViewName()==='approvals'?$('#approvalList'):activeViewName()==='runs'?$('#runDetail'):null;
      const approve=scope?.querySelector('[data-approve]');
      if(approve){e.preventDefault();approve.click()}
    }
  });

  // --- 工作台 / 待办验收 / 可观测与分析 / Issue 详情抽屉 ---
  $('.platform-sidebar .brand').onclick=()=>switchView('chat');
  $$('[data-jump]').forEach(el=>el.onclick=()=>switchView(el.dataset.jump));
  $('#refreshApprovals').onclick=()=>renderApprovals(false);
  $('#refreshObs').onclick=refreshObs;
  $('#refreshMetrics').onclick=()=>refreshMetrics(true);
  $('#exportObsReport').onclick=exportObsReport;
  $$('.obs-tab').forEach(btn=>btn.onclick=()=>switchObsTab(btn.dataset.obsTab));
  $$('.drawer-tab').forEach(btn=>btn.onclick=()=>{state.issueDetailTab=btn.dataset.dtab;$$('.drawer-tab').forEach(x=>x.classList.toggle('active',x===btn));renderIssueDetailTab()});
  const issueDrawer=$('#issueDetailDrawer');
  if(issueDrawer&&issueDrawer.parentElement!==document.body)document.body.appendChild(issueDrawer);
  $$('[data-close-drawer]').forEach(el=>el.onclick=closeIssueDrawer);
  $('#issueDetailChat')&&($('#issueDetailChat').onclick=()=>{const id=state.selectedIssue;closeIssueDrawer();if(typeof homeChatMentionIssue==='function')homeChatMentionIssue(id)});
  $('#projectSwitcher').onchange=e=>{
    state.projectFilter=e.target.value;state.projectDraft=null;state.selectedIssue=null;
    invalidateMetricsForProjectChange();
    localStorage.setItem('loopforge.activeProject',state.projectFilter);
    resetListPages();
    syncWorkflowToProject();
    renderProjectSwitcher();renderIssues();renderSync();renderProjectAdmin();
    renderSOPCatalog();renderAgents();renderAssets();renderMCPManagement();renderRuns();renderWorkflowMeta();
    renderApprovals(true);
    if($('#chatView').classList.contains('active')&&typeof renderHomeRunsPanel==='function')renderHomeRunsPanel();
    if($('#observabilityView').classList.contains('active'))refreshObs();
    if($('#knowledgeView')?.classList.contains('active')&&typeof renderKnowledgeView==='function')renderKnowledgeView();
    if($('#metricsView').classList.contains('active'))refreshMetrics();
    if($('#sopsView').classList.contains('active'))refreshWorkflowMetrics();
  };
  document.querySelectorAll('[data-scope-for]').forEach(bar=>{
    bar.querySelectorAll('[data-scope]').forEach(btn=>btn.onclick=()=>{
      const key=bar.dataset.scopeFor==='agent'?'agentScope':'sopScope';
      state[key]=btn.dataset.scope;
      if(key==='agentScope')renderAgents();else renderSOPCatalog();
    });
  });
  bindGlobalModals();
}
function bindGlobalModals(){
  const confirmModal=$('#confirmModal');
  if(confirmModal){
    $('#confirmOkBtn')?.addEventListener('click', ()=>{
      if(confirmPromiseResolve) confirmPromiseResolve(true);
    });
    $('#confirmCancelBtn')?.addEventListener('click', ()=>{
      if(confirmPromiseResolve) confirmPromiseResolve(false);
    });
    confirmModal.addEventListener('click', (e)=>{
      if(e.target===confirmModal){
        if(confirmPromiseResolve) confirmPromiseResolve(false);
      }
    });
    $('#confirmPromptInput')?.addEventListener('keydown', (e)=>{
      if(e.key==='Enter'){
        e.preventDefault();
        if(confirmPromiseResolve) confirmPromiseResolve(true);
      } else if(e.key==='Escape'){
        e.preventDefault();
        if(confirmPromiseResolve) confirmPromiseResolve(false);
      }
    });
  }

  const codeModal=$('#codeEditorModal');
  if(codeModal){
    const textarea=$('#codeEditorTextarea');
    const closeBtn=$('#codeEditorClose');
    const cancelBtn=$('#codeEditorCancel');
    const saveBtn=$('#codeEditorSave');
    const copyBtn=$('#codeEditorCopy');
    const fullscreenBtn=$('#codeEditorFullscreen');

    const closeEditor=()=>{
      codeModal.classList.remove('open');
      currentCodeEditorSave=null;
    };
    closeBtn?.addEventListener('click', closeEditor);
    cancelBtn?.addEventListener('click', closeEditor);
    codeModal.addEventListener('click', (e)=>{
      if(e.target===codeModal) closeEditor();
    });

    copyBtn?.addEventListener('click', ()=>{
      if(textarea) copyText(textarea.value, '代码已复制到剪贴板');
    });

    fullscreenBtn?.addEventListener('click', ()=>{
      const card=codeModal.querySelector('.code-editor-card');
      if(card){
        card.classList.toggle('fullscreen');
        fullscreenBtn.textContent=card.classList.contains('fullscreen')?'缩回窗口':'⛶ 全屏';
      }
    });

    saveBtn?.addEventListener('click', ()=>{
      if(currentCodeEditorSave&&textarea){
        const lang=$('#codeEditorLang')?.value||'shell';
        currentCodeEditorSave(textarea.value, lang);
      }
      closeEditor();
      toast('代码已保存并应用', 'ok');
    });

    if(textarea){
      textarea.addEventListener('keydown', (e)=>{
        if(e.key==='Tab'){
          e.preventDefault();
          const start=textarea.selectionStart;
          const end=textarea.selectionEnd;
          const val=textarea.value;
          if(e.shiftKey){
            const lineStart=val.lastIndexOf('\n', start-1)+1;
            if(val.slice(lineStart, lineStart+2)==='  '){
              textarea.value=val.slice(0, lineStart)+val.slice(lineStart+2);
              textarea.selectionStart=Math.max(lineStart, start-2);
              textarea.selectionEnd=Math.max(lineStart, end-2);
              textarea.dispatchEvent(new Event('input'));
            }
          } else {
            textarea.value=val.substring(0, start)+'  '+val.substring(end);
            textarea.selectionStart=textarea.selectionEnd=start+2;
            textarea.dispatchEvent(new Event('input'));
          }
        } else if(e.key==='Enter'){
          const start=textarea.selectionStart;
          const val=textarea.value;
          const lastNewline=val.lastIndexOf('\n', start-1);
          const prevLine=val.slice(lastNewline+1, start);
          const match=prevLine.match(/^(\s+)/);
          let indent=match?match[1]:'';
          if(prevLine.trim().endsWith(':') || prevLine.trim().endsWith('then') || prevLine.trim().endsWith('do')){
            indent+='  ';
          }
          if(indent){
            e.preventDefault();
            textarea.value=val.substring(0, start)+'\n'+indent+val.substring(textarea.selectionEnd);
            textarea.selectionStart=textarea.selectionEnd=start+1+indent.length;
            textarea.dispatchEvent(new Event('input'));
          }
        } else if((e.metaKey||e.ctrlKey) && (e.key==='s'||e.key==='S')){
          e.preventDefault();
          saveBtn?.click();
        } else if(e.key==='Escape'){
          e.preventDefault();
          closeEditor();
        }
      });
    }
  }
}
const pluginViews=new Map();
function viewTitles(){return {...Object.fromEntries([...pluginViews.values()].map(v=>[v.id,v.title])),chat:'首页',issues:'SOP 运行',rooms:'群聊',runs:'运行',metrics:'运行洞察',approvals:'待我确认',sops:'流程',flow:'流程设计',agents:'Bot',mcp:'MCP',rules:'规则',skills:'技能',dockerfiles:'Dockerfile',environments:'安装脚本',sandboxes:'环境',observability:'成本',projects:'项目',businessDomains:'业务领域',platform:'成员与权限',businessDomainManagement:'领域管理',runners:'执行机器',settings:'个人设置',access:'接入方式',guide:'接入指南',knowledge:'知识库'}}
function sidebarViewFor(name){
  const map={runs:'issues',approvals:'issues',flow:'sops',skills:'agents',mcp:'agents',rules:'agents',dockerfiles:'sandboxes',environments:'sandboxes',businessDomains:'metrics',businessDomainManagement:'platform',runners:'settings',guide:'settings',access:'settings',build:'sops',me:'settings',admin:'platform',insights:'metrics'};
  return map[name]||name;
}
function lfModelReady(){
  const stored=(typeof userById==='function'?userById(state.currentUser?.id):null)||{};
  return !!(stored.codeBuddyApiKeyConfigured||stored.codexApiKeyConfigured||stored.claudeCodeApiKeyConfigured);
}
function lfBotQueue(agentId){
  let n=0;
  (state.data?.runs||[]).forEach(r=>{
    if(!['running','queued','waiting'].includes(r.status))return;
    (r.workflowSnapshot?.nodes||[]).forEach(node=>{
      if(node.agentId!==agentId)return;
      const st=r.nodes?.[node.id]?.status;
      if(st==='running'||st==='pending'||st==='queued')n++;
    });
  });
  if(typeof roomState==='function'){
    (roomState().lastMessages||[]).forEach(m=>{(m.crew?.tasks||[]).forEach(t=>{if(t.agentId===agentId&&(t.status==='running'||t.status==='pending'))n++})});
  }
  return n;
}
function lfBotStatus(agent){
  if(!agent)return {key:'off',label:'不可用',reason:'找不到这个 Bot',queue:0};
  const live=(state.botStatus||[]).find(item=>item.agentId===agent.id);
  if(live){
    const key=live.status==='online'?'ok':live.status==='busy'?'busy':live.status==='degraded'?'wait':'off';
    const label=live.status==='online'?'可用':live.status==='busy'?'忙':live.status==='degraded'?'降级':(/key/i.test(live.reason||'')?'缺 Key':'不可用');
    return {key,label,reason:live.reason||label,queue:live.queue||0};
  }
  if(!lfModelReady())return {key:'off',label:'缺 Key',reason:'还没配置模型 API Key',queue:0};
  const queue=lfBotQueue(agent.id);
  if(queue>0)return {key:'busy',label:'忙',reason:'排队 '+queue,queue};
  return {key:'ok',label:'可用',reason:'可直接 @',queue:0};
}
function lfStatusBadge(agent){
  const st=lfBotStatus(agent);
  const extra=st.key==='busy'&&st.queue?' '+st.queue:'';
  return `<span class="lf-status lf-status-${st.key}" title="${escapeHTML(st.reason)}"><i></i>${escapeHTML(st.label+extra)}</span>`;
}
function activeViewName(){if(state.currentView)return state.currentView;const el=document.querySelector('.view.active');return el?el.id.replace(/View$/,''):''}
function setCapabilityNavExpanded(expanded){const toggle=$('#capabilityNavToggle'),group=$('#capabilityNavGroup');if(!toggle||!group)return;toggle.setAttribute('aria-expanded',String(expanded));group.hidden=!expanded;localStorage.setItem('loopforge.capabilityNavExpanded',String(expanded))}
function setNavGroupExpanded(group,expanded){const toggle=$(`[data-nav-group="${group}"]`),body=$(`[data-nav-group-body="${group}"]`);if(!toggle||!body)return;toggle.setAttribute('aria-expanded',String(expanded));body.hidden=!expanded;localStorage.setItem(`loopforge.navGroup.${group}`,String(expanded))}
function navIsCompact(){return window.matchMedia('(max-width: 960px)').matches}
function syncNavGroups(name){$$('[data-nav-group-body]').forEach(body=>{const group=body.dataset.navGroupBody,owns=!!body.querySelector(`.nav[data-view="${name}"]`);$(`[data-nav-group="${group}"]`)?.classList.toggle('child-active',owns);if(!navIsCompact())setNavGroupExpanded(group,owns)})}
function setEnvironmentNavExpanded(expanded){const toggle=$('#environmentNavToggle'),group=$('#environmentNavGroup');if(!toggle||!group)return;toggle.setAttribute('aria-expanded',String(expanded));group.hidden=!expanded;localStorage.setItem('loopforge.environmentNavExpanded',String(expanded))}
function restoredView(){
  const titles=viewTitles();
  const hash=(location.hash||'').replace(/^#\/?/,'').split(/[/?]/)[0];
  if(hash==='access')return 'guide';
  if(hash==='dashboard')return 'chat';
  if(hash==='assets')return 'dockerfiles';
  if(hash==='playbooks')return 'sops';
  if(titles[hash])return hash;
  const saved=localStorage.getItem('loopforge.activeView');
  if(saved==='playbooks')return 'sops';
  if(titles[saved])return saved;
  return 'chat';
}
function routePart(index){
  const raw=(location.hash||'').replace(/^#\/?/,'').split('?')[0];
  const parts=raw.split('/').filter(Boolean);
  const part=parts[index]||'';
  try{return decodeURIComponent(part)}catch{return part}
}
function runRouteId(){
  return routePart(0)==='runs'?routePart(1):'';
}
function flowRouteId(){
  return routePart(0)==='flow'?routePart(1):'';
}
// Only SOPs visible in the active project can be restored from the URL or localStorage.
function openableWorkflow(id){return id?catalogWorkflows().find(w=>w.id===id)||null:null}
function savedWorkflowId(){
  const id=state.workflow?.id||'';
  return id&&(state.data?.workflows||[]).some(w=>w.id===id)?id:'';
}
function flowViewHash(){const id=savedWorkflowId();return id?`#/flow/${encodeURIComponent(id)}`:'#/flow'}
function restoreOpenWorkflow(){
  return openableWorkflow(flowRouteId())||openableWorkflow(localStorage.getItem('loopforge.activeWorkflow')||'')||preferredWorkflow();
}
function rememberOpenWorkflow(){
  const id=savedWorkflowId();
  if(id)localStorage.setItem('loopforge.activeWorkflow',id);
  if((state.currentView||activeViewName())!=='flow')return;
  const next=flowViewHash();
  if(location.hash!==next)history.replaceState(null,'',next);
}
async function applyFlowRoute(){
  const id=flowRouteId();
  if(!id||state.workflow?.id===id)return;
  const wf=openableWorkflow(id);
  if(!wf){rememberOpenWorkflow();return}
  if(state.workflowDirty&&!(await afConfirm({title:'未保存更改',message:'当前流程有未保存修改，切换将放弃这些修改。继续？',danger:true}))){rememberOpenWorkflow();return}
  if(flowRouteId()!==id)return;
  state.workflow=clone(wf);
  state.workflowSaved=clone(wf);
  state.workflowDirty=false;
  state.selectedNode=null;
  localStorage.setItem('loopforge.activeWorkflow',id);
  renderWorkflowMeta();renderCanvas();renderProperties();
  refreshGenericWorkflowRoles();
}
function runViewHash(id){return id?`#/runs/${encodeURIComponent(id)}`:'#/runs'}
function syncRunHash(id){
  if((state.currentView||activeViewName())!=='runs')return;
  const next=runViewHash(id||'');
  if(location.hash!==next)history.replaceState(null,'',next);
}
function persistView(name,push=false){
  localStorage.setItem('loopforge.activeView',name);
  let next=`#/${name}`;
  if(name==='runs'){
    const linked=runRouteId();
    if(linked)next=runViewHash(linked);
  }
  if(name==='flow')next=flowViewHash();
  if(name==='settings'&&state.settingsTab)next=`#/settings/${state.settingsTab}`;
  if(location.hash===next)return;
  // push=true 时压入新历史记录，让浏览器「后退」能在平台页面间回退；
  // 否则原地替换，避免 boot 时把首个视图压栈。
  if(push)history.pushState(null,'',next);
  else history.replaceState(null,'',next);
}
// 侧栏只留几个入口；入口下的页面用顶部标签（必要时再加一层子标签）切换。
const VIEW_TABSETS=[
  {nav:'issues',label:'工作',tabs:[{view:'issues',label:'SOP 运行',count:true,subs:[['issues','需求'],['runs','运行'],['approvals','待确认']]}]},
  {nav:'sops',label:'配置',tabs:[{view:'sops',label:'流程',subs:[['sops','模板'],['flow','设计']]}]},
  {nav:'agents',label:'配置',tabs:[{view:'agents',label:'Bot',subs:[['agents','Bot'],['skills','技能'],['mcp','MCP'],['rules','规则']]}]},
  {nav:'sandboxes',label:'配置',tabs:[{view:'sandboxes',label:'环境',subs:[['sandboxes','沙箱'],['dockerfiles','Dockerfile'],['environments','安装脚本']]}]},
  {nav:'metrics',label:'洞察',tabs:[{view:'metrics',label:'运行洞察',subs:[['metrics','运行洞察'],['businessDomains','汇报']]}],plugins:true},
  {nav:'platform',label:'管理',tabs:[{view:'platform',label:'成员与权限',when:()=>isPlatformAdmin()},{view:'businessDomainManagement',label:'领域',when:()=>canAccessPlatformManagement()}]},
  {nav:'settings',label:'设置',tabs:[{view:'settings',label:'个人设置'},{view:'runners',label:'执行机器'},{view:'guide',label:'接入指南'}]},
];
function tabsetTabs(set){
  const tabs=set.tabs.filter(t=>!t.when||t.when());
  if(set.plugins)[...pluginViews.values()].forEach(v=>tabs.push({view:v.id,label:v.title}));
  return tabs;
}
function tabOwns(tab,name){return tab.view===name||(tab.subs||[]).some(s=>s[0]===name)}
function viewTabset(name){return VIEW_TABSETS.find(s=>tabsetTabs(s).some(t=>tabOwns(t,name)))||null}
function resolveSectionView(name){
  const set=VIEW_TABSETS.find(s=>s.nav===name&&!s.tabs.some(t=>t.view===name));
  if(!set)return name;
  const last=localStorage.getItem(`loopforge.section.${name}`);
  const tabs=tabsetTabs(set);
  return last&&tabs.some(t=>tabOwns(t,last))?last:(tabs[0]?.view||'chat');
}
function applyViewTabset(name,target){
  const set=viewTabset(name);if(!set)return;
  localStorage.setItem(`loopforge.section.${set.nav}`,name);
  $(`.platform-sidebar .nav[data-view="${set.nav}"]`)?.classList.add('active');
  const view=$(`#${target}View`);if(!view)return;
  const tabs=tabsetTabs(set);
  const cur=tabs.find(t=>tabOwns(t,name));
  const count=$('#approvalNavBadge')?.textContent||'';
  let pagebar=view.querySelector(':scope>.lf-pagebar');
  if(!pagebar){
    pagebar=document.createElement('div');pagebar.className='lf-pagebar';
    pagebar.innerHTML='<div class="lf-viewtabs" role="tablist"></div><div class="lf-pagebar-actions"></div>';
    view.prepend(pagebar);
    view.querySelectorAll(':scope>.page-heading>.heading-actions,:scope>.page-heading>button,:scope>.page-heading>.button').forEach(el=>pagebar.lastElementChild.appendChild(el));
  }
  const bar=pagebar.firstElementChild;
  bar.setAttribute('aria-label',set.label);
  bar.innerHTML=tabs.map(t=>{const on=t===cur;const go=on?name:(t.subs?(localStorage.getItem(`loopforge.sub.${t.view}`)||t.view):t.view);return `<button type="button" role="tab" class="lf-viewtab${on?' active':''}" data-view="${go}" aria-selected="${on}">${escapeHTML(t.label)}${t.count?`<span class="lf-viewtab-count" data-approval-count${count?'':' hidden'}>${count}</span>`:''}</button>`}).join('');
  if(cur?.subs){
    localStorage.setItem(`loopforge.sub.${cur.view}`,name);
    const sub=document.createElement('div');sub.className='lf-subtabs';
    sub.innerHTML=cur.subs.map(([v,l])=>`<button type="button" class="lf-subtab${v===name?' active':''}" data-view="${v}">${escapeHTML(l)}</button>`).join('');
    bar.appendChild(sub);
  }
}
function switchView(name){
  const legacy={build:'sops',me:'settings',admin:'platform',insights:'metrics'};
  if(legacy[name])name=legacy[name];
  name=resolveSectionView(name);
  const titles=viewTitles();
  if(name==='access')name='guide';
  if(name==='assets')name='dockerfiles';
  if(name==='playbooks')name='sops';
  const assetViews=['rules','skills','dockerfiles','environments','sandboxes'];
  const targetView=assetViews.includes(name)?'assets':name;
  if(!titles[name]||!$(`#${targetView}View`)){name='chat'}
  // 手敲 #/platform 或者从别的账号留下的 localStorage 恢复进来，都要挡住。
  if(name==='platform'&&!isPlatformAdmin())name='chat';
  if(name==='businessDomainManagement'&&!canAccessPlatformManagement())name='chat';
  const target=assetViews.includes(name)?'assets':name;
  if(name==='settings'&&typeof currentSettingsTab==='function')state.settingsTab=currentSettingsTab();
  const staying=activeViewName()===name;
  const prevView=state.currentView;
  state.currentView=name;
  const side=sidebarViewFor(name);
  $$('.nav[data-view]').forEach(n=>n.classList.toggle('active',n.dataset.view===name||n.dataset.view===side));
  $('#capabilityNavToggle')?.classList.toggle('child-active',['mcp','rules','skills'].includes(name));
  if(['mcp','rules','skills'].includes(name))setCapabilityNavExpanded(true);
  $('#environmentNavToggle')?.classList.toggle('child-active',['dockerfiles','environments','sandboxes'].includes(name));
  if(['dockerfiles','environments','sandboxes'].includes(name))setEnvironmentNavExpanded(true);
  syncNavGroups(name);
  document.body.dataset.page=name;
  $$('.view').forEach(v=>v.classList.remove('active'));
  $(`#${target}View`).classList.add('active');
  $('#pageTitle').textContent=titles[name]||name;
  applyViewTabset(name,target);
  showDirectRunButton(name==='flow');
  closeIssueDrawer();
  // 仅真实用户切换时压栈：boot 首次进入（prevView 为空）与同视图重复调用
  //（staying）不压；hashchange 后退触发的调用 prevView 非空且 hash 已是目标值，
  // persistView 内部会短路，不会重复压栈。
  persistView(name,!staying&&!!prevView);
  const nav=$('.platform-sidebar nav');
  if(nav){
    if(['chat','issues','approvals','sops'].includes(name)){
      nav.scrollTop=0;
    }else{
      const activeBtn=$(`.platform-sidebar .nav[data-view="${name}"]`);
      if(activeBtn){
        const navRect=nav.getBoundingClientRect(), btnRect=activeBtn.getBoundingClientRect();
        if(btnRect.top < navRect.top || btnRect.bottom > navRect.bottom){
          activeBtn.scrollIntoView({block:'nearest',behavior:'instant'});
        }
      }
    }
  }
  if(name==='runs'){
    const linkedRun=runRouteId();
    const hashAlreadyRuns=(location.hash||'').replace(/^#\/?/,'').split(/[/?]/)[0]==='runs';
    if(!staying){
      if(linkedRun&&(hashAlreadyRuns||!prevView))showRunDetail(linkedRun);
      else showRunList();
    }
    refreshRuns(false,true);
  }
  if(name==='issues'){refreshIssues();renderSync()}
  if(name==='sops')refreshWorkflowMetrics()
  if(name==='chat'&&typeof renderHomeChat==='function')renderHomeChat()
  if(name==='metrics'){const key=metricsQueryKey();if(state.metricsDataKey!==key&&metricsPendingKey!==key)refreshMetrics();else renderMetricsDashboard()}
  if(name==='approvals')renderApprovals(false)
  if(name==='observability')refreshObs()
  if(name==='agents'){api('/api/agents',{silent:true}).then(agents=>{state.data.agents=agents;renderAgents()}).catch(e=>toast(e.message));renderAgents()}
  if(name==='settings')renderPersonalSettings();
  if(name==='platform')renderPlatformAdminPanel();
  if(name==='businessDomainManagement')loadBusinessDomainManagement();
  if(name==='projects')renderProjectAdmin();
  if(name==='businessDomains')loadBusinessDomains();
  if(assetViews.includes(name)){state.assetType=name;if(!staying)state.selectedAsset=null;renderAssets()}
  if(name==='mcp')loadMCPMarket();
  if(name==='runners')refreshRunners();
  if(name==='guide'){renderGuide();renderQuickstart()}
  if(name==='knowledge'&&typeof renderKnowledgeView==='function')renderKnowledgeView();
  if(name==='rooms'&&typeof loadRooms==='function')loadRooms();
  if(pluginViews.has(name))renderPluginView(pluginViews.get(name));
  if(name==='flow'){state.selectedNode=state.selectedNode||state.workflow?.nodes?.[0]?.id||null;renderWorkflowMeta();renderCanvas();renderProperties();setFlowView('canvas');refreshGenericWorkflowRoles()}
}
function renderAll(){renderUserBadge();renderPlatformNav();renderProjectSwitcher();renderIssues();renderSOPCatalog();renderWorkflowMeta();renderCanvas();renderProperties();renderAgents();renderAssets();renderMCPManagement();renderRuns();renderSync();renderProjectAdmin();renderQuickstart();renderGuide();updateApprovalBadge();showDirectRunButton($('#flowView').classList.contains('active'))}
