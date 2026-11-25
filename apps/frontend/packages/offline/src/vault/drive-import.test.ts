/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Drive sobre la bóveda de punta a punta (v3.24.0, 2026-11-24): la puerta de Drive
 * (`createPrivateDriveGate`) con una `PqVault` de verdad sobre un servidor de
 * mentira (registros cifrados con el núcleo de pruebas) y la fuente del servidor
 * con sus originales. Lo que dejan las importaciones que se cortan o se cruzan ya
 * no deja la bóveda sin abrir. Datos artificiales.
 */
import { expect, test } from 'bun:test';
import type { LegacyMigrationState, LegacySourceRow } from '@sytue/config/matrix';
import { encryptedStorageMemory } from '../encrypted-storage-fixture.js';
import { migrationSourceFixture } from '../matrix/private-drive-migration-fixture.js';
import { driveItemKind, json } from '../matrix/private-drive-codec.js';
import { digest } from '../matrix/storage-file-codec.js';
import { encodeStorageJSON, storageJSONBytes } from '../matrix/storage.js';
import { createPrivateDriveGate, type PrivateDriveSource } from './drive.js';
import { PqVault } from './pq-vault.js';
import { createHarness, createMockCore } from './pq-vault-fixture.js';

const scope = { tenant: 'acme', subject: 'user-1', product: 'drive' as const };
const root = 'f0000000-0000-4000-8000-000000000001';
const files = [
  { id: '0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5f60', size: 1117 },
  { id: '0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5f61', size: 351641 },
  { id: '0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5f62', size: 2048 },
];
const raw = (value: unknown) => encodeStorageJSON(new TextEncoder().encode(JSON.stringify(value)));

/** El servidor de Drive (bóveda y fuente congelable) y cómo abrir un cliente nuevo. */
async function bench() {
  const rows: LegacySourceRow[] = [
    { kind: 'folder', id: root, payload: raw({ id: root, name: '', is_root: true }) },
  ];
  const blobs: Record<string, string> = {};
  for (const [i, file] of files.entries()) {
    const data = new Uint8Array(file.size).fill(i + 1);
    blobs['file/' + file.id] = Buffer.from(data).toString('base64');
    rows.push({
      kind: 'file',
      id: file.id,
      payload: raw({
        id: file.id,
        folder_id: root,
        name: `f${i}.bin`,
        size_bytes: file.size,
        checksum: await digest(data),
        version: 1,
      }),
    });
  }
  const state: LegacyMigrationState = { scope, mode: 'legacy', rows: 0, blobs: 0 };
  const server = createHarness(),
    core = createMockCore(),
    origin = await migrationSourceFixture(scope, { state, rows, blobs }),
    cipher = new Map<string, Uint8Array>();
  let failRecordAt = -1,
    posts = 0;
  // Sin objeto delante, como lo llama el navegador (ver el arnés).
  const servir = server.fetchImpl;
  const fetchImpl = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'https://drive.sytue.eu');
    if (url.pathname === '/drive/api/pq-vault/records' && init?.method === 'POST') {
      // Un cliente que se cierra a mitad: lo que no llegó no se guarda.
      if (++posts === failRecordAt) return new Response(null, { status: 503 });
    }
    return servir(input, init);
  }) as typeof fetch;
  const source = (): PrivateDriveSource =>
    ({
      ...origin.source,
      readCipherBlob: (_scope: unknown, id: string) =>
        Promise.resolve(cipher.get(id)!.slice() as Uint8Array<ArrayBuffer>),
      putCipherBlob: async (_scope: unknown, id: string, bytes: Uint8Array) => {
        const copy = bytes.slice();
        cipher.set(id, copy);
        return { id, product: 'drive', size_bytes: copy.length, sha256: await digest(copy) };
      },
    }) as unknown as PrivateDriveSource;
  /** Un cliente nuevo (otra pestaña u otro navegador): su propio diario, vacío. */
  const client = async () => {
    const vault = await PqVault.open({
      actor: 'user-1',
      tenant: 'acme',
      product: 'drive',
      basePath: '/drive',
      fetchImpl,
      bridge: server.bridge,
      // El núcleo de pruebas guarda sus identidades: lo comparten los clientes.
      createCore: () =>
        ({
          invoke: core.invoke.bind(core),
          close: () => {},
          closeAndDrain: () => Promise.resolve(),
        }) as never,
      offlinePort: encryptedStorageMemory().port,
    });
    const gate = createPrivateDriveGate({ actor: 'user-1', tenant: 'acme', source });
    const open = async () => {
      await gate.host.prepare(vault, new AbortController().signal);
      return gate.current(vault).repository;
    };
    const close = async () => {
      await gate.host.close(vault);
      await vault.closeAndDrain();
    };
    return { vault, open, close };
  };
  /** Las filas de Drive con más de una cabeza en la bóveda. */
  const duplicated = async () => {
    const c = await client();
    try {
      const counts = new Map<string, number>();
      for (const v of await c.vault.storage().catalog()) {
        if (json(storageJSONBytes(v.commit.metadata, 8 << 10)).kind !== driveItemKind) continue;
        counts.set(v.object_id, (counts.get(v.object_id) ?? 0) + 1);
      }
      return [...counts.values()].filter((n) => n > 1).length;
    } finally {
      await c.close();
    }
  };
  return {
    server,
    origin,
    client,
    duplicated,
    failAt: (n: number) => {
      posts = 0;
      failRecordAt = n;
    },
  };
}

test('una importación cortada a medias la termina otro cliente sin su diario, sin congelar otra época', async () => {
  const b = await bench();
  // El primero congela la fuente con su época, publica el primer fichero y el
  // cuerpo del segundo, y se corta (como el nativo del registro de producción).
  b.failAt(7);
  const first = await b.client();
  await expect(first.open()).rejects.toThrow();
  await first.close();
  b.failAt(-1);
  expect(b.origin.state.mode).toBe('frozen');
  const epoch = b.origin.state.epoch;
  const other = await b.client();
  const drive = await other.open();
  expect((await drive.root()).id).toBe(root);
  for (const file of files) expect((await drive.file(file.id)).id).toBe(file.id);
  expect(b.origin.state.epoch).toBe(epoch);
  expect(b.origin.calls.begin).toBe(1);
  await other.close();
});

test('dos clientes que abren Drive a la vez (sin congelar y a medias) abren los dos, y después también', async () => {
  const b = await bench();
  const [a, c] = [await b.client(), await b.client()];
  // Los dos ven la fuente sin congelar: uno pierde la carrera al congelarla y
  // sigue con la época del otro; los dos copian las mismas filas a la vez.
  const opened = await Promise.allSettled([a.open(), c.open()]);
  expect(opened.map((r) => r.status)).toEqual(['fulfilled', 'fulfilled']);
  await a.close();
  await c.close();
  expect(await b.duplicated()).toBe(0);
  const later = await b.client();
  expect((await (await later.open()).root()).id).toBe(root);
  await later.close();
});

test('dos clientes que terminan a la vez una importación a medias no dejan la bóveda en conflicto', async () => {
  const b = await bench();
  b.failAt(3);
  const first = await b.client();
  await expect(first.open()).rejects.toThrow();
  await first.close();
  b.failAt(-1);
  const [a, c] = [await b.client(), await b.client()];
  const opened = await Promise.allSettled([a.open(), c.open()]);
  expect(opened.map((r) => r.status)).toEqual(['fulfilled', 'fulfilled']);
  await a.close();
  await c.close();
  const later = await b.client();
  const drive = await later.open();
  expect((await drive.root()).id).toBe(root);
  for (const file of files) expect((await drive.file(file.id)).id).toBe(file.id);
  await later.close();
  expect(await b.duplicated()).toBe(0);
});

test('los turnos exclusivos de la bóveda que se solapan esperan su turno, sin bloquear los siguientes', async () => {
  const b = await bench();
  const c = await b.client();
  const storage = c.vault.storage();
  let release!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  const order: string[] = [];
  const first = storage.exclusive(async () => {
    await held;
    order.push('first');
  });
  const second = storage.exclusive(() => {
    order.push('second');
    return Promise.resolve();
  });
  release();
  await Promise.all([first, second]);
  const third = await Promise.race([
    storage.exclusive(() => Promise.resolve('third')),
    new Promise((resolve) => setTimeout(() => resolve('blocked'), 1000)),
  ]);
  expect(order).toEqual(['first', 'second']);
  expect(third).toBe('third');
  await c.close();
});
