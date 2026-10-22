/* SPDX-License-Identifier: Apache-2.0 */
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import { writeFile } from 'node:fs/promises';
const require = createRequire(
  new URL('../../../apps/frontend/apps/rooms/package.json', import.meta.url),
);
const { Room, RoomEvent, ExternalE2EEKeyProvider, Encryption_Type, setLogLevel } = await import(
  new URL('./livekit-client.esm.mjs', pathToFileURL(require.resolve('livekit-client'))).href
);
const protocol = createRequire(require.resolve('livekit-client'))('@livekit/protocol');
// Capability detection only: no camera, microphone, PeerConnection or transport is created.
globalThis.window = {
  RTCRtpScriptTransform: function () {},
  addEventListener() {},
  removeEventListener() {},
};
setLogLevel('silent');
const worker = new Worker(
  new URL('./livekit-client.e2ee.worker.mjs', pathToFileURL(require.resolve('livekit-client'))),
  { type: 'module' },
);
const keyProvider = new ExternalE2EEKeyProvider();
const room = new Room({ disconnectOnPageLeave: false, encryption: { keyProvider, worker } });
const key = Buffer.from(crypto.getRandomValues(new Uint8Array(32))).toString('base64');
function within(promise, label) {
  let timeout;
  return Promise.race([
    promise,
    new Promise((_, reject) => {
      timeout = setTimeout(() => reject(new Error(label)), 5000);
    }),
  ]).finally(() => clearTimeout(timeout));
}
try {
  room.localParticipant.identity = 'ARTIFICIAL-DEVICE-ONLY';
  await keyProvider.setKey(key);
  const enabled = within(
    new Promise((resolve, reject) => {
      room.once(RoomEvent.ParticipantEncryptionStatusChanged, (value) =>
        value ? resolve() : reject(new Error('Encryption disabled')),
      );
    }),
    'Encryption worker did not become ready',
  );
  await room.setE2EEEnabled(true);
  await enabled;
  const packets = [];
  // Only replace the last network boundary. The real SDK creates the protobuf and real Worker encrypts it.
  room.engine.ensurePublisherConnected = async () => undefined;
  room.engine.reliableChannel.send = async (bytes) => {
    packets.push(Uint8Array.from(bytes));
  };
  const plaintext = new TextEncoder().encode(JSON.stringify({ t: 'mano', alzada: true }));
  await room.localParticipant.publishData(plaintext, { reliable: true });
  assert.equal(packets.length, 1);
  const packet = protocol.DataPacket.fromBinary(packets[0]);
  assert.equal(packet.value.case, 'encryptedPacket');
  const encrypted = packet.value.value;
  assert.equal(encrypted.encryptionType, Encryption_Type.GCM);
  assert.equal(new TextDecoder().decode(packets[0]).includes('alzada'), false);
  const decoded = await within(
    room.engine.e2eeManager.handleEncryptedData(
      encrypted.encryptedValue,
      encrypted.iv,
      room.localParticipant.identity,
      encrypted.keyIndex,
    ),
    'SDK decrypt did not complete',
  );
  const payload = protocol.EncryptedPacketPayload.fromBinary(decoded.payload);
  assert.equal(payload.value.case, 'user');
  assert.deepEqual(Uint8Array.from(payload.value.value.payload), plaintext);
  const changed = encrypted.encryptedValue.slice();
  changed[0] ^= 1;
  const uuid = crypto.randomUUID();
  const rejected = within(
    new Promise((resolve, reject) => {
      const onMessage = ({ data }) => {
        if (data.data?.uuid !== uuid) return;
        worker.removeEventListener('message', onMessage);
        data.kind === 'error' ? resolve() : reject(new Error('Modified ciphertext was accepted'));
      };
      worker.addEventListener('message', onMessage);
      worker.postMessage({
        kind: 'decryptDataRequest',
        data: {
          uuid,
          payload: changed,
          iv: encrypted.iv,
          participantIdentity: room.localParticipant.identity,
          keyIndex: encrypted.keyIndex,
        },
      });
    }),
    'Worker did not reject modified ciphertext',
  );
  await rejected;
  if (process.env.SYTUE_LIVEKIT_FIXTURE)
    await writeFile(
      process.env.SYTUE_LIVEKIT_FIXTURE,
      JSON.stringify(
        {
          artificial: true,
          generator: 'livekit-client 2.22.3 official E2EE worker',
          key_b64: key,
          wire_packet: Buffer.from(packets[0]).toString('base64'),
          plaintext: Buffer.from(plaintext).toString('base64'),
        },
        null,
        2,
      ) + '\n',
    );
  process.stdout.write(
    JSON.stringify({ encrypted: true, gcm: true, recovered: true, tamperingRejected: true }) + '\n',
  );
} finally {
  worker.terminate();
  await room.disconnect();
}
process.exit(0);
