/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La página de ajustes de Mail (design.md §8.4). Antes eran ocho entradas sueltas en
 * una sección plegada del lateral, cada una abriendo su diálogo sin decir qué cambiaba.
 * Ahora el lateral tiene UNA puerta, «Ajustes», que abre una página por secciones con
 * el estado de lo que se sabe sin abrir nada, y cada fila abre su editor de siempre.
 * Ejecutar: `bun test`.
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
let pedidas: Record<string, number> = {};
let vacaciones: Record<string, unknown> = { enabled: false, subject: '', body: '' };
let firma = '';
let undoSend = 10;
let falloAlGuardarUndo = false;
let lectura: Record<string, string> = {};
let confiados: string[] = [];
const escritas: { ruta: string; cuerpo: unknown }[] = [];
const abiertas: string[] = [];
const abrirOriginal = window.open;

beforeEach(() => {
  pedidas = {};
  vacaciones = { enabled: false, subject: '', body: '' };
  firma = '';
  undoSend = 10;
  falloAlGuardarUndo = false;
  lectura = { autoAdvance: 'older', swipeRight: 'archive', swipeLeft: 'trash' };
  confiados = [];
  escritas.length = 0;
  abiertas.length = 0;
  window.open = ((url: string) => {
    abiertas.push(url);
    return null;
  }) as typeof window.open;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const ruta = new URL(String(input), ORIGEN).pathname;
    pedidas[ruta] = (pedidas[ruta] ?? 0) + 1;
    if (init?.method && init.method !== 'GET') {
      escritas.push({ ruta, cuerpo: init.body ? JSON.parse(String(init.body)) : null });
      if (ruta === '/api/undo-send' && falloAlGuardarUndo)
        return new Response('{"error":"mail_error"}', { status: 502 });
    }
    const cuerpo: Record<string, unknown> = {
      '/api/mailboxes': {
        mailboxes: [
          { id: 'in', name: 'Inbox', role: 'inbox', unread: 0 },
          { id: 'dr', name: 'Drafts', role: 'drafts', unread: 0 },
        ],
      },
      '/api/send': { ok: true, id: 'o1', status: 'held' },
      '/api/emails': { emails: [], nextCursor: '' },
      '/api/changes': { changed: [], destroyed: [], state: 's1', hasMore: false },
      '/api/vacation': vacaciones,
      '/api/signature': { signature: firma },
      '/api/undo-send': { seconds: undoSend },
      '/api/reading-prefs': lectura,
      '/api/trusted-senders': { senders: confiados },
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
  window.open = abrirOriginal;
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
  window.history.replaceState(null, '', '/mail/');
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

/** Un botón del lateral por su texto. */
function delLateral(host: HTMLElement, texto: string): HTMLElement | null {
  const nav = host.querySelector('nav');
  return (
    [...(nav?.querySelectorAll<HTMLElement>('button, a') ?? [])].find(
      (b) => b.textContent?.trim() === texto,
    ) ?? null
  );
}

const pagina = (host: HTMLElement): HTMLElement | null =>
  [...host.querySelectorAll<HTMLElement>('h2')].find((h) => h.textContent === 'Ajustes de Mail')
    ?.parentElement?.parentElement?.parentElement ?? null;

const fila = (host: HTMLElement, ajuste: string): HTMLElement | null =>
  host.querySelector<HTMLElement>(`[data-ajuste="${ajuste}"]`);

test('el lateral tiene una sola puerta a los ajustes, no las ocho entradas', async () => {
  const host = montar();
  await hasta(() => delLateral(host, 'Ajustes') !== null);
  // Es una entrada que navega, no el rótulo de una sección que se despliega.
  expect(delLateral(host, 'Ajustes')?.hasAttribute('aria-expanded')).toBe(false);
  // Lo que antes colgaba del lateral ya no está ahí: vive en la página.
  for (const viejo of ['Firma', 'Reglas', 'Alias', 'Vacaciones', 'Clientes de correo']) {
    expect(delLateral(host, viejo)).toBeNull();
  }
});

test('«Ajustes» abre la página por secciones en lugar de la lista y el lector', async () => {
  vacaciones = { enabled: true, subject: 'Fuera', body: 'Vuelvo el lunes' };
  firma = 'Ana\nSytue';
  const host = montar();
  await hasta(() => delLateral(host, 'Ajustes') !== null);
  await hasta(() => host.querySelector('input[aria-label="Buscar en el correo"]') !== null);
  delLateral(host, 'Ajustes')?.click();
  await hasta(() => pagina(host) !== null);

  const p = pagina(host);
  // Las secciones, cada una con su encabezado.
  const secciones = [...(p?.querySelectorAll('section h2, [aria-label] h2') ?? [])].map(
    (h) => h.textContent,
  );
  expect(secciones).toEqual([
    'Escribir y responder',
    'Lectura',
    'Direcciones y acceso',
    'Organizar',
    'Correo firmado (S/MIME)',
    'Otros programas de correo',
  ]);
  // Las ocho filas de antes, todas en la página.
  for (const a of [
    'signature',
    'templates',
    'vacation',
    'undo-send',
    'aliases',
    'shared',
    'rules',
    'smime-anchor',
    'smime-identity',
    'mail-clients',
  ]) {
    expect(fila(host, a)).not.toBeNull();
  }
  // La lista no está: la página ocupa su sitio.
  expect(host.querySelector('input[aria-label="Buscar en el correo"]')).toBeNull();
  // El estado, sin abrir nada: la firma está puesta y la autorespuesta responde.
  await hasta(() => fila(host, 'vacation')?.textContent?.includes('Respondiendo ahora') ?? false);
  expect(fila(host, 'signature')?.textContent).toContain('Configurada');
  expect(delLateral(host, 'Ajustes')?.getAttribute('aria-current')).toBe('page');
  // Y es la ÚNICA: «Recibidos» no sigue diciendo que se está en él.
  expect(host.querySelectorAll('nav [aria-current="page"]')).toHaveLength(1);
});

test('cada fila abre su editor, y al cerrarlo la página vuelve a leer su estado', async () => {
  const host = montar();
  await hasta(() => delLateral(host, 'Ajustes') !== null);
  delLateral(host, 'Ajustes')?.click();
  await hasta(() => fila(host, 'vacation')?.textContent?.includes('Desactivada') ?? false);
  expect(fila(host, 'signature')?.textContent).toContain('Sin configurar');

  const editar = fila(host, 'vacation')?.querySelector('button');
  // El nombre accesible dice QUÉ se edita: «Editar» solo se repite en varias filas.
  expect(editar?.getAttribute('aria-label')).toBe('Editar: Vacaciones');
  const antes = pedidas['/api/vacation'] ?? 0;
  editar?.click();
  await hasta(() => document.querySelector('[role="dialog"]') !== null);
  expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
    'Autorespuesta de vacaciones',
  );

  // Se enciende en el diálogo (otra pestaña, otro dispositivo: da igual) y se cierra.
  vacaciones = { enabled: true, subject: 'Fuera', body: '' };
  const cancelar = [...document.querySelectorAll<HTMLElement>('[role="dialog"] button')].find(
    (b) => b.textContent?.trim() === 'Cancelar',
  );
  cancelar?.click();
  await hasta(() => document.querySelector('[role="dialog"]') === null);
  await hasta(() => fila(host, 'vacation')?.textContent?.includes('Respondiendo ahora') ?? false);
  expect(pedidas['/api/vacation']).toBeGreaterThan(antes + 1);

  // «Clientes de correo» lleva a las contraseñas de aplicación de Sytue ID.
  fila(host, 'mail-clients')?.querySelector('button')?.click();
  expect(abiertas).toEqual(['https://id.example.eu/account#app-passwords']);
});

test('«Plantillas de respuesta» abre su diálogo, que lee las de la cuenta', async () => {
  const host = montar();
  await hasta(() => delLateral(host, 'Ajustes') !== null);
  delLateral(host, 'Ajustes')?.click();
  await hasta(() => fila(host, 'templates') !== null);
  const gestionar = fila(host, 'templates')?.querySelector('button');
  expect(gestionar?.getAttribute('aria-label')).toBe('Gestionar: Plantillas de respuesta');
  const antes = pedidas['/api/templates'] ?? 0;
  gestionar?.click();
  await hasta(() => document.querySelector('[role="dialog"]') !== null);
  expect(document.querySelector('[role="dialog"]')?.textContent).toContain('Tus plantillas');
  await hasta(() => (pedidas['/api/templates'] ?? 0) > antes);
});

test('elegir una carpeta en el lateral cierra la página y vuelve al correo', async () => {
  const host = montar();
  await hasta(() => delLateral(host, 'Recibidos') !== null);
  delLateral(host, 'Ajustes')?.click();
  await hasta(() => pagina(host) !== null);
  delLateral(host, 'Recibidos')?.click();
  await hasta(() => pagina(host) === null);
  expect(host.querySelector('input[aria-label="Buscar en el correo"]')).not.toBeNull();
});

/** El selector de la ventana de deshacer, en su fila. */
const selectorUndo = (host: HTMLElement): HTMLSelectElement | null =>
  fila(host, 'undo-send')?.querySelector('select') ?? null;

function elegir(sel: HTMLSelectElement, valor: string): void {
  sel.value = valor;
  sel.dispatchEvent(new Event('change', { bubbles: true }));
}

test('la ventana de deshacer enseña la de la cuenta y se guarda al elegir otra', async () => {
  undoSend = 20;
  const host = montar();
  await hasta(() => delLateral(host, 'Ajustes') !== null);
  delLateral(host, 'Ajustes')?.click();
  await hasta(() => selectorUndo(host)?.value === '20');

  const sel = selectorUndo(host)!;
  expect(sel.disabled).toBe(false);
  expect(sel.getAttribute('aria-label')).toBe('Deshacer el envío');
  // Las cuatro opciones que acepta el servicio, ni una más.
  expect([...sel.options].map((o) => o.value)).toEqual(['5', '10', '20', '30']);
  expect(fila(host, 'undo-send')?.textContent).toContain('pulsar Enviar');

  elegir(sel, '30');
  await hasta(() => escritas.some((e) => e.ruta === '/api/undo-send'));
  expect(escritas.find((e) => e.ruta === '/api/undo-send')?.cuerpo).toEqual({ seconds: 30 });
  await hasta(() => document.body.textContent?.includes('Ventana de deshacer guardada') ?? false);
  expect(selectorUndo(host)?.value).toBe('30');
});

test('si el servidor no guarda la ventana, el selector vuelve a la de antes y avisa', async () => {
  falloAlGuardarUndo = true;
  const host = montar();
  await hasta(() => delLateral(host, 'Ajustes') !== null);
  delLateral(host, 'Ajustes')?.click();
  await hasta(() => selectorUndo(host)?.value === '10');

  elegir(selectorUndo(host)!, '5');
  await hasta(
    () => document.body.textContent?.includes('No se pudo guardar la ventana de deshacer') ?? false,
  );
  await hasta(() => selectorUndo(host)?.value === '10');
});

test('«Enviar» retiene el correo la ventana que tiene la cuenta, no una fija', async () => {
  undoSend = 30;
  window.history.replaceState(null, '', '/mail/?action=compose&to=luis%40example.com&subject=Hola');
  montar();
  await hasta(() => (pedidas['/api/undo-send'] ?? 0) > 0);
  const enviar = (): HTMLButtonElement | undefined =>
    [...document.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')].find(
      (b) => b.textContent?.trim() === 'Enviar',
    );
  await hasta(() => enviar() !== undefined && (pedidas['/api/mailboxes'] ?? 0) > 0);
  await new Promise((r) => setTimeout(r, 50));
  enviar()?.click();
  await hasta(() => escritas.some((e) => e.ruta === '/api/send'));
  expect(escritas.find((e) => e.ruta === '/api/send')?.cuerpo).toMatchObject({ undoSeconds: 30 });
  // Y el aviso con «Deshacer» dura esa misma ventana.
  await hasta(() => document.body.textContent?.includes('Se enviará en 30 s.') ?? false);
});

test('Lectura: el auto-avance y los gestos enseñan los de la cuenta y se guardan al cambiarlos', async () => {
  lectura = { autoAdvance: 'newer', swipeRight: 'read', swipeLeft: 'trash' };
  const host = montar();
  await hasta(() => delLateral(host, 'Ajustes') !== null);
  delLateral(host, 'Ajustes')?.click();
  const sel = (a: string) => fila(host, a)?.querySelector('select') ?? null;
  await hasta(() => sel('auto-advance')?.value === 'newer');
  expect(sel('swipe-right')?.value).toBe('read');
  expect(sel('swipe-left')?.value).toBe('trash');
  expect([...sel('auto-advance')!.options].map((o) => o.textContent)).toEqual([
    'Abrir la siguiente (más antigua)',
    'Abrir la anterior (más reciente)',
    'Volver a la lista',
  ]);
  expect([...sel('swipe-left')!.options].map((o) => o.value)).toEqual([
    'archive',
    'trash',
    'read',
    'none',
  ]);

  elegir(sel('swipe-left')!, 'none');
  await hasta(() => escritas.some((e) => e.ruta === '/api/reading-prefs'));
  // Se guardan las tres: el servicio las reemplaza juntas.
  expect(escritas.find((e) => e.ruta === '/api/reading-prefs')?.cuerpo).toEqual({
    autoAdvance: 'newer',
    swipeRight: 'read',
    swipeLeft: 'none',
  });
  await hasta(() => document.body.textContent?.includes('Ajustes de lectura guardados') ?? false);
});

test('Lectura: los remitentes de confianza se listan y se pueden quitar', async () => {
  confiados = ['boletin@tienda.eu', 'news@club.eu'];
  const host = montar();
  await hasta(() => delLateral(host, 'Ajustes') !== null);
  delLateral(host, 'Ajustes')?.click();
  await hasta(() => fila(host, 'trusted-senders')?.textContent?.includes('news@club.eu') ?? false);
  const quitar = fila(host, 'trusted-senders')?.querySelector<HTMLButtonElement>(
    'button[aria-label="Dejar de confiar en boletin@tienda.eu"]',
  );
  expect(quitar?.textContent).toBe('Quitar');
  quitar?.click();
  await hasta(() => escritas.some((e) => e.ruta === '/api/trusted-senders'));
  expect(escritas.find((e) => e.ruta === '/api/trusted-senders')?.cuerpo).toEqual({
    address: 'boletin@tienda.eu',
    remove: true,
  });
  await hasta(
    () => !(fila(host, 'trusted-senders')?.textContent?.includes('boletin@tienda.eu') ?? true),
  );
  expect(fila(host, 'trusted-senders')?.textContent).toContain('news@club.eu');
});

test('Lectura: sin remitentes de confianza dice cómo añadirlos', async () => {
  const host = montar();
  await hasta(() => delLateral(host, 'Ajustes') !== null);
  delLateral(host, 'Ajustes')?.click();
  await hasta(() => fila(host, 'trusted-senders')?.textContent?.includes('Aún no hay') ?? false);
});
