const { test, expect } = require('@playwright/test');
const { ADMIN, watchErrors, login } = require('./helpers');

test('the dashboard and the requirement chat are separate pages', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  await expect(page.locator('#pageTitle')).toHaveText('工作总览');
  await expect(page.locator('#dashboardView #homeChat')).toHaveCount(0);
  await expect(page.locator('#homeChat')).toBeHidden();
  await expect(page.locator('#dashboardView .home-chat-side')).toHaveCount(0);

  await page.locator('#dashNewRun').click();
  await expect(page.locator('#pageTitle')).toHaveText('发起需求');
  await expect(page).toHaveURL(/#\/chat$/);
  await expect(page.locator('#homeChat')).toBeVisible();
  await expect(page.locator('#homeChatInput')).toBeVisible();
  await expect(page.locator('#homeChat .home-chat-empty')).toContainText('想改点什么');
  await page.locator('[data-home-example]').first().click();
  await expect(page.locator('#homeChatInput')).toHaveValue('给需求列表加导出 CSV');
  await page.locator('#homeChatInput').fill('');
  await expect(page.locator('.nav[data-view="chat"]')).toHaveClass(/active/);

  await page.reload();
  await expect(page.locator('#homeChat')).toBeVisible();
  await page.locator('.nav[data-view="dashboard"]').click();
  await expect(page.locator('#homeChat')).toBeHidden();
  expect(errors).toEqual([]);
});

test('personal settings split into tabs that keep unsaved input and survive a reload', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  await page.locator('.nav[data-view="settings"]').click();
  await expect(page).toHaveURL(/#\/settings\/models$/);
  await expect(page.locator('[data-settings-panel="models"]')).toBeVisible();
  await expect(page.locator('[data-settings-panel="git"]')).toBeHidden();
  await expect(page.locator('#userPasswordForm')).toBeHidden();

  await page.locator('#userCredentialForm [name=codexBaseUrl]').fill('https://example.invalid/v1');
  await page.locator('[data-settings-tab="git"]').click();
  await expect(page).toHaveURL(/#\/settings\/git$/);
  await expect(page.locator('#userCredentialForm [name=gitToken]')).toBeVisible();
  await expect(page.locator('#userCredentialForm [name=codexBaseUrl]')).toBeHidden();
  await expect(page.locator('#userCredentialForm .credential-savebar')).toHaveClass(/is-dirty/);
  await page.locator('[data-settings-tab="models"]').click();
  await expect(page.locator('#userCredentialForm [name=codexBaseUrl]')).toHaveValue('https://example.invalid/v1');
  await page.locator('#userCredentialForm [name=codexBaseUrl]').fill('');

  await page.locator('[data-settings-tab="mcp"]').click();
  await page.reload();
  await expect(page.locator('#userName')).toBeVisible();
  await expect(page.locator('#settingsView .settings-tab.active')).toHaveText('MCP 令牌');
  await expect(page.locator('[data-settings-panel="mcp"]')).toBeVisible();

  // Tabs replace the history entry, so Back leaves settings instead of stepping through tabs.
  await page.goBack();
  await expect(page.locator('#pageTitle')).toHaveText('工作总览');
  expect(errors).toEqual([]);
});

test('the account tab changes the password and signs out', async ({ page }) => {
  const errors = watchErrors(page, { allow: [/HTTP 401 GET \/api\/auth\/current/] });
  await login(page);
  await page.evaluate(() => { location.hash = '#/settings/account'; });
  const form = page.locator('#userPasswordForm');
  await expect(form).toBeVisible();
  await expect(page.locator('#userCredentialForm')).toBeHidden();
  await expect(page.locator('#settingsView')).toContainText('平台管理员');

  const change = async (from, to) => {
    await form.locator('[name=currentPassword]').fill(from);
    await form.locator('[name=newPassword]').fill(to);
    await form.locator('[type=submit]').click();
    await expect(page.locator('#toastContainer')).toContainText('密码已修改');
    await page.locator('#toastContainer .toast-item').evaluateAll(nodes => nodes.forEach(n => n.remove()));
  };
  await change(ADMIN.password, 'temporary-pass-1');
  await change('temporary-pass-1', ADMIN.password);

  await Promise.all([page.waitForNavigation(), page.locator('#settingsLogout').click()]);
  await expect(page.locator('#loginModal')).toHaveClass(/open/);
  expect(errors).toEqual([]);
});
