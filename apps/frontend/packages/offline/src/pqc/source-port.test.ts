/* SPDX-License-Identifier: Apache-2.0 */
import { expect, spyOn, test } from 'bun:test';
import { ResponseLengthError } from '@sytue/api-clients';
import {
  PqcSourcePort,
  type PrivateSourceTransport,
  type SourceDescriptor,
} from './source-port.js';
import { encodeHostJSON } from './host-codec.js';
import type { HostCallContext, HostMethod } from './host-port.js';

const scope = { tenant: 'tenant', subject: 'owner', product: 'mail' },
  epoch = '018f3340-7b12-7000-8000-000000000001';
function withDeadline<T>(work: Promise<T>): Promise<T> {
  let timer!: ReturnType<typeof setTimeout>;
  const deadline = new Promise<never>((_resolve, reject) => {
    timer = setTimeout(() => reject(new Error('source cleanup test timed out')), 3000);
  });
  return Promise.race([work, deadline]).finally(() => clearTimeout(timer));
}
function fixture(source: PrivateSourceTransport) {
  const controller = new AbortController();
  const context: HostCallContext = {
    scope,
    signal: controller.signal,
    assertOpen: () => controller.signal.throwIfAborted(),
  };
  const port = new PqcSourcePort(
    scope,
    { signal: controller.signal, assertOpen: context.assertOpen },
    source,
  );
  const call = async (
    method: HostMethod,
    value: unknown,
    ctx = context,
  ): Promise<Uint8Array<ArrayBuffer>> => {
    const handler = port.handlers[method as Exclude<HostMethod, 'current'>];
    if (!handler) throw new Error('missing handler');
    const input = encodeHostJSON(value);
    try {
      return await handler(input, ctx);
    } finally {
      input.fill(0);
    }
  };
  return { port, context, controller, call };
}
test('retained original host streams split large exact source bytes without materializing whole originals', async () => {
  const original = new Uint8Array((2 << 20) + 17);
  crypto.getRandomValues(original.subarray(0, 65536));
  const descriptor: SourceDescriptor = {
    kind: 'mime_message',
    id: epoch,
    size: original.length,
    sha256: 'a'.repeat(64),
  };
  let position = 0,
    networkReads = 0;
  const f = fixture({
    state: async () => encodeHostJSON({}),
    begin: async () => encodeHostJSON({}),
    page: async () => encodeHostJSON({}),
    open: async () =>
      new Response(
        new ReadableStream<Uint8Array<ArrayBuffer>>({
          type: 'bytes',
          pull(controller) {
            networkReads++;
            if (position === original.length) {
              controller.close();
              (controller as ReadableByteStreamController).byobRequest?.respond(0);
              return;
            }
            const end = Math.min(position + (512 << 10), original.length);
            controller.enqueue(original.slice(position, end));
            position = end;
          },
        }),
        {
          headers: {
            'content-type': 'application/octet-stream',
            'content-length': String(original.length),
          },
        },
      ),
  });
  const opened = JSON.parse(
    new TextDecoder().decode(await f.call('source.open', { epoch, row: descriptor })),
  );
  const output = new Uint8Array(original.length);
  for (let offset = 0; offset < output.length;) {
    const length = Math.min(256 << 10, output.length - offset),
      part = await f.call('source.read', { stream: opened.stream, offset, length });
    expect(part.length).toBe(length);
    output.set(part, offset);
    part.fill(0);
    offset += length;
  }
  expect(
    await f.call('source.read', { stream: opened.stream, offset: output.length, length: 0 }),
  ).toHaveLength(0);
  expect(output).toEqual(original);
  expect(networkReads).toBeGreaterThan(4);
  await f.call('source.close', { stream: opened.stream });
  await f.port.close();
  original.fill(0);
  output.fill(0);
});

test('source shutdown joins a real pending cancel and returns no late bytes', async () => {
  let readEntered!: () => void, finishCancel!: () => void;
  const entered = new Promise<void>((resolve) => {
    readEntered = resolve;
  });
  const cancelGate = new Promise<void>((resolve) => {
    finishCancel = resolve;
  });
  const descriptor: SourceDescriptor = {
    kind: 'mime_message',
    id: epoch,
    size: 6,
    sha256: 'a'.repeat(64),
  };
  const f = fixture({
    state: async () => encodeHostJSON({}),
    begin: async () => encodeHostJSON({}),
    page: async () => encodeHostJSON({}),
    open: async () =>
      new Response(
        new ReadableStream<Uint8Array<ArrayBuffer>>({
          type: 'bytes',
          pull() {
            readEntered();
            return new Promise<void>(() => {});
          },
          cancel() {
            return cancelGate;
          },
        }),
        { headers: { 'content-type': 'application/octet-stream' } },
      ),
  });
  const opened = JSON.parse(
    new TextDecoder().decode(await f.call('source.open', { epoch, row: descriptor })),
  );
  const read = f.call('source.read', { stream: opened.stream, offset: 0, length: 6 });
  const readResult = read.then(
    (value) => ({ value }),
    () => ({ value: null }),
  );
  let closed = false;
  let closing: Promise<void> | undefined;
  try {
    const first = await withDeadline(
      Promise.race([entered.then(() => 'read'), readResult.then(() => 'settled')]),
    );
    expect(first).toBe('read');
    closing = f.port.close();
    void closing.then(
      () => {
        closed = true;
      },
      () => {
        closed = true;
      },
    );
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(closed).toBe(false);
  } finally {
    finishCancel();
  }
  if (!closing) throw new Error('source close did not start');
  await withDeadline(closing);
  expect((await withDeadline(readResult)).value).toBeNull();
  await expect(
    f.call('source.read', { stream: opened.stream, offset: 0, length: 1 }),
  ).rejects.toThrow();
}, 12000);
test('source ports reject another owner and unsafe request before transport IO', async () => {
  let calls = 0;
  const f = fixture({
    state: async () => {
      calls++;
      return encodeHostJSON({});
    },
    begin: async () => {
      calls++;
      return encodeHostJSON({});
    },
    page: async () => {
      calls++;
      return encodeHostJSON({});
    },
    open: async () => {
      calls++;
      return new Response();
    },
  });
  await expect(
    f.call('source.capabilities', {}, { ...f.context, scope: { ...scope, subject: 'another' } }),
  ).rejects.toThrow();
  await expect(f.call('source.page', { epoch, after: '../arbitrary' })).rejects.toThrow();
  await expect(
    f.call('source.begin', { epoch: '../arbitrary', provision: true }),
  ).rejects.toThrow();
  expect(calls).toBe(0);
  await f.port.close();
});

const cleanupDescriptor: SourceDescriptor = {
  kind: 'mime_message',
  id: epoch,
  size: 3,
  sha256: 'a'.repeat(64),
};
function sourceWithResponse(response: Response): PrivateSourceTransport {
  return {
    state: async () => encodeHostJSON({}),
    begin: async () => encodeHostJSON({}),
    page: async () => encodeHostJSON({}),
    open: async () => response,
  };
}

test('invalid source headers join one cancel and preserve the validation and cleanup errors', async () => {
  let finish!: () => void,
    started!: () => void,
    cancellations = 0;
  const gate = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const entered = new Promise<void>((resolve) => {
    started = resolve;
  });
  const cancelError = new Error('cancel failed');
  const response = new Response(
    new ReadableStream({
      type: 'bytes',
      async cancel() {
        cancellations++;
        started();
        await gate;
        throw cancelError;
      },
    }),
    { headers: { 'content-type': 'application/octet-stream', 'content-length': '4' } },
  );
  const f = fixture(sourceWithResponse(response));
  let settled = false;
  const result = f
    .call('source.open', { epoch, row: cleanupDescriptor })
    .then(
      () => undefined,
      (error: unknown) => error,
    )
    .then((error) => {
      settled = true;
      return error;
    });
  try {
    const first = await withDeadline(
      Promise.race([entered.then(() => 'cancel'), result.then(() => 'settled')]),
    );
    expect(first).toBe('cancel');
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(settled).toBe(false);
  } finally {
    finish();
  }
  const error = await withDeadline(result);
  expect(error).toBeInstanceOf(AggregateError);
  const causes = (error as AggregateError).errors;
  expect(causes[0]).toBeInstanceOf(ResponseLengthError);
  expect(causes[1]).toBeInstanceOf(AggregateError);
  expect((causes[1] as AggregateError).errors).toContain(cancelError);
  expect(cancellations).toBe(1);
  await f.port.close();
}, 12000);

test('a source body locked by another reader is neither canceled nor unlocked', async () => {
  let cancellations = 0;
  const body = new ReadableStream<Uint8Array<ArrayBuffer>>({
    type: 'bytes',
    cancel() {
      cancellations++;
    },
  });
  const response = new Response(body, {
    headers: { 'content-type': 'application/octet-stream', 'content-length': '3' },
  });
  const other = body.getReader({ mode: 'byob' });
  const f = fixture(sourceWithResponse(response));
  const error = await f.call('source.open', { epoch, row: cleanupDescriptor }).then(
    () => undefined,
    (reason: unknown) => reason,
  );
  expect(error).toBeInstanceOf(AggregateError);
  expect((error as AggregateError).errors[1]).toBeInstanceOf(AggregateError);
  expect(((error as AggregateError).errors[1] as AggregateError).errors[0].message).toBe(
    'source_response_body_locked',
  );
  expect(body.locked).toBe(true);
  expect(cancellations).toBe(0);
  await f.port.close();
  await other.cancel();
  other.releaseLock();
});

test('serialization failure closes the acquired stream once and retains a failed close', async () => {
  let cancellations = 0;
  const cancelError = new Error('stream cancel failed');
  const f = fixture(
    sourceWithResponse(
      new Response(
        new ReadableStream({
          type: 'bytes',
          cancel() {
            cancellations++;
            throw cancelError;
          },
        }),
        { headers: { 'content-type': 'application/octet-stream', 'content-length': '3' } },
      ),
    ),
  );
  const serializeError = new Error('serialization failed');
  const original = JSON.stringify;
  const stringify = spyOn(JSON, 'stringify').mockImplementation((value: unknown) => {
    if (value && typeof value === 'object' && 'stream' in value && 'row' in value)
      throw serializeError;
    return original(value);
  });
  try {
    const error = await f.call('source.open', { epoch, row: cleanupDescriptor }).then(
      () => undefined,
      (reason: unknown) => reason,
    );
    expect(error).toBeInstanceOf(AggregateError);
    expect((error as AggregateError).errors).toEqual([serializeError, cancelError]);
    await expect(f.port.close()).rejects.toBeInstanceOf(AggregateError);
    expect(cancellations).toBe(1);
  } finally {
    stringify.mockRestore();
  }
});

test('the wrapper postcheck joins the stream it cannot deliver', async () => {
  let checks = 0,
    cancellations = 0,
    finish!: () => void,
    started!: () => void;
  const gate = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const entered = new Promise<void>((resolve) => {
    started = resolve;
  });
  const f = fixture(
    sourceWithResponse(
      new Response(
        new ReadableStream({
          type: 'bytes',
          async cancel() {
            cancellations++;
            started();
            await gate;
          },
        }),
        { headers: { 'content-type': 'application/octet-stream', 'content-length': '3' } },
      ),
    ),
  );
  const checkError = new Error('postcheck failed');
  const context = {
    ...f.context,
    assertOpen() {
      if (++checks === 3) throw checkError;
    },
  };
  let settled = false;
  const result = f
    .call('source.open', { epoch, row: cleanupDescriptor }, context)
    .then(
      () => undefined,
      (error: unknown) => error,
    )
    .then((error) => {
      settled = true;
      return error;
    });
  try {
    const first = await withDeadline(
      Promise.race([entered.then(() => 'cancel'), result.then(() => 'settled')]),
    );
    expect(first).toBe('cancel');
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(settled).toBe(false);
  } finally {
    finish();
  }
  expect(await withDeadline(result)).toBe(checkError);
  expect(cancellations).toBe(1);
  await f.port.close();
}, 12000);
