import { afterEach, expect, test } from 'bun:test';
import type { NoteInput } from '@sytue/api-clients';
import { admiteEnlacesEnEnvio, createNote, updateNote } from './notesApi.js';

const savedFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = savedFetch;
});

for (const action of ['create', 'update'] as const) {
  test(`browser ${action} refuses aliases and malformed links before fetch`, async () => {
    let calls = 0;
    globalThis.fetch = (async () => {
      calls++;
      return Response.json({});
    }) as unknown as typeof fetch;
    const inputs: unknown[] = [
      {
        cipher_version: 2,
        content_cipher: 'encv1:x',
        Links: [{ target_kind: 'note', target_id: 'secret' }],
      },
      {
        cipher_version: 2,
        content_cipher: 'encv1:x',
        links: [],
        Links: [{ target_kind: 'note', target_id: 'secret' }],
      },
      { Cipher_Version: 2, content_cipher: 'encv1:x' },
      { cipher_version: 2, content_cipher: 'encv1:x', linKs: [{ target_id: 'secret' }] },
      { cipher_version: 2, content_cipher: 'encv1:x', linkſ: [{ target_id: 'secret' }] },
      { cipher_version: 2, content_cipher: 'encv1:x', links: { label: 'secret' } },
      { cipher_version: 2, content_cipher: 'encv1:x', data: { attachment: 'secret' } },
      { cipher_version: 2, content_cipher: 'encv1:x', expected_crypto: { secret: 'private' } },
      { cipher_version: 2, content_cipher: 'encv1:x', title: 'secret' },
      { cipher_version: 2, content_cipher: 'encv1:x', unknown: 'secret' },
      { cipher_version: '2', content_cipher: 'encv1:x' },
      { cipher_version: 2, content_cipher: 'encv1:x', checked: 'yes' },
      { content: '\ud800' },
      { data: { attachment: { title: '\ud800' } } },
      { data: { attachment: { id: Number.MAX_SAFE_INTEGER + 1 } } },
      { cipher_version: 2, content_cipher: 'encv1:x', data: { client_id: '\udc00' } },
      null,
    ];
    for (const input of inputs) {
      const operation =
        action === 'create'
          ? createNote('nb', input as NoteInput)
          : updateNote('n', 'nb', input as NoteInput);
      await expect(operation).rejects.toThrow();
    }
    expect(calls).toBe(0);
  });
  test(`browser ${action} rejects an encrypted legacy request with clear links before fetch`, async () => {
    let calls = 0;
    globalThis.fetch = (async () => {
      calls++;
      return Response.json({});
    }) as unknown as typeof fetch;
    const input: NoteInput = {
      content_cipher: 'encv1:legacy',
      cipher_version: 2,
      links: [{ target_kind: 'docs_document', target_id: 'private', label: 'secret' }],
    };
    const operation = action === 'create' ? createNote('nb', input) : updateNote('n', 'nb', input);
    await expect(operation).rejects.toThrow();
    expect(calls).toBe(0);
  });
  test(`browser ${action} validates the exact serialized links before fetch`, async () => {
    let calls = 0;
    globalThis.fetch = (async () => {
      calls++;
      return Response.json({});
    }) as unknown as typeof fetch;
    for (const inherited of [false, true]) {
      const links: NonNullable<NoteInput['links']> = [];
      const hook = () => [{ target_kind: 'note', target_id: 'private', label: 'secret' }];
      if (inherited) {
        const proto = Object.create(Array.prototype) as object;
        Object.defineProperty(proto, 'toJSON', { value: hook });
        Object.setPrototypeOf(links, proto);
      } else {
        Object.defineProperty(links, 'toJSON', { value: hook });
      }
      const input: NoteInput = { cipher_version: 2, content_cipher: 'encv1:x', links };
      const operation =
        action === 'create' ? createNote('nb', input) : updateNote('n', 'nb', input);
      await expect(operation).rejects.toThrow();
    }
    expect(calls).toBe(0);
  });
  test(`browser ${action} refuses serialization failure and encryption downgrade`, async () => {
    let calls = 0;
    globalThis.fetch = (async () => {
      calls++;
      return Response.json({});
    }) as unknown as typeof fetch;
    const failed: NonNullable<NoteInput['links']> = [];
    Object.defineProperty(failed, 'toJSON', {
      value: () => {
        throw new Error('failed');
      },
    });
    const input: NoteInput = { links: failed, cipher_version: 2, content_cipher: 'encv1:x' };
    await expect(
      action === 'create' ? createNote('nb', input) : updateNote('n', 'nb', input),
    ).rejects.toThrow();
    const links: NonNullable<NoteInput['links']> = [];
    const downgrade: NoteInput = { links, cipher_version: 2, content_cipher: 'encv1:x' };
    Object.defineProperty(links, 'toJSON', {
      value: () => {
        delete downgrade.cipher_version;
        delete downgrade.content_cipher;
        return [{ target_kind: 'note', target_id: 'private' }];
      },
    });
    await expect(
      action === 'create' ? createNote('nb', downgrade) : updateNote('n', 'nb', downgrade),
    ).rejects.toThrow();
    expect(calls).toBe(0);
  });
  test(`browser ${action} sends the validated serialized snapshot only once`, async () => {
    let serializations = 0;
    let sent = '';
    let path = '';
    globalThis.fetch = (async (request: RequestInfo | URL, init?: RequestInit) => {
      path = String(request);
      sent = String(init?.body);
      return Response.json({});
    }) as typeof fetch;
    const links: NonNullable<NoteInput['links']> = [];
    Object.defineProperty(links, 'toJSON', {
      value: () => {
        serializations++;
        return [];
      },
    });
    const input: NoteInput = { cipher_version: 2, content_cipher: 'encv1:x', links };
    await (action === 'create' ? createNote('nb', input) : updateNote('n', 'nb', input));
    expect(serializations).toBe(1);
    expect(JSON.parse(sent)).toEqual({
      cipher_version: 2,
      content_cipher: 'encv1:x',
      links: [],
      ...(action === 'update' ? { notebook_id: 'nb' } : {}),
    });
    expect(path).toEndWith(
      action === 'create' ? '/api/notebooks/nb/encrypted-notes' : '/api/notes/n/encrypted',
    );
  });
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
    globalThis.fetch = (async (raw: RequestInfo | URL) => {
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

test('valid Unicode pairs remain admissible', () => {
  expect(admiteEnlacesEnEnvio({ title: 'nota 😀' })).toBe(true);
});

test('create rejects a changed serialization after observing encrypted input', async () => {
  let calls = 0;
  globalThis.fetch = (async () => {
    calls++;
    return Response.json({});
  }) as unknown as typeof fetch;
  const source: NoteInput = { cipher_version: 2, content_cipher: 'encv1:x' };
  const seen = new Set<PropertyKey>();
  const input = new Proxy(source, {
    getOwnPropertyDescriptor(target, key) {
      const descriptor = Reflect.getOwnPropertyDescriptor(target, key);
      if (descriptor && !seen.has(key)) {
        seen.add(key);
        return { ...descriptor, value: undefined };
      }
      return descriptor;
    },
    get(target, key, receiver) {
      if (key === 'toJSON')
        return () => ({ title: 'secret', links: [{ target_kind: 'note', target_id: 'private' }] });
      return Reflect.get(target, key, receiver);
    },
  });
  await expect(createNote('nb', input)).rejects.toThrow();
  expect(calls).toBe(0);
});
