// 第一次登录先指向工作总览的快速开始清单，再沿侧栏逐项说明。看完或跳过之后不再自动出现，
// 可从接入指南重开。演示模式最后回到清单去「一键开跑」，否则打开个人设置去配凭据。
const ONBOARDING_KEY = 'loopforge.onboarding.v3';

function onboardingSteps(demo) {
  const start = demo
    ? '演示模式已开启，什么都不用配：上面的对话框可以直接「一键演示」，或点清单里的「一键开跑」。跑起来后右侧能看到运行进度，停在审批节点时去批准，就能看到交付报告。'
    : '先看上面的对话框：用一句话说清需求，平台会澄清、建单并开跑。也可以照着清单配模型 Key、GitHub Token 和仓库，每项都有直达按钮。';
  return [
    { target: '#homeChat', title: '从对话开始', body: '在工作总览用对话描述需求。澄清完成后可以建单开跑；右侧能看到正在运行和待确认的工作流。' },
    { target: '#dashQuickstart', title: '或照着清单一步步来', body: start },
    { target: '.nav[data-view="issues"]', title: '需求从这里进', body: '在这里新建，或从 GitHub 导入 Issue。说清楚要做什么就可以派发；没说清的地方，澄清会继续问。' },
    { target: '.nav[data-view="approvals"]', title: '要你拍板时来这里', body: '运行停下来等你决定，会出现在「待我确认」。先看清问题，再同意或退回。没有待办时，这里是空的。' },
    { target: '.nav[data-view="runs"]', title: '进度看运行记录', body: '点进一次运行，能看到停在哪一步、已经产出了什么。' },
    demo
      ? { target: '.nav[data-view="settings"]', title: '跑真实需求前配凭据', body: '演示不需要凭据。跑真实需求前，在个人设置里填 GitHub Token（要能读写目标仓库）和一个模型 API Key（Claude Code、Codex 或 CodeBuddy 任选）。现在先回到清单，点「一键开跑」看一遍完整流程。', view: 'dashboard', finish: true, next: '开始体验' }
      : { target: '.nav[data-view="settings"]', title: '先把凭据配齐', body: '没配齐就派不出去。先填 GitHub Token，要能读写目标仓库（classic PAT 勾选 repo；fine-grained 授予 Contents 和 Pull requests 读写）。再配一个模型 API Key（Claude Code、Codex 或 CodeBuddy 任选）和个人 MCP 令牌，后者可以与 GitHub Token 共用。', view: 'settings', finish: true, next: '去填写' },
  ];
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
  // 目标太宽、左右都放不下时（比如工作总览的清单），改放到目标下方；
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
  if (typeof switchView === 'function') switchView('dashboard');
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
    finishOnboarding(step.view || 'settings');
    return;
  }
  moveOnboarding(1);
}

function finishOnboarding(view) {
  onboardingMarkDone();
  closeOnboarding();
  if (view && typeof switchView === 'function') switchView(view);
  setTimeout(() => {
    if (typeof maybeStartSetupWizard === 'function') maybeStartSetupWizard();
  }, 400);
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
  if (!step) return finishOnboarding(steps[steps.length - 1].view);
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
    if (onboardingIndex >= steps.length) return finishOnboarding(steps[steps.length - 1].view);
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
