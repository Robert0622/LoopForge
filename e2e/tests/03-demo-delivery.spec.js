const { test, expect } = require('@playwright/test');
const { watchErrors, login, visibleButton } = require('./helpers');

test('a requirement runs through the demo SOP: dispatch, approval gate, artifacts', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);

  await page.locator('.nav[data-view="issues"]').click();
  await page.locator('#newIssue').click();
  await expect(page.locator('#reqWorkflow')).toHaveValue('demo-delivery');
  await page.locator('#chatInput').fill('演示：需求列表支持导出 CSV');
  await page.locator('#chatCreateDirect').click();
  await visibleButton(page, '跳过分析并建单').click();
  await page.locator('#chatCreateLocal').click();

  const drawer = page.locator('#issueDetailDrawer');
  await expect(drawer).toContainText('演示：需求列表支持导出 CSV');
  await visibleButton(page.locator('#issueDetailDrawer'), '触发').click();

  await expect(visibleButton(page, '确认通过，继续下游')).toBeVisible({ timeout: 30_000 });
  await expect(drawer).toContainText('待审批');
  await visibleButton(page, '确认通过，继续下游').click();

  await expect(drawer).toContainText('已完成', { timeout: 30_000 });
  await expect.poll(async () => {
    const runs = await page.evaluate(() => fetch('/api/runs').then(r => r.json()));
    return runs[0]?.status;
  }, { timeout: 30_000 }).toBe('succeeded');

  await visibleButton(page, '产物').click();
  await expect(drawer).toContainText('交付报告');
  await expect(drawer).toContainText('delivery-report.md');
  expect(errors).toEqual([]);
});
