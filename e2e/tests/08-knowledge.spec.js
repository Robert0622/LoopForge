const { test, expect } = require('@playwright/test');
const { watchErrors, login } = require('./helpers');

test('an admin connects a knowledge base without the token appearing in the page', async ({ page }) => {
  const errors = watchErrors(page, { allow: [/HTTP 400 PUT \/api\/knowledge\/settings/, /status of 400/] });
  await login(page);

  await page.locator('.nav[data-view="knowledge"]').click();
  const view = page.locator('#knowledgeView');
  await expect(view).toBeVisible();
  await expect(page.locator('#pageTitle')).toHaveText('知识中心');
  await expect(view).toContainText('未接入');
  for (const tab of ['接入配置', '节点使用', '知识沉淀', '检索试用']) {
    await expect(view.locator(`[data-kb-tab]`).filter({ hasText: tab })).toBeVisible();
  }
  await expect(view).toContainText('kb_search');
  await expect(view).toContainText('knowledge.md');

  const form = view.locator('#kbSettingsForm');
  await form.locator('[name=url]').fill('ftp://kb.example.com/mcp');
  await form.locator('[type=submit]').click();
  await expect(page.locator('#toastContainer')).toContainText('知识库地址必须是 http(s)://host/mcp 形式');

  const secret = 'e2e-kb-token-should-not-render';
  await form.locator('[name=url]').fill('http://kb.example.com/mcp');
  await form.locator('[name=token]').fill(secret);
  await form.locator('[name=enabled]').uncheck();
  await form.locator('[type=submit]').click();
  await expect(page.locator('#toastContainer')).toContainText('知识库配置已保存');
  await expect(view).toContainText('未启用');
  await expect(view.locator('.kb-pill').filter({ hasText: '已配置' })).toBeVisible();
  await expect(view).not.toContainText(secret);

  await page.reload();
  await expect(view).toBeVisible();
  await expect(view).toContainText('http://kb.example.com/mcp');
  await expect(view).not.toContainText(secret);

  await view.locator('[data-kb-tab="distill"]').click();
  await expect(view).toContainText('knowledge.md');
  await view.locator('[data-kb-tab="search"]').click();
  await expect(view.locator('#kbSearchForm')).toBeVisible();
  expect(errors).toEqual([]);
});
