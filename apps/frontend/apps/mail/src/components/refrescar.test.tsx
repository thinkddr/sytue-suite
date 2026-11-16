/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tirar para actualizar en sitio: Mail absorbe `sytue:refresh` (el evento que emite
 * `usePullToRefresh` del shell) y vuelve a pedir la lista sin recargar la página,
 * con la conversación abierta donde estaba. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { EmailSummary } from '@sytue/api-clients';
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
const { fusionarPrimeraPagina } = await import('./utils.js');

function correo(id: string, asunto: string, fecha: string): EmailSummary {
  return {
    id,
    threadId: `h-${id}`,
    subject: asunto,
    from: [{ email: 'ana@ejemplo.eu' }],
    to: [{ email: 'yo@ejemplo.eu' }],
    cc: [],
    preview: '',
    receivedAt: fecha,
    keywords: { $seen: true },
    hasAttachment: false,
  };
}

// ---- La fusión de la primera página con lo ya bajado ----

const a = correo('a', 'A', '2026-11-05T10:00:00Z');
const b = correo('b', 'B', '2026-11-04T10:00:00Z');
const c = correo('c', 'C', '2026-11-03T10:00:00Z');
const d = correo('d', 'D', '2026-11-02T10:00:00Z');
const nuevo = correo('n', 'Nuevo', '2026-11-06T10:00:00Z');
const ids = (l: EmailSummary[]): string[] => l.map((e) => e.id);

test('si la página fresca cubre todo lo bajado, manda ella con su cursor', () => {
  const r = fusionarPrimeraPagina(
    { emails: [a, b], cursor: 'k1' },
    { emails: [nuevo, a, b], cursor: 'k2' },
  );
  expect(ids(r.emails)).toEqual(['n', 'a', 'b']);
  expect(r.cursor).toBe('k2');
});

test('lo ya bajado más allá de la página fresca se conserva con su cursor', () => {
  // Lo nuevo empuja a b, c y d fuera de la primera página: siguen en la lista, y el
  // cursor viejo (que empieza tras d) no los repite ni se salta nada.
  const r = fusionarPrimeraPagina(
    { emails: [a, b, c, d], cursor: 'k-viejo' },
    { emails: [nuevo, a], cursor: 'k-nuevo' },
  );
  expect(ids(r.emails)).toEqual(['n', 'a', 'b', 'c', 'd']);
  expect(r.cursor).toBe('k-viejo');
});

test('lo que cubre la página fresca lo dice ella: lo borrado desaparece', () => {
  const r = fusionarPrimeraPagina(
    { emails: [nuevo, a, b, c, d], cursor: 'k-viejo' },
    { emails: [a, c], cursor: 'k-nuevo' },
  );
  // «Nuevo» y b ya no están en el tramo que la página fresca cubre; d queda debajo.
  expect(ids(r.emails)).toEqual(['a', 'c', 'd']);
  expect(r.cursor).toBe('k-viejo');
});

test('sin más páginas en el servidor, la fresca es la carpeta entera', () => {
  const r = fusionarPrimeraPagina({ emails: [a, b, c], cursor: 'k' }, { emails: [a], cursor: '' });
  expect(ids(r.emails)).toEqual(['a']);
  expect(r.cursor).toBe('');
});

// ---- El gesto sobre la app montada ----

const hosts: HTMLElement[] = [];
const fetchOriginal = globalThis.fetch;
/** Lo que devuelve /api/emails en cada momento. */
let lista: EmailSummary[] = [];
/** Peticiones GET por ruta (sin la consulta). */
let pedidas: Record<string, number> = {};
let recargada = false;
/** Cuántas veces más ha de fallar /api/search antes de contestar. */
let fallosBusqueda = 0;
const recargaOriginal = window.location.reload;

beforeEach(() => {
  lista = [a, b];
  pedidas = {};
  recargada = false;
  fallosBusqueda = 0;
  try {
    window.location.reload = () => {
      recargada = true;
    };
  } catch {
    // happy-dom puede no dejar sustituirla: el test mira `defaultPrevented`.
  }
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = new URL(String(input), ORIGEN);
    const ruta = url.pathname;
    pedidas[ruta] = (pedidas[ruta] ?? 0) + 1;
    if (ruta === '/api/search' && fallosBusqueda > 0) {
      fallosBusqueda--;
      return new Response('{}', { status: 500, headers: { 'content-type': 'application/json' } });
    }
    const cuerpo: Record<string, unknown> = {
      '/api/mailboxes': { mailboxes: [{ id: 'in', name: 'Inbox', role: 'inbox', unread: 0 }] },
      '/api/emails': { emails: lista, nextCursor: '' },
      '/api/search': { emails: lista },
      '/api/thread': { emails: [] },
      '/api/changes': { changed: [], destroyed: [], state: 's1', hasMore: false },
    };
    // Lo demás (alias, etiquetas, compartidos, cuentas externas…) vacío pero con forma.
    const vacio = { items: [], labels: [], aliases: [], sharedMailboxes: [], primary: '' };
    return new Response(JSON.stringify(cuerpo[ruta] ?? vacio), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = fetchOriginal;
  try {
    window.location.reload = recargaOriginal;
  } catch {
    // Ver beforeEach.
  }
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
});

async function hasta(listo: () => boolean): Promise<void> {
  for (let i = 0; i < 300 && !listo(); i++) await new Promise((r) => setTimeout(r, 10));
  if (!listo()) throw new Error('la condición no se cumplió a tiempo');
}

const usuario = {
  sub: 'u',
  email: 'yo@ejemplo.eu',
  name: 'Yo',
  roles: [],
} as unknown as SytueUser;

function montar(): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
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

/** El asunto de la conversación abierta (el título del lector), o '' si no hay ninguna. */
const abierta = (host: HTMLElement): string =>
  host.querySelector('h2.sytue-h3')?.textContent?.trim() ?? '';

test('el gesto recarga la lista en sitio y deja abierta la conversación', async () => {
  // Se abre por enlace (`?thread=`, el del buscador de la suite): la lista es virtual
  // y en happy-dom, sin alturas, puede no pintar filas que pulsar.
  window.history.replaceState(null, '', '/mail/?thread=h-a');
  const host = montar();
  await hasta(() => abierta(host) === 'A');
  const antes = { emails: pedidas['/api/emails'] ?? 0, buzones: pedidas['/api/mailboxes'] ?? 0 };

  // En el servidor ha llegado un correo nuevo.
  lista = [nuevo, a, b];
  const esperas: Promise<unknown>[] = [];
  const ev = new CustomEvent('sytue:refresh', {
    cancelable: true,
    detail: { esperar: (p: Promise<unknown>) => esperas.push(p) },
  });
  window.dispatchEvent(ev);

  // Lo absorbe: el shell no recarga la página y el indicador gira hasta que llega.
  expect(ev.defaultPrevented).toBe(true);
  expect(esperas).toHaveLength(1);
  await Promise.all(esperas);
  expect(recargada).toBe(false);
  expect(pedidas['/api/emails']).toBe(antes.emails + 1);
  expect(pedidas['/api/mailboxes']).toBe(antes.buzones + 1);
  await hasta(() => host.textContent?.includes('Nuevo') ?? false);
  // La conversación sigue abierta.
  expect(abierta(host)).toBe('A');
});

test('con la búsqueda en error, el gesto es su «Reintentar» y quita el aviso', async () => {
  window.history.replaceState(null, '', '/mail/');
  const host = montar();
  await hasta(() => (pedidas['/api/emails'] ?? 0) > 0);
  fallosBusqueda = 1;
  lista = [nuevo];
  const campo = host.querySelector<HTMLInputElement>('input[aria-label="Buscar en el correo"]');
  if (!campo) throw new Error('sin campo de búsqueda');
  campo.value = 'nuevo';
  campo.dispatchEvent(new Event('input', { bubbles: true }));
  await hasta(() => (pedidas['/api/search'] ?? 0) === 1);
  await hasta(() => host.textContent?.includes('No se pudo cargar el correo') ?? false);

  const ev = new CustomEvent('sytue:refresh', {
    cancelable: true,
    detail: { esperar: () => undefined },
  });
  window.dispatchEvent(ev);
  expect(ev.defaultPrevented).toBe(true);
  // Sin la rama de error, la recarga en sitio traía los resultados pero dejaba el
  // aviso de error encima, sin enseñarlos.
  await hasta(() => host.textContent?.includes('Nuevo') ?? false);
  expect(host.textContent).not.toContain('No se pudo cargar el correo');
  expect(pedidas['/api/search']).toBe(2);
});
