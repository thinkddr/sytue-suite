/* SPDX-License-Identifier: Apache-2.0 */
import assert from 'node:assert/strict';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { createRequire } from 'node:module';
import { createBff } from '../../apps/frontend/packages/auth/src/server/bff.js';
import { crearProxyDeServicio } from '../../apps/frontend/packages/auth/src/server/proxy.js';
import { chromium, devices } from 'playwright';
import {
  bundleMatrixFixture,
  matrixFixtureAsset,
  matrixBrowserOptions,
} from './matrix-browser-fixture.mjs';
const directory = await mkdtemp(path.join(tmpdir(), 'sytue-session-owner-'));
let server, browser;
let accepted = 0,
  rejected = 0,
  bff;
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
const { SignJWT, generateKeyPair, exportJWK } = await import(
  pathToFileURL(
    createRequire(
      fileURLToPath(new URL('../../apps/frontend/packages/auth/package.json', import.meta.url)),
    ).resolve('jose'),
  ).href
);
const key = await generateKeyPair('RS256');
const jwk = { ...(await exportJWK(key.publicKey)), kid: 'owner-fixture', alg: 'RS256', use: 'sig' };
const codes = new Map();
const secret = 'browser-BFF-session-fixture-only-32bytes';
function withCookies(response, cookies) {
  const headers = new Headers(response.headers);
  for (const cookie of cookies.consume()) headers.append('set-cookie', cookie);
  return new Response(response.body, { status: response.status, headers });
}
async function login(request, actor, cookies) {
  const url = new URL('/mail/auth/login', request.url);
  const response = await bff.login({
    request: new Request(url, { headers: request.headers }),
    url,
    cookies,
  });
  const auth = new URL(response.headers.get('location'));
  const code = crypto.randomUUID();
  codes.set(code, {
    actor,
    nonce: auth.searchParams.get('nonce'),
    challenge: auth.searchParams.get('code_challenge'),
  });
  const callback = new URL('/mail/auth/callback', request.url);
  callback.searchParams.set('state', auth.searchParams.get('state'));
  callback.searchParams.set('code', code);
  const completed = await bff.callback({
    request: new Request(callback, { headers: request.headers }),
    url: callback,
    cookies,
  });
  assert.equal(completed.status, 302);
}
const unexpected = [];
try {
  await bundleMatrixFixture(
    fileURLToPath(new URL('./session-owner-client.ts', import.meta.url)),
    directory,
  );
  server = Bun.serve({
    hostname: '127.0.0.1',
    port: 0,
    async fetch(request) {
      const url = new URL(request.url),
        issuer = server.url.origin;
      if (url.pathname === '/.well-known/openid-configuration')
        return Response.json({
          issuer,
          authorization_endpoint: issuer + '/authorize',
          token_endpoint: issuer + '/token',
          userinfo_endpoint: issuer + '/userinfo',
          end_session_endpoint: issuer + '/logout',
          jwks_uri: issuer + '/jwks',
        });
      if (url.pathname === '/jwks') return Response.json({ keys: [jwk] });
      if (url.pathname === '/token' && request.method === 'POST') {
        const body = await request.formData(),
          code = codes.get(String(body.get('code')));
        if (!code) return Response.json({ error: 'invalid_grant' }, { status: 400 });
        const challenge = btoa(
          String.fromCharCode(
            ...new Uint8Array(
              await crypto.subtle.digest(
                'SHA-256',
                new TextEncoder().encode(String(body.get('code_verifier'))),
              ),
            ),
          ),
        )
          .replace(/=/g, '')
          .replace(/\+/g, '-')
          .replace(/\//g, '_');
        if (code.challenge !== challenge)
          return Response.json({ error: 'invalid_grant' }, { status: 400 });
        codes.delete(String(body.get('code')));
        const idToken = await new SignJWT({
          sub: code.actor,
          tenant: 'tenant',
          nonce: code.nonce,
          roles: ['member'],
        })
          .setProtectedHeader({ alg: 'RS256', kid: 'owner-fixture' })
          .setIssuer(issuer)
          .setAudience('mail')
          .setIssuedAt()
          .setExpirationTime('5m')
          .sign(key.privateKey);
        return Response.json({
          access_token: 'fixture-access',
          refresh_token: 'fixture-refresh',
          id_token: idToken,
          expires_in: 300,
        });
      }
      const cookies = new AstroCookies(request, { warn() {} });
      if (url.pathname === '/') {
        accepted = 0;
        rejected = 0;
        await login(request, 'owner', cookies);
        return withCookies(
          new Response(
            '<!doctype html><script>window.__sytueBase="/mail"</script><script type="module" src="/session-owner-client.js"></script>',
            { headers: { 'Content-Type': 'text/html' } },
          ),
          cookies,
        );
      }
      if (
        (url.pathname === '/test/same-actor-login' || url.pathname === '/test/other-actor-login') &&
        request.method === 'POST'
      ) {
        await login(
          request,
          url.pathname.endsWith('same-actor-login') ? 'owner' : 'other',
          cookies,
        );
        return withCookies(new Response(null, { status: 204 }), cookies);
      }
      if (url.pathname === '/mail/auth/session')
        return withCookies(await bff.session({ request, url, cookies }), cookies);
      if (url.pathname === '/mail/api/check' || url.pathname === '/mail/api/domain-conflict') {
        const proxy = crearProxyDeServicio({
          destino: 'http://fixture-service.invalid',
          bff,
          fetchImpl: async (target) => {
            accepted++;
            return Response.json(
              { ok: true },
              { status: String(target).endsWith('domain-conflict') ? 409 : 200 },
            );
          },
        });
        const response = await proxy({
          request,
          url,
          cookies,
          params: { ruta: url.pathname.split('/').at(-1) },
        });
        if (response.headers.get('X-Sytue-Session-Changed') === '1') rejected++;
        return withCookies(response, cookies);
      }
      if (url.pathname === '/favicon.ico') return new Response(null, { status: 204 });
      if (request.method !== 'GET') {
        unexpected.push(url.pathname);
        return new Response(null, { status: 403 });
      }
      return matrixFixtureAsset(url, directory);
    },
  });
  bff = createBff({
    config: {
      issuer: server.url.origin,
      clientId: 'mail',
      clientSecret: 'fixture-only',
      redirectUri: server.url.origin + '/mail/auth/callback',
      postLogoutRedirectUri: server.url.origin + '/',
    },
    sessionSecret: secret,
    basePath: '/mail',
    secureCookies: false,
  });
  browser = await chromium.launch(matrixBrowserOptions());
  for (const profile of ['desktop', 'mobile']) {
    const context = await browser.newContext(profile === 'mobile' ? devices['Pixel 7'] : {});
    const page = await context.newPage(),
      errors = [];
    page.on('pageerror', (error) => errors.push(error.name));
    await page.route('**/*', async (route) => {
      if (new URL(route.request().url()).origin === 'http://127.0.0.1:' + server.port)
        await route.continue();
      else {
        unexpected.push(new URL(route.request().url()).origin);
        await route.abort();
      }
    });
    for (const reload of [false, true]) {
      await page.goto('http://127.0.0.1:' + server.port + (reload ? '/?resume=1' : '/'));
      await page.waitForFunction(
        () => window.sessionOwnerProbe?.ok || window.sessionOwnerProbe?.stage.startsWith('failed:'),
        {},
        { timeout: 30000 },
      );
      const state = await page.evaluate(() => window.sessionOwnerProbe);
      console.log(JSON.stringify({ profile, reload, state, accepted, rejected, errors }));
      assert.equal(state.ok, true);
      const stored = await context.cookies();
      const sessionCookies = stored.filter((cookie) => cookie.name.startsWith('sytue_session_'));
      assert.ok(sessionCookies.length > 0);
      assert.ok(
        sessionCookies.every(
          (cookie) => cookie.httpOnly && cookie.path === '/' && cookie.sameSite === 'Lax',
        ),
      );
      assert.equal((await page.evaluate(() => document.cookie)).includes('sytue_session_'), false);
      assert.equal(accepted, 2);
      assert.equal(rejected, 1);
      assert.deepEqual(errors, []);
    }
    await context.close();
  }
  assert.deepEqual(unexpected, []);
  console.log('REAL_BFF_ASTROCOOKIES_PKCE_RS256_OPAQUE_OWNER_CHROMIUM_DESKTOP_MOBILE_RELOAD_GREEN');
} finally {
  await browser?.close();
  await server?.stop(true);
  await rm(directory, { recursive: true, force: true });
}
