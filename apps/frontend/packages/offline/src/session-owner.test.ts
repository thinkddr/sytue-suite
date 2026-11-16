/* SPDX-License-Identifier: Apache-2.0 */
import { afterEach, expect, test } from 'bun:test';
import { openSessionOwner, verifySessionOwner, SessionOwnerError } from './session-owner.js';
import { sessionChangedHeader, sessionOwnerHeader } from '@sytue/config/session-owner';
const originalFetch = globalThis.fetch,
  locationDescriptor = Object.getOwnPropertyDescriptor(globalThis, 'location');
afterEach(() => {
  globalThis.fetch = originalFetch;
  if (locationDescriptor) Object.defineProperty(globalThis, 'location', locationDescriptor);
  else Reflect.deleteProperty(globalThis, 'location');
});
const expected = { actor: 'owner', tenant: 'tenant', clientID: 'mail' };
const descriptor = {
  v: 1,
  actor: expected.actor,
  tenant: expected.tenant,
  client_id: expected.clientID,
  generation: 'a'.repeat(64),
};
function server(
  action: (request: Request, init?: RequestInit) => Response | Promise<Response>,
): void {
  Object.defineProperty(globalThis, 'location', {
    configurable: true,
    value: new URL('https://suite.test/mail'),
  });
  globalThis.fetch = new Proxy(originalFetch, {
    apply: (_target, _receiver, args) =>
      action(
        new Request(...(args as Parameters<typeof fetch>)),
        args[1] as RequestInit | undefined,
      ),
  });
}
function session(owner = descriptor): Response {
  return Response.json({
    authenticated: true,
    user: { id: owner.actor },
    tenant: owner.tenant,
    owner,
  });
}
test('only the fixed fresh authenticated BFF creates an opaque capability; refresh preserves its immutable owner', async () => {
  const seen: Request[] = [],
    options: (RequestInit | undefined)[] = [];
  server((request, init) => {
    seen.push(request);
    options.push(init);
    if (new URL(request.url).pathname === '/mail/auth/session') return session();
    return Response.json({ ok: true });
  });
  const owner = await openSessionOwner(expected, '/mail');
  expect(verifySessionOwner(owner, expected)).toBe(owner);
  for (const fake of [{ ...owner }, Object.create(owner), { owner: descriptor }]) {
    expect(() => verifySessionOwner(fake, expected)).toThrow('session_owner_mismatch');
  }
  expect(() => verifySessionOwner(owner, { ...expected, actor: 'other' })).toThrow(
    'session_owner_mismatch',
  );
  await owner.revalidate();
  const caller = new Headers({ [sessionOwnerHeader]: 'b'.repeat(64) });
  await owner.fetch('/mail/api/private-storage/migration', { headers: caller });
  expect(caller.get(sessionOwnerHeader)).toBe('b'.repeat(64));
  expect(seen.map((request) => new URL(request.url).pathname)).toEqual([
    '/mail/auth/session',
    '/mail/auth/session',
    '/mail/api/private-storage/migration',
  ]);
  expect(seen[0]!.headers.has(sessionOwnerHeader)).toBe(false);
  for (const request of seen.slice(1)) {
    expect(request.headers.get(sessionOwnerHeader)).toBe(descriptor.generation);
    expect(request.redirect).toBe('error');
    expect(request.cache).toBe('no-store');
  }
  for (const init of options) {
    expect(init?.credentials).toBe('same-origin');
  }
  await owner.closeAndDrain();
  expect(owner.signal.aborted).toBe(true);
});
test('session replacement revokes the old owner and can never adopt another account or generation', async () => {
  let current = descriptor;
  server(() => session(current));
  const owner = await openSessionOwner(expected, '/mail');
  current = { ...descriptor, actor: 'new-owner', generation: 'c'.repeat(64) };
  await expect(owner.revalidate()).rejects.toThrow('session_changed');
  expect(owner.signal.aborted).toBe(true);
  expect(() => verifySessionOwner(owner, expected)).toThrow();
  await expect(owner.fetch('/mail/api/private-storage/migration')).rejects.toThrow();
  await owner.closeAndDrain();
});
test('generation mismatch or unauthorized API revokes; domain conflicts do not invalidate the session', async () => {
  let mode: 'domain' | 'changed' | 'unauthorized' = 'domain';
  server((request) => {
    if (new URL(request.url).pathname.endsWith('/auth/session')) return session();
    return Response.json(
      { error: { code: mode === 'changed' ? 'session_changed' : 'conflict' } },
      {
        status: mode === 'unauthorized' ? 401 : 409,
        headers: mode === 'changed' ? { [sessionChangedHeader]: '1' } : {},
      },
    );
  });
  const owner = await openSessionOwner(expected, '/mail');
  expect((await owner.fetch('/mail/api/mutation')).status).toBe(409);
  expect(owner.signal.aborted).toBe(false);
  mode = 'changed';
  await expect(owner.fetch('/mail/api/mutation')).rejects.toThrow('session_changed');
  expect(owner.signal.aborted).toBe(true);
  await owner.closeAndDrain();
  mode = 'unauthorized';
  const next = await openSessionOwner(expected, '/mail');
  await expect(next.fetch('/mail/api/mutation')).rejects.toThrow('session_changed');
  await next.closeAndDrain();
});
test('origin, base, auth endpoint and credential header choices are denied before network IO', async () => {
  let calls = 0;
  server(() => {
    calls++;
    return session();
  });
  const owner = await openSessionOwner(expected, '/mail');
  for (const url of [
    'https://outside.test/mail/api/private',
    '/drive/api/private',
    '/mail/api-other/private',
    '/mail/auth/token',
    '/mail/auth/session?owner=other',
  ]) {
    await expect(owner.fetch(url)).rejects.toBeInstanceOf(SessionOwnerError);
  }
  await expect(
    owner.fetch('/mail/api/private', { headers: { Authorization: 'forbidden-synthetic' } }),
  ).rejects.toThrow('session_owner_mismatch');
  expect(calls).toBe(1);
  expect(owner.signal.aborted).toBe(false);
  await owner.closeAndDrain();
});
test('closing aborts immediately but joins real late IO and retains work leases until explicit cleanup release', async () => {
  let finish!: (value: Response) => void,
    entered!: () => void,
    canceled = 0;
  const ready = new Promise<void>((resolve) => {
    entered = resolve;
  });
  server((request) => {
    if (new URL(request.url).pathname.endsWith('/auth/session')) return session();
    entered();
    return new Promise<Response>((resolve) => {
      finish = resolve;
    });
  });
  const owner = await openSessionOwner(expected, '/mail'),
    release = await owner.acquire();
  const request = owner.fetch('/mail/api/private').then(
    () => false,
    () => true,
  );
  await ready;
  let drained = false;
  const closed = owner.closeAndDrain().then(() => {
    drained = true;
  });
  expect(owner.signal.aborted).toBe(true);
  await Promise.resolve();
  expect(drained).toBe(false);
  finish(
    new Response(
      new ReadableStream({
        cancel() {
          canceled++;
        },
      }),
    ),
  );
  expect(await request).toBe(true);
  expect(canceled).toBe(1);
  await Promise.resolve();
  expect(drained).toBe(false);
  release();
  release();
  await closed;
  expect(drained).toBe(true);
});
test('malformed or foreign public session JSON cannot mint an owner', async () => {
  server(() => session({ ...descriptor, actor: 'other' }));
  await expect(openSessionOwner(expected, '/mail')).rejects.toThrow('session_owner_mismatch');
  server(() =>
    Response.json({
      authenticated: true,
      user: { sub: 'owner' },
      tenant: 'tenant',
      owner: { ...descriptor, generation: 'not-a-generation' },
    }),
  );
  await expect(openSessionOwner(expected, '/mail')).rejects.toThrow('session_owner_mismatch');
});

test('a second login by the same actor still revokes the original generation', async () => {
  let current = descriptor;
  server(() => session(current));
  const owner = await openSessionOwner(expected, '/mail');
  current = { ...descriptor, generation: 'd'.repeat(64) };
  await expect(owner.revalidate()).rejects.toThrow('session_changed');
  expect(owner.signal.aborted).toBe(true);
  await owner.closeAndDrain();
});

test('the owner actor must match the real BFF user.id, never an alternative user.sub', async () => {
  server(() =>
    Response.json({
      authenticated: true,
      user: { id: 'other', sub: descriptor.actor },
      tenant: descriptor.tenant,
      owner: descriptor,
    }),
  );
  await expect(openSessionOwner(expected, '/mail')).rejects.toThrow('session_owner_mismatch');
});
