/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import type {
  StorageRootEnrollment,
  StorageRootAuthority,
  StorageRootService,
  StorageControlAction,
} from '@sytue/config/matrix';
import {
  checkedStorageAction,
  ensureStorageRoot,
  storageControlDigest,
  storageControlIntent,
} from './storage-authority.js';
import { MatrixSecurityError } from './types.js';

function fixture() {
  const root: StorageRootEnrollment = Object.freeze({
    v: 1,
    scope: Object.freeze({ tenant: 'acme', subject: 'alice', product: 'mail' as const }),
    user_id: '@sytue_storage_mail_acme_alice:matrix.test',
    target: Object.freeze({ room_id: '!inbox:matrix.test', device_id: 'DEVICE' }),
    root_key: 'A'.repeat(43),
    generation: 1,
  });
  const absent = new Error('specific enrollment conflict');
  const state = {
    active: true,
    approvals: 0,
    challenges: 0,
    signatures: 0,
    confirmations: 0,
    readError: absent as unknown,
    authority: null as StorageRootAuthority | null,
  };
  const assertOpen = (signal: AbortSignal) => {
    signal.throwIfAborted();
    if (!state.active) throw new MatrixSecurityError('device');
  };
  const device = {
    assertOpen,
    ownRoot: async () => root,
    approve: async () => {
      state.approvals++;
    },
    sign: async (_intent: unknown, action: StorageControlAction) => {
      state.signatures++;
      return {
        ...action,
        signatures: { [root.user_id]: { 'ed25519:DEVICE': 'synthetic signature' } },
      };
    },
  };
  const service: StorageRootService = {
    read: async () => {
      if (state.readError) throw state.readError;
      return state.authority!;
    },
    enrollmentRequired: (error) => error === absent,
    challenge: async (epoch, kind, request) => {
      state.challenges++;
      const intent = await storageControlIntent(request, epoch, kind),
        now = Date.now();
      return {
        request,
        action: {
          v: 1,
          ...intent,
          root_key: root.root_key,
          nonce: 'challenge_fixture',
          issued_at: now,
          expires_at: now + 60_000,
        },
      };
    },
    confirm: async (proof) => {
      state.confirmations++;
      return {
        v: 1,
        scope: root.scope,
        user_id: root.user_id,
        root_key: root.root_key,
        generation: proof.authority_generation + (proof.kind === 'root.resume' ? 1 : 0),
        status: 'active',
      };
    },
  };
  return { root, state, device, service, signal: new AbortController().signal };
}

test('storage root is enrolled only after explicit approval of this SDK request', async () => {
  const f = fixture(),
    result = await ensureStorageRoot(f.device, f.service, f.signal);
  expect(result.generation).toBe(1);
  expect([
    f.state.approvals,
    f.state.challenges,
    f.state.signatures,
    f.state.confirmations,
  ]).toEqual([1, 1, 1, 1]);
});

test('an account change during root consent stops before a challenge or signature', async () => {
  const f = fixture();
  f.device.approve = async () => {
    f.state.active = false;
  };
  await expect(ensureStorageRoot(f.device, f.service, f.signal)).rejects.toThrow('matrix_device');
  expect([f.state.challenges, f.state.signatures, f.state.confirmations]).toEqual([0, 0, 0]);
});

test('offline and unknown errors cannot become first root enrollment', async () => {
  for (const error of [new Error('offline'), { status: 404 }, { status: 403 }, { status: 500 }]) {
    const f = fixture();
    f.state.readError = error;
    await expect(ensureStorageRoot(f.device, f.service, f.signal)).rejects.toBe(error);
    expect(f.state.approvals).toBe(0);
  }
});

test('matching active roots do not ask again; revoked roots resume with the retained generation', async () => {
  for (const status of ['active', 'revoked'] as const) {
    const f = fixture();
    f.state.readError = null;
    f.state.authority = {
      v: 1,
      scope: f.root.scope,
      user_id: f.root.user_id,
      root_key: f.root.root_key,
      generation: 2,
      status,
    };
    const result = await ensureStorageRoot(f.device, f.service, f.signal);
    expect(result.generation).toBe(status === 'active' ? 2 : 3);
    expect(f.state.approvals).toBe(status === 'active' ? 0 : 1);
  }
});

test('a different root cannot become an implicit reset or rotation', async () => {
  const f = fixture();
  f.state.readError = null;
  f.state.authority = {
    v: 1,
    scope: f.root.scope,
    user_id: f.root.user_id,
    root_key: 'B'.repeat(43),
    generation: 2,
    status: 'active',
  };
  await expect(ensureStorageRoot(f.device, f.service, f.signal)).rejects.toThrow(
    'matrix_unverified',
  );
  expect([f.state.approvals, f.state.signatures]).toEqual([0, 0]);
});

test('root challenges cannot substitute generation, request, target, root or purpose', async () => {
  const f = fixture(),
    intent = await storageControlIntent(
      f.root,
      '11111111-1111-4111-8111-111111111111',
      'root.enroll',
    ),
    now = Date.now();
  const action: StorageControlAction = {
    v: 1,
    ...intent,
    root_key: f.root.root_key,
    nonce: 'nonce',
    issued_at: now,
    expires_at: now + 60_000,
  };
  for (const patch of [
    { authority_generation: 2 },
    { request_sha256: 'b'.repeat(64) },
    { target: { ...action.target, device_id: 'OTHER' } },
    { root_key: 'B'.repeat(43) },
    { kind: 'key.enroll' as const },
    { expires_at: now - 1 },
    { extra: true },
  ]) {
    expect(() => checkedStorageAction({ ...action, ...patch }, intent, f.root.root_key)).toThrow(
      'matrix_unverified',
    );
  }
});

test('canonical control digests ignore object insertion order without changing Unicode content', async () => {
  expect(await storageControlDigest({ z: 'é<𐐀', a: 1 })).toBe(
    await storageControlDigest({ a: 1, z: 'é<𐐀' }),
  );
  expect(await storageControlDigest({ a: 1, z: 'é<𐐀' })).not.toBe(
    await storageControlDigest({ a: 1, z: 'e<𐐀' }),
  );
});
