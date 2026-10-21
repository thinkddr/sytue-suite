/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { isMatrixRoomId, reference, validateBootstrap, type MatrixBootstrap } from './types.js';
import {
  RoomsMatrix,
  safeAttachmentType,
  validCallContent,
  type MatrixCallKeyContent,
} from './client.js';
const bootstrap: MatrixBootstrap = {
  actor: '8612ced0-e445-4ab5-8bcd-34a9b776c0d2',
  tenant: 'acme',
  homeserver_url: 'https://matrix.sytue.eu',
  sso_provider_id: 'oidc-sytue',
  matrix_user_id: '@sytue_8612ced0-e445-4ab5-8bcd-34a9b776c0d2:matrix.sytue.eu',
  controller_user_id: '@rooms_controller:matrix.sytue.eu',
  channels: [{ channel_id: 'channel-one', matrix_room_id: '!one:matrix.sytue.eu' }],
  encryption: 'm.megolm.v1.aes-sha2',
};
test('bootstrap binds the Matrix identity to the Sytue actor and approved HTTPS homeserver', () => {
  expect(validateBootstrap(bootstrap)).toEqual(bootstrap);
  for (const homeserver_url of [
    'http://matrix.sytue.eu',
    'https://user:password@matrix.sytue.eu',
    'https://matrix.sytue.eu/path',
    'https://matrix.sytue.eu/?token=secret',
  ]) {
    expect(() => validateBootstrap({ ...bootstrap, homeserver_url })).toThrow();
  }
  expect(() =>
    validateBootstrap({ ...bootstrap, controller_user_id: bootstrap.matrix_user_id }),
  ).toThrow();
  expect(() =>
    validateBootstrap({ ...bootstrap, matrix_user_id: '@another:matrix.sytue.eu' }),
  ).toThrow();
  expect(() =>
    validateBootstrap({ ...bootstrap, channels: [...bootstrap.channels, ...bootstrap.channels] }),
  ).toThrow();
});
test('opaque event references reject missing room and malformed event IDs', () => {
  expect(reference({ room_id: '!one:matrix.sytue.eu', event_id: '$event' })).toEqual({
    room_id: '!one:matrix.sytue.eu',
    event_id: '$event',
  });
  for (const value of [
    null,
    {},
    { room_id: 'one', event_id: '$event' },
    { room_id: '!one:matrix.sytue.eu', event_id: 'event' },
  ])
    expect(reference(value)).toBeUndefined();
});
test('v12 room IDs work in bootstrap, event references and private room validation', () => {
  const room = `!${'aB_9-'.repeat(8)}XYZ`;
  expect(isMatrixRoomId(room)).toBe(true);
  expect(
    validateBootstrap({
      ...bootstrap,
      channels: [{ channel_id: 'channel-one', matrix_room_id: room }],
    }).channels[0]?.matrix_room_id,
  ).toBe(room);
  expect(reference({ room_id: room, event_id: '$event' })).toEqual({
    room_id: room,
    event_id: '$event',
  });
});
test('room IDs reject malformed v12 digests, whitespace, controls and oversized legacy IDs', () => {
  for (const room of [
    `!${'a'.repeat(42)}`,
    `!${'a'.repeat(44)}`,
    `!${'a'.repeat(42)}+`,
    `!${'a'.repeat(42)}/`,
    `!${'a'.repeat(42)}=`,
    '!one:matrix.sytue.eu\n',
    '!one\x00:matrix.sytue.eu',
    '!one:matrix.sytue.eu\x7f',
    `!${'a'.repeat(240)}:matrix.sytue.eu`,
  ]) {
    expect(isMatrixRoomId(room)).toBe(false);
    expect(reference({ room_id: room, event_id: '$event' })).toBeUndefined();
    expect(() =>
      validateBootstrap({
        ...bootstrap,
        channels: [{ channel_id: 'channel-one', matrix_room_id: room }],
      }),
    ).toThrow('matrix_room');
  }
});
test('decrypted active documents cannot execute on the Rooms origin', () => {
  for (const type of ['text/html', 'image/svg+xml', 'application/xhtml+xml'])
    expect(safeAttachmentType(type)).toBe('application/octet-stream');
  expect(safeAttachmentType('image/png')).toBe('image/png');
});
test('call keys reject expiry, malformed key material and keys in requests', () => {
  const key: MatrixCallKeyContent = {
    v: 1,
    call_id: 'call',
    generation: 1,
    algorithm: 'AES-GCM',
    expires_at: Date.now() + 60000,
    key_b64: 'A'.repeat(43) + '=',
  };
  expect(validCallContent('org.sytue.rooms.call_key.v1', key)).toBe(true);
  expect(
    validCallContent('org.sytue.rooms.call_key.v1', { ...key, expires_at: Date.now() - 1 }),
  ).toBe(false);
  expect(validCallContent('org.sytue.rooms.call_key.v1', { ...key, key_b64: 'short' })).toBe(false);
  expect(validCallContent('org.sytue.rooms.call_key_request.v1', key)).toBe(false);
});

test('setting up recovery cannot silently replace an existing identity whose private keys are unavailable', async () => {
  let wroteSecrets = false;
  const instance = Object.assign(Object.create(RoomsMatrix.prototype), {
    bootstrap,
    crypto: {
      getUserDeviceInfo: async () => new Map(),
      getCrossSigningStatus: async () => ({
        publicKeysOnDevice: true,
        privateKeysInSecretStorage: false,
        privateKeysCachedLocally: {
          masterKey: false,
          selfSigningKey: false,
          userSigningKey: false,
        },
      }),
      bootstrapSecretStorage: async () => {
        wroteSecrets = true;
      },
    },
  }) as RoomsMatrix;
  await expect(instance.setupRecovery()).rejects.toThrow('matrix_recovery');
  expect(wroteSecrets).toBe(false);
});

test('a Rooms author cannot be relabelled independently of the encrypted Matrix sender', async () => {
  let decrypted = false;
  const instance = Object.assign(Object.create(RoomsMatrix.prototype), {
    bootstrap,
    room: async () => '!one:matrix.sytue.eu',
    client: {
      fetchRoomEvent: async () => ({
        type: 'm.room.encrypted',
        room_id: '!one:matrix.sytue.eu',
        event_id: '$signed-event',
        sender: '@sytue_other-actor:matrix.sytue.eu',
      }),
      decryptEventIfNeeded: async () => {
        decrypted = true;
      },
    },
  }) as RoomsMatrix;
  await expect(
    instance.read(
      'channel-one',
      { room_id: '!one:matrix.sytue.eu', event_id: '$signed-event' },
      bootstrap.actor,
    ),
  ).rejects.toThrow('matrix_reference');
  expect(decrypted).toBe(false);
});
