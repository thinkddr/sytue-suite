// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { reviewSecurityAlerts, validateReview } from './security-review.mjs';

const head = 'a'.repeat(40);
const env = { GITHUB_REPOSITORY: 'thinkddr/sytue-suite', DEFAULT_BRANCH: 'main', GH_TOKEN: 'test-only-token', GITHUB_SHA: head };
const path = 'services/example.go';
const manifest = { version: 1, files: [{ path, blob: 'b'.repeat(40) }], reviews: [{ number: 1, rule: 'go/log-injection', path, comment: 'Reviewed numeric field: it has a fixed integer type and cannot contain a newline or add a log record. Source and regression evidence are pinned.' }] };
const alert = (state = 'open') => ({ number: 1, state, tool: { name: 'CodeQL' }, rule: { id: 'go/log-injection' }, most_recent_instance: { ref: 'refs/heads/main', location: { path } } });

function fixture(options = {}) {
  const calls = [];
  let heads = 0;
  return { calls, fetchImpl: async (url, request) => {
    assert.ok(url.startsWith('https://api.github.com/repos/thinkddr/sytue-suite/'));
    assert.equal(request.redirect, 'error');
    calls.push({ url, ...request });
    if (url.includes('/git/ref/')) return Response.json({ object: { sha: options.moved && ++heads > 1 ? 'c'.repeat(40) : head } });
    if (request.method === 'PATCH') {
      const body = JSON.parse(request.body);
      assert.deepEqual(body, { state: 'dismissed', dismissed_reason: 'false positive', dismissed_comment: manifest.reviews[0].comment });
      return Response.json({ ...alert('dismissed'), dismissed_reason: 'false positive' });
    }
    return Response.json(options.changed ? { ...alert(), rule: { id: 'go/new-rule' } } : alert(options.state));
  } };
}

test('dry run validates metadata but never writes alert state', async () => {
  const api = fixture();
  const result = await reviewSecurityAlerts({ manifest, env, evidence: () => {}, fetchImpl: api.fetchImpl });
  assert.equal(result.results[0].state, 'open');
  assert.equal(result.results[0].applied, false);
  assert.equal(api.calls.filter(c => c.method === 'PATCH').length, 0);
});

test('apply uses only the approved reason/comment after rechecking the branch', async () => {
  const api = fixture();
  const result = await reviewSecurityAlerts({ manifest, env: { ...env, APPLY_REVIEW: 'true' }, evidence: () => {}, fetchImpl: api.fetchImpl });
  assert.equal(result.results[0].state, 'dismissed');
  assert.equal(result.results[0].applied, true);
  assert.equal(api.calls.filter(c => c.method === 'PATCH').length, 1);
  assert.ok(!JSON.stringify(result).includes(env.GH_TOKEN));
});

test('already fixed alerts retain their fixed state', async () => {
  const api = fixture({ state: 'fixed' });
  const result = await reviewSecurityAlerts({ manifest, env: { ...env, APPLY_REVIEW: 'true' }, evidence: () => {}, fetchImpl: api.fetchImpl });
  assert.equal(result.results[0].state, 'fixed');
  assert.equal(api.calls.filter(c => c.method === 'PATCH').length, 0);
});

test('metadata changes, branch advancement and stale evidence block writes', async () => {
  for (const options of [{ changed: true }, { moved: true }]) {
    const api = fixture(options);
    await assert.rejects(reviewSecurityAlerts({ manifest, env: { ...env, APPLY_REVIEW: 'true' }, evidence: () => {}, fetchImpl: api.fetchImpl }));
    assert.equal(api.calls.filter(c => c.method === 'PATCH').length, 0);
  }
  await assert.rejects(reviewSecurityAlerts({ manifest, env, evidence: () => { throw new Error('Evidence changed'); }, fetchImpl: () => assert.fail() }), /Evidence changed/);
});

test('validates all candidates before the first write', async () => {
  const api = fixture();
  const two = { ...manifest, reviews: [...manifest.reviews, { ...manifest.reviews[0], number: 2 }] };
  await assert.rejects(reviewSecurityAlerts({ manifest: two, env: { ...env, APPLY_REVIEW: 'true' }, evidence: () => {}, fetchImpl: api.fetchImpl }), /changed/);
  assert.equal(api.calls.filter(c => c.method === 'PATCH').length, 0);
});

test('invalid review manifests fail before API requests', () => {
  for (const invalid of [
    { ...manifest, files: [{ path: 'services/../../secret.go', blob: 'b'.repeat(40) }] },
    { ...manifest, reviews: [...manifest.reviews, manifest.reviews[0]] },
    { ...manifest, reviews: [{ ...manifest.reviews[0], comment: 'no evidence' }] },
    { ...manifest, reviews: [{ ...manifest.reviews[0], path: 'services/other.go' }] },
    { ...manifest, reviews: [{ ...manifest.reviews[0], comment: 'a'.repeat(281) }] },
    { ...manifest, files: [{ path: '.github/secrets.env', blob: 'b'.repeat(40) }] },
  ]) assert.throws(() => validateReview(invalid));
});

test('review evidence may pin a tracked automation source file', () => {
  const source = '.github/scripts/security-review.mjs';
  validateReview({ ...manifest, files: [{ path: source, blob: 'b'.repeat(40) }], reviews: [{ ...manifest.reviews[0], path: source }] });
});

test('does not claim a concurrent closure as an applied mutation', async () => {
  let reads = 0;
  let writes = 0;
  const result = await reviewSecurityAlerts({ manifest, env: { ...env, APPLY_REVIEW: 'true' }, evidence: () => {}, fetchImpl: async (url, options) => {
    if (url.includes('/git/ref/')) return Response.json({ object: { sha: head } });
    if (options.method === 'PATCH') writes++;
    return Response.json(alert(++reads === 1 ? 'open' : 'fixed'));
  } });
  assert.equal(writes, 0);
  assert.equal(result.results[0].state, 'fixed');
  assert.equal(result.results[0].applied, false);
});

test('the committed review list has specific evidence for every selected alert', () => {
  validateReview(JSON.parse(readFileSync(new URL('../security-reviewed-alerts.json', import.meta.url), 'utf8')));
});
