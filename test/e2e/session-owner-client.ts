/* SPDX-License-Identifier: Apache-2.0 */
import {
  openSessionOwner,
  verifySessionOwner,
} from '../../apps/frontend/packages/offline/src/session-owner.js';
import { sessionOwnerHeader } from '../../apps/frontend/packages/config/src/session-owner.js';
declare global {
  interface Window {
    sessionOwnerProbe: { ok: boolean; stage: string; credentials?: boolean };
  }
}
const state = (stage: string) => {
  window.sessionOwnerProbe = { ok: false, stage };
};
function check(value: unknown, message: string): asserts value {
  if (!value) throw Error(message);
}
async function run(): Promise<void> {
  state('opening');
  const expected = { actor: 'owner', tenant: 'tenant', clientID: 'mail' },
    send = globalThis.fetch;
  let credentials = true;
  globalThis.fetch = new Proxy(send, {
    apply: (target, receiver, args) => {
      const request = new Request(...(args as Parameters<typeof fetch>));
      credentials &&=
        request.credentials === 'same-origin' &&
        request.redirect === 'error' &&
        request.cache === 'no-store';
      return Reflect.apply(target, receiver, args);
    },
  });
  let owner: Awaited<ReturnType<typeof openSessionOwner>> | undefined, fresh: typeof owner;
  try {
    owner = await openSessionOwner(expected, '/mail');
    check(verifySessionOwner(owner, expected) === owner, 'opaque-real-owner');
    let denied = false;
    try {
      verifySessionOwner({ ...owner }, expected);
    } catch {
      denied = true;
    }
    check(denied, 'fake-owner-grant');
    await owner.revalidate();
    const valid = await owner.fetch('/mail/api/check', {
      headers: { [sessionOwnerHeader]: 'b'.repeat(64) },
    });
    check(valid.ok, 'captured-header-not-overridden');
    await valid.body?.cancel();
    const domain = await owner.fetch('/mail/api/domain-conflict');
    check(domain.status === 409 && !owner.signal.aborted, 'domain409-invalidated-owner');
    await domain.body?.cancel();
    await send('/test/same-actor-login', { method: 'POST' });
    denied = false;
    try {
      await owner.fetch('/mail/api/check');
    } catch {
      denied = true;
    }
    check(denied && owner.signal.aborted, 'same-actor-new-generation-adopted');
    await owner.closeAndDrain();
    fresh = await openSessionOwner(expected, '/mail');
    const release = await fresh.acquire();
    let drained = false;
    const closing = fresh.closeAndDrain().then(() => {
      drained = true;
    });
    await Promise.resolve();
    check(fresh.signal.aborted && !drained, 'leases-released-before-domain-cleanup');
    release();
    release();
    await closing;
    check(drained, 'lease-drain-never-completed');
    await send('/test/other-actor-login', { method: 'POST' });
    denied = false;
    try {
      await openSessionOwner(expected, '/mail');
    } catch {
      denied = true;
    }
    check(denied, 'new-account-silently-adopted');
    check(credentials, 'browser-fetch-options');
    window.sessionOwnerProbe = { ok: true, stage: 'complete', credentials };
  } finally {
    await owner?.closeAndDrain();
    await fresh?.closeAndDrain();
    globalThis.fetch = send;
  }
}
void run().catch((error) => {
  window.sessionOwnerProbe = { ok: false, stage: 'failed:' + String(error?.message ?? 'unknown') };
});
