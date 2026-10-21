// SPDX-License-Identifier: Apache-2.0
// Remote CI: actual Console wizard, synthetic Gateway and provider consent.
import { chromium } from 'playwright';
import { expect as baseExpect } from '@playwright/test';
import { mkdirSync, writeFileSync } from 'node:fs';
import { assertWizardAccessibility as accessible } from './lib/wizard-checks.mjs';

const expect = baseExpect.configure({ timeout: 15000 });

const BASE = process.env.CONSOLE_URL || 'http://127.0.0.1:4343/admin/prueba/dev/conectar-app';
const OUT = process.env.CONSOLE_OUT || '/tmp/conectar-app';
const CONSENT = 'https://slack.com/sytue-test-consent';
mkdirSync(OUT, { recursive: true });
const browser = await chromium.launch();
const results = [];

async function mount(options = {}) {
  const context = await browser.newContext({
    viewport: { width: options.width || 1280, height: options.height || 900 },
    reducedMotion: 'reduce', colorScheme: 'light',
  });
  const page = await context.newPage();
  page.setDefaultTimeout(30000);
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const state = { requests: [], visits: [], authorization: options.authorization === undefined ? CONSENT : options.authorization };
  const json = (route, body, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
  await context.route('**/api/admin/apps/instances**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const body = ['POST', 'PUT'].includes(request.method()) ? request.postDataJSON() : null;
    state.requests.push({ path, method: request.method(), body });
    if (request.method() === 'POST' && path.endsWith('/instances')) return json(route, { id: 'fixture-account' }, 201);
    if (path.endsWith('/oauth-start')) return json(route, state.authorization === null ? {} : { authorize_url: state.authorization });
    if (request.method() === 'DELETE') return json(route, { ok: true });
    // Record forbidden test/secrets/activate calls even if the UI tries to continue.
    return json(route, { error: 'unexpected_endpoint' }, 500);
  });
  await context.route('**/sytue-test-consent*', async (route) => {
    state.visits.push(route.request().url());
    return route.fulfill({ contentType: 'text/html', body: '<!doctype html><html lang="en"><title>Provider consent</title><main><h1>Provider consent</h1></main></html>' });
  });
  const url = new URL(BASE);
  if (options.fields) url.searchParams.set('fields', '1');
  await page.goto(url.href, { timeout: 180000 });
  await expect(page.getByRole('heading', { name: 'This is what you will switch on in Slack', exact: true })).toBeVisible({ timeout: 180000 });
  return { context, page, state, errors };
}

async function review(ui, fields = false) {
  await expect(ui.page.getByText('Legacy static token', { exact: true })).toHaveCount(0);
  await expect(ui.page.getByText('Legacy credential instructions', { exact: true })).toHaveCount(0);
  await expect(ui.page.locator('input[type="password"]')).toHaveCount(0);
  if (!fields) await expect(ui.page.getByText('Have this to hand before you carry on', { exact: true })).toHaveCount(0);
  await ui.page.getByRole('button', { name: 'Continue', exact: true }).click();
  if (fields) {
    await expect(ui.page.getByRole('heading', { name: 'Your Slack values', exact: true })).toBeFocused();
    await ui.page.getByRole('button', { name: 'Next', exact: true }).click();
    await expect(ui.page.getByRole('heading', { name: 'Your Slack values', exact: true })).toBeVisible();
    await ui.page.getByLabel('Workspace', { exact: false }).fill('acme');
    await expect(ui.page.locator('input[type="password"]')).toHaveCount(0);
    await ui.page.getByRole('button', { name: 'Next', exact: true }).click();
  }
  await expect(ui.page.getByRole('heading', { name: 'Review and activate', exact: true })).toBeFocused();
  await expect(ui.page.getByRole('button', { name: 'Connect with your Slack account', exact: true })).toBeVisible();
  await expect(ui.page.getByRole('button', { name: 'Test connection', exact: true })).toHaveCount(0);
  await expect(ui.page.getByText('Not tested', { exact: true })).toHaveCount(0);
  await expect(ui.page.getByText('Legacy static token', { exact: true })).toHaveCount(0);
  await expect(ui.page.getByRole('button', { name: 'Edit Check', exact: true })).toHaveCount(0);
  if (!fields) await expect(ui.page.getByRole('button', { name: 'Edit Name of the connection', exact: true })).toHaveCount(0);
}

function assertConsentOnly(ui, config = {}) {
  expect(ui.state.requests.filter((request) => /\/(test|secrets|activate)$/.test(request.path))).toEqual([]);
  expect(ui.state.requests.every((request) => request.path.startsWith('/admin/api/admin/apps/'))).toBe(true);
  expect(ui.state.requests.find((request) => request.path.endsWith('/instances'))?.body).toEqual({ template_key: 'slack', name: 'Slack', config });
  expect(ui.state.requests.find((request) => request.path.endsWith('/oauth-start'))?.body).toEqual({ return_to: 'apps' });
}

async function scenario(name, run, options = {}) {
  let ui;
  try {
    ui = await mount(options);
    await run(ui);
    expect(ui.errors).toEqual([]);
    results.push({ name, ok: true });
    console.log(`✓ ${name}`);
  } catch (error) {
    results.push({ name, ok: false, error: error.stack || String(error) });
    console.error(`✗ ${name}: ${error.message}`);
    if (ui) await ui.page.screenshot({ path: `${OUT}/failure-${name}.png`, animations: 'disabled' }).catch(() => {});
  } finally { if (ui) await ui.context.close(); }
}

try {
  await scenario('oauth-without-credentials-or-extra-steps', async (ui) => {
    await review(ui);
    await accessible(ui, 'main');
    await ui.page.screenshot({ path: `${OUT}/oauth-review.png`, animations: 'disabled' });
    await ui.page.getByRole('button', { name: 'Connect with your Slack account', exact: true }).click();
    await expect(ui.page).toHaveURL(CONSENT);
    expect(ui.state.visits).toEqual([CONSENT]);
    assertConsentOnly(ui);
  });
  await scenario('oauth-keeps-required-fields-and-skips-test', async (ui) => {
    await review(ui, true);
    await ui.page.getByRole('button', { name: 'Connect with your Slack account', exact: true }).click();
    await expect(ui.page).toHaveURL(CONSENT);
    assertConsentOnly(ui, { workspace: 'acme' });
  }, { fields: true });
  await scenario('missing-authorization-url-stays-review-and-retries', async (ui) => {
    await review(ui);
    await ui.page.getByRole('button', { name: 'Connect with your Slack account', exact: true }).click();
    await expect(ui.page.getByText('Could not activate it', { exact: true })).toBeVisible();
    await expect(ui.page.getByRole('heading', { name: 'Review and activate', exact: true })).toBeVisible();
    expect(ui.state.visits).toEqual([]);
    assertConsentOnly(ui);
    await ui.page.screenshot({ path: `${OUT}/oauth-url-missing.png`, animations: 'disabled' });
    ui.state.authorization = CONSENT;
    await ui.page.getByRole('button', { name: 'Connect with your Slack account', exact: true }).click();
    await expect(ui.page).toHaveURL(CONSENT);
    expect(ui.state.requests.filter((request) => request.path.endsWith('/instances'))).toHaveLength(1);
    assertConsentOnly(ui);
  }, { authorization: null });
  for (const [kind, authorization] of [
    ['http', 'http://slack.com/sytue-test-consent'],
    ['script', 'javascript:void(0)'],
    ['userinfo', 'https://user:password@slack.com/sytue-test-consent'],
  ]) {
    await scenario(`invalid-authorization-${kind}-never-activates`, async (ui) => {
      await review(ui);
      await ui.page.getByRole('button', { name: 'Connect with your Slack account', exact: true }).click();
      await expect(ui.page.getByText('Could not activate it', { exact: true })).toBeVisible();
      await expect(ui.page.getByRole('heading', { name: 'Review and activate', exact: true })).toBeVisible();
      expect(ui.state.visits).toEqual([]);
      assertConsentOnly(ui);
    }, { authorization });
  }
  await scenario('oauth-mobile-390', async (ui) => {
    await review(ui);
    await accessible(ui, 'main');
    expect(await ui.page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true);
    await ui.page.getByRole('button', { name: 'Connect with your Slack account', exact: true }).scrollIntoViewIfNeeded();
    await ui.page.screenshot({ path: `${OUT}/oauth-mobile-390.png`, animations: 'disabled', fullPage: true });
  }, { width: 390, height: 700 });
} finally {
  await browser.close();
  writeFileSync(`${OUT}/report.json`, `${JSON.stringify(results, null, 2)}\n`);
}
if (results.some((result) => !result.ok)) process.exitCode = 1;
