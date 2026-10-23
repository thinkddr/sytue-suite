/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { HubClient, bearer, type NotificationPref } from '@sytue/api-clients';
import {
  DmNotificationPreference,
  dmNotificationsEnabled,
  notificationOwnerMatches,
  type DmNotificationState,
} from './dmNotifications.js';
import { dmNotificationResponse } from './dmNotificationsServer.js';
import type { ServerSession } from '@sytue/auth/server';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

test('exact Hub rule overrides source silence; unrelated rules never mute direct messages', () => {
  expect(dmNotificationsEnabled([])).toBe(true);
  expect(dmNotificationsEnabled([{ source: 'mail', muted: true }])).toBe(true);
  expect(dmNotificationsEnabled([{ source: 'rooms', kind: 'rooms.mention', muted: true }])).toBe(
    true,
  );
  expect(dmNotificationsEnabled([{ source: 'rooms', muted: true }])).toBe(false);
  expect(
    dmNotificationsEnabled([
      { source: 'rooms', muted: true },
      { source: 'rooms', kind: 'rooms.message', muted: false },
    ]),
  ).toBe(true);
  expect(
    dmNotificationsEnabled([
      { source: 'rooms', muted: false },
      { source: 'rooms', kind: 'rooms.message', muted: true },
    ]),
  ).toBe(false);
});

test('unknown load suppresses changes; failures require explicit read before another write', async () => {
  let fail = true;
  let stored = true;
  let writes = 0;
  const p = new DmNotificationPreference(
    {
      async read() {
        if (fail) throw new Error('offline');
        return stored;
      },
      async write(enabled) {
        writes++;
        stored = enabled;
        throw new Error('lost acknowledgement');
      },
    },
    () => {},
  );
  await p.toggle();
  expect(writes).toBe(0);
  await p.refresh();
  expect(p.state.error).toBe(true);
  await p.toggle();
  expect(writes).toBe(0);
  fail = false;
  await p.refresh();
  await p.toggle();
  expect(writes).toBe(1);
  expect(p.state.error).toBe(true);
  await p.toggle();
  expect(writes).toBe(1);
  expect(p.state.error).toBe(true);
  await p.refresh();
  expect(p.state.enabled).toBe(false);
  expect(p.state.error).toBe(false);
});

test('a stale label adopts the newer account rule instead of writing its inverse', async () => {
  let enabled = true;
  let writes = 0;
  const p = new DmNotificationPreference(
    {
      read: async () => enabled,
      write: async () => {
        writes++;
      },
    },
    () => {},
  );
  await p.refresh();
  enabled = false;
  await p.toggle();
  expect(p.state.enabled).toBe(false);
  expect(writes).toBe(0);
});

test('double click sends one write and reads the authoritative value afterwards', async () => {
  const gate = deferred<void>();
  let enabled = true;
  let writes = 0;
  const p = new DmNotificationPreference(
    {
      read: async () => enabled,
      async write(value) {
        writes++;
        await gate.promise;
        enabled = value;
      },
    },
    () => {},
  );
  await p.refresh();
  const first = p.toggle();
  await p.toggle();
  await Promise.resolve();
  expect(writes).toBe(1);
  gate.resolve();
  await first;
  expect(p.state).toEqual({ enabled: false, loading: false, saving: false, error: false });
});

test('older reads cannot replace the newest rule even when transport ignores abort', async () => {
  const first = deferred<boolean>();
  let count = 0;
  const p = new DmNotificationPreference(
    { read: () => (++count === 1 ? first.promise : Promise.resolve(false)), write: async () => {} },
    () => {},
  );
  const old = p.refresh();
  await p.refresh();
  first.resolve(true);
  await old;
  expect(p.state.enabled).toBe(false);
});

test('logout/disposal while revalidating stops the account write and all state publication', async () => {
  const pending = deferred<boolean>();
  let calls = 0;
  let writes = 0;
  const states: DmNotificationState[] = [];
  const p = new DmNotificationPreference(
    {
      read: () => (++calls === 1 ? Promise.resolve(true) : pending.promise),
      write: async () => {
        writes++;
      },
    },
    (state) => states.push(state),
  );
  await p.refresh();
  const change = p.toggle();
  p.dispose();
  const n = states.length;
  pending.resolve(true);
  await change;
  await p.refresh();
  expect(writes).toBe(0);
  expect(states.length).toBe(n);
});

const session = {
  user: { id: 'owner', tenant: 'org' },
  accessToken: 'server-test-token',
} as ServerSession;
function req(
  method = 'GET',
  body?: unknown,
  scope: { id: string; tenant: string } = { id: 'owner', tenant: 'org' },
  origin = 'https://rooms.example',
) {
  return new Request('https://rooms.example/rooms/api/dm-notifications', {
    method,
    headers: {
      'x-sytue-preference-owner': encodeURIComponent(scope.id),
      'x-sytue-preference-tenant': encodeURIComponent(scope.tenant),
      origin,
      'content-type': 'application/json',
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
}
function hubFixture() {
  let prefs: NotificationPref[] = [
    { source: 'mail', muted: true },
    { source: 'rooms', kind: 'rooms.mention', muted: true },
  ];
  const writes: NotificationPref[] = [];
  return {
    writes,
    hub: {
      async notificationPrefs() {
        return prefs;
      },
      async setNotificationPref(pref: NotificationPref) {
        writes.push(pref);
        prefs = [...prefs.filter((p) => p.source !== pref.source || p.kind !== pref.kind), pref];
        return pref;
      },
    },
  };
}

test('BFF uses only its authenticated owner/tenant and rejects an old screen before contacting Hub', async () => {
  expect(notificationOwnerMatches({ id: '', tenant: '' }, { id: '', tenant: '' })).toBe(false);
  const fixture = hubFixture();
  for (const scope of [
    { id: 'other', tenant: 'org' },
    { id: 'owner', tenant: 'other' },
    { id: 'owner', tenant: '' },
  ]) {
    expect(
      (await dmNotificationResponse(req('PUT', { enabled: false }, scope), session, fixture.hub))
        .status,
    ).toBe(409);
  }
  expect((await dmNotificationResponse(req(), null, fixture.hub)).status).toBe(401);
  expect(fixture.writes).toEqual([]);
});

test('BFF writes exactly the DM rule, without replacing source/mention/mail rules', async () => {
  const fixture = hubFixture();
  expect(await (await dmNotificationResponse(req(), session, fixture.hub)).json()).toEqual({
    enabled: true,
  });
  expect(
    (
      await dmNotificationResponse(
        req('PUT', { enabled: false, source: 'mail', kind: '' }),
        session,
        fixture.hub,
      )
    ).status,
  ).toBe(200);
  expect(fixture.writes).toEqual([{ source: 'rooms', kind: 'rooms.message', muted: true }]);
  expect(await fixture.hub.notificationPrefs()).toEqual([
    { source: 'mail', muted: true },
    { source: 'rooms', kind: 'rooms.mention', muted: true },
    { source: 'rooms', kind: 'rooms.message', muted: true },
  ]);
});

test('BFF refuses cross-origin, wrong-typed booleans and errors without exposing credential details', async () => {
  const fixture = hubFixture();
  expect(
    (
      await dmNotificationResponse(
        req('PUT', { enabled: false }, undefined, 'https://foreign.example'),
        session,
        fixture.hub,
      )
    ).status,
  ).toBe(403);
  for (const body of [{}, { enabled: 'false' }, null, []])
    expect((await dmNotificationResponse(req('PUT', body), session, fixture.hub)).status).toBe(400);
  const response = await dmNotificationResponse(req(), session, {
    ...fixture.hub,
    notificationPrefs: async () => {
      throw new Error('private upstream detail');
    },
  });
  expect(response.status).toBe(502);
  expect(await response.text()).not.toContain('private upstream detail');
  expect(response.headers.get('cache-control')).toBe('no-store');
  expect(fixture.writes).toEqual([]);
});

test('the real shared Hub SDK uses the canonical endpoint and captured server session', async () => {
  const seen: { method: string; url: string; auth: string | null; body: string | null }[] = [];
  const hub = new HubClient({
    baseUrl: 'https://hub.example',
    auth: bearer(session.accessToken),
    fetchImpl: (async (url: string | URL | Request, options?: RequestInit) => {
      const request = new Request(url, options);
      seen.push({
        method: request.method,
        url: request.url,
        auth: request.headers.get('authorization'),
        body: request.method === 'PUT' ? await request.text() : null,
      });
      return Response.json(
        request.method === 'GET'
          ? { prefs: [] }
          : { source: 'rooms', kind: 'rooms.message', muted: true },
      );
    }) as typeof fetch,
  });
  expect((await dmNotificationResponse(req(), session, hub)).status).toBe(200);
  expect((await dmNotificationResponse(req('PUT', { enabled: false }), session, hub)).status).toBe(
    200,
  );
  expect(seen).toEqual([
    {
      method: 'GET',
      url: 'https://hub.example/api/v1/notifications/prefs',
      auth: 'Bearer server-test-token',
      body: null,
    },
    {
      method: 'PUT',
      url: 'https://hub.example/api/v1/notifications/prefs',
      auth: 'Bearer server-test-token',
      body: JSON.stringify({ source: 'rooms', kind: 'rooms.message', muted: true }),
    },
  ]);
});
