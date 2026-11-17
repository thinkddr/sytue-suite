/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tirar para actualizar en sitio: Contacts absorbe `sytue:refresh` (el evento que
 * emite `usePullToRefresh` del shell) y recarga la lista y las etiquetas sin
 * recargar la página, con la ficha abierta siguiendo al dato fresco.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { ToastProvider } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import type { Contact } from '../client/contactsApi.js';
import { ContactsApp } from './ContactsApp.js';

// La app monta «Servicios vinculados», que arma una URL con `location.origin`: hace
// falta un origen de verdad, también si otro fichero registró happy-dom antes.
const ORIGEN = 'https://sytue.test/contacts';
if (typeof document === 'undefined') GlobalRegistrator.register({ url: ORIGEN });
else (window as unknown as { happyDOM?: { setURL: (u: string) => void } }).happyDOM?.setURL(ORIGEN);

const contacto = (uid: string, nombre: string): Contact => ({
  href: `/${uid}.vcf`,
  uid,
  fullName: nombre,
  emails: [],
  phones: [],
  vcard: `BEGIN:VCARD\r\nVERSION:4.0\r\nUID:${uid}\r\nFN:${nombre}\r\nEND:VCARD`,
});

const hosts: HTMLElement[] = [];
const fetchOriginal = globalThis.fetch;
/** Lo que devuelve /api/contacts en cada momento. */
let lista: Contact[] = [];
/** Peticiones GET por ruta (sin la consulta). */
let pedidas: Record<string, number> = {};

beforeEach(() => {
  lista = [contacto('ana', 'Ana'), contacto('luis', 'Luis')];
  pedidas = {};
  globalThis.fetch = (async (url: string) => {
    const ruta = String(url).split('?')[0] ?? '';
    pedidas[ruta] = (pedidas[ruta] ?? 0) + 1;
    const cuerpo: Record<string, unknown> = {
      '/api/addressbooks': { addressBooks: [{ url: '/books/u/default/', displayName: 'Mía' }] },
      '/api/contacts': { contacts: lista },
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
  for (let i = 0; i < 200 && !listo(); i++) await new Promise((r) => setTimeout(r, 10));
  if (!listo()) throw new Error('la condición no se cumplió a tiempo');
}

const usuario = { sub: 'u', email: 'u@x.eu', name: 'U', roles: [] } as unknown as SytueUser;

/** El título de la ficha abierta (el `h2` de su cabecera), o '' si no hay ficha. */
const ficha = (host: HTMLElement): string =>
  host.querySelector('header h2')?.textContent?.trim() ?? '';

test('el gesto recarga lista y etiquetas en sitio sin perder la ficha abierta', async () => {
  // La ficha se abre por enlace (`?uid=`, el del buscador de la suite): la lista es
  // virtual y en happy-dom, sin alturas, no pinta filas que pulsar.
  window.history.replaceState(null, '', '/contacts?uid=ana');
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
  await hasta(() => ficha(host) === 'Ana');
  expect(host.textContent).toContain('2 contactos');
  const antes = { contactos: pedidas['/api/contacts'] ?? 0, grupos: pedidas['/api/groups'] ?? 0 };

  // En el servidor, Ana ha cambiado de nombre y ha llegado alguien nuevo.
  lista = [contacto('ana', 'Ana María'), contacto('eva', 'Eva'), contacto('luis', 'Luis')];
  const esperas: Promise<unknown>[] = [];
  const ev = new CustomEvent('sytue:refresh', {
    cancelable: true,
    detail: { esperar: (p: Promise<unknown>) => esperas.push(p) },
  });
  window.dispatchEvent(ev);

  // Lo absorbe: el shell no recarga la página y espera a que acabe la recarga en sitio.
  expect(ev.defaultPrevented).toBe(true);
  expect(esperas).toHaveLength(1);
  await Promise.all(esperas);
  expect(pedidas['/api/contacts']).toBe(antes.contactos + 1);
  expect(pedidas['/api/groups']).toBe(antes.grupos + 1);
  await hasta(() => host.textContent?.includes('3 contactos') ?? false);
  // La ficha sigue abierta, ya con el dato fresco.
  await hasta(() => ficha(host) === 'Ana María');
});
