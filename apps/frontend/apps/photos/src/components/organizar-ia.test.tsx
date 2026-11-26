/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Organizar con IA» en «Álbumes», de punta a punta y contra un servicio de mentira:
 * parte de las fotos sin álbum, describe SOLO las que no tienen nada, enseña la
 * propuesta antes de tocar nada, crea lo marcado (o lo añade al álbum que ya se
 * llama igual) y lo deshace. Y los estados: sin Sytue AI, nada que organizar y
 * «Parar» a mitad. Espejo de `TestOrganizar…` en `apps/fyne/apps/photos`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { Foto } from '../client/photosApi.js';
import { conAvisos } from './Fallo.js';
import { OrganizarConIA, type ServicioOrganizar } from './OrganizarConIA.js';

let registrado = false;
beforeAll(() => {
  if (typeof document !== 'undefined') return;
  GlobalRegistrator.register();
  registrado = true;
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const Organizar = conAvisos(OrganizarConIA);

let host: HTMLElement;
afterEach(async () => {
  await act(async () => render(null, host));
  host.remove();
});

function foto(id: string, description?: string): Foto {
  return {
    id,
    name: `${id}.jpg`,
    contentType: 'image/jpeg',
    sizeBytes: 1,
    width: 4,
    height: 3,
    takenAt: '2024-07-14T10:00:00Z',
    takenAtFromExif: true,
    favorite: false,
    driveFileId: `d-${id}`,
    ...(description ? { description } : {}),
  };
}

/** Un servicio de mentira que apunta lo que se le pide. */
function servicio(cambios: Partial<ServicioOrganizar> = {}) {
  const pedido: string[] = [];
  const apunta =
    <A extends unknown[], R>(nombre: string, r: (...a: A) => R) =>
    (...a: A) => {
      pedido.push(
        `${nombre} ${a
          .filter((x) => !(x instanceof AbortSignal))
          .map((x) => JSON.stringify(x))
          .join(' ')}`.trim(),
      );
      return r(...a);
    };
  let agrupadas: Foto[] = [];
  const s: ServicioOrganizar = {
    albumes: apunta('albumes', () =>
      Promise.resolve([{ id: 'v', title: 'Verano en Cádiz', count: 1 }]),
    ),
    fotosDelAlbum: apunta('fotosDelAlbum', () => Promise.resolve([{ id: 'f9' }])),
    pagina: apunta('pagina', () =>
      Promise.resolve({
        items: [
          foto('f1', 'Una cala'),
          foto('f2', 'La sierra'),
          foto('f3'),
          foto('f4'),
          foto('f5', 'Nieve'),
          foto('f9', 'Ya en un álbum'),
        ],
      }),
    ),
    describir: apunta('describir', (id: string) =>
      Promise.resolve({ description: `Descrita ${id}`, tags: ['playa'] }),
    ),
    guardarDescripcion: apunta('guardar', () => Promise.resolve()),
    agrupar: apunta('agrupar', (fotos: Foto[]) => {
      agrupadas = fotos;
      return Promise.resolve([
        { titulo: 'VERANO EN CADIZ', ids: ['f1', 'f3'] },
        { titulo: 'Montaña', ids: ['f2', 'f4', 'f5'] },
      ]);
    }),
    crearAlbum: apunta('crear', (titulo: string) =>
      Promise.resolve({ id: 'nuevo', title: titulo, count: 0 }),
    ),
    anyadirAlAlbum: apunta('añadir', () => Promise.resolve({ added: 2 })),
    borrarAlbum: apunta('borrar', () => Promise.resolve()),
    quitarDelAlbum: apunta('quitar', () => Promise.resolve()),
    enBoveda: () => Promise.resolve(false),
    ...cambios,
  };
  return { s, pedido, agrupadas: () => agrupadas };
}

let cambiada = 0;
async function montar(s: ServicioOrganizar): Promise<HTMLElement> {
  cambiada = 0;
  host = document.body.appendChild(document.createElement('div'));
  await act(async () =>
    render(<Organizar locale="es" servicio={s} alCambiar={() => cambiada++} />, host),
  );
  return host;
}

const boton = (texto: string): HTMLButtonElement => {
  const b = [...document.querySelectorAll('button')].find((x) => x.textContent?.trim() === texto);
  if (!b) throw new Error(`no hay botón «${texto}»: ${document.body.textContent}`);
  return b;
};
/** Deja correr las promesas del servicio de mentira. */
const correr = () => act(() => new Promise((r) => setTimeout(r, 0)));
async function pulsar(texto: string): Promise<void> {
  await act(async () => boton(texto).click());
  for (let i = 0; i < 20; i++) await correr();
}

test('describe solo lo que no tiene nada, enseña la propuesta y no crea nada todavía', async () => {
  const { s, pedido, agrupadas } = servicio();
  const h = await montar(s);
  await pulsar('Organizar con IA');

  expect(pedido.filter((p) => p.startsWith('describir'))).toEqual([
    'describir "f3"',
    'describir "f4"',
  ]);
  expect(pedido).toContain('guardar "f3" "Descrita f3" ["playa"]');
  // La que ya está en un álbum no entra; las descritas llegan con lo que se vio.
  expect(agrupadas().map((f) => [f.id, f.description])).toEqual([
    ['f1', 'Una cala'],
    ['f2', 'La sierra'],
    ['f3', 'Descrita f3'],
    ['f4', 'Descrita f4'],
    ['f5', 'Nieve'],
  ]);
  expect(pedido.some((p) => p.startsWith('crear') || p.startsWith('añadir'))).toBe(false);

  const filas = [...h.querySelectorAll('li')];
  expect(
    filas.map((li) => (li.querySelector('input[type="text"]') as HTMLInputElement).value),
  ).toEqual(['VERANO EN CADIZ', 'Montaña']);
  expect(filas[0]!.textContent).toContain('2 fotos');
  expect(filas[0]!.textContent).toContain('Se añadirán al álbum que ya tiene este nombre');
  expect(filas[1]!.textContent).toContain('3 fotos');
  expect(filas[1]!.textContent).not.toContain('Se añadirán');
  expect(filas[1]!.querySelectorAll('img').length).toBe(3);
  expect(h.textContent).toContain('Álbumes propuestos: 2');
});

test('crea lo marcado, añade al que ya se llama igual y lo deshace', async () => {
  const { s, pedido } = servicio();
  const h = await montar(s);
  await pulsar('Organizar con IA');

  const filas = [...h.querySelectorAll('li')];
  // Se edita el título del segundo y se desmarca el primero… y se vuelve a marcar.
  const titulo = filas[1]!.querySelector('input[type="text"]') as HTMLInputElement;
  await act(async () => {
    titulo.value = 'Sierra Nevada';
    titulo.dispatchEvent(new Event('input', { bubbles: true }));
  });
  const casilla = filas[0]!.querySelector('input[type="checkbox"]') as HTMLInputElement;
  expect(casilla.getAttribute('aria-label')).toBe('Incluir «VERANO EN CADIZ»');
  await act(async () => casilla.click());
  expect(casilla.checked).toBe(false);
  await act(async () => casilla.click());

  pedido.length = 0;
  await pulsar('Crear álbumes');
  expect(pedido).toEqual([
    'añadir "v" ["f1","f3"]',
    'crear "Sierra Nevada"',
    'añadir "nuevo" ["f2","f4","f5"]',
  ]);
  expect(cambiada).toBe(1);
  expect(document.body.textContent).toContain('Álbumes organizados: 2');
  expect(document.body.textContent).toContain('Fotos añadidas: 5');

  pedido.length = 0;
  await pulsar('Deshacer');
  expect(pedido.sort()).toEqual(['borrar "nuevo"', 'quitar "v" "f1"', 'quitar "v" "f3"']);
  expect(document.body.textContent).toContain('Se ha deshecho la organización');
});

test('lo desmarcado no se crea', async () => {
  const { s, pedido } = servicio();
  const h = await montar(s);
  await pulsar('Organizar con IA');
  const casilla = h
    .querySelectorAll('li')[1]!
    .querySelector('input[type="checkbox"]') as HTMLInputElement;
  await act(async () => casilla.click());
  pedido.length = 0;
  await pulsar('Crear álbumes');
  expect(pedido).toEqual(['añadir "v" ["f1","f3"]']);
});

test('sin Sytue AI lo dice y no sigue', async () => {
  const { s, pedido } = servicio({
    describir: () => Promise.reject(new Error('ai_not_configured')),
  });
  const h = await montar(s);
  await pulsar('Organizar con IA');
  expect(h.textContent).toContain('Sytue AI no está configurada en esta instalación');
  expect(pedido.some((p) => p.startsWith('agrupar'))).toBe(false);
});

test('sin fotos sueltas es un estado vacío, y sin grupos claros otro', async () => {
  const sinSueltas = servicio({ pagina: () => Promise.resolve({ items: [foto('f9', 'x')] }) });
  let h = await montar(sinSueltas.s);
  await pulsar('Organizar con IA');
  expect(h.querySelector('h3')?.textContent).toBe('Todas tus fotos ya están en algún álbum');
  expect(sinSueltas.pedido.some((p) => p.startsWith('agrupar'))).toBe(false);
  await act(async () => render(null, host));
  host.remove();

  h = await montar(servicio({ agrupar: () => Promise.resolve([]) }).s);
  await pulsar('Organizar con IA');
  expect(h.querySelector('h3')?.textContent).toBe(
    'Sytue AI no ha encontrado grupos claros en estas fotos',
  );
});

test('«Parar» mientras describe no llega a agrupar', async () => {
  let soltar: () => void = () => {};
  const { s, pedido } = servicio({
    describir: () =>
      new Promise((r) => {
        soltar = () => r({ description: 'x', tags: [] });
      }),
  });
  const h = await montar(s);
  await pulsar('Organizar con IA');
  expect(h.textContent).toContain('Describiendo 0 de 2…');
  await pulsar('Parar');
  soltar();
  for (let i = 0; i < 10; i++) await correr();
  expect(pedido.some((p) => p.startsWith('agrupar'))).toBe(false);
  expect(h.querySelector('[role="status"]')).toBeNull();
  expect(h.querySelectorAll('li').length).toBe(0);
});

// VISTO EN ROJO: con un `parar` de sí o no, la segunda pasada lo ponía a «no» y la
// primera, al volver su descripción, seguía hasta agrupar otra vez.
test('«Parar» y volver a empezar no deja dos pasadas a la vez', async () => {
  const pendientes: (() => void)[] = [];
  const { s, pedido } = servicio({
    describir: (id) =>
      new Promise((r) => pendientes.push(() => r({ description: `Descrita ${id}`, tags: [] }))),
  });
  await montar(s);
  await pulsar('Organizar con IA');
  await pulsar('Parar');
  await pulsar('Organizar con IA');
  for (let i = 0; i < 10; i++) {
    for (const soltar of pendientes.splice(0)) soltar();
    for (let j = 0; j < 5; j++) await correr();
  }
  expect(pedido.filter((p) => p.startsWith('agrupar')).length).toBe(1);
});

test('con el Drive en la bóveda dice que las cifradas no entran', async () => {
  const h = await montar(servicio({ enBoveda: () => Promise.resolve(true) }).s);
  await pulsar('Organizar con IA');
  expect(h.textContent).toContain(
    'Sytue AI, las fotos parecidas, cerca y los recuerdos no las incluyen',
  );
});
