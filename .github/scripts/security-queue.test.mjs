// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { selectSecurityFinding } from './security-queue.mjs';

const baseEnv = () => ({
  GITHUB_EVENT_NAME: 'schedule', GITHUB_REPOSITORY: 'example/repo', DEFAULT_BRANCH: 'main',
  GITHUB_API_URL: 'https://api.github.com', GH_TOKEN: 'prs-private-token',
  CODE_SCANNING_TOKEN: 'code-private-token', SECURITY_ALERTS_TOKEN: 'dependabot-private-token',
});
const code = (number, severity = 'high', overrides = {}) => ({ number, state: 'open', rule: { security_severity_level: severity, severity: 'warning' }, ...overrides });
const dependency = (number, severity = 'high', overrides = {}) => ({ number, state: 'open', security_advisory: { severity }, ...overrides });
const pr = (number, overrides = {}) => ({
  number, state: 'open', title: 'An ordinary PR', body: '',
  head: { ref: 'ordinary-branch', repo: { full_name: 'example/repo' } }, ...overrides,
});
const request = (number, type, alertNumber, overrides = {}) => pr(number, {
  head: { ref: `codex/security/${type}-${alertNumber}-100-1`, repo: { full_name: 'example/repo' } },
  body: `<!-- codex-security-finding: example/repo ${type} ${alertNumber} -->`, ...overrides,
});
const json = (value, options) => new Response(JSON.stringify(value), { status: 200, ...options });

function api({ pulls = [], codeAlerts = [], dependencies = [], onRequest } = {}) {
  const calls = [];
  const fetchImpl = async (url, options) => {
    calls.push({ url, options });
    onRequest?.(url, options);
    const parsed = new URL(url);
    let source;
    if (parsed.pathname.endsWith('/pulls')) source = pulls;
    else if (parsed.pathname.endsWith('/code-scanning/alerts')) source = codeAlerts;
    else if (parsed.pathname.endsWith('/dependabot/alerts')) source = dependencies;
    else assert.fail('Unexpected API endpoint.');
    const page = Number(parsed.searchParams.get('page') || '1');
    return json(source.slice((page - 1) * 100, page * 100));
  };
  return { calls, fetchImpl };
}

test('scheduled selection prioritizes severity, alert number, then type deterministically', async () => {
  const mock = api({
    codeAlerts: [code(1, 'low'), code(5, 'high'), code(8, 'critical'), code(9, 'critical')],
    dependencies: [dependency(4, 'critical'), dependency(2, 'medium')],
  });
  assert.deepEqual(await selectSecurityFinding({ env: baseEnv(), fetchImpl: mock.fetchImpl }), {
    hasFinding: true, type: 'dependabot', number: 4, reason: 'selected',
  });
  const tied = api({ codeAlerts: [code(4, 'critical')], dependencies: [dependency(4, 'critical')] });
  assert.equal((await selectSecurityFinding({ env: baseEnv(), fetchImpl: tied.fetchImpl })).type, 'code-scanning');
});

test('all requests are locally built, scoped to the default branch, and use separate tokens', async () => {
  const env = { ...baseEnv(), DEFAULT_BRANCH: 'release/production' };
  const mock = api({ onRequest: (url, options) => {
    const parsed = new URL(url);
    assert.equal(parsed.origin, 'https://api.github.com');
    assert.match(parsed.pathname, /^\/repos\/example\/repo\/(pulls|code-scanning\/alerts|dependabot\/alerts)$/);
    assert.equal(parsed.searchParams.get('per_page'), '100');
    assert.equal(parsed.searchParams.get('sort'), 'created');
    assert.equal(parsed.searchParams.get('direction'), 'asc');
    assert.equal(options.redirect, 'error');
    assert.ok(options.signal instanceof AbortSignal);
    if (parsed.pathname.endsWith('/pulls')) {
      assert.equal(parsed.searchParams.get('state'), 'all');
      assert.equal(options.headers.Authorization, 'Bearer prs-private-token');
    } else {
      assert.equal(parsed.searchParams.get('state'), 'open');
      if (parsed.pathname.endsWith('/code-scanning/alerts')) {
        assert.equal(parsed.searchParams.get('ref'), 'refs/heads/release/production');
        assert.equal(options.headers.Authorization, 'Bearer code-private-token');
      } else assert.equal(options.headers.Authorization, 'Bearer dependabot-private-token');
    }
  } });
  assert.equal((await selectSecurityFinding({ env, fetchImpl: mock.fetchImpl })).reason, 'no-open-alerts');
  assert.equal(mock.calls.length, 3);
});

test('historical branches and canonical markers deduplicate types independently, including closed PRs', async () => {
  const mock = api({
    pulls: [request(1, 'code-scanning', 42, { state: 'closed' }), pr(2, { state: 'closed', body: '<!-- codex-security-finding: example/repo dependabot 41 -->' })],
    codeAlerts: [code(42), code(43, 'medium')], dependencies: [dependency(41), dependency(42)],
  });
  assert.deepEqual(await selectSecurityFinding({ env: baseEnv(), fetchImpl: mock.fetchImpl }), {
    hasFinding: true, type: 'dependabot', number: 42, reason: 'selected',
  });
});

test('canonical open PR URLs and explicit code-scanning title ranges exclude preexisting fixes', async () => {
  const mock = api({
    pulls: [pr(18, { title: '[WIP] Fix code scanning alerts #154 to #167' }), pr(19, { body: 'Fixes [alert](https://github.com/example/repo/security/dependabot/12).' })],
    codeAlerts: [code(154), code(160), code(167), code(168, 'medium')], dependencies: [dependency(12)],
  });
  assert.deepEqual(await selectSecurityFinding({ env: baseEnv(), fetchImpl: mock.fetchImpl }), {
    hasFinding: true, type: 'code-scanning', number: 168, reason: 'selected',
  });
});

test('unrelated links, markers, bare numbers, closed human fixes, and fork branch names do not reserve findings', async () => {
  const fakeBranch = request(1, 'code-scanning', 7);
  fakeBranch.head.repo.full_name = 'someone/fork';
  const mock = api({
    pulls: [fakeBranch, pr(2, { body: '<!-- codex-security-finding: other/repo code-scanning 7 -->\nhttps://github.com/other/repo/security/code-scanning/7\nhttps://github.com/example/repo/security/code-scanning/70\nhttps://github.com/example/repo/security/code-scanning/7?query=1', title: 'Issue #7' }), pr(3, { state: 'closed', title: 'Fix code scanning alert #7', body: 'https://github.com/example/repo/security/code-scanning/7' })],
    codeAlerts: [code(7)],
  });
  assert.equal((await selectSecurityFinding({ env: baseEnv(), fetchImpl: mock.fetchImpl })).number, 7);
});

test('a closed fork marker cannot permanently reserve an alert or consume capacity', async () => {
  const fork = request(1, 'code-scanning', 7, { state: 'closed', body: '<!-- codex-security-finding: example/repo code-scanning 7 -->' });
  fork.head.repo.full_name = 'someone/fork';
  assert.deepEqual(await selectSecurityFinding({ env: baseEnv(), fetchImpl: api({ pulls: [fork], codeAlerts: [code(7)] }).fetchImpl }), {
    hasFinding: true, type: 'code-scanning', number: 7, reason: 'selected',
  });
});

test('an open fork cannot reserve alerts with canonical links or a title range', async () => {
  const fork = pr(1, {
    title: 'Fix code scanning alerts #1 to #1000',
    body: 'https://github.com/example/repo/security/code-scanning/7',
    head: { ref: 'public-contribution', repo: { full_name: 'someone/fork' } },
  });
  assert.deepEqual(await selectSecurityFinding({ env: baseEnv(), fetchImpl: api({ pulls: [fork], codeAlerts: [code(7)] }).fetchImpl }), {
    hasFinding: true, type: 'code-scanning', number: 7, reason: 'selected',
  });
});

test('two open request branches stop new work; unrelated PRs do not consume capacity', async () => {
  const full = api({ pulls: [request(1, 'code-scanning', 1), request(2, 'dependabot', 2)] });
  assert.deepEqual(await selectSecurityFinding({ env: baseEnv(), fetchImpl: full.fetchImpl }), { hasFinding: false, reason: 'capacity-reached' });
  assert.equal(full.calls.length, 3);
  const available = api({ pulls: [request(1, 'code-scanning', 1), pr(2), pr(3)], dependencies: [dependency(2)] });
  assert.equal((await selectSecurityFinding({ env: baseEnv(), fetchImpl: available.fetchImpl })).number, 2);
});

test('manual dispatch retries a closed request without querying alert lists, but never duplicates an open fix', async () => {
  const env = { ...baseEnv(), GITHUB_EVENT_NAME: 'workflow_dispatch', FINDING_TYPE: 'code-scanning', ALERT_NUMBER: '42' };
  delete env.CODE_SCANNING_TOKEN;
  delete env.SECURITY_ALERTS_TOKEN;
  const closed = api({ pulls: [request(1, 'code-scanning', 42, { state: 'closed' })] });
  assert.deepEqual(await selectSecurityFinding({ env, fetchImpl: closed.fetchImpl }), { hasFinding: true, type: 'code-scanning', number: 42, reason: 'selected' });
  assert.equal(closed.calls.length, 1);
  for (const matching of [request(1, 'code-scanning', 42, { body: '<!-- codex-security-submitted -->' }), pr(2, { title: 'Fix code scanning alerts #40 to #45' }), pr(3, { body: 'https://github.com/example/repo/security/code-scanning/42' })]) {
    const open = api({ pulls: [matching] });
    assert.equal((await selectSecurityFinding({ env, fetchImpl: open.fetchImpl })).reason, 'manual-already-open');
  }
  const capacity = api({ pulls: [request(1, 'dependabot', 1), request(2, 'dependabot', 2)] });
  assert.equal((await selectSecurityFinding({ env, fetchImpl: capacity.fetchImpl })).reason, 'capacity-reached');
});

test('a pending own request resumes before capacity, while submitted, closed, or no-longer-open requests do not resume', async () => {
  const pulls = [request(1, 'code-scanning', 42), request(2, 'dependabot', 6, { body: '<!-- codex-security-submitted -->' })];
  const active = api({ pulls, codeAlerts: [code(42)], dependencies: [dependency(6)] });
  assert.deepEqual(await selectSecurityFinding({ env: baseEnv(), fetchImpl: active.fetchImpl }), {
    hasFinding: true, type: 'code-scanning', number: 42, existingPrNumber: 1, reason: 'resume-pending',
  });
  const manual = { ...baseEnv(), GITHUB_EVENT_NAME: 'workflow_dispatch', FINDING_TYPE: 'code-scanning', ALERT_NUMBER: '42' };
  assert.equal((await selectSecurityFinding({ env: manual, fetchImpl: api({ pulls }).fetchImpl })).existingPrNumber, 1);
  assert.equal((await selectSecurityFinding({ env: baseEnv(), fetchImpl: api({ pulls, dependencies: [dependency(6)] }).fetchImpl })).reason, 'capacity-reached');
  assert.equal((await selectSecurityFinding({ env: baseEnv(), fetchImpl: api({ pulls: [request(1, 'code-scanning', 42, { state: 'closed' })], codeAlerts: [code(42)] }).fetchImpl })).reason, 'already-assigned');
  assert.equal((await selectSecurityFinding({ env: baseEnv(), fetchImpl: api({ pulls: [request(1, 'code-scanning', 42, { body: '<!-- codex-security-submitted -->' })], codeAlerts: [code(42)] }).fetchImpl })).reason, 'already-assigned');
});

test('a standalone marker on a different branch is historical but cannot resume an incompatible request PR', async () => {
  const mock = api({ pulls: [pr(1, { body: '<!-- codex-security-finding: example/repo code-scanning 42 -->' })], codeAlerts: [code(42)] });
  assert.deepEqual(await selectSecurityFinding({ env: baseEnv(), fetchImpl: mock.fetchImpl }), { hasFinding: false, reason: 'already-assigned' });
});

test('a legacy request branch without a canonical marker deduplicates but cannot resume', async () => {
  const pulls = [request(1, 'code-scanning', 42, { body: '' })];
  assert.equal((await selectSecurityFinding({ env: baseEnv(), fetchImpl: api({ pulls, codeAlerts: [code(42)] }).fetchImpl })).reason, 'already-assigned');
  const env = { ...baseEnv(), GITHUB_EVENT_NAME: 'workflow_dispatch', FINDING_TYPE: 'code-scanning', ALERT_NUMBER: '42' };
  assert.equal((await selectSecurityFinding({ env, fetchImpl: api({ pulls }).fetchImpl })).reason, 'manual-already-open');
});

test('a marked legacy branch without numeric run and attempt deduplicates without entering recovery', async () => {
  const legacy = request(1, 'code-scanning', 42, { head: { ref: 'codex/security/code-scanning-42-old-format', repo: { full_name: 'example/repo' } } });
  assert.equal((await selectSecurityFinding({ env: baseEnv(), fetchImpl: api({ pulls: [legacy], codeAlerts: [code(42)] }).fetchImpl })).reason, 'already-assigned');
});

test('manual input validation runs before API calls and cannot steer automatic events', async () => {
  const cases = [
    { GITHUB_EVENT_NAME: 'workflow_dispatch' },
    { GITHUB_EVENT_NAME: 'workflow_dispatch', FINDING_TYPE: 'dependabot', ALERT_NUMBER: '01' },
    { GITHUB_EVENT_NAME: 'workflow_dispatch', FINDING_TYPE: 'secret-scanning', ALERT_NUMBER: '1' },
    { GITHUB_EVENT_NAME: 'workflow_dispatch', FINDING_TYPE: 'code-scanning', ALERT_NUMBER: '1\n' },
    { FINDING_TYPE: 'dependabot', ALERT_NUMBER: '1' },
    { ALERT_NUMBER: '1' },
  ];
  for (const override of cases) await assert.rejects(selectSecurityFinding({ env: { ...baseEnv(), ...override }, fetchImpl: async () => assert.fail('fetch must not run') }));
});

test('locally constructed pagination covers all PR and alert pages despite hostile Link headers', async () => {
  const pulls = Array.from({ length: 100 }, (_, index) => pr(index + 1));
  pulls.push(request(101, 'code-scanning', 1, { state: 'closed' }));
  const alerts = Array.from({ length: 101 }, (_, index) => code(index + 1, index === 100 ? 'critical' : 'high'));
  const mock = api({ pulls, codeAlerts: alerts });
  const seen = [];
  const fetchImpl = async (url, options) => {
    seen.push(url);
    const response = await mock.fetchImpl(url, options);
    if (!url.includes('/dependabot/')) response.headers.set('Link', '<https://attacker.invalid/steal>; rel="next"');
    return response;
  };
  assert.equal((await selectSecurityFinding({ env: baseEnv(), fetchImpl })).number, 101);
  assert.equal(seen.length, 5);
  assert.ok(seen.every((url) => url.startsWith('https://api.github.com/repos/example/repo/')));
  assert.equal(seen.filter((url) => url.includes('page=2')).length, 2);
});

test('incomplete pagination history fails closed rather than selecting or exceeding capacity', async () => {
  let calls = 0;
  await assert.rejects(selectSecurityFinding({ env: baseEnv(), fetchImpl: async (url) => {
    calls++;
    const page = Number(new URL(url).searchParams.get('page'));
    return json(Array.from({ length: 100 }, (_, index) => pr((page - 1) * 100 + index + 1)));
  } }), /pagination limit/);
  assert.equal(calls, 20);
});

test('Dependabot uses validated after cursors and reconstructs each request locally without page', async () => {
  const mock = api();
  const urls = [];
  const result = await selectSecurityFinding({ env: baseEnv(), fetchImpl: async (url, options) => {
    if (!url.includes('/dependabot/')) return mock.fetchImpl(url, options);
    urls.push(url);
    const parsed = new URL(url);
    assert.equal(parsed.searchParams.has('page'), false);
    assert.equal(parsed.searchParams.get('state'), 'open');
    if (urls.length === 1) {
      assert.equal(parsed.searchParams.has('after'), false);
      return json([dependency(1, 'low')], { headers: { Link: '<https://api.github.com/repos/example/repo/dependabot/alerts?state=dismissed&after=cursor%2Btwo&per_page=1>; rel="next"' } });
    }
    assert.equal(parsed.searchParams.get('after'), 'cursor+two');
    assert.equal(parsed.searchParams.get('per_page'), '100');
    return json([dependency(2, 'critical')]);
  } });
  assert.deepEqual(result, { hasFinding: true, type: 'dependabot', number: 2, reason: 'selected' });
  assert.equal(urls.length, 2);
  const exactlyFull = api({ dependencies: Array.from({ length: 100 }, (_, index) => dependency(index + 1)) });
  assert.equal((await selectSecurityFinding({ env: baseEnv(), fetchImpl: exactlyFull.fetchImpl })).number, 1);
  assert.equal(exactlyFull.calls.filter((call) => call.url.includes('/dependabot/')).length, 1);
});

test('Dependabot rejects forged pagination URLs, malformed cursors, and repeated cursors', async () => {
  const headers = [
    '<https://attacker.invalid/repos/example/repo/dependabot/alerts?after=next>; rel="next"',
    '<https://api.github.com/repos/example/other/dependabot/alerts?after=next>; rel="next"',
    '<https://api.github.com/repos/example/repo/pulls?after=next>; rel="next"',
    '<https://api.github.com/repos/example/repo/dependabot/alerts?after=>; rel="next"',
    '<https://api.github.com/repos/example/repo/dependabot/alerts?after=two%0Alines>; rel="next"',
    '<https://api.github.com/repos/example/repo/dependabot/alerts?after=one&after=two>; rel="next"',
    'MALFORMED_PRIVATE_RESPONSE',
  ];
  for (const header of headers) {
    const mock = api();
    await assert.rejects(selectSecurityFinding({ env: baseEnv(), fetchImpl: async (url, options) => url.includes('/dependabot/') ? json([dependency(1)], { headers: { Link: header } }) : mock.fetchImpl(url, options) }), (error) => {
      assert.doesNotMatch(error.message, /MALFORMED_PRIVATE_RESPONSE|attacker\.invalid/); return true;
    });
  }
  let page = 0;
  const mock = api();
  await assert.rejects(selectSecurityFinding({ env: baseEnv(), fetchImpl: async (url, options) => {
    if (!url.includes('/dependabot/')) return mock.fetchImpl(url, options);
    return json([dependency(++page)], { headers: { Link: '<https://api.github.com/repos/example/repo/dependabot/alerts?after=repeat>; rel="next"' } });
  } }), /repeated pagination cursor/);
});

test('duplicate entries across pages fail closed rather than hiding incomplete history', async () => {
  const first = Array.from({ length: 100 }, (_, index) => pr(index + 1));
  await assert.rejects(selectSecurityFinding({ env: baseEnv(), fetchImpl: async (url) => json(new URL(url).searchParams.get('page') === '1' ? first : [pr(1)]) }), /duplicate/);
});

test('closed alert states are filtered and valid non-security severity falls back to rule severity', async () => {
  const closed = api({ codeAlerts: [code(1, 'critical', { state: 'fixed' })], dependencies: [dependency(2, 'critical', { state: 'dismissed' })] });
  assert.equal((await selectSecurityFinding({ env: baseEnv(), fetchImpl: closed.fetchImpl })).reason, 'no-open-alerts');
  const fallback = api({ codeAlerts: [code(1, null)], dependencies: [dependency(2, 'low')] });
  assert.equal((await selectSecurityFinding({ env: baseEnv(), fetchImpl: fallback.fetchImpl })).number, 1);
  const assigned = api({ pulls: [request(1, 'dependabot', 2, { state: 'closed' })], dependencies: [dependency(2)] });
  assert.equal((await selectSecurityFinding({ env: baseEnv(), fetchImpl: assigned.fetchImpl })).reason, 'already-assigned');
});

test('malformed PRs, alert severity/state/identity, and excessive title ranges fail closed', async () => {
  const cases = [
    { pulls: [null] }, { pulls: [{ number: 1 }] }, { pulls: [pr(0)] },
    { pulls: [pr(1, { state: 'deleted' })] }, { pulls: [pr(1, { body: {} })] },
    { pulls: [pr(1, { title: 'Fix code scanning alerts #1 to #1001' })] },
    { pulls: [pr(1, { title: 'Fix code scanning alerts #8 to #1' })] },
    { codeAlerts: [code(0)] }, { codeAlerts: [code(1, 'unknown')] },
    { codeAlerts: [code(1, 'high', { rule: null })] }, { dependencies: [dependency(1, 'high', { state: 'unknown' })] },
    { dependencies: [dependency(1, 'high', { security_advisory: null })] },
  ];
  for (const response of cases) await assert.rejects(selectSecurityFinding({ env: baseEnv(), fetchImpl: api(response).fetchImpl }));
});

test('repository, API root, default branch, and required tokens are validated before fetch', async () => {
  const cases = [
    { GITHUB_REPOSITORY: 'example/../secret' }, { GITHUB_REPOSITORY: 'example/..' },
    { GITHUB_REPOSITORY: 'https://attacker.invalid/repo' }, { GITHUB_API_URL: 'http://api.github.com' },
    { GITHUB_API_URL: 'https://api.github.com/other' }, { GITHUB_API_URL: 'https://user:pass@api.github.com' },
    { GITHUB_API_URL: 'https://api.github.com?token=secret' }, { DEFAULT_BRANCH: 'main\n' },
    { DEFAULT_BRANCH: '' }, { GH_TOKEN: '' }, { CODE_SCANNING_TOKEN: '' }, { SECURITY_ALERTS_TOKEN: '' },
    { GH_TOKEN: 'token\nsecret' },
  ];
  for (const override of cases) await assert.rejects(selectSecurityFinding({ env: { ...baseEnv(), ...override }, fetchImpl: async () => assert.fail('fetch must not run') }));
});

test('API permission, redirect, and network failures never expose tokens or response bodies', async () => {
  for (const status of [301, 401, 403, 404, 429, 500]) {
    await assert.rejects(selectSecurityFinding({ env: baseEnv(), fetchImpl: async () => new Response('prs-private-token SENSITIVE_RESPONSE', { status }) }), (error) => {
      assert.match(error.message, new RegExp(String(status)));
      assert.doesNotMatch(error.message, /private-token|SENSITIVE_RESPONSE/);
      return true;
    });
  }
  await assert.rejects(selectSecurityFinding({ env: baseEnv(), fetchImpl: async () => { throw new Error('prs-private-token SENSITIVE_RESPONSE'); } }), (error) => {
    assert.doesNotMatch(error.message, /private-token|SENSITIVE_RESPONSE/); return true;
  });
  await assert.rejects(selectSecurityFinding({ env: baseEnv(), fetchImpl: async () => ({ status: 200, redirected: true }) }), /redirected/);
});

test('a Dependabot permission error fails the whole automatic selection instead of silently skipping it', async () => {
  const mock = api({ codeAlerts: [code(1)] });
  await assert.rejects(selectSecurityFinding({ env: baseEnv(), fetchImpl: async (url, options) => url.includes('/dependabot/') ? new Response('private-token', { status: 403 }) : mock.fetchImpl(url, options) }), /403/);
});

test('malformed, oversized, and excessive list responses are rejected without exposing their content', async () => {
  for (const response of [
    new Response('SENSITIVE_JSON', { status: 200 }), json({}), json(Array.from({ length: 101 }, (_, i) => pr(i + 1))),
    new Response('X'.repeat(2 * 1024 * 1024 + 1), { status: 200 }),
    new Response('[]', { status: 200, headers: { 'content-length': String(2 * 1024 * 1024 + 1) } }),
  ]) {
    await assert.rejects(selectSecurityFinding({ env: baseEnv(), fetchImpl: async () => response }), (error) => {
      assert.doesNotMatch(error.message, /SENSITIVE_JSON/); return true;
    });
  }
});

test('GitHub outputs contain only validated selection and static reasons, including no-selection outputs', async (t) => {
  const directory = await mkdtemp(join(tmpdir(), 'security-queue-test-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const env = { ...baseEnv(), GITHUB_OUTPUT: join(directory, 'selected') };
  await selectSecurityFinding({ env, fetchImpl: api({ dependencies: [dependency(12)] }).fetchImpl });
  assert.equal(await readFile(env.GITHUB_OUTPUT, 'utf8'), 'has_finding=true\nfinding_type=dependabot\nalert_number=12\nexisting_pr_number=\nreason=selected\n');
  env.GITHUB_OUTPUT = join(directory, 'empty');
  await selectSecurityFinding({ env, fetchImpl: api().fetchImpl });
  assert.equal(await readFile(env.GITHUB_OUTPUT, 'utf8'), 'has_finding=false\nfinding_type=\nalert_number=\nexisting_pr_number=\nreason=no-open-alerts\n');
});
