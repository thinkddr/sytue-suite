/* SPDX-License-Identifier: Apache-2.0 */
import { decodeHostObject, encodeHostJSON as json } from './host-codec.js';
import { newHostPortID as nonce } from './host-port.js';
import type { StorageRepository } from '../matrix/storage-repository.js';
import {
  decodeStorageRecord,
  decodeStorageCommit,
  storageReference,
  storageIdentifier,
  storagePublicationTarget,
  type StorageRole,
} from '../matrix/storage.js';
import { storageFileRequest } from '../matrix/storage-file-upload.js';
import type { StorageFileRequest } from '../matrix/storage-file-codec.js';
import type { MatrixReference } from '../matrix/types.js';
import type { HostCallContext, HostHandler, HostMethod } from './host-port.js';
import type { PqcRuntime } from './runtime.js';

function fail(): never {
  throw new Error('storage_port_rejected');
}

function id(value: unknown): string {
  if (!storageIdentifier(value)) return fail();
  return value;
}
function text(value: unknown, maximum = 16384): string {
  if (typeof value !== 'string' || value.length > maximum || /[\0\r\n]/u.test(value)) return fail();
  return value;
}

interface Download {
  reference: MatrixReference;
  object: string;
  revision: string;
  request: StorageFileRequest;
}
interface StorageRuntime {
  invoke: PqcRuntime['invoke'];
}
/** Mechanical port adapter over the real maintained StorageRepository. It never
 * handles keys, credentials, arbitrary network locations or a second cipher.
 * A runtime owns its lifetime; the consuming app closes only this port. */
export class PqcStoragePort {
  private closed = false;
  private readonly downloads = new Map<string, Download>();
  readonly handlers: Partial<Record<Exclude<HostMethod, 'current'>, HostHandler>>;
  constructor(
    storage: StorageRepository,
    runtime: StorageRuntime,
    private readonly portID: () => string,
  ) {
    const checked = (context: HostCallContext): void => {
      context.assertOpen();
      if (
        this.closed ||
        context.scope.tenant !== storage.scope.tenant ||
        context.scope.subject !== storage.scope.subject ||
        context.scope.product !== storage.scope.product
      )
        return fail();
      storage.assertOpen(context.signal);
    };
    const handle =
      (
        run: (
          raw: Uint8Array<ArrayBuffer>,
          context: HostCallContext,
        ) => Uint8Array<ArrayBuffer> | Promise<Uint8Array<ArrayBuffer>>,
      ): HostHandler =>
      async (raw, context) => {
        checked(context);
        const output = await run(raw, context);
        try {
          checked(context);
          return output;
        } catch (error) {
          output.fill(0);
          throw error;
        }
      };
    const file = (raw: Uint8Array): Download => {
      const value = decodeHostObject(raw, fail);
      return {
        reference: storageReference(value.reference),
        object: id(value.object),
        revision: id(value.revision),
        request: {} as StorageFileRequest,
      };
    };
    const authorized = (
      value: Record<string, unknown>,
      context: HostCallContext,
      target: ReturnType<StorageRepository['publicationTarget']>,
    ): StorageRepository => {
      if (value.authorization === undefined) return storage;
      const authorization = text(value.authorization, 64),
        port = this.portID();
      if (!/^[0-9a-f]{64}$/u.test(authorization) || !/^[0-9a-f]{64}$/u.test(port)) return fail();
      return storage.withPublicationPolicy({
        target,
        signal: context.signal,
        assertOpen: () => checked(context),
        authorize: async (transaction, destination, signal) => {
          checked(context);
          signal.throwIfAborted();
          const input = json({ authorization, target: destination, transaction });
          try {
            await runtime.invoke<void>(
              { operation: 'core.publication.authorize', port, scope: storage.scope, input },
              signal,
            );
            checked(context);
            signal.throwIfAborted();
          } finally {
            input.fill(0);
          }
        },
      });
    };
    this.handlers = {
      'storage.prepare': handle(async (_raw, ctx) => {
        await storage.prepare(ctx.signal);
        return json(storage.binding());
      }),
      'storage.bindings': handle(() => json(storage.binding())),
      'storage.recoverable': handle(async (raw, ctx) => {
        const value = decodeHostObject(raw, fail);
        await storage.recoverable(storagePublicationTarget(value.target), ctx.signal);
        return new Uint8Array();
      }),
      'storage.target': handle((raw) => {
        const value = decodeHostObject(raw, fail);
        storage.publicationTarget(storagePublicationTarget(value.target));
        return new Uint8Array();
      }),
      'storage.history': handle(async (raw, ctx) => {
        const value = decodeHostObject(raw, fail),
          limit = value.limit;
        if (!Number.isSafeInteger(limit) || Number(limit) < 1 || Number(limit) > 100) return fail();
        const page = await storage.rawCatalogHistory(
          text(value.room, 255),
          text(value.cursor),
          Number(limit),
          ctx.signal,
        );
        return json({ entries: page.entries, next: page.next ?? '' });
      }),
      'storage.record.read': handle(async (raw, ctx) => {
        const value = decodeHostObject(raw, fail);
        return json(
          await storage.readRecord(
            storageReference(value.reference),
            id(value.object),
            text(value.role, 32) as StorageRole,
            ctx.signal,
          ),
        );
      }),
      'storage.record.put': handle(async (raw, ctx) => {
        const value = decodeHostObject(raw, fail),
          target = storage.publicationTarget(storagePublicationTarget(value.target)),
          record = decodeStorageRecord(value.record, storage.scope);
        return json(
          await authorized(value, ctx, target).putRecord(
            id(value.transaction),
            record,
            ctx.signal,
            target,
          ),
        );
      }),
      'storage.object.read': handle(async (raw, ctx) => {
        const value = decodeHostObject(raw, fail);
        return json(
          await storage.object(storageReference(value.reference), id(value.object), ctx.signal),
        );
      }),
      'storage.object.put': handle(async (raw, ctx) => {
        const value = decodeHostObject(raw, fail),
          target = storage.publicationTarget(storagePublicationTarget(value.target));
        return json(
          await authorized(value, ctx, target).putObject(
            id(value.transaction),
            id(value.object),
            id(value.revision),
            decodeStorageCommit(value.commit),
            ctx.signal,
            target,
          ),
        );
      }),
      'storage.file.info': handle(async (raw, ctx) => {
        const value = file(raw);
        return json(
          await storage.files().info(value.reference, value.object, value.revision, ctx.signal),
        );
      }),
      'storage.file.commitment': handle(async (raw, ctx) => {
        const value = file(raw);
        return json(
          await storage
            .files()
            .commitment(value.reference, value.object, value.revision, ctx.signal),
        );
      }),
      'storage.file.download': handle(async (raw, ctx) => {
        const value = file(raw);
        if (this.downloads.size >= 128) return fail();
        const checked = await storage
          .files()
          .commitment(value.reference, value.object, value.revision, ctx.signal);
        value.request = checked.request;
        const stream = nonce();
        if (this.downloads.has(stream)) return fail();
        this.downloads.set(stream, value);
        return json({ stream, request: checked.request, sha256: checked.sha256 });
      }),
      'storage.file.read': handle(async (raw, ctx) => {
        const value = decodeHostObject(raw, fail),
          stream = text(value.stream, 64),
          offset = value.offset,
          length = value.length;
        const file = this.downloads.get(stream);
        if (
          !file ||
          !Number.isSafeInteger(offset) ||
          !Number.isSafeInteger(length) ||
          Number(offset) < 0 ||
          Number(length) < 0 ||
          Number(length) > 256 << 10 ||
          Number(offset) > file.request.size_bytes ||
          Number(length) > file.request.size_bytes - Number(offset)
        )
          return fail();
        return storage
          .files()
          .readRange(
            file.reference,
            file.object,
            file.revision,
            Number(offset),
            Number(length),
            ctx.signal,
          );
      }),
      'storage.file.close': handle((raw) => {
        const value = decodeHostObject(raw, fail);
        this.downloads.delete(text(value.stream, 64));
        return new Uint8Array();
      }),
      'storage.file.upload': handle(async (raw, ctx) => {
        const value = decodeHostObject(raw, fail),
          source = text(value.source, 64),
          request = storageFileRequest(value.request, storage.scope),
          target = storage.publicationTarget(storagePublicationTarget(value.target)),
          port = this.portID();
        if (!/^[0-9a-f]{64}$/u.test(source) || !/^[0-9a-f]{64}$/u.test(port)) return fail();
        const reference = await authorized(value, ctx, target)
          .uploads(target)
          .uploadSource(
            id(value.job),
            request,
            {
              size: request.size_bytes,
              read: async (offset, length, signal) => {
                checked(ctx);
                signal.throwIfAborted();
                const input = json({ source, offset, length });
                try {
                  const data = await runtime.invoke<Uint8Array<ArrayBuffer>>(
                    { operation: 'core.source.read', port, scope: storage.scope, input },
                    signal,
                  );
                  try {
                    checked(ctx);
                    signal.throwIfAborted();
                    if (data.byteLength !== length) return fail();
                    return data;
                  } catch (error) {
                    data.fill(0);
                    throw error;
                  }
                } finally {
                  input.fill(0);
                }
              },
            },
            ctx.signal,
          );
        return json(reference);
      }),
      'storage.file.ack': handle(async (raw, ctx) => {
        const value = decodeHostObject(raw, fail),
          job = id(value.job),
          reference = storageReference(value.reference),
          cache = storage.writeContext().journal;
        const saved = await cache.read('j_' + job);
        try {
          checked(ctx);
          if (saved === null) return new Uint8Array();
          const row = fields(saved),
            request = storageFileRequest(row.request, storage.scope),
            target = storage.publicationTarget(storagePublicationTarget(row.target));
          await storage.uploads(target).acknowledge(job, request, reference, ctx.signal);
          return new Uint8Array();
        } finally {
          saved?.fill(0);
        }
      }),
    };
  }
  close(): Promise<void> {
    this.closed = true;
    this.downloads.clear();
    return Promise.resolve();
  }
}

const fields = (raw: Uint8Array): Record<string, unknown> => decodeHostObject(raw, fail);
