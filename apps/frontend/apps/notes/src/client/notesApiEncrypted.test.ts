import { afterEach, expect, test } from 'bun:test';
import type { NoteInput } from '@sytue/api-clients';
import { createNote, updateNote } from './notesApi.js';

const savedFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = savedFetch;
});

for (const action of ['create', 'update'] as const) {
  test(`browser ${action} rejects an old BFF without retrying a legacy write`, async () => {
    const inputs: NoteInput[] = [
      { cipher_version: 2 },
      {
        expected_crypto: {
          v: 1,
          kdf: 'PBKDF2-SHA256',
          salt: 'public',
          iterations: 600000,
          check: 'encv1:check',
        },
      },
      { content_cipher: 'encv1:legacy' },
    ];
    const requests: string[] = [];
    let writes = 0;
    globalThis.fetch = (async (raw) => {
      const path = String(raw);
      requests.push(path);
      if (path.endsWith('/encrypted-notes') || path.endsWith('/encrypted'))
        return new Response('Old BFF does not know this route', { status: 404 });
      writes++;
      return Response.json({ id: 'unsafe-legacy-write' });
    }) as typeof fetch;
    for (const input of inputs) {
      const operation =
        action === 'create' ? createNote('nb/1', input) : updateNote('n/1', 'nb/1', input);
      let caught: unknown;
      try {
        await operation;
      } catch (error) {
        caught = error;
      }
      expect(caught).toMatchObject({ status: 404 });
    }
    expect(requests).toHaveLength(inputs.length);
    for (const path of requests)
      expect(path).toEndWith(
        action === 'create'
          ? '/api/notebooks/nb%2F1/encrypted-notes'
          : '/api/notes/n%2F1/encrypted',
      );
    expect(writes).toBe(0);
  });
}
