/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import type { APIContext } from 'astro';
import type { SytueSession } from '../types.js';
import type { Bff } from './bff.js';
import { rutaDeRetornoDeApp } from './consentimiento.js';

function session(personal = false, plan?: string): SytueSession {
  return {
    user: {
      id: 'u1',
      email: 'owner@example.test',
      name: 'Owner',
      roles: ['owner'],
      tenant: 'acme',
      ...(personal ? { orgKind: 'personal', ...(plan ? { plan } : {}) } : {}),
    },
    tokens: { accessToken: 'session-access-token', idToken: '', refreshToken: '', expiresAt: 0 },
    clientId: 'app',
  } as SytueSession;
}
function bff(value: SytueSession | null, basePath: string): Bff {
  return { basePath, getSession: async () => value } as unknown as Bff;
}
function context(path: string): APIContext {
  const url = new URL(path, 'https://app.sytue.test');
  return {
    url,
    request: new Request(url),
    cookies: { get: () => undefined, set: () => {}, delete: () => {} },
  } as unknown as APIContext;
}

test('linked OAuth returns to its own app session and removes provider codes', async () => {
  for (const product of ['calendar', 'contacts', 'notes'] as const) {
    const section = `external-${product}` as const;
    const route = rutaDeRetornoDeApp({
      bff: bff(session(true), `/${product}`),
      section,
      finish: async () => ({ section, instance: 'exact-account', error: 'denied' }),
    });
    const response = await route(
      context(
        `/${product}/api/linked-services-finish?state=sealed&code=secret&return_to=https://evil.test`,
      ),
    );
    const target = new URL(response.headers.get('location')!, 'https://app.sytue.test');
    expect(target.pathname).toBe(`/${product}/acme/`);
    expect(target.searchParams.get('connect-linked')).toBe('1');
    expect(target.searchParams.get('instance')).toBe('exact-account');
    expect(target.searchParams.get('oauth_error')).toBe('denied');
    for (const key of ['state', 'code', 'return_to'])
      expect(target.searchParams.has(key)).toBe(false);
  }
});

test('app consent returns free personal accounts directly to their app, without the Admin plan guard', async () => {
  for (const section of ['storage', 'rooms', 'external-chat'] as const) {
    for (const plan of ['personal_free', '']) {
      const base = section === 'storage' ? '/drive' : '/rooms';
      const ctx = context(
        `${base}/api/finish?state=sealed&code=provider-code&return_to=https://evil.test`,
      );
      let finishes = 0;
      const route = rutaDeRetornoDeApp({
        bff: bff(session(true, plan), base),
        section,
        finish: async (token, query) => {
          finishes += 1;
          expect(token).toBe('session-access-token');
          expect(query.get('state')).toBe('sealed');
          expect(query.get('code')).toBe('provider-code');
          return { section, instance: 'account-42' };
        },
      });
      const response = await route(ctx);
      const target = new URL(response.headers.get('location')!, ctx.url);
      expect(response.status).toBe(302);
      expect(finishes).toBe(1);
      expect(target.origin).toBe(ctx.url.origin);
      expect(target.pathname).toBe(`${base}/acme/`);
      expect(
        target.searchParams.get(
          section === 'storage'
            ? 'connect-storage'
            : section === 'external-chat'
              ? 'connect-external'
              : 'connect-app',
        ),
      ).toBe('1');
      expect(target.searchParams.get('instance')).toBe('account-42');
      for (const key of ['code', 'state', 'return_to', '__org'])
        expect(target.searchParams.has(key)).toBe(false);
      expect(response.headers.get('cache-control')).toBe('no-store');
      expect(response.headers.get('referrer-policy')).toBe('no-referrer');
    }
  }
});

test('app consent never exchanges a code without a session and preserves its canonical login return', async () => {
  let finishes = 0;
  const route = rutaDeRetornoDeApp({
    bff: bff(null, '/rooms'),
    section: 'rooms',
    finish: async () => {
      finishes += 1;
      return { section: 'rooms' };
    },
  });
  const ctx = context('/rooms/api/app-connections-finish?state=sealed&code=code&__org=acme');
  const response = await route(ctx);
  const login = new URL(response.headers.get('location')!, ctx.url);
  expect(response.status).toBe(302);
  expect(login.pathname).toBe('/rooms/auth/login');
  expect(login.searchParams.get('returnTo')).toBe(
    '/rooms/acme/api/app-connections-finish?state=sealed&code=code',
  );
  expect(finishes).toBe(0);
});

test('a result sealed for another app cannot choose the destination or expose its instance', async () => {
  const route = rutaDeRetornoDeApp({
    bff: bff(session(), '/rooms'),
    section: 'rooms',
    finish: async () => ({ section: 'storage', instance: 'foreign-account', error: 'denied' }),
  });
  const ctx = context('/rooms/api/app-connections-finish?instance=attacker&connect-storage=1');
  const response = await route(ctx);
  const target = new URL(response.headers.get('location')!, ctx.url);
  expect(target.pathname).toBe('/rooms/acme/');
  expect([...target.searchParams]).toEqual([
    ['connect-app', '1'],
    ['oauth_error', 'state'],
  ]);
});

test('provider denial and failed exchanges return to a local retry without leaking exception text', async () => {
  for (const failure of ['denied', 'exchange'] as const) {
    const route = rutaDeRetornoDeApp({
      bff: bff(session(), '/drive'),
      section: 'storage',
      finish: async () => {
        if (failure === 'exchange') throw new Error('secret provider response');
        return { section: 'storage', instance: 'account-42', error: 'denied' };
      },
    });
    const ctx = context('/drive/api/storage-external-finish?return_to=//evil.test');
    const response = await route(ctx);
    const target = new URL(response.headers.get('location')!, ctx.url);
    expect(target.pathname).toBe('/drive/acme/');
    expect(target.searchParams.get('connect-storage')).toBe('1');
    expect(target.searchParams.get('oauth_error')).toBe(failure);
    expect(target.href).not.toContain('evil.test');
    expect(target.href).not.toContain('secret');
    expect(await response.text()).toBe('');
  }
});

test('meeting return is derived from the sealed gateway section, never the callback query', async () => {
  for (const actual of ['external-chat-meeting', 'external-chat']) {
    const ctx = context(
      '/rooms/api/external-conversations-finish?state=sealed&code=provider-code&meeting=1',
    );
    const route = rutaDeRetornoDeApp({
      bff: bff(session(true, 'personal_free'), '/rooms'),
      section: 'external-chat',
      finish: async () => ({ section: actual, instance: 'same-account' }),
    });
    const response = await route(ctx);
    const target = new URL(response.headers.get('location')!, ctx.url);
    expect(target.searchParams.get('instance')).toBe('same-account');
    expect(target.searchParams.get('meeting')).toBe(
      actual === 'external-chat-meeting' ? '1' : null,
    );
  }
});
