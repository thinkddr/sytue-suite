// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import test from 'node:test';
import assert from 'node:assert/strict';
import { securityInventory } from './security-inventory.mjs';

const env = { GITHUB_REPOSITORY: 'thinkddr/sytue-suite', DEFAULT_BRANCH: 'main', GH_TOKEN: 'test-only-token' };
const alert = (number, state = 'open') => ({ number, state, rule: { id: 'js/xss' }, most_recent_instance: { location: { path: 'src/a.ts', start_line: 1 }, message: { text: 'Untrusted HTML' } } });

test('inventories all pages and projects only metadata, including fixed states', async () => {
  const requests = [];
  const result = await securityInventory({ env, fetchImpl: async (url, options) => {
    requests.push(String(url));
    assert.equal(url.origin, 'https://api.github.com');
    assert.equal(url.searchParams.get('ref'), 'refs/heads/main');
    assert.equal(options.redirect, 'error');
    assert.equal(options.headers.Authorization, 'Bearer test-only-token');
    const items = requests.length === 1 ? Array.from({ length: 100 }, (_, i) => alert(i + 1)) : [{ ...alert(101, 'fixed'), credential: env.GH_TOKEN }];
    return Response.json(items);
  } });
  assert.equal(requests.length, 2);
  assert.equal(result.alerts.length, 101);
  assert.equal(result.alerts[100].state, 'fixed');
  assert.ok(!JSON.stringify(result).includes(env.GH_TOKEN));
  assert.ok(!JSON.stringify(result).includes('credential'));
});

test('rejects permissions failures rather than reporting an empty inventory', async () => {
  await assert.rejects(securityInventory({ env, fetchImpl: async () => new Response('', { status: 403 }) }), /failed \(403\)/);
});

test('rejects duplicate, malformed and unexpected states', async () => {
  for (const items of [[alert(1), alert(1)], [alert(0)], [alert(1, 'unknown')], {}, [null]]) {
    await assert.rejects(securityInventory({ env, fetchImpl: async () => Response.json(items) }), /Invalid/);
  }
});

test('does not follow a redirect with the workflow credential', async () => {
  await assert.rejects(securityInventory({ env, fetchImpl: async () => ({ ok: true, status: 200, redirected: true }) }), /failed/);
});

test('refuses invalid configuration before sending credentials', async () => {
  for (const change of [{ GITHUB_REPOSITORY: 'evil.example/path/extra' }, { DEFAULT_BRANCH: '' }, { GH_TOKEN: '' }]) {
    await assert.rejects(securityInventory({ env: { ...env, ...change }, fetchImpl: () => assert.fail('must not fetch') }));
  }
});

test('refuses an oversized page', async () => {
  await assert.rejects(securityInventory({ env, fetchImpl: async () => new Response(' '.repeat(8 * 1024 * 1024 + 1)) }), /too large/);
});

test('does not return partial results when a later page fails', async () => {
  let page = 0;
  await assert.rejects(securityInventory({ env, fetchImpl: async () => ++page === 1
    ? Response.json(Array.from({ length: 100 }, (_, i) => alert(i + 1)))
    : new Response('', { status: 403 }) }), /failed \(403\)/);
});

test('fails rather than truncating at the pagination limit', async () => {
  let page = 0;
  await assert.rejects(securityInventory({ env, fetchImpl: async () => {
    const offset = page++ * 100;
    return Response.json(Array.from({ length: 100 }, (_, i) => alert(offset + i + 1)));
  } }), /pagination limit/);
});
