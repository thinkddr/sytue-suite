/* SPDX-License-Identifier: Apache-2.0 */
// The browser wrapping reproduces the shared vectors of the desktop implementation
// (apps/nucleo/recoverywrap) byte for byte: wrap key, AAD and ciphertext.
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { MatrixCustodyError } from './custody.js';
import {
  fromBase64,
  openRecoveryWrapper,
  passwordIKM,
  recoveryWrapAAD,
  recoveryWrapKeyBytes,
  sealRecoveryKey,
  sealWithUnlockKey,
  unlockHint,
  type RecoveryWrapper,
} from './recovery-wrap.js';

interface Vector {
  name: string;
  kind: 'password' | 'passkey';
  account: string;
  key_id: string;
  credential_id: string;
  password?: string;
  prf_output?: string;
  recovery_key: string;
  wrap_key: string;
  aad: string;
  wrapper: RecoveryWrapper;
}

const vectors = JSON.parse(
  readFileSync(
    join(import.meta.dir, '../../../../../nucleo/recoverywrap/testdata/vectors.json'),
    'utf8',
  ),
) as Vector[];
const hex = (bytes: Uint8Array) => Buffer.from(bytes).toString('hex');

async function ikm(vector: Vector, password = vector.password): Promise<Uint8Array<ArrayBuffer>> {
  if (vector.kind === 'passkey') return fromBase64(vector.prf_output!);
  if (vector.wrapper.kdf.alg !== 'argon2id') throw new Error('vector kdf');
  return passwordIKM(password!, vector.wrapper.kdf);
}

test('there are vectors for both kinds', () => {
  expect(new Set(vectors.map((v) => v.kind))).toEqual(new Set(['password', 'passkey']));
});

for (const vector of vectors) {
  test(`vector ${vector.name}: wrap key, AAD and ciphertext match`, async () => {
    const material = await ikm(vector);
    expect(hex(await recoveryWrapKeyBytes(vector.kind, material))).toBe(vector.wrap_key);
    expect(
      hex(recoveryWrapAAD(vector.kind, vector.account, vector.key_id, vector.credential_id)),
    ).toBe(vector.aad);
    const sealed = await sealRecoveryKey({
      kind: vector.kind,
      account: vector.account,
      keyID: vector.key_id,
      credentialID: vector.credential_id,
      kdf: vector.wrapper.kdf,
      ikm: material,
      recoveryKey: vector.recovery_key,
      nonce: fromBase64(vector.wrapper.nonce),
    });
    expect(sealed.ciphertext).toBe(vector.wrapper.ciphertext);
    expect(await openRecoveryWrapper(vector.wrapper, vector.account, material)).toBe(
      vector.recovery_key,
    );
  }, 30_000);

  test(`vector ${vector.name}: a wrong factor or another account does not open it`, async () => {
    const wrong =
      vector.kind === 'password'
        ? await ikm(vector, vector.password + ' ')
        : new Uint8Array(32).fill(7);
    await expect(openRecoveryWrapper(vector.wrapper, vector.account, wrong)).rejects.toEqual(
      new MatrixCustodyError('wrong'),
    );
    await expect(
      openRecoveryWrapper(vector.wrapper, vector.account + 'x', await ikm(vector)),
    ).rejects.toEqual(new MatrixCustodyError('wrong'));
  }, 30_000);
}

/** ADR 0084: the `sytue-id` wrapper, sealed with the unlock key of the Sytue ID sign-in. */
const unlock = JSON.parse(
  readFileSync(
    join(import.meta.dir, '../../../../../nucleo/recoverywrap/testdata/desbloqueo.json'),
    'utf8',
  ),
) as {
  unlock_key: string;
  hint: string;
  account: string;
  key_id: string;
  recovery_key: string;
  sytue_id_wrap_key: string;
  sytue_id_wrapper: RecoveryWrapper;
};

test('the sytue-id wrapper matches the Go vector: hint, wrap key and ciphertext', async () => {
  const key = fromBase64(unlock.unlock_key);
  expect(await unlockHint(key)).toBe(unlock.hint);
  expect(hex(await recoveryWrapKeyBytes('sytue-id', key))).toBe(unlock.sytue_id_wrap_key);
  const sealed = await sealWithUnlockKey({
    unlockKey: key,
    account: unlock.account,
    keyID: unlock.key_id,
    recoveryKey: unlock.recovery_key,
    nonce: fromBase64(unlock.sytue_id_wrapper.nonce),
  });
  expect(sealed).toEqual(unlock.sytue_id_wrapper);
  expect(await openRecoveryWrapper(unlock.sytue_id_wrapper, unlock.account, key)).toBe(
    unlock.recovery_key,
  );
  await expect(
    openRecoveryWrapper(unlock.sytue_id_wrapper, unlock.account, new Uint8Array(32)),
  ).rejects.toEqual(new MatrixCustodyError('wrong'));
});
