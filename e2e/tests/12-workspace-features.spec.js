const { test, expect } = require('@playwright/test');
const { ADMIN, watchErrors, login, visibleButton, api, closeDrawer, useScriptProject } = require('./helpers');

async function openIssue(page, title) {
  await page.locator('.nav[data-view="issues"]').click();
  await page.locator('#issueSearch').fill(title);
  await expect(page.locator('#issueBoard')).toContainText(title);
  await page.getByText(title).first().click();
  const drawer = page.locator('#issueDetailDrawer');
  await expect(drawer).toContainText(title);
  return drawer;
}

async function useProject(page, id, name) {
  const projects = await api(page, '/api/projects');
  if (!projects.some(item => item.id === id)) {
    await api(page, '/api/projects', { method: 'POST', body: { id, key: id.replace(/[^a-z]/g, '').slice(0, 6).toUpperCase(), name, ownerId: 1 } });
    await page.reload();
    await expect(page.locator('#userName')).toBeVisible();
  }
  const switcher = page.locator('#projectSwitcher');
  await expect(switcher.locator(`option[value="${id}"]`)).toHaveCount(1);
  await switcher.selectOption(id);
}

test('first login walks through the onboarding tour and ends in settings', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page, { ...ADMIN, onboarding: true });
  const card = page.locator('#onboarding .onboarding-card');
  await expect(card).toBeVisible();
  await expect(page.locator('#onboardingKicker')).toHaveText('1 / 4');
  await expect(page.locator('#onboardingBody')).toContainText('GitHub');
  await expect(page.locator('.nav[data-view="issues"]')).toHaveClass(/onboarding-target/);
  await page.locator('#onboardingNext').click();
  await page.locator('#onboardingNext').click();
  await page.locator('#onboardingNext').click();
  await expect(page.locator('#onboardingNext')).toHaveText('去填写');
  await expect(page.locator('#onboardingBody')).toContainText('GitHub Token');
  await page.locator('#onboardingNext').click();
  await expect(page.locator('#onboarding')).toBeHidden();
  await expect(page.locator('#pageTitle')).toHaveText('个人设置');

  await page.reload();
  await expect(page.locator('#userName')).toBeVisible();
  await page.waitForTimeout(1200);
  await expect(page.locator('#onboarding')).toBeHidden();

  await page.locator('.nav[data-view="guide"]').click();
  await page.locator('#replayOnboarding').click();
  await expect(page.locator('#onboardingKicker')).toHaveText('1 / 4');
  await page.keyboard.press('Escape');
  await expect(page.locator('#onboarding')).toBeHidden();
  expect(errors).toEqual([]);
});

test('a requirement without history is deleted from its drawer; a delivered one cannot be', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  const created = await api(page, '/api/issues', {
    method: 'POST',
    body: { projectId: 'platform', title: '浏览器：待删除需求', kind: 'requirement', syncTarget: 'local', status: 'todo' },
  });
  const drawer = await openIssue(page, '浏览器：待删除需求');
  await drawer.locator(`[data-delete-issue="${created.id}"]`).click();
  await expect(page.locator('#confirmOkBtn')).toBeVisible();
  await page.locator('#confirmOkBtn').click();
  await expect.poll(async () => (await api(page, '/api/issues')).some(item => item.id === created.id)).toBe(false);
  await closeDrawer(page);
  await page.locator('#issueSearch').fill('浏览器：待删除需求');
  await expect(page.locator('#issueBoard')).not.toContainText('浏览器：待删除需求');

  await useScriptProject(page);
  const delivered = await openIssue(page, '浏览器：脚本 SOP 真跑');
  await expect(delivered.locator('[data-delete-issue]')).toHaveCount(0);
  const view = (await api(page, '/api/issues')).find(item => item.title === '浏览器：脚本 SOP 真跑');
  expect(view.deletionAllowed).toBe(false);
  await closeDrawer(page);
  expect(errors).toEqual([]);
});

test('a business domain is created, assigned to a project and reported', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  await useProject(page, 'e2e-bd-ui', '业务域浏览器项目');

  await page.locator('.nav[data-view="businessDomainManagement"]').click();
  await expect(page.locator('#pageTitle')).toHaveText('领域管理');
  const form = page.locator('#businessDomainForm');
  await expect(form).toBeVisible();
  if (await page.locator('#businessNew').count()) await page.locator('#businessNew').click();
  await form.locator('[name=name]').fill('浏览器增长业务');
  await form.locator('[name=id]').fill('e2e-ui-growth');
  await form.locator('[name=description]').fill('Playwright 创建');
  await form.locator('[type=submit]').click();
  await expect(page.locator('#toastContainer')).toContainText('业务域已保存');
  await expect(page.locator('[data-business-domain="e2e-ui-growth"]')).toBeVisible();

  await page.locator('.nav[data-view="projects"]').click();
  const projectForm = page.locator('#projectAdminForm');
  await expect(projectForm.locator('[name=id]')).toHaveValue('e2e-bd-ui');
  await projectForm.locator('#projectBusinessDomainSelect').selectOption('e2e-ui-growth');
  await projectForm.locator('[type=submit]').click();
  await expect.poll(async () => (await api(page, '/api/projects')).find(item => item.id === 'e2e-bd-ui').businessDomainId).toBe('e2e-ui-growth');
  const history = await api(page, '/api/projects/e2e-bd-ui/business-domain/history');
  expect(JSON.stringify(history)).toContain('e2e-ui-growth');

  await page.locator('.nav[data-view="businessDomains"]').click();
  await expect(page.locator('#pageTitle')).toHaveText('业务领域');
  const filter = page.locator('#businessReportFilter');
  await expect(filter.locator('select[name=scope] option[value="e2e-ui-growth"]')).toHaveCount(1);
  await filter.locator('select[name=scope]').selectOption('e2e-ui-growth');
  await filter.locator('[type=submit]').click();
  await expect(page.locator('.business-report-meta')).toContainText('浏览器增长业务');
  await expect(page.locator('.business-report')).toContainText('业务域浏览器项目');
  await expect(page.locator('#businessExport')).toBeEnabled();
  expect(errors).toEqual([]);
});

test('usage analytics manages external sources and filters by execution origin', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  await useProject(page, 'e2e-ext-ui', '外部数据源浏览器项目');
  await page.locator('.nav[data-view="observability"]').click();
  await expect(page.locator('#pageTitle')).toHaveText('用量分析');

  await page.locator('#obsManageSources').click();
  const dialog = page.locator('dialog.obs-source-dialog');
  await expect(dialog).toBeVisible();
  await expect(dialog.locator('[data-sources]')).toContainText('尚无外部数据源');
  await dialog.locator('[name=name]').fill('浏览器评审流水线');
  await dialog.locator('[data-add]').click();
  await dialog.locator('[data-mapping] input').fill('ci-bot');
  await dialog.locator('[data-mapping] select').selectOption('1');
  await dialog.locator('[data-create] [type=submit]').click();
  await expect(dialog.locator('[data-created]')).toBeVisible();
  const token = await dialog.locator('[data-token]').inputValue();
  expect(token).toMatch(/^afi_/);
  await expect(dialog.locator('[data-sources]')).toContainText('浏览器评审流水线');
  await expect(dialog.locator('[data-sources]')).not.toContainText(token);

  const finished = new Date(Date.now() - 60_000);
  const execution = {
    id: 'ui-event-1', type: 'workflow_execution', externalExecutionId: 'ui-exec-1', externalUserId: 'ci-bot',
    workflow: { id: 'pr-review', name: 'PR 评审' }, status: 'succeeded',
    startedAt: new Date(finished.getTime() - 300_000).toISOString(), finishedAt: finished.toISOString(),
    usage: [{ provider: 'anthropic', model: 'claude', inputTokens: 70, outputTokens: 30, costStatus: 'unknown' }],
  };
  const ingest = await page.evaluate(async ({ token, execution }) => {
    const res = await fetch('/api/insights/v1/ingest', {
      method: 'POST',
      credentials: 'omit',
      headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
      body: JSON.stringify(execution),
    });
    return { status: res.status, body: await res.text() };
  }, { token, execution });
  expect(ingest.status, ingest.body).toBeLessThan(300);

  await dialog.locator('[data-revoke]').click();
  await expect(dialog.locator('[data-sources]')).toContainText('已撤销');
  await dialog.locator('[data-close]').click();
  await expect(dialog).toHaveCount(0);

  await page.locator('#obsOriginFilter').selectOption('external');
  await expect(page.locator('#observabilityView')).toContainText('外部');
  const analytics = await api(page, '/api/analytics?projectId=e2e-ext-ui&origin=external');
  expect(analytics.summary.totalTokens).toBe(100);
  expect(errors).toEqual([]);
});

test('the flow canvas offers the AI orchestration assistant', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  await useScriptProject(page);
  await page.locator('.nav[data-view="sops"]').click();
  await page.locator('#newSOP').click();
  const form = page.locator('#sopCreateForm');
  await form.locator('[name=name]').fill('浏览器 AI 编排草稿');
  await page.locator('#sopNext').click();
  await form.locator('.template-choice', { hasText: '从空白开始' }).click();
  await page.locator('#sopFinish').click();
  await expect(page.locator('#flowView')).toBeVisible();

  await expect(page.locator('#canvasAIEntry')).toBeVisible();
  await page.locator('#canvasAIEntry').click();
  const panel = page.locator('#aiOrchestrator');
  await expect(panel).toBeVisible();
  await expect(page.locator('#aiOrchestratorInput')).toHaveAttribute('placeholder', /告诉我你的目标/);
  const suggestions = page.locator('#aiOrchestratorSuggest [data-ai-prompt]');
  await expect(suggestions).toHaveCount(4);
  const first = await suggestions.first().textContent();
  await page.locator('#aiSuggestMore').click();
  await expect(suggestions.first()).not.toHaveText(first);
  await suggestions.first().click();
  await expect(page.locator('#aiOrchestratorInput')).not.toHaveValue('');
  await page.locator('#aiOrchestratorInput').fill('');
  await page.locator('#closeAIOrchestrator').click();
  await expect(panel).toBeHidden();

  await page.locator('[data-add="script"]').click();
  await expect(page.locator('#canvasAIEntry')).toBeHidden();
  await page.locator('#openAIOrchestrator').click();
  await expect(panel).toBeVisible();
  await expect(page.locator('#aiOrchestratorInput')).toHaveAttribute('placeholder', /直接说要改哪里/);
  await page.locator('#openAIOrchestrator').click();
  await expect(panel).toBeHidden();
  expect(errors).toEqual([]);
});

test('a room lists only SOPs and expands the remainder on demand', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  await useProject(page, 'e2e-room-sops', '群聊 SOP 列表项目');
  for (let i = 1; i <= 6; i++) {
    await api(page, '/api/workflows', {
      method: 'PUT',
      body: {
        id: `e2e-room-sop-${i}`, name: `群聊列表 SOP ${i}`, version: 1, projectId: 'e2e-room-sops', status: 'published', sandboxProfileId: 'local',
        nodes: [{ id: 'run', name: 'run', type: 'script', maxAttempts: 1, script: 'true' }], edges: [],
      },
    });
  }
  await page.reload();
  await expect(page.locator('#userName')).toBeVisible();
  await page.locator('#projectSwitcher').selectOption('e2e-room-sops');
  await page.locator('.nav[data-view="rooms"]').click();
  await page.locator('#newRoom').click();
  await page.locator('#confirmPromptInput').fill('浏览器 SOP 列表群');
  await page.locator('#confirmOkBtn').click();
  await expect(page.locator('#roomTitle')).toContainText('浏览器 SOP 列表群');

  const list = page.locator('#roomSOPList');
  await expect(list.locator('[data-room-pick]')).toHaveCount(4);
  const expand = list.locator('[data-room-sop-expand="expand"]');
  await expect(expand).toHaveText(/^展开其余 \d+ 条$/);
  const hidden = Number((await expand.textContent()).match(/\d+/)[0]);
  expect(hidden).toBeGreaterThanOrEqual(2);
  await expand.click();
  await expect(list.locator('[data-room-pick]')).toHaveCount(4 + hidden);
  await list.locator('[data-room-sop-expand="collapse"]').click();
  await expect(list.locator('[data-room-pick]')).toHaveCount(4);

  await page.locator('#roomSOPSearch').fill('SOP 6');
  await expect(list.locator('[data-room-pick]')).toHaveCount(1);
  await expect(list).toContainText('群聊列表 SOP 6');
  await expect(list.locator('[data-room-sop-expand]')).toHaveCount(0);
  expect(errors).toEqual([]);
});

test('switching projects never shows the previous project metrics', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  await useScriptProject(page);
  const seen = [];
  page.on('request', req => {
    const url = new URL(req.url());
    if (url.pathname === '/api/analytics' && url.searchParams.get('requirementsOnly') === '1') seen.push(url.searchParams.get('projectId'));
  });
  await page.locator('.nav[data-view="metrics"]').click();
  await expect(page.locator('#pageTitle')).toHaveText('研发效能');
  await expect.poll(() => seen.includes('e2e-script')).toBe(true);

  await page.locator('#projectSwitcher').selectOption('platform');
  await expect.poll(() => seen[seen.length - 1]).toBe('platform');
  const key = await page.evaluate(() => state.metricsDataKey);
  await expect.poll(() => page.evaluate(() => state.metricsDataKey.split('|')[0])).toBe('platform');
  expect(key === '' || key.startsWith('platform|')).toBe(true);
  await expect(page.locator('#metricsView')).not.toContainText('浏览器：脚本 SOP 真跑');
  expect(errors).toEqual([]);
});
