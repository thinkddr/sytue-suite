// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Flujos sobre la pantalla de verdad, con un servidor falso: lo que importa es
// qué viaja y qué se ve. Eliminar o mover deja un aviso con «Deshacer»; la
// búsqueda dice si está buscando, cuántos hay o que ha fallado, y abre el
// resultado sin recargar. «Mover cuaderno a…» cambia el padre o lo saca a la raíz.
import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (typeof document === 'undefined') GlobalRegistrator.register();
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { SytueUser } from '@sytue/auth';
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
  try {
    localStorage.removeItem('sytue.notes.ocultarTerminadas');
  } catch {
    // sin almacenamiento no hay nada que limpiar
  }
});

async function until(check: () => boolean) {
  for (let i = 0; i < 300; i++) {
    if (check()) return;
    await new Promise((r) => setTimeout(r, 10));
  }
  throw new Error('UI did not reach expected state');
}

const nb = (id: string, title: string): Notebook =>
  ({
    id,
    owner_sub: 'owner',
    title,
    color: '',
    layout: 'list',
    visibility: 'personal',
    indexable: false,
    encrypted: false,
    position: id === 'nb' ? 0 : 1,
    note_count: 1,
    created_at: '2026-10-20T12:00:00Z',
    updated_at: '2026-10-20T12:00:00Z',
  }) as Notebook;

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
  created_at: '2026-10-20T12:00:00Z',
  updated_at: '2026-10-20T12:00:00Z',
} as Note;

type Propia = (method: string, path: string) => Promise<Response> | Response | undefined;
type Llamada = { method: string; path: string; body?: unknown };

function montar(propia?: Propia): Llamada[] {
  const llamadas: Llamada[] = [];
  browser.happyDOM.setURL('https://notes.invalid/notes');
  globalThis.fetch = (async (input, init) => {
    const path = String(input);
    const method = init?.method ?? 'GET';
    llamadas.push({
      method,
      path,
      ...(typeof init?.body === 'string' ? { body: JSON.parse(init.body) as unknown } : {}),
    });
    const r = propia?.(method, path);
    if (r) return r;
    if (path.includes('/notebooks/nb/notes')) return Response.json({ notes: [nota] });
    if (path.includes('/api/notebooks'))
      return Response.json({ notebooks: [nb('nb', 'Home'), nb('nb2', 'Work')] });
    if (path.includes('/api/tags')) return Response.json({ tags: [] });
    if (path.includes('/api/reminders')) return Response.json({ reminders: [] });
    if (method === 'DELETE' || method === 'POST') return new Response(null, { status: 204 });
    return Response.json({ notifications: [], unread: 0, accessToken: '', access_token: '' });
  }) as typeof fetch;
  host = document.createElement('div');
  document.body.append(host);
  render(
    <NotesApp
      user={
        {
          id: 'owner',
          tenant: 'test',
          email: 'o@example.invalid',
          name: 'O',
          roles: [],
        } as unknown as SytueUser
      }
      hubBaseUrl="https://hub.invalid"
      publicBase="https://notes.invalid/notes"
      locale="en"
      notebookInicial="nb"
    />,
    host,
  );
  return llamadas;
}

const boton = (texto: string) =>
  [...document.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.textContent?.trim() === texto,
  );

async function elegirDelMenu(item: string) {
  await until(() => Boolean(host?.textContent?.includes('Shopping list')));
  // El «Más acciones» de la NOTA, no el de la página: el que vive junto a su título.
  const mas = [
    ...document.querySelectorAll<HTMLButtonElement>('button[aria-label="More actions"]'),
  ].at(-1);
  expect(mas).toBeDefined();
  mas!.click();
  await until(() =>
    [...document.querySelectorAll('[role="menuitem"]')].some((e) => e.textContent === item),
  );
  [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')]
    .find((e) => e.textContent === item)!
    .click();
}

test('eliminar una nota avisa «enviada a la papelera» y «Deshacer» la restaura', async () => {
  const llamadas = montar();
  await elegirDelMenu('Delete note');
  await until(() => Boolean(document.body.textContent?.includes('Note moved to the trash')));
  expect(llamadas.some((l) => l.method === 'DELETE' && l.path.endsWith('/api/notes/n'))).toBe(true);
  expect(host!.textContent).not.toContain('Shopping list');
  const deshacer = boton('Undo');
  expect(deshacer).toBeDefined();
  deshacer!.click();
  await until(() =>
    llamadas.some((l) => l.method === 'POST' && l.path.endsWith('/api/notes/n/restore')),
  );
  await until(() => Boolean(host?.textContent?.includes('Shopping list')));
});

// Visto en rojo leyendo `historial.current` en el clic (el aviso no hacía nada) y
// quitando la comprobación de `activeId` (la nota salía pintada en «Work»).
test('«Deshacer» del aviso tras cambiar de cuaderno la restaura sin pintarla en el otro', async () => {
  const llamadas = montar((_method, path) =>
    path.includes('/notebooks/nb2/notes')
      ? Response.json({ notes: [{ ...nota, id: 'w', notebook_id: 'nb2', title: 'Budget' }] })
      : undefined,
  );
  await elegirDelMenu('Delete note');
  await until(() => Boolean(document.body.textContent?.includes('Note moved to the trash')));
  document.querySelector<HTMLElement>('[data-notebook-id="nb2"]')!.click();
  await until(() => Boolean(host?.textContent?.includes('Budget')));
  boton('Undo')!.click();
  await until(() =>
    llamadas.some((l) => l.method === 'POST' && l.path.endsWith('/api/notes/n/restore')),
  );
  await new Promise((r) => setTimeout(r, 50));
  expect(host!.textContent).not.toContain('Shopping list');
});

// El paso se registra al pulsar y el aviso sale cuando contesta el DELETE: si
// entre medias se cambia de cuaderno, el aviso tiene que seguir atado al
// historial donde se registró. Visto en rojo leyendo `historial.current` al
// avisar: «Deshacer» no hacía nada.
test('«Deshacer» funciona aunque se cambie de cuaderno antes de que conteste el borrado', async () => {
  let soltar: () => void = () => undefined;
  const borrado = new Promise<Response>((r) => {
    soltar = () => r(new Response(null, { status: 204 }));
  });
  const llamadas = montar((method, path) => {
    if (method === 'DELETE' && path.endsWith('/api/notes/n')) return borrado;
    return path.includes('/notebooks/nb2/notes')
      ? Response.json({ notes: [{ ...nota, id: 'w', notebook_id: 'nb2', title: 'Budget' }] })
      : undefined;
  });
  await elegirDelMenu('Delete note');
  document.querySelector<HTMLElement>('[data-notebook-id="nb2"]')!.click();
  await until(() => Boolean(host?.textContent?.includes('Budget')));
  soltar();
  await until(() => Boolean(document.body.textContent?.includes('Note moved to the trash')));
  boton('Undo')!.click();
  await until(() =>
    llamadas.some((l) => l.method === 'POST' && l.path.endsWith('/api/notes/n/restore')),
  );
});

test('mover una nota de cuaderno avisa con «Deshacer», que la devuelve al de origen', async () => {
  const llamadas = montar();
  await elegirDelMenu('Move to another notebook…');
  await until(() => Boolean(document.querySelector('[role="dialog"]')));
  const destino = [...document.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')].find(
    (b) => b.textContent?.includes('Work'),
  );
  destino!.click();
  await until(() => Boolean(document.body.textContent?.includes('Moved to «Work»')));
  boton('Undo')!.click();
  await until(
    () => llamadas.filter((l) => l.method === 'POST' && l.path.endsWith('/move')).length === 2,
  );
  await until(() => Boolean(host?.textContent?.includes('Shopping list')));
});

const hit = {
  id: 'n',
  notebook_id: 'nb2',
  notebook: 'Work',
  title: 'Quarterly plan',
  snippet: 'numbers',
  kind: 'text',
};

async function abrirBusqueda() {
  await until(() => Boolean(host?.textContent?.includes('Shopping list')));
  const entrada = [...document.querySelectorAll<HTMLElement>('button, a, [role="button"]')].find(
    (e) => e.textContent?.trim() === 'Search notes',
  );
  entrada!.click();
}

test('la búsqueda dice «buscando…», cuenta los resultados y abre uno sin recargar', async () => {
  let soltar: () => void = () => {};
  const pendiente = new Promise<void>((r) => {
    soltar = r;
  });
  montar((_method, path) => {
    if (path.includes('/api/search')) return pendiente.then(() => Response.json({ notes: [hit] }));
    if (path.includes('/notebooks/nb2/notes'))
      return Response.json({ notes: [{ ...nota, notebook_id: 'nb2', title: 'Quarterly plan' }] });
    return undefined;
  });
  await abrirBusqueda();
  await until(() => Boolean(host?.textContent?.includes('Searching…')));
  const estado = [...host!.querySelectorAll('[role="status"]')].find((e) =>
    e.textContent?.includes('Searching…'),
  );
  expect(estado?.getAttribute('aria-live')).toBe('polite');
  // Mientras busca no dice «no se ha encontrado nada».
  expect(host!.textContent).not.toContain('Nothing found');
  soltar();
  await until(() => Boolean(host?.textContent?.includes('1 result')));
  const fila = [...host!.querySelectorAll<HTMLAnchorElement>('a.notes-row')].find((a) =>
    a.textContent?.includes('Quarterly plan'),
  );
  expect(fila).toBeDefined();
  const antes = window.location.href;
  const clic = new MouseEvent('click', { bubbles: true, cancelable: true, button: 0 });
  fila!.dispatchEvent(clic);
  expect(clic.defaultPrevented).toBe(true);
  expect(window.location.href).toBe(antes);
  // El cuaderno del resultado abierto, con su nota, y la búsqueda fuera.
  await until(() => !host!.textContent?.includes('1 result'));
  await until(() => Boolean(host?.textContent?.includes('Quarterly plan')));
});

test('un fallo de la búsqueda no se disfraza de «nada encontrado» y ofrece reintentar', async () => {
  let veces = 0;
  montar((_method, path) => {
    if (!path.includes('/api/search')) return undefined;
    veces++;
    return veces === 1
      ? Response.json({ error: 'upstream_error' }, { status: 500 })
      : Response.json({ notes: [hit, { ...hit, id: 'n2', title: 'Budget' }] });
  });
  await abrirBusqueda();
  await until(() => Boolean(host?.textContent?.includes('The search could not be completed')));
  expect(host!.textContent).not.toContain('Nothing found');
  expect(host!.querySelector('[role="alert"]')).not.toBeNull();
  boton('Retry')!.click();
  await until(() => Boolean(host?.textContent?.includes('2 results')));
  expect(veces).toBe(2);
});

async function moverCuaderno(id: string) {
  await until(() => Boolean(document.querySelector(`[data-notebook-id="${id}"]`)));
  document
    .querySelector<HTMLButtonElement>(
      `[data-notebook-id="${id}"] button[aria-label="Notebook settings"]`,
    )!
    .click();
  await until(() =>
    [...document.querySelectorAll('[role="menuitem"]')].some(
      (e) => e.textContent === 'Move notebook to…',
    ),
  );
  // El del menú recién abierto: el último que hay.
  [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')]
    .filter((e) => e.textContent === 'Move notebook to…')
    .at(-1)!
    .click();
  await until(() => Boolean(document.querySelector('[role="dialog"]')));
  return document.querySelector<HTMLElement>('[role="dialog"]')!;
}

test('«Mover cuaderno a…» lo cuelga del elegido, sin ofrecer la raíz si ya está en ella', async () => {
  const llamadas = montar((method, path) =>
    method === 'PUT' && path.endsWith('/api/notebooks/nb2')
      ? Response.json({ ...nb('nb2', 'Work'), parent_id: 'nb' })
      : undefined,
  );
  const dialogo = await moverCuaderno('nb2');
  expect(dialogo.textContent).not.toContain('Top level');
  const home = [...dialogo.querySelectorAll<HTMLButtonElement>('button')].find((b) =>
    b.textContent?.includes('Home'),
  );
  expect(home).toBeDefined();
  home!.click();
  await until(() => llamadas.some((l) => l.method === 'PUT' && l.path.includes('/api/notebooks/')));
  const put = llamadas.find((l) => l.method === 'PUT' && l.path.includes('/api/notebooks/'))!;
  expect(put.path).toEndWith('/api/notebooks/nb2');
  expect(put.body).toMatchObject({ parent_id: 'nb', title: 'Work' });
});

test('«Mover cuaderno a…» saca un subcuaderno a la raíz y no lo ofrece dentro de sí', async () => {
  const hijo = { ...nb('nb2', 'Work'), parent_id: 'nb' } as Notebook;
  const llamadas = montar((method, path) => {
    if (method === 'GET' && path.endsWith('/api/notebooks'))
      return Response.json({ notebooks: [nb('nb', 'Home'), hijo] });
    if (method === 'PUT' && path.endsWith('/api/notebooks/nb'))
      return Response.json({ ...nb('nb', 'Home') });
    if (method === 'PUT' && path.endsWith('/api/notebooks/nb2'))
      return Response.json({ ...hijo, parent_id: null });
    return undefined;
  });
  // El padre no puede colgarse de su propio hijo: no queda ningún destino.
  const delPadre = await moverCuaderno('nb');
  expect(delPadre.textContent).toContain('There is nowhere else to move this notebook.');
  delPadre.querySelector<HTMLButtonElement>('button[aria-label="Close"]')?.click();
  await until(() => !document.querySelector('[role="dialog"]'));

  const dialogo = await moverCuaderno('nb2');
  const raiz = [...dialogo.querySelectorAll<HTMLButtonElement>('button')].find((b) =>
    b.textContent?.includes('Top level'),
  );
  expect(raiz).toBeDefined();
  raiz!.click();
  await until(() => llamadas.some((l) => l.method === 'PUT' && l.path.includes('/api/notebooks/')));
  const put = llamadas.find((l) => l.method === 'PUT' && l.path.includes('/api/notebooks/'))!;
  expect(put.path).toEndWith('/api/notebooks/nb2');
  expect(put.body).toMatchObject({ parent_id: null, title: 'Work' });
});

// Las mismas condiciones que el «…» de la página y que Fyne: sin administrar el
// cuaderno, o si es un día del diario, la fila no ofrece moverlo. Visto en rojo
// volviendo a mirar solo `read_only` en la fila.
test('la fila del árbol no ofrece «Mover cuaderno a…» sin poder administrarlo ni a un día', async () => {
  montar((method, path) =>
    method === 'GET' && path.endsWith('/api/notebooks')
      ? Response.json({
          notebooks: [
            nb('nb', 'Home'),
            { ...nb('nb2', 'Shared'), owner_sub: 'someone', visibility: 'organization' },
            // Un día solo sale en el árbol si tiene hijos.
            { ...nb('nb3', '2026-11-05'), daily_date: '2026-11-05' },
            { ...nb('nb4', 'Errands'), parent_id: 'nb3' },
          ],
        })
      : undefined,
  );
  for (const id of ['nb2', 'nb3']) {
    await until(() => Boolean(document.querySelector(`[data-notebook-id="${id}"]`)));
    document
      .querySelector<HTMLButtonElement>(
        `[data-notebook-id="${id}"] button[aria-label="Notebook settings"]`,
      )!
      .click();
    await until(() =>
      [...document.querySelectorAll('[role="menuitem"]')].some(
        (e) => e.textContent === 'New sub-notebook' || e.textContent === 'Notebook settings',
      ),
    );
    expect(
      [...document.querySelectorAll('[role="menuitem"]')].some(
        (e) => e.textContent === 'Move notebook to…',
      ),
    ).toBe(false);
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    await until(() => document.querySelectorAll('[role="menuitem"]').length === 0);
  }
});

// Un filtro que no encuentra nada en un cuaderno con tareas, todas hechas. Si
// además «ocultar completadas» está puesto, no es el filtro lo que las oculta:
// ofrecer quitarlo volvería al vacío de «este cuaderno está vacío». Visto en rojo
// contando `notes` en vez de las que deja el enfoque.
async function filtrarTareasHechas(ocultarHechas: boolean) {
  try {
    localStorage.setItem('sytue.notes.ocultarTerminadas', ocultarHechas ? '1' : '0');
  } catch {
    // sin almacenamiento, el test no tiene sentido; que falle abajo
  }
  const llamadas = montar((_method, path) =>
    path.includes('/notebooks/nb/notes')
      ? Response.json({ notes: [{ ...nota, kind: 'todo', checked: true }] })
      : undefined,
  );
  await until(() =>
    Boolean(document.querySelector('button[aria-label="Filter notes in this notebook"]')),
  );
  await until(() => llamadas.some((l) => l.path.includes('/notebooks/nb/notes')));
  await new Promise((r) => setTimeout(r, 50));
  document
    .querySelector<HTMLButtonElement>('button[aria-label="Filter notes in this notebook"]')!
    .click();
  const delFiltro = 'input[aria-label="Filter notes in this notebook"]';
  await until(() => Boolean(document.querySelector(delFiltro)));
  const campo = document.querySelector<HTMLInputElement>(delFiltro)!;
  campo.value = 'zzz';
  campo.dispatchEvent(new Event('input', { bubbles: true }));
  await new Promise((r) => setTimeout(r, 50));
}

test('el filtro sin coincidencias ofrece quitarlo cuando es él lo que las oculta', async () => {
  await filtrarTareasHechas(false);
  await until(() => Boolean(host?.textContent?.includes('No notes match this filter')));
  expect(boton('Clear filter')).toBeDefined();
});

test('no ofrece quitar el filtro si «ocultar completadas» ya las oculta todas', async () => {
  await filtrarTareasHechas(true);
  expect(host!.textContent).not.toContain('No notes match this filter');
  expect(boton('Clear filter')).toBeUndefined();
});

// Escribir sin enviar y marcar «incluir archivados» busca lo escrito. Si ESA falla,
// «Reintentar» repite esa misma (texto y casilla), no la anterior. Visto en rojo
// reintentando con `buscada`, que la casilla no actualizaba.
test('«Reintentar» repite la búsqueda que falló, con su texto y su casilla', async () => {
  const buscadas: string[] = [];
  montar((_method, path) => {
    if (!path.includes('/api/search')) return undefined;
    buscadas.push(path);
    return buscadas.length === 2
      ? Response.json({ error: 'upstream_error' }, { status: 500 })
      : Response.json({ notes: [hit] });
  });
  await abrirBusqueda();
  await until(() => buscadas.length === 1 && Boolean(host?.textContent?.includes('1 result')));
  const campo = host!.querySelector<HTMLInputElement>('input[aria-label="Search all your notes"]');
  expect(campo).not.toBeNull();
  campo!.value = 'plan';
  campo!.dispatchEvent(new Event('input', { bubbles: true }));
  await new Promise((r) => setTimeout(r, 20));
  const casilla = [...host!.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')].find(
    (c) => c.closest('label')?.textContent?.includes('Include archived notebooks'),
  );
  expect(casilla).toBeDefined();
  casilla!.click();
  await until(() => Boolean(host?.textContent?.includes('The search could not be completed')));
  expect(buscadas[1]).toContain('q=plan');
  boton('Retry')!.click();
  await until(() => buscadas.length === 3);
  expect(buscadas[2]).toContain('q=plan');
  expect(buscadas[2]).toContain('archived=1');
});
