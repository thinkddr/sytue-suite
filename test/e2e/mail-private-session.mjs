/* SPDX-License-Identifier: Apache-2.0 */
// Opt-in artificial native/PG/SSSS proof; the browser uses the production BFF factories.
import assert from 'node:assert/strict';
import { readFile, writeFile, mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { createRequire } from 'node:module';
import { chromium } from 'playwright';
import { createBff } from '../../apps/frontend/packages/auth/src/server/bff.js';
import { SessionSlots } from '../../apps/frontend/packages/auth/src/server/session-slots.js';
import {
  rutaMailPrivado,
  rutaCorteIdentidadMatrixPrivada,
} from '../../apps/frontend/packages/auth/src/server/mail-private-storage.js';
import {
  rutaAutoridadMatrixPrivada,
  rutaPublicacionesMatrixPrivadas,
} from '../../apps/frontend/packages/auth/src/server/matrix-storage-authority.js';
import { rutaBlobsMatrixPrivados } from '../../apps/frontend/packages/auth/src/server/matrix-storage-blobs.js';
import {
  bundleMatrixFixture,
  matrixFixtureAsset,
  matrixBrowserOptions,
} from './matrix-browser-fixture.mjs';
const [fixturePath, resultPath] = process.argv.slice(2);
assert.ok(fixturePath && resultPath, 'private artificial fixture and result paths required');
const fixture = JSON.parse(await readFile(fixturePath, 'utf8'));
assert.equal(fixture.artificial, true);
assert.equal(fixture.native_migration_completed, true);
const browserOrigin = new URL(fixture.browser_origin);
assert.equal(browserOrigin.origin, 'https://storage.test:18746');
for (const key of ['api_origin', 'blobs_origin']) {
  const origin = new URL(fixture[key]);
  assert.ok(['127.0.0.1', 'localhost', '[::1]'].includes(origin.hostname));
  assert.ok(['http:', 'https:'].includes(origin.protocol));
  assert.equal(origin.pathname, '/');
  assert.equal(origin.search, '');
  assert.equal(origin.hash, '');
  assert.equal(origin.username, '');
  assert.equal(origin.password, '');
}
assert.equal(fixture.user.id, fixture.scope.subject);
assert.equal(fixture.user.tenant, fixture.scope.tenant);
assert.equal(fixture.scope.product, 'mail');
assert.equal(
  path.dirname(path.resolve(fixture.ready_path)),
  path.dirname(path.resolve(fixturePath)),
);
assert.equal(path.dirname(path.resolve(resultPath)), path.dirname(path.resolve(fixturePath)));
const directory = await mkdtemp(path.join(tmpdir(), 'sytue-mail-private-session-'));
const requireFrontend = createRequire(
  fileURLToPath(new URL('../../apps/frontend/package.json', import.meta.url)),
);
const { AstroCookies } = await import(
  pathToFileURL(
    path.join(
      path.dirname(requireFrontend.resolve('astro/package.json')),
      'dist/core/cookies/index.js',
    ),
  ).href
);
const secret = crypto.randomUUID() + crypto.randomUUID();
const bff = createBff({
  config: {
    issuer: browserOrigin.origin,
    clientId: fixture.bff_client_id,
    clientSecret: 'artificial-driver-only',
    redirectUri: browserOrigin.origin + '/mail/auth/callback',
    postLogoutRedirectUri: browserOrigin.origin + '/mail/',
  },
  sessionSecret: secret,
  basePath: '/mail',
  secureCookies: true,
});
const slots = new SessionSlots(secret, true, 3600);
const env = {
  SYTUE_MAIL_INTERNAL_URL: fixture.api_origin,
  SYTUE_ROOMS_INTERNAL_URL: fixture.api_origin,
  SYTUE_DRIVE_INTERNAL_URL: fixture.blobs_origin,
};
// Runtime environment variables must never send this fixture's credentials to
// another deployment. The production route factories still own every path.
const fixedOrigins = new Set([
  new URL(fixture.api_origin).origin,
  new URL(fixture.blobs_origin).origin,
]);
const fixedFetch = async (input, init) => {
  const target = new URL(input instanceof Request ? input.url : input);
  assert.ok(fixedOrigins.has(target.origin), 'fixture upstream origin changed');
  return fetch(input, { ...init, redirect: 'error' });
};
const mailRoute = rutaMailPrivado({ bff, env, fetchImpl: fixedFetch }),
  cutoverRoute = rutaCorteIdentidadMatrixPrivada({
    bff,
    env,
    product: 'mail',
    fetchImpl: fixedFetch,
  }),
  authorityRoute = rutaAutoridadMatrixPrivada({ bff, env, product: 'mail', fetchImpl: fixedFetch }),
  publicationRoute = rutaPublicacionesMatrixPrivadas({
    bff,
    env,
    product: 'mail',
    fetchImpl: fixedFetch,
  }),
  blobRoute = rutaBlobsMatrixPrivados({ bff, env, product: 'mail', fetchImpl: fixedFetch });
let server, browser, resumeState;
let sessionSeeded = false,
  armLostAck = false,
  lostAckKey;
const publications = new Map();
function publicationKey(url) {
  const match = /\/rooms\/([^/]+)\/send\/m\.room\.encrypted\/([^/]+)$/.exec(url.pathname);
  return match
    ? JSON.stringify([
        fixture.scope.tenant,
        fixture.scope.subject,
        fixture.scope.product,
        decodeURIComponent(match[1]),
        decodeURIComponent(match[2]),
      ])
    : undefined;
}
const counts = { session: 0, bff: 0, matrix_send: 0, matrix_account_data: 0 },
  errors = [];
function withCookies(response, cookies) {
  const headers = new Headers(response.headers);
  for (const cookie of cookies.consume()) headers.append('set-cookie', cookie);
  return new Response(response.body, { status: response.status, headers });
}
function publicFixture() {
  return {
    artificial: true,
    native_migration_completed: true,
    bootstrap: fixture.bootstrap,
    session: fixture.session,
    scope: fixture.scope,
    recovery: fixture.recovery,
    known_id: fixture.known_id,
    known_mime: fixture.known_mime,
    ...(resumeState
      ? {
          resume: true,
          resume_expected_mime: resumeState.mime,
          baseline_pending: resumeState.baseline_pending,
        }
      : {}),
  };
}
const debugEnabled = process.env.SYTUE_MAIL_WEB_DEBUG === '1';
const debugStatePath = '/tmp/mail-private-debug-20261102.json';
const debugTriggerPath = '/tmp/mail-private-debug-trigger-20261102.json';
const debugStarted = Date.now();
let debugLast = {v: 1, status: 'starting'};
function safeFailure(probe) {
  const text = value => typeof value === 'string' && /^[a-z0-9_.:-]{1,160}$/i.test(value) ? value : 'unknown';
  return {
    stage: text(probe.failure_stage ?? probe.stage),
    code: new Set(['crypto_rejected','crypto_runtime_unavailable','matrix_login','matrix_identity','matrix_device','matrix_room','matrix_locked','matrix_recovery','matrix_unverified','matrix_reference','matrix_unsupported']).has(probe.reason) ? probe.reason : 'private_session_failed',
    rpc_failures: Array.isArray(probe.rpc_failures) ? probe.rpc_failures.slice(-8).map(text) : [],
    host_failures: Array.isArray(probe.host_failures) ? probe.host_failures.slice(-8).map(text) : [],
  };
}
async function debugRetry(probe, resume, attempt) {
  if (!debugEnabled) return false;
  const globalDeadline = fixture.debug_deadline_ms;
  assert.ok(Number.isSafeInteger(globalDeadline) && globalDeadline > 0, 'debug requires original global deadline');
  const deadline = Math.min(globalDeadline, debugStarted + 14 * 60 * 1000) - 5000;
  debugLast = {v: 1, status: 'waiting', phase: resume ? 'resume' : 'open', attempt, ...safeFailure(probe)};
  await writeFile(debugStatePath, JSON.stringify(debugLast), {mode: 0o600});
  while (Date.now() < deadline) {
    let raw;
    try { raw = await readFile(debugTriggerPath); }
    catch (error) {
      if (error?.code !== 'ENOENT') throw error;
      await new Promise(resolve => setTimeout(resolve, 500));
      continue;
    }
    let command;
    try { if (raw.byteLength <= 1024) command = JSON.parse(raw.toString('utf8')); }
    catch { /* A trigger is data, never executable text. */ }
    finally { raw.fill(0); await rm(debugTriggerPath, {force: true}); }
    if (!command || Object.keys(command).length !== 1 || !['retry', 'finish'].includes(command.action)) {
      await writeFile(debugStatePath, JSON.stringify({...debugLast, status: 'invalid-trigger'}), {mode: 0o600});
      continue;
    }
    if (command.action === 'finish') return false;
    // The failed page has already awaited SuiteMatrix.closeAndDrain. Keep this
    // context and its actual SDK/IDB identity; rebundle source, then reload.
    await bundleMatrixFixture(fileURLToPath(new URL('./mail-private-session-client.ts', import.meta.url)), directory);
    await writeFile(debugStatePath, JSON.stringify({...debugLast, status: 'retrying'}), {mode: 0o600});
    return true;
  }
  await writeFile(debugStatePath, JSON.stringify({...debugLast, status: 'expired'}), {mode: 0o600});
  return false;
}
try {
  if(debugEnabled) await rm(debugTriggerPath,{force:true});
  await bundleMatrixFixture(
    fileURLToPath(new URL('./mail-private-session-client.ts', import.meta.url)),
    directory,
  );
  server = Bun.serve({
    hostname: '127.0.0.1',
    port: 0,
    async fetch(incoming) {
      const url = new URL(
        new URL(incoming.url).pathname + new URL(incoming.url).search,
        browserOrigin,
      );
      const request = new Request(url, incoming),
        cookies = new AstroCookies(request, { warn() {} }),
        ctx = { request, url, cookies, params: {} };
      if (url.pathname === '/mail/') {
        if (!sessionSeeded) {
          const selected = await slots.begin(ctx);
          await slots.complete(ctx, selected.candidate.id, {
            generation: selected.candidate.id,
            user: {
              emailVerified: true,
              preferredUsername: 'artificial',
              groups: [],
              ...fixture.user,
            },
            tokens: {
              accessToken: fixture.sytue_access_token,
              expiresAt: fixture.sytue_expires_at * 1000,
              tokenType: 'Bearer',
            },
            clientId: fixture.bff_client_id,
          });
          sessionSeeded = true;
        } else assert.ok(await slots.read(ctx), 'reload lost its sealed BFF session');
        return withCookies(
          new Response(
            '<!doctype html><script>window.__sytueBase="/mail"</script><script type="module" src="/mail-private-session-client.js"></script>',
            { headers: { 'content-type': 'text/html', 'cache-control': 'no-store' } },
          ),
          cookies,
        );
      }
      if (url.pathname === '/test/arm-lost-ack' && request.method === 'POST') {
        assert.equal(armLostAck, false);
        assert.equal(lostAckKey, undefined);
        armLostAck = true;
        return new Response(null, { status: 204 });
      }
      if (url.pathname === '/fixture')
        return Response.json(publicFixture(), { headers: { 'cache-control': 'no-store' } });
      if (url.pathname === '/mail/auth/session') {
        counts.session++;
        return withCookies(await bff.session(ctx), cookies);
      }
      const routes = [
        ['/mail/api/private-storage/', mailRoute],
        ['/mail/api/matrix/storage/migration', cutoverRoute],
        ['/mail/api/matrix/storage/authority', authorityRoute],
        ['/mail/api/matrix/storage/publications', publicationRoute],
        ['/mail/api/matrix/storage/blobs/', blobRoute],
      ];
      for (const [prefix, handler] of routes) {
        if (
          url.pathname === prefix ||
          url.pathname.startsWith(prefix.endsWith('/') ? prefix : prefix + '/')
        ) {
          counts.bff++;
          ctx.params.ruta = url.pathname.slice(prefix.length).replace(/^\//, '');
          return withCookies(await handler(ctx), cookies);
        }
      }
      if (url.pathname === '/favicon.ico') return new Response(null, { status: 204 });
      if (request.method !== 'GET') return new Response(null, { status: 404 });
      return matrixFixtureAsset(url, directory);
    },
  });
  await writeFile(fixture.ready_path, JSON.stringify({ origin: server.url.origin }), {
    mode: 0o600,
  });
  browser = await chromium.launch(matrixBrowserOptions());
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const unexpectedOrigins = [];
  await context.route('**/*', async (route) => {
    const target = new URL(route.request().url());
    if (target.origin !== browserOrigin.origin && target.protocol !== 'blob:') {
      unexpectedOrigins.push(target.origin);
      await route.abort();
      return;
    }
    const key = publicationKey(target);
    if (!armLostAck || !key) {
      await route.continue();
      return;
    }
    armLostAck = false;
    try {
      const headers = await route.request().allHeaders();
      // Forward the exact method/body through the dedicated TLS Origin proxy.
      // APIRequestContext does not use Chrome's storage.test DNS resolver rule.
      const forwardingURL = new URL(target.pathname + target.search, 'https://127.0.0.1:18746');
      const response = await route.fetch({
        url: forwardingURL.href,
        headers: { ...headers, host: browserOrigin.host, cookie: '' },
        maxRedirects: 0,
        timeout: 60000,
      });
      assert.ok(response.ok(), 'actual Matrix request was not accepted: ' + response.status());
      lostAckKey = key;
      await page.evaluate(() => {
        if (typeof window.mailPrivateCrashAfterAccepted !== 'function')
          throw new Error('crash hook missing');
        window.mailPrivateCrashAfterAccepted();
      });
      await response.dispose();
      try {
        await route.abort('failed');
      } catch {
        /* Owner cancellation may already have aborted this exact request. */
      }
    } catch (error) {
      errors.push('lost-ack-forward:' + (error instanceof Error ? error.name : 'failure'));
      try {
        await route.abort('failed');
      } catch {
        /* Already canceled. */
      }
    }
  });
  const page = await context.newPage();
  page.on('pageerror', (error) => errors.push(error.name));
  page.on('request', (request) => {
    const target = new URL(request.url()),
      key = publicationKey(target);
    if (key) {
      counts.matrix_send++;
      publications.set(key, (publications.get(key) ?? 0) + 1);
    } else if (target.pathname.includes('/account_data/')) counts.matrix_account_data++;
  });
  const phases = [];
  for (const resume of [false, true]) {
    let probe, debugAttempt = 0;
    for (;;) {
      await page.goto(browserOrigin.origin + '/mail/', {waitUntil: 'domcontentloaded', timeout: 120000});
      await page.waitForFunction(
        () => window.mailPrivateProbe?.ok || window.mailPrivateProbe?.stage === 'failed',
        {}, {timeout: 300000},
      );
      probe = await page.evaluate(() => window.mailPrivateProbe);
      if (probe.ok || !await debugRetry(probe, resume, ++debugAttempt)) break;
    }
    assert.equal(
      probe.ok,
      true,
      'mail private proof ' + (probe.failure_stage ?? probe.stage) + ': ' + safeFailure(probe).code +
        ' rpc=' + JSON.stringify(safeFailure(probe).rpc_failures) + ' host=' + JSON.stringify(safeFailure(probe).host_failures) + ' bootstrap=' + JSON.stringify({identity_version:fixture.bootstrap.storage_identity_version??0,channels_array:Array.isArray(fixture.bootstrap.channels),provider_present:typeof fixture.bootstrap.sso_provider_id==='string'&&fixture.bootstrap.sso_provider_id.length>0,scope_match:fixture.bootstrap.actor===fixture.scope.subject&&fixture.bootstrap.tenant===fixture.scope.tenant&&fixture.bootstrap.storage_product===fixture.scope.product,user_matches:fixture.bootstrap.matrix_user_id===fixture.session.user_id,controller_matches:fixture.bootstrap.controller_user_id==='@rooms_controller:'+browserOrigin.host,home_matches:fixture.bootstrap.homeserver_url===browserOrigin.origin}),
    );
    assert.ok(probe.result);
    assert.equal(probe.result.paused, true);
    assert.equal(probe.result.pending_verified, true);
    assert.equal(probe.result.closed_and_retained, true);
    phases.push({
      resume,
      timings: probe.result.timings,
      pending_count: probe.result.pending_count,
    });
    if (!resume) {
      assert.equal(probe.result.lost_ack, true);
      assert.ok(lostAckKey);
      assert.equal(publications.get(lostAckKey), 1);
      assert.ok(probe.result.mime);
      assert.ok(probe.result.pending_count > probe.result.baseline_pending);
      resumeState = { mime: probe.result.mime, baseline_pending: probe.result.baseline_pending };
    } else {
      assert.equal(probe.result.ack_recovered, true);
      assert.equal(
        publications.get(lostAckKey),
        1,
        'accepted transaction was sent again after reload',
      );
      assert.equal(probe.result.mime, resumeState.mime);
      assert.ok(probe.result.message_id);
      await writeFile(resultPath, JSON.stringify(probe.result), { mode: 0o600 });
    }
  }
  const stored = await context.cookies();
  const sessionCookies = stored.filter((cookie) => cookie.name.startsWith('__Host-sytue_session_'));
  assert.ok(sessionCookies.length > 0);
  assert.ok(
    sessionCookies.every((cookie) => cookie.httpOnly && cookie.secure && cookie.path === '/'),
  );
  assert.equal((await page.evaluate(() => document.cookie)).includes('sytue_session_'), false);
  assert.deepEqual(errors, []);
  assert.deepEqual(unexpectedOrigins, []);
  const publicationCounts = [...publications].map(([key, requests]) => ({
    scope_room_txn_sha256: new Bun.CryptoHasher('sha256').update(key).digest('hex'),
    requests,
    lost_ack: key === lostAckKey,
  }));
  console.log(
    JSON.stringify({
      proof: 'actual-native-browser-native-mail',
      phases,
      counts,
      publications: publicationCounts,
    }),
  );
  console.log('REAL_MAIL_MANAGER_BFF_SDK_PQ_SSSS_ACCEPTED_ACK_LOST_RELOAD_GO_EXACT_COMMIT_GREEN');
  await context.close();
} finally {
  if(debugEnabled){await writeFile(debugStatePath,JSON.stringify({...debugLast,status:'runner_closed'}),{mode:0o600});await rm(debugTriggerPath,{force:true});}
  await browser?.close();
  await server?.stop(true);
  await rm(directory, { recursive: true, force: true });
}
