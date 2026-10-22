/* SPDX-License-Identifier: Apache-2.0 */
import { beforeAll, expect, test } from 'bun:test';
import {
  Attachment,
  DecryptionSettings,
  DecryptionErrorCode,
  DeviceId,
  EncryptedAttachment,
  OlmMachine,
  RoomId,
  TrustRequirement,
  UserId,
  initAsync,
} from '@matrix-org/matrix-sdk-crypto-wasm';
import fixture from '../../../../../../../test/fixtures/rooms-matrix-go.json';

beforeAll(async () => {
  await initAsync();
});

test('RustCrypto decrypts the artificial mautrix/goolm message without claiming a verified sender', async () => {
  expect(fixture.artificial).toBe(true);
  const machine = await OlmMachine.initialize(
    new UserId('@fixture_js_reader:matrix.test'),
    new DeviceId('ARTIFICIALJSREADER'),
  );
  try {
    await machine.importExportedRoomKeys(JSON.stringify(fixture.sessions), () => undefined);
    const event = fixture.events[0];
    if (!event) throw new Error('Missing Go encrypted event');
    const settings = new DecryptionSettings(TrustRequirement.Untrusted);
    const result = await machine
      .decryptRoomEvent(JSON.stringify(event), new RoomId(event.room_id), settings)
      .catch((error: unknown) => {
        const detail = error as { code?: unknown; description?: unknown };
        throw new Error(
          `Artificial fixture decrypt: ${String(detail.code)} ${String(detail.description)}`,
        );
      });
    try {
      expect(JSON.parse(result.event).content.body).toBe(fixture.plaintext);
      expect(result.shieldState(true)).not.toBeNull();
    } finally {
      result.free();
      settings.free();
    }
  } finally {
    machine.close();
  }
});

test('cross-signed-only live policy rejects imported historical keys whose sender cannot be verified', async () => {
  const machine = await OlmMachine.initialize(
    new UserId('@strict_js_reader:matrix.test'),
    new DeviceId('STRICTREADER'),
  );
  try {
    await machine.importExportedRoomKeys(JSON.stringify(fixture.sessions), () => undefined);
    const event = fixture.events[0];
    if (!event) throw new Error('Missing Go encrypted event');
    const settings = new DecryptionSettings(TrustRequirement.CrossSigned);
    try {
      let failure: { code?: unknown } | null = null;
      try {
        await machine.decryptRoomEvent(JSON.stringify(event), new RoomId(event.room_id), settings);
      } catch (error) {
        failure = error as { code?: unknown };
      }
      expect(failure?.code).toBe(DecryptionErrorCode.UnknownSenderDevice);
    } finally {
      settings.free();
    }
  } finally {
    machine.close();
  }
});

test('RustCrypto decrypts the artificial Go Matrix v2 attachment and rejects modified bytes', () => {
  const original = new Uint8Array(Buffer.from(fixture.attachment.ciphertext, 'base64'));
  const attachment = new EncryptedAttachment(
    original,
    JSON.stringify(fixture.attachment.descriptor),
  );
  try {
    expect(Buffer.from(Attachment.decrypt(attachment)).toString('base64')).toBe(
      fixture.attachment.plaintext,
    );
  } finally {
    attachment.free();
  }
  const changed = original.slice();
  changed[0] = (changed[0] ?? 0) ^ 1;
  const tampered = new EncryptedAttachment(changed, JSON.stringify(fixture.attachment.descriptor));
  try {
    expect(() => Attachment.decrypt(tampered)).toThrow();
  } finally {
    tampered.free();
  }
});
