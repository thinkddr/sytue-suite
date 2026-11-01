/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { createClient, EventType, type MatrixEvent, type IEvent } from 'matrix-js-sdk';
import { EventShieldColour, EventShieldReason } from 'matrix-js-sdk/lib/crypto-api';
import type { CryptoBackend } from 'matrix-js-sdk/lib/common-crypto/CryptoBackend';
import {
  SECRET_STORAGE_ALGORITHM_V1_AES,
  ServerSideSecretStorageImpl,
  type AccountDataClient,
} from 'matrix-js-sdk/lib/secret-storage';
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

async function createSsssHarness() {
  const accountData = new Map<string, Record<string, unknown>>();
  const rawKey = new Uint8Array(32);
  for (let i = 0; i < rawKey.length; i++) rawKey[i] = (i * 17 + 0x29) & 0xff;
  const adapter = {
    getAccountDataFromServer: async (eventType: string) =>
      structuredClone(accountData.get(eventType) ?? null),
    setAccountData: async (eventType: string, content: Record<string, unknown>) => {
      accountData.set(eventType, structuredClone(content));
      return {};
    },
  } as unknown as AccountDataClient;
  const secretStorage = new ServerSideSecretStorageImpl(adapter, {
    getSecretStorageKey: async ({ keys }) => {
      const id = Object.keys(keys)[0];
      return id ? [id, rawKey.slice() as Uint8Array<ArrayBuffer>] : null;
    },
  });
  const added = await secretStorage.addKey(
    SECRET_STORAGE_ALGORITHM_V1_AES,
    { key: rawKey.slice() as Uint8Array<ArrayBuffer>, name: 'sytue-vault-test' },
    'default_vault_key',
  );
  await secretStorage.setDefaultKeyId(added.keyId);
  return { accountData, secretStorage, keyId: added.keyId };
}

test('SSSS vault binds secret ID, scope, revision and authenticated ciphertext', async () => {
  const { accountData, secretStorage, keyId } = await createSsssHarness();
  const { instance } = fixture();
  Object.assign(instance, {
    trusted: async () => true,
  });
  Object.assign(instance.client, { secretStorage });

  expect(await instance.readSecret('vault_item')).toBeNull();

  const secretPayload = new TextEncoder().encode('{"pin":"987654","amount":9007199254740993}');
  await instance.putSecret('vault_item', secretPayload, 2);

  const stored = accountData.get('org.sytue.vault.vault_item') as {
    encrypted: Record<string, { iv: string; ciphertext: string; mac: string }>;
  };
  expect(JSON.stringify(stored)).not.toContain('987654');

  const loaded = await instance.readSecret('vault_item', 2);
  expect(loaded?.revision).toBe(2);
  expect(new TextDecoder().decode(loaded?.payload)).toBe(
    '{"pin":"987654","amount":9007199254740993}',
  );

  // Rollback protection: demanding minRevision 3 rejects revision 2.
  await expect(instance.readSecret('vault_item', 3)).rejects.toThrow('matrix_identity');

  // Event-name / HKDF info binding: moving ciphertext under another vault key fails.
  accountData.set('org.sytue.vault.moved_item', structuredClone(stored));
  await expect(instance.readSecret('moved_item', 1)).rejects.toThrow('matrix_identity');

  // Cross-product scope substitution fails even with valid SSSS key.
  const mailInstance = Object.assign(Object.create(SuiteMatrix.prototype), {
    bootstrap: { ...structuredClone(bootstrap), storage_product: 'mail' },
    closed: false,
    trusted: async () => true,
    client: { secretStorage },
  }) as SuiteMatrix;
  await expect(mailInstance.readSecret('vault_item', 1)).rejects.toThrow('matrix_identity');

  // Tampering with the SSSS IV fails HMAC/envelope verification.
  const tampered = structuredClone(stored);
  const entry = tampered.encrypted[keyId]!;
  entry.iv = (entry.iv.startsWith('A') ? 'B' : 'A') + entry.iv.slice(1);
  accountData.set('org.sytue.vault.vault_item', tampered);
  await expect(instance.readSecret('vault_item', 1)).rejects.toThrow('matrix_identity');
});

test('recovered private storage accepts SSSS-attested owner sessions and rejects unattested backup injections', async () => {
  const { secretStorage } = await createSsssHarness();
  const writer = fixture();
  writer.state.raw = {
    type: String(EventType.RoomMessageEncrypted),
    room_id: room,
    sender: own,
    event_id: '$record',
    content: {
      algorithm: 'm.megolm.v1.aes-sha2',
      session_id: 'session_verified_owner_1',
      sender_key: 'owner',
      ciphertext: 'opaque_ciphertext',
    },
  };
  Object.assign(writer.instance, {
    trusted: async () => true,
    crypto: {
      ...writer.instance['crypto'],
      checkKeyBackupAndEnable: async () => ({
        backupInfo: { algorithm: 'm.megolm_backup.v1.curve25519-aes-sha2', version: '1' },
        trustInfo: { trusted: true, matchesDecryptionKey: true },
      }),
    },
  });
  Object.assign(writer.instance.client, { secretStorage });

  const saved = await writer.instance.storage().putRecord('opaque', record());
  await writer.instance.backup();

  // Simulate fresh recovered device where Megolm backup restores with AUTHENTICITY_NOT_GUARANTEED.
  const reader = fixture();
  reader.state.shield = EventShieldColour.GREY;
  reader.state.reason = EventShieldReason.AUTHENTICITY_NOT_GUARANTEED;
  reader.state.raw = structuredClone(writer.state.raw);
  Object.assign(reader.instance.client, { secretStorage });

  const recovered = await reader.instance.storage().readRecord(saved, 'object', 'content');
  expect(new TextDecoder().decode(storageJSONBytes(recovered.payload))).toBe(
    '{"amount":900719925474099312345}',
  );

  // Unattested Curve25519 backup injection (different session_id or sender_key) is rejected!
  reader.state.raw = {
    ...reader.state.raw,
    content: {
      algorithm: 'm.megolm.v1.aes-sha2',
      session_id: 'session_unauthenticated_backup_injected',
      sender_key: 'owner',
      ciphertext: 'opaque_ciphertext',
    },
  };
  await expect(reader.instance.storage().readRecord(saved, 'object', 'content')).rejects.toThrow(
    'matrix_unverified',
  );

  // Non-backup shield warnings (e.g. UNSIGNED_DEVICE) still fail even for an attested session!
  reader.state.raw = structuredClone(writer.state.raw);
  reader.state.shield = EventShieldColour.RED;
  reader.state.reason = EventShieldReason.UNSIGNED_DEVICE;
  await expect(reader.instance.storage().readRecord(saved, 'object', 'content')).rejects.toThrow(
    'matrix_unverified',
  );
});
