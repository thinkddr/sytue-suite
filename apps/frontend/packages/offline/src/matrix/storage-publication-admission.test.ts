/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { MatrixEvent } from 'matrix-js-sdk';
import {
  StoragePublicationAdmission,
  StoragePublicationPending,
  type StorageAdmissionContext,
} from './storage-publication-admission.js';
import type { PqcRuntime } from '../pqc/runtime.js';
const scope = { tenant: 'fixture', subject: 'owner', product: 'mail' };
const room = '!private:matrix.test',
  user = '@owner:matrix.test',
  sha = 'a'.repeat(64);
function fixture(fault?: string, status = 'accepted') {
  let checked = 0,
    accepted = 0,
    valid = true;
  const buffers: Uint8Array[] = [];
  const bytes = () => {
    const value = Uint8Array.of(1, 2, 3);
    buffers.push(value);
    return value;
  };
  const invoke: Pick<PqcRuntime, 'invoke'>['invoke'] = async <T>(
    request: Record<string, unknown>,
  ): Promise<T> => {
    switch (request.operation) {
      case 'publication.authority.decode':
        return { root_key: 'A'.repeat(43), user_id: user } as T;
      case 'publication.recovery.decode':
        return {
          status,
          cipher_sha256: sha,
          device_id: 'ORIGINAL',
          reference: status === 'accepted' ? { room_id: room, event_id: '$accepted' } : undefined,
        } as T;
      case 'publication.wire.digest':
        expect(request.require_marker).toBe(true);
        return { sha256: fault === 'cipher' ? 'b'.repeat(64) : sha } as T;
      default:
        throw Error('unlisted core operation');
    }
  };
  const context: StorageAdmissionContext = {
    scope,
    userID: user,
    deviceID: 'CURRENT',
    rootKey: 'A'.repeat(43),
    runtime: { invoke },
    revalidate: async () => {
      checked++;
      if (!valid) throw Error('session_retired');
    },
    authority: async () => bytes(),
    reserve: async () => {
      throw Error('recovery cannot reserve');
    },
    sign: async () => {
      throw Error('recovery cannot sign');
    },
    recovery: async () => (fault === 'absent' ? undefined : bytes()),
    accepted: async () => {
      accepted++;
      if (fault === 'late-owner') valid = false;
      return new MatrixEvent({
        event_id: fault === 'event' ? '$foreign' : '$accepted',
        room_id: fault === 'room' ? '!foreign:matrix.test' : room,
        sender: fault === 'sender' ? '@foreign:matrix.test' : user,
        type: fault === 'type' ? 'm.room.message' : 'm.room.encrypted',
        content: {
          device_id: fault === 'device' ? 'OTHER' : 'ORIGINAL',
          ciphertext: 'fixture-only-public-cipher',
        },
        origin_server_ts: 1,
      });
    },
  };
  return {
    admission: new StoragePublicationAdmission(context),
    buffers,
    state: () => ({ checked, accepted }),
  };
}
test('public recovery hints grant no fresh reservation or send; only exact SDK wire returns an old reference', async () => {
  const f = fixture();
  expect(
    await f.admission.recoverBeforeSend(room, 'original-transaction', new AbortController().signal),
  ).toEqual({ room_id: room, event_id: '$accepted' });
  expect(f.state().accepted).toBe(1);
  expect(f.state().checked).toBeGreaterThanOrEqual(3);
  expect(f.buffers.every((v) => v.every((b) => b === 0))).toBe(true);
  const absent = fixture('absent');
  expect(
    await absent.admission.recoverBeforeSend(
      room,
      'original-transaction',
      new AbortController().signal,
    ),
  ).toBeUndefined();
  expect(absent.state().accepted).toBe(0);
  const pending = fixture(undefined, 'reserved');
  await expect(
    pending.admission.recoverBeforeSend(room, 'original-transaction', new AbortController().signal),
  ).rejects.toBeInstanceOf(StoragePublicationPending);
  expect(pending.state().accepted).toBe(0);
});
test.each(['event', 'room', 'sender', 'device', 'type', 'cipher', 'late-owner'])(
  'recovery refuses substituted %s or a retired original session',
  async (fault) => {
    const f = fixture(fault);
    await expect(
      f.admission.recoverBeforeSend(room, 'original-transaction', new AbortController().signal),
    ).rejects.toThrow();
    expect(f.buffers.every((v) => v.every((b) => b === 0))).toBe(true);
  },
);
