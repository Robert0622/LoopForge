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

test('the admin can sign in and lands on the dashboard', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);
  await expect(page.locator('#userName')).toContainText('Administrator');
  await expect(page.locator('#dashboardView')).toBeVisible();
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
