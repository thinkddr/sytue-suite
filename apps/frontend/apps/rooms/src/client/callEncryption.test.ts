/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { ExternalE2EEKeyProvider } from 'livekit-client';
import { CallEncryption } from './callEncryption.js';
import { RoomsMatrix, setRoomsMatrix, type MatrixCallKeyContent } from './matrix/client.js';
import type { MatrixCallScope } from './matrix/types.js';

const actor = '11111111-1111-4111-8111-111111111111';
const other = '22222222-2222-4222-8222-222222222222';
const device = 'ARTIFICIAL_LOCAL';
const call = '33333333-3333-4333-8333-333333333333';
const join = '44444444-4444-4444-8444-444444444444';
function binding(): MatrixCallScope {
  return {
    call_id: call,
    matrix_room_id: `!${'a'.repeat(43)}`,
    livekit_room: 'artificial-generation-1',
    generation: 1,
    expires_at_ms: Date.now() + 60000,
    leader_subject: actor,
    leader_device_id: device,
    members: [
      { subject: actor, device_id: device },
      { subject: other, device_id: 'ARTIFICIAL_REMOTE' },
    ],
  };
}
function matrix(trace: string[]): void {
  let receive: ((content: MatrixCallKeyContent) => void) | undefined;
  const instance = Object.assign(Object.create(RoomsMatrix.prototype), {
    bootstrap: { actor, homeserver_url: 'https://matrix.test' },
    client: { getDeviceId: () => device },
    prepareCallScope: async () => undefined,
    onEncryptedCustom: (
      _scope: MatrixCallScope,
      type: string,
      callback: (content: MatrixCallKeyContent) => void,
    ) => {
      if (type.endsWith('call_key.v1')) receive = callback;
      return () => {
        receive = undefined;
      };
    },
    sendEncryptedCustom: async (scope: MatrixCallScope, type: string) => {
      trace.push(type.endsWith('request.v1') ? 'request-key' : 'share-key');
      if (type.endsWith('request.v1'))
        queueMicrotask(() =>
          receive?.({
            v: 1,
            call_id: call,
            generation: scope.generation,
            algorithm: 'AES-GCM',
            expires_at: scope.expires_at_ms,
            key_b64: 'B'.repeat(43) + '=',
          }),
        );
    },
  }) as RoomsMatrix;
  setRoomsMatrix(instance);
}
function connection(scope: MatrixCallScope) {
  return {
    identity: actor,
    joinId: join,
    encryption: scope,
    room: scope.livekit_room,
    serverUrl: 'wss://artificial.invalid',
    token: 'artificial-test-only',
  };
}

test('call publishing waits for key installation and server ready; a new epoch reconnects before publishing', async () => {
  const trace: string[] = [];
  matrix(trace);
  const previousFetch = globalThis.fetch;
  let scope = binding();
  let readyCalls = 0;
  let denied = false;
  const provider = new ExternalE2EEKeyProvider();
  const setKey = provider.setKey.bind(provider);
  provider.setKey = async (key: string | ArrayBuffer) => {
    trace.push('install-key');
    await setKey(key);
  };
  globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (denied) return new Response('{}', { status: 403 });
    if (!init?.body) return Response.json(scope);
    const body = JSON.parse(String(init.body)) as { action: string };
    if (body.action === 'ready') {
      readyCalls++;
      trace.push('server-ready');
      return Response.json({});
    }
    if (body.action === 'reconnect') return Response.json(connection(scope));
    throw new Error('Unexpected artificial request');
  }) as typeof fetch;
  let encryption: CallEncryption | undefined;
  try {
    encryption = await CallEncryption.prepare(call, connection(scope), provider, {
      pause: async () => {
        trace.push('pause');
      },
      connect: async () => {
        trace.push('connect');
      },
      ready: async () => {
        trace.push('publish');
      },
      failed: () => {
        trace.push('failed');
      },
    });
    expect(trace).toEqual(['pause', 'install-key', 'share-key']);
    expect(readyCalls).toBe(0);
    expect(encryption.ready).toBe(false);
    await encryption.started();
    expect(trace.slice(-2)).toEqual(['server-ready', 'publish']);
    expect(encryption.ready).toBe(true);
    trace.length = 0;
    scope = {
      ...scope,
      generation: 2,
      matrix_room_id: `!${'b'.repeat(43)}`,
      livekit_room: 'artificial-generation-2',
      leader_subject: other,
      leader_device_id: 'ARTIFICIAL_REMOTE',
    };
    await encryption.refresh();
    expect(trace).toEqual([
      'pause',
      'request-key',
      'install-key',
      'connect',
      'server-ready',
      'publish',
    ]);
    denied = true;
    await expect(encryption.refresh()).rejects.toThrow();
    expect(encryption.ready).toBe(false);
    expect(trace.at(-1)).toBe('failed');
  } finally {
    encryption?.close();
    globalThis.fetch = previousFetch;
    setRoomsMatrix(null);
  }
});

test('a short permission lease stops a call at its expiry without waiting for the polling interval', async () => {
  const trace: string[] = [];
  matrix(trace);
  const scope = { ...binding(), expires_at_ms: Date.now() + 200 };
  const encryption = await CallEncryption.prepare(
    call,
    connection(scope),
    new ExternalE2EEKeyProvider(),
    {
      pause: async () => undefined,
      connect: async () => undefined,
      ready: async () => undefined,
      failed: () => {
        trace.push('expired');
      },
    },
  );
  try {
    await new Promise((resolve) => setTimeout(resolve, 250));
    expect(trace).toEqual(['share-key', 'expired']);
    expect(encryption.ready).toBe(false);
  } finally {
    encryption.close();
    setRoomsMatrix(null);
  }
});
