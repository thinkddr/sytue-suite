/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pasar de conversación desde el lector (MAIL-11): «Más reciente» y «Más antigua»
 * hacen lo que j/k sin volver a la lista, se apagan en los extremos y, en la última
 * conversación cargada, piden la página siguiente y abren la que llega. Ejecutar:
 * `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { SytueUser } from '@sytue/auth';

const ORIGEN = 'https://sytue.test/mail/';
if (typeof document === 'undefined') GlobalRegistrator.register({ url: ORIGEN });
else (window as unknown as { happyDOM?: { setURL: (u: string) => void } }).happyDOM?.setURL(ORIGEN);
process.env.BASE_URL ??= '/mail/';
(globalThis as { IntersectionObserver?: unknown }).IntersectionObserver ??= class {
  observe(): void {}
  disconnect(): void {}
};

const { I18nProvider } = await import('@sytue/i18n/preact');
const { ToastProvider } = await import('@sytue/ui');
const { MailApp } = await import('./MailApp.js');

const hosts: HTMLElement[] = [];
const fetchOriginal = globalThis.fetch;
const paginasPedidas: string[] = [];

function correo(id: string, hora: number) {
  return {
    id,
    threadId: `h-${id}`,
    subject: `Asunto ${id}`,
    from: [{ email: 'ana@ejemplo.eu' }],
    to: [{ email: 'yo@ejemplo.eu' }],
    cc: [],
    preview: `Texto ${id}`,
    receivedAt: `2026-11-05T${String(hora).padStart(2, '0')}:00:00Z`,
    keywords: { $seen: true },
    hasAttachment: false,
  };
}

beforeEach(() => {
  paginasPedidas.length = 0;
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = new URL(String(input), ORIGEN);
    const ruta = url.pathname;
    let emails: unknown = { emails: [], nextCursor: '' };
    if (ruta === '/api/emails' && url.searchParams.get('mailboxId') === 'in') {
      const cursor = url.searchParams.get('cursor') ?? '';
      paginasPedidas.push(cursor);
      // Dos páginas: «a» y «b» primero; «c», la más antigua, detrás del cursor.
      emails =
        cursor === 'p2'
          ? { emails: [correo('c', 10)], nextCursor: '' }
          : { emails: [correo('a', 12), correo('b', 11)], nextCursor: 'p2' };
    }
    const cuerpo: Record<string, unknown> = {
      '/api/mailboxes': {
        mailboxes: [
          { id: 'in', name: 'Inbox', role: 'inbox', unread: 0, totalEmails: 3 },
          { id: 'ar', name: 'Archive', role: 'archive', unread: 0 },
        ],
      },
      '/api/emails': emails,
      '/api/changes': { changed: [], destroyed: [], state: 's1', hasMore: false },
      '/api/trusted-senders': { senders: [] },
      '/api/body': { text: 'Hola' },
    };
    const vacio = { items: [], labels: [], aliases: [], sharedMailboxes: [], primary: '' };
    return new Response(JSON.stringify(cuerpo[ruta] ?? vacio), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
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

const usuario = { sub: 'u', email: 'yo@ejemplo.eu', name: 'Yo', roles: [] } as unknown as SytueUser;

function montar(): HTMLElement {
  const host = document.body.appendChild(document.createElement('div'));
  hosts.push(host);
  render(
    <I18nProvider locale="es">
      <ToastProvider>
        <MailApp
          user={usuario}
          hubBaseUrl="https://hub.example.eu"
          idBaseUrl="https://id.example.eu"
          locale="es"
        />
      </ToastProvider>
    </I18nProvider>,
    host,
  );
  return host;
}

const fila = (host: HTMLElement, id: string): HTMLElement | undefined =>
  [...host.querySelectorAll<HTMLElement>('[role="option"]')].find((li) =>
    li.textContent?.includes(`Asunto ${id}`),
  );
const abierta = (host: HTMLElement): string | undefined =>
  [...host.querySelectorAll('h2')].find((h) => h.textContent?.startsWith('Asunto '))?.textContent ??
  undefined;
const boton = (host: HTMLElement, nombre: string): HTMLButtonElement | undefined =>
  host.querySelector<HTMLButtonElement>(`button[aria-label="${nombre}"]`) ?? undefined;
const MAS_RECIENTE = 'Conversación más reciente';
const MAS_ANTIGUA = 'Conversación más antigua';

test('los botones del lector pasan de conversación y se apagan en la primera', async () => {
  const host = montar();
  await hasta(() => fila(host, 'a') !== undefined);
  fila(host, 'a')?.click();
  await hasta(() => abierta(host) === 'Asunto a' && boton(host, MAS_RECIENTE) !== undefined);
  // La más reciente de todas no tiene nada por encima.
  expect(boton(host, MAS_RECIENTE)?.disabled).toBe(true);
  expect(boton(host, MAS_ANTIGUA)?.disabled).toBe(false);
  // El atajo, en la pista.
  expect(boton(host, MAS_ANTIGUA)?.title).toBe(`${MAS_ANTIGUA} (J)`);

  boton(host, MAS_ANTIGUA)?.click();
  await hasta(() => abierta(host) === 'Asunto b');
  expect(boton(host, MAS_RECIENTE)?.disabled).toBe(false);

  boton(host, MAS_RECIENTE)?.click();
  await hasta(() => abierta(host) === 'Asunto a');
});

test('en la última cargada, «Más antigua» pide la página siguiente y abre la que llega', async () => {
  const host = montar();
  await hasta(() => fila(host, 'b') !== undefined);
  fila(host, 'b')?.click();
  await hasta(() => abierta(host) === 'Asunto b' && boton(host, MAS_ANTIGUA) !== undefined);
  // Quedan páginas: el botón sigue encendido aunque «b» sea la última cargada.
  expect(boton(host, MAS_ANTIGUA)?.disabled).toBe(false);

  boton(host, MAS_ANTIGUA)?.click();
  await hasta(() => paginasPedidas.includes('p2'));
  await hasta(() => abierta(host) === 'Asunto c');
  // Ya no queda nada debajo de la más antigua.
  await hasta(() => boton(host, MAS_ANTIGUA)?.disabled === true);
});
