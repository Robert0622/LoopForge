const { test, expect } = require('@playwright/test');
const { watchErrors, login, openView } = require('./helpers');

const SIDEBAR = ['chat', 'rooms', 'issues', 'sops', 'agents', 'sandboxes', 'metrics', 'observability', 'knowledge', 'settings'];
const SUBVIEWS = ['runs', 'approvals', 'flow', 'skills', 'mcp', 'rules', 'dockerfiles', 'environments', 'projects', 'runners', 'guide', 'businessDomains', 'businessDomainManagement', 'platform'];

test('every view renders without errors or failed requests', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  const plugins = await page.$$eval('#pluginNavGroup [data-view]', els => els.map(e => e.dataset.view));
  const views = [...new Set([...SIDEBAR, ...SUBVIEWS, ...plugins])];
  expect(views.length).toBeGreaterThanOrEqual(15);
  for (const view of views) {
    await test.step(view, async () => {
      const nav = page.locator(`.nav[data-view="${view}"]:visible`);
      if (await nav.count()) await nav.first().click();
      else await openView(page, view);
      const container = ['rules', 'skills', 'dockerfiles', 'environments', 'sandboxes'].includes(view) ? 'assets' : view;
      await expect(page.locator(`#${container}View`)).toBeVisible();
      await page.waitForLoadState('networkidle');
    });
  }
  expect(errors).toEqual([]);
});

test('the MCP market lists public servers and GitHub is installed by default', async ({ page }) => {
  await login(page);
  await openView(page, 'mcp');
  await expect(page.locator('#mcpView')).toContainText('GitHub');
  await expect(page.locator('#mcpView')).toContainText('Context7');
  await expect(page.locator('#mcpView')).toContainText('已添加');
});
