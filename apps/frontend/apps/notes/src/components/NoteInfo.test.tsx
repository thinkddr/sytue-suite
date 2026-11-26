// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import '../client/html-estatico.js';
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { formatDateTime } from '@sytue/i18n';
import type { Note } from '@sytue/api-clients';
import { NoteInfo } from './NoteInfo.js';
import { NoteCard } from './NoteCard.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const hosts: HTMLElement[] = [];
const fetchOriginal = globalThis.fetch;
const nada = () => {};
afterEach(() => {
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
  globalThis.fetch = fetchOriginal;
});

// La clave con sus variables, para ver QUÉ se pidió traducir y con qué número.
const t = ((clave: string, vars?: Record<string, unknown>) =>
  vars ? `${clave}${JSON.stringify(vars)}` : clave) as never;

function nota(o: Partial<Note> = {}): Note {
  return {
    id: 'n1',
    notebook_id: 'c1',
    owner_sub: 'yo',
    kind: 'text',
    title: 'Lista',
    content: '<ul><li>pan</li><li>leche</li></ul><p>y <b>café</b></p>',
    x: 0,
    y: 0,
    col: 0,
    position: 0,
    width: 320,
    collapsed: false,
    checked: false,
    pinned: false,
    visibility: 'personal',
    indexable: false,
    created_at: '2026-10-20T08:15:00Z',
    updated_at: '2026-11-02T17:40:00Z',
    ...o,
  };
}

function host(): HTMLElement {
  const h = document.createElement('div');
  hosts.push(h);
  document.body.appendChild(h);
  return h;
}

function retroenlaces(cuantas: number | 'falla'): string[] {
  const pedidas: string[] = [];
  globalThis.fetch = (async (url: string) => {
    pedidas.push(String(url));
    if (cuantas === 'falla') return new Response('{}', { status: 500 });
    const notes = Array.from({ length: cuantas }, (_, i) => nota({ id: `o${i}` }));
    return new Response(JSON.stringify({ notes }), { status: 200 });
  }) as typeof fetch;
  return pedidas;
}

function filas(): Record<string, string> {
  const out: Record<string, string> = {};
  for (const dt of document.querySelectorAll('dt')) {
    out[dt.textContent ?? ''] = dt.nextElementSibling?.textContent ?? '';
  }
  return out;
}

// Los efectos de Preact corren tras el pintado, y la respuesta llega después.
const esperar = () => new Promise((r) => setTimeout(r, 30));

test('info counts the plain text and shows dates, notebook and how many notes link here', async () => {
  const pedidas = retroenlaces(2);
  const n = nota();
  render(
    <NoteInfo
      t={t}
      locale="en"
      note={n}
      titulo={n.title ?? ''}
      contenido={n.content ?? ''}
      cuaderno="Casa"
      onClose={nada}
    />,
    host(),
  );
  expect(filas()['noteInfo.backlinks']).toBe('…');
  await esperar();
  await esperar();
  expect(pedidas[0]).toContain('/api/backlinks?note=n1');
  expect(filas()).toEqual({
    'noteInfo.words': '4',
    'noteInfo.characters': '16',
    'noteInfo.reading': 'noteInfo.readingValue{"count":1}',
    'noteInfo.created': formatDateTime('en', '2026-10-20T08:15:00Z'),
    'noteInfo.edited': formatDateTime('en', '2026-11-02T17:40:00Z'),
    'noteInfo.notebook': 'Casa',
    // Lo que sale de la nota (ninguno en esta), junto a lo que entra.
    'noteInfo.links': '0',
    'noteInfo.backlinks': 'notebook.notes{"count":2}',
  });
  expect(document.body.textContent).toContain('Lista');
});

test('a locked encrypted note says it cannot count rather than showing zero', async () => {
  retroenlaces('falla');
  const n = nota({ content: '', content_cipher: 'xyz' });
  render(
    <NoteInfo
      t={t}
      locale="en"
      note={n}
      titulo=""
      contenido={null}
      cuaderno="Secreto"
      onClose={nada}
    />,
    host(),
  );
  await esperar();
  await esperar();
  const f = filas();
  expect(f['noteInfo.words']).toBe('—');
  expect(f['noteInfo.characters']).toBe('—');
  expect(f['noteInfo.reading']).toBe('—');
  expect(f['noteInfo.backlinks']).toBe('—');
});

test('the «…» menu of a note offers «Info» only when the screen handles it', async () => {
  const pintar = (onInfo?: () => void) => {
    const h = host();
    render(
      <NoteCard
        t={t}
        note={nota()}
        seleccionada={false}
        onSelect={nada}
        onChange={nada}
        onDelete={nada}
        onDuplicate={nada}
        onTogglePin={nada}
        onShare={nada}
        onTags={nada}
        onBacklinks={nada}
        {...(onInfo ? { onInfo } : {})}
      />,
      h,
    );
    return h;
  };
  let abierta = 0;
  const h = pintar(() => abierta++);
  (h.querySelector('button[aria-label="common.more"]') as HTMLButtonElement).click();
  await esperar();
  const entrada = [...document.querySelectorAll('[role="menuitem"]')].find(
    (e) => e.textContent === 'note.info',
  ) as HTMLElement | undefined;
  expect(entrada).toBeDefined();
  entrada!.click();
  expect(abierta).toBe(1);
});
