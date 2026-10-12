const { test, expect } = require('@playwright/test');
const { watchErrors, login, visibleButton, api, waitForRun, closeDrawer, useScriptProject } = require('./helpers');

async function createBlankSOP(page, name) {
  await useScriptProject(page);
  await page.locator('.nav[data-view="sops"]').click();
  await page.locator('#newSOP').click();
  const form = page.locator('#sopCreateForm');
  await expect(form).toBeVisible();
  await form.locator('[name=name]').fill(name);
  await page.locator('#sopNext').click();
  await form.locator('.template-choice', { hasText: '从空白开始' }).click();
  await page.locator('#sopFinish').click();
  await expect(page.locator('#flowView')).toBeVisible();
  await expect(page.locator('#toastContainer')).toContainText('SOP 草稿已创建');
}

async function writeScript(page, script) {
  await page.locator('[data-add="script"]').click();
  await page.locator('#flowOutline .flow-step').last().click();
  await page.locator('#properties [data-prop-tab="task"]').click();
  await page.locator('#openScriptNodeEditor').click();
  await page.locator('#codeEditorTextarea').fill(script);
  await page.locator('#codeEditorSave').click();
  await expect(page.locator('#codeEditorModal')).not.toHaveClass(/open/);
}

async function publish(page) {
  await page.locator('#publishWorkflow').click();
  await expect(page.locator('#toastContainer')).toContainText('已发布');
  return page.locator('#workflowSelect').inputValue();
}

test('a shell SOP designed in the editor runs in the local sandbox and keeps its artifact', async ({ page }) => {
  test.setTimeout(90_000);
  const errors = watchErrors(page);
  await login(page);
  await createBlankSOP(page, '浏览器脚本交付');
  await writeScript(page, "printf 'e2e-browser-marker\\n' > marker.txt\n");
  await page.evaluate(() => {
    const node = state.workflow.nodes.find(item => item.type === 'script');
    node.artifactOutputs = [{ name: 'marker', path: 'marker.txt', mediaType: 'text/plain', required: true }];
  });
  const workflowId = await publish(page);

  const drawer = await createRequirement(page, workflowId, '浏览器：脚本 SOP 真跑');
  await visibleButton(page, '触发').click();
  await expect(drawer).toContainText('已完成', { timeout: 30_000 });

  const run = await page.evaluate(async title => {
    const issues = await fetch('/api/issues').then(res => res.json());
    const issue = issues.find(item => item.title === title);
    const tasks = await fetch('/api/tasks').then(res => res.json());
    const task = tasks.find(item => item.issueId === issue.id);
    return fetch('/api/runs/' + encodeURIComponent(task.runId)).then(res => res.json());
  }, '浏览器：脚本 SOP 真跑');
  expect(run.status).toBe('succeeded');
  const marker = (run.artifacts || []).find(item => item.name === 'marker');
  expect(marker).toBeTruthy();
  const artifact = await page.evaluate(async ({ id, artifactId }) => {
    const res = await fetch('/api/runs/' + encodeURIComponent(id) + '/artifacts/' + encodeURIComponent(artifactId));
    return res.text();
  }, { id: run.id, artifactId: marker.id });
  expect(artifact).toContain('e2e-browser-marker');

  await closeDrawer(page);
  await page.locator('.nav[data-view="runs"]').click();
  await expect(page.locator('#runList')).toContainText('浏览器：脚本 SOP 真跑');
  await page.locator('.run-card', { hasText: '浏览器：脚本 SOP 真跑' }).click();
  await expect(page.locator('#runsDetailSection, #runDetail, .run-detail-main').first()).toBeVisible();
  expect(errors).toEqual([]);
});

test('an approval gate stops a real run until 待我确认 lets it continue', async ({ page }) => {
  test.setTimeout(90_000);
  const errors = watchErrors(page);
  await login(page);
  await createBlankSOP(page, '浏览器审批交付');
  await writeScript(page, "printf 'before-gate\\n'\n");
  await page.locator('[data-add="approval"]').click();
  await page.locator('#flowOutline .flow-step').first().click();
  await visibleButton(page, '连接到下一步').click();
  await page.locator('#flowOutline .flow-step').last().click();
  await page.locator('[data-add="script"]').click();
  await page.locator('#flowOutline .flow-step').nth(1).click();
  await visibleButton(page, '连接到下一步').click();
  await page.locator('#flowOutline .flow-step').last().click();
  const workflowId = await publish(page);

  const drawer = await createRequirement(page, workflowId, '浏览器：审批后继续');
  await visibleButton(page, '触发').click();
  await expect(drawer).toContainText('待审批', { timeout: 30_000 });
  const issue = (await api(page, '/api/issues')).find(item => item.title === '浏览器：审批后继续');
  const task = (await api(page, '/api/tasks')).find(item => item.issueId === issue.id);

  await closeDrawer(page);
  await page.locator('.nav[data-view="approvals"]').click();
  const card = page.locator('#approvalList .ap-card', { hasText: '浏览器：审批后继续' });
  await expect(card).toBeVisible();
  await card.locator('button', { hasText: '确认通过，继续下游' }).click();
  const run = await waitForRun(page, task.runId, /succeeded/);
  expect(run.status).toBe('succeeded');
  expect(errors).toEqual([]);
});

test('a script that exits non-zero fails the run and can be cancelled while sleeping', async ({ page }) => {
  test.setTimeout(90_000);
  const errors = watchErrors(page);
  await login(page);

  await createBlankSOP(page, '浏览器失败脚本');
  await writeScript(page, 'echo e2e-browser-failed; exit 1\n');
  const failId = await publish(page);
  const drawer = await createRequirement(page, failId, '浏览器：脚本失败');
  await visibleButton(page, '触发').click();
  await expect(drawer).toContainText('失败', { timeout: 30_000 });
  const failedIssue = (await api(page, '/api/issues')).find(item => item.title === '浏览器：脚本失败');
  const failedTask = (await api(page, '/api/tasks')).find(item => item.issueId === failedIssue.id);
  const failed = await waitForRun(page, failedTask.runId, /failed/);
  const failedText = JSON.stringify(failed.nodes || {});
  expect(failedText).toMatch(/e2e-browser-failed|exit status 1/);
  expect(failedText).not.toContain('Repository not found');
  await closeDrawer(page);

  await createBlankSOP(page, '浏览器取消脚本');
  await writeScript(page, 'sleep 30\n');
  const cancelId = await publish(page);
  await createRequirement(page, cancelId, '浏览器：取消睡眠');
  await visibleButton(page, '触发').click();
  const issue = (await api(page, '/api/issues')).find(item => item.title === '浏览器：取消睡眠');
  const task = (await api(page, '/api/tasks')).find(item => item.issueId === issue.id);
  await waitForRun(page, task.runId, /running/);
  await closeDrawer(page);
  await page.locator('.nav[data-view="runs"]').click();
  await page.locator('.run-card', { hasText: '浏览器：取消睡眠' }).click();
  await page.locator('[data-cancel-run]').click();
  await page.locator('#confirmOkBtn').click();
  const cancelled = await waitForRun(page, task.runId, /cancelled/);
  expect(cancelled.status).toBe('cancelled');
  expect(errors).toEqual([]);
});

async function createRequirement(page, workflowId, title) {
  const { visibleButton } = require('./helpers');
  await page.locator('.nav[data-view="dashboard"]').click();
  await page.locator('#dashNewRequirement').click();
  await page.locator('#reqWorkflow').selectOption(workflowId);
  await page.locator('#chatInput').fill(title);
  await page.locator('#chatCreateDirect').click();
  await visibleButton(page, '跳过分析并建单').click();
  await page.locator('#chatCreateLocal').click();
  const drawer = page.locator('#issueDetailDrawer');
  await expect(drawer).toContainText(title);
  return drawer;
}
