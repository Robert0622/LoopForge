const { test, expect } = require('@playwright/test');
const { watchErrors, login } = require('./helpers');

test('every view renders without errors or failed requests', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  const views = await page.$$eval('[data-view]', els => [...new Set(els.map(e => e.dataset.view))]);
  expect(views.length).toBeGreaterThanOrEqual(15);
  for (const view of views) {
    await test.step(view, async () => {
      const nav = page.locator(`[data-view="${view}"]:visible`).first();
      if (await nav.count()) await nav.click();
      else await page.evaluate(v => { location.hash = `#/${v}`; }, view);
      const container = ['rules', 'skills', 'dockerfiles', 'environments', 'sandboxes'].includes(view) ? 'assets' : view;
      await expect(page.locator(`#${container}View`)).toBeVisible();
      await page.waitForLoadState('networkidle');
    });
  }
  expect(errors).toEqual([]);
});

test('the MCP market lists public servers and GitHub is installed by default', async ({ page }) => {
  await login(page);
  await page.evaluate(() => { location.hash = '#/mcp'; });
  await expect(page.locator('#mcpView')).toContainText('GitHub');
  await expect(page.locator('#mcpView')).toContainText('Context7');
  await expect(page.locator('#mcpView')).toContainText('已添加');
});
