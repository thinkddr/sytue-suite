// SPDX-License-Identifier: Apache-2.0
// Real PlanCard, authenticated Account layout and CSP. No IdP or provider is contacted.
import { chromium } from 'playwright';
import { expect as baseExpect } from '@playwright/test';
import { mkdirSync, writeFileSync } from 'node:fs';
import { sealSession, hostCookieName, SESSION_COOKIE } from '../../apps/frontend/packages/auth/src/server/cookies.ts';
import { assertWizardAccessibility as accessible, describeWizardState, openWizardFixture } from './lib/wizard-checks.mjs';

const expect = baseExpect.configure({ timeout: 15000 });
const BASE = process.env.ACCOUNT_CAPACITY_URL || 'http://127.0.0.1:4343/account/dev/almacenamiento';
const OUT = process.env.ACCOUNT_CAPACITY_OUT || '/tmp/almacenamiento-externo/account';
mkdirSync(OUT, { recursive: true });
const secret = process.env.SYTUE_SESSION_SECRET;
if (!secret) throw new Error('The fixture runner must provide its ephemeral session secret');
const token = 'quota-fixture-token';
const session = await sealSession({
  clientId: process.env.SYTUE_OIDC_CLIENT_ID || 'prueba',
  user: {
    id: 'quota-fixture', email: 'quota@example.invalid', emailVerified: true,
    name: 'Capacity fixture', preferredUsername: 'capacity', roles: [], groups: [],
    orgKind: 'personal', plan: 'personal_free',
  },
  tokens: { accessToken: token, expiresAt: Date.now() + 3600000, tokenType: 'Bearer' },
}, secret);
const baseQuota = { limit_bytes: 100, used_bytes: 20, available_bytes: 80, external_storage: true };
const knownQuota = {
  ...baseQuota, limit_bytes: 1124, used_bytes: 220, available_bytes: 904,
  base_limit_bytes: 100, base_used_bytes: 20,
  external_capacity: { total_bytes: 1024, used_bytes: 200, available_bytes: 824 },
};
const quotas = {
  known: knownQuota,
  partial: { ...knownQuota, external_capacity: { ...knownQuota.external_capacity, partial: true } },
  unknown: { ...baseQuota, base_limit_bytes: 100, base_used_bytes: 20, external_capacity: { total_bytes: null, used_bytes: null, available_bytes: null, partial: true } },
  legacy: baseQuota,
};
const browser = await chromium.launch();
const results = [];
try {
  for (const width of [1280, 320]) {
    for (const [capacity, quota] of Object.entries(quotas)) {
      const name = `${capacity}-${width}`;
      const context = await browser.newContext({
        viewport: { width, height: 900 }, hasTouch: width === 320, isMobile: width === 320,
        reducedMotion: 'reduce', locale: 'en-GB',
      });
      const page = await context.newPage();
      const errors = [];
      const quotaRequests = [];
      let tokenReads = 0;
      page.on('pageerror', (error) => errors.push(error.message));
      page.on('request', (request) => {
        if (new URL(request.url()).pathname === '/account/auth/token') tokenReads++;
      });
      try {
        await context.addCookies([{
          name: hostCookieName(SESSION_COOKIE, false), value: session, url: new URL(BASE).origin,
          httpOnly: true, secure: false, sameSite: 'Lax',
        }]);
        await context.route('**/api/plan', (route) => route.fulfill({
          contentType: 'application/json', body: JSON.stringify({
            plan: 'personal_free', status: 'active', storage_bytes: 100,
            paid_until: null, upgrade_available: false,
          }),
        }));
        await context.route('**/api/v1/quota', (route) => {
          const request = route.request();
          const headers = {
            'access-control-allow-origin': new URL(BASE).origin,
            'access-control-allow-methods': 'GET, OPTIONS',
            'access-control-allow-headers': 'authorization',
          };
          if (request.method() === 'OPTIONS') return route.fulfill({ status: 204, headers });
          quotaRequests.push({ method: request.method(), authorization: request.headers().authorization });
          return route.fulfill({ headers, contentType: 'application/json', body: JSON.stringify(quota) });
        });
        await openWizardFixture(page, BASE);
        await expect(page).toHaveURL(BASE);
        await expect(page.getByRole('heading', { name: 'Sytue Account', exact: true })).toBeVisible();
        await expect(page.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '20');
        await expect(page.getByText('Connected storage extends your Sytue capacity.', { exact: true })).toBeVisible();
        await expect(page.getByText('Sytue: 20 B of 100 B', { exact: true })).toBeVisible();
        const known = capacity === 'known' || capacity === 'partial';
        await expect(page.getByText(known ? '220 B / 1.1 KB' : '20 B / 100 B', { exact: true })).toBeVisible();
        await expect(page.getByText('Connected storage adds 1.0 KB.', { exact: true })).toHaveCount(known ? 1 : 0);
        await expect(page.getByText('The provider does not report its capacity.', { exact: true })).toHaveCount(capacity === 'known' ? 0 : 1);
        expect(tokenReads).toBeGreaterThan(0);
        expect(quotaRequests).toEqual([{ method: 'GET', authorization: `Bearer ${token}` }]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1)).toBe(false);
        await accessible({ page }, 'main');
        expect(errors).toEqual([]);
        if (capacity === 'known') await page.screenshot({ path: `${OUT}/${name}.png`, animations: 'disabled' });
        results.push({ name, ok: true });
        console.log(`✓ account-capacity-${name}`);
      } catch (error) {
        results.push({ name, ok: false, error: error.stack || String(error) });
        console.error(`✗ account-capacity-${name}: ${error.message}`);
        console.error(JSON.stringify({ name, diagnostic: await describeWizardState({ page }).catch(String) }));
        await page.screenshot({ path: `${OUT}/failure-${name}.png`, animations: 'disabled' }).catch(() => {});
      } finally {
        await context.close();
      }
    }
  }
} finally {
  await browser.close();
  writeFileSync(`${OUT}/report.json`, JSON.stringify(results, null, 2));
}
if (results.some((result) => !result.ok)) process.exitCode = 1;
