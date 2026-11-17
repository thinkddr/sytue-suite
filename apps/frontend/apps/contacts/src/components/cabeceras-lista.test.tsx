/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las cabeceras A–Z de la lista (CONT-05), con la app montada: salen con la libreta
 * entera, en el orden de la colación y con «#» al final; desaparecen con una búsqueda;
 * no cuentan como elementos de la lista y el teclado se las salta. La agrupación en sí
 * se prueba contra los vectores compartidos con Fyne (client/iniciales.test.ts).
 *
 * VISTO EN ROJO (2026-11-12): sin `listaConCabeceras` en ContactsApp no había ninguna
 * cabecera; sin `conCabeceras`, la búsqueda las seguía pintando; sin `filaDe`, Fin
 * desplazaba a la fila equivocada. Deshecho.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { ToastProvider } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import type { Contact } from '../client/contactsApi.js';
import { ContactsApp, CONTACT_LIST_ID, CONTACT_SEARCH_ID } from './ContactsApp.js';

const ORIGEN = 'https://sytue.test/contacts';
if (typeof document === 'undefined') GlobalRegistrator.register({ url: ORIGEN });
else (window as unknown as { happyDOM?: { setURL: (u: string) => void } }).happyDOM?.setURL(ORIGEN);

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

// Desordenados a propósito: la app los ordena con la colación del español.
const NOMBRES = ['Zoe', '123 Taxi', 'Ñoño', 'Ana', 'Álvaro', 'Nuria', 'beatriz'];
const libreta: Contact[] = NOMBRES.map((n, i) => ({
  href: `/c${i}.vcf`,
  uid: `c${i}`,
  fullName: n,
  emails: [],
  phones: [],
  vcard: `BEGIN:VCARD\r\nVERSION:4.0\r\nUID:c${i}\r\nFN:${n}\r\nEND:VCARD`,
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

const cabeceras = (host: HTMLElement): string[] =>
  [...host.querySelectorAll(`#${CONTACT_LIST_ID} > div > [role="presentation"]`)].map(
    (e) => e.textContent?.trim() ?? '',
  );

const nombresEnFilas = (host: HTMLElement): string[] =>
  [...host.querySelectorAll(`#${CONTACT_LIST_ID} [data-fila] [data-abrir]`)].map(
    (e) => e.textContent?.trim() ?? '',
  );

function tecla(key: string, en: EventTarget = document.activeElement ?? document.body): void {
  en.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
}

function filaConFoco(): number | null {
  const fila = (document.activeElement as HTMLElement | null)?.closest('[data-fila]');
  return fila ? Number(fila.getAttribute('data-fila')) : null;
}

test('la libreta entera va por iniciales, con la Ñ aparte y «#» al final', async () => {
  const host = await montar();
  expect(cabeceras(host)).toEqual(['A', 'B', 'N', 'Ñ', 'Z', '#']);
  expect(nombresEnFilas(host)).toEqual([
    'Álvaro',
    'Ana',
    'beatriz',
    'Nuria',
    'Ñoño',
    'Zoe',
    '123 Taxi',
  ]);
  // Las cabeceras no son elementos: «Ana, 2 de 7» habla de contactos, no de filas.
  const elementos = [...host.querySelectorAll(`#${CONTACT_LIST_ID} [role="listitem"]`)];
  expect(elementos.length).toBe(7);
  expect(elementos.map((e) => e.getAttribute('aria-posinset'))).toEqual([
    '1',
    '2',
    '3',
    '4',
    '5',
    '6',
    '7',
  ]);
  expect(elementos.every((e) => e.getAttribute('aria-setsize') === '7')).toBe(true);
});

test('con una búsqueda no hay cabeceras', async () => {
  const host = await montar();
  const buscador = host.querySelector<HTMLInputElement>(`#${CONTACT_SEARCH_ID}`);
  if (!buscador) throw new Error('sin buscador');
  buscador.value = 'a';
  buscador.dispatchEvent(new Event('input', { bubbles: true }));
  await hasta(() => cabeceras(host).length === 0);
  expect(nombresEnFilas(host).length).toBeGreaterThan(1);
  buscador.value = '';
  buscador.dispatchEvent(new Event('input', { bubbles: true }));
  await hasta(() => cabeceras(host).length === 6);
});

test('el teclado se salta las cabeceras', async () => {
  const host = await montar();
  tecla('j', document.body);
  await hasta(() => filaConFoco() === 0);
  tecla('ArrowDown');
  tecla('ArrowDown');
  // De «Ana» (último de la A) a «beatriz», sin pararse en la B.
  await hasta(() => filaConFoco() === 2);
  expect(document.activeElement?.textContent).toContain('beatriz');
  tecla('End');
  await hasta(() => filaConFoco() === 6);
  expect(document.activeElement?.textContent).toContain('123 Taxi');
  expect(cabeceras(host)).toContain('#');
});
