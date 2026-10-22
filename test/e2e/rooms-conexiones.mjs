// SPDX-License-Identifier: Apache-2.0
// Remote CI: real Rooms modal, synthetic Gateway responses and provider consent.
import { chromium } from 'playwright';
import { expect as baseExpect } from '@playwright/test';
import {
  assertWizardLayout as layout,
  assertWizardAccessibility as accessible,
  describeWizardState,
  openWizardFixture,
} from './lib/wizard-checks.mjs';
import { mkdirSync, writeFileSync } from 'node:fs';

const expect = baseExpect.configure({ timeout: 15000 });

const BASE = process.env.ROOMS_URL || 'http://127.0.0.1:4343/rooms/prueba/dev/conexiones';
const OUT = process.env.ROOMS_OUT || '/tmp/rooms-conexiones';
mkdirSync(OUT, { recursive: true });
const browser = await chromium.launch();
const results = [];
const PROVIDERS = {
  'slack-rooms': { name: 'Slack', host: 'slack.com', platform: 'slack' },
  'microsoft-teams-rooms': {
    name: 'Microsoft Teams',
    host: 'login.microsoftonline.com',
    platform: 'teams',
  },
  'google-chat': { name: 'Google Chat', host: 'accounts.google.com', platform: 'googlechat' },
};

async function mount(options = {}) {
  const context = await browser.newContext({
    viewport: { width: options.width || 1280, height: options.height || 900 },
    isMobile: Boolean(options.mobile),
    hasTouch: Boolean(options.mobile),
    reducedMotion: 'reduce',
    colorScheme: options.scheme || 'light',
  });
  const page = await context.newPage();
  page.setDefaultTimeout(30000);
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const state = {
    connected: new Set(options.connected || []),
    extraAccounts: [],
    created: new Set(options.connected || []),
    requests: [],
    visits: [],
    bridges: [],
    consentFailure: Boolean(options.consentFailure),
    denied: Boolean(options.denied),
    destinationFailure: Boolean(options.destinationFailure),
    bridgeFailure: Boolean(options.bridgeFailure),
    available: options.available !== false,
    disconnectFailure: Boolean(options.disconnectFailure),
    revocations: [],
    providerReady: options.providerReady !== false,
  };
  const json = (route, body, status = 200) =>
    route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
  await context.route('**/rooms/api/integrations*', (route) =>
    json(route, {
      incoming: [],
      outgoing: [],
      bots: [],
      bridges: state.bridges,
      whatsapp: [],
      teams: [],
    }),
  );
  await context.route('**/rooms/api/commands*', (route) => json(route, { commands: [] }));
  await context.route('**/rooms/api/app-connections*', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const body = request.method() === 'POST' ? request.postDataJSON() : null;
    state.requests.push({
      method: request.method(),
      instance: url.searchParams.get('instance'),
      body,
    });
    if (request.method() === 'GET') {
      if (url.searchParams.has('instance')) {
        if (state.destinationFailure) return json(route, { error: 'provider_unavailable' }, 502);
        return json(route, {
          items: [
            { id: 'remote-general', name: 'General' },
            { id: 'remote-design', name: 'Design and planning' },
          ],
        });
      }
      return json(route, {
        available: state.available,
        providers: Object.entries(PROVIDERS).map(([key, provider]) => ({
          key,
          name: provider.name,
          scopes: [],
          connected: state.connected.has(key),
          available: state.providerReady,
          ...(state.created.has(key) ? { instance_id: `account-${key}` } : {}),
        })),
        items: [...[...state.created].map((key) => ({
          id: `account-${key}`,
          template_key: key,
          name: PROVIDERS[key].name,
          connected: state.connected.has(key),
        })), ...state.extraAccounts],
      });
    }
    if (body.action === 'consent') {
      if (state.consentFailure) return json(route, { error: 'consent_failed' }, 503);
      const provider = PROVIDERS[body.provider];
      let instance = body.instance_id;
      if (!instance && body.new_account && state.created.has(body.provider)) {
        instance = `account-${body.provider}-${state.extraAccounts.length+2}`;
        state.extraAccounts.push({ id: instance, template_key: body.provider, name: `${provider.name} second account`, connected: false });
      }
      if (!instance) { instance = `account-${body.provider}`; state.created.add(body.provider); }
      return json(route, {
        instance_id: instance,
        authorize_url: `https://${provider.host}/sytue-test-consent?provider=${body.provider}&instance=${instance}`,
      });
    }
    if (body.action === 'connect-bridge') {
      if (state.bridgeFailure) return json(route, { error: 'bridge_rejected' }, 422);
      const key = state.extraAccounts.find((item) => item.id === body.instance_id)?.template_key ?? body.instance_id.replace('account-', '');
      const bridge = {
        id: `bridge-${key}`,
        platform: PROVIDERS[key].platform,
        channelId: body.channel_id,
        target: body.remote_id,
      };
      state.bridges.push(bridge);
      return json(route, { bridge }, 201);
    }
    if (body.action === 'disconnect') {
      if (state.disconnectFailure) return json(route, { error: 'revocation_failed' }, 502);
      const key = state.extraAccounts.find((item) => item.id === body.instance_id)?.template_key ?? body.instance_id.replace('account-', '');
      const extra = state.extraAccounts.find((item) => item.id === body.instance_id);
      if (extra) extra.connected = false; else state.connected.delete(key);
      state.revocations.push(body.instance_id);
      return json(route, { ok: true });
    }
    return json(route, { error: 'unexpected_action' }, 400);
  });
  for (const host of new Set(Object.values(PROVIDERS).map((provider) => provider.host))) {
    await context.route(`https://${host}/sytue-test-consent*`, async (route) => {
      const consentURL = new URL(route.request().url());
      const key = consentURL.searchParams.get('provider');
      const instance = consentURL.searchParams.get('instance');
      state.visits.push(key);
      const callback = new URL(BASE);
      callback.searchParams.set('lang', 'en');
      callback.searchParams.set('keep', 'preserved');
      callback.searchParams.set('connect-app', '1');
      callback.searchParams.set('instance', instance);
      if (state.denied) callback.searchParams.set('oauth_error', 'access_denied');
      else { const extra = state.extraAccounts.find((item) => item.id === instance); if (extra) extra.connected = true; else state.connected.add(key); }
      await route.fulfill({ status: 302, headers: { location: callback.href } });
    });
  }
  const url = new URL(BASE);
  url.searchParams.set('lang', options.lang || 'en');
  await openWizardFixture(page, url.href);
  const trigger = page.locator('main').getByRole('button').first();
  await trigger.waitFor({ state: 'visible', timeout: 60000 });
  await trigger.click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible({ timeout: 180000 });
  await page.evaluate((scheme) => {
    document.documentElement.dataset.theme = scheme;
  }, options.scheme || 'light');
  return { context, page, dialog, state, errors };
}

// Cada app tiene su tarjeta en la pestaña «Apps»: ya no hay un «Conectar» genérico.
async function openWizard(ui, provider) {
  await ui.dialog
    .getByRole('button', { name: `Continue in ${PROVIDERS[provider].name}`, exact: true })
    .click();
  await expect(ui.page.getByRole('dialog')).toHaveCount(1);
}
async function authorize(ui, key) {
  await openWizard(ui, key);
  await expect(
    ui.dialog.getByRole('heading', { name: 'Authorize your account', exact: true }),
  ).toBeVisible();
  await expect(ui.dialog.locator('input[type="password"]')).toHaveCount(0);
  await ui.dialog
    .getByRole('button', { name: `Continue in ${PROVIDERS[key].name}`, exact: true })
    .click();
  await ui.page.waitForURL((url) => url.searchParams.get('keep') === 'preserved', { timeout: 30000 });
  await expect(ui.dialog).toBeVisible({ timeout: 30000 });
  await expect(
    ui.dialog.getByRole('heading', { name: 'Choose the channels', exact: true }),
  ).toBeVisible({ timeout: 30000 });
  expect(ui.state.visits).toEqual([key]);
  expect(new URL(ui.page.url()).searchParams.get('keep')).toBe('preserved');
  expect(new URL(ui.page.url()).searchParams.has('instance')).toBe(false);
}
async function reviewBridge(ui) {
  await ui.dialog.locator('#app-rooms-channel').selectOption('local-general');
  await ui.dialog.locator('#app-remote-channel').selectOption('remote-general');
  await ui.dialog.getByRole('button', { name: 'Continue', exact: true }).click();
  await expect(ui.dialog.getByRole('heading', { name: 'Review', exact: true })).toBeVisible();
  await expect(
    ui.dialog.getByText(
      'New messages in #general will send a notification and a Rooms link to General.',
      { exact: true },
    ),
  ).toBeVisible();
}
async function capture(ui, name) {
  await ui.page.screenshot({ path: `${OUT}/${name}.png`, animations: 'disabled' });
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
    if (ui)
      console.error(
        JSON.stringify({ name, diagnostic: await describeWizardState(ui).catch(String) }),
      );
    if (ui) await capture(ui, `failure-${name}`).catch(() => {});
  } finally {
    if (ui) await ui.context.close();
  }
}

try {
  for (const key of Object.keys(PROVIDERS)) {
    await scenario(`${key}-add-second-account-selects-the-new-identity`, async (ui) => {
      await openWizard(ui, key);
      await expect(ui.dialog.locator('#app-connected-account')).toHaveValue(`account-${key}`);
      await ui.dialog.getByRole('button', { name: 'Add account', exact: true }).click();
      await ui.dialog.getByRole('button', { name: `Continue in ${PROVIDERS[key].name}`, exact: true }).click();
      await expect(ui.dialog.locator('#app-connected-account')).toHaveValue(`account-${key}-2`);
      expect(ui.state.requests.find((request) => request.body?.action === 'consent').body).toEqual({ action: 'consent', provider: key, new_account: true });
      expect(ui.state.connected.has(key)).toBe(true);
      expect(ui.state.extraAccounts).toHaveLength(1);
      expect(ui.state.extraAccounts[0].connected).toBe(true);
    }, { connected: [key] });
  }
  await scenario('unconfigured-provider-does-not-offer-authorizing', async (ui) => {
    await openWizard(ui, 'google-chat');
    await expect(ui.dialog.getByText('Connecting to Google Chat has not been configured yet.', { exact: true })).toBeVisible();
    await expect(ui.dialog.getByRole('button', { name: 'Continue in Google Chat', exact: true })).toBeDisabled();
    expect(ui.state.requests.filter((request) => request.method === 'POST')).toHaveLength(0);
    expect(ui.state.visits).toEqual([]);
    await layout(ui);
    await accessible(ui);
    await capture(ui, 'unconfigured-provider-mobile');
  }, { providerReady: false, width: 320, height: 720, mobile: true });

  await scenario('unconfigured-connected-account-still-allows-revocation', async (ui) => {
    await openWizard(ui, 'slack-rooms');
    await expect(ui.dialog.getByText('Connecting to Slack has not been configured yet.', { exact: true })).toBeVisible();
    await expect(ui.dialog.getByRole('button', { name: 'Continue', exact: true })).toBeDisabled();
    await ui.dialog.getByRole('button', { name: 'Disconnect', exact: true }).click();
    await ui.dialog.getByRole('status').filter({ hasText: 'Disconnect Slack?' })
      .getByRole('button', { name: 'Disconnect', exact: true }).click();
    await expect(ui.dialog.getByText('Account disconnected', { exact: true })).toBeVisible();
    expect(ui.state.revocations).toEqual(['account-slack-rooms']);
  }, { providerReady: false, connected: ['slack-rooms'] });

  for (const key of ['slack-rooms', 'microsoft-teams-rooms', 'google-chat']) {
    await scenario(`${key}-consent-and-share`, async (ui) => {
      await authorize(ui, key);
      await reviewBridge(ui);
      await accessible(ui);
      await ui.dialog.getByRole('button', { name: 'Connect channels', exact: true }).click();
      await expect(
        ui.dialog.getByRole('heading', { name: 'Connection ready', exact: true }),
      ).toBeFocused();
      const request = ui.state.requests.find(
        (request) => request.body?.action === 'connect-bridge',
      );
      expect(request.body).toEqual({
        action: 'connect-bridge',
        instance_id: `account-${key}`,
        channel_id: 'local-general',
        remote_id: 'remote-general',
      });
      expect(ui.state.bridges).toHaveLength(1);
      await capture(ui, `${key}-connected`);
    });
  }
  await scenario(
    'existing-account-skips-consent-destinations-retry',
    async (ui) => {
      await openWizard(ui, 'slack-rooms');
      await expect(
        ui.dialog.getByRole('heading', { name: 'Choose the channels', exact: true }),
      ).toBeVisible();
      await expect(ui.dialog.getByRole('alert')).toContainText(
        'The connection could not be completed.',
      );
      await expect(ui.dialog.getByRole('button', { name: 'Continue', exact: true })).toBeDisabled();
      ui.state.destinationFailure = false;
      await ui.dialog.getByRole('button', { name: 'Retry', exact: true }).click();
      await reviewBridge(ui);
      expect(ui.state.visits).toEqual([]);
    },
    { connected: ['slack-rooms'], destinationFailure: true },
  );
  await scenario(
    'denied-consent-can-be-retried',
    async (ui) => {
      await openWizard(ui, 'slack-rooms');
      await ui.dialog.getByRole('button', { name: 'Continue in Slack', exact: true }).click();
      await expect(ui.dialog.getByRole('alert')).toContainText('Access was not granted.');
      expect(
        ui.state.requests.filter((request) => request.body?.action === 'connect-bridge'),
      ).toHaveLength(0);
      ui.state.denied = false;
      await ui.dialog.getByRole('button', { name: 'Continue in Slack', exact: true }).click();
      await expect(
        ui.dialog.getByRole('heading', { name: 'Choose the channels', exact: true }),
      ).toBeVisible();
    },
    { denied: true },
  );
  await scenario(
    'consent-start-failure-stays-actionable',
    async (ui) => {
      await openWizard(ui, 'google-chat');
      await ui.dialog.getByRole('button', { name: 'Continue in Google Chat', exact: true }).click();
      await expect(ui.dialog.getByRole('alert')).toContainText(
        'The connection could not be completed.',
      );
      expect(ui.state.visits).toEqual([]);
      ui.state.consentFailure = false;
      await ui.dialog.getByRole('button', { name: 'Continue in Google Chat', exact: true }).click();
      await expect(
        ui.dialog.getByRole('heading', { name: 'Choose the channels', exact: true }),
      ).toBeVisible();
    },
    { consentFailure: true },
  );
  await scenario(
    'bridge-test-rejected-then-retried',
    async (ui) => {
      await openWizard(ui, 'slack-rooms');
      await reviewBridge(ui);
      await ui.dialog.getByRole('button', { name: 'Connect channels', exact: true }).click();
      await expect(ui.dialog.getByRole('alert')).toContainText(
        'The connection could not be completed.',
      );
      expect(ui.state.bridges).toHaveLength(0);
      ui.state.bridgeFailure = false;
      await ui.dialog.getByRole('button', { name: 'Connect channels', exact: true }).click();
      await expect(
        ui.dialog.getByRole('heading', { name: 'Connection ready', exact: true }),
      ).toBeVisible();
      expect(ui.state.bridges).toHaveLength(1);
    },
    { connected: ['slack-rooms'], bridgeFailure: true },
  );
  await scenario(
    'unavailable-account-does-not-offer-consent',
    async (ui) => {
      await openWizard(ui, 'slack-rooms');
      await expect(
        ui.dialog.getByText('Connecting apps is not available for your account.', { exact: true }),
      ).toBeVisible();
      await expect(ui.dialog.locator('input[type="radio"]')).toHaveCount(0);
      expect(ui.state.requests.filter((request) => request.method === 'POST')).toHaveLength(0);
    },
    { available: false },
  );
  await scenario(
    'keyboard-provider-review-and-back',
    async (ui) => {
      await openWizard(ui, 'slack-rooms');
      await expect(
        ui.dialog.getByRole('heading', { name: 'Choose the channels', exact: true }),
      ).toBeFocused();
      await ui.dialog.locator('#app-remote-channel').selectOption('remote-general');
      await ui.dialog.getByRole('button', { name: 'Continue', exact: true }).focus();
      await ui.page.keyboard.press('Enter');
      await expect(ui.dialog.getByRole('heading', { name: 'Review', exact: true })).toBeFocused();
      await ui.page.keyboard.press('Tab');
      expect(
        await ui.page.evaluate(() => Boolean(document.activeElement?.closest('[role="dialog"]'))),
      ).toBe(true);
      await ui.dialog.getByRole('button', { name: 'Back', exact: true }).focus();
      await ui.page.keyboard.press('Enter');
      await expect(
        ui.dialog.getByRole('heading', { name: 'Choose the channels', exact: true }),
      ).toBeFocused();
      await expect(ui.dialog.locator('#app-remote-channel')).toHaveValue('remote-general');
    },
    { connected: ['slack-rooms'] },
  );
  await scenario(
    'connected-account-disconnect-inline-confirmation',
    async (ui) => {
      await openWizard(ui, 'slack-rooms');
      await expect(ui.dialog.getByText('Connected', { exact: true })).toBeVisible();
      await ui.dialog.getByRole('button', { name: 'Disconnect', exact: true }).click();
      await expect(ui.page.getByRole('dialog')).toHaveCount(1);
      const confirmation = ui.dialog.getByRole('status').filter({ hasText: 'Disconnect Slack?' });
      await expect(confirmation).toContainText(
        'Rooms will stop sending notifications and links through this account.',
      );
      await confirmation.getByRole('button', { name: 'Cancel', exact: true }).click();
      expect(ui.state.revocations).toEqual([]);
      await ui.dialog.getByRole('button', { name: 'Disconnect', exact: true }).click();
      await confirmation.getByRole('button', { name: 'Disconnect', exact: true }).click();
      await expect(ui.dialog.getByText('Account disconnected', { exact: true })).toBeVisible();
      expect(ui.state.revocations).toEqual(['account-slack-rooms']);
      expect(
        ui.state.requests
          .filter((request) => request.body?.action === 'disconnect')
          .map((request) => request.body),
      ).toEqual([{ action: 'disconnect', instance_id: 'account-slack-rooms' }]);
      await expect(ui.dialog.getByText('Connected', { exact: true })).toHaveCount(0);
    },
    { connected: ['slack-rooms'] },
  );
  await scenario(
    'revocation-failure-keeps-account-connected-for-retry',
    async (ui) => {
      await openWizard(ui, 'google-chat');
      await ui.dialog.getByRole('button', { name: 'Disconnect', exact: true }).click();
      const confirmation = ui.dialog
        .getByRole('status')
        .filter({ hasText: 'Disconnect Google Chat?' });
      await confirmation.getByRole('button', { name: 'Disconnect', exact: true }).click();
      await expect(ui.dialog.getByRole('alert')).toContainText(
        'The connection could not be completed.',
      );
      await expect(ui.dialog.getByText('Connected', { exact: true })).toBeVisible();
      expect(ui.state.revocations).toEqual([]);
      ui.state.disconnectFailure = false;
      await confirmation.getByRole('button', { name: 'Disconnect', exact: true }).click();
      await expect(ui.dialog.getByText('Account disconnected', { exact: true })).toBeVisible();
      expect(ui.state.revocations).toEqual(['account-google-chat']);
    },
    { connected: ['google-chat'], disconnectFailure: true },
  );
  for (const width of [320, 390]) {
    await scenario(
      `mobile-${width}-single-modal-footer-keyboard`,
      async (ui) => {
        await openWizard(ui, 'microsoft-teams-rooms');
        await expect(
          ui.dialog.getByRole('heading', { name: 'Choose the channels', exact: true }),
        ).toBeFocused();
        await layout(ui);
        await accessible(ui);
        await capture(ui, `mobile-${width}-channels`);
        await reviewBridge(ui);
        await ui.page.setViewportSize({ width, height: 400 });
        await layout(ui);
        await accessible(ui);
        await capture(ui, `mobile-${width}-review-short-viewport`);
        const active = await ui.page.evaluate(() => document.activeElement?.textContent);
        expect(active).toContain('Review');
      },
      {
        width,
        height: 720,
        mobile: true,
        connected: ['microsoft-teams-rooms'],
        scheme: width === 320 ? 'light' : 'dark',
      },
    );
  }
} finally {
  writeFileSync(`${OUT}/results.json`, JSON.stringify(results, null, 2));
  await browser.close();
}
if (results.some((result) => !result.ok)) process.exitCode = 1;
