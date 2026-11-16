import { expect, test } from 'bun:test';
import { SignJWT, generateKeyPair, exportJWK } from 'jose';
import type { SytueSession } from '../types.js';
import { createBff, type BffContext, type CookieJar } from './bff.js';
import { SessionSlots, newSessionGeneration, sessionOwner } from './session-slots.js';
import { seal } from './cookies.js';
import { sessionOwnerHeader, sessionChangedHeader } from '@sytue/config/session-owner';
import { crearProxyDeServicio } from './proxy.js';
const SECRET = 'test-session-slot-secret-only-32bytes';
function user(id = 'alice', tenant = 'acme'): SytueSession['user'] {
  return {
    id,
    tenant,
    email: id + '@example.test',
    emailVerified: true,
    name: id,
    preferredUsername: id,
    locale: 'en',
    roles: ['member'],
    groups: [],
  };
}
function session(id = 'alice', tenant = 'acme', generation = newSessionGeneration()): SytueSession {
  return {
    user: user(id, tenant),
    clientId: 'sytue-mail',
    generation,
    tokens: {
      accessToken: 'fixture-access-' + id,
      refreshToken: 'fixture-refresh-' + id,
      expiresAt: Date.now() + 300000,
      tokenType: 'Bearer',
    },
  };
}
class BrowserCookies {
  values = new Map<string, string>();
  request(path = '/mail/auth/session', headers: Record<string, string> = {}) {
    const local = new Map(this.values);
    const writes: Array<[string, string | null]> = [];
    const cookies: CookieJar = {
      get: (name) => {
        const value = local.get(name);
        return value === undefined ? undefined : { value };
      },
      set: (name, value) => {
        local.set(name, value);
        writes.push([name, value]);
      },
      delete: (name) => {
        local.delete(name);
        writes.push([name, null]);
      },
    };
    const url = new URL(path, 'http://mail.example.test');
    const requestHeaders = new Headers(headers);
    requestHeaders.set(
      'cookie',
      [...this.values].map(([name, value]) => name + '=' + value).join('; '),
    );
    const ctx: BffContext = {
      url,
      request: new Request(url, { headers: requestHeaders }),
      cookies,
    };
    return {
      ctx,
      writes,
      apply: () => {
        for (const [name, value] of writes) {
          if (value === null) this.values.delete(name);
          else this.values.set(name, value);
        }
      },
    };
  }
}
async function initial(
  slots: SessionSlots,
  browser: BrowserCookies,
  id = 'alice',
  tenant = 'acme',
) {
  const req = browser.request();
  const selected = await slots.begin(req.ctx);
  await slots.complete(req.ctx, selected.candidate.id, session(id, tenant, selected.candidate.id));
  req.apply();
  return selected.candidate.id;
}
function deferred() {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { promise, release };
}

test('immutable slots retain the new login after an old callback or refresh arrives', async () => {
  const slots = new SessionSlots(SECRET, true, 3600),
    browser = new BrowserCookies();
  const generationA = await initial(slots, browser);
  const oldRefresh = browser.request();
  const captured = await slots.read(oldRefresh.ctx);
  expect(captured).not.toBeNull();
  const generationB = await initial(slots, browser, 'bob', 'other');
  await slots.persist(oldRefresh.ctx, captured!, {
    ...captured!.session,
    tokens: { ...captured!.session.tokens, accessToken: 'new-A' },
  });
  oldRefresh.apply();
  expect((await slots.read(browser.request().ctx))?.session.generation).toBe(generationB);
  expect(generationB).not.toBe(generationA);
  expect(oldRefresh.writes.some(([name]) => name === slots.selectionCookie)).toBe(false);
});

test('a delayed logout only revokes the slot visible to that request', async () => {
  const slots = new SessionSlots(SECRET, true, 3600),
    browser = new BrowserCookies();
  await initial(slots, browser);
  const oldLogout = browser.request();
  await slots.revokeCurrent(oldLogout.ctx);
  const generationB = await initial(slots, browser, 'bob');
  oldLogout.apply();
  expect((await slots.read(browser.request().ctx))?.session.generation).toBe(generationB);
});

test('revocation survives a callback and refresh completing after logout', async () => {
  const slots = new SessionSlots(SECRET, true, 3600),
    browser = new BrowserCookies();
  const generation = await initial(slots, browser);
  const callback = browser.request(),
    refresh = browser.request();
  const captured = await slots.read(refresh.ctx);
  const logout = browser.request();
  await slots.revokeCurrent(logout.ctx);
  logout.apply();
  await slots.complete(callback.ctx, generation, session('alice', 'acme', generation));
  callback.apply();
  await slots.persist(refresh.ctx, captured!, captured!.session);
  refresh.apply();
  expect(await slots.read(browser.request().ctx)).toBeNull();
  expect(await slots.canComplete(browser.request().ctx, generation)).toBe(false);
});

test('pending and cancelled login preserve the prior account, completed missing slot never does', async () => {
  const slots = new SessionSlots(SECRET, true, 3600),
    browser = new BrowserCookies();
  const prior = await initial(slots, browser);
  const next = browser.request();
  const pending = await slots.begin(next.ctx);
  next.apply();
  expect((await slots.read(browser.request().ctx))?.session.generation).toBe(prior);
  const cancelled = browser.request();
  slots.cancel(cancelled.ctx, pending.candidate.id);
  cancelled.apply();
  expect((await slots.read(browser.request().ctx))?.session.generation).toBe(prior);
  const done = browser.request();
  await slots.complete(
    done.ctx,
    pending.candidate.id,
    session('bob', 'acme', pending.candidate.id),
  );
  done.apply();
  // A stale duplicate error response cannot replace a completed account with fallback.
  cancelled.apply();
  expect((await slots.read(browser.request().ctx))?.session.user.id).toBe('bob');
  browser.values.delete('__Host-sytue_session_slot_' + pending.candidate.id);
  expect(await slots.read(browser.request().ctx)).toBeNull();
});

test('legacy cookie upgrade assigns one stable HMAC generation without replacing pointer or anchor', async () => {
  const slots = new SessionSlots(SECRET, true, 3600),
    browser = new BrowserCookies();
  const legacy = session();
  delete legacy.generation;
  const anchor = await seal(legacy, SECRET);
  browser.values.set('__Host-sytue_session', anchor);
  const one = browser.request(),
    two = browser.request();
  const a = await slots.read(one.ctx),
    b = await slots.read(two.ctx);
  expect(a?.session.generation).toMatch(/^[a-f0-9]{64}$/);
  expect(a?.session.generation).toBe(b?.session.generation);
  one.apply();
  two.apply();
  expect(browser.values.get('__Host-sytue_session')).toBe(anchor);
  expect(one.writes.every(([name]) => name.startsWith('__Host-sytue_session_slot_'))).toBe(true);
  const selected = browser.request();
  await slots.persist(selected.ctx, a!, {
    ...a!.session,
    tokens: { ...a!.session.tokens, accessToken: 'rotated' },
  });
  selected.apply();
  expect((await slots.read(browser.request().ctx))?.session.generation).toBe(a?.session.generation);
  const generationB = await initial(slots, browser, 'bob');
  two.apply();
  expect((await slots.read(browser.request().ctx))?.session.generation).toBe(generationB);
});

test('generation distinguishes same actor re-login and tenant switch; descriptor contains no tokens', async () => {
  const slots = new SessionSlots(SECRET, true, 3600),
    browser = new BrowserCookies();
  const a = await initial(slots, browser),
    b = await initial(slots, browser);
  expect(a).not.toBe(b);
  const current = (await slots.read(browser.request().ctx))!;
  expect(
    slots.guard(browser.request('/mail/api/messages', { [sessionOwnerHeader]: a }).ctx, current)
      ?.status,
  ).toBe(409);
  expect(
    slots.guard(browser.request('/mail/api/messages', { [sessionOwnerHeader]: b }).ctx, current),
  ).toBeNull();
  const descriptor = sessionOwner(current.session)!;
  expect(Object.keys(descriptor).sort()).toEqual([
    'actor',
    'client_id',
    'generation',
    'tenant',
    'v',
  ]);
  expect(JSON.stringify(descriptor)).not.toContain('fixture-access');
  await initial(slots, browser, 'alice', 'other');
  const changed = slots.guard(
    browser.request('/mail/api/messages', { [sessionOwnerHeader]: b }).ctx,
    await slots.read(browser.request().ctx),
  );
  expect(changed?.headers.get(sessionChangedHeader)).toBe('1');
});

async function oidcFixture() {
  const key = await generateKeyPair('RS256');
  const jwk = {
    ...(await exportJWK(key.publicKey)),
    kid: 'slot-fixture',
    alg: 'RS256',
    use: 'sig',
  };
  const codes = new Map<
    string,
    { nonce: string; challenge: string; id: string; tenant: string; gate?: Promise<void> }
  >();
  let tokenCalls = 0;
  let refreshGate: Promise<void> | undefined;
  const server: Bun.Server<undefined> = Bun.serve<undefined>({
    hostname: '127.0.0.1',
    port: 0,
    async fetch(request): Promise<Response> {
      const url = new URL(request.url);
      const issuer = server.url.origin;
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
      if (url.pathname !== '/token') return new Response(null, { status: 404 });
      tokenCalls++;
      const body = await request.formData();
      if (body.get('grant_type') === 'refresh_token') {
        if (refreshGate) await refreshGate;
        return Response.json({
          access_token: 'refreshed-fixture-access',
          refresh_token: 'refreshed-fixture-refresh',
          expires_in: 300,
        });
      }
      const code = codes.get(String(body.get('code')));
      if (!code) return Response.json({ error: 'invalid_grant' }, { status: 400 });
      const verifier = String(body.get('code_verifier'));
      const challenge = btoa(
        String.fromCharCode(
          ...new Uint8Array(
            await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier)),
          ),
        ),
      )
        .replace(/=/g, '')
        .replace(/\+/g, '-')
        .replace(/\//g, '_');
      if (challenge !== code.challenge)
        return Response.json({ error: 'invalid_grant' }, { status: 400 });
      codes.delete(String(body.get('code')));
      if (code.gate) await code.gate;
      const idToken = await new SignJWT({
        sub: code.id,
        tenant: code.tenant,
        nonce: code.nonce,
        email: code.id + '@example.test',
        roles: ['member'],
      })
        .setProtectedHeader({ alg: 'RS256', kid: 'slot-fixture' })
        .setIssuer(issuer)
        .setAudience('sytue-mail')
        .setIssuedAt()
        .setExpirationTime('5m')
        .sign(key.privateKey);
      return Response.json({
        access_token: 'fixture-access-' + code.id,
        refresh_token: 'fixture-refresh-' + code.id,
        id_token: idToken,
        expires_in: 300,
      });
    },
  });
  const bff = createBff({
    config: {
      issuer: server.url.origin,
      clientId: 'sytue-mail',
      clientSecret: 'fixture-only',
      redirectUri: 'http://mail.example.test/mail/auth/callback',
      postLogoutRedirectUri: 'http://mail.example.test/mail/',
    },
    sessionSecret: SECRET,
    basePath: '/mail',
    secureCookies: false,
  });
  async function begin(
    browser: BrowserCookies,
    id = 'alice',
    tenant = 'acme',
    gate?: Promise<void>,
  ) {
    const req = browser.request('/mail/auth/login?returnTo=%2Fmail%2Finbox');
    const res = await bff.login(req.ctx);
    req.apply();
    const auth = new URL(res.headers.get('location')!);
    const state = auth.searchParams.get('state')!;
    const code = crypto.randomUUID();
    codes.set(code, {
      nonce: auth.searchParams.get('nonce')!,
      challenge: auth.searchParams.get('code_challenge')!,
      id,
      tenant,
      ...(gate ? { gate } : {}),
    });
    return {
      state,
      path: '/mail/auth/callback?code=' + code + '&state=' + encodeURIComponent(state),
    };
  }
  return {
    bff,
    begin,
    stop: () => server.stop(true),
    calls: () => tokenCalls,
    setRefreshGate: (gate: Promise<void>) => {
      refreshGate = gate;
    },
  };
}

test('real BFF code+PKCE/RS256 callbacks and refresh keep a later account selected', async () => {
  const fixture = await oidcFixture(),
    browser = new BrowserCookies();
  try {
    const loginA = await fixture.begin(browser);
    const cbA = browser.request(loginA.path);
    expect((await fixture.bff.callback(cbA.ctx)).status).toBe(302);
    cbA.apply();
    const ownerA = (await (await fixture.bff.session(browser.request().ctx)).json()) as {
      owner: { generation: string };
    };
    const refreshHold = deferred();
    fixture.setRefreshGate(refreshHold.promise);
    const refresh = browser.request('/mail/auth/refresh');
    const refreshing = fixture.bff.refresh(refresh.ctx);
    while (fixture.calls() < 2) await new Promise((resolve) => setTimeout(resolve, 1));
    const callbackHold = deferred(),
      loginOld = await fixture.begin(browser, 'old-pending', 'acme', callbackHold.promise);
    const old = browser.request(loginOld.path);
    const completingOld = fixture.bff.callback(old.ctx);
    while (fixture.calls() < 3) await new Promise((resolve) => setTimeout(resolve, 1));
    const loginB = await fixture.begin(browser, 'bob', 'other'),
      cbB = browser.request(loginB.path);
    expect((await fixture.bff.callback(cbB.ctx)).status).toBe(302);
    cbB.apply();
    refreshHold.release();
    callbackHold.release();
    expect((await refreshing).status).toBe(200);
    refresh.apply();
    await completingOld;
    old.apply();
    const current = (await (await fixture.bff.session(browser.request().ctx)).json()) as {
      user: { id: string; tenant: string };
      owner: { generation: string };
    };
    expect(current.user.id).toBe('bob');
    expect(current.user.tenant).toBe('other');
    expect(current.owner.generation).not.toBe(ownerA.owner.generation);
    const before = fixture.calls();
    const guarded = browser.request('/mail/auth/refresh', {
      [sessionOwnerHeader]: ownerA.owner.generation,
    });
    const rejected = await fixture.bff.refresh(guarded.ctx);
    expect(rejected.status).toBe(409);
    expect(rejected.headers.get(sessionChangedHeader)).toBe('1');
    expect(fixture.calls()).toBe(before);
  } finally {
    fixture.stop();
  }
});

test('real BFF logout fence wins over an in-flight callback without erasing a later login', async () => {
  const fixture = await oidcFixture(),
    browser = new BrowserCookies();
  try {
    const first = await fixture.begin(browser),
      callback = browser.request(first.path);
    await fixture.bff.callback(callback.ctx);
    callback.apply();
    const delayedLogout = browser.request('/mail/auth/logout?scope=local');
    expect((await fixture.bff.logout(delayedLogout.ctx)).status).toBe(204);
    const next = await fixture.begin(browser, 'bob'),
      nextCallback = browser.request(next.path);
    await fixture.bff.callback(nextCallback.ctx);
    nextCallback.apply();
    delayedLogout.apply();
    expect((await fixture.bff.getSession(browser.request().ctx))?.user.id).toBe('bob');
    const hold = deferred(),
      pending = await fixture.begin(browser, 'charlie', 'other', hold.promise),
      pendingCallback = browser.request(pending.path);
    const completing = fixture.bff.callback(pendingCallback.ctx);
    const priorCalls = fixture.calls();
    while (fixture.calls() === priorCalls) await new Promise((resolve) => setTimeout(resolve, 1));
    const logout = browser.request('/mail/auth/logout?scope=local');
    await fixture.bff.logout(logout.ctx);
    logout.apply();
    hold.release();
    await completing;
    pendingCallback.apply();
    expect(await fixture.bff.getSession(browser.request().ctx)).toBeNull();
    const replay = await fixture.bff.callback(browser.request(pending.path).ctx);
    expect(replay.status).toBe(409);
    expect(replay.headers.get('location')).toBeNull();
  } finally {
    fixture.stop();
  }
});

test('proxy rejects captured old generation before any upstream call', async () => {
  const fixture = await oidcFixture(),
    browser = new BrowserCookies();
  try {
    const login = await fixture.begin(browser),
      callback = browser.request(login.path);
    await fixture.bff.callback(callback.ctx);
    callback.apply();
    const proxy = crearProxyDeServicio({
      destino: 'http://service.invalid',
      bff: fixture.bff,
      fetchImpl: (async () => {
        throw Error('must not contact upstream');
      }) as unknown as typeof fetch,
    });
    const req = browser.request('/mail/api/messages', { [sessionOwnerHeader]: 'a'.repeat(64) });
    const response = await proxy({
      ...req.ctx,
      params: { ruta: 'messages' },
    } as unknown as Parameters<typeof proxy>[0]);
    expect(response.status).toBe(409);
    expect(response.headers.get(sessionChangedHeader)).toBe('1');
  } finally {
    fixture.stop();
  }
});

test('real BFF refresh preserves owner generation and denies expired unrefreshable sessions', async () => {
  const fixture = await oidcFixture(),
    browser = new BrowserCookies();
  try {
    const login = await fixture.begin(browser),
      callback = browser.request(login.path);
    await fixture.bff.callback(callback.ctx);
    callback.apply();
    const before = (await (await fixture.bff.session(browser.request().ctx)).json()) as {
      owner: { generation: string };
    };
    const refresh = browser.request('/mail/auth/refresh', {
      [sessionOwnerHeader]: before.owner.generation,
    });
    expect((await fixture.bff.refresh(refresh.ctx)).status).toBe(200);
    refresh.apply();
    const after = (await (await fixture.bff.session(browser.request().ctx)).json()) as {
      owner: { generation: string };
    };
    expect(after.owner.generation).toBe(before.owner.generation);
    const expired = session();
    expired.tokens = {
      accessToken: 'expired-fixture',
      expiresAt: Date.now() - 1000,
      tokenType: 'Bearer',
    };
    const old = new BrowserCookies();
    old.values.set('sytue_session', await seal(expired, SECRET));
    expect((await fixture.bff.session(old.request().ctx)).status).toBe(401);
  } finally {
    fixture.stop();
  }
});

test('lost completed cookies never revive the legacy anchor or a prior fallback', async () => {
  const slots = new SessionSlots(SECRET, true, 3600),
    browser = new BrowserCookies();
  const legacy = session();
  delete legacy.generation;
  browser.values.set('__Host-sytue_session', await seal(legacy, SECRET));
  const selected = await initial(slots, browser, 'bob');
  expect(browser.values.has('__Host-sytue_session')).toBe(false);
  browser.values.delete('__Host-sytue_session_slot_' + selected);
  browser.values.delete('__Host-sytue_session_completed_' + selected);
  expect(await slots.read(browser.request().ctx)).toBeNull();
  browser.values.delete(slots.selectionCookie);
  expect(await slots.read(browser.request().ctx)).toBeNull();
});

test('login start bounds obsolete token cookies while preserving the active fallback and tombstones', async () => {
  const slots = new SessionSlots(SECRET, true, 3600),
    browser = new BrowserCookies();
  const original = await initial(slots, browser);
  const logout = browser.request();
  await slots.revokeCurrent(logout.ctx);
  logout.apply();
  for (let index = 0; index < 30; index++) await initial(slots, browser, 'owner-' + index);
  expect(
    [...browser.values.keys()].filter((name) => name.startsWith('__Host-sytue_session_slot_')),
  ).toHaveLength(2);
  expect(browser.values.get('__Host-sytue_session_revoked_' + original)).toBe('1');
  const previous = (await slots.read(browser.request().ctx))!.session.generation;
  const start = browser.request();
  await slots.begin(start.ctx);
  start.apply();
  expect((await slots.read(browser.request().ctx))?.session.generation).toBe(previous);
});

test('a session over the 4096-byte cookie limit is split, read back, shrunk and revoked whole', async () => {
  const slots = new SessionSlots(SECRET, true, 3600),
    browser = new BrowserCookies();
  const login = browser.request();
  const selected = await slots.begin(login.ctx);
  const id = selected.candidate.id;
  // Tokens del tamaño de los de Sytue ID con roles y buzones: la sesión sellada pasa de 4 KB.
  const grande = session('alice', 'acme', id);
  grande.tokens = { ...grande.tokens, accessToken: 'a'.repeat(3000), idToken: 'i'.repeat(2500) };
  await slots.complete(login.ctx, id, grande);
  login.apply();
  const nombres = [...browser.values.keys()].filter((n) => n.includes('slot_' + id));
  expect(nombres.length).toBeGreaterThan(1);
  for (const n of nombres) expect(n.length + browser.values.get(n)!.length).toBeLessThan(4096);
  expect((await slots.read(browser.request().ctx))?.session.tokens.idToken).toBe(
    grande.tokens.idToken,
  );

  // Un refresco con tokens más cortos no puede dejar trozos viejos que corrompan la sesión.
  const refresh = browser.request();
  const actual = await slots.read(refresh.ctx);
  await slots.persist(refresh.ctx, actual!, {
    ...grande,
    tokens: { ...grande.tokens, accessToken: 'b', idToken: 'c' },
  });
  refresh.apply();
  expect([...browser.values.keys()].filter((n) => n.includes('slot_' + id))).toHaveLength(1);
  expect((await slots.read(browser.request().ctx))?.session.tokens.accessToken).toBe('b');

  // Y el cierre de sesión borra todos los trozos.
  const grande2 = browser.request();
  await slots.persist(grande2.ctx, (await slots.read(grande2.ctx))!, grande);
  grande2.apply();
  const logout = browser.request();
  await slots.revokeCurrent(logout.ctx);
  logout.apply();
  expect([...browser.values.keys()].filter((n) => n.includes('slot_' + id))).toHaveLength(0);
  expect(await slots.read(browser.request().ctx)).toBeNull();
});
