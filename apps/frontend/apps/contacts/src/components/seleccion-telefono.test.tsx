/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL ESCRITORIO NO VA EN EL TELÉFONO (roadmap, «Rendimiento y estética de la web», medido
 * el 2026-11-24): una casilla en cada fila y el «Importar / Exportar» con texto en la
 * cabecera. Por debajo de 768 las casillas solo salen en el MODO DE SELECCIÓN, al que se
 * entra manteniendo el dedo sobre un contacto o con «Seleccionar» del «…» —la vía que no
 * pide el gesto: teclado y lector de pantalla—; en él, tocar una fila la marca en vez de
 * abrirla. El «…» lleva también lo de importar y exportar y «Revisar duplicados».
 *
 * happy-dom no evalúa `max-md:`: el corte lo dice la clase y el comportamiento que
 * depende de él (`useEnHoja`) se prueba con `matchMedia` fingido.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { ToastProvider } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import type { Contact } from '../client/contactsApi.js';
import { ContactsApp, CONTACT_LIST_ID } from './ContactsApp.js';

const ORIGEN = 'https://sytue.test/contacts';
if (typeof document === 'undefined') GlobalRegistrator.register({ url: ORIGEN });
else (window as unknown as { happyDOM?: { setURL: (u: string) => void } }).happyDOM?.setURL(ORIGEN);

// La lista es virtual: sin alto no pinta filas (como en teclado-lista.test.tsx).
const geometriaOriginal = new Map<string, PropertyDescriptor | undefined>();
beforeAll(() => {
  for (const [campo, medida] of [
    ['clientHeight', 600],
    ['clientWidth', 800],
  ] as const) {
    geometriaOriginal.set(campo, Object.getOwnPropertyDescriptor(HTMLElement.prototype, campo));
    Object.defineProperty(HTMLElement.prototype, campo, { configurable: true, get: () => medida });
  }
});
afterAll(() => {
  for (const [campo, d] of geometriaOriginal) {
    if (d) Object.defineProperty(HTMLElement.prototype, campo, d);
    else Reflect.deleteProperty(HTMLElement.prototype, campo);
  }
});

const nombre = (i: number): string => `Contacto ${String(i).padStart(2, '0')}`;
const libreta: Contact[] = Array.from({ length: 6 }, (_, i) => ({
  href: `/c${i}.vcf`,
  uid: `c${i}`,
  fullName: nombre(i),
  emails: [`c${i}@x.eu`],
  phones: [],
  vcard: `BEGIN:VCARD\r\nVERSION:4.0\r\nUID:c${i}\r\nFN:${nombre(i)}\r\nEND:VCARD`,
}));
// Dos con el mismo correo: «Revisar duplicados» tiene algo que revisar. Al final del
// orden, para que la fila 1 siga siendo «Contacto 01».
libreta.push({ ...libreta[0]!, href: '/dup.vcf', uid: 'dup', fullName: 'Zeta' });

const hosts: HTMLElement[] = [];
const fetchOriginal = globalThis.fetch;
const matchMediaOriginal = window.matchMedia;
let telefono = true;

beforeEach(() => {
  telefono = true;
  window.history.replaceState(null, '', '/contacts');
  window.matchMedia = ((q: string) => ({
    matches: telefono && q.includes('48rem'),
    media: q,
    addEventListener: () => {},
    removeEventListener: () => {},
  })) as unknown as typeof window.matchMedia;
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
  window.matchMedia = matchMediaOriginal;
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

/** Las envolturas de las casillas de las filas: `max-md:hidden` fuera del modo. */
const casillas = (host: HTMLElement): HTMLElement[] => [
  ...host.querySelectorAll<HTMLElement>(`#${CONTACT_LIST_ID} [data-fila] label`),
];
const ocultasEnTelefono = (host: HTMLElement): boolean =>
  casillas(host).every((l) => l.className.includes('max-md:hidden'));
const ficha = (host: HTMLElement): string =>
  host.querySelector('header h2')?.textContent?.trim() ?? '';

/** Abre el «…» del teléfono y devuelve sus entradas por su texto. */
async function menuDelTelefono(host: HTMLElement): Promise<Map<string, HTMLElement>> {
  const mas = host.querySelector<HTMLButtonElement>('button[aria-label="Más acciones"]');
  expect(mas).not.toBeNull();
  // Es el del teléfono: el del escritorio, con texto, se esconde por debajo de 768.
  expect(mas!.closest('.md\\:hidden')).not.toBeNull();
  mas!.click();
  await hasta(() => document.querySelector('[role="menu"]') !== null);
  return new Map(
    [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')].map((e) => [
      e.textContent?.trim() ?? '',
      e,
    ]),
  );
}

test('en el teléfono no hay casilla en cada fila hasta entrar en el modo de selección', async () => {
  const host = await montar();
  expect(casillas(host).length).toBeGreaterThan(0);
  expect(ocultasEnTelefono(host)).toBe(true);
  // El «Importar / Exportar» con texto es del escritorio.
  const io = [...host.querySelectorAll('button')].find((b) =>
    b.textContent?.includes('Importar / Exportar'),
  );
  expect(io?.closest('.max-md\\:hidden')).not.toBeNull();
});

test('el «…» trae «Seleccionar», los duplicados y lo de importar y exportar', async () => {
  const host = await montar();
  const menu = await menuDelTelefono(host);
  expect([...menu.keys()]).toEqual([
    'Seleccionar',
    'Revisar duplicados (1)',
    'Importar vCard (.vcf)',
    'Importar CSV (Google/Outlook)',
    'Exportar vCard (.vcf)',
    'Exportar CSV',
  ]);
  // «Revisar duplicados» de la cabecera también es solo del escritorio.
  const dup = [...host.querySelectorAll('button')].find(
    (b) => b.textContent === 'Revisar duplicados (1)',
  );
  expect(dup?.className).toContain('max-md:hidden');
});

test('«Seleccionar» enseña las casillas, tocar una fila la marca y «Cancelar» sale', async () => {
  const host = await montar();
  (await menuDelTelefono(host)).get('Seleccionar')!.click();
  await hasta(() => !ocultasEnTelefono(host));
  expect(document.querySelector('[role="menu"]')).toBeNull();
  const cancelar = [...host.querySelectorAll('button')].find((b) => b.textContent === 'Cancelar');
  expect(cancelar).toBeDefined();

  // Tocar la fila marca y no abre la ficha; la fila lo dice como conmutador.
  const fila = host.querySelector<HTMLButtonElement>('[data-fila="1"] [data-abrir]')!;
  fila.click();
  await hasta(() => /(^|\D)1 contacto seleccionado/.test(document.body.textContent ?? ''));
  expect(ficha(host)).toBe('');
  expect(host.querySelector('[data-fila="1"] [data-abrir]')!.getAttribute('aria-pressed')).toBe(
    'true',
  );

  // Desmarcarla deja el modo puesto (se entró a propósito) y vuelve «Cancelar».
  host.querySelector<HTMLButtonElement>('[data-fila="1"] [data-abrir]')!.click();
  await hasta(() => [...host.querySelectorAll('button')].some((b) => b.textContent === 'Cancelar'));
  expect(ocultasEnTelefono(host)).toBe(false);
  [...host.querySelectorAll<HTMLButtonElement>('button')]
    .find((b) => b.textContent === 'Cancelar')!
    .click();
  await hasta(() => ocultasEnTelefono(host));
  // Fuera del modo, tocar la fila vuelve a abrir la ficha.
  host.querySelector<HTMLButtonElement>('[data-fila="1"] [data-abrir]')!.click();
  await hasta(() => ficha(host) === nombre(1));
});

test('en el escritorio la fila abre la ficha aunque haya algo marcado', async () => {
  telefono = false;
  const host = await montar();
  host.querySelector<HTMLInputElement>('[data-fila="0"] input[type="checkbox"]')!.click();
  await hasta(() => /(^|\D)1 contacto seleccionado/.test(document.body.textContent ?? ''));
  host.querySelector<HTMLButtonElement>('[data-fila="1"] [data-abrir]')!.click();
  await hasta(() => ficha(host) === nombre(1));
  expect(host.querySelector('[data-fila="1"] [data-abrir]')!.hasAttribute('aria-pressed')).toBe(
    false,
  );
});

/**
 * Las acciones de la libreta (y las de la etiqueta) eran botones de icono sueltos en la
 * cabecera del teléfono, junto al «…»: tres iconos que no se distinguían (roadmap,
 * «Rendimiento y estética de la web»). En el teléfono van dentro del «…», en un grupo con
 * su nombre; el botón de la cabecera queda para el escritorio. VISTO EN ROJO con el
 * botón de la libreta a la vista y sin grupo en el «…».
 */
test('en el teléfono las acciones de la libreta van en el «…», en su grupo', async () => {
  const conFetch = globalThis.fetch;
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    if (String(url).split('?')[0] === '/api/addressbooks')
      return new Response(
        JSON.stringify({
          addressBooks: [
            { url: '/books/u/default/', displayName: 'Mía', id: 'b1', isDefault: true },
          ],
        }),
        { status: 200 },
      );
    return conFetch(url, init);
  }) as typeof fetch;
  const host = await montar();
  const boton = host.querySelector<HTMLButtonElement>(
    'button[aria-label="Acciones de la libreta"]',
  );
  expect(boton?.closest('.max-md\\:hidden')).not.toBeNull();
  const menu = await menuDelTelefono(host);
  expect(menu.has('Cambiar el nombre')).toBe(true);
  // La predeterminada no se puede borrar: el grupo solo trae «Cambiar el nombre».
  expect(menu.has('Eliminar libreta')).toBe(false);
  const grupo = menu.get('Cambiar el nombre')!.closest('[role="group"]');
  expect(grupo?.getAttribute('aria-label')).toBe('Acciones de la libreta');
});
