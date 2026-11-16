import { afterEach, beforeAll, expect, test } from 'bun:test';
import { MatrixClient, MatrixEvent, MatrixError } from 'matrix-js-sdk';
import { logger } from 'matrix-js-sdk/lib/logger';
import { RustBackupManager } from 'matrix-js-sdk/lib/rust-crypto/backup';
import { RoomEncryptor } from 'matrix-js-sdk/lib/rust-crypto/RoomEncryptor';
import { OutgoingRequestProcessor } from 'matrix-js-sdk/lib/rust-crypto/OutgoingRequestProcessor';
import { RustCrypto } from 'matrix-js-sdk/lib/rust-crypto/rust-crypto';
import { AllDevicesIsolationMode } from 'matrix-js-sdk/lib/crypto-api';
import {
  BackupDecryptionKey,
  DecryptionSettings,
  DeviceId,
  OlmMachine,
  RoomId,
  TrustRequirement,
  UserId,
  initAsync,
} from '@matrix-org/matrix-sdk-crypto-wasm';

// Real Rust encryption and the SDK request/ack processor. The HTTP seam controls
// backup failures and races; full trust/recovery is covered by the Synapse proof.
beforeAll(async () => {
  await initAsync();
});
const machines: OlmMachine[] = [];
afterEach(() => {
  for (const machine of machines.splice(0)) machine.close();
});
const gate = (): { promise: Promise<void>; resolve: () => void } => {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
};
async function fixture() {
  const machine = await OlmMachine.initialize(
    new UserId('@backup:test'),
    new DeviceId('BACKUP_TEST'),
  );
  machines.push(machine);
  const key = BackupDecryptionKey.createRandomKey();
  await machine.enableBackupV1(key.megolmV1PublicKey.publicKeyBase64, '1');
  const entered = gate();
  const release = gate();
  const uploads: Record<string, unknown>[] = [];
  let inFlight = 0;
  let maximumInFlight = 0;
  let reject = false;
  const http = {
    authedRequest: async (_method: unknown, path: string, _query: unknown, raw: string) => {
      expect(path).toBe('/_matrix/client/v3/room_keys/keys');
      maximumInFlight = Math.max(maximumInFlight, ++inFlight);
      entered.resolve();
      await release.promise;
      inFlight--;
      if (reject)
        throw new MatrixError({ errcode: 'M_FORBIDDEN', error: 'synthetic backup failure' }, 403);
      uploads.push(JSON.parse(raw) as Record<string, unknown>);
      return JSON.stringify({ count: 1, etag: 'accepted' });
    },
  };
  const processor = new OutgoingRequestProcessor(logger, machine, http as never);
  const manager = new RustBackupManager(logger, machine, http as never, processor);
  // Isolate publication ordering from server trust discovery in this unit proof.
  const state = manager as unknown as {
    activeBackupVersion: string;
    checkKeyBackupAndEnable: () => Promise<unknown>;
    uploadBackupBatch: () => Promise<unknown>;
  };
  state.activeBackupVersion = '1';
  state.checkKeyBackupAndEnable = async () => ({ trustInfo: { trusted: true } });
  const roomID = '!backup:test';
  const room = {
    roomId: roomID,
    getJoinedMembers: () => [],
    getEncryptionTargetMembers: async () => [],
    shouldEncryptForInvitedMembers: () => false,
    getHistoryVisibility: () => 'shared',
    getBlacklistUnverifiedDevices: () => false,
    getMyMembership: () => 'join',
  };
  const encryptor = new RoomEncryptor(
    logger,
    machine,
    { ensureSessionsForUsers: async () => {} } as never,
    { doProcessOutgoingRequests: async () => {}, outgoingRequestProcessor: processor } as never,
    room as never,
    { algorithm: 'm.megolm.v1.aes-sha2' },
  );
  const crypto = {
    getOlmMachineOrThrow: () => machine,
    roomEncryptors: { [roomID]: encryptor },
    globalBlacklistUnverifiedDevices: true,
    deviceIsolationMode: new AllDevicesIsolationMode(true),
    requireKeyBackupBeforeSending: false,
    flushKeyBackup: () => manager.flushKeyBackup(),
    getActiveSessionBackupVersion: () => manager.getActiveBackupVersion(),
  };
  RustCrypto.prototype.setRequireKeyBackupBeforeSending.call(crypto as never, true);
  const event = () =>
    new MatrixEvent({
      room_id: roomID,
      type: 'm.room.message',
      content: { msgtype: 'm.text', body: 'private publication' },
    });
  return {
    machine,
    key,
    manager,
    state,
    encryptor,
    crypto,
    event,
    entered,
    release,
    uploads,
    maximumInFlight: () => maximumInFlight,
    fail: () => {
      reject = true;
    },
    allow: () => {
      reject = false;
    },
    encrypt: (value: MatrixEvent) =>
      RustCrypto.prototype.encryptEvent.call(crypto as never, value, room as never),
  };
}

test('publication waits for the exact session backup acknowledgement', async () => {
  const f = await fixture();
  const event = f.event();
  let published = false;
  const pending = f.encrypt(event).then(() => {
    published = true;
  });
  await f.entered.promise;
  expect(published).toBe(false);
  expect(f.uploads).toHaveLength(0);
  const escaped = await Promise.race([
    pending.then(() => true),
    new Promise<boolean>((done) => setTimeout(() => done(false), 30)),
  ]);
  expect(escaped).toBe(false);
  f.release.resolve();
  await pending;
  expect(published).toBe(true);
  const wire = event.getWireContent();
  const rooms = f.uploads[0]!.rooms as Record<
    string,
    {
      sessions: Record<
        string,
        { session_data: { ephemeral: string; mac: string; ciphertext: string } }
      >;
    }
  >;
  const data = rooms['!backup:test']!.sessions[wire.session_id as string]!.session_data;
  const recovered = JSON.parse(
    f.key.decryptV1(data.ephemeral, data.mac, data.ciphertext),
  ) as Record<string, unknown>;
  const fresh = await OlmMachine.initialize(new UserId('@backup:test'), new DeviceId('FRESH_TEST'));
  machines.push(fresh);
  await fresh.importExportedRoomKeys(
    JSON.stringify([
      {
        ...recovered,
        room_id: '!backup:test',
        session_id: wire.session_id,
        sender_key: wire.sender_key,
      },
    ]),
    () => {},
  );
  const decoded = await fresh.decryptRoomEvent(
    JSON.stringify({
      type: 'm.room.encrypted',
      sender: '@backup:test',
      event_id: '$backed:test',
      origin_server_ts: 1,
      content: wire,
    }),
    new RoomId('!backup:test'),
    new DecryptionSettings(TrustRequirement.Untrusted),
  );
  expect((JSON.parse(decoded.event) as { content: { body: string } }).content.body).toBe(
    'private publication',
  );
  expect(await f.machine.backupRoomKeys()).toBeUndefined();
});

test('foreground and background uploads share one acknowledged queue', async () => {
  const f = await fixture();
  const event = f.event();
  const pending = f.encrypt(event);
  await f.entered.promise;
  const background = f.state.uploadBackupBatch();
  const secondForeground = f.manager.flushKeyBackup();
  f.release.resolve();
  await Promise.all([pending, background, secondForeground]);
  expect(f.maximumInFlight()).toBe(1);
  expect(f.uploads).toHaveLength(1);
});

test('backup failure rejects publication and permits a later safe retry', async () => {
  const f = await fixture();
  f.fail();
  const pending = f.encrypt(f.event());
  await f.entered.promise;
  f.release.resolve();
  await expect(pending).rejects.toThrow();
  expect(f.uploads).toHaveLength(0);
  f.allow();
  await f.encrypt(f.event());
  expect(f.uploads).toHaveLength(1);
});

test('backup version change during acknowledgement rejects publication', async () => {
  const f = await fixture();
  const pending = f.encrypt(f.event());
  await f.entered.promise;
  f.state.activeBackupVersion = '2';
  f.release.resolve();
  await expect(pending).rejects.toThrow('Key backup changed');
});

test('membership change during backup rejects publication', async () => {
  const f = await fixture();
  const pending = f.encrypt(f.event());
  await f.entered.promise;
  f.encryptor.onRoomMembership({ membership: 'leave' } as never);
  f.release.resolve();
  await expect(pending).rejects.toThrow('Room authority changed');
});

test('session retirement during backup rejects publication', async () => {
  const f = await fixture();
  const pending = f.encrypt(f.event());
  await f.entered.promise;
  await f.encryptor.forceDiscardSession();
  f.release.resolve();
  await expect(pending).rejects.toThrow('Room authority changed');
});

test('an inactive backup never permits publication', async () => {
  const f = await fixture();
  await f.machine.disableBackup();
  await expect(f.encrypt(f.event())).rejects.toThrow('trusted key backup');
  expect(f.uploads).toHaveLength(0);
});

test('publication waits for the client provenance guard after acknowledged backup', async () => {
  const f = await fixture();
  const entered = gate(),
    release = gate();
  RustCrypto.prototype.setEncryptedEventPublicationGuard.call(f.crypto as never, async (event) => {
    expect(event.getWireContent().algorithm).toBe('m.megolm.v1.aes-sha2');
    expect(f.uploads).toHaveLength(1);
    entered.resolve();
    await release.promise;
  });
  const pending = f.encrypt(f.event());
  await f.entered.promise;
  f.release.resolve();
  await entered.promise;
  expect(
    await Promise.race([
      pending.then(() => true),
      new Promise<boolean>((done) => setTimeout(() => done(false), 30)),
    ]),
  ).toBe(false);
  release.resolve();
  await pending;
});

test('failed provenance storage rejects publication', async () => {
  const f = await fixture();
  RustCrypto.prototype.setEncryptedEventPublicationGuard.call(f.crypto as never, async () => {
    throw new Error('synthetic provenance persistence failure');
  });
  f.release.resolve();
  await expect(f.encrypt(f.event())).rejects.toThrow('provenance persistence failure');
});

test('backup rotation during provenance persistence rejects publication', async () => {
  const f = await fixture();
  RustCrypto.prototype.setEncryptedEventPublicationGuard.call(f.crypto as never, async () => {
    f.state.activeBackupVersion = '2';
  });
  f.release.resolve();
  await expect(f.encrypt(f.event())).rejects.toThrow('Key backup changed');
});

test('cached encrypted events retain their original trusted backup and room witness', async () => {
  const f = await fixture(),
    event = f.event();
  f.release.resolve();
  await f.encrypt(event);
  await RustCrypto.prototype.revalidateEncryptedEventPublication.call(f.crypto as never, event);
  f.state.activeBackupVersion = '2';
  await expect(
    RustCrypto.prototype.revalidateEncryptedEventPublication.call(f.crypto as never, event),
  ).rejects.toThrow('Original trusted backup');
});
test('cached event is rejected after membership or session retirement without re-encryption', async () => {
  for (const retire of ['membership', 'session']) {
    const f = await fixture(),
      event = f.event();
    f.release.resolve();
    await f.encrypt(event);
    if (retire === 'membership') f.encryptor.onRoomMembership({ membership: 'leave' } as never);
    else await f.encryptor.forceDiscardSession();
    await expect(
      RustCrypto.prototype.revalidateEncryptedEventPublication.call(f.crypto as never, event),
    ).rejects.toThrow('original publication witness');
  }
});
test('failed original backup cannot turn its already-encrypted cache into a publication permit', async () => {
  const f = await fixture(),
    event = f.event();
  f.fail();
  f.release.resolve();
  await expect(f.encrypt(event)).rejects.toThrow();
  f.allow();
  await expect(
    RustCrypto.prototype.revalidateEncryptedEventPublication.call(f.crypto as never, event),
  ).rejects.toThrow('original publication witness');
});
test('HTTP event boundary rechecks every cached retry and carries the exact job abort signal', async () => {
  const event = new MatrixEvent({
    room_id: '!http:test',
    type: 'm.room.encrypted',
    content: { algorithm: 'm.megolm.v1.aes-sha2', ciphertext: 'opaque' },
  });
  event.setTxnId('stable-http-transaction');
  const abort = new AbortController();
  let checks = 0,
    requests = 0;
  const client = {
    logger: { debug: () => {} },
    http: {
      authedRequest: async (
        _method: unknown,
        _path: unknown,
        _query: unknown,
        content: Record<string, unknown>,
        options: { abortSignal: AbortSignal },
      ) => {
        requests++;
        expect(options.abortSignal).toBe(abort.signal);
        expect(content.test_opaque_admission).toBe('opaque');
        return { event_id: '$accepted' };
      },
    },
  };
  MatrixClient.prototype.setEventHttpGuard.call(client as never, async (candidate) => {
    checks++;
    if (checks > 1) throw new Error('original policy revoked');
    candidate.getWireContent().test_opaque_admission = 'opaque';
    return abort.signal;
  });
  const send = () =>
    (
      MatrixClient.prototype as unknown as {
        sendEventHttpRequest: (event: MatrixEvent) => Promise<unknown>;
      }
    ).sendEventHttpRequest.call(client, event);
  await send();
  await expect(send()).rejects.toThrow('policy revoked');
  expect(requests).toBe(1);
  expect(checks).toBe(2);
  MatrixClient.prototype.setEventHttpGuard.call(client as never, async () => abort.signal);
  abort.abort();
  await expect(send()).rejects.toThrow();
  expect(requests).toBe(1);
});
