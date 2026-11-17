/* SPDX-License-Identifier: Apache-2.0 */
/**
 * The browser scripts of the Sytue ID pages (ADR 0084, 0085): login, relay and bridge.
 * They are plain scripts served inline by the Go service, so this test loads them
 * as the browser would and checks them against the shared Go vectors
 * (apps/nucleo/recoverywrap/testdata/desbloqueo.json), checks what they send
 * to the server (only the salt request and envelopes the server cannot open), what
 * they keep in this browser (non-extractable keys and ciphertext only) and what the
 * bridge hands to an app (never the unlock key).
 */
import { beforeAll, describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { openRecoveryWrapper, type RecoveryWrapper } from './recovery-wrap.js';

const pages = join(import.meta.dir, '../../../../../../services/id/internal/http/e2e');
const source = (name: string) => readFileSync(join(pages, name), 'utf8');

interface Envelope {
  v: 1;
  kind: 'password' | 'passkey';
  credential_id: string;
  kdf: Record<string, unknown>;
  hint: string;
  nonce: string;
  ciphertext: string;
}
interface Vectors {
  unlock_key: string;
  hint: string;
  user_id: string;
  password: string;
  password_kek: string;
  password_envelope: Envelope;
  credential_id: string;
  prf_input: string;
  prf_output: string;
  passkey_kek: string;
  passkey_envelope: Envelope;
  native: {
    app: { d: string; x: string; y: string };
    app_public: string;
    ephemeral: { d: string; x: string; y: string };
    nonce: string;
    plaintext: string;
    blob: string;
    info: string;
  };
  legacy_password_ikm: string;
  account: string;
  key_id: string;
  recovery_key: string;
  sytue_id_wrapper: RecoveryWrapper;
}
const vectors = JSON.parse(
  readFileSync(
    join(import.meta.dir, '../../../../../nucleo/recoverywrap/testdata/desbloqueo.json'),
    'utf8',
  ),
) as Vectors;

const b64 = (bytes: Uint8Array) => Buffer.from(bytes).toString('base64');
const unb64 = (s: string) => new Uint8Array(Buffer.from(s, 'base64'));
const hex = (bytes: Uint8Array) => Buffer.from(bytes).toString('hex');
const b64u = (bytes: Uint8Array) => Buffer.from(bytes).toString('base64url');

type Listener = (event: unknown) => void;
interface FakeWindow extends Record<string, unknown> {
  listeners: Record<string, Listener[]>;
  addEventListener: (type: string, fn: Listener) => void;
}
type Store = {
  get: (k: string) => Promise<unknown>;
  put: (k: string, v: unknown) => Promise<void>;
  putIfAbsent: (k: string, v: unknown) => Promise<unknown>;
  del: (k: string) => Promise<void>;
  keys: () => Promise<string[]>;
  clear: () => Promise<void>;
};
interface E2E {
  store: Store;
  hint: (key: Uint8Array) => Promise<Uint8Array>;
  passwordIKM: (pw: string, kdf: Record<string, unknown>) => Promise<Uint8Array>;
  passwordKEK: (pw: string, kdf: Record<string, unknown>) => Promise<CryptoKey>;
  passkeyKEK: (prf: Uint8Array) => Promise<CryptoKey>;
  passkeyPRFInput: () => Promise<Uint8Array>;
  sealEnvelope: (o: Record<string, unknown>) => Promise<Envelope>;
  openEnvelope: (kek: CryptoKey, user: string, env: Envelope) => Promise<Uint8Array>;
  sealDelivery: (
    pub: string,
    payload: unknown,
    eph?: { d: string; x: string; y: string },
    nonce?: Uint8Array,
  ) => Promise<string>;
  derivePassword: (email: string, pw: string, f: typeof fetch) => Promise<Record<string, unknown>>;
  rememberPasskey: (credential: unknown) => Promise<void>;
  relay: (cfg: Record<string, unknown>, opts?: Record<string, unknown>) => Promise<RelayResult>;
  bridge: (cfg: Record<string, unknown>, win: FakeWindow, now?: () => number) => boolean;
  saveUnlock: (sub: string, key: Uint8Array) => Promise<unknown>;
  legacyKey: (item: Record<string, unknown>, ikm: Uint8Array) => Promise<unknown>;
  legacyBox: (item: Record<string, unknown>, ikm: Uint8Array) => Promise<unknown>;
  load: (name: string) => Promise<Record<string, unknown> | null>;
  migrate: () => Promise<void>;
  accountAllowed: (origin: string, sub: string, account: string) => boolean;
  passkeyEnroll: (cfg: Record<string, unknown>, opts?: Record<string, unknown>) => Promise<boolean>;
  passwordEnvelope: (o: {
    sub: string;
    current: string;
    next: string;
  }) => Promise<{ envelope?: Envelope; error?: string }>;
  keepPassword: (sub: string, env: Envelope) => Promise<void>;
  returnTo: (next: string, blob: string, ua: string) => { auto: string; manual: string };
  passwordChangeForm: (form: unknown, opts?: Record<string, unknown>) => void;
  PENDING_TTL: number;
}
interface RelayResult {
  unlocked?: boolean;
  blob?: string;
  lost?: boolean;
  enroll?: boolean;
  startOver?: () => Promise<RelayResult>;
}

/**
 * The store as IndexedDB keeps it: every value goes through the structured clone, so
 * a CryptoKey stays a CryptoKey and nothing else survives by reference.
 */
function memoryStore(): Store & { data: Map<string, unknown> } {
  const data = new Map<string, unknown>();
  return {
    data,
    get: async (k) => structuredClone(data.get(k)),
    put: async (k, v) => void data.set(k, structuredClone(v)),
    putIfAbsent: async (k, v) => {
      if (!data.has(k)) data.set(k, structuredClone(v));
      return structuredClone(data.get(k));
    },
    del: async (k) => void data.delete(k),
    keys: async () => [...data.keys()],
    clear: async () => data.clear(),
  };
}

/**
 * Nothing in this browser's store can be read: every key is a non-extractable
 * CryptoKey, nothing is raw bytes, and no text is a secret in any encoding.
 */
async function expectSealedStore(store: Store, secrets: Uint8Array[], extra: string[] = []) {
  let keys = 0;
  const walk = (value: unknown, path: string): void => {
    if (value instanceof CryptoKey) {
      keys++;
      expect({ path, extractable: value.extractable }).toEqual({ path, extractable: false });
      return;
    }
    if (ArrayBuffer.isView(value) || value instanceof ArrayBuffer)
      throw new Error(`raw bytes in the store at ${path}`);
    if (typeof value === 'string') {
      for (const secret of secrets) {
        for (const encoded of [b64(secret), b64u(secret), hex(secret)])
          expect(value.includes(encoded)).toBe(false);
      }
      for (const text of extra) expect(value.includes(text)).toBe(false);
      return;
    }
    if (Array.isArray(value)) value.forEach((v, i) => walk(v, `${path}[${i}]`));
    else if (value && typeof value === 'object')
      for (const [k, v] of Object.entries(value)) walk(v, `${path}.${k}`);
  };
  for (const name of await store.keys()) walk(await store.get(name), name);
  return keys;
}

/** A raw KEK of the vectors as the login page keeps it: a non-extractable AES-GCM key. */
const kekKey = (raw: Uint8Array) =>
  crypto.subtle.importKey('raw', new Uint8Array(raw), 'AES-GCM', false, ['encrypt', 'decrypt']);

function fakeWindow(extra: Record<string, unknown> = {}): FakeWindow {
  const listeners: Record<string, Listener[]> = {};
  return {
    crypto: globalThis.crypto,
    hashwasm: (globalThis as { hashwasm?: unknown }).hashwasm,
    location: { origin: 'https://id.sytue.eu' },
    setTimeout: globalThis.setTimeout,
    clearTimeout: globalThis.clearTimeout,
    listeners,
    addEventListener: (type: string, fn: Listener) => {
      (listeners[type] ??= []).push(fn);
    },
    ...extra,
  };
}

/** Runs the page scripts in a window of their own, as the browser does. */
function load(win: FakeWindow, ...names: string[]): E2E {
  for (const name of names) new Function('window', source(name))(win);
  const e2e = win.SytueE2E as E2E;
  e2e.store = memoryStore();
  return e2e;
}

beforeAll(() => {
  // The vendored Argon2id sets `hashwasm` on the global object, as in the page.
  new Function(source('argon2.umd.min.js'))();
});

/** Recorded requests: what reaches the server. */
function recorder(respond: (url: string, init: RequestInit) => unknown = () => ({})) {
  const calls: { url: string; body: string }[] = [];
  const f = (async (url: string, init: RequestInit) => {
    calls.push({ url, body: String(init?.body ?? '') });
    const out = respond(url, init);
    return new Response(out === null ? null : JSON.stringify(out), {
      status: out === null ? 409 : 200,
    });
  }) as unknown as typeof fetch;
  return { f, calls };
}

/** Nothing secret travels: the server only ever sees what it already may see. */
function expectNoSecrets(calls: { body: string }[], secrets: Uint8Array[], extra: string[] = []) {
  for (const call of calls) {
    for (const secret of secrets) {
      expect(call.body).not.toContain(b64(secret));
      expect(call.body).not.toContain(b64u(secret));
      expect(call.body).not.toContain(hex(secret));
    }
    for (const text of extra) expect(call.body).not.toContain(text);
  }
}

const passwordKDF = vectors.password_envelope.kdf;
const key = unb64(vectors.unlock_key);
const passwordKEK = Uint8Array.from(Buffer.from(vectors.password_kek, 'hex'));
const passkeyKEK = Uint8Array.from(Buffer.from(vectors.passkey_kek, 'hex'));

describe('shared vectors', () => {
  test('the vendored Argon2id is hash-wasm from node_modules, byte for byte', () => {
    const vendored = source('argon2.umd.min.js');
    const upstream = readFileSync(
      join(dirname(require.resolve('hash-wasm')), 'argon2.umd.min.js'),
      'utf8',
    );
    expect(vendored).toBe(upstream);
  });

  test('hint, KEKs, envelopes and the PRF input match Go; the KEKs are never bytes', async () => {
    const e2e = load(fakeWindow(), 'comun.js');
    expect(b64(await e2e.hint(key))).toBe(vectors.hint);
    expect(b64(await e2e.passkeyPRFInput())).toBe(vectors.prf_input);
    const prf = unb64(vectors.prf_output);
    const derived = {
      password: await e2e.passwordKEK(vectors.password, passwordKDF),
      passkey: await e2e.passkeyKEK(prf),
    };
    // The PRF output is wiped as soon as it is a key.
    expect(prf.every((b) => b === 0)).toBe(true);
    for (const [env, kek] of [
      [vectors.password_envelope, derived.password],
      [vectors.passkey_envelope, derived.passkey],
    ] as const) {
      expect(kek).toBeInstanceOf(CryptoKey);
      expect(kek.extractable).toBe(false);
      expect([...kek.usages].sort()).toEqual(['decrypt', 'encrypt']);
      await expect(crypto.subtle.exportKey('raw', kek)).rejects.toThrow();
      // The derived (non-extractable) KEK seals exactly the Go envelope: it is the vector KEK.
      const sealed = await e2e.sealEnvelope({
        kek,
        kind: env.kind,
        user: vectors.user_id,
        cred: env.credential_id,
        kdf: env.kdf,
        key,
        nonce: unb64(env.nonce),
      });
      expect(sealed.ciphertext).toBe(env.ciphertext);
      expect(sealed.hint).toBe(env.hint);
      expect(b64(await e2e.openEnvelope(kek, vectors.user_id, env))).toBe(vectors.unlock_key);
      await expect(e2e.openEnvelope(kek, 'another-user', env)).rejects.toThrow();
      await expect(
        e2e.openEnvelope(
          kek === derived.passkey ? derived.password : derived.passkey,
          vectors.user_id,
          env,
        ),
      ).rejects.toThrow();
    }
  });

  test('the native delivery is the Go blob, and the legacy Argon2id opens 0082 wrappers', async () => {
    const e2e = load(fakeWindow(), 'comun.js');
    const blob = await e2e.sealDelivery(
      vectors.native.app_public,
      JSON.parse(vectors.native.plaintext),
      vectors.native.ephemeral,
      unb64(vectors.native.nonce),
    );
    expect(blob).toBe(vectors.native.blob);
    const legacy = await e2e.passwordIKM(vectors.password, {
      ...passwordKDF,
      salt: b64(Uint8Array.from({ length: 16 }, (_, i) => 0x10 + i)),
    });
    expect(b64(legacy)).toBe(vectors.legacy_password_ikm);
  });
});

/** Opens a native delivery as the app does (recoverywrap.OpenDelivery). */
async function openDelivery(
  blob: string,
): Promise<{ sub: string; key: string; legacy?: { ikm: string }[] }> {
  const raw = new Uint8Array(Buffer.from(blob, 'base64url'));
  const curve = { name: 'ECDH', namedCurve: 'P-256' };
  const app = vectors.native.app;
  const priv = await crypto.subtle.importKey(
    'jwk',
    { kty: 'EC', crv: 'P-256', ...app },
    curve,
    false,
    ['deriveBits'],
  );
  const eph = await crypto.subtle.importKey('raw', raw.slice(0, 65), curve, false, []);
  const shared = new Uint8Array(
    await crypto.subtle.deriveBits({ name: 'ECDH', public: eph }, priv, 256),
  );
  const base = await crypto.subtle.importKey('raw', shared, 'HKDF', false, ['deriveKey']);
  const salt = new Uint8Array([...raw.slice(0, 65), ...unb64url(vectors.native.app_public)]);
  const aes = await crypto.subtle.deriveKey(
    { name: 'HKDF', hash: 'SHA-256', salt, info: new TextEncoder().encode(vectors.native.info) },
    base,
    { name: 'AES-GCM', length: 256 },
    false,
    ['decrypt'],
  );
  const plain = await crypto.subtle.decrypt(
    {
      name: 'AES-GCM',
      iv: raw.slice(65, 77),
      additionalData: new TextEncoder().encode(vectors.native.info),
    },
    aes,
    raw.slice(77),
  );
  return JSON.parse(new TextDecoder().decode(plain));
}
const unb64url = (s: string) => new Uint8Array(Buffer.from(s, 'base64url'));

/** The ADR 0082 password wrapper of the Rooms account, and what opens it. */
const oldVectors = JSON.parse(
  readFileSync(
    join(import.meta.dir, '../../../../../nucleo/recoverywrap/testdata/vectors.json'),
    'utf8',
  ),
) as { kind: string; account: string; recovery_key: string; wrapper: RecoveryWrapper }[];
const oldPassword = oldVectors.find((v) => v.kind === 'password')!;
const legacyIKM = unb64(vectors.legacy_password_ikm);
const oldSalt = (oldPassword.wrapper.kdf as { salt: string }).salt;

describe('login page', () => {
  test('derives the KEK in the browser as a key nobody can read; the server only sees the e-mail', async () => {
    const win = fakeWindow({ document: { querySelector: () => null } });
    const e2e = load(win, 'comun.js', 'login.js');
    const { f, calls } = recorder(() => ({ kdf: passwordKDF }));
    const pending = await e2e.derivePassword('ana@acme.example', vectors.password, f);
    const kek = pending.kek as CryptoKey;
    expect(kek).toBeInstanceOf(CryptoKey);
    expect(kek.extractable).toBe(false);
    const sealed = await e2e.sealEnvelope({
      kek,
      kind: 'password',
      user: vectors.user_id,
      kdf: passwordKDF,
      key,
      nonce: unb64(vectors.password_envelope.nonce),
    });
    expect(sealed.ciphertext).toBe(vectors.password_envelope.ciphertext);
    expect(calls.map((c) => c.url)).toEqual(['/login/unlock/salt']);
    expect(JSON.parse(calls[0]!.body)).toEqual({ email: 'ana@acme.example' });
  });

  test('the legacy request in return_to adds the 0082 Argon2id, sealed; the form is sent after deriving', async () => {
    const salt = Buffer.from(Uint8Array.from({ length: 16 }, (_, i) => 0x10 + i)).toString(
      'base64url',
    );
    const returnTo = `/oauth2/authorize?client_id=x&sytue_legacy=${encodeURIComponent('p.' + salt)}`;
    const handlers: Record<string, (e: unknown) => void> = {};
    const fields: Record<string, { value: string; disabled?: boolean }> = {
      'input[name=password]': { value: vectors.password },
      'input[name=email]': { value: 'ana@acme.example' },
      'button[type=submit]': { value: '' },
    };
    let submitted = 0;
    const form = {
      addEventListener: (type: string, fn: (e: unknown) => void) => (handlers[type] = fn),
      querySelector: (q: string) => fields[q] ?? null,
    };
    const doc = {
      querySelector: (q: string) =>
        q === 'input[name=return_to]'
          ? { value: returnTo }
          : q === 'form[action="/login/"]'
            ? form
            : null,
    };
    const { f, calls } = recorder(() => ({ kdf: passwordKDF }));
    const win = fakeWindow({
      document: doc,
      fetch: f,
      indexedDB: {},
      HTMLFormElement: { prototype: { submit: () => submitted++ } },
    });
    // The page scripts read `global.fetch` when they run: provide it before loading.
    const e2e = (() => {
      new Function('window', source('comun.js'))(win);
      const e = win.SytueE2E as E2E;
      e.store = memoryStore();
      new Function('window', source('login.js'))(win);
      return e;
    })();
    let prevented = false;
    handlers.submit!({ defaultPrevented: false, preventDefault: () => (prevented = true) });
    expect(prevented).toBe(true);
    for (let i = 0; i < 200 && !submitted; i++) await new Promise((r) => setTimeout(r, 25));
    expect(submitted).toBe(1);
    const pending = (await e2e.load('pending')) as {
      kek: CryptoKey;
      legacy: { salt: string; box: unknown }[];
    };
    expect(pending.kek.extractable).toBe(false);
    expect(pending.legacy).toHaveLength(1);
    expect(pending.legacy[0]!.salt).toBe(oldSalt);
    // Nothing readable in this browser: not the KEK, not the 0082 Argon2id, not the password.
    expect(
      await expectSealedStore(e2e.store, [passwordKEK, legacyIKM], [vectors.password]),
    ).toBeGreaterThan(0);
    expectNoSecrets(calls, [passwordKEK, legacyIKM], [vectors.password]);
  });

  test('a passkey sign-in keeps the PRF only as a key, and wipes what the authenticator gave', async () => {
    const win = fakeWindow({ document: { querySelector: () => null } });
    const e2e = load(win, 'comun.js', 'login.js');
    const prf = unb64(vectors.prf_output);
    const first = prf.slice().buffer;
    await e2e.rememberPasskey({
      rawId: unb64url(vectors.credential_id).buffer,
      getClientExtensionResults: () => ({ prf: { results: { first } } }),
    });
    expect(new Uint8Array(first).every((b) => b === 0)).toBe(true);
    const pending = (await e2e.load('pending')) as { kek: CryptoKey; cred: string };
    expect(pending.cred).toBe(vectors.credential_id);
    const sealed = await e2e.sealEnvelope({
      kek: pending.kek,
      kind: 'passkey',
      user: vectors.user_id,
      cred: vectors.credential_id,
      kdf: vectors.passkey_envelope.kdf,
      key,
      nonce: unb64(vectors.passkey_envelope.nonce),
    });
    expect(sealed.ciphertext).toBe(vectors.passkey_envelope.ciphertext);
    await expectSealedStore(e2e.store, [prf, passkeyKEK]);
  });
});

describe('relay', () => {
  const now = 1_000_000_000;
  const base = {
    sub: vectors.user_id,
    next: 'https://mail.sytue.eu/auth/callback?code=c&state=s',
    app_key: '',
    salt: passwordKDF.salt,
    creds: [vectors.credential_id],
    auth_age: 5_000,
    hint: '',
    envelopes: [] as Envelope[],
  };
  const pendingPassword = async (kek?: CryptoKey, at = now - 10_000, legacy: unknown[] = []) => ({
    v: 2,
    kind: 'password',
    kek: kek ?? (await kekKey(passwordKEK)),
    kdf: passwordKDF,
    legacy,
    at,
  });
  /** The unlock key this browser keeps, opened the only way an app can: through the bridge's key. */
  async function storedOpens(e2e: E2E, sub: string, expected: Uint8Array) {
    const rec = (await e2e.load('auk:' + sub)) as { hint: string; wrap: CryptoKey } | null;
    expect(rec).not.toBeNull();
    expect(rec!.hint).toBe(b64(await e2e.hint(expected)));
    expect(rec!.wrap.extractable).toBe(false);
  }

  test('the first sign-in creates the unlock key, keeps it sealed and uploads only its envelope', async () => {
    const e2e = load(fakeWindow(), 'comun.js', 'relay.js');
    await e2e.store.put('pending', await pendingPassword());
    const { f, calls } = recorder();
    const r = await e2e.relay(
      { ...base, app_key: vectors.native.app_public },
      { fetch: f, now: () => now },
    );
    expect(r.unlocked).toBe(true);
    expect('key' in r).toBe(false);
    expect(calls.map((c) => c.url)).toEqual(['/e2e/envelopes']);
    const body = JSON.parse(calls[0]!.body) as { envelope: Envelope; reset: boolean; sub: string };
    expect(body.reset).toBe(false);
    expect(body.sub).toBe(vectors.user_id);
    const created = await e2e.openEnvelope(
      await kekKey(passwordKEK),
      vectors.user_id,
      body.envelope,
    );
    expectNoSecrets(calls, [created, passwordKEK]);
    expect(await e2e.load('pending')).toBeNull();
    await storedOpens(e2e, vectors.user_id, created);
    await expectSealedStore(e2e.store, [created, passwordKEK]);
    const delivered = await openDelivery(r.blob!);
    expect(delivered.sub).toBe(vectors.user_id);
    expect(delivered.key).toBe(b64(created));
  });

  test('an existing envelope opens with the password just typed: nothing is uploaded', async () => {
    const e2e = load(fakeWindow(), 'comun.js', 'relay.js');
    await e2e.store.put('pending', await pendingPassword());
    const { f, calls } = recorder();
    const r = await e2e.relay(
      { ...base, hint: vectors.hint, envelopes: [vectors.password_envelope] },
      { fetch: f, now: () => now },
    );
    expect(r.unlocked).toBe(true);
    expect(calls).toEqual([]);
    await storedOpens(e2e, vectors.user_id, key);
    await expectSealedStore(e2e.store, [key, passwordKEK]);
  });

  test('a native app gets the key this browser already holds, sealed for it only', async () => {
    const e2e = load(fakeWindow(), 'comun.js', 'relay.js');
    await e2e.saveUnlock(vectors.user_id, key.slice());
    const { f, calls } = recorder();
    const r = await e2e.relay(
      { ...base, hint: vectors.hint, app_key: vectors.native.app_public },
      { fetch: f, now: () => now },
    );
    expect(calls).toEqual([]);
    expect((await openDelivery(r.blob!)).key).toBe(vectors.unlock_key);
    // The web gets nothing in the URL.
    const web = await e2e.relay({ ...base, hint: vectors.hint }, { fetch: f, now: () => now });
    expect(web).toEqual({ unlocked: true, blob: '' });
  });

  test('after a password change, the browser that holds the key re-wraps it with the new password', async () => {
    const e2e = load(fakeWindow(), 'comun.js', 'relay.js');
    const otherKEK = await e2e.passwordKEK('una contraseña nueva', passwordKDF);
    await e2e.store.put('pending', await pendingPassword(otherKEK));
    await e2e.saveUnlock(vectors.user_id, key.slice());
    const { f, calls } = recorder();
    const r = await e2e.relay(
      { ...base, hint: vectors.hint, envelopes: [vectors.password_envelope] },
      { fetch: f, now: () => now },
    );
    expect(r.unlocked).toBe(true);
    const body = JSON.parse(calls[0]!.body) as { envelope: Envelope; reset: boolean };
    expect(body.envelope.hint).toBe(vectors.hint);
    expect(b64(await e2e.openEnvelope(otherKEK, vectors.user_id, body.envelope))).toBe(
      vectors.unlock_key,
    );
    expectNoSecrets(calls, [key]);
    await expectSealedStore(e2e.store, [key]);
  });

  test('a password that no longer opens it, with no key here, asks before starting over', async () => {
    const e2e = load(fakeWindow(), 'comun.js', 'relay.js');
    const otherKEK = await e2e.passwordKEK('restablecida', passwordKDF);
    await e2e.store.put('pending', await pendingPassword(otherKEK));
    const { f, calls } = recorder();
    const r = await e2e.relay(
      { ...base, hint: vectors.hint, envelopes: [vectors.password_envelope] },
      { fetch: f, now: () => now },
    );
    expect(r.lost).toBe(true);
    expect(calls).toEqual([]);
    const restarted = await r.startOver!();
    expect(restarted.unlocked).toBe(true);
    const body = JSON.parse(calls[0]!.body) as { envelope: Envelope; reset: boolean };
    expect(body.reset).toBe(true);
    expect(body.envelope.hint).not.toBe(vectors.hint);
    const fresh = await e2e.openEnvelope(otherKEK, vectors.user_id, body.envelope);
    await storedOpens(e2e, vectors.user_id, fresh);
    await expectSealedStore(e2e.store, [fresh]);
  });

  test('a pending KEK from an attempt after this session opened, or for another account, is ignored', async () => {
    const e2e = load(fakeWindow(), 'comun.js', 'relay.js');
    const { f, calls } = recorder();
    // Derived 1 s ago, but the session is 5 min old: a failed attempt, not this login.
    await e2e.store.put('pending', await pendingPassword(undefined, now - 1_000));
    let r = await e2e.relay({ ...base, auth_age: 300_000 }, { fetch: f, now: () => now });
    expect(r.unlocked).toBe(false);
    await e2e.store.put('pending', await pendingPassword());
    r = await e2e.relay({ ...base, salt: b64(new Uint8Array(16)) }, { fetch: f, now: () => now });
    expect(r.unlocked).toBe(false);
    expect(calls).toEqual([]);
  });

  test('a new passkey enrols with the key this browser holds; without it, nothing happens', async () => {
    const e2e = load(fakeWindow(), 'comun.js', 'relay.js');
    const pending = async () => ({
      v: 2,
      kind: 'passkey',
      kek: await kekKey(passkeyKEK),
      cred: vectors.credential_id,
      legacy: [],
      at: now - 1_000,
    });
    await e2e.store.put('pending', await pending());
    const { f, calls } = recorder();
    let r = await e2e.relay(
      { ...base, hint: vectors.hint, envelopes: [vectors.password_envelope] },
      { fetch: f, now: () => now },
    );
    expect(r.unlocked).toBe(false);
    // The page says to sign in once with the password here.
    expect(r.enroll).toBe(true);
    expect(calls).toEqual([]);
    await e2e.store.put('pending', await pending());
    await e2e.saveUnlock(vectors.user_id, key.slice());
    r = await e2e.relay(
      { ...base, hint: vectors.hint, envelopes: [vectors.password_envelope] },
      { fetch: f, now: () => now },
    );
    expect(r.unlocked).toBe(true);
    const body = JSON.parse(calls[0]!.body) as { envelope: Envelope };
    expect(body.envelope.kind).toBe('passkey');
    expect(body.envelope.credential_id).toBe(vectors.credential_id);
    expect(
      b64(await e2e.openEnvelope(await kekKey(passkeyKEK), vectors.user_id, body.envelope)),
    ).toBe(vectors.unlock_key);
    expectNoSecrets(calls, [key, passkeyKEK]);
  });

  test('the 0082 transition: the bridge gets keys for the old wrappers, a native app their IKM', async () => {
    const e2e = load(fakeWindow(), 'comun.js', 'relay.js');
    const boxed = await e2e.legacyBox({ kind: 'password', salt: oldSalt }, legacyIKM.slice());
    await e2e.store.put('pending', await pendingPassword(undefined, now - 10_000, [boxed]));
    const { f } = recorder();
    const r = await e2e.relay(
      {
        ...base,
        hint: vectors.hint,
        envelopes: [vectors.password_envelope],
        app_key: vectors.native.app_public,
      },
      { fetch: f, now: () => now },
    );
    const delivered = await openDelivery(r.blob!);
    expect(delivered.legacy?.[0]?.ikm).toBe(vectors.legacy_password_ikm);
    const legacy = (await e2e.load('legacy:' + vectors.user_id)) as {
      items: { salt: string; key: CryptoKey }[];
    };
    expect(legacy.items[0]!.salt).toBe(oldSalt);
    expect(legacy.items[0]!.key.extractable).toBe(false);
    await expectSealedStore(e2e.store, [key, legacyIKM, passwordKEK]);
  });
});

describe('the store of the ADR 0084 pages', () => {
  test('what they wrote in clear becomes non-extractable keys on first use; the clear copy is overwritten', async () => {
    const win = fakeWindow();
    const e2e = load(win, 'comun.js', 'relay.js', 'bridge.js');
    const clearKey = key.slice();
    const clearKEK = passwordKEK.slice();
    const clearIKM = legacyIKM.slice();
    await e2e.store.put('auk:0f3c', { key: clearKey, hint: unb64(vectors.hint) });
    await e2e.store.put('pending', {
      kind: 'password',
      kek: clearKEK,
      kdf: passwordKDF,
      legacy: [{ kind: 'password', salt: oldSalt, credential_id: '', ikm: clearIKM }],
      at: Date.now(),
    });
    await e2e.store.put('legacy:0f3c', {
      items: [{ kind: 'password', salt: oldSalt, credential_id: '', ikm: legacyIKM.slice() }],
      at: Date.now(),
    });
    await e2e.store.put('junk', 'ICEiIyQl');
    await e2e.migrate();
    await expectSealedStore(e2e.store, [key, passwordKEK, legacyIKM]);
    expect(await e2e.store.get('junk')).toBeUndefined();
    const rec = (await e2e.load('auk:0f3c')) as { hint: string };
    expect(rec.hint).toBe(vectors.hint);
    const pending = (await e2e.load('pending')) as { kek: CryptoKey };
    expect(
      (
        await e2e.sealEnvelope({
          kek: pending.kek,
          kind: 'password',
          user: vectors.user_id,
          kdf: passwordKDF,
          key,
          nonce: unb64(vectors.password_envelope.nonce),
        })
      ).ciphertext,
    ).toBe(vectors.password_envelope.ciphertext);
  });

  test('the vault key is created once, even when two pages race for it', async () => {
    const e2e = load(fakeWindow(), 'comun.js');
    const [a, b] = await Promise.all([
      e2e.saveUnlock('ana', key.slice()),
      e2e.saveUnlock('bea', key.slice()),
    ]);
    expect(a).toBeTruthy();
    expect(b).toBeTruthy();
    const keys = await e2e.store.keys();
    expect(keys.filter((k) => k === 'vault')).toHaveLength(1);
    // Both boxes open with the one vault that stayed.
    const relay = load(fakeWindow(), 'comun.js', 'relay.js');
    relay.store = e2e.store;
    for (const sub of ['ana', 'bea']) {
      const { f } = recorder();
      const r = await relay.relay(
        { sub, next: '', app_key: vectors.native.app_public, hint: '', creds: [], envelopes: [] },
        { fetch: f },
      );
      expect((await openDelivery(r.blob!)).key).toBe(vectors.unlock_key);
    }
  });
});

describe('bridge', () => {
  const sub = '0f3c';
  function bridgeWindow(origin = 'https://mail.sytue.eu') {
    const sent: { msg: Record<string, unknown>; origin: string }[] = [];
    const parent = {
      postMessage: (msg: Record<string, unknown>, target: string) =>
        sent.push({ msg: structuredClone(msg), origin: target }),
    };
    const win = fakeWindow({ parent });
    const e2e = load(win, 'comun.js', 'bridge.js');
    const message = (data: unknown, from = origin, source: unknown = parent) => {
      for (const fn of win.listeners.message ?? []) fn({ data, origin: from, source });
    };
    return { win, e2e, parent, sent, message, origin };
  }
  const settle = async (sent: unknown[], n: number) => {
    for (let i = 0; i < 200 && sent.length < n; i++) await new Promise((r) => setTimeout(r, 5));
    await new Promise((r) => setTimeout(r, 10));
  };
  /** Nothing the bridge posts is, or contains, the unlock key: not as bytes, not encoded. */
  function expectNoUnlockKey(sent: { msg: Record<string, unknown> }[]) {
    const walk = (value: unknown): void => {
      if (ArrayBuffer.isView(value) || value instanceof ArrayBuffer)
        throw new Error('the bridge posted raw bytes');
      if (value instanceof CryptoKey) throw new Error('the bridge posted a key');
      if (typeof value === 'string')
        for (const encoded of [b64(key), b64u(key), hex(key)])
          expect(value.includes(encoded)).toBe(false);
      else if (value && typeof value === 'object') Object.values(value).forEach(walk);
    };
    for (const { msg } of sent) walk(msg);
  }

  test('opens the wrapper of the account the app asks for and returns only its recovery key', async () => {
    const { win, e2e, sent, message } = bridgeWindow();
    await e2e.saveUnlock(sub, key.slice());
    expect(e2e.bridge({ origin: 'https://mail.sytue.eu' }, win)).toBe(true);
    expect(sent).toEqual([
      { msg: { type: 'sytue-e2e-ready', v: 2 }, origin: 'https://mail.sytue.eu' },
    ]);
    message({
      type: 'sytue-e2e-unwrap',
      id: '1',
      sub,
      account: vectors.account,
      wrappers: [vectors.sytue_id_wrapper],
    });
    await settle(sent, 2);
    expect(sent[1]!.origin).toBe('https://mail.sytue.eu');
    expect(sent[1]!.msg).toEqual({
      type: 'sytue-e2e-result',
      id: '1',
      ok: true,
      recovery_key: vectors.recovery_key,
      kind: 'sytue-id',
    });
    expectNoUnlockKey(sent);
  });

  test('seals a wrapper that opens with the unlock key, without ever posting the key', async () => {
    const { win, e2e, sent, message } = bridgeWindow();
    await e2e.saveUnlock(sub, key.slice());
    e2e.bridge({ origin: 'https://mail.sytue.eu' }, win);
    message({
      type: 'sytue-e2e-wrap',
      id: 'w',
      sub,
      account: vectors.account,
      key_id: vectors.key_id,
      recovery_key: vectors.recovery_key,
    });
    await settle(sent, 2);
    const out = sent[1]!.msg as { ok: boolean; wrapper: RecoveryWrapper; legacy: boolean };
    expect(out.ok).toBe(true);
    expect(out.legacy).toBe(false);
    expect(out.wrapper.kdf).toEqual({ alg: 'sytue-id-unlock-hkdf-sha256', hint: vectors.hint });
    // Go and the earlier web code open it with the unlock key (recoverywrap.UnwrapSytueID).
    expect(await openRecoveryWrapper(out.wrapper, vectors.account, key)).toBe(vectors.recovery_key);
    expectNoUnlockKey(sent);
  });

  test('answers only its parent, from the origin the server validated, for that person and their accounts', async () => {
    const { win, e2e, sent, message } = bridgeWindow();
    await e2e.saveUnlock(sub, key.slice());
    e2e.bridge({ origin: 'https://mail.sytue.eu' }, win);
    const unwrap = {
      type: 'sytue-e2e-unwrap',
      id: '1',
      sub,
      account: vectors.account,
      wrappers: [vectors.sytue_id_wrapper],
    };
    message(unwrap, 'https://evil.example');
    message(unwrap, 'https://x.apps.sytue.eu');
    message(unwrap, 'https://mail.sytue.eu', { postMessage: () => undefined });
    // The protocol before ADR 0085, which handed the key out, gets no answer at all.
    message({ type: 'sytue-e2e-get', id: 'old', sub });
    await settle(sent, 2);
    expect(sent).toHaveLength(1);
    // Another person: this browser has no key for them.
    message({ ...unwrap, id: '2', sub: 'bea', account: '@sytue_bea:sytue.eu' });
    // Another person's account with this person's key: refused before trying.
    message({ ...unwrap, id: '3', account: '@sytue_bea:sytue.eu' });
    // Another product's isolated account from Mail: refused.
    message({ ...unwrap, id: '4', account: `@sytue_storage_drive_acme_${sub}:sytue.eu` });
    await settle(sent, 4);
    const byID = Object.fromEntries(sent.slice(1).map((s) => [s.msg.id, s.msg]));
    expect(byID['2']).toMatchObject({ ok: false, error: 'signin' });
    expect(byID['3']).toMatchObject({ ok: false, error: 'account' });
    expect(byID['4']).toMatchObject({ ok: false, error: 'account' });
    expectNoUnlockKey(sent);
  });

  test('an isolated account only from its own product, and a few operations a second at most', async () => {
    const e2e = load(fakeWindow(), 'comun.js', 'bridge.js');
    const drive = `@sytue_storage_drive_acme_${sub}:sytue.eu`;
    expect(e2e.accountAllowed('https://drive.sytue.eu', sub, drive)).toBe(true);
    expect(e2e.accountAllowed('https://mail.sytue.eu', sub, drive)).toBe(false);
    expect(
      e2e.accountAllowed('https://docus.sytue.eu', sub, `@sytue_storage_docs_a_${sub}:x`),
    ).toBe(true);
    expect(e2e.accountAllowed('https://docs.sytue.eu', sub, `@sytue_storage_docs_a_${sub}:x`)).toBe(
      false,
    );
    expect(e2e.accountAllowed('https://drive.sytue.eu', sub, '@sytue_storage_drive_acme_x:x')).toBe(
      false,
    );
    expect(e2e.accountAllowed('https://drive.sytue.eu', sub, `@sytue_storage_drive_${sub}:x`)).toBe(
      false,
    );
    expect(e2e.accountAllowed('https://mail.sytue.eu', sub, `@sytue_${sub}:sytue.eu`)).toBe(true);
    // A host of an organisation serves every app under one origin (`acme.sytue.eu/drive`,
    // `/mail`…): its apps open their isolated accounts there too.
    expect(e2e.accountAllowed('https://acme.sytue.eu', sub, drive)).toBe(true);
    expect(
      e2e.accountAllowed('https://acme.sytue.eu', sub, `@sytue_storage_docs_acme_${sub}:x`),
    ).toBe(true);
    // Still only that person's accounts.
    expect(e2e.accountAllowed('https://acme.sytue.eu', sub, '@sytue_storage_drive_acme_x:x')).toBe(
      false,
    );
    expect(e2e.accountAllowed('https://mail.sytue.eu', sub, `@sytue_${sub}x:sytue.eu`)).toBe(false);

    const { win, e2e: b, sent, message } = bridgeWindow();
    await b.saveUnlock(sub, key.slice());
    let t = 5_000;
    b.bridge({ origin: 'https://mail.sytue.eu' }, win, () => t);
    for (let i = 0; i < 12; i++) message({ type: 'sytue-e2e-forget-legacy', id: `f${i}`, sub });
    await settle(sent, 13);
    const busy = sent.filter((s) => s.msg.error === 'busy');
    expect(busy).toHaveLength(2);
    t += 1_001;
    message({ type: 'sytue-e2e-forget-legacy', id: 'later', sub });
    await settle(sent, 14);
    expect(sent.at(-1)!.msg).toMatchObject({ id: 'later', ok: true });
  });

  test('opens an ADR 0082 wrapper with what the sign-in page computed, and tells the app to forget it', async () => {
    const { win, e2e, sent, message } = bridgeWindow();
    await e2e.saveUnlock(sub, key.slice());
    await e2e.store.put('legacy:' + sub, {
      v: 2,
      at: Date.now(),
      items: [await e2e.legacyKey({ kind: 'password', salt: oldSalt }, legacyIKM.slice())],
    });
    e2e.bridge({ origin: 'https://mail.sytue.eu' }, win);
    message({
      type: 'sytue-e2e-unwrap',
      id: 'old',
      sub,
      account: oldPassword.account,
      wrappers: [oldPassword.wrapper],
    });
    message({
      type: 'sytue-e2e-wrap',
      id: 'new',
      sub,
      account: oldPassword.account,
      key_id: oldPassword.wrapper.key_id,
      recovery_key: oldPassword.recovery_key,
    });
    await settle(sent, 3);
    const byID = Object.fromEntries(sent.slice(1).map((s) => [s.msg.id, s.msg]));
    expect(byID.old).toMatchObject({
      ok: true,
      kind: 'password',
      recovery_key: oldPassword.recovery_key,
    });
    expect(byID.new).toMatchObject({ ok: true, legacy: true });
    message({ type: 'sytue-e2e-forget-legacy', id: 'f', sub });
    await settle(sent, 4);
    expect(await e2e.store.get('legacy:' + sub)).toBeUndefined();
    expectNoUnlockKey(sent);
    await expectSealedStore(e2e.store, [key, legacyIKM]);
  });

  test('does nothing outside an iframe', () => {
    const win = fakeWindow();
    win.parent = win;
    const e2e = load(win, 'comun.js', 'bridge.js');
    expect(e2e.bridge({ origin: 'https://mail.sytue.eu' }, win)).toBe(false);
  });
});

describe('a new passkey unlocks from the moment it is added (ADR 0085)', () => {
  const cfg = {
    sub: vectors.user_id,
    cred: vectors.credential_id,
    rp_id: 'sytue.eu',
    next: '/account?pk=ok',
  };
  /** The authenticator: it records what it is asked and answers with the vector PRF. */
  function authenticator(prf: Uint8Array | null = unb64(vectors.prf_output)) {
    const asked: Record<string, unknown>[] = [];
    return {
      asked,
      credentials: {
        get: async (options: { publicKey: Record<string, unknown> }) => {
          asked.push(options.publicKey);
          return {
            getClientExtensionResults: () =>
              prf ? { prf: { results: { first: prf.slice().buffer } } } : {},
          };
        },
      },
    };
  }

  test('the PRF given at creation seals the envelope right away, with no second touch', async () => {
    const e2e = load(fakeWindow(), 'comun.js', 'passkey.js');
    await e2e.saveUnlock(vectors.user_id, key.slice());
    await e2e.store.put('enroll', {
      v: 2,
      cred: vectors.credential_id,
      kek: await kekKey(passkeyKEK),
      at: Date.now() - 1_000,
    });
    const a = authenticator();
    const { f, calls } = recorder();
    expect(await e2e.passkeyEnroll(cfg, { credentials: a.credentials, fetch: f })).toBe(true);
    expect(a.asked).toEqual([]);
    expect(calls.map((c) => c.url)).toEqual(['/e2e/envelopes']);
    const body = JSON.parse(calls[0]!.body) as { envelope: Envelope; sub: string };
    expect(body.sub).toBe(vectors.user_id);
    expect(body.envelope).toMatchObject({ kind: 'passkey', credential_id: vectors.credential_id });
    expect(
      b64(await e2e.openEnvelope(await kekKey(passkeyKEK), vectors.user_id, body.envelope)),
    ).toBe(vectors.unlock_key);
    expectNoSecrets(calls, [key, passkeyKEK]);
    // The KEK left by the creation is used once.
    expect(await e2e.store.get('enroll')).toBeUndefined();
    await expectSealedStore(e2e.store, [key, passkeyKEK]);
  });

  test('otherwise the passkey is asked once in Sytue ID, for that credential and with the PRF', async () => {
    const e2e = load(fakeWindow(), 'comun.js', 'passkey.js');
    await e2e.saveUnlock(vectors.user_id, key.slice());
    const a = authenticator();
    const { f, calls } = recorder();
    expect(await e2e.passkeyEnroll(cfg, { credentials: a.credentials, fetch: f })).toBe(true);
    expect(a.asked).toHaveLength(1);
    const asked = a.asked[0] as {
      rpId: string;
      allowCredentials: { id: Uint8Array }[];
      extensions: { prf: { eval: { first: Uint8Array } } };
    };
    expect(asked.rpId).toBe('sytue.eu');
    expect(b64u(asked.allowCredentials[0]!.id)).toBe(vectors.credential_id);
    expect(b64(asked.extensions.prf.eval.first)).toBe(vectors.prf_input);
    const body = JSON.parse(calls[0]!.body) as { envelope: Envelope };
    expect(
      b64(await e2e.openEnvelope(await kekKey(passkeyKEK), vectors.user_id, body.envelope)),
    ).toBe(vectors.unlock_key);
  });

  test('a browser without the unlock key, or an authenticator without PRF, uploads nothing', async () => {
    const e2e = load(fakeWindow(), 'comun.js', 'passkey.js');
    const a = authenticator();
    const { f, calls } = recorder();
    expect(await e2e.passkeyEnroll(cfg, { credentials: a.credentials, fetch: f })).toBe(false);
    expect(a.asked).toEqual([]);
    await e2e.saveUnlock(vectors.user_id, key.slice());
    expect(
      await e2e.passkeyEnroll(cfg, { credentials: authenticator(null).credentials, fetch: f }),
    ).toBe(false);
    expect(calls).toEqual([]);
  });
});

describe('a password change re-seals the envelope at once (ADR 0085)', () => {
  const now = 1_000_000_000;
  const relayCfg = {
    sub: vectors.user_id,
    next: 'https://mail.sytue.eu/auth/callback?code=c&state=s',
    app_key: '',
    salt: passwordKDF.salt,
    creds: [] as string[],
    auth_age: 5_000,
    hint: vectors.hint,
    envelopes: [vectors.password_envelope],
  };
  /** A browser where the person signed in with their password: the relay kept a copy of the envelope. */
  async function signedIn(...scripts: string[]) {
    // The page checks that this browser has IndexedDB; the store itself is the test's.
    const win = fakeWindow({ indexedDB: {} });
    const e2e = load(win, 'comun.js', 'relay.js', ...scripts);
    await e2e.store.put('pending', {
      v: 2,
      kind: 'password',
      kek: await kekKey(passwordKEK),
      kdf: passwordKDF,
      legacy: [],
      at: now - 10_000,
    });
    const { f } = recorder();
    await e2e.relay(relayCfg, { fetch: f, now: () => now });
    return { e2e, win };
  }

  test('the relay keeps the current password envelope (public) next to the key', async () => {
    const { e2e } = await signedIn();
    const rec = (await e2e.load('auk:' + vectors.user_id)) as { pw: Envelope };
    expect(rec.pw).toMatchObject({
      kind: 'password',
      ciphertext: vectors.password_envelope.ciphertext,
      hint: vectors.hint,
    });
    await expectSealedStore(e2e.store, [key, passwordKEK]);
  });

  test('only the current password gets an envelope for the new one, with the same salt', async () => {
    const { e2e } = await signedIn();
    const fresh = 'otra contraseña larga ñ€';
    const wrong = await e2e.passwordEnvelope({
      sub: vectors.user_id,
      current: 'no es la actual',
      next: fresh,
    });
    expect(wrong).toEqual({ error: 'wrong' });
    const r = await e2e.passwordEnvelope({
      sub: vectors.user_id,
      current: vectors.password,
      next: fresh,
    });
    expect(r.envelope!.kdf).toEqual(passwordKDF);
    expect(r.envelope!.hint).toBe(vectors.hint);
    const kek = await e2e.passwordKEK(fresh, passwordKDF);
    expect(b64(await e2e.openEnvelope(kek, vectors.user_id, r.envelope!))).toBe(vectors.unlock_key);
    // Without the copy (another person, a browser that never saw the password): nothing.
    expect(
      await e2e.passwordEnvelope({ sub: 'otra', current: vectors.password, next: fresh }),
    ).toEqual({ error: 'signin' });
  }, 30_000);

  test('the bridge does it only for «Mi cuenta»', async () => {
    const { e2e, win } = await signedIn('bridge.js');
    const sent: { msg: Record<string, unknown> }[] = [];
    const parent = { postMessage: (msg: Record<string, unknown>) => sent.push({ msg }) };
    const ask = (password: boolean) => {
      const frame = fakeWindow({ parent });
      const b = load(frame, 'comun.js', 'bridge.js');
      b.store = e2e.store;
      b.bridge(
        { origin: 'https://account.sytue.eu', ...(password ? { password: true } : {}) },
        frame,
      );
      for (const fn of frame.listeners.message ?? [])
        fn({
          data: {
            type: 'sytue-e2e-password',
            id: password ? 'yes' : 'no',
            sub: vectors.user_id,
            current: vectors.password,
            next: 'otra contraseña larga ñ€',
          },
          origin: 'https://account.sytue.eu',
          source: parent,
        });
    };
    void win;
    ask(false);
    ask(true);
    for (let i = 0; i < 400 && !sent.some((s) => s.msg.id === 'yes'); i++)
      await new Promise((r) => setTimeout(r, 25));
    expect(sent.some((s) => s.msg.id === 'no')).toBe(false);
    const out = sent.find((s) => s.msg.id === 'yes')!.msg as { ok: boolean; envelope: Envelope };
    expect(out.ok).toBe(true);
    expect(out.envelope.kind).toBe('password');
  }, 30_000);

  test('the Sytue ID account page sends the envelope with the change, and the change even without it', async () => {
    const { e2e } = await signedIn('cuenta.js');
    const handlers: Record<string, (e: unknown) => void> = {};
    const fields: Record<string, { value: string; disabled?: boolean }> = {
      'input[name=current_password]': { value: vectors.password },
      'input[name=new_password]': { value: 'otra contraseña larga ñ€' },
      'input[name=unlock_envelope]': { value: '' },
      'button[type=submit]': { value: '' },
    };
    let submitted = 0;
    const form = {
      getAttribute: (n: string) => (n === 'data-sub' ? vectors.user_id : null),
      addEventListener: (type: string, fn: (e: unknown) => void) => (handlers[type] = fn),
      querySelector: (q: string) => fields[q] ?? null,
    };
    e2e.passwordChangeForm(form, { submit: () => submitted++ });
    let prevented = false;
    handlers.submit!({ defaultPrevented: false, preventDefault: () => (prevented = true) });
    expect(prevented).toBe(true);
    for (let i = 0; i < 400 && !submitted; i++) await new Promise((r) => setTimeout(r, 25));
    expect(submitted).toBe(1);
    const env = JSON.parse(fields['input[name=unlock_envelope]']!.value) as Envelope;
    const kek = await e2e.passwordKEK('otra contraseña larga ñ€', passwordKDF);
    expect(b64(await e2e.openEnvelope(kek, vectors.user_id, env))).toBe(vectors.unlock_key);
  }, 30_000);
});

describe('the way back to a mobile app (ADR 0085)', () => {
  const android =
    'Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Mobile Safari/537.36';
  const iphone =
    'Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1';
  const next = 'sytue-mail://auth/callback?code=c0de&state=st4te';

  test('on Android it leaves by itself as an intent to the app scheme, and the button keeps the scheme', () => {
    const e2e = load(fakeWindow(), 'comun.js', 'relay.js');
    const to = e2e.returnTo(next, 'B10B', android);
    expect(to.auto).toBe(
      'intent://auth/callback?code=c0de&state=st4te&sytue_unlock=B10B#Intent;scheme=sytue-mail;end',
    );
    expect(to.manual).toBe(next + '&sytue_unlock=B10B');
  });

  test('everywhere else, and for the web or the desktop loopback, the URL is left as it is', () => {
    const e2e = load(fakeWindow(), 'comun.js', 'relay.js');
    for (const [url, ua] of [
      [next, iphone],
      ['https://mail.sytue.eu/auth/callback?code=c', android],
      ['http://127.0.0.1:8921/auth/callback?code=c', android],
    ] as const) {
      const to = e2e.returnTo(url, '', ua);
      expect(to).toEqual({ auto: url, manual: url });
    }
    // A delivery is added with the right separator.
    expect(e2e.returnTo('sytue://auth/callback', 'X', iphone).auto).toBe(
      'sytue://auth/callback?sytue_unlock=X',
    );
  });

  test('the relay page uses it for both ways back', () => {
    const relay = source('relay.js');
    expect(relay).toContain('E.returnTo(');
    expect(relay).toContain('link.href = to.manual');
    expect(relay).toContain('global.location.replace(to.auto)');
  });
});
