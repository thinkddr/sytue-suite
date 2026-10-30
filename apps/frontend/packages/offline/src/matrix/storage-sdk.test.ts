/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { createClient, EventType, type MatrixEvent, type IEvent } from 'matrix-js-sdk';
import { EventShieldColour, EventShieldReason } from 'matrix-js-sdk/lib/crypto-api';
import type { CryptoBackend } from 'matrix-js-sdk/lib/common-crypto/CryptoBackend';
import { SuiteMatrix } from './client.js';
import { privateDeviceFetch } from './private-policy.js';
import { createStorageRecord, storageJSONBytes, type StorageRecord } from './storage.js';
import type { MatrixBootstrap } from './types.js';
const own = '@sytue_11111111-1111-4111-8111-111111111111:matrix.test';
const room = '!private:matrix.test';
const bootstrap: MatrixBootstrap = {
  actor: '11111111-1111-4111-8111-111111111111',
  tenant: 'acme',
  homeserver_url: 'https://matrix.test',
  matrix_user_id: own,
  controller_user_id: '@rooms_controller:matrix.test',
  channels: [],
  sso_provider_id: 'sytue',
  encryption: 'm.megolm.v1.aes-sha2',
  storage_product: 'drive',
  storage_room_id: room,
  storage_room_ids: [room],
};
const record = (): StorageRecord =>
  createStorageRecord(
    { tenant: bootstrap.tenant, subject: bootstrap.actor, product: 'drive' },
    'object',
    'revision',
    'content',
    new TextEncoder().encode('{"amount":900719925474099312345}'),
  );

/** Transport/policy fixtures only. Independent SDK crypto vectors exercise actual encryption separately. */
function fixture() {
  const state = {
    shield: EventShieldColour.NONE,
    reason: null as EventShieldReason | null,
    raw: {
      type: String(EventType.RoomMessageEncrypted),
      room_id: room,
      sender: own,
      event_id: '$record',
      content: { algorithm: 'artificial-boundary-fixture' },
    } as Partial<IEvent>,
    record: record(),
    posts: 0,
    decrypts: 0,
    next: null as string | null,
    chunk: [] as Partial<IEvent>[],
    forwarded: undefined as string | undefined,
  };
  const backend = {
    decryptEvent: async () => ({
      clearEvent: {
        type: String(EventType.RoomMessage),
        content: { msgtype: 'm.text', body: '', 'org.sytue.storage': state.record },
      },
      keyForwardedBy: state.forwarded,
    }),
  } as CryptoBackend;
  const instance = Object.assign(Object.create(SuiteMatrix.prototype), {
    bootstrap: structuredClone(bootstrap),
    closed: false,
    privateDraftRoom: async () => {},
    crypto: {
      getEncryptionInfoForEvent: async () => ({
        shieldColour: state.shield,
        shieldReason: state.reason,
      }),
    },
    client: {
      sendEvent: async (
        id: string,
        type: string,
        content: { body: string; 'org.sytue.storage': StorageRecord },
        txn: string,
      ) => {
        expect(id).toBe(room);
        expect(type).toBe(String(EventType.RoomMessage));
        expect(content.body).toBe('');
        expect(txn).toBe('sytue-storage-opaque');
        state.posts++;
        state.record = structuredClone(content['org.sytue.storage']);
        return { event_id: '$record' };
      },
      fetchRoomEvent: async () => structuredClone(state.raw),
      decryptEventIfNeeded: async (event: MatrixEvent) => {
        state.decrypts++;
        await event.attemptDecryption(backend);
      },
      createMessagesRequest: async () => ({ chunk: state.chunk, end: state.next }),
    },
  }) as SuiteMatrix;
  return { instance, state };
}

test('the actual SDK HTTP path refuses plaintext application events for private products', async () => {
  const requests: Request[] = [];
  const send = (async (input, init) => {
    requests.push(new Request(input, init));
    return new Response('{"event_id":"$opaque"}', {
      headers: { 'Content-Type': 'application/json' },
    });
  }) as typeof fetch;
  const client = createClient({
    baseUrl: 'https://matrix.test',
    userId: own,
    accessToken: 'artificial-only',
    fetchFn: privateDeviceFetch(
      'https://matrix.test',
      own,
      async () => null,
      () => false,
      send,
    ),
  });
  await expect(
    client.sendEvent(
      room,
      EventType.RoomMessage,
      { msgtype: 'm.text', body: 'must never reach the server' },
      'plain',
    ),
  ).rejects.toThrow();
  expect(requests).toHaveLength(0);
  await client.sendEvent(
    room,
    EventType.RoomMessageEncrypted,
    {
      algorithm: 'm.megolm.v1.aes-sha2',
      ciphertext: 'artificial-ciphertext',
      session_id: 'artificial',
      sender_key: 'artificial',
      device_id: 'artificial',
    },
    'opaque',
  );
  expect(requests).toHaveLength(1);
  expect(await requests[0]?.text()).not.toContain('must never reach the server');
});

test('private adapter writes through SDK, checks exact readback and preserves original JSON bytes', async () => {
  const { instance, state } = fixture();
  const repository = instance.storage();
  const saved = await repository.putRecord('opaque', record());
  expect(saved).toEqual({ room_id: room, event_id: '$record' });
  expect(state.posts).toBe(1);
  expect(
    new TextDecoder().decode(
      storageJSONBytes((await repository.readRecord(saved, 'object', 'content')).payload),
    ),
  ).toBe('{"amount":900719925474099312345}');
});

test('private adapter rejects plaintext, a foreign/controller sender and mismatched encrypted receipts before decryption', async () => {
  const { instance, state } = fixture();
  for (const patch of [
    { type: String(EventType.RoomMessage) },
    { sender: '@coworker:matrix.test' },
    { sender: bootstrap.controller_user_id },
    { room_id: '!foreign:matrix.test' },
    { event_id: '$foreign' },
  ]) {
    const original = state.raw;
    state.raw = { ...original, ...patch };
    await expect(
      instance.storage().readRecord({ room_id: room, event_id: '$record' }, 'object', 'content'),
    ).rejects.toThrow();
    state.raw = original;
  }
  expect(state.decrypts).toBe(0);
  await expect(
    instance
      .storage()
      .readRecord({ room_id: '!foreign:matrix.test', event_id: '$record' }, 'object', 'content'),
  ).rejects.toThrow();
});

test('successful decryption cannot override SDK warnings about private sender authenticity', async () => {
  const { instance, state } = fixture();
  for (const reason of [
    EventShieldReason.UNVERIFIED_IDENTITY,
    EventShieldReason.UNSIGNED_DEVICE,
    EventShieldReason.UNKNOWN_DEVICE,
    EventShieldReason.AUTHENTICITY_NOT_GUARANTEED,
    EventShieldReason.VERIFICATION_VIOLATION,
    EventShieldReason.MISMATCHED_SENDER,
  ]) {
    state.shield = EventShieldColour.RED;
    state.reason = reason;
    await expect(
      instance.storage().readRecord({ room_id: room, event_id: '$record' }, 'object', 'content'),
    ).rejects.toThrow('matrix_unverified');
  }
  state.shield = EventShieldColour.NONE;
  state.reason = null;
  state.forwarded = '@coworker:matrix.test';
  await expect(
    instance.storage().readRecord({ room_id: room, event_id: '$record' }, 'object', 'content'),
  ).rejects.toThrow('matrix_unverified');
});

test('logout during prepare or fetch prevents private output and plaintext history is never an empty catalog', async () => {
  const { instance, state } = fixture();
  Object.assign(instance, {
    privateDraftRoom: async () => {
      Object.assign(instance, { closed: true });
    },
  });
  await expect(instance.storage().putRecord('opaque', record())).rejects.toThrow('matrix_device');
  expect(state.posts).toBe(0);
  Object.assign(instance, { closed: false, privateDraftRoom: async () => {} });
  state.chunk = [{ ...state.raw, type: String(EventType.RoomMessage) }];
  await expect(instance.storage().catalog()).rejects.toThrow('matrix_unverified');
  Object.assign(instance.client, {
    fetchRoomEvent: async () => {
      Object.assign(instance, { closed: true });
      return state.raw;
    },
  });
  await expect(
    instance.storage().readRecord({ room_id: room, event_id: '$record' }, 'object', 'content'),
  ).rejects.toThrow('matrix_device');
});
