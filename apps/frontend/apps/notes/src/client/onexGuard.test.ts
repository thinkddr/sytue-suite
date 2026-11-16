// SPDX-License-Identifier: Apache-2.0
import { expect, test } from 'bun:test';
import {
  formatoImportable,
  validateImportBeforeUpload,
  validateNotebookBackupBeforeUpload,
} from './onexGuard.js';

const book = {
  title: 'PRIVATE_TITLE',
  owner_sub: 'PRIVATE_OWNER',
  encrypted: true,
  crypto: {
    v: 1,
    kdf: 'PBKDF2-SHA256',
    salt: 'abcdefghijklmnop',
    iterations: 600000,
    check: 'encv1:test',
  },
};
const clean = { kind: 'text', content_cipher: 'encv1:AAAA', data: { client_id: 'capture' } };
const archive = (notebook: unknown, notas: unknown) =>
  JSON.stringify({ formato: 'sytue.notepads', version: 1, notebook, notas });
const bytes = (text: string) => new TextEncoder().encode(text) as Uint8Array<ArrayBuffer>;

test('encrypted archives stay local and ordinary clear archives remain eligible', () => {
  expect(() => validateNotebookBackupBeforeUpload(archive(book, [clean]))).toThrow();
  expect(() =>
    validateNotebookBackupBeforeUpload(
      archive({ title: 'plain' }, [{ title: 'x', links: [{ label: 'plain' }] }]),
    ),
  ).not.toThrow();
});

test('rejects encrypted legacy content and structure before upload', () => {
  for (const patch of [
    { title: 'private' },
    { content: '<p>private</p>' },
    { links: [{ target_kind: 'note', target_id: 'x', label: 'private' }] },
    { data: { attachments: ['private'] } },
    { data: { repeated_into: 'old' } },
    { data_json: '{"secret":true}' },
    { states: [{ name: 'private' }] },
    { group_id: 'old' },
    { repeated_into: 'old' },
    { content_cipher: '' },
  ]) {
    expect(() =>
      validateNotebookBackupBeforeUpload(archive(book, [{ ...clean, ...patch }])),
    ).toThrow();
  }
  expect(() =>
    validateNotebookBackupBeforeUpload(archive({ ...book, parent_id: 'old' }, [clean])),
  ).toThrow();
  expect(() => validateNotebookBackupBeforeUpload(archive({ title: 'plain' }, [clean]))).toThrow();
});

test('rejects duplicate, escaped and case-folded aliases rather than disagreeing with Go', () => {
  const cases = [
    '{"notebook":{"encrypted":false},"notebook":{"encrypted":true},"notas":[]}',
    '{"notebook":{"encrypted":false,"Encrypted":true},"notas":[]}',
    '{"notebook":{"encrypted":false},"notas":[{"content_cipher":"","Content_Cipher":"encv1:AA"}]}',
    '{"notebook":{"encrypted":true},"notaſ":[{"content_cipher":"encv1:AA","links":[{"label":"secret"}]}]}',

    '{"notebook":{"encrypted":true},"notas":[{"content_cipher":"encv1:AA","links":[],"l\\u0069nks":[{"label":"secret"}]}]}',
  ];
  for (const value of cases) expect(() => validateNotebookBackupBeforeUpload(value)).toThrow();
});

test('real browser preview and import stop unsafe archives before fetch', async () => {
  const { previsualizarVolcado, importarVolcado } = await import('./notesApi.js');
  const saved = globalThis.fetch;
  let calls = 0;
  globalThis.fetch = (async () => {
    calls++;
    return Response.json({ notebook: { title: 'book' }, notas: [], importadas: 0, descartadas: 0 });
  }) as unknown as typeof fetch;
  try {
    const unsafe = archive(book, [{ ...clean, links: [{ label: 'private', target_id: 'x' }] }]);
    await expect(previsualizarVolcado(bytes(unsafe), 'copia.onex')).rejects.toThrow();
    await expect(importarVolcado(bytes(unsafe), 'copia.onex')).rejects.toThrow();
    expect(calls).toBe(0);
    await expect(previsualizarVolcado(bytes(archive(book, [clean])))).rejects.toThrow();
    expect(calls).toBe(0);
    await previsualizarVolcado(bytes(archive({ title: 'plain' }, [{ title: 'ordinary' }])));
    expect(calls).toBe(1);
  } finally {
    globalThis.fetch = saved;
  }
});

test('unknown fields cannot hide data in an encrypted transfer', () => {
  const cases = [
    JSON.stringify({
      formato: 'sytue.notepads',
      version: 1,
      notebook: book,
      notas: [clean],
      extra: 'secret',
    }),
    archive({ ...book, extra: 'secret' }, [clean]),
    archive({ ...book, crypto: { ...book.crypto, extra: 'secret' } }, [clean]),
    archive(book, [{ ...clean, extra: 'secret' }]),
    archive(book, [{ ...clean, data: { __proto__: null, extra: 'secret' } }]),
  ];
  for (const value of cases) expect(() => validateNotebookBackupBeforeUpload(value)).toThrow();
});

// Markdown, ZIP and Evernote imports carry no ciphertext and travel as bytes; any
// other file is read as a `.onex` and keeps the encrypted-backup guard, whatever
// its extension says.
test('the import format is recognised as the service does', () => {
  const encrypted = bytes(archive(book, [clean]));
  expect(formatoImportable(new Uint8Array([0x50, 0x4b, 0x03, 0x04, 0xff]), 'b.zip')).toBe('zip');
  expect(formatoImportable(bytes('<?xml version="1.0"?><en-export>'), 'r.enex')).toBe('enex');
  expect(formatoImportable(bytes('# Idea'), 'C:\\notas\\Idea.MD')).toBe('markdown');
  expect(formatoImportable(new Uint8Array([0xff, 0xfe]), 'roto.md')).toBe('onex');
  expect(formatoImportable(encrypted, 'copia.onex')).toBe('onex');
  expect(() => validateImportBeforeUpload(encrypted, 'copia.onex')).toThrow();
  expect(() => validateImportBeforeUpload(encrypted, 'copia.json')).toThrow();
  expect(() => validateImportBeforeUpload(bytes('# Idea'), 'Idea.md')).not.toThrow();
  expect(() => validateImportBeforeUpload(new Uint8Array(32 * 1024 * 1024 + 1), 'x.md')).toThrow();
});
