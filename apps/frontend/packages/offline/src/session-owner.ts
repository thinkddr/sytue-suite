/* SPDX-License-Identifier: Apache-2.0 */
import {
  isSessionOwnerDescriptor,
  sessionChangedHeader,
  sessionOwnerHeader,
  type SessionOwnerDescriptor,
} from '@sytue/config/session-owner';
import { withBase } from '@sytue/config/base-path';

export interface SessionOwnerExpected {
  readonly actor: string;
  readonly tenant: string;
  readonly clientID?: string;
}
export class SessionOwnerError extends Error {
  constructor(readonly code: 'session_changed' | 'session_owner_mismatch' | 'session_unavailable') {
    super(code);
    this.name = 'SessionOwnerError';
  }
}
/** Created only by a fresh fixed BFF session response. Public JSON metadata,
 * structural lookalikes and a new login cannot replace this captured owner. */
export interface OpaqueSessionOwner {
  readonly signal: AbortSignal;
  readonly fetch: typeof globalThis.fetch;
  revalidate: (signal?: AbortSignal) => Promise<void>;
  acquire: (signal?: AbortSignal) => Promise<() => void>;
  /** Abort IO now; retain leases until their actual Go/host cleanup releases. */
  closeAndDrain: () => Promise<void>;
}
interface OwnerRecord {
  readonly descriptor: Readonly<SessionOwnerDescriptor>;
  readonly owner: OpaqueSessionOwner;
  assertOpen: () => void;
}
const owners = new WeakMap<object, OwnerRecord>();
const mismatch = (): never => {
  throw new SessionOwnerError('session_owner_mismatch');
};
function expectedOwner(value: SessionOwnerExpected): Readonly<SessionOwnerExpected> {
  if (
    !value ||
    typeof value.actor !== 'string' ||
    !value.actor ||
    typeof value.tenant !== 'string' ||
    !value.tenant ||
    (value.clientID !== undefined && (typeof value.clientID !== 'string' || !value.clientID))
  )
    return mismatch();
  return Object.freeze({
    actor: value.actor,
    tenant: value.tenant,
    ...(value.clientID === undefined ? {} : { clientID: value.clientID }),
  });
}
function matches(owner: SessionOwnerDescriptor, expected: SessionOwnerExpected): boolean {
  return (
    owner.actor === expected.actor &&
    owner.tenant === expected.tenant &&
    (expected.clientID === undefined || owner.client_id === expected.clientID)
  );
}
/** Suite factories validate the real WeakMap capability against their own SDK
 * scope. No descriptor, cast, copied method or callback can mint authority. */
export function verifySessionOwner(
  value: unknown,
  expected: SessionOwnerExpected,
): OpaqueSessionOwner {
  if (!value || typeof value !== 'object') return mismatch();
  const record = owners.get(value);
  if (!record || !matches(record.descriptor, expectedOwner(expected))) return mismatch();
  record.assertOpen();
  return record.owner;
}
async function responseOwner(response: Response): Promise<Readonly<SessionOwnerDescriptor>> {
  if (!response.ok || response.redirected) throw new SessionOwnerError('session_unavailable');
  if (!response.body) return mismatch();
  const reader = response.body.getReader(),
    parts: Uint8Array[] = [];
  let size = 0;
  try {
    while (true) {
      const next = await reader.read();
      if (next.done) break;
      size += next.value.byteLength;
      if (size > 256 << 10) {
        next.value.fill(0);
        return mismatch();
      }
      parts.push(next.value);
    }
    const raw = new Uint8Array(size);
    try {
      let offset = 0;
      for (const part of parts) {
        raw.set(part, offset);
        offset += part.byteLength;
      }
      const value: unknown = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(raw));
      if (!value || typeof value !== 'object' || Array.isArray(value)) return mismatch();
      const session = value as {
        authenticated?: unknown;
        user?: unknown;
        tenant?: unknown;
        owner?: unknown;
      };
      if (
        session.authenticated !== true ||
        !isSessionOwnerDescriptor(session.owner) ||
        !session.user ||
        typeof session.user !== 'object' ||
        Array.isArray(session.user) ||
        (session.user as { id?: unknown }).id !== session.owner.actor ||
        session.tenant !== session.owner.tenant
      )
        return mismatch();
      return Object.freeze({ ...session.owner });
    } catch (error) {
      if (error instanceof SessionOwnerError) throw error;
      return mismatch();
    } finally {
      raw.fill(0);
    }
  } finally {
    for (const part of parts) part.fill(0);
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}
export async function openSessionOwner(
  expectedInput: SessionOwnerExpected,
  basePath = '/',
  lifetime?: AbortSignal,
): Promise<OpaqueSessionOwner> {
  const expected = expectedOwner(expectedInput),
    origin = globalThis.location.origin,
    sessionURL = new URL(withBase(basePath, '/auth/session'), origin);
  if (
    sessionURL.origin !== origin ||
    sessionURL.search ||
    sessionURL.hash ||
    sessionURL.username ||
    sessionURL.password ||
    !sessionURL.pathname.endsWith('/auth/session')
  )
    return mismatch();
  const base = sessionURL.pathname.slice(0, -'/auth/session'.length),
    api = base + '/api';
  const send = globalThis.fetch,
    controller = new AbortController(),
    active = new Set<Promise<unknown>>(),
    leases = new Set<object>();
  let closed = false,
    closePromise: Promise<void> | undefined,
    emptied: (() => void) | undefined;
  const revoke = () => {
    closed = true;
    if (!controller.signal.aborted) controller.abort(new SessionOwnerError('session_changed'));
  };
  const abort = () => revoke();
  lifetime?.addEventListener('abort', abort, { once: true });
  if (lifetime?.aborted) abort();
  const check = (signal?: AbortSignal) => {
    controller.signal.throwIfAborted();
    signal?.throwIfAborted();
    if (closed) throw new SessionOwnerError('session_changed');
  };
  const tracked = <T>(run: () => Promise<T>): Promise<T> => {
    const work = run();
    active.add(work);
    void work.then(
      () => active.delete(work),
      () => active.delete(work),
    );
    return work;
  };
  const responseChanged = async (response: Response, session: boolean): Promise<void> => {
    if (
      response.status !== 401 &&
      !(response.status === 409 && (session || response.headers.get(sessionChangedHeader) === '1'))
    )
      return;
    revoke();
    await response.body?.cancel().catch(() => {});
    throw new SessionOwnerError('session_changed');
  };
  const initial = await (async () => {
    let response: Response | undefined;
    try {
      check();
      response = await send.call(globalThis, sessionURL, {
        method: 'GET',
        credentials: 'same-origin',
        redirect: 'error',
        cache: 'no-store',
        headers: { Accept: 'application/json' },
        signal: controller.signal,
      });
      await responseChanged(response, true);
      check();
      const descriptor = await responseOwner(response);
      check();
      if (!matches(descriptor, expected)) return mismatch();
      return descriptor;
    } catch (error) {
      revoke();
      await response?.body?.cancel().catch(() => {});
      lifetime?.removeEventListener('abort', abort);
      throw error;
    }
  })();
  const revalidate = (signal?: AbortSignal): Promise<void> =>
    tracked(async () => {
      check(signal);
      const response = await send.call(globalThis, sessionURL, {
        method: 'GET',
        credentials: 'same-origin',
        redirect: 'error',
        cache: 'no-store',
        headers: { Accept: 'application/json', [sessionOwnerHeader]: initial.generation },
        signal: signal ? AbortSignal.any([signal, controller.signal]) : controller.signal,
      });
      try {
        await responseChanged(response, true);
        check(signal);
        const current = await responseOwner(response);
        check(signal);
        if (
          current.actor !== initial.actor ||
          current.tenant !== initial.tenant ||
          current.client_id !== initial.client_id ||
          current.generation !== initial.generation
        ) {
          revoke();
          throw new SessionOwnerError('session_changed');
        }
      } catch (error) {
        await response.body?.cancel().catch(() => {});
        if (error instanceof SessionOwnerError && error.code === 'session_owner_mismatch') revoke();
        throw error;
      }
    });
  const dispatch = async (
    input: Parameters<typeof fetch>[0],
    init?: Parameters<typeof fetch>[1],
  ): Promise<Response> => {
    const inherited = init?.signal ?? (input instanceof Request ? input.signal : undefined);
    check(inherited ?? undefined);
    const url = new URL(input instanceof Request ? input.url : String(input), origin);
    if (
      url.origin !== origin ||
      url.username ||
      url.password ||
      url.hash ||
      !(
        url.pathname === api ||
        url.pathname.startsWith(api + '/') ||
        (url.pathname === sessionURL.pathname && !url.search)
      )
    )
      return mismatch();
    const headers = new Headers(
      init?.headers ?? (input instanceof Request ? input.headers : undefined),
    );
    if (headers.has('Authorization') || headers.has('Cookie') || headers.has('Proxy-Authorization'))
      return mismatch();
    headers.set(sessionOwnerHeader, initial.generation);
    const request = new Request(input instanceof Request ? input : url, {
      ...init,
      headers,
      credentials: 'same-origin',
      redirect: 'error',
      cache: 'no-store',
      signal: inherited ? AbortSignal.any([inherited, controller.signal]) : controller.signal,
    });
    const response = await send.call(globalThis, request, {
      credentials: 'same-origin',
      redirect: 'error',
      cache: 'no-store',
      signal: request.signal,
    });
    try {
      await responseChanged(response, url.pathname === sessionURL.pathname);
      check(inherited ?? undefined);
      if (response.redirected) return mismatch();
      return response;
    } catch (error) {
      await response.body?.cancel().catch(() => {});
      throw error;
    }
  };
  const guarded = new Proxy(send, {
    apply: (_target, _receiver, args) =>
      tracked(() => dispatch(...(args as Parameters<typeof fetch>))),
  });
  const owner: OpaqueSessionOwner = Object.freeze({
    signal: controller.signal,
    fetch: guarded,
    revalidate,
    acquire: async (signal?: AbortSignal) => {
      await revalidate(signal);
      check(signal);
      const lease = {};
      leases.add(lease);
      let released = false;
      return () => {
        if (!released) {
          released = true;
          leases.delete(lease);
          if (!leases.size) {
            emptied?.();
            emptied = undefined;
          }
        }
      };
    },
    closeAndDrain: () => {
      if (!closePromise)
        closePromise = (async () => {
          revoke();
          await Promise.allSettled(active);
          if (leases.size)
            await new Promise<void>((resolve) => {
              emptied = resolve;
            });
          lifetime?.removeEventListener('abort', abort);
        })();
      return closePromise;
    },
  });
  owners.set(owner, { descriptor: initial, owner, assertOpen: () => check() });
  return owner;
}
