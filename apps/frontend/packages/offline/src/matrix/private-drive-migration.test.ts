/* SPDX-License-Identifier: Apache-2.0 */
import {
  compareLegacySourceCursor,
  type LegacyMigrationState,
  type LegacySourceRow,
} from '@sytue/config/matrix';
import { encodeStorageJSON } from './storage.js';
import { expect, test } from 'bun:test';
import { storageWriteFixture } from './storage-write-fixture.js';
import { migrationSourceFixture } from './private-drive-migration-fixture.js';
import { PrivateDriveRepository } from './private-drive.js';
import { driveObject, json, driveMigrationKind, sourceKind } from './private-drive-codec.js';
import { storageJSONBytes } from './storage.js';
const scope = { tenant: 'artificial-storage', subject: 'artificial', product: 'drive' as const };
test('initial web migration imports all source kinds, historical binaries and an empty-checksum source before marking ready', async () => {
  const f = storageWriteFixture(scope),
    source = await migrationSourceFixture(scope),
    drive = new PrivateDriveRepository(f.repository());
  const state = await drive.migrate(source.source);
  expect(state.mode).toBe('frozen');
  expect((await drive.root()).id).toBe('root');
  expect((await drive.file('file')).name).toBe('Original');
  for (const row of source.rows)
    expect(await drive.readRaw(sourceKind(row.kind), row.id)).toEqual(
      storageJSONBytes(row.payload),
    );
  expect(source.calls.opens).toBe(2);
  const before = { ...source.calls };
  await drive.migrate(source.source);
  expect(source.calls.pages).toBe(before.pages);
  expect(source.calls.opens).toBe(before.opens);
  expect(
    (await f.cache.listIDs()).filter((k) => k.startsWith('stage_') || k.includes('row_')),
  ).toHaveLength(0);
  const catalog = await f.repository().catalog(),
    receipt = catalog.find(
      (v) => json(storageJSONBytes(v.commit.metadata)).kind === driveMigrationKind,
    )!;
  expect(json(storageJSONBytes(receipt.commit.metadata)).rows).toBe(source.rows.length);
  await drive.close();
  await f.close();
});
test('crypto preparation must finish before any source request or write fence', async () => {
  const f = storageWriteFixture(scope),
    source = await migrationSourceFixture(scope),
    drive = new PrivateDriveRepository(f.repository());
  f.hooks.prepare = () => Promise.reject(new Error('no owner key'));
  await expect(drive.migrate(source.source)).rejects.toThrow();
  expect(source.calls).toEqual({ state: 0, begin: 0, pages: 0, opens: 0, cancelled: 0 });
  expect(f.sends).toHaveLength(0);
  await drive.close();
  await f.close();
});
test.each(['begin', 'row', 'completion'])(
  'a lost %s ACK reopens the same frozen source and confirms existing copies before completion',
  async (stage) => {
    const f = storageWriteFixture(scope),
      source = await migrationSourceFixture(scope);
    let lost = false;
    if (stage === 'begin')
      source.hooks.begin = () => {
        if (!lost) {
          lost = true;
          return Promise.reject(new Error('begin accepted without ACK'));
        }
        return Promise.resolve();
      };
    f.hooks.recordAck = (record) => {
      if (record.role === 'index' && !lost) {
        const meta = json(storageJSONBytes(json(storageJSONBytes(record.payload)).metadata));
        if (
          (stage === 'row' && meta.kind === 'org.sytue.drive.item.v1') ||
          (stage === 'completion' && meta.kind === driveMigrationKind)
        ) {
          lost = true;
          return Promise.reject(new Error('index accepted without ACK'));
        }
      }
      return Promise.resolve();
    };
    let drive = new PrivateDriveRepository(f.repository());
    await expect(drive.migrate(source.source)).rejects.toThrow();
    expect(lost).toBe(true);
    await drive.close();
    const epoch = source.state.epoch;
    drive = new PrivateDriveRepository(
      f.repository({ room_id: '!rotated:matrix.test', device_id: 'DEVICE' }),
    );
    const state = await drive.migrate(source.source);
    expect(state.epoch).toBe(epoch);
    expect((await drive.file('file')).name).toBe('Original');
    expect(source.calls.begin).toBe(1);
    expect((await f.cache.listIDs()).filter((id) => id.includes('complete_'))).toHaveLength(0);
    const object = await driveObject('file', 'file');
    expect((await f.repository().catalog()).filter((v) => v.object_id === object)).toHaveLength(1);
    await drive.close();
    await f.close();
  },
);
test.each(['count', 'order', 'body'])(
  'an invalid %s source cannot publish a completion receipt',
  async (kind) => {
    const f = storageWriteFixture(scope),
      source = await migrationSourceFixture(scope),
      drive = new PrivateDriveRepository(f.repository());
    if (kind === 'count')
      source.hooks.page = (page) => {
        if (!page.next) page.rows.pop();
      };
    if (kind === 'order')
      source.hooks.page = (page) => {
        page.rows.reverse();
      };
    if (kind === 'body')
      source.hooks.body = (key, data) =>
        key.startsWith('file/') ? new Uint8Array(data.length).fill(7) : data;
    await expect(drive.migrate(source.source)).rejects.toThrow();
    expect(
      (await f.repository().catalog()).some(
        (v) => json(storageJSONBytes(v.commit.metadata)).kind === driveMigrationKind,
      ),
    ).toBe(false);
    expect(() => drive.file('file')).toThrow('not_ready');
    await drive.close();
    await f.close();
  },
);

test('source rows share one catalog snapshot instead of scanning remote history for each row', async () => {
  const f = storageWriteFixture(scope),
    source = await migrationSourceFixture(scope);
  for (let n = 0; n < 70; n++)
    source.rows.push({
      kind: 'share',
      id: 's_' + String(n).padStart(3, '0'),
      payload: encodeStorageJSON(
        new TextEncoder().encode(JSON.stringify({ id: 's_' + n, file_id: 'file' })),
      ),
    });
  source.rows.sort(compareLegacySourceCursor);
  let pages = 0;
  f.hooks.history = () => {
    pages++;
  };
  const drive = new PrivateDriveRepository(f.repository());
  await drive.migrate(source.source);
  expect(pages).toBeLessThan(source.rows.length);
  expect((await drive.file('file')).name).toBe('Original');
  await drive.close();
  await f.close();
});

const sourceVector = process.env.SYTUE_DRIVE_PRIVATE_SOURCE_VECTOR;
test.skipIf(!sourceVector)(
  'real PostgreSQL retained source is imported by web with exact JSON and historical binary verification',
  async () => {
    if (!sourceVector!.startsWith('/tmp/sytue-private-drive-vector-'))
      throw new Error('disposable source fixture required');
    const { readFile } = await import('node:fs/promises');
    const vector = JSON.parse(await readFile(sourceVector!, 'utf8')) as {
      state: LegacyMigrationState;
      rows: LegacySourceRow[];
      blobs: Record<string, string>;
      root: string;
      file: string;
    };
    expect(vector.state.scope).toEqual(scope);
    const f = storageWriteFixture(scope),
      source = await migrationSourceFixture(scope, vector),
      drive = new PrivateDriveRepository(f.repository());
    await drive.migrate(source.source);
    expect((await drive.root()).id).toBe(vector.root);
    for (const row of vector.rows)
      expect(await drive.readRaw(sourceKind(row.kind), row.id)).toEqual(
        storageJSONBytes(row.payload),
      );
    expect(source.calls.opens).toBe(Object.keys(vector.blobs).length);
    expect(source.calls.begin).toBe(0);
    expect((await f.cache.listIDs()).filter((id) => id.startsWith('stage_'))).toHaveLength(0);
    await drive.close();
    await f.close();
  },
  120_000,
);

test('public data stays unavailable while copied rows exist but the completion receipt is pending', async () => {
  const f = storageWriteFixture(scope),
    source = await migrationSourceFixture(scope),
    drive = new PrivateDriveRepository(f.repository());
  let arrived!: () => void,
    release!: () => void,
    once = false;
  const waiting = new Promise<void>((resolve) => {
      arrived = resolve;
    }),
    gate = new Promise<void>((resolve) => {
      release = resolve;
    });
  f.hooks.blobAck = async () => {
    if (!once) {
      once = true;
      arrived();
      await gate;
    }
  };
  const importing = drive.migrate(source.source);
  await waiting;
  expect(() => drive.root()).toThrow('not_ready');
  expect(() => drive.readRaw('favorite', 'star')).toThrow('not_ready');
  release();
  await importing;
  expect((await drive.file('file')).name).toBe('Original');
  await drive.close();
  await f.close();
});
