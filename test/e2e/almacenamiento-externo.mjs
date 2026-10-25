// SPDX-License-Identifier: Apache-2.0
// Real Drive «Add storage» picker in astro dev. Network replies are synthetic; no provider account is used.
import { createReportDirectory, writeReport } from './lib/report-output.mjs';
import { chromium } from 'playwright';
import { expect as baseExpect } from '@playwright/test';
import { assertWizardLayout as layout, assertWizardAccessibility as accessible, describeWizardState, openWizardFixture } from './lib/wizard-checks.mjs';
import { mkdirSync } from 'node:fs';

const expect = baseExpect.configure({ timeout: 15000 });

const BASE = process.env.STORAGE_URL || 'http://127.0.0.1:4343/drive/prueba/dev/almacenamiento';
const OUT = createReportDirectory('almacenamiento-externo', process.env.STORAGE_OUT);
const browser = await chromium.launch();
const results = [];
const PROVIDERS = {
  gdrive: { template: 'google-drive', cloud: 'google-drive', host: 'accounts.google.com', name: 'Google Drive' },
  onedrive: { template: 'onedrive', cloud: 'onedrive', host: 'login.microsoftonline.com', name: 'OneDrive' },
  dropbox: { template: 'dropbox-storage', cloud: 'dropbox', host: 'www.dropbox.com', name: 'Dropbox' },
};

async function mount(options = {}) {
  const context = await browser.newContext({
    viewport: { width: options.width || 1280, height: options.height || 900 },
    hasTouch: Boolean(options.mobile),
    isMobile: Boolean(options.mobile),
    reducedMotion: 'reduce',
    colorScheme: options.scheme || 'light',
  });
  const page = await context.newPage();
  page.setDefaultTimeout(30000);
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const state = {
    // null: quien mira no administra y Drive contesta 403.
    storage: options.storage === undefined ? { connected: false, available: true } : options.storage,
    accounts: options.accounts || [],
    personalAccounts: options.personalAccounts || [],
    saveFailure: Boolean(options.saveFailure),
    deleteConflict: Boolean(options.deleteConflict),
    nasReady: false,
    requests: [],
    personalConnects: [],
    providers: options.providers ?? { gdrive: true, onedrive: true, dropbox: true },
    personal: options.personal ?? ['google-drive', 'onedrive', 'dropbox'],
    quotaReads: 0,
  };
  const json = (route, value, status = 200) => route.fulfill({
    status, contentType: 'application/json', body: JSON.stringify(value),
  });
  await context.route('**/api/quota', (route) => {
    state.quotaReads++;
    const connected = state.storage?.connected === true || state.personalAccounts.some((account) => account.connected);
    const known = connected && options.knownCapacity === true;
    return json(route, {
      usedBytes: known ? 220 : 20, limitBytes: known ? 1124 : 100,
      availableBytes: known ? 904 : 80,
      baseUsedBytes: 20, baseLimitBytes: 100, externalStorage: connected,
      externalCapacity: connected ? {
        totalBytes: known ? 1024 : null,
        usedBytes: known ? 200 : null,
        availableBytes: known ? 824 : null,
        ...(options.partialCapacity ? { partial: true } : {}),
      } : null,
    });
  });
  await context.route('**/api/personal-storage/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === 'DELETE') {
      state.personalAccounts = state.personalAccounts.filter((account) => !path.endsWith(`/${account.instance_id}`));
      return route.fulfill({ status: 204 });
    }
    if (path.endsWith('/items')) return json(route, { items: [] });
    if (path.endsWith('/accounts') && options.failAccountsAfterDisconnect && state.personalAccounts.length === 0) {
      return json(route, { error: 'inventory_unavailable' }, 503);
    }
    if (request.url().endsWith('/connect')) {
      const body = request.postDataJSON();
      state.personalConnects.push(body);
      return json(route, { authorize_url: `https://${PROVIDERS.gdrive.host}/sytue-test-personal?provider=${body.provider}` });
    }
    return json(route, { items: state.personalAccounts, providers: Object.values(PROVIDERS).map((p) => ({ provider: p.cloud, available: state.personal.includes(p.cloud) })) });
  });
  await context.route('**/drive/api/storage-external*', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const method = request.method();
    const body = method === 'POST' || method === 'PUT' ? request.postDataJSON() : undefined;
    state.requests.push({ path, method, body });
    if (state.storage === null) return json(route, { error: 'forbidden' }, 403);
    if (path.endsWith('-instances')) return json(route, {
      items: state.accounts.map((account) => ({ oauth_available: true, ...account })),
      providers: state.providers,
    });
    if (path.endsWith('-consent')) {
      const provider = PROVIDERS[body.kind];
      let id = `account-${body.kind}`;
      for (let suffix = 2; state.accounts.some((item) => item.id === id); suffix++) id = `account-${body.kind}-${suffix}`;
      state.accounts.push({ id, template_key: provider.template, name: `${provider.name} account`, connected: false });
      return json(route, { instance_id: id, authorize_url: `https://${provider.host}/sytue-test-consent?kind=${body.kind}&instance=${id}` });
    }
    if (path.endsWith('-nas')) {
      if (method === 'POST') state.storage = { connected: true, available: true, kind: 'nas', endpoint: 'https://nas-prueba.example.eu', label: 'Home NAS' };
      return json(route, {
        endpoint: 'https://nas-prueba.example.eu', command: 'docker run --rm example.invalid/nas-agent:demo',
        online: state.nasReady, reachable: state.nasReady,
      });
    }
    if (method === 'GET') return json(route, state.storage);
    if (method === 'PUT') {
      if (state.saveFailure) return json(route, { error: { code: 'unprocessable', message: 'Could not reach the bucket: Connection refused' } }, 422);
      state.storage = { connected: true, available: true, ...body };
      return json(route, state.storage);
    }
    if (method === 'DELETE') {
      if (state.deleteConflict) return json(route, { error: 'files_remain' }, 409);
      state.storage = { connected: false, available: true };
      return route.fulfill({ status: 204 });
    }
    return json(route, { error: 'unexpected_request' }, 400);
  });
  await context.route(`https://${PROVIDERS.gdrive.host}/sytue-test-personal*`, (route) =>
    route.fulfill({ status: 200, contentType: 'text/html', body: '<main>consent</main>' }));
  for (const provider of Object.values(PROVIDERS)) {
    await context.route(`https://${provider.host}/sytue-test-consent*`, async (route) => {
      const instance = new URL(route.request().url()).searchParams.get('instance');
      state.accounts = state.accounts.map((account) => account.id === instance ? { ...account, connected: true } : account);
      const callback = new URL(BASE);
      callback.searchParams.set('lang', 'en');
      callback.searchParams.set('connect-storage', '1');
      callback.searchParams.set('instance', instance);
      await route.fulfill({ status: 302, headers: { location: callback.href } });
    });
  }

  const url = new URL(BASE);
  url.searchParams.set('lang', options.lang || 'en');
  if (options.autoOpen !== false) url.searchParams.set('connect-storage', '1');
  await openWizardFixture(page, url.href);
  const dialog = page.getByRole('dialog');
  if (options.autoOpen !== false) await expect(dialog).toBeVisible({ timeout: 180000 });
  await page.evaluate((scheme) => { document.documentElement.dataset.theme = scheme; }, options.scheme || 'light');
  return { page, context, dialog, state, errors };
}

const option = (ui, kind) => ui.dialog.locator(`button[value="${kind}"]`);
async function fillServer(ui, kind = 'webdav') {
  await ui.dialog.locator('#external-endpoint').fill(kind === 's3' ? 's3.fr-par.scw.cloud' : kind === 'nas' ? 'http://192.168.1.50:5005' : 'https://cloud.example.eu/webdav');
  if (kind === 's3') await ui.dialog.locator('#external-bucket').fill('documents');
  await ui.dialog.locator('#external-access_key').fill('demo-user');
  await ui.dialog.locator('#external-secret_key').fill('demo-secret');
}
async function screenshot(ui, name) {
  await expect(ui.dialog).toHaveCSS('opacity', '1');
  await ui.page.screenshot({ path: `${OUT}/${name}.png`, animations: 'disabled' });
}
async function scenario(name, run, options = {}) {
  if (process.env.STORAGE_SCENARIO && process.env.STORAGE_SCENARIO !== name) return;
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
    if (ui) console.error(JSON.stringify({ name, diagnostic: await describeWizardState(ui).catch(String) }));
    if (ui) await ui.page.screenshot({ path: `${OUT}/failure-${name}.png`, animations: 'disabled' }).catch(() => {});
  } finally {
    if (ui) await ui.context.close();
  }
}

try {
  for (const capacity of ['known', 'partial', 'unknown']) {
    const knownCapacity = capacity !== 'unknown';
    await scenario(`capacity-refresh-${capacity}`, async (ui) => {
      await expect(ui.page.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '20');
      const initialReads = ui.state.quotaReads;
      await ui.page.getByRole('button', { name: 'Add storage', exact: true }).click();
      await expect(ui.dialog).toBeVisible();
      await option(ui, 's3').click();
      await fillServer(ui, 's3');
      await ui.dialog.locator('input[name="accept_terms"]').check();
      await ui.page.getByRole('button', { name: 'Connect', exact: true }).click();
      await expect(ui.dialog).toHaveCount(0);
      await expect(ui.page.getByText('Connected storage extends your Sytue capacity.', { exact: true })).toBeVisible();
      expect(ui.state.quotaReads).toBeGreaterThan(initialReads);
      if (knownCapacity) {
        await expect(ui.page.getByText('Connected storage adds 1.0 KB.', { exact: true })).toBeVisible();
        await expect(ui.page.getByText('220 B of 1.1 KB', { exact: true })).toBeVisible();
        await expect(ui.page.getByText('Sytue: 20 B of 100 B', { exact: true })).toBeVisible();
        await expect(ui.page.getByText('The provider does not report its capacity.', { exact: true })).toHaveCount(capacity === 'partial' ? 1 : 0);
        if (capacity === 'known') await ui.page.screenshot({ path: `${OUT}/capacity-known.png`, animations: 'disabled' });
      } else {
        await expect(ui.page.getByText('The provider does not report its capacity.', { exact: true })).toBeVisible();
        await expect(ui.page.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '20');
        await expect(ui.page.getByText(/Connected storage adds/)).toHaveCount(0);
      }
      await ui.page.getByRole('button', { name: 'documents', exact: true }).click();
      await ui.page.getByRole('button', { name: 'Disconnect', exact: true }).click();
      await expect(ui.dialog).toHaveCount(0);
      await expect(ui.page.getByText('Connected storage extends your Sytue capacity.', { exact: true })).toHaveCount(0);
      await expect(ui.page.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '20');
      expect(ui.state.quotaReads).toBeGreaterThan(initialReads + 1);
    }, { knownCapacity, partialCapacity: capacity === 'partial', autoOpen: false });
  }
  for (const failAccountsAfterDisconnect of [false, true]) {
    await scenario(`personal-disconnect-refreshes-capacity${failAccountsAfterDisconnect ? '-inventory-fails' : ''}`, async (ui) => {
      await expect(ui.page.getByText('Connected storage adds 1.0 KB.', { exact: true })).toBeVisible();
      const initialReads = ui.state.quotaReads;
      await ui.page.getByRole('button', { name: 'Google Drive', exact: true }).click();
      await expect(ui.page.getByRole('button', { name: 'Disconnect', exact: true })).toBeVisible();
      await expect.poll(() => ui.state.quotaReads).toBeGreaterThan(initialReads);
      const beforeDisconnect = ui.state.quotaReads;
      await ui.page.getByRole('button', { name: 'Disconnect', exact: true }).click();
      await expect(ui.page.getByText('Connected storage extends your Sytue capacity.', { exact: true })).toHaveCount(0);
      await expect(ui.page.getByText('20 B of 100 B', { exact: true })).toBeVisible();
      await expect(ui.page.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '20');
      await expect.poll(() => ui.state.quotaReads).toBeGreaterThan(beforeDisconnect);
      expect(ui.state.personalAccounts).toEqual([]);
    }, {
      storage: null, autoOpen: false, knownCapacity: true,
      personalAccounts: [{ instance_id: 'personal-google', provider: 'google-drive', connected: true }],
      failAccountsAfterDisconnect,
    });
  }
  await scenario('personal-cloud-is-one-click', async (ui) => {
    await expect(ui.dialog.locator('input[name="for_org"]')).toHaveCount(0);
    await expect(option(ui, 'webdav')).toHaveCount(0);
    await screenshot(ui, 'personal-picker-desktop');
    await accessible(ui);
    await option(ui, 'onedrive').click();
    await ui.page.waitForURL((url) => url.pathname === '/sytue-test-personal');
    expect(ui.state.personalConnects).toEqual([{ provider: 'onedrive', return_to: 'drive' }]);
    expect(ui.state.requests.filter((r) => r.path.endsWith('-consent'))).toHaveLength(0);
  }, { storage: null });

  for (const [kind, provider] of Object.entries(PROVIDERS)) {
    await scenario(`org-${kind}-consent-returns-to-one-screen`, async (ui) => {
      await ui.dialog.locator('input[name="for_org"]').check();
      await option(ui, kind).click();
      await ui.page.waitForURL((url) => url.searchParams.get('instance') === `account-${kind}`, { timeout: 30000 });
      await expect(ui.dialog).toBeVisible({ timeout: 30000 });
      await expect(ui.dialog.getByText(`${provider.name} account`, { exact: true })).toBeVisible({ timeout: 30000 });
      const consent = ui.state.requests.filter((r) => r.path.endsWith('-consent'));
      expect(consent.map((r) => r.body)).toEqual([{ kind, new_account: true }]);
      const save = ui.page.getByRole('button', { name: 'Connect', exact: true });
      await expect(save).toBeDisabled();
      await ui.dialog.locator('input[name="accept_terms"]').check();
      await screenshot(ui, `${kind}-confirm-desktop`);
      await save.click();
      await expect(ui.dialog).toHaveCount(0);
      const saved = ui.state.requests.filter((r) => r.method === 'PUT');
      expect(saved).toHaveLength(1);
      expect(saved[0].body).toEqual({ kind, instance_id: `account-${kind}`, accept_terms: true });
      expect(ui.state.requests.filter((r) => r.method === 'POST' && r.path.endsWith('/storage-external'))).toHaveLength(0);
    });
  }

  await scenario('org-authorized-account-skips-consent', async (ui) => {
    await ui.dialog.locator('input[name="for_org"]').check();
    await option(ui, 'gdrive').click();
    await expect(ui.dialog.getByText('Sales Google', { exact: true })).toBeVisible();
    expect(ui.state.requests.filter((r) => r.path.endsWith('-consent'))).toHaveLength(0);
  }, { accounts: [{ id: 'ready', template_key: 'google-drive', name: 'Sales Google', connected: true }] });

  await scenario('org-consent-without-encryption-key', async (ui) => {
    await expect(option(ui, 'webdav')).toHaveCount(0);
    await ui.dialog.locator('input[name="for_org"]').check();
    await option(ui, 'dropbox').click();
    await ui.page.waitForURL((url) => url.searchParams.get('instance') === 'account-dropbox');
    await expect(ui.dialog.locator('input[name="accept_terms"]')).toBeVisible();
  }, { storage: { connected: false, available: false } });

  await scenario('unconfigured-cloud-stays-disabled-with-servers-available', async (ui) => {
    for (const kind of Object.keys(PROVIDERS)) await expect(option(ui, kind)).toBeDisabled();
    await ui.dialog.locator('input[name="for_org"]').check();
    for (const kind of Object.keys(PROVIDERS)) await expect(option(ui, kind)).toBeDisabled();
    await expect(ui.dialog.getByText('Connecting to Google Drive has not been configured yet.', { exact: true })).toBeVisible();
    await layout(ui);
    await accessible(ui);
    await screenshot(ui, 'unconfigured-cloud-mobile');
    await option(ui, 'webdav').click();
    await expect(ui.dialog.locator('#external-endpoint')).toBeVisible();
  }, { providers: { gdrive: false, onedrive: false, dropbox: false }, personal: [], width: 320, height: 640, mobile: true });

  await scenario('webdav-one-form-save-error-inline', async (ui) => {
    await option(ui, 'webdav').click();
    const save = ui.page.getByRole('button', { name: 'Connect', exact: true });
    await fillServer(ui);
    await expect(save).toBeDisabled();
    await ui.dialog.locator('input[name="accept_terms"]').check();
    await screenshot(ui, 'webdav-form-desktop');
    await accessible(ui);
    await save.click();
    await expect(ui.dialog.getByRole('alert')).toContainText('Connection refused');
    ui.state.saveFailure = false;
    await save.click();
    await expect(ui.dialog).toHaveCount(0);
    const saved = ui.state.requests.filter((r) => r.method === 'PUT');
    expect(saved).toHaveLength(2);
    expect(saved[1].body).toMatchObject({ kind: 'webdav', endpoint: 'https://cloud.example.eu/webdav', accept_terms: true });
    expect(ui.state.requests.filter((r) => r.method === 'POST')).toHaveLength(0);
  }, { saveFailure: true });

  await scenario('required-fields-block-save', async (ui) => {
    await option(ui, 's3').click();
    await ui.dialog.locator('input[name="accept_terms"]').check();
    await ui.page.getByRole('button', { name: 'Connect', exact: true }).click();
    expect(ui.state.requests.filter((r) => r.method === 'PUT')).toHaveLength(0);
    await fillServer(ui, 's3');
    await ui.page.getByRole('button', { name: 'Connect', exact: true }).click();
    await expect(ui.dialog).toHaveCount(0);
    expect(ui.state.requests.find((r) => r.method === 'PUT').body).toMatchObject({ kind: 's3', bucket: 'documents' });
  });

  await scenario('nas-command-survives-close-and-reopen', async (ui) => {
    await option(ui, 'nas').click();
    await fillServer(ui, 'nas');
    await ui.dialog.locator('input[name="accept_terms"]').check();
    await ui.page.getByRole('button', { name: 'Create the tunnel', exact: true }).click();
    await expect(ui.dialog.locator('pre')).toContainText('docker run');
    await screenshot(ui, 'nas-command-desktop');
    await ui.page.getByRole('button', { name: 'Close', exact: true }).last().click();
    await expect(ui.dialog).toHaveCount(0);
    await ui.page.getByRole('button', { name: 'Home NAS', exact: true }).click();
    await expect(ui.dialog.locator('pre')).toContainText('docker run');
    expect(ui.state.requests.filter((r) => r.path.endsWith('-nas') && r.method === 'POST')).toHaveLength(1);
    ui.state.nasReady = true;
    await expect(ui.dialog).toHaveCount(0, { timeout: 15000 });
  });

  await scenario('disconnect-conflict-preserves-storage', async (ui) => {
    await ui.page.getByRole('button', { name: 'documents', exact: true }).click();
    await ui.page.getByRole('button', { name: 'Disconnect', exact: true }).click();
    await expect(ui.dialog.getByRole('alert')).toContainText('There are still files');
    expect(ui.state.storage.connected).toBe(true);
    ui.state.deleteConflict = false;
    await ui.page.getByRole('button', { name: 'Disconnect', exact: true }).click();
    await expect(ui.dialog).toHaveCount(0);
    await expect(ui.page.getByRole('button', { name: 'documents', exact: true })).toHaveCount(0);
    await expect(ui.page.getByRole('button', { name: 'Add storage', exact: true })).toBeVisible();
  }, { autoOpen: false, deleteConflict: true, storage: { connected: true, available: true, kind: 's3', bucket: 'documents' } });

  await scenario('keyboard-and-focus-return', async (ui) => {
    const trigger = ui.page.getByRole('button', { name: 'Add storage', exact: true });
    await trigger.click();
    await expect(ui.dialog).toBeVisible();
    await option(ui, 'webdav').focus();
    await ui.page.keyboard.press('Enter');
    await expect(ui.dialog.locator('#external-endpoint')).toBeVisible();
    await ui.page.keyboard.press('Escape');
    await expect(ui.dialog).toHaveCount(0);
    await expect(trigger).toBeFocused();
  }, { autoOpen: false });

  for (const lang of ['es', 'de']) {
    for (const width of [390, 320]) {
      await scenario(`mobile-${lang}-${width}`, async (ui) => {
        await screenshot(ui, `picker-${lang}-${width}`);
        await layout(ui);
        await accessible(ui);
        await option(ui, 'webdav').click();
        await fillServer(ui);
        await expect(ui.dialog.locator('input[name="accept_terms"]')).toBeVisible();
        await screenshot(ui, `form-${lang}-${width}`);
        await layout(ui);
        await accessible(ui);
      }, { lang, width, height: width === 320 ? 640 : 844, mobile: true, scheme: lang === 'de' ? 'dark' : 'light' });
    }
  }
} finally {
  await browser.close();
  writeReport(`${OUT}/report.json`, JSON.stringify(results, null, 2));
}
if (results.some((result) => !result.ok)) process.exitCode = 1;
