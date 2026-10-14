// 第一次登录沿侧栏逐项说明，中间指一下首页的对话输入框。看完或跳过之后
// 不再自动出现，可从接入指南重开。结束时打开个人设置里还没配好的那一页。
const ONBOARDING_KEY = 'loopforge.onboarding.v3';

function onboardingSteps(demo) {
  const start = demo
    ? '提需求、问进度、让它画图出文档都在这里说，可以粘贴截图、@ 已有需求。演示模式已开启：没配 Key 也能点「先跑一遍演示」看完整流程，停在审批节点时右侧会出现「去确认」。'
    : '提需求、问进度、让它画图出文档都在这里说，可以粘贴截图、@ 已有需求。需求说清后会出一张开跑卡片，进度在右侧看。';
  const last = demo
    ? { title: '跑真实需求前配凭据', body: '演示不需要凭据。跑真实需求前要配一个模型 API Key（Claude Code、Codex 或 CodeBuddy 任选）和能读写目标仓库的 GitHub Token。点「去配置」打开个人设置。', next: '去配置' }
    : { title: '先把凭据配齐', body: '没配齐就派不出去。先配一个模型 API Key（Claude Code、Codex 或 CodeBuddy 任选），再填能读写目标仓库的 GitHub Token（classic PAT 勾选 repo；fine-grained 授予 Contents 和 Pull requests 读写）。GitHub MCP 会自动复用这个 Token。', next: '去填写' };
  return [
    { target: '.nav[data-view="chat"]', title: '首页就是对话', body: '平台的事都可以先在首页问。它是这个项目的 PM 助理，会查需求、查运行，帮你澄清需求并按 SOP 开跑。' },
    { target: '#homeChatForm', title: '在这里说', body: start },
    { target: '.nav[data-view="issues"]', title: '需求、运行、待确认都在「交付」', body: '顶部三个标签：需求看板管规划，运行看 Agent 正在哪一步，待我确认是等你拍板的节点。有东西要你确认时，这里会亮起角标。' },
    { target: '.nav[data-view="rooms"]', title: '没有固定流程的活，交给「群聊」', body: '在群里 @LoopForge 说一句要做什么，队长会拉几个 Bot 分工、交接、汇报；有问题也可以直接 @ 它问，或回复某条消息接着问。' },
    { target: '.nav[data-view="build"]', title: '流程、角色、能力、环境都在「构建」', body: 'SOP、角色、技能/MCP/规则、沙箱和项目都在这里，用顶部标签切换。日常不用动，要调整流程时再来。' },
    { target: '.nav[data-view="me"]', ...last, finish: true },
  ];
}

// 引导结束后要去配的那一页：先模型 Key，再 GitHub Token；都配好了就留在原地。
function onboardingSettingsTab() {
  const stored = (typeof userById === 'function' ? userById(state.currentUser?.id) : null) || {};
  if (!(stored.codeBuddyApiKeyConfigured || stored.codexApiKeyConfigured || stored.claudeCodeApiKeyConfigured)) return 'models';
  if (!stored.gitTokenConfigured) return 'git';
  return '';
}

function onboardingDemo() {
  return typeof quickstartDemoAvailable === 'function' && typeof state !== 'undefined' && quickstartDemoAvailable();
}

// 卡片贴在侧栏项右侧，竖向对准这一项；右侧放不下才改到左边。
function onboardingPlace(rect, card, viewport) {
  const gap = 14;
  const margin = 16;
  let left = rect.right + gap;
  let top = rect.top + rect.height / 2 - card.height / 2;
  let placement = 'right';
  if (left + card.width > viewport.width - margin) {
    left = rect.left - gap - card.width;
    placement = 'left';
  }
  // 目标太宽、左右都放不下时（比如首页的输入框），改放到目标下方；
  // 只有上方完整放得下才放上方，免得盖住目标的标题。
  if (left < margin) {
    left = rect.left + 24;
    top = rect.bottom + gap;
    placement = 'below';
    if (top + card.height > viewport.height - margin && rect.top - gap - card.height >= margin) {
      top = rect.top - gap - card.height;
      placement = 'above';
    }
  }
  top = Math.max(margin, Math.min(top, viewport.height - card.height - margin));
  left = Math.max(margin, Math.min(left, viewport.width - card.width - margin));
  return { top, left, placement };
}

function onboardingSeen() {
  try { return localStorage.getItem(ONBOARDING_KEY) === 'done'; } catch (e) { return true; }
}

function onboardingMarkDone() {
  try { localStorage.setItem(ONBOARDING_KEY, 'done'); } catch (e) {}
}

let onboardingIndex = 0;
let onboardingTarget = null;

function onboardingRoot() {
  let root = document.getElementById('onboarding');
  if (root) return root;
  root = document.createElement('div');
  root.id = 'onboarding';
  root.hidden = true;
  root.innerHTML = '<div class="onboarding-catch"></div><div class="onboarding-spot" hidden></div><div class="onboarding-card" role="dialog" aria-modal="true" aria-labelledby="onboardingTitle"><div class="onboarding-arrow"></div><p class="onboarding-kicker" id="onboardingKicker"></p><h2 id="onboardingTitle"></h2><p id="onboardingBody"></p><div class="onboarding-actions"><button type="button" class="button ghost small" id="onboardingSkip">跳过</button><span class="onboarding-grow"></span><button type="button" class="button ghost small" id="onboardingBack">上一步</button><button type="button" class="button primary small" id="onboardingNext">下一步</button></div></div>';
  document.body.appendChild(root);
  root.querySelector('#onboardingSkip').onclick = () => finishOnboarding();
  root.querySelector('#onboardingBack').onclick = () => moveOnboarding(-1);
  root.querySelector('#onboardingNext').onclick = () => onboardingAdvance();
  root.querySelector('.onboarding-catch').onclick = event => event.stopPropagation();
  return root;
}

function startOnboarding() {
  onboardingIndex = 0;
  if (typeof setQuickstartHidden === 'function') setQuickstartHidden(false);
  if (typeof switchView === 'function') switchView('chat');
  if (typeof renderQuickstart === 'function') renderQuickstart();
  const root = onboardingRoot();
  root.hidden = false;
  document.body.classList.add('onboarding-open');
  showOnboardingStep();
}

function maybeStartOnboarding() {
  const user = typeof state !== 'undefined' ? state.currentUser : null;
  if (!user || user.isGuest || onboardingSeen()) return;
  if (typeof runRouteId === 'function' && runRouteId()) return;
  if (typeof flowRouteId === 'function' && flowRouteId()) return;
  setTimeout(() => {
    if (!onboardingSeen()) startOnboarding();
  }, 700);
}

function moveOnboarding(delta) {
  const steps = onboardingSteps(onboardingDemo());
  onboardingIndex = Math.max(0, Math.min(steps.length - 1, onboardingIndex + delta));
  showOnboardingStep();
}

function onboardingAdvance() {
  const step = onboardingSteps(onboardingDemo())[onboardingIndex];
  if (step && step.finish) {
    finishOnboarding();
    return;
  }
  moveOnboarding(1);
}

function finishOnboarding() {
  onboardingMarkDone();
  closeOnboarding();
  const tab = onboardingSettingsTab();
  if (tab && typeof openSettings === 'function') openSettings(tab);
}

function closeOnboarding() {
  const root = document.getElementById('onboarding');
  if (root) root.hidden = true;
  document.body.classList.remove('onboarding-open');
  if (onboardingTarget) onboardingTarget.classList.remove('onboarding-target');
  onboardingTarget = null;
  const spot = document.querySelector('.onboarding-spot');
  if (spot) spot.hidden = true;
}

function showOnboardingStep() {
  const steps = onboardingSteps(onboardingDemo());
  const step = steps[onboardingIndex];
  if (!step) return finishOnboarding();
  const root = onboardingRoot();
  root.hidden = false;
  document.body.classList.add('onboarding-open');
  root.querySelector('#onboardingKicker').textContent = (onboardingIndex + 1) + ' / ' + steps.length;
  root.querySelector('#onboardingTitle').textContent = step.title;
  root.querySelector('#onboardingBody').textContent = step.body;
  const back = root.querySelector('#onboardingBack');
  const next = root.querySelector('#onboardingNext');
  back.hidden = onboardingIndex === 0;
  next.textContent = step.finish ? (step.next || '完成') : '下一步';
  if (onboardingTarget) onboardingTarget.classList.remove('onboarding-target');
  onboardingTarget = step.target ? document.querySelector(step.target) : null;
  if (onboardingTarget && (onboardingTarget.hidden || !onboardingTarget.getClientRects().length)) onboardingTarget = null;
  if (step.target && !onboardingTarget) {
    onboardingIndex += 1;
    if (onboardingIndex >= steps.length) return finishOnboarding();
    showOnboardingStep();
    return;
  }
  if (onboardingTarget) {
    onboardingTarget.classList.add('onboarding-target');
    onboardingTarget.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  }
  placeOnboarding();
  requestAnimationFrame(() => placeOnboarding());
}

function placeOnboarding() {
  const root = document.getElementById('onboarding');
  if (!root || root.hidden || !onboardingTarget) return;
  const card = root.querySelector('.onboarding-card');
  const spot = root.querySelector('.onboarding-spot');
  const arrow = card.querySelector('.onboarding-arrow');
  const rect = onboardingTarget.getBoundingClientRect();
  const pad = 4;
  spot.hidden = false;
  spot.style.top = (rect.top - pad) + 'px';
  spot.style.left = (rect.left - pad) + 'px';
  spot.style.width = (rect.width + pad * 2) + 'px';
  spot.style.height = (rect.height + pad * 2) + 'px';
  const placed = onboardingPlace(rect, { width: card.offsetWidth || 320, height: card.offsetHeight || 148 }, { width: window.innerWidth, height: window.innerHeight });
  card.style.top = placed.top + 'px';
  card.style.left = placed.left + 'px';
  card.classList.remove('is-right', 'is-left', 'is-below', 'is-above');
  card.classList.add('is-' + placed.placement);
  if (!arrow) return;
  if (placed.placement === 'below' || placed.placement === 'above') {
    arrow.style.top = '';
    return;
  }
  const cardRect = card.getBoundingClientRect();
  const y = rect.top + rect.height / 2 - cardRect.top - 7;
  arrow.style.top = Math.max(18, Math.min(y, cardRect.height - 32)) + 'px';
}

function bindOnboardingReplay() {
  const button = document.getElementById('replayOnboarding');
  if (!button || button.dataset.bound) return;
  button.dataset.bound = '1';
  button.onclick = () => startOnboarding();
}

if (typeof window !== 'undefined' && typeof window.addEventListener === 'function') {
  window.addEventListener('resize', () => placeOnboarding());
  window.addEventListener('keydown', event => {
    const root = document.getElementById('onboarding');
    if (!root || root.hidden) return;
    if (event.key === 'Escape') finishOnboarding();
    if (event.key === 'ArrowRight') onboardingAdvance();
    if (event.key === 'ArrowLeft' && onboardingIndex > 0) moveOnboarding(-1);
  });
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', bindOnboardingReplay);
  else bindOnboardingReplay();
}
