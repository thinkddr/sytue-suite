import { encodeStorageScopeHeader } from '@sytue/config/matrix';
/* SPDX-License-Identifier: Apache-2.0 */
import { expect, spyOn, test } from 'bun:test';
import { PqcServicePort, privateServiceRoute } from './service-port.js';
import { PqcServiceSource } from './service-source.js';
import { PqcWorkPort } from './work-port.js';
import { decodeHostObject, encodeHostJSON } from './host-codec.js';
import type { HostCallContext, HostScope } from './host-port.js';
import type { PqcRuntime } from './runtime.js';
const scope = { tenant: 'tenant', subject: 'owner', product: 'mail' };
const controller = new AbortController();
const context: HostCallContext = { scope, signal: controller.signal, assertOpen() {} };
Object.defineProperty(globalThis, 'location', {
  configurable: true,
  value: new URL('https://mail.test'),
});
function frame(
  operation: string,
  body = new Uint8Array(),
  extra: Record<string, unknown> = {},
): Uint8Array<ArrayBuffer> {
  const metadata = encodeHostJSON({ operation, size: body.length, ...extra });
  const output = new Uint8Array(4 + metadata.length + body.length);
  new DataView(output.buffer).setUint32(0, metadata.length, false);
  output.set(metadata, 4);
  output.set(body, 4 + metadata.length);
  return output;
}
function bytesResponse(value: Uint8Array): Response {
  let sent = false;
  return new Response(
    new ReadableStream({
      type: 'bytes',
      pull(output) {
        if (sent) {
          output.close();
          (output as ReadableByteStreamController).byobRequest?.respond(0);
          return;
        }
        sent = true;
        output.enqueue(value.slice());
      },
    }),
    { headers: { 'Content-Type': 'application/json', 'Content-Length': String(value.length) } },
  );
}
const unavailable: Pick<PqcRuntime, 'invoke'> = {
  invoke: async () => {
    throw new Error('unexpected core source call');
  },
};
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
test('fixed native API operations keep raw response bytes and reject scope/route/header choices before fetch', async () => {
  let fetched = 0,
    verified = 0,
    expectedPath = 'https://mail.test/base/api/private-storage/migration',
    expectedMethod = 'GET';
  const raw = new TextEncoder().encode(
    '{"integer":900719925474099312345,"private":"client metadata"}',
  );
  const host = new PqcServicePort(
    scope,
    {
      signal: controller.signal,
      assertOpen() {},
      revalidate: async () => {
        verified++;
      },
      fetch: async (input, init) => {
        fetched++;
        expect(String(input)).toBe(expectedPath);
        expect(init?.method).toBe(expectedMethod);
        expect(init?.redirect).toBe('error');
        expect(new Headers(init?.headers).has('Authorization')).toBe(false);
        return bytesResponse(raw);
      },
    },
    unavailable,
    () => 'a'.repeat(64),
    '/base',
    50 << 20,
  );
  const metadata = await host.handlers['service.request']!(frame('mail.source.state'), context);
  const value = decodeHostObject(metadata, () => {
    throw Error('bad fixture');
  });
  const read = await host.handlers['service.read']!(
    encodeHostJSON({ stream: value.stream, offset: 0, length: 256 << 10 }),
    context,
  );
  expect(read).toEqual(raw);
  expect(fetched).toBe(1);
  expect(verified).toBe(2);
  await expect(
    host.handlers['service.request']!(
      frame('mail.source.state', new Uint8Array(), { url: 'https://outside.test' }),
      context,
    ),
  ).rejects.toThrow();
  await expect(
    host.handlers['service.request']!(frame('mail.source.state'), {
      ...context,
      scope: { ...scope, subject: 'other' },
    }),
  ).rejects.toThrow();
  await expect(
    host.handlers['service.request']!(
      frame('mail.source.page', new Uint8Array(), { arguments: ['bad-epoch', ''] }),
      context,
    ),
  ).rejects.toThrow();
  expect(fetched).toBe(1);
  await host.handlers['service.close']!(encodeHostJSON({ stream: value.stream }), context);
  expectedPath = 'https://mail.test/base/api/private-storage/ingress/enrollment';
  expectedMethod = 'POST';
  const enrollmentBody = encodeHostJSON({ epoch: '22222222-2222-4222-8222-222222222222' });
  const enrollment = decodeHostObject(
    await host.handlers['service.request']!(
      frame('mail.ingress.enrollment', enrollmentBody, { content_type: 'application/json' }),
      context,
    ),
    () => {
      throw Error('bad fixture');
    },
  );
  expect(fetched).toBe(2);
  await host.handlers['service.close']!(encodeHostJSON({ stream: enrollment.stream }), context);
  await expect(
    host.handlers['service.request']!(
      frame('mail.ingress.enrollment.other', enrollmentBody, { content_type: 'application/json' }),
      context,
    ),
  ).rejects.toThrow();
  expect(fetched).toBe(2);
  enrollmentBody.fill(0);
  await host.close();
  raw.fill(0);
  read.fill(0);
});
test('a replaced session is rejected before fetch and a late response is canceled before any output', async () => {
  let fetched = 0,
    canceled = 0,
    valid = true;
  const witness = {
    signal: controller.signal,
    assertOpen() {},
    revalidate: async () => {
      if (!valid) throw Error('session replaced');
    },
    fetch: async () => {
      fetched++;
      valid = false;
      return new Response(
        new ReadableStream({
          type: 'bytes',
          cancel() {
            canceled++;
          },
        }),
      );
    },
  };
  const host = new PqcServicePort(scope, witness, unavailable, () => 'a'.repeat(64), '/', 50 << 20);
  await expect(
    host.handlers['service.request']!(frame('mail.source.state'), context),
  ).rejects.toThrow('session replaced');
  expect(fetched).toBe(1);
  expect(canceled).toBe(1);
  await expect(
    host.handlers['service.request']!(frame('mail.source.state'), context),
  ).rejects.toThrow('session replaced');
  expect(fetched).toBe(1);
  await host.close();
});
test('work witnesses release exactly once, join every cleanup callback, and never mix actors', async () => {
  let released = 0,
    verified = 0,
    acquired = 0;
  const work = new PqcWorkPort(
    scope,
    { signal: controller.signal, assertOpen() {} },
    {
      revalidate: async () => {
        verified++;
      },
      acquire: async () => {
        acquired++;
        return () => {
          released++;
          if (acquired === 99) throw Error('synthetic');
        };
      },
    },
  );
  await work.handlers['authority.revalidate']!(new Uint8Array(), context);
  const raw = await work.handlers['lease.acquire']!(new Uint8Array(), context);
  const value = decodeHostObject(raw, () => {
    throw Error('bad fixture');
  });
  expect(verified).toBe(1);
  expect(released).toBe(0);
  await work.handlers['lease.release']!(encodeHostJSON(value), context);
  await work.handlers['lease.release']!(encodeHostJSON(value), context);
  expect(released).toBe(1);
  await expect(
    work.handlers['lease.acquire']!(new Uint8Array(), {
      ...context,
      scope: { ...scope, product: 'drive' } as HostScope,
    }),
  ).rejects.toThrow();
  await work.handlers['lease.acquire']!(new Uint8Array(), context);
  await work.handlers['lease.acquire']!(new Uint8Array(), context);
  await work.close();
  expect(released).toBe(3);
  await expect(work.handlers['lease.acquire']!(new Uint8Array(), context)).rejects.toThrow();
});

test('all work cleanup callbacks finish even when one throws synchronously', async () => {
  let acquired = 0,
    joined = 0;
  const work = new PqcWorkPort(
    scope,
    { signal: controller.signal, assertOpen() {} },
    {
      revalidate: async () => {},
      acquire: async () => {
        const index = ++acquired;
        return () => {
          joined++;
          if (index === 1) throw Error('synthetic cleanup failure');
        };
      },
    },
  );
  await work.handlers['lease.acquire']!(new Uint8Array(), context);
  await work.handlers['lease.acquire']!(new Uint8Array(), context);
  await expect(work.close()).rejects.toThrow('work_port_cleanup_failed');
  expect(joined).toBe(2);
});

test('a rejected descriptor is cancelled and joined before the request settles', async () => {
  let checks = 0,
    cancels = 0,
    releaseCancel!: () => void,
    cancellationStarted!: () => void;
  const cancelStarted = new Promise<void>((resolve) => {
    cancellationStarted = resolve;
  });
  const cancelFinished = new Promise<void>((resolve) => {
    releaseCancel = resolve;
  });
  const host = new PqcServicePort(
    scope,
    {
      signal: controller.signal,
      assertOpen() {},
      revalidate: async () => {},
      fetch: async () =>
        new Response(
          new ReadableStream({
            type: 'bytes',
            async cancel() {
              cancels++;
              cancellationStarted();
              await cancelFinished;
            },
          }),
        ),
    },
    unavailable,
    () => 'a'.repeat(64),
    '/',
    50 << 20,
  );
  const rejectedContext: HostCallContext = {
    ...context,
    assertOpen() {
      if (++checks === 4) throw new Error('descriptor witness expired');
    },
  };
  let settled = false;
  const request = host.handlers['service.request']!(
    frame('mail.source.state'),
    rejectedContext,
  ).then(
    () => {
      settled = true;
      return undefined;
    },
    (error: unknown) => {
      settled = true;
      return error;
    },
  );
  let testFailure: { error: unknown } | undefined;
  try {
    await bounded(cancelStarted, 'descriptor cancellation');
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect(settled).toBe(false);
    releaseCancel();
    const failure = await bounded(request, 'rejected descriptor');
    expect(failure).toBeInstanceOf(Error);
    expect((failure as Error).message).toContain('descriptor witness expired');
    await bounded(host.close(), 'descriptor host close');
    expect(cancels).toBe(1);
  } catch (error) {
    testFailure = { error };
  }
  releaseCancel();
  try {
    await bounded(Promise.allSettled([request, host.close()]), 'descriptor teardown');
  } catch (error) {
    if (testFailure !== undefined)
      throw new AggregateError([testFailure.error, error], 'test teardown failed');
    throw error;
  }
  if (testFailure !== undefined) throw testFailure.error;
}, 30000);

test('request preserves both revalidation and response cancellation errors', async () => {
  let validations = 0,
    cancels = 0;
  const expired = new Error('expired after fetch');
  const cancellation = new Error('response cancel failed');
  const host = new PqcServicePort(
    scope,
    {
      signal: controller.signal,
      assertOpen() {},
      revalidate: async () => {
        if (++validations === 2) throw expired;
      },
      fetch: async () =>
        new Response(
          new ReadableStream({
            type: 'bytes',
            cancel() {
              cancels++;
              throw cancellation;
            },
          }),
        ),
    },
    unavailable,
    () => 'a'.repeat(64),
    '/',
    50 << 20,
  );
  let failure: unknown;
  try {
    await host.handlers['service.request']!(frame('mail.source.state'), context);
  } catch (error) {
    failure = error;
  }
  expect(failure).toBeInstanceOf(AggregateError);
  expect((failure as AggregateError).errors).toEqual([expired, cancellation]);
  expect(cancels).toBe(1);
  await host.close();
});

test('a response locked by another reader is reported without disturbing that reader', async () => {
  let validations = 0,
    cancellations = 0;
  const expired = new Error('expired after fetch');
  const response = new Response(
    new ReadableStream({
      type: 'bytes',
      cancel() {
        cancellations++;
      },
    }),
  );
  const reader = response.body!.getReader();
  const host = new PqcServicePort(
    scope,
    {
      signal: controller.signal,
      assertOpen() {},
      revalidate: async () => {
        if (++validations === 2) throw expired;
      },
      fetch: async () => response,
    },
    unavailable,
    () => 'a'.repeat(64),
    '/',
    50 << 20,
  );
  try {
    let failure: unknown;
    try {
      await host.handlers['service.request']!(frame('mail.source.state'), context);
    } catch (error) {
      failure = error;
    }
    expect(failure).toBeInstanceOf(AggregateError);
    expect((failure as AggregateError).errors[0]).toBe(expired);
    expect((failure as AggregateError).errors[1]).toEqual(
      new Error('service_response_body_locked'),
    );
    expect(response.body!.locked).toBe(true);
    expect(cancellations).toBe(0);
    await host.close();
  } finally {
    reader.releaseLock();
  }
});

test('a failing buffered release still closes an already opened response stream', async () => {
  const requestConstructor = globalThis.Request;
  const releaseFailure = new Error('buffer release failed');
  const cancelFailure = new Error('opened response cancel failed');
  let cancellations = 0;
  globalThis.Request = class extends requestConstructor {
    constructor(...args: ConstructorParameters<typeof Request>) {
      super(...args);
      throw new Error('request streaming unavailable');
    }
  } as typeof Request;
  const buffer = spyOn(PqcServiceSource.prototype, 'buffer').mockResolvedValue({
    bytes: Uint8Array.of(120),
    release() {
      throw releaseFailure;
    },
  });
  const host = new PqcServicePort(
    scope,
    {
      signal: controller.signal,
      assertOpen() {},
      revalidate: async () => {},
      fetch: async () =>
        new Response(
          new ReadableStream({
            type: 'bytes',
            cancel() {
              cancellations++;
              throw cancelFailure;
            },
          }),
        ),
    },
    unavailable,
    () => 'a'.repeat(64),
    '/',
    50 << 20,
  );
  try {
    let failure: unknown;
    try {
      await host.handlers['service.request']!(
        frame('mail.submission.send', new Uint8Array(), {
          content_type: 'message/rfc822',
          submission: 'safe',
          source: 'b'.repeat(64),
          size: 1,
        }),
        context,
      );
    } catch (error) {
      failure = error;
    }
    expect(failure).toBeInstanceOf(AggregateError);
    expect((failure as AggregateError).errors).toEqual([releaseFailure, cancelFailure]);
    expect(cancellations).toBe(1);
    await expect(host.close()).rejects.toThrow('service_port_cleanup_failed');
    expect(cancellations).toBe(1);
  } finally {
    buffer.mockRestore();
    globalThis.Request = requestConstructor;
  }
});

test('concurrent port closes join port-initiated upload and response cleanup once', async () => {
  let resolveFetch!: (response: Response) => void,
    fetchStarted!: () => void,
    releaseUpload!: () => void,
    sourceCloses = 0,
    responseCancels = 0,
    fetches = 0;
  const started = new Promise<void>((resolve) => {
    fetchStarted = resolve;
  });
  const delayedFetch = new Promise<Response>((resolve) => {
    resolveFetch = resolve;
  });
  const uploadFinished = new Promise<void>((resolve) => {
    releaseUpload = resolve;
  });
  const uploadFailure = new Error('upload cleanup failed');
  const responseFailure = new Error('response cleanup failed');
  const close = spyOn(PqcServiceSource.prototype, 'close').mockImplementation(async () => {
    sourceCloses++;
    await uploadFinished;
    throw uploadFailure;
  });
  const runtime = {
    invoke: async (request: { operation: string }) =>
      request.operation === 'core.source.read'
        ? Uint8Array.of(120)
        : { sha256: '2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881' },
  } as unknown as Pick<PqcRuntime, 'invoke'>;
  const host = new PqcServicePort(
    scope,
    {
      signal: controller.signal,
      assertOpen() {},
      revalidate: async () => {},
      fetch: async () => {
        if (++fetches === 1)
          return new Response(
            new ReadableStream({
              type: 'bytes',
              cancel() {
                responseCancels++;
                throw responseFailure;
              },
            }),
          );
        fetchStarted();
        return delayedFetch;
      },
    },
    runtime,
    () => 'a'.repeat(64),
    '/',
    50 << 20,
  );
  let request: Promise<unknown> | undefined;
  let pendingResults: Promise<PromiseSettledResult<void>[]> | undefined;
  let testFailure: { error: unknown } | undefined;
  try {
    await host.handlers['service.request']!(frame('mail.source.state'), context);
    request = host.handlers['service.request']!(
      frame('mail.submission.send', new Uint8Array(), {
        content_type: 'message/rfc822',
        submission: 'safe',
        source: 'b'.repeat(64),
        size: 1,
      }),
      context,
    ).then(
      () => undefined,
      (error: unknown) => error,
    );
    await bounded(started, 'upload fetch');
    let settled = false;
    const first = host.close().finally(() => {
      settled = true;
    });
    const second = host.close();
    pendingResults = Promise.allSettled([first, second, host.close()]);
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect(settled).toBe(false);
    releaseUpload();
    const results = await bounded(pendingResults, 'concurrent host closes');
    expect(results.every((result) => result.status === 'rejected')).toBe(true);
    const error = (results[0] as PromiseRejectedResult).reason as AggregateError;
    expect(error.errors).toContain(uploadFailure);
    expect(
      error.errors.some(
        (entry: unknown) =>
          entry instanceof AggregateError && entry.errors.includes(responseFailure),
      ),
    ).toBe(true);
    expect(sourceCloses).toBe(1);
    expect(responseCancels).toBe(1);
    resolveFetch(new Response(new ReadableStream({ type: 'bytes' })));
    expect(await bounded(request, 'late upload response')).toBeInstanceOf(Error);
    expect(sourceCloses).toBe(1);
    await expect(host.close()).rejects.toThrow('service_port_cleanup_failed');
  } catch (error) {
    testFailure = { error };
  }
  releaseUpload();
  resolveFetch(new Response(null));
  try {
    await bounded(
      Promise.allSettled([
        host.close(),
        ...(request ? [request] : []),
        ...(pendingResults ? [pendingResults] : []),
      ]),
      'concurrent close teardown',
    );
  } catch (error) {
    if (testFailure !== undefined)
      throw new AggregateError([testFailure.error, error], 'test teardown failed');
    throw error;
  } finally {
    close.mockRestore();
  }
  if (testFailure !== undefined) throw testFailure.error;
}, 30000);

test('Contacts source shares the host transport but cannot select Mail, CRUD or another scope', async () => {
  const contacts = { ...scope, product: 'contacts' };
  const ctx = { ...context, scope: contacts };
  let fetched = 0,
    verified = 0;
  const host = new PqcServicePort(
    contacts,
    {
      signal: controller.signal,
      assertOpen() {},
      revalidate: async () => {
        verified++;
      },
      fetch: async (input, init) => {
        fetched++;
        expect(String(input)).toBe('https://mail.test/contacts/api/private-storage/migration');
        expect(new Headers(init?.headers).get('X-Sytue-Storage-Scope')).toBe(
          encodeStorageScopeHeader(contacts),
        );
        expect(init?.redirect).toBe('error');
        return bytesResponse(Uint8Array.of(123, 125));
      },
    },
    unavailable,
    () => 'a'.repeat(64),
    '/contacts',
    8 << 20,
  );
  const value = decodeHostObject(
    await host.handlers['service.request']!(frame('contacts.source.state'), ctx),
    () => {
      throw Error('fixture');
    },
  );
  expect(fetched).toBe(1);
  expect(verified).toBe(2);
  for (const operation of [
    'mail.source.state',
    'mail.routing.get',
    'mail.ingress.body',
    'drive.cipher.usage',
    'contacts.cards.list',
  ])
    await expect(host.handlers['service.request']!(frame(operation), ctx)).rejects.toThrow();
  await expect(
    host.handlers['service.request']!(frame('contacts.source.state'), context),
  ).rejects.toThrow();
  expect(fetched).toBe(1);
  await host.handlers['service.close']!(encodeHostJSON({ stream: value.stream }), ctx);
  await host.close();
  expect(
    privateServiceRoute(
      'contacts.source.open',
      ['11111111-1111-4111-8111-111111111111', 'card', '22222222-2222-4222-8222-222222222222'],
      'contacts',
    ),
  ).toEqual([
    'GET',
    '/private-storage/migration/11111111-1111-4111-8111-111111111111/blobs/card/22222222-2222-4222-8222-222222222222',
  ]);
  for (const args of [
    [
      '11111111-1111-4111-8111-111111111111',
      'mime_message',
      '22222222-2222-4222-8222-222222222222',
    ],
    ['11111111-1111-4111-8111-111111111111', 'card', '00000000-0000-0000-0000-000000000000'],
  ])
    expect(() => privateServiceRoute('contacts.source.open', args, 'contacts')).toThrow();
  expect(() => privateServiceRoute('contacts.source.state', [], 'mail')).toThrow();
  expect(() => privateServiceRoute('mail.source.state', [], 'contacts')).toThrow();
});

test('Contacts source counts actual streamed bytes against its 8 MiB limit', async () => {
  const contacts = { ...scope, product: 'contacts' },
    ctx = { ...context, scope: contacts };
  const host = new PqcServicePort(
    contacts,
    {
      signal: controller.signal,
      assertOpen() {},
      revalidate: async () => {},
      fetch: async () => {
        let remaining = (8 << 20) + 1;
        return new Response(
          new ReadableStream({
            type: 'bytes',
            pull(output) {
              if (!remaining) {
                output.close();
                (output as ReadableByteStreamController).byobRequest?.respond(0);
                return;
              }
              const chunk = Math.min(remaining, 256 << 10);
              remaining -= chunk;
              output.enqueue(new Uint8Array(chunk));
            },
          }),
        );
      },
    },
    unavailable,
    () => 'a'.repeat(64),
    '/contacts',
    8 << 20,
  );
  const value = decodeHostObject(
    await host.handlers['service.request']!(
      frame('contacts.source.open', new Uint8Array(), {
        arguments: [
          '11111111-1111-4111-8111-111111111111',
          'card',
          '22222222-2222-4222-8222-222222222222',
        ],
      }),
      ctx,
    ),
    () => {
      throw Error('fixture');
    },
  );
  let offset = 0,
    rejected = false;
  try {
    for (;;) {
      const bytes = await host.handlers['service.read']!(
        encodeHostJSON({ stream: value.stream, offset, length: 256 << 10 }),
        ctx,
      );
      if (!bytes.length) break;
      offset += bytes.length;
      bytes.fill(0);
    }
  } catch {
    rejected = true;
  }
  expect(rejected).toBe(true);
  expect(offset).toBeLessThanOrEqual(8 << 20);
  await host.close();
});
