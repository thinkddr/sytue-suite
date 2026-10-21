import { expect, test } from 'bun:test';
import { NotesClient, type NoteCipherMigration, type NoteInput } from './index.js';

const crypto = {
  v: 1 as const,
  kdf: 'PBKDF2-SHA256' as const,
  salt: 'public',
  iterations: 600000,
  check: 'encv1:public',
};

for (const action of ['create', 'update'] as const) {
  test(`${action}: old server cannot accept encrypted or incomplete writes through legacy endpoints`, async () => {
    const inputs: NoteInput[] = [
      { cipher_version: 2, expected_crypto: crypto, content_cipher: 'encv1:cipher' },
      { cipher_version: 2 },
      { expected_crypto: crypto },
      { content_cipher: 'encv1:legacy' },
      { cipher_version: 3 } as unknown as NoteInput,
      { expected_crypto: null } as unknown as NoteInput,
    ];
    let writes = 0;
    const requests: string[] = [];
    const client = new NotesClient({
      baseUrl: 'https://notes.invalid',
      fetchImpl: async (url, init) => {
        const path = new URL(String(url)).pathname;
        requests.push(path);
        expect(init?.method).toBe(action === 'create' ? 'POST' : 'PUT');
        if (path.endsWith('/encrypted') || path.endsWith('/encrypted-notes'))
          return new Response('Not found', { status: 404 });
        writes++;
        return Response.json({ id: 'silently-accepted-legacy' });
      },
    });
    for (const input of inputs) {
      const operation =
        action === 'create'
          ? client.createNote('nb/1', input)
          : client.updateNote('n/1', 'nb/1', input);
      await expect(operation).rejects.toMatchObject({ status: 404 });
    }
    expect(requests).toEqual(
      inputs.map(() =>
        action === 'create'
          ? '/api/v1/notebooks/nb%2F1/encrypted-notes'
          : '/api/v1/notes/n%2F1/encrypted',
      ),
    );
    expect(writes).toBe(0);
  });
}

test('new server receives complete encrypted writes and plaintext keeps its original endpoints', async () => {
  const requests: { path: string; body: unknown }[] = [];
  const client = new NotesClient({
    baseUrl: 'https://notes.invalid',
    fetchImpl: async (url, init) => {
      requests.push({ path: new URL(String(url)).pathname, body: JSON.parse(String(init?.body)) });
      return Response.json({ id: 'n' });
    },
  });
  const encrypted = {
    cipher_version: 2 as const,
    expected_crypto: crypto,
    content_cipher: 'encv1:cipher',
  };
  await client.createNote('nb', encrypted);
  await client.updateNote('n', 'nb', encrypted);
  await client.createNote('nb', { title: 'plain' });
  await client.updateNote('n', 'nb', { title: 'plain' });
  expect(requests).toEqual([
    { path: '/api/v1/notebooks/nb/encrypted-notes', body: encrypted },
    { path: '/api/v1/notes/n/encrypted', body: { ...encrypted, notebook_id: 'nb' } },
    { path: '/api/v1/notebooks/nb/notes', body: { title: 'plain' } },
    { path: '/api/v1/notes/n', body: { title: 'plain', notebook_id: 'nb' } },
  ]);
});

test('encrypted content migration sends complete CAS exactly once on conflict', async () => {
  const input: NoteCipherMigration = {
    cipher_version: 2,
    expected_updated_at: '2026-10-20T12:00:00.123456Z',
    expected_crypto: {
      v: 1,
      kdf: 'PBKDF2-SHA256',
      salt: 'public',
      iterations: 600000,
      check: 'encv1:public',
    },
    content_cipher: 'encv1:cipher',
    data: { client_id: 'opaque' },
  };
  let calls = 0;
  const client = new NotesClient({
    baseUrl: 'https://notes.invalid',
    auth: { getAccessToken: () => 'test-token' },
    fetchImpl: (async (url, init) => {
      calls++;
      expect(String(url)).toBe('https://notes.invalid/api/v1/notes/n%2F1/encrypted-content');
      expect(init?.method).toBe('POST');
      expect(JSON.parse(String(init?.body))).toEqual(input);
      return new Response(JSON.stringify({ error: { code: 'note_changed' } }), { status: 409 });
    }) as typeof fetch,
  });
  let caught: unknown;
  try {
    await client.migrateEncryptedContent('n/1', input);
  } catch (err) {
    caught = err;
  }
  expect((caught as { code?: string }).code).toBe('note_changed');
  expect(calls).toBe(1);
});

test('paginated notes retain exact raw JSON and restore visible order', async () => {
  let calls = 0;
  const raw = '{"future":9007199254740993123456789,"drawing":1.234567890123456789}';
  const client = new NotesClient({
    baseUrl: 'https://notes.invalid',
    auth: { getAccessToken: () => 'test-token' },
    fetchImpl: (async (url) => {
      const parsed = new URL(String(url));
      expect(parsed.pathname).toBe('/api/v1/notebooks/nb/notes/page');
      expect(parsed.searchParams.get('limit')).toBe('100');
      expect(parsed.searchParams.get('raw_data')).toBe('1');
      calls++;
      const body =
        calls === 1
          ? {
              notes: [
                {
                  id: 'a',
                  position: 9,
                  col: 0,
                  pinned: false,
                  created_at: '2026-10-20',
                  data_json: raw,
                },
              ],
              next_cursor: 'a',
            }
          : {
              notes: [
                {
                  id: 'b',
                  position: 1,
                  col: 0,
                  pinned: false,
                  created_at: '2026-10-20',
                  data_json: '{}',
                },
              ],
            };
      if (calls === 2) expect(parsed.searchParams.get('cursor')).toBe('a');
      return Response.json(body);
    }) as typeof fetch,
  });
  const rows = await client.listNotes('nb');
  expect(rows.map((n) => n.id)).toEqual(['b', 'a']);
  expect(rows[1]?.data_json).toBe(raw);
  expect(calls).toBe(2);
});
