const { test, expect } = require('@playwright/test');
const { watchErrors, login } = require('./helpers');

test('phase 1 sidebar, command palette, destructive confirm and stream states', async ({ page }) => {
  const errors = watchErrors(page);
  await login(page);

  const side = page.locator('.platform-sidebar');
  await expect(side.locator('.nav-section', { hasText: '工作' })).toBeVisible();
  await expect(side).toContainText('配置');
  await expect(side).toContainText('洞察');
  await expect(side.locator('.nav[data-view="issues"] .nav-label')).toHaveText('SOP 运行');
  await expect(side.locator('.nav[data-view="rooms"] .nav-label')).toHaveText('群聊');
  await expect(side.locator('.nav[data-view="agents"] .nav-label')).toHaveText('Bot');
  await expect(side.locator('.nav[data-view="sandboxes"] .nav-label')).toHaveText('环境');
  await expect(side.locator('.nav[data-view="metrics"] .nav-label')).toHaveText('运行洞察');
  await expect(side.locator('.nav[data-view="observability"] .nav-label')).toHaveText('成本');
  await expect(side.locator('#commandPaletteTrigger')).toContainText('搜索或跳转');
  await expect(page.locator('#roomSaveSOP')).toHaveText('存为 SOP');

  await page.locator('#commandPaletteTrigger').click();
  await expect(page.locator('#commandPalette')).toBeVisible();
  await expect(page.locator('.command-footer')).toContainText('筛选类型');
  await page.keyboard.press('Escape');
  await expect(page.locator('#commandPalette')).toBeHidden();

  const confirmed = page.evaluate(() => afConfirm({
    title: '删除知识库「竞品资料库」？',
    message: '此操作不可撤销。',
    confirmText: '删除知识库',
    danger: true,
    confirmName: '竞品资料库',
  }));
  const ok = page.locator('#confirmOkBtn');
  await expect(ok).toBeDisabled();
  await expect(ok).toHaveClass(/danger-outline/);
  await expect(ok).not.toHaveCSS('background-color', 'rgb(225, 29, 72)');
  await page.locator('#confirmNameInput').fill('别的名字');
  await expect(ok).toBeDisabled();
  await page.locator('#confirmNameInput').fill('竞品资料库');
  await expect(ok).toBeEnabled();
  await ok.click();
  expect(await confirmed).toBe(true);

  await page.locator('.nav[data-view="rooms"]').click();
  await page.waitForLoadState('networkidle');
  await page.evaluate(() => lfPreviewRoomStream({ phase: 'queue', name: '研究员' }));
  await expect(page.locator('.rm-stream').last()).toContainText('排队中');
  await page.evaluate(() => lfPreviewRoomStream({ phase: 'think', name: '研究员' }));
  await expect(page.locator('.rm-stream').last()).toContainText('思考中');
  await page.evaluate(() => lfPreviewRoomStream({ phase: 'slow', name: '研究员' }));
  const slow = page.locator('.rm-stream.is-slow');
  await expect(slow).toContainText('仍在处理');
  await expect(slow.locator('[data-stream-stop]')).toBeVisible();
  await expect(slow.locator('[data-stream-retry]')).toBeVisible();
  await expect(page.locator('.workspace-spinner, .page-spinner')).toHaveCount(0);
  await page.evaluate(() => lfPreviewRoomStream({ phase: 'error', error: '缺少 API Key', name: '研究员' }));
  const card = page.locator('.rm-error-card');
  await expect(card).toContainText('配置 Key');
  await expect(card).toContainText('换个 Bot');
  await expect(card).toContainText('重试');

  const badge = await page.evaluate(() => {
    const user = typeof userById === 'function' ? userById(state.currentUser?.id) : null;
    if (user) {
      user.codeBuddyApiKeyConfigured = false;
      user.codexApiKeyConfigured = false;
      user.claudeCodeApiKeyConfigured = false;
    }
    const host = document.createElement('div');
    host.innerHTML = lfStatusBadge({ id: 'missing-key', name: '研究员' });
    document.body.appendChild(host);
    const badgeEl = host.querySelector('.lf-status');
    const dot = host.querySelector('i');
    const bg = getComputedStyle(dot).backgroundColor;
    return { text: host.textContent, bg, off: badgeEl.className };
  });
  expect(badge.text).toContain('缺 Key');
  expect(badge.off).toContain('lf-status-off');
  expect(badge.bg).not.toBe('rgb(229, 72, 77)');
  expect(badge.bg).not.toBe('rgb(242, 85, 90)');

  const ia = await page.evaluate(() => {
    const subs = (VIEW_TABSETS.find(set => set.nav === 'issues')?.tabs?.[0]?.subs || []).map(item => item[1]);
    const mock = lfBotStatus({id: 'lf-demo-exec-probe', name: '演示执行器', executorId: 'mock'});
    return {
      zones: [...document.querySelectorAll('.room-zone-k, #roomSaveSOP')].map(el => el.textContent.trim()),
      subs,
      approvals: document.querySelector('#approvalsView h1')?.textContent.trim(),
      mock: `${mock.label} ${mock.key}`,
    };
  });
  expect(ia.zones).toEqual(['成员状态', '费用·进度', '存为 SOP']);
  expect(ia.subs).toEqual(['进行中', '待确认', '已完成', '需求']);
  expect(ia.approvals).toBe('待确认');
  expect(ia.mock).toBe('可用 ok');

  const dark = await page.evaluate(() => {
    document.documentElement.dataset.theme = 'dark';
    return getComputedStyle(document.documentElement).getPropertyValue('--lf-bg').trim();
  });
  expect(dark.toLowerCase()).toBe('#08090c');

  expect(errors).toEqual([]);
});
