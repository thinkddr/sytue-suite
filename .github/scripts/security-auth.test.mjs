// SPDX-License-Identifier: Apache-2.0
import test from 'node:test';
import assert from 'node:assert/strict';
import { selectDependabotCredential } from './security-auth.mjs';
const env = { GITHUB_REPOSITORY: 'thinkddr/sytue-suite', ALERT_PRIMARY_TOKEN: 'primary-test-only', ALERT_CONNECTOR_TOKEN: 'connector-test-only' };
function fixture(statuses) {
  const calls = [];
  return { calls, fetchImpl: async (url, options) => { calls.push({url, ...options}); assert.equal(url, 'https://api.github.com/repos/thinkddr/sytue-suite/dependabot/alerts?state=open&per_page=1'); assert.equal(options.method, 'GET'); assert.equal(options.redirect, 'error'); return Response.json([], { status: statuses.shift() }); } };
}
test('primary read access succeeds without exposing the connector credential', async () => {
  const api=fixture([200]); assert.equal(await selectDependabotCredential({env,...api}), 'primary'); assert.equal(api.calls.length,1); assert.equal(api.calls[0].headers.Authorization, `Bearer ${env.ALERT_PRIMARY_TOKEN}`);
});
for (const status of [401,403]) test(`uses the existing connector only after primary authorization fails (${status})`, async () => {
  const api=fixture([status,200]); assert.equal(await selectDependabotCredential({env,...api}), 'connector'); assert.equal(api.calls.length,2); assert.equal(api.calls[1].headers.Authorization, `Bearer ${env.ALERT_CONNECTOR_TOKEN}`);
});
test('missing primary can use an authorized connector', async () => {
  const api=fixture([200]); assert.equal(await selectDependabotCredential({env:{...env,ALERT_PRIMARY_TOKEN:''},...api}), 'connector'); assert.equal(api.calls.length,1);
});
test('denied credentials fail explicitly and do not become an empty inventory', async () => {
  const api=fixture([403,403]); await assert.rejects(selectDependabotCredential({env,...api}), /Grant Dependabot alerts: read/); assert.equal(api.calls.length,2);
});
for (const status of [404,500]) test(`does not retry unrelated failures (${status}) with a broader credential`, async () => {
  const api=fixture([status]); await assert.rejects(selectDependabotCredential({env,...api}), /credentials were not retried/); assert.equal(api.calls.length,1);
});
test('rejects untrusted origins, invalid repositories and malformed tokens before any request', async () => {
  for (const change of [{GITHUB_API_URL:'https://attacker.invalid'},{GITHUB_REPOSITORY:'owner/..'},{GITHUB_REPOSITORY:'../repo'},{ALERT_PRIMARY_TOKEN:'token\nheader'},{ALERT_PRIMARY_TOKEN:'',ALERT_CONNECTOR_TOKEN:''}]) await assert.rejects(selectDependabotCredential({env:{...env,...change},fetchImpl:()=>assert.fail()}));
});
test('network failures and redirects never expose the alternate credential', async () => {
  const api=fixture([]); let calls=0;
  await assert.rejects(selectDependabotCredential({env,fetchImpl:async()=>{calls++;throw new Error('sensitive network detail');}}),/could not reach/); assert.equal(calls,1);
  await assert.rejects(selectDependabotCredential({env,fetchImpl:async()=>({redirected:true})}),/refused a redirect/);
});
test('identical configured tokens are attempted only once', async () => {
  const api=fixture([403]); await assert.rejects(selectDependabotCredential({env:{...env,ALERT_CONNECTOR_TOKEN:env.ALERT_PRIMARY_TOKEN},...api}),/cannot read/); assert.equal(api.calls.length,1);
});

test('uses a permitted job token before trying the connector credential', async () => {
  const api=fixture([403,200]); assert.equal(await selectDependabotCredential({env:{...env,ALERT_JOB_TOKEN:'job-test-only'},...api}), 'job'); assert.equal(api.calls[1].headers.Authorization, 'Bearer job-test-only');
});
test('a denied job token may fall back to the authorized connector', async () => {
  const api=fixture([403,403,200]); assert.equal(await selectDependabotCredential({env:{...env,ALERT_JOB_TOKEN:'job-test-only'},...api}), 'connector'); assert.equal(api.calls.length,3);
});
for (const [status, headers] of [[429, {}], [403, {'x-ratelimit-remaining':'0'}], [403, {'retry-after':'60'}]]) test(`does not switch credentials on rate limits ${JSON.stringify(headers)}`, async () => {
  let calls=0; await assert.rejects(selectDependabotCredential({env,fetchImpl:async()=>{calls++;return Response.json({}, {status,headers});}}), /rate limit reached/); assert.equal(calls,1);
});
