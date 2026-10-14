const { test, expect } = require('@playwright/test');
const { watchErrors, login } = require('./helpers');

// e2e/serve.sh loads everything under examples/extensions.

test('a web plugin adds a working page that survives a reload', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);

  const nav = page.locator('#pluginNavGroup .nav[data-view="delivery-board"]');
  await expect(page.locator('#pluginNavGroup')).toContainText('扩展');
  await expect(nav).toContainText('大盘');
  await nav.click();

  const view = page.locator('#delivery-boardView');
  await expect(view).toBeVisible();
  await expect(page.locator('#pageTitle')).toHaveText('大盘');
  await expect(view).toContainText('次运行');
  await expect(view.locator('.db-col')).toHaveCount(5);
  expect(await view.locator('.db-board').evaluate(el => getComputedStyle(el).display)).toBe('grid');

  await page.reload();
  await expect(view).toBeVisible();
  await expect(nav).toHaveClass(/active/);

  await page.locator('.nav[data-view="issues"]').click();
  await expect(view).toBeHidden();
  await page.goBack();
  await expect(view).toBeVisible();
  expect(errors).toEqual([]);
});

test('an operator SOP template can be copied with its roles', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);

  await page.locator('.nav[data-view="sops"]').click();
  await page.locator('[data-scope-for="sop"] [data-scope="generic"]').click();
  const card = page.locator('.sop-card.generic[data-adopt-generic="docs-update"]');
  await expect(card).toContainText('文档更新 SOP');
  await card.locator('.adopt-action').click();
  await expect(page.locator('#genericTemplateModal')).toHaveClass(/open/);
  await page.locator('#confirmGenericTemplate').click();

  const flow = page.locator('#flowView');
  await expect(flow).toBeVisible();
  await expect(flow).toContainText('文档工程师');
  await expect(flow).toContainText('文档审查员');
  await expect(flow).not.toContainText('角色不可用');
  expect(errors).toEqual([]);
});

test('custom MCP market entries are listed and searchable', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);

  const market = await page.evaluate(() => fetch('/api/mcp-market?q=kb').then(r => r.json()));
  expect(market.items.map(i => i.id)).toContain('team-knowledge');

  if (await page.locator('[data-nav-group="config"]').getAttribute('aria-expanded') !== 'true') await page.locator('[data-nav-group="config"]').click();
  await page.locator('.nav[data-view="mcp"]').click();
  await page.locator('#mcpMarketSearch').fill('Team knowledge');
  const card = page.locator('.mcp-market-card').filter({ hasText: 'Team knowledge base (example)' });
  await expect(card).toBeVisible();
  await expect(card.locator('a', { hasText: '查看详情' })).toHaveCount(0);
  await card.locator('[data-mcp-install]').click();
  await expect(card.locator('[data-mcp-install]')).toBeDisabled();
  expect(errors).toEqual([]);
});
