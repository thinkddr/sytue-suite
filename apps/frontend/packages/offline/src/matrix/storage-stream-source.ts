/* SPDX-License-Identifier: Apache-2.0 */
/** Sequential HTTP input for encrypted capture. Consumers own and clear yielded byte chunks. */
import { maxCipherBlockBytes } from '@sytue/config/matrix';
import { invalid } from './storage-file-codec.js';
import type { StorageFileSource } from './storage-file-upload.js';
export interface StorageCaptureSource extends StorageFileSource {
  finish?: (signal: AbortSignal) => Promise<void>;
  close?: () => Promise<void>;
}
/** Default readers work across supported browsers; no BYOB or whole-response buffering. */
export function streamCaptureSource(
  input: ReadableStream<Uint8Array<ArrayBuffer>>,
  size: number,
  signal: AbortSignal,
): StorageCaptureSource {
  const reader = input.getReader();
  let held: Uint8Array<ArrayBuffer> | undefined,
    start = 0,
    consumed = 0,
    received = 0,
    finished = false,
    closed = false,
    cancelling: Promise<void> | undefined,
    cancelError: Error | undefined;
  const check = () => {
    signal.throwIfAborted();
    if (closed) invalid();
  };
  const stop = () => {
    cancelling ??= reader.cancel(signal.reason).catch((error) => {
      cancelError = error instanceof Error ? error : new Error('storage_source_cancelled');
    });
    return cancelling;
  };
  const abort = () => {
    void stop();
  };
  signal.addEventListener('abort', abort, { once: true });
  if (signal.aborted) abort();
  const next = async (): Promise<Uint8Array<ArrayBuffer> | undefined> => {
    check();
    const result = await reader.read();
    try {
      check();
      if (result.done) return undefined;
      if (
        !(result.value instanceof Uint8Array) ||
        result.value.length === 0 ||
        result.value.length > maxCipherBlockBytes ||
        result.value.length > size - received
      )
        invalid();
      received += result.value.length;
      return result.value;
    } catch (error) {
      result.value?.fill(0);
      throw error;
    }
  };
  return {
    size,
    read: async (offset, length) => {
      check();
      if (offset !== consumed || length < 0 || offset > size - length) invalid();
      const out = new Uint8Array(length);
      let used = 0;
      try {
        while (used < length) {
          check();
          if (!held) {
            held = await next();
            start = 0;
            if (!held) invalid();
          }
          check();
          const n = Math.min(length - used, held.length - start);
          out.set(held.subarray(start, start + n), used);
          start += n;
          used += n;
          consumed += n;
          if (start === held.length) {
            held.fill(0);
            held = undefined;
          }
        }
        check();
        return out;
      } catch (error) {
        out.fill(0);
        throw error;
      }
    },
    finish: async () => {
      check();
      if (consumed !== size || held) invalid();
      const extra = await next();
      if (extra) {
        extra.fill(0);
        invalid();
      }
      finished = true;
    },
    close: async () => {
      if (closed) return;
      closed = true;
      signal.removeEventListener('abort', abort);
      held?.fill(0);
      held = undefined;
      try {
        if (!finished || cancelling) await stop();
        if (cancelError) throw cancelError;
      } finally {
        reader.releaseLock();
      }
    },
  };
}
