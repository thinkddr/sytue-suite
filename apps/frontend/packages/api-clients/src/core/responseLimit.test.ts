import { expect, test } from 'bun:test';
import { HttpClient, HttpError, ResponseTooLargeError } from './http.js';

const encoder = new TextEncoder();

function streamed(chunks: Uint8Array[], headers: HeadersInit = {}, status = 200) {
  const state = { reads: 0, cancels: 0 };
  const body = new ReadableStream<Uint8Array>(
    {
      pull(controller) {
        const chunk = chunks[state.reads];
        if (chunk) {
          state.reads++;
          controller.enqueue(chunk);
        } else controller.close();
      },
      cancel() {
        state.cancels++;
      },
    },
    { highWaterMark: 0 },
  );
  return { state, body, response: new Response(body, { headers, status }) };
}

function client(response: Response) {
  return new HttpClient({ baseUrl: 'https://fixture.invalid', fetchImpl: async () => response });
}

for (const headers of [{}, { 'content-length': '1' }]) {
  test(`chunked overflow cancels before reading the remaining body (${JSON.stringify(headers)})`, async () => {
    const fixture = streamed(
      ['{"n":', '123', '}'].map((s) => encoder.encode(s)),
      headers,
    );
    await expect(client(fixture.response).json({ maxResponseBytes: 7 })).rejects.toBeInstanceOf(
      ResponseTooLargeError,
    );
    expect(fixture.state).toEqual({ reads: 2, cancels: 1 });
    expect(fixture.body.locked).toBe(false);
  });
}

test('oversize Content-Length cancels without consuming a chunk', async () => {
  const fixture = streamed([encoder.encode('{}')], { 'content-length': '999' });
  await expect(client(fixture.response).json({ maxResponseBytes: 2 })).rejects.toMatchObject({
    name: 'ResponseTooLargeError',
    maxBytes: 2,
  });
  expect(fixture.state).toEqual({ reads: 0, cancels: 1 });
});

test('compressed Content-Length cannot be compared with decoded response bytes', async () => {
  const fixture = streamed([encoder.encode('{}')], {
    'content-length': '999',
    'content-encoding': 'gzip',
  });
  expect(await client(fixture.response).json({ maxResponseBytes: 2 })).toEqual({});
  expect(fixture.state.cancels).toBe(0);
});

test('UTF8 split across chunks, a BOM, and the exact byte boundary decode correctly', async () => {
  const raw = encoder.encode('\uFEFF{"text":"😀á"}');
  const fixture = streamed([...raw].map((byte) => Uint8Array.of(byte)));
  expect(await client(fixture.response).json({ maxResponseBytes: raw.byteLength })).toEqual({
    text: '😀á',
  });
  expect(fixture.state.cancels).toBe(0);
  expect(fixture.body.locked).toBe(false);
  const short = streamed([...raw].map((byte) => Uint8Array.of(byte)));
  await expect(
    client(short.response).json({ maxResponseBytes: raw.byteLength - 1 }),
  ).rejects.toBeInstanceOf(ResponseTooLargeError);
  expect(short.state.cancels).toBe(1);
});

test('204 and null bodies preserve the JSON/text contract', async () => {
  expect(
    await client(new Response(null, { status: 204 })).json({ maxResponseBytes: 2 }),
  ).toBeUndefined();
  expect(await client(new Response(null)).text({ maxResponseBytes: 2 })).toBe('');
  await expect(client(new Response(null)).json({ maxResponseBytes: 2 })).rejects.toBeInstanceOf(
    SyntaxError,
  );
});

test('HTTP status and error code survive bounded parsing', async () => {
  const raw = '{"error":{"code":"note_changed"}}';
  const fixture = streamed([encoder.encode(raw)], {}, 409);
  let caught: unknown;
  try {
    await client(fixture.response).json({ maxResponseBytes: encoder.encode(raw).byteLength });
  } catch (error) {
    caught = error;
  }
  expect(caught).toBeInstanceOf(HttpError);
  expect(caught).toMatchObject({ status: 409, code: 'note_changed', body: raw });
});

test('an oversize HTTP error body is cancelled and cannot become a status-based fallback', async () => {
  const fixture = streamed([encoder.encode('unbounded error')], {}, 503);
  await expect(client(fixture.response).json({ maxResponseBytes: 2 })).rejects.toMatchObject({
    name: 'ResponseTooLargeError',
    maxBytes: 2,
  });
  expect(fixture.state.cancels).toBe(1);
});

test('AbortSignal reaches fetch and an aborted body retains the abort reason', async () => {
  const abort = new AbortController();
  let start: () => void = () => {};
  const started = new Promise<void>((resolve) => {
    start = resolve;
  });
  const body = new ReadableStream<Uint8Array>(
    {
      start(controller) {
        abort.signal.addEventListener('abort', () => controller.error(abort.signal.reason), {
          once: true,
        });
      },
      pull() {
        start();
      },
    },
    { highWaterMark: 0 },
  );
  const sdk = new HttpClient({
    baseUrl: 'https://fixture.invalid',
    fetchImpl: async (_url, init) => {
      expect(init?.signal).toBe(abort.signal);
      return new Response(body);
    },
  });
  const pending = sdk.json({ maxResponseBytes: 2, signal: abort.signal });
  await started;
  const reason = new DOMException('fixture cancelled', 'AbortError');
  abort.abort(reason);
  await expect(pending).rejects.toBe(reason);
  expect(body.locked).toBe(false);
});

test('a bounded HTTP error body preserves a custom aborted signal reason', async () => {
  const abort = new AbortController();
  const reason = 'fixture custom cancellation';
  abort.abort(reason);
  const body = new ReadableStream<Uint8Array>({
    pull(controller) {
      controller.error(reason);
    },
  });
  await expect(
    client(new Response(body, { status: 404 })).json({
      maxResponseBytes: 2,
      signal: abort.signal,
    }),
  ).rejects.toBe(reason);
  expect(body.locked).toBe(false);
});

test('unbounded HTTP errors retain their existing status contract when a body aborts', async () => {
  const body = new ReadableStream<Uint8Array>({
    pull(controller) {
      controller.error(new DOMException('fixture abort', 'AbortError'));
    },
  });
  await expect(client(new Response(body, { status: 404 })).json({})).rejects.toMatchObject({
    name: 'HttpError',
    status: 404,
  });
});

test('other domains keep unbounded native parsing unless they explicitly opt in', async () => {
  const raw = JSON.stringify({ text: 'ordinary response'.repeat(20) });
  const fixture = streamed([encoder.encode(raw)], { 'content-length': '99999999' });
  expect(await client(fixture.response).json({})).toEqual(JSON.parse(raw));
  expect(fixture.state.cancels).toBe(0);
});

for (const maxResponseBytes of [0, -1, Number.NaN, Number.POSITIVE_INFINITY]) {
  test(`invalid byte budget ${maxResponseBytes} fails before fetch`, async () => {
    let calls = 0;
    const sdk = new HttpClient({
      baseUrl: 'https://fixture.invalid',
      fetchImpl: async () => {
        calls++;
        return Response.json({});
      },
    });
    await expect(sdk.json({ maxResponseBytes })).rejects.toBeInstanceOf(RangeError);
    expect(calls).toBe(0);
  });
}
