const { expect } = require('@playwright/test');

const ADMIN = { login: 'admin', password: process.env.E2E_ADMIN_PASSWORD || 'e2e-admin-pass' };

// Collects everything a user would perceive as broken: uncaught exceptions,
// console errors (including CSP-blocked handlers) and failed requests.
function watchErrors(page, { allow = [] } = {}) {
  const errors = [];
  const allowed = text => allow.some(pattern => pattern.test(text));
  page.on('pageerror', e => errors.push(`pageerror: ${e.message}`));
  page.on('console', m => {
    if (m.type() === 'error' && !allowed(m.text())) errors.push(`console: ${m.text()}`);
  });
  page.on('response', r => {
    const line = `HTTP ${r.status()} ${r.request().method()} ${new URL(r.url()).pathname}`;
    if (r.status() >= 400 && !allowed(line)) errors.push(line);
  });
  return errors;
}

// The first-login tour covers the page with a click-catching overlay, so
// every test skips it except the one that exercises it (onboarding: true).
async function login(page, { login: name, password, onboarding = false } = ADMIN) {
  if (!onboarding) await page.addInitScript(() => localStorage.setItem('loopforge.onboarding.v3', 'done'));
  await page.goto('/');
  const form = page.locator('#loginForm');
  await expect(form).toBeVisible();
  await form.locator('[name=loginName]').fill(name);
  await form.locator('[name=password]').fill(password);
  await Promise.all([page.waitForNavigation(), form.locator('[type=submit]').click()]);
  await expect(page.locator('#userName')).toBeVisible();
}

function visibleButton(page, text) {
  return page.locator('button:visible').filter({ hasText: new RegExp(`^\\s*${text}\\s*$`) }).first();
}

async function closeDrawer(page) {
  const drawer = page.locator('#issueDetailDrawer');
  if (await drawer.evaluate(el => el.classList.contains('open')).catch(() => false)) {
    await drawer.locator('.drawer-close').click();
    await expect(drawer).not.toHaveClass(/open/);
  }
}

async function openView(page, view) {
  await page.evaluate(name => { location.hash = `#/${name}`; }, view);
}

// Script SOPs execute in the local sandbox. A project default repository makes
// every dispatch clone that repo first, so these runs use a project with none.
async function useScriptProject(page) {
  const projects = await api(page, '/api/projects');
  if (!projects.some(item => item.id === 'e2e-script')) {
    await api(page, '/api/projects', {
      method: 'POST',
      body: { id: 'e2e-script', key: 'ESOP', name: '脚本沙箱项目' },
    });
    await page.reload();
    await expect(page.locator('#userName')).toBeVisible();
  }
  const switcher = page.locator('#projectSwitcher');
  await expect(switcher.locator('option[value="e2e-script"]')).toHaveCount(1);
  await switcher.selectOption('e2e-script');
}

async function api(page, path, { method = 'GET', body } = {}) {
  return page.evaluate(async ({ path, method, body }) => {
    const res = await fetch(path, {
      method,
      headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await res.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch { data = text; }
    if (!res.ok) {
      const message = data && data.error ? data.error : text;
      throw new Error(`${method} ${path} -> ${res.status} ${message}`);
    }
    return data;
  }, { path, method, body });
}

async function waitForRun(page, runId, accept, timeout = 30_000) {
  let last = null;
  await expect.poll(async () => {
    last = await api(page, `/api/runs/${encodeURIComponent(runId)}`);
    return last.status;
  }, { timeout }).toEqual(expect.stringMatching(accept));
  return last;
}

module.exports = { ADMIN, watchErrors, login, visibleButton, api, waitForRun, closeDrawer, openView, useScriptProject };
