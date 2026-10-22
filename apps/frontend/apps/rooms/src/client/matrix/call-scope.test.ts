/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { RoomsMatrix, type MatrixCallKeyContent } from './client.js';
import { validateCallScope, type MatrixCallScope, type MatrixCallMember } from './types.js';

const actor = '11111111-1111-4111-8111-111111111111';
const other = '22222222-2222-4222-8222-222222222222';
const device = 'ARTIFICIAL_LOCAL';
const member: MatrixCallMember = { subject: other, device_id: 'ARTIFICIAL_REMOTE' };
function scope(): MatrixCallScope {
  return {
    call_id: '33333333-3333-4333-8333-333333333333',
    matrix_room_id: `!${'a'.repeat(43)}`,
    livekit_room: 'artificial-call-generation-2',
    generation: 2,
    expires_at_ms: Date.now() + 60000,
    leader_subject: actor,
    leader_device_id: device,
    members: [{ subject: actor, device_id: device }, member],
  };
}
test('call bindings reject expired leases, missing devices, duplicate subjects and unbound leaders', () => {
  const binding = scope();
  expect(validateCallScope(binding, binding.call_id, actor, device)).toBe(binding);
  for (const value of [
    { ...binding, expires_at_ms: Date.now() - 1 },
    { ...binding, expires_at_ms: Date.now() + 20 * 60 * 1000 },
    { ...binding, members: [member] },
    { ...binding, members: [...binding.members, { subject: actor, device_id: 'REPLACED_DEVICE' }] },
    { ...binding, leader_device_id: 'RETIRED_DEVICE' },
  ])
    expect(() => validateCallScope(value, binding.call_id, actor, device)).toThrow();
  expect(() => validateCallScope(binding, binding.call_id, actor, 'RETIRED_DEVICE')).toThrow(
    'matrix_identity',
  );
});

test('call secrets use the SDK Olm API for exact leased devices and never a room event', async () => {
  const binding = scope();
  const calls: { type: string; devices: { userId: string; deviceId: string }[] }[] = [];
  let queued = 0;
  const instance = Object.assign(Object.create(RoomsMatrix.prototype), {
    bootstrap: { actor, homeserver_url: 'https://matrix.sytue.test' },
    client: {
      getDeviceId: () => device,
      queueToDevice: async () => {
        queued++;
      },
      sendEvent: () => {
        throw new Error('A call key must never be sent through Megolm');
      },
    },
    prepareCallScope: async () => undefined,
    callDevice: async () => undefined,
    crypto: {
      encryptToDeviceMessages: async (
        type: string,
        devices: { userId: string; deviceId: string }[],
      ) => {
        calls.push({ type, devices });
        return { batch: devices.map(() => ({})) };
      },
    },
  }) as RoomsMatrix;
  const key: MatrixCallKeyContent = {
    v: 1,
    call_id: binding.call_id,
    generation: binding.generation,
    algorithm: 'AES-GCM',
    expires_at: binding.expires_at_ms,
    key_b64: 'A'.repeat(43) + '=',
  };
  await instance.sendEncryptedCustom(binding, 'org.sytue.rooms.call_key.v1', key);
  expect(calls).toEqual([
    {
      type: 'org.sytue.rooms.call_key.v1',
      devices: [{ userId: `@sytue_${other}:matrix.sytue.test`, deviceId: member.device_id }],
    },
  ]);
  expect(queued).toBe(1);
  await expect(
    instance.sendEncryptedCustom(binding, 'org.sytue.rooms.call_key.v1', key, {
      ...member,
      device_id: 'RETIRED_DEVICE',
    }),
  ).rejects.toThrow('matrix_identity');
  expect(calls).toHaveLength(1);
  await expect(
    instance.sendEncryptedCustom(
      { ...binding, leader_subject: other, leader_device_id: member.device_id },
      'org.sytue.rooms.call_key.v1',
      key,
    ),
  ).rejects.toThrow('matrix_identity');
  expect(calls).toHaveLength(1);
});

test('a missing Olm recipient cannot silently turn into a partial call-key distribution', async () => {
  const binding = scope();
  let queued = false;
  const instance = Object.assign(Object.create(RoomsMatrix.prototype), {
    bootstrap: { actor, homeserver_url: 'https://matrix.sytue.test' },
    client: {
      getDeviceId: () => device,
      queueToDevice: async () => {
        queued = true;
      },
    },
    prepareCallScope: async () => undefined,
    callDevice: async () => undefined,
    crypto: { encryptToDeviceMessages: async () => ({ batch: [] }) },
  }) as RoomsMatrix;
  const key: MatrixCallKeyContent = {
    v: 1,
    call_id: binding.call_id,
    generation: binding.generation,
    algorithm: 'AES-GCM',
    expires_at: binding.expires_at_ms,
    key_b64: 'A'.repeat(43) + '=',
  };
  await expect(
    instance.sendEncryptedCustom(binding, 'org.sytue.rooms.call_key.v1', key),
  ).rejects.toThrow('matrix_device');
  expect(queued).toBe(false);
});
