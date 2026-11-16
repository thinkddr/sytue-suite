/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El teclado de la lista: una libreta de 1.000 contactos se recorre sin ratón. Una
 * sola parada de Tab (la fila del cursor), ↑ ↓ Inicio Fin con el foco en la lista,
 * `j`/`k` desde cualquier sitio (y la ficha abierta sigue al cursor), `/` al
 * buscador, `x` marca, `?` abre la hoja. La tabla de `hojaDeAtajos.ts` se prueba
 * ejecutándola; el cableado, montando la app con una ventana de verdad (happy-dom
 * no tiene alturas: se fingen como en el test de `VirtualList`).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { ToastProvider } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import type { Contact } from '../client/contactsApi.js';
import { ContactsApp, CONTACT_LIST_ID, CONTACT_SEARCH_ID } from './ContactsApp.js';
import { accionDeAtajo, accionDeFlecha, filaDestino, gruposDeAtajos } from './hojaDeAtajos.js';

const ORIGEN = 'https://sytue.test/contacts';
if (typeof document === 'undefined') GlobalRegistrator.register({ url: ORIGEN });
else (window as unknown as { happyDOM?: { setURL: (u: string) => void } }).happyDOM?.setURL(ORIGEN);

// ---- La tabla ----

test('las letras se escuchan en toda la app; las flechas, solo en la lista', () => {
  expect(accionDeAtajo('j')).toBe('siguiente');
  expect(accionDeAtajo('k')).toBe('anterior');
  expect(accionDeAtajo('/')).toBe('buscar');
  expect(accionDeAtajo('e')).toBe('editar');
  expect(accionDeAtajo('s')).toBe('destacar');
  expect(accionDeAtajo('x')).toBe('marcar');
  expect(accionDeAtajo('?')).toBe('ayuda');
  expect(accionDeAtajo('n')).toBe('nuevo');
  // Fuera de la lista las flechas son del scroll de la página.
  expect(accionDeAtajo('ArrowDown')).toBeNull();
  expect(accionDeAtajo('Home')).toBeNull();
  // Intro lo da el botón de la fila: nadie lo escucha a mano.
  expect(accionDeAtajo('Enter')).toBeNull();
  expect(accionDeFlecha('ArrowDown')).toBe('siguiente');
  expect(accionDeFlecha('End')).toBe('ultimo');
  expect(accionDeFlecha('j')).toBeNull();
});

test('el destino de cada movimiento se queda dentro de la lista', () => {
  expect(filaDestino('siguiente', 3, 10)).toBe(4);
  expect(filaDestino('siguiente', 9, 10)).toBe(9);
  expect(filaDestino('anterior', 0, 10)).toBe(0);
  expect(filaDestino('primero', 7, 10)).toBe(0);
  expect(filaDestino('ultimo', 0, 1000)).toBe(999);
  // La primera j sin nada abierto aterriza en el cursor, no en el siguiente.
  expect(filaDestino('siguiente', 0, 10, false)).toBe(0);
  expect(filaDestino('siguiente', 0, 0)).toBeNull();
  expect(filaDestino('buscar', 0, 10)).toBeNull();
});

test('la hoja de «?» cuenta todas las teclas que la app escucha, en el idioma de quien lee', () => {
  const t = ((k: string) => `«${k}»`) as never;
  const grupos = gruposDeAtajos(t);
  expect(grupos.map((g) => g.titulo)).toEqual([
    '«shortcuts.group.move»',
    '«shortcuts.group.actions»',
  ]);
  const teclas = grupos.flatMap((g) => g.atajos.flatMap((a) => a.teclas));
  for (const k of ['j', '↓', 'k', '↑', '«shortcuts.keyHome»', '«shortcuts.keyEnd»', '↵', '/'])
    expect(teclas).toContain(k);
  for (const k of ['n', 'e', 's', 'x', '?']) expect(teclas).toContain(k);
  // Ninguna descripción se queda sin texto.
  for (const g of grupos) for (const a of g.atajos) expect(a.descripcion).toMatch(/^«.+»$/);
});

// ---- El cableado, con la app montada ----

const VIEWPORT_H = 600;
const geometriaOriginal = new Map<string, PropertyDescriptor | undefined>();

beforeAll(() => {
  for (const [campo, medida] of [
    ['clientHeight', VIEWPORT_H],
    ['clientWidth', 800],
  ] as const) {
    geometriaOriginal.set(campo, Object.getOwnPropertyDescriptor(HTMLElement.prototype, campo));
    Object.defineProperty(HTMLElement.prototype, campo, {
      configurable: true,
      get: () => medida,
    });
  }
});

afterAll(() => {
  for (const [campo, d] of geometriaOriginal) {
    if (d) Object.defineProperty(HTMLElement.prototype, campo, d);
    else Reflect.deleteProperty(HTMLElement.prototype, campo);
  }
});

const nombre = (i: number): string => `Contacto ${String(i).padStart(4, '0')}`;
const libreta: Contact[] = Array.from({ length: 1000 }, (_, i) => ({
  href: `/c${i}.vcf`,
  uid: `c${i}`,
  fullName: nombre(i),
  emails: [],
  phones: [],
  vcard: `BEGIN:VCARD\r\nVERSION:4.0\r\nUID:c${i}\r\nFN:${nombre(i)}\r\nEND:VCARD`,
}));

const hosts: HTMLElement[] = [];
const fetchOriginal = globalThis.fetch;

beforeEach(() => {
  window.history.replaceState(null, '', '/contacts');
  globalThis.fetch = (async (url: string) => {
    const ruta = String(url).split('?')[0] ?? '';
    const cuerpo: Record<string, unknown> = {
      '/api/addressbooks': { addressBooks: [{ url: '/books/u/default/', displayName: 'Mía' }] },
      '/api/contacts': { contacts: libreta },
      '/api/groups': { groups: [] },
      '/api/favorites': { uids: [] },
      '/api/directorio': {},
    };
    return new Response(JSON.stringify(cuerpo[ruta] ?? {}), { status: 200 });
  }) as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = fetchOriginal;
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
});

async function hasta(listo: () => boolean): Promise<void> {
  for (let i = 0; i < 300 && !listo(); i++) await new Promise((r) => setTimeout(r, 10));
  if (!listo()) throw new Error('la condición no se cumplió a tiempo');
}

const usuario = { sub: 'u', email: 'u@x.eu', name: 'U', roles: [] } as unknown as SytueUser;

async function montar(): Promise<HTMLElement> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  hosts.push(host);
  render(
    <I18nProvider locale="es">
      <ToastProvider>
        <ContactsApp user={usuario} hubBaseUrl="https://hub.example.eu" locale="es" />
      </ToastProvider>
    </I18nProvider>,
    host,
  );
  await hasta(() => host.querySelector(`#${CONTACT_LIST_ID} [data-fila="0"]`) !== null);
  return host;
}

/** El índice de la fila con el foco, o null. */
function filaConFoco(): number | null {
  const fila = (document.activeElement as HTMLElement | null)?.closest('[data-fila]');
  return fila ? Number(fila.getAttribute('data-fila')) : null;
}

function tecla(key: string, en: EventTarget = document.activeElement ?? document.body): void {
  en.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
}

const ficha = (host: HTMLElement): string =>
  host.querySelector('header h2')?.textContent?.trim() ?? '';

test('la lista es UNA parada de Tab, no tres por contacto', async () => {
  const host = await montar();
  const filas = host.querySelectorAll(`#${CONTACT_LIST_ID} [data-fila]`);
  expect(filas.length).toBeGreaterThan(5);
  // Las paradas viven todas en la fila del cursor (la 0); el resto, fuera de Tab.
  const paradas = [
    ...host.querySelectorAll<HTMLElement>(`#${CONTACT_LIST_ID} button, #${CONTACT_LIST_ID} input`),
  ].filter((el) => el.tabIndex >= 0);
  expect(paradas.length).toBeGreaterThan(0);
  for (const el of paradas) expect(el.closest('[data-fila]')?.getAttribute('data-fila')).toBe('0');
});

test('↓ ↑ Inicio Fin recorren las 1.000 filas con el foco, sin abrir fichas', async () => {
  const host = await montar();
  tecla('j', document.body);
  await hasta(() => filaConFoco() === 0);
  tecla('ArrowDown');
  await hasta(() => filaConFoco() === 1);
  tecla('ArrowDown');
  tecla('ArrowUp');
  await hasta(() => filaConFoco() === 1);
  // Fin: la fila 999 está lejos de la ventana; el virtualizador la pinta y recoge el foco.
  tecla('End');
  await hasta(() => filaConFoco() === 999);
  expect(document.activeElement?.textContent).toContain(nombre(999));
  // La parada de Tab se ha ido con el cursor.
  const parada = host.querySelector<HTMLElement>(`[data-fila="999"] [data-abrir]`);
  expect(parada?.tabIndex).toBe(0);
  expect(host.querySelector<HTMLElement>(`[data-fila="0"] [data-abrir]`)?.tabIndex ?? -1).toBe(-1);
  tecla('Home');
  await hasta(() => filaConFoco() === 0);
  // Moverse no abre: la ficha sigue vacía.
  expect(ficha(host)).toBe('');
  // Mil filas renderizadas: en un runner ARM cargado los 5 s por defecto no bastan.
}, 20_000);

test('con una ficha abierta, j y k la llevan a la siguiente y a la anterior', async () => {
  const host = await montar();
  host.querySelector<HTMLElement>(`[data-fila="0"] [data-abrir]`)?.click();
  await hasta(() => ficha(host) === nombre(0));
  tecla('j', document.body);
  await hasta(() => ficha(host) === nombre(1));
  expect(filaConFoco()).toBe(1);
  tecla('j');
  await hasta(() => ficha(host) === nombre(2));
  tecla('k');
  await hasta(() => ficha(host) === nombre(1));
}, 20_000);

test('«/» lleva al buscador, y en él las letras se escriben', async () => {
  await montar();
  tecla('/', document.body);
  await hasta(() => document.activeElement?.id === CONTACT_SEARCH_ID);
  const buscador = document.activeElement as HTMLInputElement;
  const ev = new KeyboardEvent('keydown', { key: 'j', bubbles: true, cancelable: true });
  buscador.dispatchEvent(ev);
  expect(ev.defaultPrevented).toBe(false);
  expect(document.activeElement).toBe(buscador);
});

test('«x» marca la fila del cursor y «?» abre la hoja, que se calla las demás teclas', async () => {
  const host = await montar();
  tecla('j', document.body);
  await hasta(() => filaConFoco() === 0);
  tecla('ArrowDown');
  await hasta(() => filaConFoco() === 1);
  tecla('x');
  await hasta(
    () =>
      host.querySelector<HTMLInputElement>('[data-fila="1"] input[type="checkbox"]')?.checked ===
      true,
  );
  // Solo ESA: la barra del lote cuenta una (en happy-dom las filas miden 0 y la
  // ventana se recoloca, así que se cuenta en la barra y no en las casillas pintadas).
  await hasta(() => /(^|\D)1 contacto seleccionado/.test(document.body.textContent ?? ''));

  tecla('?', document.body);
  await hasta(() => document.querySelector('[role="dialog"]') !== null);
  const dialogo = document.querySelector('[role="dialog"]');
  expect(dialogo?.textContent).toContain('Atajos de teclado');
  expect(dialogo?.textContent).toContain('Contacto siguiente');
  // Con la hoja abierta, j no mueve nada detrás.
  tecla('j', document.body);
  await new Promise((r) => setTimeout(r, 30));
  expect(ficha(host)).toBe('');
}, 20_000);
