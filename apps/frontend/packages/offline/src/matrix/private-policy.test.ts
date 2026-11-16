/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { createClient, Direction } from 'matrix-js-sdk';
import { DeviceVerificationStatus } from 'matrix-js-sdk/lib/crypto-api';
import { privateDeviceFetch, matrixLifetimeFetch } from './private-policy.js';
import { SuiteMatrix, type MatrixRecoveryUI, type MatrixSessionAdapter } from './client.js';
import { validateBootstrap, matrixStorePrefix, type MatrixBootstrap } from './types.js';

const approved = new DeviceVerificationStatus({ localVerified: true, crossSigningVerified: false });
const unapproved = new DeviceVerificationStatus({
  localVerified: false,
  crossSigningVerified: false,
});
const own = '@sytue_00000000-0000-4000-8000-000000000001:matrix.test';
const target = 'https://matrix.test/_matrix/client/v3/sendToDevice/m.room.encrypted/opaque';
const batch = (user = own, device = 'VERIFIED'): string =>
  JSON.stringify({
    messages: {
      [user]: {
        [device]: { algorithm: 'm.olm.v1.curve25519-aes-sha2', ciphertext: { artificial: true } },
      },
    },
  });
const put = (body = batch()): RequestInit => ({ method: 'PUT', body });
const bootstrap: MatrixBootstrap = {
  actor: '00000000-0000-4000-8000-000000000001',
  tenant: 'acme',
  homeserver_url: 'https://matrix.test',
  matrix_user_id: own,
  controller_user_id: '@rooms_controller:matrix.test',
  sso_provider_id: 'sytue',
  channels: [],
  encryption: 'm.megolm.v1.aes-sha2',
  storage_product: 'drive',
  storage_room_id: '!private:matrix.test',
  storage_room_ids: ['!private:matrix.test'],
};

test('isolated account metadata cannot borrow legacy or foreign recovery stores', async () => {
  const isolated: MatrixBootstrap = {
    ...bootstrap,
    storage_identity_version: 2,
    matrix_user_id: `@sytue_storage_drive_acme_${bootstrap.actor}:matrix.test`,
  };
  expect(validateBootstrap(isolated).storage_identity_version).toBe(2);
  expect(matrixStorePrefix(isolated, 'DEVICE')).not.toBe(matrixStorePrefix(bootstrap, 'DEVICE'));
  for (const invalid of [
    { ...isolated, matrix_user_id: own },
    { ...isolated, storage_product: 'mail' },
    { ...isolated, tenant: 'other' },
    { ...isolated, actor: 'another-person' },
    { ...isolated, storage_product: undefined },
    { ...isolated, storage_identity_version: 1 },
    ...[3, -1, '2', null].map((version) => ({ ...isolated, storage_identity_version: version })),
  ])
    expect(() => validateBootstrap(invalid as MatrixBootstrap)).toThrow('matrix_identity');
  const ui: MatrixRecoveryUI = {
    saveRecoveryKey: async () => {},
    readRecoveryKey: async () => '',
    authenticate: async () => {},
  };
  const adapter: MatrixSessionAdapter = {
    readSession: async () => ({
      user_id: own,
      device_id: 'LEGACY',
      access_token: 'artificial',
      storage_scope: { tenant: 'acme', subject: bootstrap.actor, product: 'drive' },
    }),
    forgetSession: async () => {},
    refreshBootstrap: async () => isolated,
  };
  await expect(SuiteMatrix.open(isolated, ui, adapter)).rejects.toThrow('matrix_identity');
});

test('private key batches reach only concrete verified devices of the owner', async () => {
  const sent: Request[] = [];
  const send = (async (input, init) => {
    sent.push(new Request(input, init));
    return new Response('{}');
  }) as typeof fetch;
  const fetcher = privateDeviceFetch(
    'https://matrix.test',
    own,
    async () => approved,
    () => false,
    send,
  );
  await fetcher(target, put());
  expect(sent).toHaveLength(1);
  expect(sent[0]?.redirect).toBe('error');
  expect(await sent[0]?.text()).toBe(batch());
  for (const user of ['@verified_coworker:matrix.test', '@rooms_controller:matrix.test']) {
    await expect(fetcher(target, put(batch(user)))).rejects.toThrow('matrix_identity');
  }
  for (const device of ['*', 'bad\0device']) {
    await expect(fetcher(target, put(batch(own, device)))).rejects.toThrow('matrix_unverified');
  }
  const unverified = privateDeviceFetch(
    'https://matrix.test',
    own,
    async () => unapproved,
    () => false,
    send,
  );
  await expect(unverified(target, put())).rejects.toThrow('matrix_unverified');
  expect(sent).toHaveLength(1);
});

test('the maintained Matrix HTTP client uses the same recipient guard for real SDK batches', async () => {
  const requests: Request[] = [];
  const send = (async (input, init) => {
    requests.push(new Request(input, init));
    return new Response('{}', { headers: { 'Content-Type': 'application/json' } });
  }) as typeof fetch;
  const client = createClient({
    baseUrl: 'https://matrix.test',
    userId: own,
    accessToken: 'artificial-only',
    fetchFn: privateDeviceFetch(
      'https://matrix.test',
      own,
      async () => approved,
      () => false,
      send,
    ),
  });
  const content = new Map([
    ['VERIFIED', { algorithm: 'm.olm.v1.curve25519-aes-sha2', ciphertext: { artificial: true } }],
  ]);
  await client.sendToDevice('m.room.encrypted', new Map([[own, content]]), 'opaque');
  expect(requests).toHaveLength(1);
  expect(requests[0]?.url).toBe(target);
  await expect(
    client.sendToDevice(
      'm.room.encrypted',
      new Map([['@verified_coworker:matrix.test', content]]),
      'blocked',
    ),
  ).rejects.toThrow();
  expect(requests).toHaveLength(1);
});

test('a trust lookup cannot swap the checked URL, body or headers before dispatch', async () => {
  let release!: (verification: DeviceVerificationStatus | null) => void;
  let entered!: () => void;
  const checking = new Promise<DeviceVerificationStatus | null>((resolve) => {
    release = resolve;
  });
  const ready = new Promise<void>((resolve) => {
    entered = resolve;
  });
  let outgoing: Request | undefined;
  const send = (async (input, init) => {
    outgoing = new Request(input, init);
    return new Response('{}');
  }) as typeof fetch;
  const fetcher = privateDeviceFetch(
    'https://matrix.test',
    own,
    () => {
      entered();
      return checking;
    },
    () => false,
    send,
  );
  const url = new URL(target);
  const headers = new Headers({ Authorization: 'Bearer artificial-original' });
  const init = { ...put(), headers };
  const pending = fetcher(url, init);
  await ready;
  url.hostname = 'foreign.invalid';
  headers.set('Authorization', 'Bearer artificial-changed');
  init.body = batch('@someone_else:matrix.test');
  release(approved);
  await pending;
  expect(outgoing?.url).toBe(target);
  expect(outgoing?.headers.get('Authorization')).toBe('Bearer artificial-original');
  expect(await outgoing?.text()).toBe(batch());
});

test('closing a private client while trust is checked prevents queued key delivery', async () => {
  let release!: (verification: DeviceVerificationStatus | null) => void;
  let closed = false;
  let posts = 0;
  const trust = new Promise<DeviceVerificationStatus | null>((resolve) => {
    release = resolve;
  });
  const send: typeof fetch = Object.assign(
    async () => {
      posts++;
      return new Response('{}');
    },
    { preconnect: () => {} },
  );
  const fetcher = privateDeviceFetch(
    'https://matrix.test',
    own,
    () => trust,
    () => closed,
    send,
  );
  const pending = fetcher(target, put());
  closed = true;
  release(approved);
  await expect(pending).rejects.toThrow('matrix_device');
  expect(posts).toBe(0);
});

test('unknown key routes, plaintext room keys, malformed batches and foreign origins fail closed', async () => {
  let posts = 0;
  const send: typeof fetch = Object.assign(
    async () => {
      posts++;
      return new Response('{}');
    },
    { preconnect: () => {} },
  );
  const fetcher = privateDeviceFetch(
    'https://matrix.test',
    own,
    async () => approved,
    () => false,
    send,
  );
  for (const [url, init] of [
    [target.replace('matrix.test', 'foreign.invalid'), put()],
    [target.replace('m.room.encrypted', 'm.room_key'), put()],
    [target.replace('/v3/', '/unstable/new-protocol/'), put()],
    [target, put('{')],
    [target, put('{}')],
    [target, { method: 'PUT', body: new Uint8Array([1, 2, 3]) }],
  ] as const)
    await expect(fetcher(url, init)).rejects.toThrow();
  expect(posts).toBe(0);
});

test('private bootstraps and sessions cannot reuse a legacy Rooms device or another product/tenant', async () => {
  expect(validateBootstrap(bootstrap).storage_product).toBe('drive');
  const ui: MatrixRecoveryUI = {
    saveRecoveryKey: async () => {},
    readRecoveryKey: async () => '',
    authenticate: async () => {},
  };
  const session = { user_id: own, device_id: 'ROOMS_DEVICE', access_token: 'artificial-token' };
  const adapter: MatrixSessionAdapter = {
    readSession: async () => session,
    forgetSession: async () => {},
    refreshBootstrap: async () => bootstrap,
  };
  await expect(SuiteMatrix.open(bootstrap, ui, adapter)).rejects.toThrow('matrix_identity');
  for (const scope of [
    { product: 'mail', tenant: 'acme', subject: bootstrap.actor },
    { product: 'drive', tenant: 'other', subject: bootstrap.actor },
    { product: 'drive', tenant: 'acme', subject: 'another-person' },
  ] as const) {
    adapter.readSession = async () => ({ ...session, storage_scope: scope });
    await expect(SuiteMatrix.open(bootstrap, ui, adapter)).rejects.toThrow('matrix_identity');
  }
  for (const invalid of [
    { ...bootstrap, channels: [{ channel_id: 'room', matrix_room_id: '!room:matrix.test' }] },
    { ...bootstrap, storage_room_ids: [] },
    { ...bootstrap, storage_room_ids: ['!private:matrix.test', '!private:matrix.test'] },
    { ...bootstrap, tenant: 'acme\0foreign' },
  ])
    expect(() => validateBootstrap(invalid)).toThrow('matrix_identity');
});

test('device lifetime cancels ordinary Matrix response bodies and preserves request cancellation', async () => {
  const lifetime = new AbortController(),
    caller = new AbortController();
  let calls = 0;
  let signal: AbortSignal | undefined;
  const fetcher = privateDeviceFetch(
    'https://matrix.test',
    own,
    async () => approved,
    () => false,
    (async (input, init) => {
      calls++;
      signal = new Request(input, init).signal;
      return new Response(
        new ReadableStream({
          start(controller) {
            signal!.addEventListener('abort', () => controller.error(signal!.reason), {
              once: true,
            });
          },
        }),
      );
    }) as typeof fetch,
    lifetime.signal,
  );
  const response = await fetcher('https://matrix.test/_matrix/client/v3/sync', {
    signal: caller.signal,
  });
  const body = response.text();
  caller.abort(new Error('caller stopped'));
  await expect(body).rejects.toThrow('caller stopped');
  const second = await fetcher('https://matrix.test/_matrix/client/v3/sync');
  const secondBody = second.text();
  lifetime.abort(new Error('device stopped'));
  await expect(secondBody).rejects.toThrow('device stopped');
  await expect(fetcher(target, put())).rejects.toThrow('device stopped');
  expect(calls).toBe(2);
});

test('maintained Matrix HTTP recognizes lifetime abort as cancellation without wrapping it as a connection failure', async () => {
  const lifetime = new AbortController();
  let began!: () => void;
  const started = new Promise<void>((resolve) => {
    began = resolve;
  });
  const fetcher = privateDeviceFetch(
    'https://matrix.test',
    own,
    async () => approved,
    () => false,
    (async (input, init) => {
      const signal = new Request(input, init).signal;
      began();
      return new Promise<Response>((_resolve, reject) =>
        signal.addEventListener(
          'abort',
          () => reject(signal.reason instanceof Error ? signal.reason : new Error('cancelled')),
          { once: true },
        ),
      );
    }) as typeof fetch,
    lifetime.signal,
  );
  const sdk = createClient({
    baseUrl: 'https://matrix.test',
    userId: own,
    deviceId: 'ARTIFICIAL',
    accessToken: 'artificial-only',
    fetchFn: fetcher,
    disableVoip: true,
  });
  const pending = sdk.whoami();
  await started;
  lifetime.abort();
  await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
  sdk.stopClient();
});

test('Rooms history requests use the device lifetime without retrying a revoked read', async () => {
  const lifetime = new AbortController();
  let began!: () => void;
  const started = new Promise<void>((resolve) => {
    began = resolve;
  });
  let calls = 0;
  const sdk = createClient({
    baseUrl: 'https://matrix.test',
    userId: own,
    deviceId: 'ROOMS_HISTORY',
    accessToken: 'artificial-only',
    disableVoip: true,
    fetchFn: matrixLifetimeFetch(lifetime.signal, (async (input, init) => {
      calls++;
      const request = new Request(input, init);
      expect(new URL(request.url).pathname).toBe(
        '/_matrix/client/v3/rooms/!history%3Amatrix.test/messages',
      );
      began();
      return new Promise<Response>((_resolve, reject) =>
        request.signal.addEventListener('abort', () => reject(request.signal.reason), {
          once: true,
        }),
      );
    }) as typeof fetch),
  });
  const pending = sdk.createMessagesRequest('!history:matrix.test', null, 100, Direction.Backward);
  await started;
  lifetime.abort();
  await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
  expect(calls).toBe(1);
  sdk.stopClient();
});
