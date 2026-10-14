const { test, expect } = require('@playwright/test');
const { watchErrors, login, api } = require('./helpers');

test('room SSE shows a first token quickly and a stall strip with stop', async ({ page }) => {
  test.setTimeout(40_000);
  const errors = watchErrors(page, { allow: [/EventSource/, /net::ERR_/, /messages\/.*\/stream/] });
  await login(page);
  await page.locator('.nav[data-view="rooms"]').click();
  await expect(page.locator('#pageTitle')).toHaveText('群聊');
  await page.locator('#newRoom').click();
  await page.locator('#confirmPromptInput').fill('流式验收群');
  await page.locator('#confirmOkBtn').click();
  await expect(page.locator('#roomTitle')).toContainText('流式验收群');

  const roomId = await page.evaluate(() => roomState().activeId);
  const fast = await api(page, '/api/rooms/' + roomId + '/messages', {
    method: 'POST',
    body: {
      body: '@LoopForge 你好',
      fakeModel: { text: '甲乙丙丁', firstTokenMs: 200, tokenMs: 30 },
    },
  });
  expect(fast.id).toBeTruthy();
  const started = Date.now();
  await page.evaluate(() => refreshRoomMessages(true));
  await expect(page.locator('#roomLog')).toContainText('甲', { timeout: 1500 });
  expect(Date.now() - started).toBeLessThan(1500);
  await expect(page.locator('#roomLog')).toContainText('甲乙丙丁');

  await api(page, '/api/rooms/' + roomId + '/messages', {
    method: 'POST',
    body: {
      body: '@LoopForge 什么进度',
      fakeModel: { text: '戊己庚', firstTokenMs: 80, tokenMs: 20, stallAfter: 1, stallMs: 12000 },
    },
  });
  await page.evaluate(() => refreshRoomMessages(true));
  await expect(page.locator('#roomLog')).toContainText('戊');
  const slow = page.locator('#roomLog .rm-stream.is-slow').last();
  await expect(slow).toContainText('仍在处理，已用', { timeout: 12_000 });
  await expect(slow.locator('[data-stream-stop]')).toBeVisible();
  await expect(slow.locator('[data-stream-retry]')).toBeVisible();
  await slow.locator('[data-stream-stop]').click();
  await expect(slow).toBeHidden({ timeout: 3000 });
  expect(errors).toEqual([]);
});
