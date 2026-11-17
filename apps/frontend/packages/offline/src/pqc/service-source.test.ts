/* SPDX-License-Identifier: Apache-2.0 */
import { expect, spyOn, test } from 'bun:test';
import { PqcServiceSource } from './service-source.js';
import type { PqcRuntime } from './runtime.js';

const scope = { tenant: 'tenant', subject: 'owner', product: 'mail' };
const port = 'a'.repeat(64),
  source = 'b'.repeat(64);
async function bounded<T>(promise: Promise<T>, label: string): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([
      promise,
      new Promise<never>((_resolve, reject) => {
        timer = setTimeout(() => reject(new Error(`timed out: ${label}`)), 2000);
      }),
    ]);
  } finally {
    if (timer !== undefined) clearTimeout(timer);
  }
}
test('transient compatibility buffer checks Go EOF/SHA and erases after fetch; only one is retained', async () => {
  const original = new TextEncoder().encode('exact original MIME\r\n'.repeat(18000));
  const hash = new Uint8Array(await crypto.subtle.digest('SHA-256', original));
  const checksum = Array.from(hash, (b) => b.toString(16).padStart(2, '0')).join('');
  hash.fill(0);
  let reads = 0,
    finals = 0;
  const runtime: Pick<PqcRuntime, 'invoke'> = {
    invoke: async <T>(request) => {
      const input = JSON.parse(new TextDecoder().decode(request.input));
      expect(request.port).toBe(port);
      expect(request.scope).toEqual(scope);
      expect(input.source).toBe(source);
      if (request.operation === 'core.source.finalize') {
        finals++;
        return { sha256: checksum } as T;
      }
      expect(request.operation).toBe('core.source.read');
      expect(input.length).toBeLessThanOrEqual(256 << 10);
      reads++;
      return original.slice(input.offset, input.offset + input.length) as T;
    },
  };
  const signal = new AbortController().signal;
  const retained = new PqcServiceSource(
    runtime,
    port,
    scope,
    source,
    original.length,
    () => {},
    signal,
  );
  const body = await retained.buffer(50 << 20);
  expect(body.bytes).toEqual(original);
  expect(reads).toBeGreaterThan(1);
  expect(finals).toBe(1);
  const second = new PqcServiceSource(
    runtime,
    port,
    scope,
    source,
    original.length,
    () => {},
    signal,
  );
  await expect(second.buffer(50 << 20)).rejects.toThrow('mail_upload_busy');
  body.release();
  body.release();
  expect(body.bytes.every((byte) => byte === 0)).toBe(true);
  const next = await second.buffer(50 << 20);
  next.release();
  await retained.close();
  await second.close();
  original.fill(0);
});
test('wrong Go hash, maximum and late canceled reads never produce an upload buffer', async () => {
  const original = new TextEncoder().encode('private');
  const runtime: Pick<PqcRuntime, 'invoke'> = {
    invoke: async <T>(request) =>
      (request.operation === 'core.source.finalize'
        ? { sha256: '0'.repeat(64) }
        : original.slice()) as T,
  };
  const controller = new AbortController();
  const wrong = new PqcServiceSource(
    runtime,
    port,
    scope,
    source,
    original.length,
    () => {},
    controller.signal,
  );
  await expect(wrong.buffer(50 << 20)).rejects.toThrow('mail_source_rejected');
  await expect(wrong.buffer(1)).rejects.toThrow('mail_upload_streaming_required');
  const late = new Uint8Array([1, 2, 3]);
  let deliver!: (value: unknown) => void, entered!: () => void;
  const started = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const pending = new Promise<unknown>((resolve) => {
    deliver = resolve;
  });
  const blocked: Pick<PqcRuntime, 'invoke'> = {
    invoke: <T>() => {
      entered();
      return pending as Promise<T>;
    },
  };
  const reader = new PqcServiceSource(blocked, port, scope, source, 3, () => {}, controller.signal);
  const result = reader.buffer(50 << 20);
  await started;
  let closed = false;
  const drained = reader.close().then(() => {
    closed = true;
  });
  await Promise.resolve();
  expect(closed).toBe(false);
  deliver(late);
  await expect(result).rejects.toThrow();
  await drained;
  expect(late.every((byte) => byte === 0)).toBe(true);
  await wrong.close();
  original.fill(0);
});

test('request stream cancellation and direct closes join the same retained read', async () => {
  const late = Uint8Array.of(1, 2, 3);
  let deliver!: (value: unknown) => void,
    entered!: () => void,
    seenSignal: AbortSignal | undefined,
    abortClose: Promise<void> | undefined;
  const started = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const pending = new Promise<unknown>((resolve) => {
    deliver = resolve;
  });
  const runtime: Pick<PqcRuntime, 'invoke'> = {
    invoke: <T>(_request, signal) => {
      seenSignal = signal;
      signal?.addEventListener(
        'abort',
        () => {
          abortClose = owner.close();
        },
        { once: true },
      );
      entered();
      return pending as Promise<T>;
    },
  };
  const owner = new PqcServiceSource(
    runtime,
    port,
    scope,
    source,
    3,
    () => {},
    new AbortController().signal,
  );
  const reader = owner.stream().getReader();
  const read = reader.read().then(
    () => undefined,
    (error: unknown) => error,
  );
  let cancellation: Promise<unknown> | undefined;
  let close: Promise<void> | undefined;
  let testFailure: { error: unknown } | undefined;
  try {
    await bounded(started, 'stream read');
    cancellation = reader.cancel().then(
      () => undefined,
      (error: unknown) => error,
    );
    close = owner.close();
    expect(owner.close()).toBe(close);
    let settled = false;
    const observedClose = close.then(
      () => {
        settled = true;
      },
      () => {
        settled = true;
      },
    );
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect(settled).toBe(false);
    expect(seenSignal?.aborted).toBe(true);
    expect(abortClose).toBe(close);
    await expect(owner.buffer(50 << 20)).rejects.toThrow();
    deliver(late);
    await bounded(Promise.all([read, cancellation, observedClose]), 'stream cancellation drain');
    expect(late.every((byte) => byte === 0)).toBe(true);
  } catch (error) {
    testFailure = { error };
  }
  deliver(late);
  try {
    await bounded(
      Promise.allSettled([read, owner.close(), ...(cancellation ? [cancellation] : [])]),
      'stream teardown',
    );
  } catch (error) {
    if (testFailure !== undefined)
      throw new AggregateError([testFailure.error, error], 'test teardown failed');
    throw error;
  } finally {
    reader.releaseLock();
  }
  if (testFailure !== undefined) throw testFailure.error;
}, 30000);

test('an abort failure stays sticky while the retained read is joined', async () => {
  const late = Uint8Array.of(1, 2, 3);
  const abortFailure = new Error('synthetic abort failure');
  let deliver!: (value: unknown) => void, entered!: () => void;
  const started = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const pending = new Promise<unknown>((resolve) => {
    deliver = resolve;
  });
  const runtime: Pick<PqcRuntime, 'invoke'> = {
    invoke: <T>() => {
      entered();
      return pending as Promise<T>;
    },
  };
  const owner = new PqcServiceSource(
    runtime,
    port,
    scope,
    source,
    3,
    () => {},
    new AbortController().signal,
  );
  const read = owner.buffer(50 << 20).then(
    () => undefined,
    (error: unknown) => error,
  );
  const cancelController = (owner as unknown as { cancel: AbortController }).cancel;
  const abort = spyOn(cancelController, 'abort').mockImplementation(() => {
    throw abortFailure;
  });
  let close: Promise<void> | undefined;
  let observed: Promise<unknown> | undefined;
  let testFailure: { error: unknown } | undefined;
  try {
    await bounded(started, 'abort failure read');
    close = owner.close();
    expect(owner.close()).toBe(close);
    let settled = false;
    observed = close.then(
      () => {
        settled = true;
        return undefined;
      },
      (error: unknown) => {
        settled = true;
        return error;
      },
    );
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect(settled).toBe(false);
    deliver(late);
    expect(await bounded(observed, 'sticky abort failure')).toBe(abortFailure);
    expect(await owner.close().catch((error: unknown) => error)).toBe(abortFailure);
    await bounded(read, 'abort failure read drain');
    expect(late.every((byte) => byte === 0)).toBe(true);
  } catch (error) {
    testFailure = { error };
  }
  deliver(late);
  try {
    await bounded(
      Promise.allSettled([read, owner.close(), ...(observed ? [observed] : [])]),
      'abort failure teardown',
    );
  } catch (error) {
    if (testFailure !== undefined)
      throw new AggregateError([testFailure.error, error], 'test teardown failed');
    throw error;
  } finally {
    abort.mockRestore();
  }
  if (testFailure !== undefined) throw testFailure.error;
}, 30000);

test('a synchronous reentrant close waits until the launching read is registered', async () => {
  const late = Uint8Array.of(1, 2, 3);
  let deliver!: (value: unknown) => void,
    entered!: () => void,
    reentrant: Promise<void> | undefined;
  const started = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const pending = new Promise<unknown>((resolve) => {
    deliver = resolve;
  });
  const runtime: Pick<PqcRuntime, 'invoke'> = {
    invoke: <T>() => {
      reentrant = owner.close();
      entered();
      return pending as Promise<T>;
    },
  };
  const owner = new PqcServiceSource(
    runtime,
    port,
    scope,
    source,
    3,
    () => {},
    new AbortController().signal,
  );
  const read = owner.buffer(50 << 20).then(
    () => undefined,
    (error: unknown) => error,
  );
  let testFailure: { error: unknown } | undefined;
  try {
    await bounded(started, 'reentrant read');
    expect(reentrant).toBe(owner.close());
    let settled = false;
    const observed = reentrant!.then(
      () => {
        settled = true;
      },
      () => {
        settled = true;
      },
    );
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect(settled).toBe(false);
    deliver(late);
    await bounded(Promise.all([read, observed]), 'reentrant close');
    expect(late.every((byte) => byte === 0)).toBe(true);
  } catch (error) {
    testFailure = { error };
  }
  deliver(late);
  try {
    await bounded(Promise.allSettled([read, owner.close()]), 'reentrant teardown');
  } catch (error) {
    if (testFailure !== undefined)
      throw new AggregateError([testFailure.error, error], 'test teardown failed');
    throw error;
  }
  if (testFailure !== undefined) throw testFailure.error;
}, 30000);
