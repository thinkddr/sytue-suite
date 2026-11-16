/* SPDX-License-Identifier: Apache-2.0 */
import { newHostPortID } from './host-port.js';
interface Stream {
  readonly reader: ReadableStreamBYOBReader;
  readonly maximum: number;
  readonly controller: AbortController;
  readonly detach: () => void;
  offset: number;
  active: Promise<Uint8Array<ArrayBuffer>> | null;
  cancellation: Promise<void> | null;
  closed: boolean;
  closing: Promise<void> | null;
}
/** Shared bounded byte IO for retained originals and native Go API responses.
 * Close joins the reader's visible promises, not unreported provider work. A
 * provider whose cancel waits for this same close or outer read can leave
 * draining pending.
 * No parser, route, credentials or domain policy lives in this reader. */
export class PqcResponseStreams {
  private closed = false;
  private readonly streams = new Map<string, Stream>();
  constructor(
    private readonly reject: () => never,
    private readonly newID: () => string = newHostPortID,
  ) {}
  open(
    response: Response,
    maximum: number,
    controller: AbortController,
    detach: () => void,
  ): string {
    if (this.closed || this.streams.size >= 128 || !Number.isSafeInteger(maximum) || maximum < 0)
      return this.reject();
    // Reject a collision before locking the caller's response. On failure the
    // caller still owns the body and is responsible for cancelling it.
    const id = this.newID();
    if (this.streams.has(id)) return this.reject();
    const body =
      response.body ??
      new ReadableStream<Uint8Array<ArrayBuffer>>({
        type: 'bytes',
        start(controller) {
          controller.close();
        },
      });
    // A body already locked by another owner cannot be cancelled or joined here.
    const reader = body.getReader({ mode: 'byob' });
    this.streams.set(id, {
      reader,
      maximum,
      controller,
      detach,
      offset: 0,
      active: null,
      cancellation: null,
      closed: false,
      closing: null,
    });
    return id;
  }
  async read(
    id: string,
    offset: number,
    length: number,
    signal: AbortSignal,
  ): Promise<Uint8Array<ArrayBuffer>> {
    const stream = this.streams.get(id);
    if (
      !stream ||
      stream.closed ||
      stream.active ||
      !Number.isSafeInteger(offset) ||
      !Number.isSafeInteger(length) ||
      offset !== stream.offset ||
      length < 0 ||
      length > 256 << 10
    )
      return this.reject();
    const pending = this.readStream(stream, Math.min(length, stream.maximum - offset), signal);
    stream.active = pending;
    try {
      const result = await pending;
      try {
        // The read can settle just before close or caller abort gets a turn.
        // Recheck before resolving our own promise and erase rejected bytes.
        signal.throwIfAborted();
        stream.controller.signal.throwIfAborted();
        if (stream.closed) return this.reject();
        return result;
      } catch (error) {
        result.fill(0);
        throw error;
      }
    } catch (error) {
      // pending is settled before this catch, so joining cancellation cannot
      // wait on this read. Concurrent close joins the same cancellation.
      if (stream.closed || signal.aborted || stream.controller.signal.aborted) {
        stream.closed = true;
        try {
          await this.cancel(
            stream,
            signal.aborted ? signal.reason : stream.controller.signal.reason,
          );
        } catch (cancelError) {
          throw new AggregateError([error, cancelError], 'response_stream_cleanup_failed');
        }
      }
      throw error;
    } finally {
      stream.active = null;
    }
  }
  private async readStream(
    stream: Stream,
    length: number,
    signal: AbortSignal,
  ): Promise<Uint8Array<ArrayBuffer>> {
    const output = new Uint8Array(length);
    let written = 0,
      last: Uint8Array<ArrayBuffer> | undefined;
    const check = () => {
      signal.throwIfAborted();
      stream.controller.signal.throwIfAborted();
      if (stream.closed) return this.reject();
    };
    const abort = (reason: unknown) => {
      stream.closed = true;
      void this.cancel(stream, reason);
    };
    const abortCaller = () => abort(signal.reason),
      abortOwner = () => abort(stream.controller.signal.reason);
    signal.addEventListener('abort', abortCaller, { once: true });
    stream.controller.signal.addEventListener('abort', abortOwner, { once: true });
    if (signal.aborted) abortCaller();
    if (stream.controller.signal.aborted) abortOwner();
    try {
      check();
      if (length === 0) {
        const end = await stream.reader.read(new Uint8Array(1));
        last = end.value;
        check();
        if (!end.done || last?.byteLength) return this.reject();
        return output;
      }
      while (written < length) {
        check();
        const value = await stream.reader.read(new Uint8Array(length - written));
        last = value.value;
        check();
        if (last?.byteLength) {
          if (last.byteLength > length - written) return this.reject();
          output.set(last, written);
          written += last.byteLength;
          last.fill(0);
          last = undefined;
        } else if (!value.done) return this.reject();
        if (value.done) break;
      }
      check();
      stream.offset += written;
      if (written === length) return output;
      const result = output.slice(0, written);
      output.fill(0);
      return result;
    } catch (error) {
      output.fill(0);
      throw error;
    } finally {
      last?.fill(0);
      signal.removeEventListener('abort', abortCaller);
      stream.controller.signal.removeEventListener('abort', abortOwner);
    }
  }
  private cancel(stream: Stream, reason?: unknown): Promise<void> {
    if (!stream.cancellation) {
      // Install the shared promise before invoking cancel, including when the
      // underlying source synchronously triggers another close or abort.
      stream.cancellation = Promise.resolve().then(() => stream.reader.cancel(reason));
      // The abort listener cannot await; closeStream still observes the error.
      void stream.cancellation.catch(() => {});
    }
    return stream.cancellation;
  }
  async closeStream(id: string): Promise<void> {
    const stream = this.streams.get(id);
    if (!stream) return;
    if (!stream.closing) {
      stream.closed = true;
      // Publish the sticky close before abort or detach can reenter this method.
      stream.closing = Promise.resolve().then(async () => {
        const errors: unknown[] = [];
        try {
          stream.controller.abort();
        } catch (error) {
          errors.push(error);
        }
        try {
          stream.detach();
        } catch (error) {
          errors.push(error);
        }
        const cancellation = this.cancel(stream);
        const [cancelResult] = await Promise.allSettled([
          cancellation,
          ...(stream.active ? [stream.active] : []),
        ]);
        if (cancelResult?.status === 'rejected') errors.push(cancelResult.reason);
        try {
          stream.reader.releaseLock();
        } catch (error) {
          errors.push(error);
        }
        if (errors.length === 1) throw errors[0];
        if (errors.length > 1) throw new AggregateError(errors, 'response_stream_cleanup_failed');
        // Failed cleanup remains in the map with its rejected closing promise:
        // future close/closeStream calls must not report success or reuse its ID.
        this.streams.delete(id);
      });
    }
    await stream.closing;
  }
  async close(): Promise<void> {
    this.closed = true;
    const results = await Promise.allSettled(
      [...this.streams.keys()].map((id) => this.closeStream(id)),
    );
    const errors = results.filter(
      (result): result is PromiseRejectedResult => result.status === 'rejected',
    );
    if (errors.length)
      throw new AggregateError(
        errors.map((result): unknown => result.reason),
        'response_stream_cleanup_failed',
      );
  }
}
