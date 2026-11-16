/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Auto-avance y gestos en la app entera (D-MAIL-08): archivar la conversación
 * abierta abre la siguiente (la más antigua) en vez de volver a la lista, salvo que
 * la cuenta prefiera otra cosa; y deslizar una fila con el dedo hace lo que la
 * cuenta eligió (por defecto, a la izquierda, a la papelera). Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
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
const escritas: { ruta: string; cuerpo: Record<string, unknown> }[] = [];
let lectura: Record<string, string> = {};
let prefsLeidas = 0;

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
  escritas.length = 0;
  prefsLeidas = 0;
  lectura = { autoAdvance: 'older', swipeRight: 'archive', swipeLeft: 'trash' };
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), ORIGEN);
    const ruta = url.pathname;
    if (ruta === '/api/reading-prefs') prefsLeidas++;
    if (init?.method && init.method !== 'GET')
      escritas.push({ ruta, cuerpo: init.body ? JSON.parse(String(init.body)) : {} });
    const cuerpo: Record<string, unknown> = {
      '/api/mailboxes': {
        mailboxes: [
          { id: 'in', name: 'Inbox', role: 'inbox', unread: 0, totalEmails: 3 },
          { id: 'ar', name: 'Archive', role: 'archive', unread: 0 },
          { id: 'tr', name: 'Trash', role: 'trash', unread: 0 },
        ],
      },
      // La lista llega de la más reciente a la más antigua.
      '/api/emails':
        url.searchParams.get('mailboxId') === 'in'
          ? { emails: [correo('a', 12), correo('b', 11), correo('c', 10)], nextCursor: '' }
          : { emails: [], nextCursor: '' },
      '/api/changes': { changed: [], destroyed: [], state: 's1', hasMore: false },
      '/api/reading-prefs': lectura,
      '/api/trusted-senders': { senders: [] },
      '/api/batch': { moved: 1 },
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
/** El asunto que enseña el lector (su encabezado). */
const abierta = (host: HTMLElement): string | undefined =>
  [...host.querySelectorAll('h2')].find((h) => h.textContent?.startsWith('Asunto '))?.textContent ??
  undefined;
const archivarEnLector = (host: HTMLElement): HTMLButtonElement | undefined =>
  [...host.querySelectorAll<HTMLButtonElement>('button[aria-label="Archivar"]')].find(
    (b) => !b.closest('[role="option"]'),
  );

test('archivar la conversación abierta abre la siguiente, la más antigua', async () => {
  const host = montar();
  await hasta(() => fila(host, 'b') !== undefined);
  fila(host, 'b')?.click();
  await hasta(() => abierta(host) === 'Asunto b' && archivarEnLector(host) !== undefined);
  archivarEnLector(host)?.click();
  await hasta(() => escritas.some((e) => e.ruta === '/api/batch'));
  expect(escritas.find((e) => e.ruta === '/api/batch')?.cuerpo).toEqual({
    ids: ['b'],
    moveTo: 'ar',
  });
  await hasta(() => abierta(host) === 'Asunto c');
  expect(fila(host, 'b')).toBeUndefined();
});

test('con «Volver a la lista» archivar cierra el lector', async () => {
  lectura = { autoAdvance: 'list', swipeRight: 'archive', swipeLeft: 'trash' };
  const host = montar();
  // Que las preferencias de la cuenta ya estén leídas antes de archivar.
  await hasta(() => fila(host, 'b') !== undefined && prefsLeidas > 0);
  await new Promise((r) => setTimeout(r, 30));
  fila(host, 'b')?.click();
  await hasta(() => abierta(host) === 'Asunto b' && archivarEnLector(host) !== undefined);
  archivarEnLector(host)?.click();
  await hasta(() => escritas.some((e) => e.ruta === '/api/batch'));
  await hasta(() => abierta(host) === undefined);
});

test('deslizar una fila a la izquierda la manda a la papelera', async () => {
  const host = montar();
  await hasta(() => fila(host, 'a') !== undefined);
  const li = fila(host, 'a')!;
  const puntero = (tipo: string, x: number) =>
    act(() => {
      li.dispatchEvent(
        new PointerEvent(tipo, {
          bubbles: true,
          cancelable: true,
          pointerId: 3,
          pointerType: 'touch',
          isPrimary: true,
          clientX: x,
          clientY: 40,
        }),
      );
    });
  puntero('pointerdown', 300);
  puntero('pointermove', 280);
  expect(li.textContent).toContain('Mover a la papelera');
  puntero('pointermove', 150);
  puntero('pointerup', 150);
  await hasta(() => escritas.some((e) => e.ruta === '/api/batch'));
  expect(escritas.find((e) => e.ruta === '/api/batch')?.cuerpo).toEqual({
    ids: ['a'],
    moveTo: 'tr',
  });
  await hasta(() => fila(host, 'a') === undefined);
});
