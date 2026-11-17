/* SPDX-License-Identifier: Apache-2.0 */
/**
 * The web custody against the REAL Sytue ID bridge (services/id/internal/http/e2e),
 * run in a window of its own as the hidden iframe would: the app asks for operations,
 * gets its account's recovery key or wrapper back, and never sees the unlock key. Nor
 * does it keep anything: the custody only talks to its BFF and to the bridge.
 */
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { BrowserKeyCustody } from './custody-browser.js';
import { MatrixCustodyError } from './custody.js';
import { fromBase64, type RecoveryWrapper } from './recovery-wrap.js';
import { forgetLegacyMaterial, unwrapInBridge, wrapInBridge } from './unlock-bridge.js';

const account = '@sytue_storage_drive_acme_0f3c:sytue.eu';
const subject = '0f3c';
const key = 'EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP';
const unlockKey = new Uint8Array(32).map((_, i) => 0x20 + i);
const idOrigin = 'https://id.sytue.eu';

/** The ADR 0082 vectors: a password wrapper and a passkey wrapper, and their factors. */
const old = JSON.parse(
  readFileSync(
    join(import.meta.dir, '../../../../../nucleo/recoverywrap/testdata/vectors.json'),
    'utf8',
  ),
) as { name: string; account: string; password?: string; wrapper: RecoveryWrapper }[];
const desbloqueo = JSON.parse(
  readFileSync(
    join(import.meta.dir, '../../../../../nucleo/recoverywrap/testdata/desbloqueo.json'),
    'utf8',
  ),
) as { legacy_password_ikm: string };

const pages = join(import.meta.dir, '../../../../../../services/id/internal/http/e2e');
const script = (name: string) => readFileSync(join(pages, name), 'utf8');
const encodings = (bytes: Uint8Array) => [
  Buffer.from(bytes).toString('base64'),
  Buffer.from(bytes).toString('base64url'),
  Buffer.from(bytes).toString('hex'),
];

function service(options: { putStatus?: number; legacy?: boolean } = {}) {
  const stored: RecoveryWrapper[] = [];
  const requests: { method: string; url: string; body?: string }[] = [];
  const fetchImpl = (async (input: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET';
    requests.push({ method, url: input, ...(init?.body ? { body: String(init.body) } : {}) });
    if (input.includes('/recovery-key'))
      return Response.json({ recovery_key: key }, { status: options.legacy ? 200 : 404 });
    if (method === 'PUT') {
      if ((options.putStatus ?? 200) !== 200) return new Response(null, { status: 503 });
      const wrapper = JSON.parse(String(init!.body)) as RecoveryWrapper;
      const index = stored.findIndex(
        (w) => w.kind === wrapper.kind && w.credential_id === wrapper.credential_id,
      );
      if (index >= 0) stored.splice(index, 1);
      stored.push(wrapper);
      return Response.json({ id: 'id' });
    }
    return Response.json({ wrappers: stored, legacy_escrow: options.legacy === true });
  }) as unknown as typeof fetch;
  return { stored, requests, fetchImpl };
}

type Listener = (event: MessageEvent) => void;
interface Bridge {
  store: { get: (k: string) => Promise<unknown>; put: (k: string, v: unknown) => Promise<void> };
  saveUnlock: (sub: string, key: Uint8Array) => Promise<unknown>;
  legacyKey: (item: Record<string, unknown>, ikm: Uint8Array) => Promise<unknown>;
}

/**
 * The app at `appOrigin` with Sytue ID's real bridge behind its hidden iframes. Every
 * iframe runs the bridge scripts in a window of its own over the one store of
 * `id.<apex>`; `received` is everything the bridge posted to the app.
 */
function sytueID(appOrigin = 'https://drive.sytue.eu') {
  const data = new Map<string, unknown>();
  const store = {
    get: async (k: string) => structuredClone(data.get(k)),
    put: async (k: string, v: unknown) => void data.set(k, structuredClone(v)),
    putIfAbsent: async (k: string, v: unknown) => {
      if (!data.has(k)) data.set(k, structuredClone(v));
      return structuredClone(data.get(k));
    },
    del: async (k: string) => void data.delete(k),
    keys: async () => [...data.keys()],
    clear: async () => data.clear(),
  };
  const received: unknown[] = [];
  const appListeners = new Set<Listener>();
  const win = {
    location: { origin: appOrigin },
    addEventListener: (_: string, fn: Listener) => appListeners.add(fn),
    removeEventListener: (_: string, fn: Listener) => appListeners.delete(fn),
  };
  function iframe() {
    const inside: Listener[] = [];
    const contentWindow = {
      postMessage: (msg: unknown, target: string) => {
        if (target !== idOrigin) return;
        for (const fn of inside)
          fn({ data: structuredClone(msg), origin: appOrigin, source: parent } as MessageEvent);
      },
    };
    const parent = {
      postMessage: (msg: unknown, target: string) => {
        if (target !== appOrigin) return;
        received.push(structuredClone(msg));
        for (const fn of appListeners)
          fn({
            data: structuredClone(msg),
            origin: idOrigin,
            source: contentWindow,
          } as MessageEvent);
      },
    };
    return {
      src: '',
      hidden: false,
      tabIndex: 0,
      setAttribute: () => undefined,
      remove: () => undefined,
      contentWindow,
      load() {
        const bridgeWin: Record<string, unknown> = {
          crypto: globalThis.crypto,
          location: { origin: idOrigin },
          parent,
          addEventListener: (type: string, fn: Listener) => type === 'message' && inside.push(fn),
        };
        new Function('window', script('comun.js'))(bridgeWin);
        (bridgeWin.SytueE2E as { store: unknown }).store = store;
        new Function('window', script('bridge.js'))(bridgeWin);
        const o = new URL(this.src).searchParams.get('o');
        (bridgeWin.SytueE2E as { bridge: (c: unknown, w: unknown) => boolean }).bridge(
          { origin: o },
          bridgeWin,
        );
      },
    };
  }
  const doc = {
    createElement: () => iframe(),
    body: { appendChild: (frame: ReturnType<typeof iframe>) => frame.load() },
  };
  // The Sytue ID side, to set this browser up as a sign-in would.
  const id: Record<string, unknown> = { crypto: globalThis.crypto, location: { origin: idOrigin } };
  new Function('window', script('comun.js'))(id);
  const e2e = id.SytueE2E as Bridge & { store: unknown };
  e2e.store = store;
  const options = (sub: string) => ({
    idOrigin,
    subject: sub,
    win: win as never,
    doc: doc as never,
  });
  return {
    e2e,
    data,
    received,
    bridge: {
      unwrap: (sub: string, input: { account: string; wrappers: readonly RecoveryWrapper[] }) =>
        unwrapInBridge(options(sub), input),
      wrap: (sub: string, input: { account: string; keyID: string; recoveryKey: string }) =>
        wrapInBridge(options(sub), input),
      forgetLegacy: (sub: string) => forgetLegacyMaterial(options(sub)),
    },
  };
}

/** Nothing the app received from Sytue ID is the unlock key, in any form. */
function expectNoUnlockKey(received: unknown[], secrets: Uint8Array[] = [unlockKey]) {
  const walk = (value: unknown): void => {
    if (ArrayBuffer.isView(value) || value instanceof ArrayBuffer || value instanceof CryptoKey)
      throw new Error('the app received key material');
    if (typeof value === 'string') {
      for (const secret of secrets)
        for (const encoded of encodings(secret)) expect(value.includes(encoded)).toBe(false);
    } else if (value && typeof value === 'object') Object.values(value).forEach(walk);
  };
  received.forEach(walk);
}

// The app keeps nothing: any use of the browser's storage during a test fails it.
const touched: string[] = [];
const trap = (name: string) =>
  new Proxy(
    {},
    {
      get: (_, prop) => {
        touched.push(`${name}.${String(prop)}`);
        throw new Error(`the custody used ${name}`);
      },
    },
  );
const globals = globalThis as Record<string, unknown>;
const saved: Record<string, unknown> = {};
beforeEach(() => {
  touched.length = 0;
  for (const name of ['indexedDB', 'localStorage', 'sessionStorage']) {
    saved[name] = globals[name];
    globals[name] = trap(name);
  }
});
afterEach(() => {
  for (const name of ['indexedDB', 'localStorage', 'sessionStorage']) globals[name] = saved[name];
  expect(touched).toEqual([]);
});

test('wraps and opens through the bridge of Sytue ID; neither the app nor the service sees the unlock key', async () => {
  const s = service();
  const id = sytueID();
  await id.e2e.saveUnlock(subject, unlockKey.slice());
  const custody = new BrowserKeyCustody({
    base: '/drive',
    isolated: true,
    idOrigin,
    fetchImpl: s.fetchImpl,
    bridge: id.bridge,
  });
  await custody.wrap(key, { account, subject, keyID: 'K1' });
  expect(s.requests[0]).toMatchObject({
    method: 'PUT',
    url: '/drive/api/matrix/recovery-wrappers?identity_version=2',
  });
  const body = s.requests[0]!.body!;
  expect(body).not.toContain('EsT3');
  for (const encoded of encodings(unlockKey)) expect(body).not.toContain(encoded);
  expect(s.stored[0]).toMatchObject({ kind: 'sytue-id', credential_id: '', key_id: 'K1' });
  expect(await custody.state(['K1'])).toEqual({ methods: ['sytue-id'], legacy: false, older: '' });
  expect(await custody.unwrap({ account, subject, keyIDs: ['K1'] })).toBe(key);
  // Bound to its account: another account does not open it.
  await expect(custody.unwrap({ account: account + 'x', subject, keyIDs: ['K1'] })).rejects.toEqual(
    new MatrixCustodyError('signin'),
  );
  expectNoUnlockKey(id.received);
  expect(id.received.length).toBeGreaterThan(0);
}, 30_000);

test('the bridge refuses another product, and the custody does not send the person to sign in for it', async () => {
  const s = service();
  const id = sytueID('https://mail.sytue.eu');
  await id.e2e.saveUnlock(subject, unlockKey.slice());
  const custody = new BrowserKeyCustody({
    base: '/mail',
    isolated: true,
    idOrigin,
    fetchImpl: s.fetchImpl,
    bridge: id.bridge,
  });
  // Mail asking to seal Drive's isolated account.
  await expect(custody.wrap(key, { account, subject, keyID: 'K1' })).rejects.toEqual(
    new MatrixCustodyError('unavailable'),
  );
  expect(s.requests).toEqual([]);
});

test('without an unlock key in this browser it asks for the Sytue ID sign-in, never a password', async () => {
  const s = service();
  const id = sytueID('https://rooms.sytue.eu');
  const custody = new BrowserKeyCustody({
    base: '/rooms',
    isolated: false,
    idOrigin,
    fetchImpl: s.fetchImpl,
    bridge: id.bridge,
  });
  await expect(
    custody.wrap(key, { account: '@sytue_0f3c:sytue.eu', subject, keyID: 'K1' }),
  ).rejects.toEqual(new MatrixCustodyError('signin'));
  expect(s.requests).toEqual([]);
  expect(custody.signInURL('/rooms/?id=1', '')).toBe(
    '/rooms/auth/login?unlock=1&returnTo=%2Frooms%2F%3Fid%3D1',
  );
});

test('a wrapper sealed with an unlock key the person no longer has is told apart', async () => {
  const s = service();
  const before = sytueID();
  await before.e2e.saveUnlock(subject, new Uint8Array(32).fill(9));
  await new BrowserKeyCustody({
    base: '/drive',
    isolated: true,
    idOrigin,
    fetchImpl: s.fetchImpl,
    bridge: before.bridge,
  }).wrap(key, { account, subject, keyID: 'K1' });
  const now = sytueID();
  await now.e2e.saveUnlock(subject, unlockKey.slice());
  const custody = new BrowserKeyCustody({
    base: '/drive',
    isolated: true,
    idOrigin,
    fetchImpl: s.fetchImpl,
    bridge: now.bridge,
  });
  await expect(custody.unwrap({ account, subject, keyIDs: ['K1'] })).rejects.toEqual(
    new MatrixCustodyError('wrong'),
  );
});

test('the ADR 0082 wrappers open in Sytue ID with what its sign-in page computed, and move to the new kind', async () => {
  const s = service();
  const passwordVector = old.find((v) => v.wrapper.kind === 'password')!;
  const passkeyVector = old.find((v) => v.wrapper.kind === 'passkey')!;
  s.stored.push(passwordVector.wrapper, passkeyVector.wrapper);
  const legacyIKM = fromBase64(desbloqueo.legacy_password_ikm);
  const id = sytueID('https://rooms.sytue.eu');
  await id.e2e.saveUnlock(subject, unlockKey.slice());
  await id.e2e.store.put('legacy:' + subject, {
    v: 2,
    at: Date.now(),
    items: [
      await id.e2e.legacyKey(
        { kind: 'password', salt: (passwordVector.wrapper.kdf as { salt: string }).salt },
        legacyIKM.slice(),
      ),
    ],
  });
  const custody = new BrowserKeyCustody({
    base: '/rooms',
    isolated: false,
    idOrigin,
    fetchImpl: s.fetchImpl,
    bridge: id.bridge,
  });
  const state = await custody.state([passwordVector.wrapper.key_id]);
  // The Sytue ID sign-in page gets the public parameters: the salt, never a secret.
  expect(state.older).toBe('p.EBESExQVFhcYGRobHB0eHw');
  expect(state.methods).toEqual(['password']);
  const opened = await custody.unwrap({
    account: passwordVector.account,
    subject,
    keyIDs: [passwordVector.wrapper.key_id],
  });
  expect(opened).toBe(key);
  await custody.wrap(opened, {
    account: passwordVector.account,
    subject,
    keyID: passwordVector.wrapper.key_id,
  });
  // Moved: Sytue ID forgot what opened the old wrapper.
  expect(id.data.has('legacy:' + subject)).toBe(false);
  expect(s.stored.some((w) => w.kind === 'sytue-id')).toBe(true);
  // Every request carried public parameters and ciphertext only.
  for (const r of s.requests) {
    for (const encoded of encodings(legacyIKM)) expect(r.body ?? '').not.toContain(encoded);
    expect(r.body ?? '').not.toContain('EsT3');
  }
  expectNoUnlockKey(id.received, [unlockKey, legacyIKM]);
}, 30_000);

test('a refused upload is unavailable and stores nothing', async () => {
  const refused = service({ putStatus: 503 });
  const id = sytueID();
  await id.e2e.saveUnlock(subject, unlockKey.slice());
  await expect(
    new BrowserKeyCustody({
      base: '/drive',
      isolated: true,
      idOrigin,
      fetchImpl: refused.fetchImpl,
      bridge: id.bridge,
    }).wrap(key, { account, subject, keyID: 'K1' }),
  ).rejects.toEqual(new MatrixCustodyError('unavailable'));
  expect(refused.stored).toHaveLength(0);
});

test('the retired service copy is offered only for the Rooms account', async () => {
  const s = service({ legacy: true });
  const id = sytueID();
  const options = { idOrigin, fetchImpl: s.fetchImpl, bridge: id.bridge };
  const rooms = new BrowserKeyCustody({ base: '/rooms', isolated: false, ...options });
  const isolated = new BrowserKeyCustody({ base: '/drive', isolated: true, ...options });
  expect(await rooms.state(undefined)).toEqual({ methods: [], legacy: true, older: '' });
  expect(await rooms.legacyRecoveryKey()).toBe(key);
  expect(await isolated.state(undefined)).toEqual({ methods: [], legacy: false, older: '' });
  expect(await isolated.legacyRecoveryKey()).toBeNull();
});
