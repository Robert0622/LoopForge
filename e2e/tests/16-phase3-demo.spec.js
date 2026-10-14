const { test, expect } = require('@playwright/test');
const { watchErrors, login, api } = require('./helpers');

test('demo path: command palette, example crew, save SOP, fail-resume, stall', async ({ page }) => {
  test.setTimeout(90_000);
  const errors = watchErrors(page, { allow: [/EventSource/, /net::ERR_/, /messages\/.*\/stream/] });
  await login(page);

  await page.locator('#commandPaletteTrigger').click();
  await expect(page.locator('#commandResults')).toContainText('最近');
  await expect(page.locator('#commandResults')).toContainText('常用动作');
  await page.locator('#commandSearch').fill('群聊');
  await expect(page.locator('.command-kind', { hasText: '群聊' }).first()).toBeVisible();
  await page.locator('#commandSearch').fill('组件');
  await page.locator('#commandResults button', { hasText: '组件' }).first().click();
  await expect(page.locator('#pageTitle')).toHaveText('组件');
  await expect(page.locator('.button.danger-outline')).toBeVisible();
  await expect(page.locator('.lf-status-ok')).toContainText('可用');
  await expect(page.locator('.lf-status-busy')).toContainText('忙');
  await expect(page.locator('.lf-status-off')).toContainText('缺 Key');

  await page.locator('.nav[data-view="chat"]').click();
  await expect(page.locator('.hc-live-strip')).toContainText('进行中');
  await page.locator('.hc-example', { hasText: '多 Bot 做登录页' }).click();
  await expect(page.locator('#pageTitle')).toHaveText('群聊');
  await expect(page.locator('#roomLog')).toContainText('结论', { timeout: 8000 });
  await expect(page.locator('#roomLog')).toContainText('搞定了', { timeout: 15000 });
  await page.locator('#roomSaveSOP').click();
  await expect(page.locator('#roomLog')).toContainText('已沉淀成 SOP', { timeout: 8000 });

  const roomId = await page.evaluate(() => roomState().activeId);
  await api(page, '/api/rooms/' + roomId + '/messages', {
    method: 'POST',
    body: {
      body: '@LoopForge 实现导出按钮',
      fakeModel: { crewStep: true, failStep: 1, firstTokenMs: 40, tokenMs: 4 },
    },
  });
  await page.evaluate(() => refreshRoomMessages(true));
  const card = page.locator('#roomLog .rm-error-card').last();
  await expect(card).toContainText('重试', { timeout: 8000 });
  await card.locator('[data-err-act="retry"]').click();
  await expect(page.locator('#roomLog')).toContainText('再试一次', { timeout: 8000 });

  await api(page, '/api/rooms/' + roomId + '/messages', {
    method: 'POST',
    body: {
      body: '@LoopForge 实现暂停条',
      fakeModel: { crewStep: true, text: '甲乙丙丁', firstTokenMs: 80, tokenMs: 20, stallAfter: 1, stallMs: 12000 },
    },
  });
  await page.evaluate(() => refreshRoomMessages(true));
  await expect(page.locator('#roomLog')).toContainText('甲', { timeout: 1500 });
  const slow = page.locator('#roomLog .rm-stream.is-slow').last();
  await expect(slow).toContainText('仍在处理，已用', { timeout: 12_000 });
  await slow.locator('[data-stream-stop]').click();
  await expect(slow).toBeHidden({ timeout: 4000 });
  expect(errors).toEqual([]);
});
