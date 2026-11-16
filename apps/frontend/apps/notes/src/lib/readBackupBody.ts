// SPDX-License-Identifier: Apache-2.0

/**
 * Read actual bytes, including chunked requests, without buffering past the limit.
 * The absolute deadline also bounds a peer that never resolves a read.
 */
export async function readBackupBody(
  request: Request,
  limit = 32 * 1024 * 1024,
  deadlineMs = 120_000,
): Promise<string> {
  return new TextDecoder('utf-8', { fatal: true }).decode(
    await readBackupBytes(request, limit, deadlineMs),
  );
}

/**
 * The same bounded read as `readBackupBody`, returning the exact bytes: a ZIP
 * of Markdown is binary, and decoding it as text would corrupt it.
 */
export async function readBackupBytes(
  request: Request,
  limit = 32 * 1024 * 1024,
  deadlineMs = 120_000,
): Promise<Uint8Array<ArrayBuffer>> {
  if (!Number.isSafeInteger(deadlineMs) || deadlineMs < 1)
    throw new Error('invalid backup deadline');
  const signal = request.signal;
  if (signal.aborted) throw signal.reason ?? new DOMException('Aborted', 'AbortError');
  if (!request.body) return new Uint8Array(0);
  const reader = request.body.getReader();
  // Aggregate network chunks into fixed-size blocks. A peer may send a large
  // number of tiny chunks, so retaining one string per read is not bounded by
  // the byte limit alone.
  const blocks: Uint8Array<ArrayBuffer>[] = [];
  const blockSize = 64 * 1024;
  const maxChunks = 16 * 1024;
  let size = 0;
  let used = 0;
  let chunks = 0;
  let rejectStopped!: (reason: unknown) => void;
  const stopped = new Promise<never>((_, reject) => {
    rejectStopped = reject;
  });
  const onAbort = () => rejectStopped(signal.reason ?? new DOMException('Aborted', 'AbortError'));
  const timer = setTimeout(
    () => rejectStopped(new Error('backup read deadline exceeded')),
    deadlineMs,
  );
  signal.addEventListener('abort', onAbort, { once: true });
  const read = async (): Promise<Uint8Array<ArrayBuffer>> => {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      if (++chunks > maxChunks) throw new Error('too many backup fragments');
      if (value.byteLength > limit - size) throw new Error('backup too large');
      let offset = 0;
      while (offset < value.byteLength) {
        let block = blocks.at(-1);
        if (!block || used === block.length) {
          block = new Uint8Array(Math.min(blockSize, limit - size));
          blocks.push(block);
          used = 0;
        }
        const count = Math.min(block.length - used, value.byteLength - offset);
        block.set(value.subarray(offset, offset + count), used);
        offset += count;
        used += count;
        size += count;
      }
    }
    const out = new Uint8Array(size);
    let at = 0;
    for (const [index, block] of blocks.entries()) {
      const part = index === blocks.length - 1 ? block.subarray(0, used) : block;
      out.set(part, at);
      at += part.byteLength;
    }
    return out;
  };
  try {
    // Register the listener before reading and close the gap with a second
    // signal check; both deadline and abort cover every read with one timer.
    if (signal.aborted) onAbort();
    return await Promise.race([read(), stopped]);
  } catch (error) {
    // A peer-controlled stream may never settle cancellation. Start it, but do
    // not let that prevent the rejected request from returning.
    try {
      void reader.cancel(error).catch(() => undefined);
    } catch {
      // Preserve the failure even if an unusual stream throws synchronously.
    }
    throw error;
  } finally {
    clearTimeout(timer);
    signal.removeEventListener('abort', onAbort);
    try {
      reader.releaseLock();
    } catch {
      // Cancellation is best effort when the source ignores both cancellation
      // and release; the response must not wait for that source.
    }
  }
}
