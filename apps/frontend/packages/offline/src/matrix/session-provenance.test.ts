/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import type { PqcRuntime } from '../pqc/runtime.js';
import { MatrixSessionProvenance, type MatrixSessionDescriptor } from './session-provenance.js';

// Orchestration only: the actual canonical Go codec is exercised in Node/Wasm
// and the real Matrix browser/native proof. This fixture defines no cipher/hash.
function fixture() {
  const values = new Map<string, { payload: Uint8Array; revision: number }>();
  const payloads = new Map<string, Uint8Array>();
  let reads = 0,
    writes = 0,
    active = true;
  const core = {
    invoke: async <T>(request: Record<string, unknown>): Promise<T> => {
      const session = JSON.parse(
        new TextDecoder().decode(request.session as Uint8Array),
      ) as MatrixSessionDescriptor;
      const id = 'artificial-' + session.session_id;
      const payload =
        payloads.get(id) ??
        new TextEncoder().encode('artificial canonical tuple ' + session.session_id);
      payloads.set(id, payload);
      if (request.operation === 'matrix.session.proof.encode')
        return { id, payload: payload.slice() } as T;
      if (request.operation === 'matrix.session.proof.decode') {
        const actual = request.payload as Uint8Array;
        return (actual.length === payload.length &&
          actual.every((byte, index) => byte === payload[index])) as T;
      }
      throw new Error('unexpected orchestration operation');
    },
  } as unknown as PqcRuntime;
  const vault = {
    assertOpen: () => {
      if (!active) throw new Error('device closed');
    },
    read: async (id: string) => {
      reads++;
      const value = values.get(id);
      return value ? { payload: value.payload.slice(), revision: value.revision } : null;
    },
    write: async (id: string, payload: Uint8Array) => {
      writes++;
      values.set(id, { payload: payload.slice(), revision: 1 });
    },
  };
  const proof = new MatrixSessionProvenance(
    core,
    { tenant: 'artificial', subject: 'owner', product: 'rooms' },
    vault,
  );
  const session = (id: string): MatrixSessionDescriptor => ({
    room_id: '!room:artificial',
    session_id: id,
    sender_key: 'SDK sender',
    sender_user: '@owner:artificial',
    signing_key: 'SDK signing key',
  });
  return {
    proof,
    session,
    values,
    vault,
    reads: () => reads,
    writes: () => writes,
    close: () => {
      active = false;
      proof.close();
    },
  };
}

test('session provenance never truncates historical SSSS entries when its hot cache evicts', async () => {
  const f = fixture();
  for (let index = 0; index < 514; index++) await f.proof.attest(f.session(String(index)));
  expect(f.values.size).toBe(514);
  const before = f.reads();
  expect(await f.proof.contains(f.session('0'))).toBe(true);
  expect(f.reads()).toBe(before + 1);
  expect(f.values.size).toBe(514);
  const writes = f.writes();
  await f.proof.attest(f.session('0'));
  expect(f.writes()).toBe(writes);
});

test('a missing proof is rechecked after another authorized device supplies it', async () => {
  const f = fixture();
  expect(await f.proof.contains(f.session('late'))).toBe(false);
  await f.proof.attest(f.session('late'));
  expect(await f.proof.contains(f.session('late'))).toBe(true);
});

test('invalid or changed SSSS provenance never gets overwritten or accepted', async () => {
  const f = fixture();
  f.values.set('artificial-bad', { payload: new Uint8Array([0]), revision: 1 });
  await expect(f.proof.attest(f.session('bad'))).rejects.toThrow('matrix_unverified');
  expect(f.writes()).toBe(0);
  expect(f.values.get('artificial-bad')!.payload).toEqual(new Uint8Array([0]));
  f.values.set('artificial-revision', { payload: new Uint8Array([0]), revision: 2 });
  await expect(f.proof.contains(f.session('revision'))).rejects.toThrow('matrix_unverified');
});

test('late vault output is wiped and never becomes authenticated after close', async () => {
  const f = fixture();
  let entered!: () => void, release!: () => void;
  const started = new Promise<void>((done) => {
    entered = done;
  });
  const gate = new Promise<void>((done) => {
    release = done;
  });
  const late = new Uint8Array([7, 8, 9]);
  f.vault.read = async () => {
    entered();
    await gate;
    return { payload: late, revision: 1 };
  };
  const pending = f.proof.contains(f.session('closed'));
  await started;
  f.close();
  release();
  await expect(pending).rejects.toThrow('matrix_device');
  expect(late).toEqual(new Uint8Array(3));
  expect(f.writes()).toBe(0);
});
