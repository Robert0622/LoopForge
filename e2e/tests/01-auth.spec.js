const { test, expect } = require('@playwright/test');
const { ADMIN, watchErrors, login } = require('./helpers');

test('a first-time visitor gets a login form that cannot be dismissed', async ({ page }) => {
  const errors = watchErrors(page, { allow: [/HTTP 401 GET \/api\/auth\/current/] });
  await page.goto('/');
  const modal = page.locator('#loginModal');
  await expect(modal).toHaveClass(/open/);
  await expect(page.locator('#loginForm [name=loginName]')).toBeVisible();
  await expect(modal.locator('.login-close:visible')).toHaveCount(0);
  await expect(page.locator('body')).not.toContainText('启动失败');
  await page.keyboard.press('Escape');
  await expect(modal).toHaveClass(/open/);
  expect(errors).toEqual([]);
});

test('a wrong password shows an error and keeps the form open', async ({ page }) => {
  await page.goto('/');
  const form = page.locator('#loginForm');
  await form.locator('[name=loginName]').fill(ADMIN.login);
  await form.locator('[name=password]').fill('definitely-wrong');
  await form.locator('[type=submit]').click();
  await expect(form.locator('.login-error')).toBeVisible();
  await expect(page.locator('#loginModal')).toHaveClass(/open/);
});

test('the admin can sign in and lands on the home chat', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  await expect(page.locator('#userName')).toContainText('Administrator');
  await expect(page.locator('#chatView')).toBeVisible();
  await expect(page.locator('#pageTitle')).toHaveText('首页');
  await expect(page.locator('.nav[data-view="dashboard"]')).toHaveCount(0);
  await expect(page.locator('#sidebarSyncState')).toHaveText('本地 SQLite');
  expect(errors).toEqual([]);
});

test('signing out returns to the login form', async ({ page }) => {
  await login(page);
  await page.locator('#userBadge').click();
  await Promise.all([page.waitForNavigation(), page.locator('#uppLogoutBtn').click()]);
  await expect(page.locator('#loginModal')).toHaveClass(/open/);
  await expect(page.locator('#loginModal .login-close:visible')).toHaveCount(0);
});

test('an expired session brings the login form back instead of a broken page', async ({ page, context }) => {
  await login(page);
  await context.clearCookies();
  await page.locator('[data-view="issues"]:visible').first().click();
  await expect(page.locator('#loginModal')).toHaveClass(/open/);
  await expect(page.locator('body')).not.toContainText('启动失败');
});

test('a visitor registers from the login form and lands as a regular member', async ({ page }) => {
  const errors = watchErrors(page, { allow: [/HTTP 401 GET \/api\/auth\/current/] });
  await page.addInitScript(() => localStorage.setItem('loopforge.onboarding.v3', 'done'));
  await page.goto('/');
  const form = page.locator('#loginForm');
  await form.locator('[data-login-mode="register"]').click();
  await expect(form.locator('.login-title')).toHaveText('注册账号');
  await form.locator('[name=loginName]').fill('carol');
  await form.locator('[name=displayName]').fill('Carol');
  await form.locator('[name=password]').fill('short');
  await form.locator('[type=submit]').click();
  await expect(page.locator('#loginModal')).toHaveClass(/open/);
  await form.locator('[name=password]').fill('carol-password-1');
  await Promise.all([page.waitForNavigation(), form.locator('[type=submit]').click()]);
  await expect(page.locator('#userName')).toContainText('Carol');
  await expect(page.locator('.nav[data-view="platform"]')).toBeHidden();
  expect(errors).toEqual([]);
});

test('an admin closes sign-up and the login form stops offering it', async ({ page, browser }) => {
  const errors = watchErrors(page);
  await login(page);
  await page.locator('[data-view="platform"]:visible').click();
  const toggle = page.locator('#platformSignupOpen');
  await expect(toggle).toBeChecked();
  await toggle.uncheck();
  await expect(page.locator('#toastContainer')).toContainText('已关闭注册');

  const guest = await browser.newContext();
  const guestPage = await guest.newPage();
  await guestPage.goto('/');
  await expect(guestPage.locator('#loginForm')).toBeVisible();
  await expect(guestPage.locator('#loginForm .login-switch')).toContainText('联系平台管理员');
  await expect(guestPage.locator('[data-login-mode="register"]')).toHaveCount(0);
  const status = await guestPage.evaluate(() => fetch('/api/auth/register', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ loginName: 'mallory', password: 'mallory-password' }),
  }).then(r => r.status));
  expect(status).toBe(403);
  await guest.close();

  await toggle.check();
  await expect(page.locator('#toastContainer')).toContainText('已开放注册');
  expect(errors).toEqual([]);
});
