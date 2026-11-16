/* SPDX-License-Identifier: Apache-2.0 */
import { PqcRuntime } from '@sytue/offline/pqc/runtime';
import { withBase } from '@sytue/ui';
import type { ChatMessage, SearchMetadata, SearchMetadataPage } from '../../lib/chat.js';
import {
  captureMatrixScope,
  assertMatrixScope,
  decryptMessage,
  rememberDecryptedMessage,
} from './messages.js';
import { MatrixSecurityError } from './types.js';

type Candidate = { metadata: SearchMetadata; message: ChatMessage };
function document(row: SearchMetadata, message?: ChatMessage): Record<string, unknown> {
  const m = message ?? row.message;
  return {
    author_sub: m.author.id,
    author_name: m.author.name ?? '',
    channel_slug: row.channel_slug,
    channel_name: row.channel_name,
    created_at: m.createdAt,
    saved: row.saved,
    pinned: row.pinned,
    body: message?.body ?? '',
    has_file: Boolean(message?.attachments?.length),
  };
}
function identity(row: SearchMetadata): string {
  return row.tenant + '\0' + row.id;
}
function sameBinding(a: SearchMetadata, b: SearchMetadata): boolean {
  return (
    a.message.channelId === b.message.channelId &&
    a.message.author.id === b.message.author.id &&
    Boolean(a.message.matrix_event) &&
    Boolean(b.message.matrix_event) &&
    a.message.matrix_event!.room_id === b.message.matrix_event!.room_id &&
    a.message.matrix_event!.event_id === b.message.matrix_event!.event_id
  );
}
async function metadataRequest<T>(body: unknown, cursor: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(
    withBase(import.meta.env.BASE_URL ?? '/rooms', '/api/search-metadata') +
      (body === undefined ? '?cursor=' + encodeURIComponent(cursor) : ''),
    {
      method: body === undefined ? 'GET' : 'POST',
      credentials: 'include',
      cache: 'no-store',
      headers: {
        accept: 'application/json',
        ...(body === undefined ? {} : { 'content-type': 'application/json' }),
      },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      signal,
    },
  );
  if (!response.ok) throw new MatrixSecurityError('identity');
  return (await response.json()) as T;
}

/** Full history is scanned by stable metadata pages; the query and AST stay in
 * the authorized browser and use the exact same Go parser/matcher as Fyne. */
export async function searchEncryptedMessages(
  raw: string,
  signal?: AbortSignal,
): Promise<{ messages: ChatMessage[] }> {
  if (!raw.trim()) return { messages: [] };
  const scope = captureMatrixScope();
  const life = new AbortController();
  const abort = (): void => life.abort();
  signal?.addEventListener('abort', abort, { once: true });
  scope.signal.addEventListener('abort', abort, { once: true });
  if (signal?.aborted || scope.signal.aborted) life.abort();
  const check = (): void => {
    life.signal.throwIfAborted();
    assertMatrixScope(scope);
  };
  const core = new PqcRuntime({ signal: life.signal, assertOpen: check });
  const encoder = new TextEncoder(),
    decoder = new TextDecoder();
  let ast: Uint8Array | undefined;
  const evaluate = async (
    operation: 'metadata' | 'match',
    rows: Candidate[],
  ): Promise<boolean[]> => {
    check();
    if (!rows.length) return [];
    const docs = encoder.encode(
      JSON.stringify(
        rows.map((row) => document(row.metadata, operation === 'match' ? row.message : undefined)),
      ),
    );
    let result: Uint8Array | undefined;
    try {
      result = await core.invoke<Uint8Array>(
        { operation: 'rooms.search.' + operation, query: ast, documents: docs },
        life.signal,
      );
      check();
      return JSON.parse(decoder.decode(result)) as boolean[];
    } finally {
      docs.fill(0);
      result?.fill(0);
    }
  };
  const decrypt = async (rows: SearchMetadata[]): Promise<Candidate[]> => {
    const out: Candidate[] = [];
    for (let first = 0; first < rows.length; first += 8) {
      check();
      const batch = await Promise.all(
        rows.slice(first, first + 8).map(async (metadata) => {
          if (!metadata.message.matrix_event) throw new MatrixSecurityError('unsupported');
          const message = await decryptMessage(metadata.message, false);
          check();
          if (message.decryptionFailed) throw new MatrixSecurityError('unverified');
          return { metadata, message };
        }),
      );
      out.push(...batch);
    }
    return out;
  };
  const visible = async (candidates: Candidate[]): Promise<Candidate[]> => {
    if (!candidates.length) return [];
    const page = await metadataRequest<{ messages: SearchMetadata[] }>(
      { messages: candidates.map((c) => ({ id: c.metadata.id, tenant: c.metadata.tenant })) },
      '',
      life.signal,
    );
    check();
    const previous = new Map(candidates.map((c) => [identity(c.metadata), c]));
    const rows = page.messages.filter((row) => previous.has(identity(row)));
    const allowed = await evaluate(
      'metadata',
      rows.map((metadata) => ({ metadata, message: metadata.message })),
    );
    const current: Candidate[] = [];
    for (let i = 0; i < rows.length; i++) {
      if (!allowed[i]) continue;
      const metadata = rows[i]!,
        old = previous.get(identity(metadata))!;
      if (!sameBinding(old.metadata, metadata)) {
        current.push(...(await decrypt([metadata])));
      } else
        current.push({
          metadata,
          message: {
            ...old.message,
            author: metadata.message.author,
            ...(metadata.message.editedAt ? { editedAt: metadata.message.editedAt } : {}),
          },
        });
    }
    const matches = await evaluate('match', current);
    return current.filter((_, i) => matches[i]);
  };
  try {
    const input = encoder.encode(raw);
    try {
      ast = await core.invoke<Uint8Array>(
        { operation: 'rooms.search.parse', query: input },
        life.signal,
      );
    } finally {
      input.fill(0);
    }
    let cursor = '',
      selected: Candidate[] = [];
    searching: for (;;) {
      check();
      const page = await metadataRequest<SearchMetadataPage>(undefined, cursor, life.signal);
      check();
      const metadata = page.messages.map((row) => ({ metadata: row, message: row.message }));
      const allowed = await evaluate('metadata', metadata);
      const batch = await decrypt(page.messages.filter((_, i) => allowed[i]));
      const matches = await evaluate('match', batch);
      selected.push(...batch.filter((_, i) => matches[i]));
      if (selected.length >= 30 || !page.next_cursor) {
        selected = await visible(selected);
        if (selected.length >= 30 || !page.next_cursor) {
          selected = selected.slice(0, 30);
          if (selected.length) {
            const final = await metadataRequest<{ messages: SearchMetadata[] }>(
              { messages: selected.map((c) => ({ id: c.metadata.id, tenant: c.metadata.tenant })) },
              '',
              life.signal,
            );
            check();
            const latest = new Map(final.messages.map((row) => [identity(row), row]));
            const flags = await evaluate(
              'metadata',
              final.messages.map((metadata) => ({ metadata, message: metadata.message })),
            );
            const allowed = new Set(final.messages.filter((_, i) => flags[i]).map(identity));
            for (const item of selected) {
              const next = latest.get(identity(item.metadata));
              if (
                !next ||
                !allowed.has(identity(item.metadata)) ||
                !sameBinding(item.metadata, next)
              ) {
                cursor = '';
                selected = [];
                continue searching;
              }
            }
          }
          check();
          for (const item of selected) rememberDecryptedMessage(scope, item.message);
          return { messages: selected.map((item) => item.message) };
        }
      }
      if (page.next_cursor === cursor) throw new MatrixSecurityError('identity');
      cursor = page.next_cursor!;
    }
  } finally {
    ast?.fill(0);
    core.close();
    signal?.removeEventListener('abort', abort);
    scope.signal.removeEventListener('abort', abort);
    life.abort();
  }
}
