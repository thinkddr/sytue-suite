/* SPDX-License-Identifier: Apache-2.0 */
import { decodeHostObject } from './host-codec.js';
import { newHostPortID as nonce } from './host-port.js';
import { EncryptedOfflineValues } from '../encrypted-storage.js';
import type { HostCallContext, HostHandler, HostScope } from './host-port.js';
const encoder = new TextEncoder();
const component = '[0-9a-f]{32}';
const keyPattern = new RegExp('^' + component + '/' + component + '/' + component + '$');
const prefixPattern = new RegExp('^' + component + '(?:/' + component + ')?/$');
function fail(): never {
  throw new Error('encrypted_journal_rejected');
}

function exact(value: Record<string, unknown>, keys: string[]): void {
  if (Object.keys(value).length !== keys.length || keys.some((key) => !Object.hasOwn(value, key)))
    fail();
}
function key(raw: Uint8Array): string {
  const value = decodeHostObject(raw, fail);
  exact(value, ['key']);
  if (typeof value.key !== 'string' || !keyPattern.test(value.key)) return fail();
  return value.key;
}

interface HeldLock {
  release: () => void;
  done: Promise<void>;
}
/** Production journal adapter. It never writes a clear value or replaces IDB
 * CryptoKey records with JSON/localStorage. SDK lease ownership stays outside. */
export class PqcEncryptedJournal {
  private closed = false;
  private readonly cache: EncryptedOfflineValues;
  private readonly locks = new Map<string, HeldLock>();
  private readonly contexts = new Set<HostCallContext>();
  private readonly ownerPrefix: Promise<string>;
  readonly scope: Readonly<HostScope>;
  readonly handlers: {
    'journal.read': HostHandler;
    'journal.write': HostHandler;
    'journal.delete': HostHandler;
    'journal.keys': HostHandler;
    'journal.lock': HostHandler;
    'journal.unlock': HostHandler;
  };
  constructor(scope: Readonly<HostScope>, active: () => boolean) {
    this.scope = Object.freeze({
      tenant: scope.tenant,
      subject: scope.subject,
      product: scope.product,
    });
    this.ownerPrefix = crypto.subtle
      .digest('SHA-256', encoder.encode(scope.tenant + '\0' + scope.subject + '\0' + scope.product))
      .then((digest) =>
        Array.from(new Uint8Array(digest).slice(0, 16), (byte) =>
          byte.toString(16).padStart(2, '0'),
        ).join(''),
      );
    this.cache = new EncryptedOfflineValues(
      JSON.stringify([scope.tenant, scope.subject, scope.product]),
      'go-private-journal-v1',
      () =>
        !this.closed &&
        active() &&
        [...this.contexts].some((context) => {
          try {
            context.assertOpen();
            return true;
          } catch {
            return false;
          }
        }),
    );
    const handlers: typeof this.handlers = {
      'journal.read': async (raw, context) => {
        context.assertOpen();
        const value = await this.cache.read(await this.scopedKey(key(raw), context));
        try {
          context.assertOpen();
          const output = new Uint8Array(1 + (value?.byteLength ?? 0));
          if (value) {
            output[0] = 1;
            output.set(value, 1);
          }
          return output;
        } finally {
          value?.fill(0);
        }
      },
      'journal.write': async (raw, context) => {
        context.assertOpen();
        if (raw.byteLength < 4) return fail();
        const length = new DataView(raw.buffer, raw.byteOffset, 4).getUint32(0, false);
        if (length > 8192 || length > raw.byteLength - 4) return fail();
        const id = await this.scopedKey(key(raw.subarray(4, 4 + length)), context);
        const body = raw.subarray(4 + length);
        if (body.byteLength > 8 << 20) return fail();
        await this.cache.write(id, body);
        context.assertOpen();
        return new Uint8Array();
      },
      'journal.delete': async (raw, context) => {
        context.assertOpen();
        await this.cache.delete(await this.scopedKey(key(raw), context));
        context.assertOpen();
        return new Uint8Array();
      },
      'journal.keys': async (raw, context) => {
        context.assertOpen();
        const value = decodeHostObject(raw, fail);
        exact(value, ['prefix', 'after', 'limit']);
        if (
          typeof value.prefix !== 'string' ||
          !prefixPattern.test(value.prefix) ||
          typeof value.after !== 'string' ||
          (value.after && !keyPattern.test(value.after)) ||
          !Number.isSafeInteger(value.limit) ||
          Number(value.limit) < 1 ||
          Number(value.limit) > 128
        )
          return fail();
        // Metadata only. The wire response is bounded to one page and no
        // encrypted body is opened to count or enumerate retained work.
        await this.scopedKey(value.prefix, context);
        if (value.after && !value.after.startsWith(value.prefix)) return fail();
        const page = await this.cache.listIDPage(Number(value.limit), value.prefix, value.after);
        context.assertOpen();
        return encoder.encode(JSON.stringify({ keys: page.ids, next: page.next }));
      },
      'journal.lock': async (raw, context) =>
        this.lock(await this.scopedKey(key(raw), context), context),
      'journal.unlock': async (raw, context) => {
        context.assertOpen();
        const value = decodeHostObject(raw, fail);
        exact(value, ['lock']);
        if (typeof value.lock !== 'string' || !/^[0-9a-f]{64}$/.test(value.lock)) return fail();
        const held = this.locks.get(value.lock);
        if (!held) return fail();
        this.locks.delete(value.lock);
        held.release();
        await held.done;
        context.assertOpen();
        return new Uint8Array();
      },
    };
    this.handlers = Object.freeze(
      Object.fromEntries(
        Object.entries(handlers).map(([method, handler]) => [
          method,
          async (raw: Uint8Array<ArrayBuffer>, context: HostCallContext) => {
            context.assertOpen();
            this.contexts.add(context);
            try {
              return await handler(raw, context);
            } finally {
              this.contexts.delete(context);
            }
          },
        ]),
      ) as typeof this.handlers,
    );
  }
  private async scopedKey(id: string, context: HostCallContext): Promise<string> {
    const prefix = await this.ownerPrefix;
    context.assertOpen();
    if (!id.startsWith(prefix + '/')) return fail();
    return id;
  }
  private async lock(key: string, context: HostCallContext): Promise<Uint8Array<ArrayBuffer>> {
    context.assertOpen();
    if (!navigator.locks || this.closed) return fail();
    const id = nonce();
    let acquired!: () => void, failed!: (error: unknown) => void, release!: () => void;
    const ready = new Promise<void>((resolve, reject) => {
      acquired = resolve;
      failed = reject;
    });
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const name = JSON.stringify(['sytue-go-journal', this.scope, key]);
    const done = navigator.locks.request(
      name,
      { mode: 'exclusive', signal: context.signal },
      async () => {
        context.assertOpen();
        acquired();
        await held;
      },
    );
    void done.catch(failed);
    this.locks.set(id, { release, done });
    try {
      await ready;
      context.assertOpen();
      return encoder.encode(JSON.stringify({ lock: id }));
    } catch (error) {
      this.locks.delete(id);
      release();
      await done.catch(() => {});
      throw error;
    }
  }
  /** Retains encrypted pending work. It only closes access and releases locks. */
  async close(): Promise<void> {
    this.closed = true;
    const held = [...this.locks.values()];
    this.locks.clear();
    for (const lock of held) lock.release();
    await Promise.allSettled(held.map((lock) => lock.done));
    this.cache.close();
  }
}
