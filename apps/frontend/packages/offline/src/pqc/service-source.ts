/* SPDX-License-Identifier: Apache-2.0 */
import { encodeHostJSON } from './host-codec.js';
import type { HostScope } from './host-port.js';
import type { PqcRuntime } from './runtime.js';
const fail = (): never => {
  throw new Error('mail_source_rejected');
};
let bufferedUpload = false;
const hardMaximum = 50 << 20;

/** Feature detection follows the Fetch request contract, never a user agent. */
export function supportsRequestStreams(): boolean {
  let duplex = false;
  try {
    const init: RequestInit & { readonly duplex?: 'half' } = {
      method: 'POST',
      body: new ReadableStream({
        start(controller) {
          controller.close();
        },
      }),
      get duplex() {
        duplex = true;
        return 'half' as const;
      },
    };
    const request = new Request('https://request-stream.test.invalid', init);
    return duplex && !request.headers.has('Content-Type');
  } catch {
    return false;
  }
}
/** One immutable transient MIME capability. Go owns the original reader and its
 * exact EOF/hash. Buffers exist only when request streams are unavailable. */
export class PqcServiceSource {
  private readonly cancel = new AbortController();
  private closed = false;
  private closing: Promise<void> | null = null;
  private readonly active = new Set<Promise<unknown>>();
  constructor(
    private readonly runtime: Pick<PqcRuntime, 'invoke'>,
    private readonly port: string,
    private readonly scope: Readonly<HostScope>,
    private readonly source: string,
    private readonly size: number,
    private readonly assertOpen: () => void,
    private readonly signal: AbortSignal,
  ) {}
  private check(): void {
    this.assertOpen();
    this.signal.throwIfAborted();
    this.cancel.signal.throwIfAborted();
    if (this.closed) return fail();
  }
  private async tracked<T>(work: Promise<T>): Promise<T> {
    this.active.add(work);
    try {
      return await work;
    } finally {
      this.active.delete(work);
    }
  }
  private async read(offset: number, length: number): Promise<Uint8Array<ArrayBuffer>> {
    return this.tracked(
      (async () => {
        this.check();
        const input = encodeHostJSON({ source: this.source, offset, length });
        let data: Uint8Array<ArrayBuffer> | undefined;
        try {
          data = await this.runtime.invoke<Uint8Array<ArrayBuffer>>(
            { operation: 'core.source.read', port: this.port, scope: this.scope, input },
            AbortSignal.any([this.signal, this.cancel.signal]),
          );
          this.check();
          if (data.byteLength !== length) return fail();
          const output = data;
          data = undefined;
          return output;
        } finally {
          input.fill(0);
          data?.fill(0);
        }
      })(),
    );
  }
  private async finalize(): Promise<string> {
    return this.tracked(
      (async () => {
        this.check();
        const input = encodeHostJSON({ source: this.source });
        try {
          const value = await this.runtime.invoke<{ sha256: string }>(
            { operation: 'core.source.finalize', port: this.port, scope: this.scope, input },
            AbortSignal.any([this.signal, this.cancel.signal]),
          );
          this.check();
          if (!value || typeof value.sha256 !== 'string' || !/^[0-9a-f]{64}$/u.test(value.sha256))
            return fail();
          return value.sha256;
        } finally {
          input.fill(0);
        }
      })(),
    );
  }
  stream(): ReadableStream<Uint8Array<ArrayBuffer>> {
    let offset = 0;
    return new ReadableStream({
      pull: (output) =>
        this.tracked(
          (async () => {
            this.check();
            if (offset === this.size) {
              await this.finalize();
              this.check();
              output.close();
              return;
            }
            const data = await this.read(offset, Math.min(256 << 10, this.size - offset));
            try {
              this.check();
              offset += data.byteLength;
              output.enqueue(data);
            } catch (error) {
              data.fill(0);
              throw error;
            }
          })(),
        ),
      cancel: () => this.close(),
    });
  }
  buffer(
    maximum: number,
  ): Promise<{ readonly bytes: Uint8Array<ArrayBuffer>; release: () => void }> {
    return this.tracked(this.bufferOwned(maximum));
  }
  private async bufferOwned(
    maximum: number,
  ): Promise<{ readonly bytes: Uint8Array<ArrayBuffer>; release: () => void }> {
    if (
      !Number.isSafeInteger(maximum) ||
      maximum <= 0 ||
      this.size > maximum ||
      this.size > hardMaximum
    )
      throw new Error('mail_upload_streaming_required');
    if (bufferedUpload) throw new Error('mail_upload_busy');
    this.check();
    bufferedUpload = true;
    let bytes = new Uint8Array(0);
    try {
      bytes = new Uint8Array(this.size);
      for (let offset = 0; offset < this.size;) {
        const data = await this.read(offset, Math.min(256 << 10, this.size - offset));
        try {
          this.check();
          bytes.set(data, offset);
          offset += data.byteLength;
        } finally {
          data.fill(0);
        }
      }
      const expected = await this.finalize();
      const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', bytes));
      try {
        this.check();
        const checksum = Array.from(digest, (byte) => byte.toString(16).padStart(2, '0')).join('');
        if (checksum !== expected) return fail();
      } finally {
        digest.fill(0);
      }
      let released = false;
      return {
        bytes,
        release() {
          if (!released) {
            released = true;
            bytes.fill(0);
            bufferedUpload = false;
          }
        },
      };
    } catch (error) {
      bytes.fill(0);
      bufferedUpload = false;
      throw error;
    }
  }
  close(): Promise<void> {
    if (!this.closing) {
      this.closed = true;
      // Publish before abort and take the snapshot in a microtask: a reentrant
      // close can happen while tracked work is still registering itself. A
      // provider that waits on this very close can keep the drain pending.
      this.closing = Promise.resolve().then(async () => {
        let abortFailure: unknown;
        let abortFailed = false;
        try {
          this.cancel.abort();
        } catch (error) {
          abortFailure = error;
          abortFailed = true;
        }
        await Promise.allSettled(this.active);
        if (abortFailed) throw abortFailure;
      });
    }
    return this.closing;
  }
}
