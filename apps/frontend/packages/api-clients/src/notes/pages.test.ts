import { expect, test } from 'bun:test';
import { NotesClient, type Note } from './index.js';
import { ResponseTooLargeError } from '../core/http.js';

function note(id: string, fields: Partial<Note> = {}): Note {
  return {
    id,
    notebook_id: 'nb',
    owner_sub: 'fixture',
    kind: 'text',
    x: 0,
    y: 0,
    col: 0,
    position: 0,
    width: 320,
    collapsed: false,
    checked: false,
    pinned: false,
    created_at: '2026-10-20T12:00:00Z',
    updated_at: '2026-10-20T12:00:00Z',
    ...fields,
  };
}

function client(fetchImpl: typeof fetch): NotesClient {
  return new NotesClient({
    baseUrl: 'https://notes.invalid',
    auth: { getAccessToken: () => 'fixture-token' },
    fetchImpl,
  });
}

test('one thousand ordinary notes use ten bounded pages and retain exact foreign JSON', async () => {
  let calls = 0;
  const raw = '{"future":9007199254740993123456789,"ratio":1.234567890123456789}';
  const sdk = client(async (url, init) => {
    const parsed = new URL(String(url));
    expect(parsed.pathname).toBe('/api/v1/notebooks/nb%2F1/notes/page');
    expect(parsed.searchParams.get('limit')).toBe('100');
    expect(parsed.searchParams.get('raw_data')).toBe('1');
    expect(init?.method).toBe('GET');
    const from = Number(parsed.searchParams.get('cursor') ?? 0);
    calls++;
    const notes = Array.from({ length: 100 }, (_, i) =>
      note(String(from + i + 1), { position: 1000 - from - i, data_json: raw }),
    );
    return Response.json({
      notes,
      ...(from < 900 ? { next_cursor: String(from + 100) } : {}),
    });
  });
  const rows = await sdk.listNotes('nb/1');
  expect(calls).toBe(10);
  expect(rows).toHaveLength(1000);
  expect(rows[0]?.id).toBe('1000');
  expect(rows[999]?.id).toBe('1');
  expect(new Set(rows.map((n) => n.id)).size).toBe(1000);
  expect(rows.every((n) => n.data_json === raw)).toBe(true);
});

test('byte-limited pages may contain fewer than one hundred notes without ending the list', async () => {
  const pages = [
    { notes: [note('a', { position: 3 })], next_cursor: 'a' },
    {
      notes: [note('b', { pinned: true }), note('c', { position: 1 })],
      next_cursor: 'c',
    },
    { notes: [note('d', { position: 2 })] },
  ];
  let calls = 0;
  const sdk = client(async (url) => {
    const parsed = new URL(String(url));
    expect(parsed.searchParams.get('cursor')).toBe([null, 'a', 'c'][calls]);
    return Response.json(pages[calls++]);
  });
  expect((await sdk.listNotes('nb')).map((n) => n.id)).toEqual(['b', 'c', 'd', 'a']);
  expect(calls).toBe(3);
});

test('only the first missing capability request is paid before persistent legacy limit one', async () => {
  const requests: URL[] = [];
  const sdk = client(async (url) => {
    const parsed = new URL(String(url));
    requests.push(parsed);
    if (parsed.pathname.endsWith('/page')) return new Response('missing', { status: 404 });
    expect(parsed.searchParams.get('limit')).toBe('1');
    expect(parsed.searchParams.get('raw_data')).toBe('1');
    if (!parsed.searchParams.has('cursor'))
      return Response.json({ notes: [note('a')], next_cursor: 'a' });
    expect(parsed.searchParams.get('cursor')).toBe('a');
    return Response.json({ notes: [note('b')] });
  });
  expect((await sdk.listNotes('nb')).map((n) => n.id)).toEqual(['a', 'b']);
  expect(requests.map((r) => r.pathname)).toEqual([
    '/api/v1/notebooks/nb/notes/page',
    '/api/v1/notebooks/nb/notes',
    '/api/v1/notebooks/nb/notes',
  ]);
});

test('a backend rollback midway restarts instead of mixing modern and cursor-ignorant legacy lists', async () => {
  let calls = 0;
  const sdk = client(async (url) => {
    const parsed = new URL(String(url));
    calls++;
    if (calls === 1)
      return Response.json({
        notes: [note('a', { data_json: '{"snapshot":"modern"}' })],
        next_cursor: 'a',
      });
    if (calls === 2) {
      expect(parsed.pathname).toEndWith('/notes/page');
      expect(parsed.searchParams.get('cursor')).toBe('a');
      return new Response('capability removed', { status: 404 });
    }
    expect(parsed.pathname).toEndWith('/notes');
    expect(parsed.searchParams.get('limit')).toBe('1');
    expect(parsed.searchParams.has('cursor')).toBe(false);
    return Response.json({
      notes: [note('a', { data_json: '{"snapshot":"legacy"}' }), note('b')],
    });
  });
  const rows = await sdk.listNotes('nb');
  expect(rows.map((n) => n.id)).toEqual(['a', 'b']);
  expect(rows[0]?.data_json).toBe('{"snapshot":"legacy"}');
  expect(calls).toBe(3);
});

for (const status of [400, 401, 403, 409, 413, 429, 500, 503]) {
  test(`HTTP ${status} aborts the list without a legacy fallback or partial success`, async () => {
    let calls = 0;
    const sdk = client(async (url) => {
      expect(new URL(String(url)).pathname).toEndWith('/notes/page');
      calls++;
      if (calls === 1) return Response.json({ notes: [note('a')], next_cursor: 'a' });
      return Response.json({ error: { code: 'fixture_error' } }, { status });
    });
    await expect(sdk.listNotes('nb')).rejects.toMatchObject({ status });
    expect(calls).toBe(2);
  });
}

for (const failure of ['network', 'json'] as const) {
  test(`${failure} failure cannot be mistaken for a missing capability`, async () => {
    let calls = 0;
    const sdk = client(async (url) => {
      expect(new URL(String(url)).pathname).toEndWith('/notes/page');
      calls++;
      if (calls === 1) return Response.json({ notes: [note('a')], next_cursor: 'a' });
      if (failure === 'network') throw new TypeError('fixture offline');
      return new Response('not JSON');
    });
    await expect(sdk.listNotes('nb')).rejects.toBeInstanceOf(
      failure === 'network' ? TypeError : SyntaxError,
    );
    expect(calls).toBe(2);
  });
}

test('legacy 404 does not restart again', async () => {
  let calls = 0;
  const sdk = client(async () => {
    calls++;
    return new Response('missing', { status: 404 });
  });
  await expect(sdk.listNotes('nb')).rejects.toMatchObject({ status: 404 });
  expect(calls).toBe(2);
});

for (const invalid of ['cycle', 'empty'] as const) {
  test(`${invalid} cursor pages stop without another request`, async () => {
    let calls = 0;
    const sdk = client(async () => {
      calls++;
      return Response.json({
        notes: invalid === 'empty' ? [] : [note(String(calls))],
        next_cursor: 'same',
      });
    });
    await expect(sdk.listNotes('nb')).rejects.toThrow('notes: invalid cursor');
    expect(calls).toBe(invalid === 'empty' ? 1 : 2);
  });
}

test('legacy fallback cannot materialize an unbounded response from a pre-pagination backend', async () => {
  let calls = 0;
  let cancelled = false;
  let reads = 0;
  const sdk = client(async () => {
    calls++;
    if (calls === 1) return new Response('missing capability', { status: 404 });
    return new Response(
      new ReadableStream<Uint8Array>(
        {
          pull(controller) {
            reads++;
            controller.enqueue(new TextEncoder().encode('{"notes":[]}'));
            controller.close();
          },
          cancel() {
            cancelled = true;
          },
        },
        { highWaterMark: 0 },
      ),
      { headers: { 'content-length': String((32 << 20) + 1) } },
    );
  });
  await expect(sdk.listNotes('nb')).rejects.toBeInstanceOf(ResponseTooLargeError);
  expect(calls).toBe(2);
  expect(reads).toBe(0);
  expect(cancelled).toBe(true);
});

test('an oversized 404 error response never authorizes a legacy fallback', async () => {
  let calls = 0;
  let cancelled = false;
  const sdk = client(async () => {
    calls++;
    return new Response(
      new ReadableStream<Uint8Array>({
        cancel() {
          cancelled = true;
        },
      }),
      { status: 404, headers: { 'content-length': String((32 << 20) + 1) } },
    );
  });
  await expect(sdk.listNotes('nb')).rejects.toBeInstanceOf(ResponseTooLargeError);
  expect(calls).toBe(1);
  expect(cancelled).toBe(true);
});

test('an aborted 404 body preserves its original error and never requests legacy notes', async () => {
  let calls = 0;
  const reason = new DOMException('fixture aborted error body', 'AbortError');
  const body = new ReadableStream<Uint8Array>({
    pull(controller) {
      controller.error(reason);
    },
  });
  const sdk = client(async () => {
    calls++;
    return new Response(body, { status: 404 });
  });
  await expect(sdk.listNotes('nb')).rejects.toBe(reason);
  expect(calls).toBe(1);
  expect(body.locked).toBe(false);
});
