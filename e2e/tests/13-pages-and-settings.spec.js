const { test, expect } = require('@playwright/test');
const { ADMIN, watchErrors, login } = require('./helpers');

test('the home page is the chat: no dashboard, attachments, @ mentions and no surprise runs', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  await expect(page.locator('#pageTitle')).toHaveText('首页');
  await expect(page.locator('#dashboardView')).toHaveCount(0);
  const home = page.locator('#homeChat');
  await expect(home.locator('.hc-brand')).toHaveText('LoopForge');
  await expect(page.locator('#homeChatInput')).toBeVisible();
  await expect(home.locator('#hcRuns')).toContainText('正在运行');
  await page.goto('/#/dashboard');
  await expect(page.locator('#pageTitle')).toHaveText('首页');

  const cards = home.locator('[data-home-sop]');
  expect(await cards.count()).toBeGreaterThan(1);
  await cards.nth(1).click();
  await expect(home.locator('[data-home-sop]').nth(1)).toHaveClass(/is-selected/);

  // Attach a file through the picker; it uploads right away and shows in the tray.
  await page.locator('#homeChatFile').setInputFiles({ name: 'notes.md', mimeType: 'text/markdown', buffer: Buffer.from('# 备注\n导出要带表头') });
  await expect(home.locator('.hc-file-chip')).toContainText('notes.md');
  await expect(home.locator('.hc-file-chip')).not.toHaveClass(/is-uploading/);

  // @ opens the requirement picker; picking one adds a reference chip.
  await page.locator('#homeChatInput').fill('看下 @');
  await expect(home.locator('#homeMention')).toBeVisible();
  const first = home.locator('#homeMention [data-mention]').first();
  if (await first.count()) {
    await page.locator('#homeChatInput').press('Enter');
    await expect(home.locator('.hc-tray .hc-ref-chip')).toHaveCount(1);
  }

  // Without a model key a message gets an explanation, never a surprise run.
  const runsBefore = await page.evaluate(() => (state.data.runs || []).length);
  await page.locator('#homeChatInput').fill('你好');
  await page.locator('#homeChatInput').press('Enter');
  await expect(home.locator('.hc-msg.is-user .hc-bubble')).toHaveText('你好');
  await expect(home.locator('.hc-msg.is-user .hc-file')).toContainText('notes.md');
  await expect(home.locator('.hc-msg.is-bot').last()).toContainText('还没配置 CodeBuddy Key');
  await expect(home.locator('.hc-msg.is-bot').last().locator('[data-chat-action="settings-models"]')).toBeVisible();
  expect(await page.evaluate(() => (state.data.runs || []).length)).toBe(runsBefore);
  await expect(page.locator('#issueDetailDrawer')).toBeHidden();

  await home.locator('#homeChatNew').click();
  await expect(home.locator('.hc-brand')).toBeVisible();
  await expect(page.locator('.nav[data-view="chat"]')).toHaveClass(/active/);

  // The top-bar AI assistant button lands here too.
  await page.locator('.nav[data-view="issues"]').click();
  await page.locator('.ai-launch').click();
  await expect(page.locator('#pageTitle')).toHaveText('首页');
  await expect(page.locator('#aiAssistant')).toBeHidden();
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
  await expect(page.locator('#pageTitle')).toHaveText('首页');
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
