/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (typeof document === 'undefined') GlobalRegistrator.register();
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { Note, Notebook } from '@sytue/api-clients';
import { PublicNotebook } from './PublicNotebook.js';
import { empaquetar, nuevosParametros } from '../client/crypto.js';

const savedFetch = globalThis.fetch;
let host: HTMLElement | undefined;

afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
    host = undefined;
  }
  globalThis.fetch = savedFetch;
});

async function until(check: () => boolean): Promise<void> {
  for (let i = 0; i < 300; i++) {
    if (check()) return;
    await new Promise((r) => setTimeout(r, 10));
  }
  throw new Error('PublicNotebook did not reach expected state');
}

test('PublicNotebook unlocks and re-locks encrypted shared notes locally with HTML sanitization and zero network calls', async () => {
  let networkCalls = 0;
  globalThis.fetch = (async () => {
    networkCalls += 1;
    throw new Error('unexpected network request from PublicNotebook');
  }) as unknown as typeof fetch;

  const frase = 'frase-secreta-cuaderno';
  const { key, params } = await nuevosParametros(frase);
  const cipher = await empaquetar(key, {
    title: 'Hoja privada compartida',
    content: '<p>Texto seguro</p><img src="x" onerror="window.__xss=1"><script>evil()</script>',
    data: {},
  });

  const notebook = {
    id: 'nb-public-enc',
    owner_sub: '',
    title: 'Cuaderno publicado cifrado',
    color: '',
    layout: 'list',
    visibility: 'public',
    indexable: false,
    encrypted: true,
    crypto: params,
    position: 0,
    note_count: 1,
    created_at: '2026-11-01T10:00:00Z',
    updated_at: '2026-11-01T10:00:00Z',
  } as Notebook;

  const notes: Note[] = [
    {
      id: 'note-1',
      notebook_id: 'nb-public-enc',
      owner_sub: '',
      kind: 'text',
      title: '',
      content: '',
      content_cipher: cipher,
      data: {},
      x: 0,
      y: 0,
      col: 0,
      position: 0,
      width: 320,
      collapsed: false,
      checked: false,
      pinned: false,
      visibility: 'public',
      indexable: false,
      created_at: '2026-11-01T10:00:00Z',
      updated_at: '2026-11-01T10:00:00Z',
    },
  ];

  host = document.createElement('div');
  document.body.appendChild(host);
  render(<PublicNotebook locale="es" notebook={notebook} notes={notes} />, host);

  expect(host.textContent).toContain('Bloqueado');
  expect(host.textContent).not.toContain('Hoja privada compartida');

  const btnDesbloquear = Array.from(host.querySelectorAll('button')).find((b) =>
    b.textContent?.includes('Desbloquear cuaderno'),
  );
  expect(btnDesbloquear).toBeDefined();
  btnDesbloquear!.click();

  await until(() => document.querySelector('input[type="password"]') !== null);
  const input = document.querySelector('input[type="password"]') as HTMLInputElement;
  input.value = frase;
  input.dispatchEvent(new Event('input', { bubbles: true }));

  await until(() => {
    const submit = document.querySelector('button[type="submit"]') as HTMLButtonElement | null;
    return Boolean(submit && !submit.disabled);
  });
  const form = document.getElementById('cuaderno-clave') as HTMLFormElement;
  form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));

  await until(() => Boolean(host?.textContent?.includes('Hoja privada compartida')));
  expect(host.textContent).toContain('Texto seguro');
  expect(host.innerHTML).not.toContain('<script');
  expect(host.innerHTML).not.toContain('onerror');
  expect(networkCalls).toBe(0);

  const btnBloquear = Array.from(host.querySelectorAll('button')).find((b) =>
    b.textContent?.includes('Bloquear ahora'),
  );
  expect(btnBloquear).toBeDefined();
  btnBloquear!.click();

  await until(() => Boolean(host?.textContent?.includes('Bloqueado')));
  expect(host.textContent).not.toContain('Hoja privada compartida');
  expect(host.textContent).not.toContain('Texto seguro');
  expect(networkCalls).toBe(0);
});
