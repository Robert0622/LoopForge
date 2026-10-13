const { test, expect } = require('@playwright/test');
const { watchErrors } = require('./helpers');

// A second server starts with no account at all (see playwright.config.js);
// the first visitor creates the admin there.
const setupURL = process.env.E2E_SETUP_URL || (process.env.E2E_BASE_URL ? '' : `http://127.0.0.1:${process.env.E2E_SETUP_PORT || '18498'}`);
test.skip(!setupURL, 'needs a fresh instance without an admin password');
test.use({ baseURL: setupURL });

test('the first visitor creates the admin, takes the tour and lands on settings', async ({ page, browser }) => {
  const errors = watchErrors(page);
  await page.goto('/');
  const setup = page.locator('#setupForm');
  await expect(setup).toBeVisible();
  await expect(page.locator('#loginModal.open')).toHaveCount(0);
  await setup.locator('[name=loginName]').fill('alice');
  await setup.locator('[name=displayName]').fill('Alice');
  await setup.locator('[name=password]').fill('alice-password');
  await setup.locator('[name=confirm]').fill('alice-passwrod');
  await setup.locator('[type=submit]').click();
  await expect(setup.locator('.login-error')).toHaveText('两次输入的密码不一致');
  await setup.locator('[name=confirm]').fill('alice-password');
  await Promise.all([page.waitForNavigation(), setup.locator('[type=submit]').click()]);

  await expect(page.locator('#userName')).toContainText('Alice');
  await expect(page.locator('#onboardingKicker')).toHaveText('1 / 6');
  await page.locator('#onboardingSkip').click();
  await expect(page.locator('#pageTitle')).toHaveText('个人设置');
  await expect(page).toHaveURL(/#\/settings\/models$/);
  await expect(page.locator('.nav[data-view="platform"]')).toBeVisible();
  const projects = await page.evaluate(() => fetch('/api/projects').then(r => r.json()));
  expect(projects.find(p => p.id === 'platform').ownerId).toBeGreaterThan(0);

  const late = await browser.newContext({ baseURL: setupURL });
  const latePage = await late.newPage();
  await latePage.goto('/');
  await expect(latePage.locator('#loginForm')).toBeVisible();
  await expect(latePage.locator('#setupForm')).toHaveCount(0);
  const again = await latePage.evaluate(() => fetch('/api/auth/setup', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ loginName: 'eve', password: 'eve-password' }),
  }).then(r => r.status));
  expect(again).toBe(409);
  await late.close();
  expect(errors).toEqual([]);
});
