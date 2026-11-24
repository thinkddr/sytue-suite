// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, readdir, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { publishSecurityRequest } from './security-publish.mjs';
import { renderRequest } from './security-request.mjs';

async function fixture(t, { existing = '71', comments = [], body, freshBody, failure, prChanges = {} } = {}) {
  const directory = await mkdtemp(join(tmpdir(), 'security-publish-test-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const workspace = join(directory, 'checkout');
  const requestDirectory = join(directory, 'request');
  await mkdir(workspace);
  await mkdir(requestDirectory);
  const rendered = renderRequest({ type: 'dependabot', number: 42, repository: 'example/repo', url: 'https://github.com/example/repo/security/dependabot/42', data: {} }, { baseSha: 'a'.repeat(40) });
  await Promise.all([['request.md', rendered.requestBody], ['pr-body.md', rendered.prBody], ['codex-comment.md', rendered.codexComment]].map(([name, content]) => writeFile(join(requestDirectory, name), content)));
  const env = {
    GH_TOKEN: 'test-private-token', GITHUB_REPOSITORY: 'example/repo', DEFAULT_BRANCH: 'main', FINDING_TYPE: 'dependabot', ALERT_NUMBER: '42',
    GITHUB_RUN_ID: '1234', GITHUB_RUN_ATTEMPT: '2', GITHUB_WORKSPACE: workspace,
    REQUEST_PATH: rendered.requestPath, PR_TITLE: rendered.prTitle, SECURITY_REQUEST_DIR: requestDirectory,
    EXISTING_PR_NUMBER: existing, GITHUB_OUTPUT: join(directory, 'output'), GITHUB_STEP_SUMMARY: join(directory, 'summary'),
  };
  const calls = [];
  let views = 0;
  let editedBody;
  const runCommand = async (binary, args, options) => {
    calls.push({ binary, args, options });
    const operation = [binary, ...args.slice(0, 2)].join(' ');
    if (operation === failure) throw new Error('SECRET-EXTERNAL-ERROR test-private-token');
    if (binary === 'git' || args[0] === 'auth') return '';
    if (args[0] === 'api' && args[1] === 'user') return 'connected-user\n';
    if (args[0] === 'api' && args.includes('--paginate')) return JSON.stringify([comments]);
    if (args[0] === 'pr' && args[1] === 'create') return 'https://github.com/example/repo/pull/71\n';
    if (args[0] === 'pr' && args[1] === 'view') {
      views++;
      return JSON.stringify({
        headRefName: existing ? 'codex/security/dependabot-42-100-1' : 'codex/security/dependabot-42-1234-2',
        headRepositoryOwner: { login: 'example' }, headRepository: { name: 'repo', nameWithOwner: 'example/repo' },
        isCrossRepository: false, baseRefName: 'main', state: 'OPEN', body: (views > 1 ? freshBody : undefined) ?? body ?? rendered.prBody,
        ...prChanges,
      });
    }
    if (args[0] === 'pr' && args[1] === 'comment') return 'https://github.com/example/repo/pull/71#issuecomment-9001\n';
    if (args[0] === 'pr' && args[1] === 'edit') {
      editedBody = await readFile(args[args.indexOf('--body-file') + 1], 'utf8');
      return 'https://github.com/example/repo/pull/71\n';
    }
    assert.fail(`Unexpected command ${operation}`);
  };
  return { directory, workspace, requestDirectory, rendered, env, calls, runCommand, editedBody: () => editedBody };
}

test('new finding commits only the controlled request file, opens a draft and marks confirmed handoff', async (t) => {
  const f = await fixture(t, { existing: '' });
  const result = await publishSecurityRequest(f);
  assert.equal(result.pr_url, 'https://github.com/example/repo/pull/71');
  assert.equal(result.pr_number, '71');
  assert.equal(await readFile(join(f.workspace, f.rendered.requestPath), 'utf8'), f.rendered.requestBody);
  const commit = f.calls.find(({ binary, args }) => binary === 'git' && args.includes('commit'));
  assert.ok(commit.args.includes('core.hooksPath=/dev/null'));
  assert.deepEqual(commit.args.slice(-3), ['--only', '--', f.rendered.requestPath]);
  const push = f.calls.find(({ binary, args }) => binary === 'git' && args.includes('push'));
  assert.ok(push.args.includes('HEAD:refs/heads/codex/security/dependabot-42-1234-2'));
  const create = f.calls.find(({ args }) => args[0] === 'pr' && args[1] === 'create');
  assert.ok(create.args.includes('--draft'));
  assert.match(f.editedBody(), /<!-- codex-security-submitted -->\n$/);
  assert.equal(await readFile(f.env.GITHUB_OUTPUT, 'utf8'), 'pr_url=https://github.com/example/repo/pull/71\npr_number=71\n');
  assert.match(await readFile(f.env.GITHUB_STEP_SUMMARY, 'utf8'), /PR #71/);
  assert.ok(f.calls.every(({ args }) => !args.some((arg) => arg.includes(f.env.GH_TOKEN))));
  assert.deepEqual((await readdir(f.requestDirectory)).sort(), ['codex-comment.md', 'pr-body.md', 'request.md']);
});

test('pending request resumes without git, push or PR creation and retains fresh human edits', async (t) => {
  const original = '<!-- codex-security-finding: example/repo dependabot 42 -->\n\nOriginal body.';
  const freshBody = `${original}\n\nHuman review added after the mention.`;
  const f = await fixture(t, { body: original, freshBody });
  await publishSecurityRequest(f);
  assert.ok(f.calls.every(({ binary, args }) => binary !== 'git' && args[0] !== 'auth' && args[1] !== 'create'));
  assert.equal(f.editedBody(), `${freshBody}\n\n<!-- codex-security-submitted -->\n`);
  assert.deepEqual(await readdir(f.workspace), []);
});

test('reuses a normalized identical comment only when authored by the token account', async (t) => {
  const f = await fixture(t);
  const comments = [{ body: ` \n${f.rendered.codexComment.replace(/\n/g, '\r\n')}\n`, user: { login: 'Connected-User' } }];
  const originalRunner = f.runCommand;
  f.runCommand = (binary, args, options) => args[0] === 'api' && args.includes('--paginate') ? originalRunner(binary, args, options).then(() => JSON.stringify([comments])) : originalRunner(binary, args, options);
  const result = await publishSecurityRequest(f);
  assert.equal(result.reusedComment, true);
  assert.ok(!f.calls.some(({ args }) => args[0] === 'pr' && args[1] === 'comment'));
  assert.match(f.editedBody(), /codex-security-submitted/);
});

test('an identical comment from another account does not count as the connected handoff', async (t) => {
  const f = await fixture(t);
  const originalRunner = f.runCommand;
  f.runCommand = (binary, args, options) => args[0] === 'api' && args.includes('--paginate') ? originalRunner(binary, args, options).then(() => JSON.stringify([[{ body: f.rendered.codexComment, user: { login: 'someone-else' } }]])) : originalRunner(binary, args, options);
  await publishSecurityRequest(f);
  assert.equal(f.calls.filter(({ args }) => args[0] === 'pr' && args[1] === 'comment').length, 1);
});

test('already submitted request remains unchanged and never repeats the mention', async (t) => {
  const f = await fixture(t, { body: '<!-- codex-security-finding: example/repo dependabot 42 -->\n<!-- codex-security-submitted -->\n' });
  await publishSecurityRequest(f);
  assert.equal(f.calls.length, 1);
  assert.equal(f.editedBody(), undefined);
});

test('comment failure leaves no submitted marker or successful outputs and hides external error text', async (t) => {
  const f = await fixture(t, { failure: 'gh pr comment' });
  await assert.rejects(publishSecurityRequest(f), (error) => /send the Codex mention/.test(error.message) && !/SECRET|test-private-token/.test(error.message));
  assert.equal(f.editedBody(), undefined);
  assert.ok(!f.calls.some(({ args }) => args[0] === 'pr' && args[1] === 'edit'));
  await assert.rejects(readFile(f.env.GITHUB_OUTPUT), { code: 'ENOENT' });
});

test('mark failure remains resumable and emits no success outputs', async (t) => {
  const f = await fixture(t, { failure: 'gh pr edit' });
  await assert.rejects(publishSecurityRequest(f), /mark the confirmed Codex handoff/);
  await assert.rejects(readFile(f.env.GITHUB_OUTPUT), { code: 'ENOENT' });
  assert.deepEqual((await readdir(f.requestDirectory)).sort(), ['codex-comment.md', 'pr-body.md', 'request.md']);
});

test('rejects mismatched existing PR identities before posting or making git changes', async (t) => {
  const changes = [
    { headRepositoryOwner: { login: 'attacker' } }, { headRepository: { name: 'elsewhere' } },
    { headRepository: { name: 'repo', nameWithOwner: 'attacker/repo' } },
    { isCrossRepository: true }, { baseRefName: 'untrusted-branch' }, { state: 'CLOSED' },
    { body: '<!-- codex-security-finding: example/repo dependabot 43 -->' },
    { headRefName: 'codex/security/dependabot-420-100-1' }, { headRefName: 'attacker-branch' },
  ];
  for (const prChanges of changes) {
    const f = await fixture(t, { prChanges });
    await assert.rejects(publishSecurityRequest(f), /PR identity|PR repository/);
    assert.equal(f.calls.length, 1);
  }
});

test('rejects unsafe configuration, paths and unexpected GitHub response URLs', async (t) => {
  const f = await fixture(t, { existing: '' });
  for (const changes of [
    { GH_TOKEN: 'secret\nvalue' }, { GITHUB_REPOSITORY: 'owner/..' }, { ALERT_NUMBER: '042' },
    { REQUEST_PATH: '../../escape' }, { PR_TITLE: 'Other title' }, { GITHUB_RUN_ID: '0' },
    { EXISTING_PR_NUMBER: '7\n8' }, { DEFAULT_BRANCH: '--main' }, { DEFAULT_BRANCH: 'main..other' },
    { SECURITY_REQUEST_DIR: f.workspace }, { GITHUB_OUTPUT: 'relative' },
  ]) await assert.rejects(publishSecurityRequest({ ...f, env: { ...f.env, ...changes } }));
  assert.equal(f.calls.length, 0);
  const originalRunner = f.runCommand;
  f.runCommand = async (binary, args, options) => args[0] === 'pr' && args[1] === 'create' ? 'https://attacker.invalid/example/repo/pull/71' : originalRunner(binary, args, options);
  await assert.rejects(publishSecurityRequest(f), /unexpected pull request URL/);
  assert.ok(!f.calls.some(({ args }) => args[1] === 'comment'));
});

test('rejects artifact and destination symlinks without writing outside the checkout', async (t) => {
  const f = await fixture(t, { existing: '' });
  const outside = join(f.directory, 'outside');
  await mkdir(outside);
  await symlink(outside, join(f.workspace, 'docs'));
  await assert.rejects(publishSecurityRequest(f), /real directories/);
  assert.deepEqual(await readdir(outside), []);
  assert.equal(f.calls.length, 0);
  await rm(join(f.workspace, 'docs'));
  await rm(join(f.requestDirectory, 'request.md'));
  await symlink(join(f.requestDirectory, 'pr-body.md'), join(f.requestDirectory, 'request.md'));
  await assert.rejects(publishSecurityRequest(f));
  assert.equal(f.calls.length, 0);
});
