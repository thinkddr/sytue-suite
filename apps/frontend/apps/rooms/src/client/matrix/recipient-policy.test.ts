/* SPDX-License-Identifier: Apache-2.0 */
import { beforeAll, expect, test } from 'bun:test';
import {
  CollectStrategy,
  DecryptionSettings,
  DeviceId,
  DeviceLists,
  EncryptionAlgorithm,
  EncryptionSettings,
  HistoryVisibility,
  OlmMachine,
  OtherUserIdentity,
  RequestType,
  RoomId,
  TrustRequirement,
  UserId,
  initAsync,
} from '@matrix-org/matrix-sdk-crypto-wasm';
import { RoomsMatrix } from './client.js';

beforeAll(async () => {
  await initAsync();
});

type WireObject = Record<string, unknown>;
interface FixtureUser {
  id: string;
  device: string;
  machine: OlmMachine;
  keys: WireObject;
  signing: WireObject;
  oneTime: WireObject;
}
async function user(id: string): Promise<FixtureUser> {
  const device = 'ARTIFICIAL_TEST_DEVICE';
  const machine = await OlmMachine.initialize(new UserId(id), new DeviceId(device));
  const signing = await machine.bootstrapCrossSigning(true);
  let keys: WireObject = {},
    oneTime: WireObject = {};
  const uploads = (await machine.outgoingRequests()).filter(
    (request) => request.type === RequestType.KeysUpload,
  );
  if (signing.uploadKeysRequest) uploads.push(signing.uploadKeysRequest);
  for (const request of uploads) {
    const uploadRequest = request as { body: string; id: string; type: RequestType };
    const upload = JSON.parse(uploadRequest.body) as {
      device_keys?: WireObject;
      one_time_keys?: WireObject;
    };
    keys = upload.device_keys ?? keys;
    oneTime = { ...oneTime, ...upload.one_time_keys };
    await machine.markRequestAsSent(
      uploadRequest.id,
      uploadRequest.type,
      JSON.stringify({ one_time_key_counts: { signed_curve25519: Object.keys(oneTime).length } }),
    );
  }
  const signatures = JSON.parse(signing.uploadSignaturesRequest.body) as Record<
    string,
    Record<string, { signatures?: WireObject }>
  >;
  const signed = signatures[id]?.[device];
  if (signed?.signatures) {
    const original = (keys.signatures ?? {}) as Record<string, Record<string, string>>;
    const added = signed.signatures as Record<string, Record<string, string>>;
    keys.signatures = { ...original, [id]: { ...original[id], ...added[id] } };
  }
  const result = {
    id,
    device,
    machine,
    keys,
    signing: JSON.parse(signing.uploadSigningKeysRequest.body) as WireObject,
    oneTime,
  };
  signing.free();
  return result;
}
async function query(reader: FixtureUser, users: FixtureUser[]): Promise<void> {
  const request = reader.machine.queryKeysForUsers(users.map((entry) => new UserId(entry.id)));
  await reader.machine.markRequestAsSent(
    request.id,
    RequestType.KeysQuery,
    JSON.stringify({
      device_keys: Object.fromEntries(
        users.map((entry) => [entry.id, { [entry.device]: entry.keys }]),
      ),
      master_keys: Object.fromEntries(users.map((entry) => [entry.id, entry.signing.master_key])),
      self_signing_keys: Object.fromEntries(
        users.map((entry) => [entry.id, entry.signing.self_signing_key]),
      ),
    }),
  );
  request.free();
}
async function establishOlm(reader: FixtureUser, users: FixtureUser[]): Promise<void> {
  const request = await reader.machine.getMissingSessions(
    users.map((entry) => new UserId(entry.id)),
  );
  if (!request) return;
  await reader.machine.markRequestAsSent(
    request.id,
    RequestType.KeysClaim,
    JSON.stringify({
      one_time_keys: Object.fromEntries(
        users.map((entry) => [
          entry.id,
          { [entry.device]: Object.fromEntries(Object.entries(entry.oneTime).slice(0, 1)) },
        ]),
      ),
    }),
  );
  request.free();
}

test('official trusted-only Megolm policy excludes a self-cross-signed controller, including key requests', async () => {
  const sender = await user('@sytue_11111111-1111-4111-8111-111111111111:matrix.test');
  const human = await user('@sytue_22222222-2222-4222-8222-222222222222:matrix.test');
  const controller = await user('@rooms_controller:matrix.test');
  const users = [sender, human, controller];
  const room = '!artificial:matrix.test';
  const settings = new EncryptionSettings();
  settings.algorithm = EncryptionAlgorithm.MegolmV1AesSha2;
  settings.historyVisibility = HistoryVisibility.Joined;
  // This is the official strategy used by AllDevicesIsolationMode(true) + globalBlacklistUnverifiedDevices.
  settings.sharingStrategy = CollectStrategy.deviceBasedStrategy(true, true);
  try {
    for (const entry of users) await query(entry, users);
    const humanIdentity = await sender.machine.getIdentity(new UserId(human.id));
    if (!(humanIdentity instanceof OtherUserIdentity))
      throw new Error('Missing artificial human identity');
    const verification = await humanIdentity.verify();
    const signatures = JSON.parse(verification.body) as Record<string, Record<string, WireObject>>;
    const signedMaster = Object.values(signatures[human.id] ?? {})[0];
    if (!signedMaster) throw new Error('Missing artificial human verification signature');
    human.signing.master_key = signedMaster;
    verification.free();
    humanIdentity.free();
    await query(sender, users);
    const verified = await sender.machine.getIdentity(new UserId(human.id));
    expect(verified?.isVerified()).toBe(true);
    verified?.free();
    const controllerDevice = await sender.machine.getDevice(
      new UserId(controller.id),
      new DeviceId(controller.device),
    );
    if (!controllerDevice) throw new Error('Missing artificial controller device');
    expect(controllerDevice.isCrossSignedByOwner()).toBe(true);
    expect(controllerDevice.isCrossSigningTrusted()).toBe(false);
    controllerDevice.free();
    await establishOlm(sender, [human, controller]);
    const requests = await sender.machine.shareRoomKey(
      new RoomId(room),
      users.map((entry) => new UserId(entry.id)),
      settings,
    );
    let humanReceived = false;
    for (const request of requests) {
      const payload = JSON.parse(request.body) as {
        messages: Record<string, Record<string, unknown>>;
      };
      if (request.event_type === 'm.room.encrypted') {
        expect(payload.messages[controller.id]).toBeUndefined();
        for (const content of Object.values(payload.messages[human.id] ?? {})) {
          humanReceived = true;
          await human.machine.receiveSyncChangesMsc4186(
            JSON.stringify([{ type: request.event_type, sender: sender.id, content }]),
            new DeviceLists(),
            new Map(),
          );
        }
      }
      await sender.machine.markRequestAsSent(request.id, RequestType.ToDevice, '{}');
      request.free();
    }
    expect(humanReceived).toBe(true);
    const encrypted = JSON.parse(
      await sender.machine.encryptRoomEvent(
        new RoomId(room),
        'm.room.message',
        JSON.stringify({ msgtype: 'm.text', body: 'ARTIFICIAL PRIVATE CONTENT' }),
      ),
    ) as WireObject;
    const event = JSON.stringify({
      type: 'm.room.encrypted',
      content: encrypted,
      sender: sender.id,
      event_id: '$artificial',
      origin_server_ts: 1792152000000,
    });
    const decryption = new DecryptionSettings(TrustRequirement.Untrusted);
    try {
      const result = await human.machine.decryptRoomEvent(event, new RoomId(room), decryption);
      expect(JSON.parse(result.event).content.body).toBe('ARTIFICIAL PRIVATE CONTENT');
      result.free();
      await expect(
        controller.machine.decryptRoomEvent(event, new RoomId(room), decryption),
      ).rejects.toThrow();
    } finally {
      decryption.free();
    }
    // A joined controller can ask for the key, but it has never been verified by the human sender.
    await sender.machine.receiveSyncChangesMsc4186(
      JSON.stringify([
        {
          type: 'm.room_key_request',
          sender: controller.id,
          content: {
            action: 'request',
            requesting_device_id: controller.device,
            request_id: 'artificial-controller-request',
            body: {
              algorithm: 'm.megolm.v1.aes-sha2',
              room_id: room,
              sender_key: sender.machine.identityKeys.curve25519.toBase64(),
              session_id: encrypted.session_id,
            },
          },
        },
      ]),
      new DeviceLists(),
      new Map(),
    );
    for (const request of await sender.machine.outgoingRequests()) {
      if (request.type === RequestType.ToDevice) {
        const payload = JSON.parse(request.body) as { messages: Record<string, unknown> };
        expect(payload.messages[controller.id]).toBeUndefined();
      }
      request.free();
    }
  } finally {
    settings.free();
    for (const entry of users) entry.machine.close();
  }
}, 15000);

test('verification never approves the provisioning controller or arbitrary Matrix identities', async () => {
  let requested = false;
  const instance = Object.assign(Object.create(RoomsMatrix.prototype), {
    bootstrap: { homeserver_url: 'https://matrix.sytue.eu' },
    crypto: {
      requestDeviceVerification: async () => {
        requested = true;
      },
    },
  }) as RoomsMatrix;
  for (const id of [
    '@rooms_controller:matrix.sytue.eu',
    '@sytue_not-a-human:matrix.sytue.eu',
    '@sytue_11111111-1111-4111-8111-111111111111:other.test',
  ]) {
    expect(instance.canVerifyUser(id)).toBe(false);
    await expect(instance.verifyRecipient(id, 'DEVICE')).rejects.toThrow('matrix_unverified');
  }
  expect(
    instance.canVerifyUser('@sytue_11111111-1111-4111-8111-111111111111:matrix.sytue.eu'),
  ).toBe(true);
  expect(requested).toBe(false);
});

test('a previously trusted controller fails closed before any new session can be shared', async () => {
  for (const identityTrusted of [true, false]) {
    let discarded = false;
    const instance = Object.assign(Object.create(RoomsMatrix.prototype), {
      bootstrap: { controller_user_id: '@rooms_controller:matrix.sytue.eu' },
      policyRooms: new Set(),
      crypto: {
        getUserVerificationStatus: async () => ({ isCrossSigningVerified: () => identityTrusted }),
        getUserDeviceInfo: async () =>
          new Map([['@rooms_controller:matrix.sytue.eu', new Map([['OLDDEVICE', {}]])]]),
        getDeviceVerificationStatus: async () => ({ isVerified: () => true }),
        forceDiscardSession: async () => {
          discarded = true;
        },
      },
    }) as RoomsMatrix;
    const policy = instance as unknown as { applyRecipientPolicy: (room: string) => Promise<void> };
    await expect(policy.applyRecipientPolicy('!artificial:matrix.test')).rejects.toThrow(
      'matrix_identity',
    );
    expect(discarded).toBe(false);
  }
});
