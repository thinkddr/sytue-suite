/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { StorageRepository, type StorageRecordTransport } from './storage-repository.js';
import {
  StorageRecordError,
  createStorageRecord,
  decodeStorageRecord,
  decodeStorageObject,
  decodeStorageCommit,
  encodeStorageJSON,
  storageJSONBytes,
  storageCatalogHeads,
  storageObjectKind,
  type StorageRecord,
  type StorageRecordEntry,
  type StorageObjectCommit,
} from './storage.js';
import type { MatrixReference, MatrixStorageScope } from './types.js';
const scope: MatrixStorageScope = { tenant: 'acme', subject: 'person', product: 'drive' };
const room = '!private:matrix.test';
const ref = (event: string, room_id = room): MatrixReference => ({
  room_id,
  event_id: '$' + event,
});
const bytes = (value: string): Uint8Array => new TextEncoder().encode(value);
const precise =
  '{"amount":900719925474099312345,"fraction":1.00000000000000000001,"title":"<private> \u2028"}';
function entry(
  id: string,
  parents: string[] = [],
  state: StorageObjectCommit['state'] = 'live',
  object = 'object',
): StorageRecordEntry {
  const commit: StorageObjectCommit = {
    kind: storageObjectKind,
    state,
    metadata: encodeStorageJSON(bytes(precise)),
    ...(state !== 'deleted'
      ? { content: { kind: 'record', revision_id: 'content', reference: ref('content') } }
      : {}),
    parents: parents.map((parent) => ref(parent)),
  };
  return {
    reference: ref(id),
    record: createStorageRecord(
      scope,
      object,
      id,
      'index',
      bytes(JSON.stringify(commit)),
      parents.map((parent) => '$' + parent),
    ),
  };
}
class Memory implements StorageRecordTransport {
  readonly rooms = [room];
  readonly records = new Map<string, StorageRecord>();
  readonly transactions = new Map<string, MatrixReference>();
  readonly pages = new Map<
    string,
    { entries: StorageRecordEntry[]; scanned: number; next: string | null }
  >();
  closed = false;
  posts = 0;
  assertOpen = (): void => {
    if (this.closed) throw new Error('closed');
  };
  put = async (transaction: string, record: StorageRecord): Promise<MatrixReference> => {
    this.posts++;
    let result = this.transactions.get(transaction);
    if (!result) {
      result = ref('put' + this.posts);
      this.transactions.set(transaction, result);
      this.records.set(result.event_id, structuredClone(record));
    }
    return result;
  };
  read = async (reference: MatrixReference): Promise<StorageRecord> => {
    const record = this.records.get(reference.event_id);
    if (!record) throw new Error('missing');
    return structuredClone(record);
  };
  history = async (room: string, cursor: string | null) =>
    this.pages.get(JSON.stringify([room, cursor])) ?? { entries: [], scanned: 0, next: null };
}

test('native byte contracts preserve precise JSON and reject scope, role and encoding confusion', () => {
  const record = createStorageRecord(scope, 'object', 'revision', 'content', bytes(precise), [
    '$old',
  ]);
  expect(new TextDecoder().decode(storageJSONBytes(record.payload))).toBe(precise);
  expect(decodeStorageRecord(JSON.parse(JSON.stringify(record)), scope)).toEqual(record);
  for (const input of [
    { ...record, v: 2 },
    { ...record, scope: { ...scope, product: 'mail' } },
    { ...record, scope: { ...scope, subject: 'another' } },
    { ...record, scope: { ...scope, tenant: 'another' } },
    { ...record, object_id: '../private' },
    { ...record, revision_id: 'x'.repeat(129) },
    { ...record, role: 'state' },
    { ...record, previous: ['$duplicate', '$duplicate'] },
    { ...record, previous: ['$bad id'] },
    { ...record, scope: { ...scope, subject: '\ud800' } },
    { ...record, scope: { ...scope, tenant: 'x'.repeat(64) } },
    { ...record, payload: btoa('{') },
    { ...record, payload: btoa('"\xff"') },
    { ...record, payload: record.payload.slice(0, -1) },
  ])
    expect(() => decodeStorageRecord(input, scope)).toThrow(StorageRecordError);
  expect(() => storageJSONBytes('eyJ4IjoxfR==')).toThrow(); // non-canonical padding bits
  expect(() => encodeStorageJSON(bytes('\ufeff{}'))).toThrow();
  expect(() =>
    createStorageRecord(scope, 'x', 'y', 'content', bytes(JSON.stringify('x'.repeat(40 << 10)))),
  ).toThrow();
});

test('catalog keeps concurrent edit/delete, resolves only explicit parents and survives long histories', () => {
  const all = [entry('base'), entry('edit', ['base']), entry('delete', ['base'], 'deleted')];
  expect(storageCatalogHeads(scope, all).map((version) => version.revision_id)).toEqual([
    'delete',
    'edit',
  ]);
  const merge = entry('merge', ['edit', 'delete']);
  for (const order of [
    [...all, merge],
    [merge, ...all.toReversed()],
  ])
    expect(storageCatalogHeads(scope, order).map((version) => version.revision_id)).toEqual([
      'merge',
    ]);
  const chain: StorageRecordEntry[] = [];
  for (let i = 0; i < 10000; i++) chain.push(entry('r' + i, i === 0 ? [] : ['r' + (i - 1)]));
  expect(
    storageCatalogHeads(scope, chain.toReversed()).map((version) => version.revision_id),
  ).toEqual(['r9999']);
  expect(
    new TextDecoder().decode(
      storageJSONBytes(decodeStorageObject(all[0]!, scope).commit.metadata, 8192),
    ),
  ).toBe(precise);
});

test('missing or foreign parents, cycles, duplicate events and unbound tombstones fail instead of hiding objects', () => {
  expect(() => storageCatalogHeads(scope, [entry('edit', ['missing'])])).toThrow(
    'storage_incomplete',
  );
  for (const records of [
    [entry('base', [], 'live', 'foreign'), entry('edit', ['base'])],
    [entry('a', ['b']), entry('b', ['a'])],
    [entry('base'), entry('base')],
    [entry('self', ['self'])],
    [{ ...entry('base'), record: { ...entry('base').record, previous: ['$foreign'] } }],
  ])
    expect(() => storageCatalogHeads(scope, records)).toThrow(StorageRecordError);
  const commit = decodeStorageObject(entry('base'), scope).commit;
  expect(() => decodeStorageCommit({ ...commit, state: 'deleted' })).toThrow();
  expect(() => decodeStorageCommit({ ...commit, state: 'live', content: null })).toThrow();
  expect(() =>
    decodeStorageCommit({
      ...commit,
      metadata: encodeStorageJSON(bytes(JSON.stringify('x'.repeat(8193)))),
    }),
  ).toThrow();
});

test('write confirmation rejects changed transaction contents and every substituted authenticated field', async () => {
  const transport = new Memory(),
    repository = new StorageRepository(scope, transport);
  const record = createStorageRecord(scope, 'object', 'content', 'content', bytes(precise));
  const saved = await repository.putRecord('stable', record);
  expect(await repository.putRecord('stable', record)).toEqual(saved);
  await expect(
    repository.putRecord('stable', { ...record, revision_id: 'changed' }),
  ).rejects.toThrow();
  const original = transport.read;
  for (const patch of [
    { scope: { ...scope, tenant: 'other' } },
    { object_id: 'foreign' },
    { revision_id: 'changed' },
    { role: 'trash' as const },
    { previous: ['$foreign'] },
    { payload: encodeStorageJSON(bytes('{"amount":1}')) },
  ]) {
    transport.read = async (reference) => ({ ...(await original(reference)), ...patch });
    await expect(repository.putRecord('stable', record)).rejects.toThrow();
  }
  transport.read = original;
  expect(await repository.readRecord(saved, 'object', 'content')).toEqual(record);
});

test('private object writes bind parents and content before publishing the index', async () => {
  const transport = new Memory(),
    repository = new StorageRepository(scope, transport);
  transport.records.set(
    '$content',
    createStorageRecord(scope, 'object', 'content', 'content', bytes(precise)),
  );
  const commit = decodeStorageObject(entry('base'), scope).commit;
  const base = await repository.putObject('base', 'object', 'base', commit);
  const edit = await repository.putObject('edit', 'object', 'edit', {
    ...commit,
    parents: [base.reference],
    state: 'trash',
  });
  expect(edit.commit.state).toBe('trash');
  expect(edit.commit.content).toEqual(base.commit.content);
  const posts = transport.posts;
  for (const change of [
    { ...commit, content: { ...commit.content!, revision_id: 'foreign' } },
    {
      ...commit,
      content: { ...commit.content!, reference: ref('content', '!foreign:matrix.test') },
    },
    { ...commit, content: { ...commit.content!, kind: 'file' as const } },
    { ...commit, parents: [base.reference] },
  ])
    await expect(
      repository.putObject('bad', 'object', change.parents ? 'base' : 'bad', change),
    ).rejects.toThrow();
  expect(transport.posts).toBe(posts);
  const deletion = await repository.putObject('delete', 'object', 'delete', {
    kind: storageObjectKind,
    state: 'deleted',
    metadata: encodeStorageJSON(bytes('{}')),
    parents: [edit.reference],
  });
  expect(deletion.commit.content).toBeUndefined();
  expect(transport.records.has('$content')).toBe(true); // logical deletion retains the source/revisions
});

test('catalog exhausts empty-index pages and every pinned room; incomplete or looping pages never become empty success', async () => {
  const transport = new Memory(),
    repository = new StorageRepository(scope, transport);
  transport.pages.set(JSON.stringify([room, null]), { entries: [], scanned: 1, next: 'older' });
  transport.pages.set(JSON.stringify([room, 'older']), {
    entries: [entry('base')],
    scanned: 1,
    next: null,
  });
  expect((await repository.catalog()).map((version) => version.revision_id)).toEqual(['base']);
  await expect(repository.catalog(undefined, 1)).rejects.toThrow('storage_incomplete');
  transport.pages.set(JSON.stringify([room, 'older']), { entries: [], scanned: 1, next: 'older' });
  await expect(repository.catalog()).rejects.toThrow('storage_incomplete');
  transport.pages.set(JSON.stringify([room, null]), { entries: [], scanned: 0, next: 'new-token' });
  await expect(repository.catalog()).rejects.toThrow('storage_incomplete');
  transport.pages.set(JSON.stringify([room, null]), {
    entries: [entry('edit', ['missing'])],
    scanned: 1,
    next: null,
  });
  await expect(repository.catalog()).rejects.toThrow('storage_incomplete');
  transport.rooms.push('!prior:matrix.test');
  const multiple = new StorageRepository(scope, transport);
  transport.pages.set(JSON.stringify([room, null]), {
    entries: [{ ...entry('edit', ['base']), record: entry('edit', ['base']).record }],
    scanned: 1,
    next: null,
  });
  const base = entry('base');
  base.reference.room_id = '!prior:matrix.test';
  const edit = entry('edit', ['base']);
  const decoded = decodeStorageObject(edit, scope);
  decoded.commit.parents = [base.reference];
  edit.record = createStorageRecord(
    scope,
    'object',
    'edit',
    'index',
    bytes(JSON.stringify(decoded.commit)),
    ['$base'],
  );
  transport.pages.set(JSON.stringify([room, null]), { entries: [edit], scanned: 1, next: null });
  transport.pages.set(JSON.stringify(['!prior:matrix.test', null]), {
    entries: [base],
    scanned: 1,
    next: null,
  });
  expect((await multiple.catalog()).map((version) => version.revision_id)).toEqual(['edit']);
});

test('pending operations freeze mutable input and cannot report completion after logout/cancellation', async () => {
  const transport = new Memory(),
    repository = new StorageRepository(scope, transport);
  const record = createStorageRecord(scope, 'object', 'content', 'content', bytes(precise), [
    '$original',
  ]);
  let release!: () => void, entered!: () => void;
  const waiting = new Promise<void>((resolve) => {
      release = resolve;
    }),
    ready = new Promise<void>((resolve) => {
      entered = resolve;
    });
  const put = transport.put;
  transport.put = async (transaction, record) => {
    entered();
    await waiting;
    return put(transaction, record);
  };
  const pending = repository.putRecord('frozen', record);
  await ready;
  record.previous![0] = '$changed';
  record.scope.tenant = 'changed';
  release();
  const saved = await pending;
  expect(transport.records.get(saved.event_id)?.previous).toEqual(['$original']);
  expect(transport.records.get(saved.event_id)?.scope.tenant).toBe('acme');
  transport.closed = true;
  await expect(
    repository.putRecord(
      'afterlogout',
      createStorageRecord(scope, 'object', 'content', 'content', bytes('{}')),
    ),
  ).rejects.toThrow('closed');
  const abort = new AbortController();
  abort.abort();
  await expect(repository.catalog(abort.signal)).rejects.toThrow();
});
