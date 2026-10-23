// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, rm, stat, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { normalizeAlert, prepareAlert, validateSelection } from './security-alert.mjs';

const codeAlert = () => ({
  number: 42,
  state: 'open',
  html_url: 'https://attacker.invalid/steal',
  secret: 'RAW_SECRET_MUST_NOT_LEAK',
  rule: { id: 'js/sql-injection', description: 'SQL injection\u0000\r\n', severity: 'error', security_severity_level: 'high', full_description: 'RAW_SECRET_MUST_NOT_LEAK' },
  tool: { name: 'CodeQL', version: '2.20' },
  most_recent_instance: {
    ref: 'refs/heads/main', state: 'open',
    message: { text: 'Untrusted input reaches a query.\u001b' },
    location: { path: 'services/example.js', start_line: 9, end_line: 10 },
    extra: 'RAW_SECRET_MUST_NOT_LEAK',
  },
});

const dependabotAlert = () => ({
  number: 42, state: 'open', secret: 'RAW_SECRET_MUST_NOT_LEAK',
  security_advisory: { ghsa_id: 'GHSA-aaaa-bbbb-cccc', cve_id: 'CVE-2026-1234', severity: 'high', summary: 'Upgrade the package.' },
  dependency: { package: { name: 'example', ecosystem: 'npm' }, manifest_path: 'package-lock.json', scope: 'runtime' },
  security_vulnerability: { vulnerable_version_range: '< 2.0.0', first_patched_version: { identifier: '2.0.0' } },
});

async function fixture(t, type = 'code-scanning') {
  const directory = await mkdtemp(join(tmpdir(), 'security-alert-test-'));
  const workspace = join(directory, 'checkout');
  await mkdir(workspace);
  t.after(() => rm(directory, { recursive: true, force: true }));
  return {
    directory,
    workspace,
    env: {
      FINDING_TYPE: type, ALERT_NUMBER: '42', GITHUB_REPOSITORY: 'example/repo',
      DEFAULT_BRANCH: 'main', GITHUB_API_URL: 'https://api.github.com',
      SECURITY_ALERTS_TOKEN: 'test-token-never-log', GITHUB_WORKSPACE: workspace,
      ALERT_CONTEXT_PATH: join(directory, 'context.json'), GITHUB_OUTPUT: join(directory, 'outputs'),
    },
  };
}

test('selection accepts only the two supported alert types and strict positive integers', () => {
  assert.deepEqual(validateSelection('code-scanning', '42'), { type: 'code-scanning', number: 42 });
  assert.deepEqual(validateSelection('dependabot', 1), { type: 'dependabot', number: 1 });
  for (const type of ['secret-scanning', '../dependabot', 'code-scanning\n', '', undefined]) {
    assert.throws(() => validateSelection(type, '42'));
  }
  for (const number of ['0', '-1', '01', '1.1', '1e2', ' 1', '1\n', '1/../../secrets', '1?x=1', '9007199254740992', undefined]) {
    assert.throws(() => validateSelection('code-scanning', number));
  }
});

test('normalization rejects closed, mismatched, or malformed alerts', () => {
  assert.throws(() => normalizeAlert('code-scanning', 42, { ...codeAlert(), state: 'dismissed' }, 'main'), /not open/);
  assert.throws(() => normalizeAlert('dependabot', 42, { ...dependabotAlert(), state: 'fixed' }, 'main'), /not open/);
  assert.throws(() => normalizeAlert('code-scanning', 43, codeAlert(), 'main'), /unexpected alert number/);
  assert.throws(() => normalizeAlert('code-scanning', 42, null, 'main'), /unexpected alert number/);
  assert.throws(() => normalizeAlert('code-scanning', 42, { ...codeAlert(), rule: null }, 'main'), /malformed/);
  assert.throws(() => normalizeAlert('dependabot', 42, { ...dependabotAlert(), dependency: null }, 'main'), /malformed/);
});

test('code-scanning requires an open instance on the default branch', () => {
  for (const instance of [null, { ref: 'refs/heads/feature', state: 'open' }, { ref: 'refs/heads/main', state: 'fixed' }]) {
    assert.throws(() => normalizeAlert('code-scanning', 42, { ...codeAlert(), most_recent_instance: instance }, 'main'), /default branch/);
  }
});

test('normalization allowlists fields, removes control characters, and limits text', () => {
  const alert = codeAlert();
  alert.most_recent_instance.message.text = `\u202e${'A'.repeat(9000)}`;
  const normalized = normalizeAlert('code-scanning', 42, alert, 'main');
  const json = JSON.stringify(normalized);
  assert.equal(json.includes('RAW_SECRET_MUST_NOT_LEAK'), false);
  assert.equal(json.includes('attacker.invalid'), false);
  assert.equal(normalized.data.rule.description, 'SQL injection   ');
  assert.equal(normalized.data.message.length, 5000);
  assert.equal(/[\u0000-\u001f\u007f-\u009f\u202a-\u202e]/u.test(normalized.data.message), false);
  assert.equal(normalized.data.location.startLine, 9);
  const dependency = normalizeAlert('dependabot', 42, dependabotAlert(), 'main');
  assert.equal(dependency.data.dependency.firstPatchedVersion, '2.0.0');
  assert.equal(JSON.stringify(dependency).includes('RAW_SECRET_MUST_NOT_LEAK'), false);
});

test('prepareAlert fetches one selected endpoint and writes private, projected context outside checkout', async (t) => {
  const { env } = await fixture(t);
  let calls = 0;
  const result = await prepareAlert({ env, fetchImpl: async (url, options) => {
    calls++;
    assert.equal(url, 'https://api.github.com/repos/example/repo/code-scanning/alerts/42');
    assert.equal(options.headers.Authorization, 'Bearer test-token-never-log');
    assert.equal(options.redirect, 'error');
    return new Response(JSON.stringify(codeAlert()), { status: 200 });
  } });
  assert.equal(calls, 1);
  assert.equal(result.findingUrl, 'https://github.com/example/repo/security/code-scanning/42');
  const context = JSON.parse(await readFile(result.contextPath, 'utf8'));
  assert.equal(context.url, result.findingUrl);
  assert.equal(context.type, 'code-scanning');
  assert.equal(context.number, 42);
  assert.equal(context.repository, 'example/repo');
  assert.equal(JSON.stringify(context).includes('RAW_SECRET_MUST_NOT_LEAK'), false);
  assert.equal((await stat(result.contextPath)).mode & 0o777, 0o600);
  assert.equal(await readFile(env.GITHUB_OUTPUT, 'utf8'), `alert_context_path=${result.contextPath}\nfinding_url=${result.findingUrl}\n`);
});

test('Dependabot context includes only selected dependency and advisory metadata', async (t) => {
  const { env } = await fixture(t, 'dependabot');
  delete env.GITHUB_OUTPUT;
  const result = await prepareAlert({ env, fetchImpl: async (url) => {
    assert.equal(url, 'https://api.github.com/repos/example/repo/dependabot/alerts/42');
    return new Response(JSON.stringify(dependabotAlert()), { status: 200 });
  } });
  const context = JSON.parse(await readFile(result.contextPath, 'utf8'));
  assert.equal(context.url, 'https://github.com/example/repo/security/dependabot/42');
  assert.equal(context.data.advisory.ghsaId, 'GHSA-aaaa-bbbb-cccc');
  assert.equal(context.data.dependency.vulnerableVersionRange, '< 2.0.0');
});

test('a recent PR instance resolves to an open default-branch instance using a locally constructed URL', async (t) => {
  const { env } = await fixture(t);
  const alert = codeAlert();
  alert.instances_url = 'https://attacker.invalid/steal-token';
  alert.most_recent_instance.ref = 'refs/pull/17/merge';
  alert.most_recent_instance.location.path = 'wrong-pr-file.js';
  const defaultInstance = codeAlert().most_recent_instance;
  let calls = 0;
  const result = await prepareAlert({ env, fetchImpl: async (url, options) => {
    calls++;
    assert.equal(options.redirect, 'error');
    if (calls === 1) {
      assert.equal(url, 'https://api.github.com/repos/example/repo/code-scanning/alerts/42');
      return new Response(JSON.stringify(alert), { status: 200 });
    }
    assert.equal(url, 'https://api.github.com/repos/example/repo/code-scanning/alerts/42/instances?ref=refs%2Fheads%2Fmain&per_page=100&page=1');
    return new Response(JSON.stringify([
      { ...defaultInstance, state: 'fixed' },
      { ...defaultInstance, ref: 'refs/pull/17/merge', location: { path: 'wrong-pr-file.js' } },
      defaultInstance,
    ]), { status: 200 });
  } });
  assert.equal(calls, 2);
  const context = JSON.parse(await readFile(result.contextPath, 'utf8'));
  assert.equal(context.data.location.path, 'services/example.js');
  assert.equal(JSON.stringify(context).includes('wrong-pr-file.js'), false);
  assert.equal(JSON.stringify(context).includes('attacker.invalid'), false);
});

test('instance lookup rejects closed or mismatched alerts before the second API call', async (t) => {
  const { env } = await fixture(t);
  for (const override of [{ state: 'fixed' }, { number: 43 }]) {
    let calls = 0;
    await assert.rejects(prepareAlert({ env, fetchImpl: async () => {
      calls++;
      assert.equal(calls, 1);
      return new Response(JSON.stringify({ ...codeAlert(), ...override, most_recent_instance: null }), { status: 200 });
    } }));
    assert.equal(calls, 1);
  }
});

test('instance lookup rejects missing default-branch open instances and malformed responses', async (t) => {
  const { env } = await fixture(t);
  for (const response of [[], [{ ref: 'refs/heads/main', state: 'fixed' }], [{ ref: 'refs/heads/feature', state: 'open' }], {}]) {
    let calls = 0;
    await assert.rejects(prepareAlert({ env, fetchImpl: async () => {
      calls++;
      return new Response(JSON.stringify(calls === 1 ? { ...codeAlert(), most_recent_instance: null } : response), { status: 200 });
    } }), /default branch|malformed/);
    assert.equal(calls, 2);
  }
});

test('instance lookup constructs bounded pagination without trusting API link headers', async (t) => {
  const { env } = await fixture(t);
  let calls = 0;
  const result = await prepareAlert({ env, fetchImpl: async (url) => {
    calls++;
    if (calls === 1) return new Response(JSON.stringify({ ...codeAlert(), most_recent_instance: null }), { status: 200 });
    if (calls === 2) {
      assert.match(url, /&page=1$/);
      return new Response(JSON.stringify(Array.from({ length: 100 }, () => ({ ref: 'refs/heads/main', state: 'fixed' }))), {
        status: 200, headers: { link: '<https://attacker.invalid/steal-token>; rel="next"' },
      });
    }
    assert.match(url, /^https:\/\/api\.github\.com\/repos\/example\/repo\/code-scanning\/alerts\/42\/instances\?ref=refs%2Fheads%2Fmain&per_page=100&page=2$/);
    return new Response(JSON.stringify([codeAlert().most_recent_instance]), { status: 200 });
  } });
  assert.equal(calls, 3);
  assert.equal(JSON.parse(await readFile(result.contextPath, 'utf8')).data.location.path, 'services/example.js');
});

test('invalid repository, API root, token, and output paths fail before fetch', async (t) => {
  const { env, workspace } = await fixture(t);
  const cases = [
    { GITHUB_REPOSITORY: 'example/../secrets' }, { GITHUB_REPOSITORY: 'example/..' },
    { GITHUB_REPOSITORY: 'https://attacker.invalid/a' }, { GITHUB_REPOSITORY: 'example/repo?x=1' },
    { GITHUB_API_URL: 'http://api.github.com' }, { GITHUB_API_URL: 'https://api.github.com/other' },
    { GITHUB_API_URL: 'https://user:pass@api.github.com' }, { GITHUB_API_URL: 'https://api.github.com?x=1' },
    { SECURITY_ALERTS_TOKEN: '' }, { SECURITY_ALERTS_TOKEN: 'token\nsecret' },
    { DEFAULT_BRANCH: '' }, { ALERT_CONTEXT_PATH: 'relative.json' },
    { ALERT_CONTEXT_PATH: join(workspace, 'context.json') },
  ];
  for (const values of cases) {
    await assert.rejects(prepareAlert({ env: { ...env, ...values }, fetchImpl: async () => assert.fail('fetch must not run') }));
  }
});

test('output path cannot escape the checkout restriction through a symlink', async (t) => {
  const { env, directory, workspace } = await fixture(t);
  const link = join(directory, 'checkout-link');
  await symlink(workspace, link);
  await assert.rejects(prepareAlert({ env: { ...env, ALERT_CONTEXT_PATH: join(link, 'context.json') }, fetchImpl: async () => assert.fail('fetch must not run') }), /outside/);
});

test('existing context files are not overwritten', async (t) => {
  const { env } = await fixture(t);
  const options = { env, fetchImpl: async () => new Response(JSON.stringify(codeAlert()), { status: 200 }) };
  await prepareAlert(options);
  await assert.rejects(prepareAlert(options), { code: 'EEXIST' });
});

test('API and network errors never expose response bodies, tokens, or raw failures', async (t) => {
  const { env } = await fixture(t);
  for (const status of [401, 403, 404, 429, 500]) {
    await assert.rejects(prepareAlert({ env, fetchImpl: async () => new Response('SENSITIVE_RESPONSE_BODY test-token-never-log', { status }) }), (error) => {
      assert.match(error.message, new RegExp(String(status)));
      assert.doesNotMatch(error.message, /SENSITIVE_RESPONSE_BODY|test-token-never-log/);
      return true;
    });
  }
  await assert.rejects(prepareAlert({ env, fetchImpl: async () => { throw new Error('test-token-never-log'); } }), (error) => {
    assert.doesNotMatch(error.message, /test-token-never-log/);
    return true;
  });
});

test('malformed and oversized responses fail without exposing raw response text', async (t) => {
  const { env } = await fixture(t);
  for (const response of [
    new Response('SENSITIVE_INVALID_JSON', { status: 200 }),
    new Response('{}', { status: 200 }),
    new Response('A'.repeat(1024 * 1024 + 1), { status: 200 }),
    new Response('{}', { status: 200, headers: { 'content-length': String(1024 * 1024 + 1) } }),
  ]) {
    await assert.rejects(prepareAlert({ env, fetchImpl: async () => response }), (error) => {
      assert.doesNotMatch(error.message, /SENSITIVE_INVALID_JSON/);
      return true;
    });
  }
});
