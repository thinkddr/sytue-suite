/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { createStorageRecord } from './storage.js';
import {
  decodeOwnerProof,
  sealStorageOwnerRecord,
  verifyStorageOwnerRecord,
  storageRecordDigest,
} from './owner-proof.js';
const scope = { tenant: 'équipe', subject: 'person<>&💻', product: 'drive' as const };
const room = '!privée:matrix.test';
const record = () =>
  createStorageRecord(
    scope,
    'object',
    'revision',
    'content',
    new TextEncoder().encode(
      '{"text":"private <filename> 💻","integer":900719925474099312345,"fraction":1.000000000000000001}',
    ),
    ['$old-é:matrix.test', '$old-two:matrix.test'],
  );
const key = () => ({ id: 'artificial-key', key: crypto.getRandomValues(new Uint8Array(32)) });

test('owner proofs use maintained Matrix SSSS and bind every scoped field and room', async () => {
  const owner = key(),
    value = record(),
    proof = await sealStorageOwnerRecord(room, value, scope, owner);
  await verifyStorageOwnerRecord(room, value, scope, proof, owner);
  const again = await sealStorageOwnerRecord(room, value, scope, owner);
  expect(again.iv).not.toBe(proof.iv);
  expect(again.ciphertext).not.toBe(proof.ciphertext);
  const changed = [
    { ...value, scope: { ...scope, tenant: 'other' } },
    { ...value, scope: { ...scope, subject: 'other' } },
    { ...value, scope: { ...scope, product: 'mail' as const } },
    { ...value, object_id: 'other' },
    { ...value, revision_id: 'other' },
    { ...value, role: 'index' as const },
    { ...value, previous: ['$other:matrix.test'] },
    { ...value, previous: [...value.previous!].reverse() },
    { ...value, payload: btoa('{"integer":900719925474099312346}') },
  ];
  for (const other of changed)
    await expect(verifyStorageOwnerRecord(room, other, other.scope, proof, owner)).rejects.toThrow(
      'matrix_unverified',
    );
  await expect(
    verifyStorageOwnerRecord('!foreign:matrix.test', value, scope, proof, owner),
  ).rejects.toThrow('matrix_unverified');
  expect(JSON.stringify(proof).length).toBeLessThan(512);
  expect(JSON.stringify(proof)).not.toContain('private');
  expect(await storageRecordDigest(room, value, scope)).toMatch(/^[0-9a-f]{64}$/);
});

test('malformed, substituted and forged SSSS proofs never authenticate a private record', async () => {
  const owner = key(),
    value = record(),
    proof = await sealStorageOwnerRecord(room, value, scope, owner);
  for (const bad of [
    null,
    {},
    { ...proof, v: 2 },
    { ...proof, algorithm: 'other' },
    { ...proof, iv: 'AA' },
    { ...proof, iv: proof.iv + '====' },
  ])
    expect(() => decodeOwnerProof(bad)).toThrow('matrix_unverified');
  const iv = Uint8Array.from(atob(proof.iv), (c) => c.charCodeAt(0));
  iv[0] = (iv[0] ?? 0) ^ 1;
  for (const other of [
    { ...proof, key_id: 'other' },
    { ...proof, iv: btoa(String.fromCharCode(...iv)) },
    { ...proof, mac: btoa(String.fromCharCode(...new Uint8Array(32))) },
    { ...proof, ciphertext: btoa(String.fromCharCode(...new Uint8Array(86))) },
  ])
    await expect(verifyStorageOwnerRecord(room, value, scope, other, owner)).rejects.toThrow(
      'matrix_unverified',
    );
  await expect(
    verifyStorageOwnerRecord(room, value, scope, proof, { ...key(), id: owner.id }),
  ).rejects.toThrow('matrix_unverified');
});
