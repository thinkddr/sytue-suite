// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import { runInNewContext } from 'node:vm';

const workflow = await readFile(new URL('../workflows/codex-security-fix.yml', import.meta.url), 'utf8');
const lines = workflow.split(/\r?\n/u);
const jobsIndex = lines.indexOf('jobs:');
const selectIndex = lines.indexOf('  select:', jobsIndex + 1);
assert.ok(jobsIndex >= 0 && selectIndex > jobsIndex, 'The workflow must define jobs.select.');
const nextJobIndex = lines.findIndex((line, index) => index > selectIndex && /^  [A-Za-z_][\w-]*:/u.test(line));
const selectLines = lines.slice(selectIndex + 1, nextJobIndex < 0 ? undefined : nextJobIndex);
const conditionIndex = selectLines.indexOf('    if: >-');
assert.ok(conditionIndex >= 0, 'jobs.select.if must remain an explicit folded condition.');
const conditionLines = [];
for (const line of selectLines.slice(conditionIndex + 1)) {
  if (!line.startsWith('      ')) break;
  conditionLines.push(line.trim());
}
assert.ok(conditionLines.length > 0, 'jobs.select.if must contain an expression.');
const condition = conditionLines.join(' ');

function githubContext({
  eventName = 'workflow_run',
  defaultBranch = 'main',
  ref = `refs/heads/${defaultBranch}`,
  headBranch = defaultBranch,
  headRepository = 'example/repo',
  sourceEvent = 'push',
  conclusion = 'success',
} = {}) {
  const event = { repository: { default_branch: defaultBranch } };
  if (eventName === 'workflow_run') {
    event.workflow_run = {
      conclusion,
      head_branch: headBranch,
      head_repository: { full_name: headRepository },
      event: sourceEvent,
    };
  }
  return { repository: 'example/repo', ref, event_name: eventName, event };
}

function shouldSelect(overrides) {
  // Evaluate only the trusted, checked-in workflow expression with fixture data.
  return runInNewContext(`(${condition})`, {
    github: githubContext(overrides),
    format(template, value) {
      assert.equal(template, 'refs/heads/{0}');
      return template.replace('{0}', value);
    },
    fromJSON: JSON.parse,
    contains: (values, value) => values.includes(value),
  }, { timeout: 100 });
}

for (const conclusion of ['success', 'failure', 'cancelled']) {
  test(`trusted default-branch CodeQL completions run after ${conclusion}`, () => {
    for (const sourceEvent of ['push', 'schedule', 'workflow_dispatch']) {
      assert.equal(shouldSelect({ conclusion, sourceEvent }), true, sourceEvent);
    }
  });
}

test('a workflow from a fork cannot select work even when its branch name matches', () => {
  for (const conclusion of ['success', 'failure', 'cancelled']) {
    assert.equal(shouldSelect({ conclusion, headRepository: 'someone/fork' }), false);
  }
});

test('pull request and pull_request_target workflow sources cannot select work', () => {
  for (const sourceEvent of ['pull_request', 'pull_request_target']) {
    assert.equal(shouldSelect({ sourceEvent }), false, sourceEvent);
  }
});

test('a non-default source branch cannot select work', () => {
  assert.equal(shouldSelect({ headBranch: 'feature/security' }), false);
});

test('own push, schedule, and manual events run on the default ref without workflow_run metadata', () => {
  for (const eventName of ['push', 'schedule', 'workflow_dispatch']) {
    assert.equal(shouldSelect({ eventName }), true, eventName);
  }
});

test('every supported event rejects non-default execution refs', () => {
  for (const eventName of ['push', 'schedule', 'workflow_dispatch', 'workflow_run']) {
    for (const ref of ['refs/heads/feature/security', 'refs/pull/123/merge']) {
      assert.equal(shouldSelect({ eventName, ref }), false, `${eventName}: ${ref}`);
    }
  }
});

test('the trusted branch follows the configured repository default', () => {
  assert.equal(shouldSelect({ defaultBranch: 'release/production' }), true);
  assert.equal(shouldSelect({ defaultBranch: 'release/production', ref: 'refs/heads/main' }), false);
  assert.equal(shouldSelect({ defaultBranch: 'release/production', headBranch: 'main' }), false);
});

test('selection and preparation use the same verified Dependabot credential', () => {
  const expressions = [...workflow.matchAll(/^\s+SECURITY_ALERTS_TOKEN: \$\{\{ (.+) \}\}$/gm)].map(m => m[1]);
  assert.equal(expressions.length, 2);
  const secrets = { SECURITY_ALERTS_TOKEN: 'dedicated-test-token', CODEX_PR_TOKEN: 'connector-test-token' };
  for (const credential of ['primary', 'connector']) {
    const selected = new Function('steps', 'secrets', `return ${expressions[0]}`)({ alert_auth: { outputs: { credential } } }, secrets);
    const prepared = new Function('needs', 'secrets', 'github', `return ${expressions[1]}`)({ select: { outputs: { finding_type: 'dependabot', dependabot_credential: credential } } }, secrets, { token: 'job-test-token' });
    assert.equal(selected, credential === 'connector' ? secrets.CODEX_PR_TOKEN : secrets.SECURITY_ALERTS_TOKEN);
    assert.equal(prepared, selected);
  }
  const codeScanning = new Function('needs', 'secrets', 'github', `return ${expressions[1]}`)({ select: { outputs: { finding_type: 'code-scanning', dependabot_credential: '' } } }, {}, { token: 'job-test-token' });
  assert.equal(codeScanning, 'job-test-token');
});

test('manual Code Scanning selection does not require Dependabot credentials', () => {
  assert.match(workflow, /id: alert_auth\n\s+if: inputs\.finding_type != 'code-scanning'/);
  assert.doesNotMatch(workflow, /ALERT_TOKEN_CONFIGURED/);
});
