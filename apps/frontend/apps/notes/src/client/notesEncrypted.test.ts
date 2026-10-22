import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (typeof document === 'undefined') GlobalRegistrator.register();
import { expect, test } from 'bun:test';
import type { Note, NoteCipherMigration } from '@sytue/api-clients';
import { nuevosParametros, empaquetar, desempaquetar, descifrarTexto } from './crypto.js';
import { cifrarContenidoNota, abrirContenidoNota, migrarNotasCifradas } from './notesEncrypted.js';

function nota(extra: Partial<Note> = {}): Note {
  return {
    id: 'n',
    owner_sub: 'owner',
    visibility: 'personal',
    indexable: false,
    notebook_id: 'nb',
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
    updated_at: '2026-10-20T12:00:00.123456Z',
    ...extra,
  };
}
test('create/edit encrypt every auxiliary field, restore attachments and leave only opaque controls', async () => {
  const { key } = await nuevosParametros('test password');
  const data = {
    attachments: [{ name: 'private.pdf', dataUrl: 'data:application/pdf;base64,U0VDUkVUTw==' }],
    url: 'https://private.invalid',
    icon: 'key',
    future: { keep: [1, 2] },
    client_id: 'opaque',
    carry_count: 4,
  };
  const body = await cifrarContenidoNota(
    key,
    { kind: 'text', data },
    'secret title',
    '<p>secret body</p>',
  );
  const wire = JSON.stringify(body);
  for (const text of [
    'private.pdf',
    'data:application',
    'private.invalid',
    'future',
    'secret title',
    'secret body',
  ])
    expect(wire).not.toContain(text);
  expect(body.data).toEqual({ client_id: 'opaque' });
  expect(body.cipher_version).toBe(2);
  const { claro, migrar } = await abrirContenidoNota(
    key,
    nota({ ...body, data: { client_id: 'opaque', carry_count: 5 } } as Partial<Note>),
  );
  expect(claro.data).toEqual({ ...data, carry_count: 5 });
  expect(migrar).toBe(false);
  const edit = await cifrarContenidoNota(key, { data: claro.data ?? {} }, claro.title, 'changed');
  expect((await desempaquetar(key, edit.content_cipher!))?.data).toEqual({
    attachments: data.attachments,
    url: data.url,
    icon: data.icon,
    future: data.future,
  });
});
test('legacy cipher+Data and encrypted toggles use full crypto+timestamp CAS; no plaintext leaves client', async () => {
  const { key, params } = await nuevosParametros('test password');
  const old = nota({
    content_cipher: await empaquetar(key, { title: 'title', content: 'body' }),
    data: { attachments: [{ dataUrl: 'data:legacy' }], unknown: 42, client_id: 'opaque' },
  });
  old.data_json = JSON.stringify(old.data);
  const toggle = nota({
    id: 'plain',
    title: 'historic',
    content: 'body',
    data: { unknown: 'old' },
    data_json: '{"unknown":"old"}',
  });
  const requests: NoteCipherMigration[] = [];
  const results: Note[] = [];
  await migrarNotasCifradas(key, params, {
    list: async () => [old, toggle],
    current: () => true,
    accept: (n) => results.push(n),
    save: async (id, input) => {
      requests.push(input);
      return nota({ id, content_cipher: input.content_cipher, data: input.data ?? {} });
    },
  });
  expect(requests).toHaveLength(2);
  expect(results).toHaveLength(2);
  expect(requests[0]?.expected_crypto).toEqual(params);
  expect(requests[0]?.expected_updated_at).toBe(old.updated_at);
  expect(JSON.stringify(requests)).not.toContain('data:legacy');
  expect(JSON.stringify(requests)).not.toContain('historic');
  expect((await desempaquetar(key, requests[0]!.content_cipher))?.data).toEqual({
    attachments: [{ dataUrl: 'data:legacy' }],
    unknown: 42,
    client_id: 'opaque',
  });
  expect((await desempaquetar(key, requests[1]!.content_cipher))?.title).toBe('historic');
});
test('uncertain write and conflict preserve original, stop batch and never replay', async () => {
  const { key, params } = await nuevosParametros('test password');
  for (const error of [new TypeError('network lost'), new Error('note_changed')]) {
    const original = nota({
      title: 'old',
      data: { private: 'keep' },
      data_json: '{"private":"keep"}',
    });
    let writes = 0;
    let accepted = 0;
    let caught: unknown;
    try {
      await migrarNotasCifradas(key, params, {
        list: async () => [original, nota({ id: 'next', title: 'next' })],
        current: () => true,
        accept: () => accepted++,
        save: async () => {
          writes++;
          throw error;
        },
      });
    } catch (err) {
      caught = err;
    }
    expect(caught).toBe(error);
    expect(writes).toBe(1);
    expect(accepted).toBe(0);
    expect(original.data).toEqual({ private: 'keep' });
    expect(original.title).toBe('old');
  }
});
test('identity/lock changes during list and upload prevent stale writes/results', async () => {
  const { key, params } = await nuevosParametros('test password');
  let current = true,
    writes = 0,
    accepted = 0;
  await migrarNotasCifradas(key, params, {
    list: async () => {
      current = false;
      return [nota({ title: 'old' })];
    },
    current: () => current,
    accept: () => accepted++,
    save: async () => {
      writes++;
      return nota();
    },
  });
  expect(writes).toBe(0);
  expect(accepted).toBe(0);
  current = true;
  await migrarNotasCifradas(key, params, {
    list: async () => [nota({ title: 'old' }), nota({ id: 'next', title: 'next' })],
    current: () => current,
    accept: () => accepted++,
    save: async () => {
      writes++;
      current = false;
      return nota();
    },
  });
  expect(writes).toBe(1);
  expect(accepted).toBe(0);
});
test('conflicting legacy/envelope data and non-object legacy are retained, never silently replaced', async () => {
  const { key, params } = await nuevosParametros('test password');
  const original = nota({
    content_cipher: await empaquetar(key, { title: 'x', content: 'y', data: { unknown: 1 } }),
    data: { unknown: 2 },
  });
  let writes = 0;
  let caught: unknown;
  try {
    await migrarNotasCifradas(key, params, {
      list: async () => [original],
      current: () => true,
      accept: () => {},
      save: async () => {
        writes++;
        return nota();
      },
    });
  } catch (err) {
    caught = err;
  }
  expect(caught).toBeInstanceOf(Error);
  expect(writes).toBe(0);
  expect(original.data).toEqual({ unknown: 2 });
});

test('raw migration, title edit and known Data edit preserve exact foreign numbers across reopening', async () => {
  const { key, params } = await nuevosParametros('test password');
  const raw =
    '{"future":9007199254740993123456789,"drawing":{"ratio":1.234567890123456789},"icon":"old","client_id":"opaque"}';
  const n = nota({
    title: 'historic',
    data: JSON.parse(raw) as Record<string, unknown>,
    data_json: raw,
  });
  let migrated: Note = nota();
  await migrarNotasCifradas(key, params, {
    list: async () => [n],
    current: () => true,
    accept: (value) => {
      migrated = value;
    },
    save: async (id, input) =>
      nota({ id, content_cipher: input.content_cipher, data: input.data ?? {} }),
  });
  const first = await abrirContenidoNota(key, migrated);
  expect(first.claro.data_json).toBe(raw);
  const title = await cifrarContenidoNota(
    key,
    { data: first.claro.data ?? {} },
    'edited title',
    first.claro.content,
    first.claro,
  );
  const opened = await desempaquetar(key, title.content_cipher!);
  expect(opened?.data_json).toBe(raw);
  expect(opened?.title).toBe('edited title');
  const changed = await cifrarContenidoNota(
    key,
    { data: { ...opened?.data, icon: 'new' } },
    'edited title',
    'body',
    opened!,
  );
  const envelope = JSON.parse(await descifrarTexto(key, changed.content_cipher!)) as {
    data_json: string;
    data_patch: Record<string, unknown>;
  };
  expect(envelope.data_json).toBe(raw);
  expect(envelope.data_patch).toEqual({ icon: 'new' });
  const final = await desempaquetar(key, changed.content_cipher!);
  expect(final?.data?.icon).toBe('new');
  const deleted = await cifrarContenidoNota(
    key,
    { data: { ...final?.data, icon: undefined } },
    'edited title',
    'body',
    final!,
  );
  const afterDelete = await desempaquetar(key, deleted.content_cipher!);
  expect(afterDelete?.data?.icon).toBeUndefined();
  expect(afterDelete?.data_json).toBe(raw);
  expect(final?.data_json).toContain('9007199254740993123456789');
  expect(final?.data_json).toContain('1.234567890123456789');
  let caught: unknown;
  try {
    await cifrarContenidoNota(
      key,
      { data: { ...final?.data, drawing: { ratio: 2, other: 'changed' } } },
      'x',
      'y',
      final!,
    );
  } catch (err) {
    caught = err;
  }
  expect(caught).toBeInstanceOf(Error);
  expect(final?.data_json).toBe(raw);
});

test('old server without raw legacy JSON cannot silently migrate rounded Data', async () => {
  const { key, params } = await nuevosParametros('test password');
  let writes = 0;
  let caught: unknown;
  try {
    await migrarNotasCifradas(key, params, {
      list: async () => [nota({ title: 'old', data: { unknown: 42 } })],
      current: () => true,
      accept: () => {},
      save: async () => {
        writes++;
        return nota();
      },
    });
  } catch (err) {
    caught = err;
  }
  expect(caught).toBeInstanceOf(Error);
  expect(writes).toBe(0);
});

test('NULL legacy column preserves the only private copy inside ciphertext and its precise JSON', async () => {
  const { key, params } = await nuevosParametros('test password');
  const raw = '{"future":9007199254740993123456789,"drawing":{"ratio":1.234567890123456789}}';
  const cipher = await empaquetar(key, { title: 'inside', content: 'body' }, raw);
  const original = nota({ content_cipher: cipher, data_json: 'null' });
  let migrated: Note = nota();
  await migrarNotasCifradas(key, params, {
    list: async () => [original],
    current: () => true,
    accept: (n) => {
      migrated = n;
    },
    save: async (id, input) =>
      nota({ id, content_cipher: input.content_cipher, data: input.data ?? {} }),
  });
  const clear = await abrirContenidoNota(key, migrated);
  expect(clear.claro.data_json).toBe(raw);
  expect(clear.migrar).toBe(false);
  const edit = await cifrarContenidoNota(
    key,
    { data: clear.claro.data ?? {} },
    'changed',
    'body',
    clear.claro,
  );
  expect((await desempaquetar(key, edit.content_cipher!))?.data_json).toBe(raw);
});

test('five MiB of auxiliary HTML fit encrypted without expansion; oversized writes retain the original', async () => {
  const { key } = await nuevosParametros('test password');
  const html = '<>&'.repeat(Math.floor((5 * 1024 * 1024 - 128) / 3));
  const body = await cifrarContenidoNota(key, { data: { html } }, 'title', 'body');
  expect(body.content_cipher!.length).toBeLessThan(16 * 1024 * 1024);
  expect((await desempaquetar(key, body.content_cipher!))?.data?.html).toBe(html);
  expect(body.data).toEqual({});
  const original = { data: { html: 'x'.repeat(16 * 1024 * 1024) } };
  let rejected: unknown;
  try {
    await cifrarContenidoNota(key, original, 'title', 'body');
  } catch (err) {
    rejected = err;
  }
  expect(rejected).toBeInstanceOf(Error);
  expect(original.data.html.length).toBe(16 * 1024 * 1024);
});
