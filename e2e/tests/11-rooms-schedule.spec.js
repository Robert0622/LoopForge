const { test, expect } = require('@playwright/test');
const { watchErrors, login, api, waitForRun, useScriptProject } = require('./helpers');

test('a room can chat and dispatch a published script SOP', async ({ page }) => {
  test.setTimeout(90_000);
  const errors = watchErrors(page);
  await login(page);
  await useScriptProject(page);

  const workflows = await api(page, '/api/workflows');
  const sop = workflows.find(item => item.name === '浏览器脚本交付' && item.status === 'published');
  expect(sop, 'script SOP from the designer test').toBeTruthy();

  await page.locator('.nav[data-view="rooms"]').click();
  await expect(page.locator('#pageTitle')).toHaveText('协作群聊');
  await page.locator('#newRoom').click();
  await page.locator('#confirmPromptInput').fill('浏览器值班群');
  await page.locator('#confirmOkBtn').click();
  await expect(page.locator('#roomTitle')).toContainText('浏览器值班群');

  await page.locator('#roomInput').fill('先发一条群消息');
  await page.locator('#roomSend').click();
  await expect(page.locator('#roomLog')).toContainText('先发一条群消息');

  await page.locator('#roomModeIssue').click();
  await page.locator('#roomInput').fill('群里派发脚本 SOP');
  await page.locator('#roomSOP').selectOption(sop.id);
  await page.locator('#roomDispatch').click();
  await expect(page.locator('#roomLog')).toContainText('群里派发脚本 SOP');

  await expect.poll(async () => {
    const tasks = await api(page, '/api/tasks');
    const issues = await api(page, '/api/issues');
    const issue = issues.find(item => item.title === '群里派发脚本 SOP');
    const task = issue && tasks.find(item => item.issueId === issue.id);
    return task && task.runId;
  }).toBeTruthy();
  const issues = await api(page, '/api/issues');
  const issue = issues.find(item => item.title === '群里派发脚本 SOP');
  const task = (await api(page, '/api/tasks')).find(item => item.issueId === issue.id);
  const run = await waitForRun(page, task.runId, /succeeded/);
  expect(run.status).toBe('succeeded');
  expect(errors).toEqual([]);
});

test('a schedule job dispatches an issue that has not run yet', async ({ page }) => {
  test.setTimeout(90_000);
  const errors = watchErrors(page);
  await login(page);
  await useScriptProject(page);
  const workflows = await api(page, '/api/workflows');
  const sop = workflows.find(item => item.name === '浏览器脚本交付' && item.status === 'published');
  expect(sop).toBeTruthy();
  await api(page, '/api/issues', {
    method: 'POST',
    body: {
      projectId: 'e2e-script', title: '定时任务要派发的需求', kind: 'requirement', syncTarget: 'local',
      status: 'in_review', workflowId: sop.id,
    },
  });

  await page.locator('.nav[data-view="issues"]').click();
  await page.locator('#newScheduleJob').click();
  const form = page.locator('#scheduleJobForm');
  await form.locator('[name=name]').fill('浏览器定时派发');
  await form.locator('[name=workflowId]').selectOption(sop.id);
  await form.locator('input[name=issueStatus][value=backlog]').uncheck();
  await form.locator('input[name=issueStatus][value=todo]').uncheck();
  await form.locator('input[name=issueStatus][value=in_review]').check();
  await form.locator('[type=submit]').click();
  await expect(page.locator('#scheduleJobList')).toContainText('浏览器定时派发');
  await page.locator('#scheduleJobList button', { hasText: '立即运行' }).click();

  await expect.poll(async () => {
    const jobs = await api(page, '/api/schedule-jobs');
    const job = jobs.find(item => item.name === '浏览器定时派发');
    return job && job.lastStatus;
  }, { timeout: 20_000 }).toBe('success');
  const issue = (await api(page, '/api/issues')).find(item => item.title === '定时任务要派发的需求');
  const task = (await api(page, '/api/tasks')).find(item => item.issueId === issue.id);
  expect(task && task.runId).toBeTruthy();
  const run = await waitForRun(page, task.runId, /succeeded/);
  expect(run.status).toBe('succeeded');
  expect(errors).toEqual([]);
});

test('issue search, close and reopen work from the list', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  const created = await api(page, '/api/issues', {
    method: 'POST',
    body: { projectId: 'platform', title: '只在列表里关闭', kind: 'requirement', syncTarget: 'local', status: 'todo' },
  });
  await page.locator('.nav[data-view="issues"]').click();
  await page.locator('#issueSearch').fill('只在列表里关闭');
  await expect(page.locator('#issueBoard')).toContainText('只在列表里关闭');
  await page.getByText('只在列表里关闭').first().click();
  const drawer = page.locator('#issueDetailDrawer');
  await expect(drawer).toContainText('只在列表里关闭');
  await drawer.locator('[data-close-issue]').click();
  await page.locator('#confirmOkBtn').click();
  await expect.poll(async () => (await api(page, '/api/issues/' + created.id)).issue.status).toMatch(/closed|done/);
  await expect(drawer.locator('[data-reopen-issue]')).toBeVisible();
  await drawer.locator('[data-reopen-issue]').click();
  await page.locator('#confirmOkBtn').click();
  await expect.poll(async () => (await api(page, '/api/issues/' + created.id)).issue.status).not.toBe('closed');
  expect(errors).toEqual([]);
});
