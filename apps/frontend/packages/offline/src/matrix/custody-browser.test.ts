/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { BrowserKeyCustody } from './custody-browser.js';
import { MatrixCustodyError } from './custody.js';
import type { RecoveryWrapper } from './recovery-wrap.js';

const account = '@sytue_storage_drive_acme_0f3c:sytue.eu';
const key = 'EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP';

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

/** A synthetic authenticator: PRF = SHA-256(credential || salt). */
function authenticator(options: { prf?: boolean; refuse?: string } = {}) {
  const rawId = new Uint8Array([1, 2, 3, 4]);
  const calls: CredentialRequestOptions[] = [];
  return {
    calls,
    get: async (request: CredentialRequestOptions) => {
      calls.push(request);
      if (options.refuse) throw new DOMException('refused', options.refuse);
      const prf = (
        request.publicKey!.extensions as {
          prf: {
            eval?: { first: Uint8Array };
            evalByCredential?: Record<string, { first: Uint8Array }>;
          };
        }
      ).prf;
      const salt = prf.eval?.first ?? Object.values(prf.evalByCredential ?? {})[0]!.first;
      const first = await crypto.subtle.digest('SHA-256', new Uint8Array([...rawId, ...salt]));
      return {
        rawId: rawId.buffer,
        getClientExtensionResults: () =>
          options.prf === false ? {} : { prf: { results: { first } } },
      } as unknown as Credential;
    },
  };
}

test('a password wrap goes to the isolated account and opens only with that password', async () => {
  const s = service();
  const custody = new BrowserKeyCustody({
    base: '/drive',
    isolated: true,
    rpId: 'sytue.eu',
    fetchImpl: s.fetchImpl,
  });
  await custody.wrap(
    key,
    { kind: 'password', password: 'synthetic pass' },
    { account, keyID: 'K1' },
  );
  expect(s.requests[0]).toMatchObject({
    method: 'PUT',
    url: '/drive/api/matrix/recovery-wrappers?identity_version=2',
  });
  // The service never receives the key or the password.
  expect(s.requests[0]!.body).not.toContain('EsT3');
  expect(s.requests[0]!.body).not.toContain('synthetic pass');
  expect(await custody.state(['K1'])).toEqual({ methods: ['password'], legacy: false });
  expect(await custody.state(['other'])).toEqual({ methods: [], legacy: false });
  await expect(
    custody.unwrap({ kind: 'password', password: 'wrong' }, { account, keyIDs: ['K1'] }),
  ).rejects.toEqual(new MatrixCustodyError('wrong'));
  // A wrong attempt locks nothing out: the right one still opens it.
  expect(
    await custody.unwrap(
      { kind: 'password', password: 'synthetic pass' },
      { account, keyIDs: ['K1'] },
    ),
  ).toBe(key);
}, 30_000);

test('a passkey wrap uses the PRF of the chosen credential under the Sytue ID RP', async () => {
  const s = service();
  const a = authenticator();
  const custody = new BrowserKeyCustody({
    base: '/rooms',
    isolated: false,
    rpId: 'sytue.eu',
    fetchImpl: s.fetchImpl,
    credentials: a,
  });
  await custody.wrap(key, { kind: 'passkey' }, { account, keyID: 'K1' });
  expect(s.requests[0]!.url).toBe('/rooms/api/matrix/recovery-wrappers');
  expect(s.stored[0]).toMatchObject({ kind: 'passkey', credential_id: 'AQIDBA' });
  expect(a.calls[0]!.publicKey!.rpId).toBe('sytue.eu');
  expect(await custody.unwrap({ kind: 'passkey' }, { account, keyIDs: ['K1'] })).toBe(key);
  expect(a.calls[1]!.publicKey!.allowCredentials?.length).toBe(1);
});

test('no PRF, a dismissed dialog and a refused upload are told apart', async () => {
  const s = service();
  const options = { base: '/rooms', isolated: false, rpId: 'sytue.eu', fetchImpl: s.fetchImpl };
  await expect(
    new BrowserKeyCustody({ ...options, credentials: authenticator({ prf: false }) }).wrap(
      key,
      { kind: 'passkey' },
      { account, keyID: 'K1' },
    ),
  ).rejects.toEqual(new MatrixCustodyError('unsupported'));
  await expect(
    new BrowserKeyCustody({
      ...options,
      credentials: authenticator({ refuse: 'NotAllowedError' }),
    }).wrap(key, { kind: 'passkey' }, { account, keyID: 'K1' }),
  ).rejects.toEqual(new MatrixCustodyError('cancelled'));
  const refused = service({ putStatus: 503 });
  await expect(
    new BrowserKeyCustody({ ...options, fetchImpl: refused.fetchImpl }).wrap(
      key,
      { kind: 'password', password: 'p' },
      { account, keyID: 'K1' },
    ),
  ).rejects.toEqual(new MatrixCustodyError('unavailable'));
  expect(s.stored).toHaveLength(0);
}, 30_000);

test('the retired service copy is offered only for the Rooms account', async () => {
  const s = service({ legacy: true });
  const rooms = new BrowserKeyCustody({
    base: '/rooms',
    isolated: false,
    rpId: 'x',
    fetchImpl: s.fetchImpl,
  });
  const isolated = new BrowserKeyCustody({
    base: '/drive',
    isolated: true,
    rpId: 'x',
    fetchImpl: s.fetchImpl,
  });
  expect(await rooms.state(undefined)).toEqual({ methods: [], legacy: true });
  expect(await rooms.legacyRecoveryKey()).toBe(key);
  expect(await isolated.state(undefined)).toEqual({ methods: [], legacy: false });
  expect(await isolated.legacyRecoveryKey()).toBeNull();
});
