/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La sección «Cuentas de correo externas» del lateral (B4). Antes solo se recargaba
 * al cerrar el diálogo: al volver del proveedor, o tras conectar o desconectar, el
 * lateral seguía con la lista vieja detrás del diálogo, y un fallo de carga dejaba
 * la sección vacía sin decir por qué. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { useState } from 'preact/hooks';
import { act } from 'preact/test-utils';
import { en } from '../i18n/messages/en.js';
import type { VistaExterna } from './vistaExterna.js';

// Si otro fichero del mismo proceso ya registró happy-dom (sin URL, about:blank), las
// rutas relativas del cliente no casan con el fetch de abajo: se fija la URL igual
// que en ExternalMail.test.tsx. Fallaba según el orden en que Bun carga los ficheros.
if (typeof document === 'undefined') GlobalRegistrator.register({ url: 'https://sytue.test/mail' });
else
  (window as unknown as { happyDOM?: { setURL: (url: string) => void } }).happyDOM?.setURL(
    'https://sytue.test/mail',
  );

const nueva = {
  instance_id: 'nueva',
  provider: 'gmail',
  name: 'Gmail',
  address: 'ana@gmail.com',
  connected: true,
  services: ['mail'],
  capabilities: {},
};
const trabajo = { ...nueva, instance_id: 'trabajo', provider: 'imap', address: 'ana@trabajo.eu' };
const soloCalendario = {
  ...nueva,
  instance_id: 'calendario',
  address: 'ana@calendario.eu',
  services: ['calendar'],
};
let cuentas: unknown[] = [];
let fallar = false;
const pedidas: string[] = [];

beforeEach(() => {
  // En beforeEach y no al cargar: otro fichero de la suite pone su propio fetch.
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const ruta = url.pathname.replace(/^\/mail(?=\/api\/)/, '');
    const metodo = init?.method ?? 'GET';
    pedidas.push(`${metodo} ${ruta}`);
    const json = (body: unknown, status = 200) =>
      new Response(JSON.stringify(body), {
        status,
        headers: { 'content-type': 'application/json' },
      });
    if (metodo === 'DELETE') {
      const id = decodeURIComponent(ruta.split('/').pop() ?? '');
      cuentas = cuentas.filter((c) => (c as { instance_id: string }).instance_id !== id);
      return new Response(null, { status: 204 });
    }
    if (ruta.endsWith('/external-mail/summary')) {
      if (fallar) return json({ error: { code: 'internal' } }, 500);
      // Como el gateway: solo las cuentas con buzón, cada una con su estado.
      return json({
        items: cuentas
          .filter((c) => ((c as { services?: string[] }).services ?? ['mail']).includes('mail'))
          .map((c) => ({ state: 'ok', folders: [], ...(c as object) })),
      });
    }
    if (ruta.endsWith('/external-mail/accounts')) {
      if (fallar) return json({ error: { code: 'internal' } }, 500);
      return json({
        items: cuentas,
        providers: [{ provider: 'gmail', available: true, services: [] }],
      });
    }
    return json({ items: [] });
  }) as unknown as typeof fetch;
});

const { ExternalAccountsSection, MailboxesSection, useExternalAccounts } =
  await import('./ExternalAccountsSection.js');
const { arbolDeBuzones } = await import('./buzones.js');
const { ExternalMail } = await import('./ExternalMail.js');
const t = ((key: keyof typeof en, vars?: Record<string, string>) => {
  const texto = en[key];
  return typeof texto === 'string'
    ? texto.replace(/\{(\w+)\}/g, (marca, k: string) => vars?.[k] ?? marca)
    : texto;
}) as unknown as Parameters<typeof ExternalMail>[0]['t'];
/** El triángulo de «Todos los buzones de entrada», con su buzón en el rótulo. */
const cuentasDeEntradas = t('nav.accountsIn', { mailbox: en['nav.allInboxes'] });

afterEach(() => {
  render(null, document.body);
  document.body.innerHTML = '';
  pedidas.length = 0;
  cuentas = [];
  fallar = false;
  sessionStorage.clear();
  window.history.replaceState(null, '', '/mail');
  abiertas.length = 0;
  elegidas.length = 0;
});

const carpetasGmail: { id: string; name: string; role?: string; unread?: number }[] = [
  { id: 'TRASH', name: 'TRASH', role: 'trash' },
  { id: 'Label_1', name: 'Facturas' },
  { id: 'INBOX', name: 'INBOX', role: 'inbox', unread: 4 },
  { id: 'SENT', name: 'SENT', role: 'sent' },
];

/** Lo que el lateral pidió abrir: en el panel (cuenta/carpeta, «unificado:rol», «cuenta:») o el diálogo. */
const abiertas: string[] = [];
const elegidas: string[] = [];
/** Las carpetas de Sytue del lateral: su Entrada con dos sin leer. */
const sytue = [{ id: 'sytue-inbox', name: 'Inbox', role: 'inbox', unreadEmails: 2 }];

/** Lo mismo que hace MailApp con la sección y el diálogo. */
function Lateral(): preact.JSX.Element {
  const [open, setOpen] = useState(() =>
    new URLSearchParams(window.location.search).has('connect-mail'),
  );
  const external = useExternalAccounts();
  const arbol = arbolDeBuzones('en', sytue, external.summary);
  const [vista, setVista] = useState<VistaExterna | null>(null);
  // Una carpeta se abre en el panel principal; el diálogo, solo para volver a entrar.
  const abrir = (instancia: string, carpeta: string) => {
    abiertas.push(`${instancia}/${carpeta}`);
    setVista({ tipo: 'carpeta', cuenta: instancia, carpeta });
  };
  const reconectar = (instancia: string) => {
    abiertas.push(`${instancia}/`);
    setOpen(true);
  };
  return (
    <div>
      <nav>
        <MailboxesSection
          t={t}
          mailboxes={arbol.unificados}
          accounts={arbol.cuentas}
          sytueFolders={sytue}
          sytueTitle="ana@sytue.eu"
          selectedMailbox={vista ? undefined : 'sytue-inbox'}
          selectedExternal={vista}
          onSelectSytue={(id) => elegidas.push(id)}
          onOpenFolder={abrir}
          onOpenUnified={(rol) => {
            abiertas.push(`unificado:${rol}`);
            setVista({ tipo: 'unificado', rol });
          }}
        />
        <ExternalAccountsSection
          t={t}
          accounts={arbol.cuentas}
          error={external.error}
          selectedExternal={vista}
          onRetry={external.retry}
          onOpen={reconectar}
          onOpenFolder={abrir}
          onAdd={() => setOpen(true)}
        />
      </nav>
      {open ? (
        <ExternalMail
          t={t}
          onAccounts={external.update}
          onOpenAccount={(cuenta) => {
            abiertas.push(`cuenta:${cuenta.instance_id}`);
            setOpen(false);
            // Como MailApp: su Entrada, que se elige al leer sus carpetas.
            setVista({ tipo: 'carpeta', cuenta: cuenta.instance_id, carpeta: '' });
          }}
          onClose={() => setOpen(false)}
        />
      ) : null}
    </div>
  );
}

async function esperar(): Promise<void> {
  for (let i = 0; i < 4; i++)
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
}
async function montar(search = ''): Promise<void> {
  window.history.replaceState(null, '', `/mail${search}`);
  await act(async () => {
    render(<Lateral />, document.body);
  });
  await esperar();
}
function lateral(): string {
  return document.querySelector('nav')?.textContent ?? '';
}
async function pulsar(boton: HTMLElement | undefined | null): Promise<void> {
  if (!boton) throw new Error('no está el botón');
  await act(async () => boton.click());
  await esperar();
}
function botones(texto: string): HTMLButtonElement[] {
  return [...document.querySelectorAll('button')].filter((b) => b.textContent?.trim() === texto);
}
const cuentasPedidas = () => pedidas.filter((p) => p === 'GET /api/external-mail/accounts');
const resumenesPedidos = () => pedidas.filter((p) => p === 'GET /api/external-mail/summary');

test('el lateral lista las cuentas con buzón y siempre ofrece «Añadir cuenta de correo»', async () => {
  cuentas = [trabajo, soloCalendario];
  await montar();
  expect(lateral()).toContain('ana@trabajo.eu');
  expect(lateral()).not.toContain('ana@calendario.eu');
  // «Añadir» es la última fila de la sección, también sin ninguna cuenta.
  const filas = [...(document.querySelector('nav')?.querySelectorAll('button') ?? [])];
  expect(filas.at(-1)?.textContent).toContain(en['externalMail.add']);
  // El lateral lee el resumen (carpetas y no leídos de cada cuenta), no la lista del
  // diálogo. VISTO EN ROJO (2026-11-06) con `accounts()` en el hook.
  expect(resumenesPedidos()).toHaveLength(1);
  expect(cuentasPedidas()).toHaveLength(0);
});

// Una cuenta de iCloud es una conexión IMAP con los servidores de Apple: en el
// lateral lleva la nube, no la persona de las demás. Fyne hace lo mismo (menu.go).
test('una cuenta de iCloud lleva la nube en el lateral', async () => {
  const servidor = (host: string) => ({ host, port: 993, username: '', security: 'tls' });
  cuentas = [
    trabajo,
    {
      ...trabajo,
      instance_id: 'icloud',
      address: 'ana@icloud.com',
      configuration: {
        incoming: servidor('imap.mail.me.com'),
        outgoing: servidor('smtp.mail.me.com'),
      },
    },
  ];
  await montar();
  const icono = (direccion: string) =>
    [...(document.querySelector('nav')?.querySelectorAll('button') ?? [])]
      .find((b) => b.textContent?.includes(direccion))
      ?.querySelector('path')
      ?.getAttribute('d');
  expect(icono('ana@icloud.com')).toBe('M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z');
  expect(icono('ana@trabajo.eu')).not.toBe(icono('ana@icloud.com'));
});

// VISTO EN ROJO (2026-11-06): con el `.catch(() => undefined)` de antes la sección
// se quedaba con «Añadir» solo y sin aviso.
test('si las cuentas no cargan, la sección lo dice y deja reintentar', async () => {
  fallar = true;
  await montar();
  const aviso = document.querySelector('nav [role="alert"]');
  expect(aviso?.textContent).toContain(en['externalMail.accountsError']);
  fallar = false;
  cuentas = [trabajo];
  await pulsar(botones(en['common.retry'])[0]);
  expect(document.querySelector('nav [role="alert"]')).toBeNull();
  expect(lateral()).toContain('ana@trabajo.eu');
});

// Fase B, punto 4: la cuenta que acaba de conectarse se abre en el panel principal y
// el diálogo se cierra. VISTO EN ROJO (2026-11-06) con el buzón del diálogo de antes:
// la cuenta se quedaba en un modal encima de la bandeja.
test('al volver del proveedor la cuenta se abre en el panel y sale en el lateral, pidiendo una vez', async () => {
  cuentas = [nueva];
  await montar('?connect-mail=1&instance=nueva');
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  expect(abiertas).toEqual(['cuenta:nueva']);
  expect(lateral()).toContain('ana@gmail.com');
  // El resumen que sale al montar ya trae la cuenta nueva: no se pide otra vez.
  expect(cuentasPedidas()).toHaveLength(1);
  expect(resumenesPedidos()).toHaveLength(1);
});

// VISTO EN ROJO (2026-11-06): sin `staleIn` en `update`, la cuenta conectada con el
// diálogo abierto no salía en el lateral hasta recargar la página.
test('una cuenta que aparece en la lista del diálogo pide el resumen otra vez', async () => {
  cuentas = [trabajo];
  await montar();
  expect(lateral()).not.toContain('ana@gmail.com');
  cuentas = [trabajo, nueva];
  await pulsar(botones(en['externalMail.add'])[0]);
  expect(resumenesPedidos()).toHaveLength(2);
  expect(lateral()).toContain('ana@gmail.com');
  // Volver a cargar la misma lista no vuelve a pedirlo, ni cerrar el diálogo.
  await pulsar(
    document.querySelector<HTMLButtonElement>('[role="dialog"] button[aria-label="Close"]'),
  );
  expect(resumenesPedidos()).toHaveLength(2);
  await pulsar(botones(en['externalMail.add'])[0]);
  expect(resumenesPedidos()).toHaveLength(2);
});

test('desconectar quita la cuenta del lateral sin cerrar el diálogo', async () => {
  cuentas = [nueva, trabajo];
  await montar();
  await pulsar(botones(en['externalMail.add'])[0]);
  expect(lateral()).toContain('ana@gmail.com');
  const fila = document.querySelector<HTMLElement>('[role="dialog"] [data-instance="nueva"]');
  await pulsar(
    [...(fila?.querySelectorAll('button') ?? [])].find(
      (b) => b.textContent?.trim() === en['externalMail.disconnect'],
    ),
  );
  // La confirmación es el último diálogo abierto.
  const confirmar = [...document.querySelectorAll('[role="dialog"], [role="alertdialog"]')]
    .at(-1)
    ?.querySelectorAll('button');
  await pulsar(
    [...(confirmar ?? [])].find((b) => b.textContent?.trim() === en['externalMail.disconnect']),
  );
  expect(pedidas).toContain('DELETE /api/external-mail/nueva');
  expect(document.querySelector('[role="dialog"]')).not.toBeNull();
  expect(lateral()).not.toContain('ana@gmail.com');
  expect(lateral()).toContain('ana@trabajo.eu');
});

// El diálogo ya no lee correo —las carpetas se abren en el panel principal—, así que
// cerrarlo no cambia ningún contador: no se vuelve a pedir nada. Los contadores los
// refresca el panel al leer o mover (`settle`). VISTO EN ROJO (2026-11-06) con el
// `settle` al cerrar que pedía el buzón del diálogo: un resumen por cada cierre.
test('cerrar el diálogo no vuelve a pedir las cuentas ni el resumen', async () => {
  cuentas = [{ ...nueva, folders: carpetasGmail }];
  await montar('?connect-mail=1');
  expect(document.querySelector('[role="dialog"]')).not.toBeNull();
  const antes = cuentasPedidas().length;
  const resumenes = resumenesPedidos().length;
  await pulsar(
    document.querySelector<HTMLButtonElement>('[role="dialog"] button[aria-label="Close"]'),
  );
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  expect(cuentasPedidas()).toHaveLength(antes);
  expect(resumenesPedidos()).toHaveLength(resumenes);
});

/** La rama de una cuenta (o de un buzón unificado) en el lateral, por su rótulo. */
function rama(rotulo: string): HTMLButtonElement | undefined {
  return [...document.querySelectorAll<HTMLButtonElement>('nav button[aria-expanded]')].find(
    (b) => b.title === rotulo,
  );
}
/** Una fila del lateral por su rótulo, sea rama o no. */
function fila(rotulo: string): HTMLButtonElement | undefined {
  return [...document.querySelectorAll<HTMLButtonElement>('nav li > button')].find(
    (b) => b.title === rotulo,
  );
}
/** Los rótulos que cuelgan de una rama abierta, con su contador si lo llevan. */
function hijos(boton: HTMLButtonElement | undefined): string[] {
  const grupo = boton?.parentElement?.querySelector(':scope > ul[role="group"]');
  return [...(grupo?.querySelectorAll('button') ?? [])].map((b) => b.textContent ?? '');
}

// Fase B, punto 3: cada cuenta es una rama plegable con sus carpetas, como en Apple
// Mail. VISTO EN ROJO (2026-11-06) con la fila suelta de antes (sin `expanded` ni
// carpetas) y con `item.name` en vez de `mailboxLabel` («INBOX» en el lateral).
test('cada cuenta se despliega con sus carpetas traducidas, ordenadas y con sus no leídos', async () => {
  cuentas = [{ ...nueva, folders: carpetasGmail }];
  await montar();
  const cuenta = rama('ana@gmail.com');
  expect(cuenta?.getAttribute('aria-expanded')).toBe('true');
  expect(hijos(cuenta)).toEqual(['Inbox4', 'Sent', 'Trash', 'Facturas']);
  // Abierta, el contador lo lleva la Entrada; plegada, la cuenta.
  expect(cuenta?.textContent).toBe('ana@gmail.com');
  await pulsar(cuenta);
  expect(rama('ana@gmail.com')?.getAttribute('aria-expanded')).toBe('false');
  expect(hijos(rama('ana@gmail.com'))).toEqual([]);
  expect(rama('ana@gmail.com')?.textContent).toBe('ana@gmail.com4');
  // Plegar no abre nada: solo las carpetas abren el buzón.
  expect(abiertas).toEqual([]);
  await pulsar(rama('ana@gmail.com'));
  const enviados = [
    ...(rama('ana@gmail.com')?.parentElement?.querySelectorAll('ul button') ?? []),
  ].find((b) => b.textContent === 'Sent') as HTMLButtonElement | undefined;
  await pulsar(enviados);
  expect(abiertas).toEqual(['nueva/SENT']);
});

// Fase B, punto 7: la cuenta que se abre desde el diálogo —recién conectada o con
// «Abrir buzón»— se despliega aunque se hubiera plegado, para que su Entrada se vea
// elegida. VISTO EN ROJO (2026-11-07) sin el efecto de `reveal`.
test('abrir una cuenta desde el diálogo despliega su rama plegada', async () => {
  cuentas = [{ ...nueva, folders: carpetasGmail }];
  await montar();
  await pulsar(rama('ana@gmail.com'));
  expect(rama('ana@gmail.com')?.getAttribute('aria-expanded')).toBe('false');
  await pulsar(fila(en['externalMail.add']));
  await pulsar(botones(en['externalMail.open'])[0]);
  expect(abiertas).toEqual(['cuenta:nueva']);
  expect(rama('ana@gmail.com')?.getAttribute('aria-expanded')).toBe('true');
  // Y sigue siendo de la persona: plegarla otra vez la pliega.
  await pulsar(rama('ana@gmail.com'));
  expect(rama('ana@gmail.com')?.getAttribute('aria-expanded')).toBe('false');
});

// VISTO EN ROJO (2026-11-06): con el filtro de `connected` de antes, la cuenta que
// perdió el permiso desaparecía del lateral sin decir por qué.
test('una cuenta que perdió el inicio de sesión lo dice en su sitio y lleva a reconectarla', async () => {
  cuentas = [
    { ...nueva, connected: false, folders: carpetasGmail },
    { ...trabajo, state: 'unavailable' },
  ];
  await montar();
  expect(hijos(rama('ana@gmail.com'))).toEqual([en['externalMail.reconnectNeeded']]);
  expect(hijos(rama('ana@trabajo.eu'))).toEqual([en['externalMail.accountUnavailable']]);
  await pulsar(botones(en['externalMail.reconnectNeeded'])[0]);
  expect(abiertas).toEqual(['nueva/']);
  // La que no contestó vuelve a pedir el resumen.
  const antes = resumenesPedidos().length;
  await pulsar(botones(en['externalMail.accountUnavailable'])[0]);
  expect(resumenesPedidos()).toHaveLength(antes + 1);
});

// «Buzones», arriba: solo con alguna cuenta externa (con Sytue sola sería «Carpetas»
// dos veces), con la suma de no leídos y cada buzón desplegable en el de cada cuenta.
// VISTO EN ROJO (2026-11-06) sin el `accounts.length === 0` (salía sin cuentas) y
// abriendo la rama al revés (`useBranches(true)`).
test('«Buzones» junta la Entrada de Sytue y la de cada cuenta', async () => {
  await montar();
  expect(lateral()).not.toContain(en['nav.mailboxes']);
  render(null, document.body);
  cuentas = [{ ...nueva, folders: carpetasGmail }];
  await montar();
  expect(lateral()).toContain(en['nav.mailboxes']);
  const todas = fila(en['nav.allInboxes']);
  expect(todas?.textContent).toBe(`${en['nav.allInboxes']}6`);
  // Su triángulo, aparte de la fila (que abre la Entrada mezclada).
  expect(rama(cuentasDeEntradas)?.getAttribute('aria-expanded')).toBe('false');
  await pulsar(rama(cuentasDeEntradas));
  // Cada fuente con su contador, Sytue también: 2 + 4 es el 6 de la rama. VISTO EN
  // ROJO (2026-11-06) con la fila de Sytue sin `count`.
  expect(hijos(fila(en['nav.allInboxes']))).toEqual(['ana@sytue.eu2', 'ana@gmail.com4']);
  const fuentes = fila(en['nav.allInboxes'])?.parentElement?.querySelectorAll('ul button');
  // Marcada a la vista, pero la «página actual» es la de «Carpetas».
  expect(fuentes?.[0]?.className).toContain('sytue-nav-current');
  expect(fuentes?.[0]?.hasAttribute('aria-current')).toBe(false);
  await pulsar(fuentes?.[0] as HTMLButtonElement);
  expect(elegidas).toEqual(['sytue-inbox']);
  await pulsar(
    fila(en['nav.allInboxes'])?.parentElement?.querySelectorAll<HTMLButtonElement>('ul button')[1],
  );
  expect(abiertas).toEqual(['nueva/INBOX']);
  // Abierta en el panel, la Entrada de la cuenta va marcada en las dos secciones, y
  // la «página actual» es la de su cuenta.
  const marcadas = [...document.querySelectorAll<HTMLButtonElement>('nav .sytue-nav-current')];
  expect(marcadas.map((b) => b.title)).toEqual(['ana@gmail.com', en['folder.inbox']]);
  expect(marcadas.map((b) => b.getAttribute('aria-current'))).toEqual([null, 'page']);
  // Y Enviados, que Sytue no tiene en esta prueba, también sale: lo tiene la cuenta.
  expect(fila(en['nav.allSent'])).toBeDefined();
});

// Fase B, punto 5: «Todos los buzones de entrada» es un buzón en sí, como en Apple
// Mail: la fila abre la Entrada de todas las cuentas mezclada y su triángulo, aparte,
// pliega. VISTO EN ROJO (2026-11-06) con la fila que solo plegaba: no había forma de
// abrir la Entrada mezclada, y pulsarla dos veces la volvía a cerrar.
test('«Todos los buzones de entrada» abre la Entrada mezclada y se marca', async () => {
  cuentas = [{ ...nueva, folders: carpetasGmail }];
  await montar();
  const todas = fila(en['nav.allInboxes']);
  expect(todas?.hasAttribute('aria-expanded')).toBe(false);
  await pulsar(todas);
  expect(abiertas).toEqual(['unificado:inbox']);
  expect(fila(en['nav.allInboxes'])?.getAttribute('aria-current')).toBe('page');
  // Abrirla no la despliega: eso es su triángulo.
  expect(rama(cuentasDeEntradas)?.getAttribute('aria-expanded')).toBe('false');
});

// Fase B, punto 5 completo: «Todos los enviados» y «Todos los borradores» también son
// buzones en sí, con su triángulo aparte; Archivo, Spam y Papelera solo se despliegan
// (vaciar o borrar para siempre es de una carpeta, no de una mezcla).
// VISTO EN ROJO (2026-11-07): con `mailbox.rol === 'inbox'` de antes, «Todos los
// enviados» solo plegaba; con `esMezclado` devolviendo true para todo, «Toda la
// papelera» abría una mezcla; y comparando solo el `tipo`, «Todos los buzones de
// entrada» seguía marcado con los enviados a la vista. Deshecho.
test('«Todos los enviados» abre los Enviados mezclados; la papelera solo se despliega', async () => {
  cuentas = [
    { ...nueva, folders: [...carpetasGmail, { id: 'DRAFT', name: 'DRAFT', role: 'drafts' }] },
  ];
  await montar();
  const enviados = fila(en['nav.allSent']);
  expect(enviados?.hasAttribute('aria-expanded')).toBe(false);
  await pulsar(enviados);
  expect(abiertas).toEqual(['unificado:sent']);
  expect(fila(en['nav.allSent'])?.getAttribute('aria-current')).toBe('page');
  expect(fila(en['nav.allInboxes'])?.hasAttribute('aria-current')).toBe(false);
  // Su triángulo, aparte, despliega la carpeta de cada cuenta sin cambiar la vista.
  const triangulo = rama(t('nav.accountsIn', { mailbox: en['nav.allSent'] }));
  expect(triangulo?.getAttribute('aria-expanded')).toBe('false');
  await pulsar(triangulo);
  expect(hijos(fila(en['nav.allSent']))).toEqual(['ana@gmail.com']);
  expect(abiertas).toEqual(['unificado:sent']);
  await pulsar(fila(en['nav.allDrafts']));
  expect(abiertas).toEqual(['unificado:sent', 'unificado:drafts']);
  expect(fila(en['nav.allDrafts'])?.getAttribute('aria-current')).toBe('page');
  // La papelera: la fila entera pliega y despliega, y no abre nada.
  const papelera = rama(en['nav.allTrash']);
  expect(papelera?.getAttribute('aria-expanded')).toBe('false');
  await pulsar(papelera);
  expect(rama(en['nav.allTrash'])?.getAttribute('aria-expanded')).toBe('true');
  expect(abiertas).toEqual(['unificado:sent', 'unificado:drafts']);
});

// Con todas las cuentas pidiendo volver a iniciar sesión, ninguna aporta carpetas y
// «Buzones» sería «Carpetas» otra vez. VISTO EN ROJO (2026-11-06) con el
// `accounts.length === 0` de antes.
test('«Buzones» no sale si ninguna cuenta externa aporta carpetas', async () => {
  cuentas = [{ ...nueva, connected: false, folders: carpetasGmail }];
  await montar();
  expect(lateral()).toContain('ana@gmail.com');
  expect(lateral()).not.toContain(en['nav.mailboxes']);
});
