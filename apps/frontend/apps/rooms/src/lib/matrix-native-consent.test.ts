/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { matrixNativeConsent } from './matrix-native-consent.js';

const current = {
  actor: 'fixture-owner',
  state: 'a'.repeat(43),
  origin: 'https://rooms.sytue.test',
  basePath: '/rooms',
};
const home = {
  actor: current.actor,
  tenant: 'acme',
  homeserver_url: 'https://matrix.sytue.test',
  sso_provider_id: 'oidc-sytue',
};
const callback = new URL('/rooms/matrix/native/callback', current.origin);
callback.searchParams.set('state', current.state);
const target = new URL('/_matrix/client/v3/login/sso/redirect/oidc-sytue', home.homeserver_url);
target.searchParams.set('redirectUrl', callback.href);
const preview = {
  ...home,
  state: current.state,
  authorize_url: target.href,
  product: 'docs',
  purpose: 'resources:docs',
  resource_approved: false,
};

test('resource handoff requires an explicit app-specific approval before SSO', () => {
  const unapproved = matrixNativeConsent(preview, home, current);
  expect(unapproved.requiresApproval).toBe(true);
  expect(unapproved.app?.name).toBe('Sytue Docus');
  expect(unapproved.tenant).toBe('acme');
  expect(
    matrixNativeConsent({ ...preview, resource_approved: true }, home, current).requiresApproval,
  ).toBe(false);
  expect(matrixNativeConsent({ ...preview, purpose: '' }, home, current).app).toBeUndefined();
});

const { resource_approved: omittedApproval, ...withoutApproval } = preview;
void omittedApproval;

test('changed account, tenant, purpose, SSO origin and callback cannot bypass consent', () => {
  for (const changed of [
    { ...preview, actor: 'another-account' },
    { ...preview, tenant: 'another-tenant' },
    { ...preview, purpose: 'resources:mail' },
    { ...preview, product: 'unregistered' },
    withoutApproval,
    { ...preview, state: 'b'.repeat(43) },
    { ...preview, authorize_url: target.href.replace('matrix.sytue.test', 'matrix.attacker.test') },
  ])
    expect(() => matrixNativeConsent(changed, home, current)).toThrow();
  const replaced = new URL(target);
  replaced.searchParams.set(
    'redirectUrl',
    'https://attacker.test/rooms/matrix/native/callback?state=' + current.state,
  );
  expect(() =>
    matrixNativeConsent({ ...preview, authorize_url: replaced.href }, home, current),
  ).toThrow();
});
