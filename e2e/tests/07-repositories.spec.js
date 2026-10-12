const { test, expect } = require('@playwright/test');
const { watchErrors, login } = require('./helpers');

test('a GitHub repository can be added to the project and becomes its default', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);

  await page.locator('#manageRepositories').click();
  const modal = page.locator('#repositoryModal');
  await expect(modal).toHaveClass(/open/);
  await modal.locator('[name=repoUrl]').fill('git@github.com:octo-org/hello-world.git');
  await modal.locator('[name=defaultBranch]').fill('main');
  await modal.locator('[name=makeDefault]').check();
  await modal.locator('button[type=submit]').click();

  await expect(page.locator('#toastContainer')).toContainText('仓库绑定已保存');
  await expect(modal).toContainText('octo-org/hello-world');
  const saved = await page.evaluate(async () => {
    const repos = await fetch('/api/repositories').then(r => r.json());
    const projects = await fetch('/api/projects').then(r => r.json());
    return { repo: repos.find(r => r.repoPath === 'octo-org/hello-world'), projects };
  });
  expect(saved.repo).toBeTruthy();
  expect(saved.repo.webUrl).toBe('https://github.com/octo-org/hello-world');
  expect(saved.projects.some(p => p.defaultRepositoryId === saved.repo.id)).toBe(true);
  expect(errors).toEqual([]);
});
