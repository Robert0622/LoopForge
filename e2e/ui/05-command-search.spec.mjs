import {test,expect} from './fixtures.mjs';
import {post,del,track,cleanup} from '../lib/api.mjs';
import {cfg} from '../lib/env.mjs';

test.afterEach(cleanup);

test('command search follows visible English labels, issue numbers and keyboard focus', async ({page})=>{
  await page.addInitScript(()=>localStorage.setItem('loopforge.lang','en'));
  const title=`Palette ${cfg.runTag}`;
  const issue=await post('/api/issues',{projectId:cfg.projectId,title,description:'Command search acceptance',sopMode:'none'});
  track(`palette issue ${issue.id}`,()=>del(`/api/issues/${encodeURIComponent(issue.id)}`));
  await page.goto('/#/issues');
  const opener=page.locator('#commandPaletteTrigger');
  const search=page.locator('#commandSearch');
  const close=page.locator('#commandClose');
  const label=await page.locator('#pageTitle').innerText();
  const row=page.locator(`.issue-table-row[data-id="${issue.id}"]`);
  await expect(row).toBeVisible();
  const key=await row.locator('.issue-key-cell').getAttribute('title');
  await opener.click();
  await search.fill(label);
  await expect(page.getByRole('option').filter({hasText:label}).first()).toBeVisible();
  await search.fill(key);
  await expect(page.getByRole('option').first()).toContainText(key);
  await search.fill('no matching search command');
  await expect(page.locator('#commandEmpty')).toBeVisible();
  await page.keyboard.press('Tab');
  await expect(close).toBeFocused();
  await page.keyboard.press('Shift+Tab');
  await expect(search).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(opener).toBeFocused();
  await page.keyboard.press('Control+k');
  await search.press('ArrowDown');
  await expect(search).toHaveAttribute('aria-activedescendant','command-result-1');
  await search.press('Enter');
  await expect(page).toHaveURL(/#\/collaboration$/);
});
