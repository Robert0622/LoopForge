const { test, expect } = require('@playwright/test');
const { watchErrors, login, visibleButton } = require('./helpers');

async function createRequirement(page, workflowId, title) {
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

test('a copied built-in template resolves its roles and explains missing credentials', async ({ page }) => {
  // The copied SOP runs a real CLI executor, so its first node fails without an API key.
  const errors = watchErrors(page);
  await login(page);

  await page.locator('.nav[data-view="sops"]').click();
  await page.locator('[data-scope-for="sop"] [data-scope="generic"]').click();
  await page.locator('.sop-card.generic[data-adopt-generic="generic-backend-delivery"] .adopt-action').click();
  await expect(page.locator('#genericTemplateModal')).toHaveClass(/open/);
  await page.locator('#confirmGenericTemplate').click();

  await expect(page.locator('#flowView')).toBeVisible();
  await expect(page.locator('#flowView')).toContainText('需求负责人');
  await expect(page.locator('#flowView')).not.toContainText('角色不可用');

  const copied = await page.evaluate(() => fetch('/api/workflows').then(r => r.json())
    .then(list => list.find(w => w.name === '后端交付 SOP' && w.projectId)));
  expect(copied).toBeTruthy();

  const drawer = await createRequirement(page, copied.id, '模板：给 README 增加安装说明');
  await visibleButton(page, '触发').click();

  const failure = drawer.locator('.node-progress-error').first();
  await expect(failure).toContainText('API Key', { timeout: 30_000 });
  await failure.locator('[data-goto-settings]').click();
  await expect(drawer).toBeHidden();
  await expect(page.locator('#settingsView')).toBeVisible();
  expect(errors).toEqual([]);
});

test('a dispatch that fails to start shows the reason and can be retried', async ({ page }) => {
  // Generic templates need a code repository; the demo project has none.
  const errors = watchErrors(page, { allow: [/HTTP 4\d\d POST \/api\/issues\/[^/]+\/dispatch/, /status of 4\d\d/] });
  await login(page);

  const drawer = await createRequirement(page, 'generic-backend-delivery', '模板：没有仓库时派发');
  const toast = page.locator('#toastContainer');
  for (let attempt = 0; attempt < 2; attempt++) {
    await visibleButton(page, '触发').click();
    await expect(toast).toContainText('仓库', { timeout: 10_000 });
    await expect(toast).not.toContainText('已经跑过');
    await expect(drawer).toBeVisible();
    await page.locator('#toastContainer .toast-item').evaluateAll(nodes => nodes.forEach(n => n.remove()));
  }
  expect(errors).toEqual([]);
});
