/* SPDX-License-Identifier: Apache-2.0 */
/** Artificial retained source; no credentials, real accounts or plaintext fallback. */
import {
  compareLegacySourceCursor,
  type LegacySourceRow,
  type LegacyMigrationState,
  type MatrixStorageScope,
  type LegacySourceCursor,
} from '@sytue/config/matrix';
import { encodeStorageJSON } from './storage.js';
import { digest } from './storage-file-codec.js';
import type { DriveMigrationSource } from './private-drive-migration.js';
export async function migrationSourceFixture(
  scope: MatrixStorageScope,
  provided?: {
    state: LegacyMigrationState;
    rows: LegacySourceRow[];
    blobs: Record<string, string>;
  },
) {
  const encoder = new TextEncoder(),
    data = encoder.encode('ARTIFICIAL original source ' + 'ñ 漢字 '.repeat(8000)),
    old = encoder.encode('ARTIFICIAL historical source');
  const raw = (value: unknown) => encodeStorageJSON(encoder.encode(JSON.stringify(value)));
  const rows: LegacySourceRow[] = provided
    ? structuredClone(provided.rows)
    : [
        {
          kind: 'folder',
          id: 'root',
          payload: raw({ id: 'root', name: 'Private root', is_root: true }),
        },
        {
          kind: 'file',
          id: 'file',
          payload: raw({
            id: 'file',
            folder_id: 'root',
            name: 'Original',
            size_bytes: data.length,
            version: 1,
            checksum: await digest(data),
            extension: { note: 'unknown preserved' },
          }),
        },
        {
          kind: 'version',
          id: 'old',
          payload: raw({ id: 'old', file_id: 'file', size_bytes: old.length, version: 1 }),
        },
        {
          kind: 'share',
          id: 'share',
          payload: raw({ id: 'share', file_id: 'file', role: 'reader' }),
        },
        { kind: 'favorite', id: 'star', payload: raw({ id: 'star', file_id: 'file' }) },
        {
          kind: 'request',
          id: 'request',
          payload: raw({ id: 'request', folder_id: 'root', allow_write: true }),
        },
        {
          kind: 'provenance',
          id: 'proof',
          payload: raw({ id: 'proof', file_id: 'file', source: 'artificial' }),
        },
      ];
  rows.sort(compareLegacySourceCursor);
  const blobs = provided
    ? new Map(
        Object.entries(provided.blobs).map(([key, data]) => [
          key,
          Uint8Array.from(atob(data), (c) => c.charCodeAt(0)),
        ]),
      )
    : new Map([
        ['file/file', data],
        ['version/old', old],
      ]);
  let state: LegacyMigrationState = provided
    ? structuredClone(provided.state)
    : { scope: { ...scope }, mode: 'legacy', rows: 0, blobs: 0 };
  const calls = { state: 0, begin: 0, pages: 0, opens: 0, cancelled: 0 };
  const hooks: {
    begin?: () => Promise<void>;
    page?: (page: {
      scope: MatrixStorageScope;
      epoch: string;
      rows: LegacySourceRow[];
      next?: LegacySourceCursor;
    }) => void;
    body?: (key: string, data: Uint8Array<ArrayBuffer>) => Uint8Array<ArrayBuffer>;
  } = {};
  const source: DriveMigrationSource = {
    privateStorageState: () => {
      calls.state++;
      return Promise.resolve(structuredClone(state));
    },
    beginPrivateStorageMigration: async (expected, epoch) => {
      calls.begin++;
      if (
        JSON.stringify(scope) !== JSON.stringify(expected) ||
        (state.mode !== 'legacy' && state.epoch !== epoch)
      )
        throw new Error('invalid artificial begin');
      state = {
        scope: { ...scope },
        mode: 'frozen',
        epoch,
        started_at: '2026-10-29T10:00:00Z',
        rows: rows.length,
        blobs: blobs.size,
      };
      await hooks.begin?.();
      return structuredClone(state);
    },
    privateStorageSourcePage: (expected, epoch, after) => {
      calls.pages++;
      if (
        state.mode !== 'frozen' ||
        state.epoch !== epoch ||
        JSON.stringify(scope) !== JSON.stringify(expected)
      )
        return Promise.reject(new Error('invalid artificial page'));
      const remaining = rows.filter((row) => !after || compareLegacySourceCursor(row, after) > 0),
        selected = remaining.slice(0, 2).map((row) => structuredClone(row));
      const last = selected.at(-1),
        page = {
          scope: { ...scope },
          epoch,
          rows: selected,
          ...(remaining.length > 2 && last ? { next: { kind: last.kind, id: last.id } } : {}),
        };
      hooks.page?.(page);
      return Promise.resolve(page);
    },
    openPrivateStorageSource: (expected, epoch, kind, id, size) => {
      calls.opens++;
      const key = kind + '/' + id,
        original = blobs.get(key);
      if (
        state.epoch !== epoch ||
        JSON.stringify(scope) !== JSON.stringify(expected) ||
        !original ||
        original.length !== size
      )
        return Promise.reject(new Error('invalid artificial body'));
      const data = hooks.body?.(key, original.slice()) ?? original;
      let offset = 0;
      const body = new ReadableStream<Uint8Array<ArrayBuffer>>(
        {
          pull(controller) {
            if (offset === data.length) {
              controller.close();
              return;
            }
            const bytes = data.slice(offset, Math.min(data.length, offset + 65537));
            offset += bytes.length;
            controller.enqueue(bytes);
          },
          cancel() {
            calls.cancelled++;
          },
        },
        { highWaterMark: 0 },
      );
      return Promise.resolve(
        new Response(body, {
          headers: {
            'content-type': 'application/octet-stream',
            'content-length': String(data.length),
          },
        }),
      );
    },
  };
  return {
    source,
    calls,
    hooks,
    rows,
    blobs,
    get state() {
      return structuredClone(state);
    },
  };
}
