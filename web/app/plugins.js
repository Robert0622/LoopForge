// Web plugins (LOOPFORGE_WEB_PLUGINS) load after the core scripts and extend
// the UI through window.LoopForge. See docs/extending.md.
// boot() may fall back to the dashboard before a plugin script registers the
// view named in the URL, so remember the route the page was opened with.
const initialPluginRoute=(()=>{try{return (location.hash||'').replace(/^#\/?/,'').split(/[/?]/)[0]||localStorage.getItem('loopforge.activeView')||''}catch{return ''}})();
function pluginContext(){
  return {api,toast,escapeHTML,project:state.data?currentProject()||null:null,user:state.currentUser,isPlatformAdmin:isPlatformAdmin()};
}
function renderPluginView(view){
  const body=$(`#${view.id}View .plugin-view-body`);
  if(!body)return;
  try{view.render(body,pluginContext())}
  catch(e){console.error(`plugin view ${view.id}:`,e);body.innerHTML=`<div class="empty">插件页面渲染失败：${escapeHTML(e.message)}</div>`}
}
function pluginNavGroup(){
  let group=$('#pluginNavGroup');
  if(group)return group;
  group=document.createElement('div');
  group.id='pluginNavGroup';
  group.innerHTML='<p>扩展</p>';
  const anchor=$('#platformNavGroup');
  anchor.parentNode.insertBefore(group,anchor);
  return group;
}
function registerPluginView(def){
  const id=String(def?.id||'').trim();
  if(!/^[a-z][a-z0-9-]{1,39}$/.test(id))throw new Error(`LoopForge.registerView: invalid id "${id}" (lowercase letters, digits and "-")`);
  if(pluginViews.has(id)||viewTitles()[id]||$(`#${id}View`))throw new Error(`LoopForge.registerView: view "${id}" already exists`);
  const title=String(def.title||'').trim();
  if(!title)throw new Error(`LoopForge.registerView: view "${id}" needs a title`);
  if(typeof def.render!=='function')throw new Error(`LoopForge.registerView: view "${id}" needs a render(container, ctx) function`);
  const view={id,title,render:def.render};
  const section=document.createElement('section');
  section.id=`${id}View`;
  section.className='view page-view plugin-view';
  section.innerHTML='<div class="plugin-view-body"></div>';
  $('#settingsView').parentNode.appendChild(section);
  const btn=document.createElement('button');
  btn.className='nav';
  btn.dataset.view=id;
  btn.innerHTML=`<span class="nav-icon">${escapeHTML(def.icon||'◆')}</span><span class="nav-label">${escapeHTML(title)}</span>`;
  btn.onclick=()=>switchView(id);
  pluginNavGroup().appendChild(btn);
  pluginViews.set(id,view);
  if(initialPluginRoute===id&&state.currentView==='chat')switchView(id);
  return view;
}
window.LoopForge=Object.freeze({
  apiVersion:1,
  registerView:registerPluginView,
  api:(path,options)=>api(path,options),
  toast:(message,type)=>toast(message,type),
  escapeHTML:v=>escapeHTML(v),
  currentProject:()=>state.data?currentProject()||null:null,
  currentUser:()=>state.currentUser,
  navigate:name=>switchView(name),
});
