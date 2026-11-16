// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Dos dispositivos con la misma nota. Cada guardado dice sobre qué versión se editó;
// si el servicio responde que otro guardó entretanto (409 note_edit_conflict), la
// pantalla no pisa lo del otro ni pierde lo de aquí: pregunta, y cada respuesta hace
// lo que dice. Si lo del otro no tocaba nada de lo que se guarda (la movió), no hay
// pregunta: se guarda sobre la versión nueva.
import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (typeof document === 'undefined') GlobalRegistrator.register();
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { Note, Notebook } from '@sytue/api-clients';
import { NotesApp } from './NotesApp.js';

const savedFetch = globalThis.fetch;
let host: HTMLElement | undefined;
const browser = window as unknown as { happyDOM: { setURL: (url: string) => void } };
const originalURL = window.location.href;
afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
    host = undefined;
  }
  globalThis.fetch = savedFetch;
  browser.happyDOM.setURL(originalURL);
});

async function until(check: () => boolean) {
  for (let i = 0; i < 300; i++) {
    if (check()) return;
    await new Promise((r) => setTimeout(r, 10));
  }
  throw new Error('UI did not reach expected state');
}

const cuaderno = {
  id: 'nb',
  owner_sub: 'owner',
  title: 'Home',
  color: '',
  layout: 'list',
  visibility: 'personal',
  indexable: false,
  encrypted: false,
  position: 0,
  note_count: 1,
  created_at: '2026-10-20T12:00:00Z',
  updated_at: '2026-10-20T12:00:00Z',
} as Notebook;

const V0 = '2026-10-20T12:00:00Z';
const V1 = '2026-10-20T12:05:00.123456Z';
const nota = {
  id: 'n',
  notebook_id: 'nb',
  owner_sub: 'owner',
  kind: 'text',
  title: 'Shopping list',
  content: '<p>milk</p>',
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
  created_at: V0,
  updated_at: V0,
} as Note;

type Llamada = { method: string; path: string; body?: Record<string, unknown> };

/**
 * El servidor: la nota la ha guardado ya otro dispositivo (`enServidor`). Un PUT con
 * otra versión esperada recibe el 409; con la del servidor, se guarda.
 */
function montar(enServidor: Note): { llamadas: Llamada[]; servidor: { nota: Note } } {
  const llamadas: Llamada[] = [];
  const servidor = { nota: enServidor };
  let primeraLectura = true;
  browser.happyDOM.setURL('https://notes.invalid/notes');
  globalThis.fetch = (async (input, init) => {
    const path = String(input);
    const method = init?.method ?? 'GET';
    const body =
      typeof init?.body === 'string'
        ? (JSON.parse(init.body) as Record<string, unknown>)
        : undefined;
    llamadas.push({ method, path, ...(body ? { body } : {}) });
    if (path.includes('/notebooks/nb/notes')) {
      // La primera lectura es la de este dispositivo, de antes de que guardara el otro.
      const lista = primeraLectura ? [nota] : [servidor.nota];
      primeraLectura = false;
      return Response.json({ notes: lista });
    }
    if (method === 'PUT' && path.endsWith('/api/notes/n')) {
      if (body?.expected_updated_at !== servidor.nota.updated_at)
        return Response.json({ error: 'note_edit_conflict' }, { status: 409 });
      servidor.nota = {
        ...servidor.nota,
        ...(body as Partial<Note>),
        updated_at: '2026-10-20T12:09:00Z',
      };
      return Response.json(servidor.nota);
    }
    if (method === 'POST' && path.endsWith('/api/notebooks/nb/notes'))
      return Response.json({ ...nota, id: 'copia', ...(body as Partial<Note>) });
    if (path.includes('/api/notebooks')) return Response.json({ notebooks: [cuaderno] });
    if (path.includes('/api/tags')) return Response.json({ tags: [] });
    if (path.includes('/api/reminders')) return Response.json({ reminders: [] });
    return Response.json({ notifications: [], unread: 0, accessToken: '', access_token: '' });
  }) as typeof fetch;
  host = document.createElement('div');
  document.body.append(host);
  render(
    <NotesApp
      user={
        { id: 'owner', tenant: 'test', email: 'o@example.invalid', name: 'O', roles: [] } as never
      }
      hubBaseUrl="https://hub.invalid"
      publicBase="https://notes.invalid/notes"
      locale="en"
      notebookInicial="nb"
    />,
    host,
  );
  return { llamadas, servidor };
}

const boton = (texto: string) =>
  [...document.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.textContent?.trim() === texto,
  );

/** Fijar la nota desde su menú: un guardado entero de la nota, como escribir en ella. */
async function fijar() {
  await until(() => Boolean(host?.textContent?.includes('Shopping list')));
  const mas = [
    ...document.querySelectorAll<HTMLButtonElement>('button[aria-label="More actions"]'),
  ].at(-1)!;
  mas.click();
  await until(() =>
    [...document.querySelectorAll('[role="menuitem"]')].some((e) => e.textContent === 'Pin to top'),
  );
  [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')]
    .find((e) => e.textContent === 'Pin to top')!
    .click();
}

const puts = (ll: Llamada[]) => ll.filter((l) => l.method === 'PUT' && l.path.endsWith('/notes/n'));
const dialogo = () => document.querySelector('[role="dialog"]');

test('el guardado lleva la versión sobre la que se editó', async () => {
  const { llamadas } = montar(nota);
  await fijar();
  await until(() => puts(llamadas).length === 1);
  expect(puts(llamadas)[0]!.body?.expected_updated_at).toBe(V0);
  expect(dialogo()).toBeNull();
});

test('si el otro solo la movió, se guarda sobre su versión sin preguntar', async () => {
  const { llamadas, servidor } = montar({ ...nota, x: 400, position: 3, updated_at: V1 });
  await fijar();
  await until(() => puts(llamadas).length === 2);
  expect(puts(llamadas).map((p) => p.body?.expected_updated_at)).toEqual([V0, V1]);
  await until(() => servidor.nota.pinned);
  expect(dialogo()).toBeNull();
});

test('si el otro cambió el texto, pregunta enseñando las dos y «Quedarme con la mía» guarda la de aquí', async () => {
  const { llamadas, servidor } = montar({ ...nota, content: '<p>bread</p>', updated_at: V1 });
  await fijar();
  await until(() =>
    Boolean(dialogo()?.textContent?.includes('This note changed on another device')),
  );
  expect(dialogo()!.textContent).toContain('milk');
  expect(dialogo()!.textContent).toContain('bread');
  // Mientras se decide no se ha pisado nada.
  expect(puts(llamadas)).toHaveLength(1);
  boton('Keep mine')!.click();
  await until(() => puts(llamadas).length === 2);
  expect(puts(llamadas)[1]!.body?.expected_updated_at).toBe(V1);
  expect(puts(llamadas)[1]!.body?.content).toBe('<p>milk</p>');
  await until(() => dialogo() === null);
  expect(servidor.nota.pinned).toBe(true);
});

test('«Ver la otra» deja la del otro en la nota y no vuelve a guardar la de aquí', async () => {
  const { llamadas } = montar({ ...nota, content: '<p>bread</p>', updated_at: V1 });
  await fijar();
  await until(() => Boolean(boton('See the other one')));
  boton('See the other one')!.click();
  await until(() => dialogo() === null);
  await until(() => Boolean(host?.textContent?.includes('bread')));
  await new Promise((r) => setTimeout(r, 50));
  expect(puts(llamadas)).toHaveLength(1);
});

test('«Guardar las dos» deja la del otro y crea una nota nueva con la de aquí', async () => {
  const { llamadas } = montar({ ...nota, content: '<p>bread</p>', updated_at: V1 });
  await fijar();
  await until(() => Boolean(boton('Save both')));
  boton('Save both')!.click();
  await until(() =>
    llamadas.some((l) => l.method === 'POST' && l.path.endsWith('/notebooks/nb/notes')),
  );
  const nueva = llamadas.find(
    (l) => l.method === 'POST' && l.path.endsWith('/notebooks/nb/notes'),
  )!;
  expect(nueva.body?.title).toBe('Shopping list (your version)');
  expect(nueva.body?.content).toBe('<p>milk</p>');
  await until(() => Boolean(host?.textContent?.includes('bread')));
  expect(puts(llamadas)).toHaveLength(1);
});
