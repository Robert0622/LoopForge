// 工作总览顶部的「快速开始」清单：按真实状态打勾，每一项都有直达按钮。
// 正式使用的几步全部完成后自动消失；手动收起后，重看引导时会再打开。
const QUICKSTART_HIDDEN_KEY = 'loopforge.quickstart.hidden';
const QUICKSTART_DEMO_WORKFLOW = 'demo-delivery';

function quickstartSteps(ctx) {
  const demo = ctx.demo || {};
  const demoSteps = demo.available ? [
    { id: 'demo-run', title: '一键跑一个演示需求', hint: '不需要 API Key，也不需要仓库。平台用内置的演示 SOP 建一张需求并开跑。', done: !!demo.started, action: { label: demo.started ? '再跑一个' : '一键开跑', run: 'demo' } },
    { id: 'demo-approve', title: '批准方案，看交付结果', hint: '运行会停在人工审批节点等你拍板。在需求详情里点「确认通过」，跑完后打开「产物」看交付报告。', done: !!demo.finished, action: demo.waitingIssueId ? { label: '去批准', issue: demo.waitingIssueId } : demo.latestIssueId ? { label: '打开需求', issue: demo.latestIssueId } : null },
  ] : [];
  const realSteps = [
    { id: 'model-key', title: '配一个模型 API Key', hint: 'Claude Code、Codex、CodeBuddy 任选其一，在个人设置里填写并保存。Agent 用它调用模型。', done: !!ctx.modelKey, action: { label: '去配置', view: 'settings' } },
    { id: 'git-token', title: '配 GitHub Token', hint: '用来克隆仓库、推送工作分支和开 PR，需要目标仓库的读写权限。', done: !!ctx.gitToken, action: { label: '去配置', view: 'settings' } },
    { id: 'repo', title: '给项目绑定 GitHub 仓库', hint: '在项目设置里粘贴仓库地址。Agent 会在这个仓库的工作分支上改代码。', done: !!ctx.repoBound, action: { label: '去绑定', view: 'projects' } },
    { id: 'issue', title: '新建一张需求并派发', hint: '写清楚要做什么、怎么算做完。没说清的地方，澄清会继续追问。', done: !!ctx.realIssue, action: { label: '新建需求', run: 'newRequirement' } },
    { id: 'result', title: '跟进运行，验收结果', hint: '要你拍板时会出现在「待我确认」；跑完后在运行记录里看产物和 PR。', done: !!ctx.realFinished, action: { label: '看运行记录', view: 'runs' } },
  ];
  return { demoSteps, realSteps, complete: realSteps.every(step => step.done) };
}

function quickstartHidden() {
  try { return localStorage.getItem(QUICKSTART_HIDDEN_KEY) === '1'; } catch (e) { return false; }
}

function setQuickstartHidden(hidden) {
  try {
    if (hidden) localStorage.setItem(QUICKSTART_HIDDEN_KEY, '1');
    else localStorage.removeItem(QUICKSTART_HIDDEN_KEY);
  } catch (e) {}
}

function quickstartDemoAvailable() {
  return (state.data.workflows || []).some(w => w.id === QUICKSTART_DEMO_WORKFLOW);
}

function quickstartContext() {
  const user = state.currentUser || {};
  const stored = userById(user.id) || {};
  const projectId = state.projectFilter || '';
  const inProject = item => !projectId || item.projectId === projectId;
  // `loopforge --self-test` titles its demo requirement "自检：…"; it must not tick any of the user's steps.
  const selfTest = issue => issue.workflowId === QUICKSTART_DEMO_WORKFLOW && String(issue.title || '').startsWith('自检：');
  const issues = (state.data.issues || []).filter(issue => inProject(issue) && !selfTest(issue));
  const isDemo = issue => issue.workflowId === QUICKSTART_DEMO_WORKFLOW;
  const demoIssues = issues.filter(isDemo);
  const realIssues = issues.filter(issue => !isDemo(issue));
  const dispatched = issue => !!(issue.latestTaskId || issue.latestRunId || ['in_progress', 'done', 'closed'].includes(issue.status));
  const waiting = demoIssues.find(issue => issueCurrentStage(issue).key === 'waiting');
  return {
    modelKey: stored.claudeCodeApiKeyConfigured || stored.codexApiKeyConfigured || stored.codeBuddyApiKeyConfigured,
    gitToken: stored.gitTokenConfigured,
    repoBound: (state.data.repositories || []).some(inProject),
    realIssue: realIssues.some(dispatched),
    realFinished: realIssues.some(issue => issueRunHealth(issue) === 'closed'),
    demo: {
      available: quickstartDemoAvailable(),
      started: demoIssues.some(dispatched),
      finished: demoIssues.some(issue => issueRunHealth(issue) === 'closed'),
      waitingIssueId: waiting ? waiting.id : '',
      latestIssueId: demoIssues[0] ? demoIssues[0].id : '',
    },
  };
}

function quickstartStepHTML(step, index) {
  const action = step.action;
  const button = action ? `<button type="button" class="button ${step.done ? 'ghost' : 'primary'} small" data-qs-step="${step.id}">${escapeHTML(step.done && action.view ? '查看' : action.label)}</button>` : '';
  return `<li class="qs-step ${step.done ? 'is-done' : ''}"><span class="qs-check" aria-hidden="true">${step.done ? '✓' : index + 1}</span><div class="qs-text"><b>${escapeHTML(step.title)}</b><small>${escapeHTML(step.hint)}</small></div>${button}</li>`;
}

function renderQuickstart() {
  const host = $('#dashQuickstart');
  if (!host) return;
  const user = state.currentUser;
  const plan = user && !user.isGuest ? quickstartSteps(quickstartContext()) : null;
  if (!plan || plan.complete || quickstartHidden()) {
    host.hidden = true;
    host.innerHTML = '';
    return;
  }
  if (!state.doctorStatus) loadDoctor();
  const done = plan.realSteps.filter(step => step.done).length;
  const demoBlock = plan.demoSteps.length ? `<section class="qs-section qs-demo"><header><b>先体验一遍</b><span>演示模式已开启，什么都不用配</span></header><ol>${plan.demoSteps.map(quickstartStepHTML).join('')}</ol></section>` : '';
  host.hidden = false;
  host.innerHTML = `<div class="qs-head"><div><span class="qs-eyebrow">快速开始</span><h2>照着这张清单，第一张需求就能跑起来</h2><p>做完一项会自动打勾，五项都完成后这张卡片会自动收起。</p></div><div class="qs-progress"><b>${done} / ${plan.realSteps.length}</b><span>正式使用</span></div><button type="button" class="qs-close" data-qs-close title="收起，之后可在接入指南里点「重看引导」再打开">收起</button></div>${doctorQuickstartLine()}<div class="qs-body">${demoBlock}<section class="qs-section"><header><b>正式使用</b><span>在自己的仓库上跑真实需求</span></header><ol>${plan.realSteps.map(quickstartStepHTML).join('')}</ol></section></div>`;
  host.querySelector('[data-qs-close]').onclick = () => {
    setQuickstartHidden(true);
    renderQuickstart();
    toast('快速开始已收起，可在接入指南里点「重看引导」再打开');
  };
  const steps = [...plan.demoSteps, ...plan.realSteps];
  host.querySelectorAll('[data-qs-step]').forEach(button => {
    const step = steps.find(item => item.id === button.dataset.qsStep);
    button.onclick = () => runQuickstartAction(step.action, button);
  });
}

async function runQuickstartAction(action, button) {
  if (!action) return;
  if (action.view) return switchView(action.view);
  if (action.issue) return openIssueDetail(action.issue);
  if (action.run === 'newRequirement') return openRequirementWizard();
  if (action.run === 'demo') return startDemoRequirement(button);
}

async function startDemoRequirement(button, opts = {}) {
  const projectId = state.projectFilter || (state.data.projects || [])[0]?.id;
  if (!projectId) return toast('还没有项目，请先在项目管理里建一个');
  if (button) button.disabled = true;
  try {
    let issue = await api('/api/issues', { method: 'POST', body: JSON.stringify({
      projectId,
      title: '演示：需求列表支持导出 CSV',
      description: '在需求列表右上角加一个「导出 CSV」按钮，导出当前筛选结果，包含标题、状态、负责人和更新时间。',
      kind: 'requirement',
      priority: 'medium',
      status: 'todo',
      syncTarget: 'local',
      workflowId: QUICKSTART_DEMO_WORKFLOW,
      assigneeUserId: Number(state.currentUser?.id) || 0,
    }) });
    rememberIssue(issue);
    const task = await api(`/api/issues/${issue.id}/dispatch`, { method: 'POST', body: JSON.stringify({ workflowId: QUICKSTART_DEMO_WORKFLOW }) });
    issue = { ...issue, latestTaskId: task.id, status: 'in_progress' };
    rememberIssue(issue);
    (state.data.tasks || (state.data.tasks = [])).unshift(task);
    if (task.runId) {
      startLiveRun(task.runId);
      try {
        const run = await api(`/api/runs/${encodeURIComponent(task.runId)}`, { silent: true });
        if (run?.id) state.data.runs = sortRunsDescending([run, ...(state.data.runs || []).filter(r => r.id !== run.id)]);
      } catch (e) {}
    }
    renderIssues();
    renderDashboard();
    if (!opts.stay) {
      await openIssueDetail(issue.id, 'sop');
      toast('演示需求已开跑。几秒后会停在「确认方案」，点「确认通过，继续下游」放行', 'ok');
    }
    Promise.all([refreshIssues(), refreshRuns(true)]).catch(() => {});
    setTimeout(() => refreshIssues(), 2000);
    setTimeout(() => refreshIssues(), 10000);
    return { issue, task };
  } catch (err) {
    if (opts.stay) throw err;
    toast(err.message, 'err');
  } finally {
    if (button) button.disabled = false;
  }
}
