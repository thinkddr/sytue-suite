// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import test from 'node:test';
import assert from 'node:assert/strict';
import { goSecurityTraces } from './security-traces.mjs';

const env = { GITHUB_REPOSITORY: 'thinkddr/sytue-suite', DEFAULT_BRANCH: 'main' };
const analysis = { id: 42, category: '/language:go', tool: { name: 'CodeQL' }, commit_sha: 'a'.repeat(40) };

test('reads Go SARIF through the fixed API path and projects trace locations', () => {
  const calls = [];
  const result = goSecurityTraces({ env, api: (path, accept) => {
    calls.push(path);
    if (calls.length === 1) return [{ ...analysis, category: '/language:python' }, analysis];
    assert.equal(path, 'repos/thinkddr/sytue-suite/code-scanning/analyses/42');
    assert.equal(accept, 'application/sarif+json');
    const location = { physicalLocation: { artifactLocation: { index: 0 }, region: { startLine: 10, startColumn: 2 } }, message: { text: 'source' } };
    return { token: 'not exported', runs: [{ artifacts: [{ location: { uri: 'src/a.go' } }], results: [{ ruleId: 'go/log-injection', locations: [location], codeFlows: [{ threadFlows: [{ locations: [{ location }] }] }] }] }] };
  } });
  assert.equal(result.analyzedCommit, 'a'.repeat(40));
  assert.equal(result.results[0].flows[0][0].path, 'src/a.go');
  assert.equal(result.results[0].flows[0][0].line, 10);
  assert.ok(!JSON.stringify(result).includes('not exported'));
});

test('does not follow attacker-controlled analysis IDs', () => {
  let calls = 0;
  assert.throws(() => goSecurityTraces({ env, api: () => { calls++; return [{ ...analysis, id: 'https://evil.example/' }]; } }), /valid Go analysis/);
  assert.equal(calls, 1);
});

test('fails explicitly for missing analysis, invalid SARIF and API denial', () => {
  assert.throws(() => goSecurityTraces({ env, api: () => [] }), /valid Go analysis/);
  assert.throws(() => goSecurityTraces({ env, api: path => path.endsWith('/42') ? {} : [analysis] }), /Invalid SARIF/);
  assert.throws(() => goSecurityTraces({ env, api: () => { throw new Error('403'); } }), /403/);
});

test('can inspect a PR analysis while still executing trusted default-branch code', () => {
  let seen = '';
  goSecurityTraces({ env: { ...env, ANALYSIS_REF: 'refs/pull/40/merge' }, api: path => {
    if (path.endsWith('/42')) return { runs: [] };
    seen = path;
    return [analysis];
  } });
  assert.ok(seen.includes('ref=refs%2Fpull%2F40%2Fmerge'));
  assert.throws(() => goSecurityTraces({ env: { ...env, ANALYSIS_REF: 'https://evil.example' }, api: () => assert.fail() }), /Invalid analysis ref/);
});
