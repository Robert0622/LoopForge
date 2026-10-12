const { test, expect } = require('@playwright/test');
const { watchErrors, login } = require('./helpers');

test('a platform admin creates an account that can then sign in', async ({ page, browser }) => {
  const errors = watchErrors(page);
  await login(page);
  await page.locator('[data-view="platform"]:visible').click();
  const form = page.locator('#platformAccountForm');
  await form.locator('[name=loginName]').fill('bob');
  await form.locator('[name=displayName]').fill('Bob');
  await form.locator('[name=password]').fill('bob-password-1');
  await form.locator('[type=submit]').click();
  await expect(form.locator('[name=loginName]')).toHaveValue('');
  expect(errors).toEqual([]);

  const other = await browser.newContext();
  const bobPage = await other.newPage();
  await login(bobPage, { login: 'bob', password: 'bob-password-1' });
  await expect(bobPage.locator('#userName')).toContainText('Bob');
  await other.close();
});
