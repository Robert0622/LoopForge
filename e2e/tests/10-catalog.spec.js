const { test, expect } = require('@playwright/test');
const { watchErrors, login, api, openView } = require('./helpers');

async function saveAsset(page, view, fields) {
  await openView(page, view);
  await expect(page.locator('#assetsView')).toBeVisible();
  await page.locator('#newAsset').click();
  const form = page.locator('#assetForm');
  for (const [name, value] of Object.entries(fields)) {
    await form.locator(`[name=${name}]`).fill(value);
  }
  await form.locator('[type=submit]').click();
  await expect(page.locator('#toastContainer')).toContainText(/已|保存|创建/);
}

test('roles, rules, skills, dockerfiles and sandboxes round-trip in the UI', async ({ page }) => {
  test.setTimeout(90_000);
  const errors = watchErrors(page);
  await login(page);

  await page.locator('.nav[data-view="agents"]').click();
  await page.locator('#newAgent').click();
  const agent = page.locator('#agentForm');
  await expect(agent).toBeVisible();
  await agent.locator('[name=name]').fill('浏览器角色');
  await agent.locator('[name=id]').fill('browser-role');
  await agent.locator('[name=executorId]').selectOption({ index: 0 });
  await agent.locator('[name=description]').fill('端到端创建的角色，不参与模型调用');
  await page.locator('#agentSave').click();
  await expect(page.locator('#toastContainer')).toContainText(/保存|已/);
  await expect(page.locator('#agentList')).toContainText('浏览器角色');

  await saveAsset(page, 'rules', { id: 'browser-rule', name: '浏览器规则', content: '产物必须落在工作区。' });
  await expect(page.locator('#assetList')).toContainText('浏览器规则');
  await saveAsset(page, 'skills', { id: 'browser-skill', name: '浏览器技能', description: '端到端技能' });
  await page.locator('#skillMdPick').setInputFiles({ name: 'SKILL.md', mimeType: 'text/markdown', buffer: Buffer.from('# 技能\n\n检查工作区。') });
  await page.locator('#confirmOkBtn').click();
  await expect(page.locator('#toastContainer')).toContainText('已覆盖为仅 SKILL.md');
  await expect(page.locator('#assetList')).toContainText('浏览器技能');
  await saveAsset(page, 'dockerfiles', { id: 'browser-dockerfile', name: '浏览器 Dockerfile', content: 'FROM scratch\n' });
  await expect(page.locator('#assetList')).toContainText('浏览器 Dockerfile');
  await saveAsset(page, 'sandboxes', { id: 'browser-sandbox', name: '浏览器沙箱' });
  await expect(page.locator('#assetList')).toContainText('浏览器沙箱');

  await page.reload();
  await expect(page.locator('#userName')).toBeVisible();
  const agents = await api(page, '/api/agents');
  expect(agents.some(item => item.id === 'browser-role')).toBeTruthy();
  const rules = await api(page, '/api/rules');
  expect(rules.some(item => item.id === 'browser-rule')).toBeTruthy();
  const skills = await api(page, '/api/skills');
  const skill = skills.find(item => item.id === 'browser-skill');
  expect(JSON.stringify(skill)).toContain('SKILL.md');
  expect(errors).toEqual([]);
});

test('a project, a custom MCP and the personal pages render their working controls', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);

  await page.locator('.nav[data-view="projects"]').click();
  await page.locator('#newProject').click();
  const project = page.locator('#projectAdminForm');
  await project.locator('[name=id]').fill('browser-proj');
  await project.locator('[name=key]').fill('BRW');
  await project.locator('[name=name]').fill('浏览器项目');
  await project.locator('[type=submit]').click();
  await expect(page.locator('#projectAdminList')).toContainText('浏览器项目');

  await openView(page, 'mcp');
  await page.locator('#newCustomMcp').click();
  const mcp = page.locator('#customMcpForm');
  await mcp.locator('[name=id]').fill('browser-mcp');
  await mcp.locator('[name=name]').fill('浏览器 MCP');
  await mcp.locator('[name=transport]').selectOption('stdio');
  await mcp.locator('[name=command]').fill('true');
  await mcp.locator('[type=submit]').click();
  await page.locator('[data-mcp-tab="installed"]').click();
  await expect(page.locator('#mcpMarketGrid')).toContainText('浏览器 MCP');

  await page.locator('.nav[data-view="settings"]').click();
  await expect(page.locator('#settingsView')).toContainText('密码');
  await page.locator('.nav[data-view="runners"]').click();
  await expect(page.locator('#bindToken')).not.toHaveText('生成中…', { timeout: 15_000 });
  await page.locator('.nav[data-view="guide"]').click();
  await expect(page.locator('#guideView')).toBeVisible();
  await page.locator('.nav[data-view="metrics"]').click();
  await expect(page.locator('#metricsBody')).toBeVisible();
  await page.locator('.nav[data-view="observability"]').click();
  await page.locator('[data-obs-tab="delivery"]').click();
  await expect(page.locator('#obsBody')).toBeVisible();
  expect(errors).toEqual([]);
});
