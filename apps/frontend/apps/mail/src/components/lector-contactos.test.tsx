/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Añadir a Contactos» / «Ver en Contactos» en el menú «⋯» del lector (MAIL-12):
 * añadir crea la ficha UNA vez, avisa con la acción «Ver» y la entrada pasa a «Ver en
 * Contactos»; un remitente que ya está abre su ficha. Ni en Spam, ni a un no-reply, ni
 * a uno mismo. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { EmailSummary } from '@sytue/api-clients';

if (typeof document === 'undefined') GlobalRegistrator.register();
process.env.BASE_URL ??= '/mail/';

const { ThemeProvider, ToastProvider } = await import('@sytue/ui');
const { MessageReader } = await import('./MessageReader.js');
const { groupByThread } = await import('./utils.js');

const ID = '0b6f6c2e-6a39-4bd2-9a57-4f2d3c9f5a10';

/** Lo que ha pedido el lector y la libreta simulada (una dirección → un id). */
let llamadas: { metodo: string; url: string; cuerpo: unknown }[] = [];
let libreta = new Map<string, string>();

globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
  const u = new URL(String(url), 'https://mail.sytue.eu');
  const metodo = init?.method ?? 'GET';
  const cuerpo = init?.body ? (JSON.parse(String(init.body)) as { email: string }) : null;
  llamadas.push({ metodo, url: u.pathname + u.search, cuerpo });
  if (u.pathname === '/api/contacts' && metodo === 'GET') {
    return Response.json({ id: libreta.get(u.searchParams.get('email') ?? '') ?? null });
  }
  if (u.pathname === '/api/contacts' && metodo === 'POST' && cuerpo) {
    const ya = libreta.get(cuerpo.email);
    libreta.set(cuerpo.email, ya ?? ID);
    return Response.json({ id: ya ?? ID, created: !ya }, { status: ya ? 200 : 201 });
  }
  return Response.json({});
}) as unknown as typeof fetch;

const abiertas: string[] = [];
window.open = ((u?: string | URL) => {
  abiertas.push(String(u));
  return null;
}) as typeof window.open;

afterEach(() => {
  document.body.innerHTML = '';
  llamadas = [];
  libreta = new Map();
  abiertas.length = 0;
});

function correo(de: string): EmailSummary {
  return {
    id: 'm1',
    threadId: 'h1',
    subject: 'Presupuesto',
    from: [{ name: 'Ana Ruiz', email: de }],
    to: [{ email: 'yo@sytue.eu' }],
    cc: [],
    preview: 'Hola',
    receivedAt: '2026-11-05T10:00:00Z',
    keywords: { $seen: true },
    hasAttachment: false,
  };
}

const nada = (): void => {};

async function montar(de: string, opts: { enSpam?: boolean } = {}): Promise<HTMLElement> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(
      <ThemeProvider>
        <ToastProvider>
          <MessageReader
            locale="es"
            conversation={groupByThread([correo(de)])[0] ?? null}
            onReply={nada}
            onCompose={nada}
            onToggleFlag={nada}
            onMarkUnread={nada}
            onArchive={null}
            onTrash={null}
            onSpam={null}
            onNotSpam={null}
            enSpam={opts.enSpam ?? false}
            onPurge={null}
            onMove={nada}
            moveTargets={[]}
            labels={[]}
            onToggleLabel={nada}
            propias={['yo@sytue.eu', 'alias@sytue.eu']}
          />
        </ToastProvider>
      </ThemeProvider>,
      host,
    );
  });
  // La pregunta a Contacts sale en un efecto: que conteste antes de mirar el menú.
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  return host;
}

function opcion(host: HTMLElement, texto: string): HTMLElement | undefined {
  const mas = host.querySelector<HTMLButtonElement>('button[aria-label="Más acciones"]');
  // Un menú que siguiera abierto de la vuelta anterior se cierra antes: el disparador
  // conmuta.
  if (document.querySelector('[role="menu"]')) act(() => mas?.click());
  act(() => mas?.click());
  return [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')].find((el) =>
    el.textContent?.includes(texto),
  );
}

test('añadir crea la ficha una sola vez, avisa con «Ver» y la entrada pasa a «Ver en Contactos»', async () => {
  const host = await montar('ana@example.eu');
  const anadir = opcion(host, 'Añadir a Contactos');
  expect(anadir).toBeDefined();
  await act(async () => {
    anadir?.click();
    await new Promise((r) => setTimeout(r, 0));
  });

  const altas = llamadas.filter((l) => l.metodo === 'POST');
  expect(altas).toEqual([
    {
      metodo: 'POST',
      url: '/api/contacts',
      cuerpo: { name: 'Ana Ruiz', email: 'ana@example.eu' },
    },
  ]);
  expect(document.body.textContent).toContain('Añadido a Contactos');
  const ver = [...document.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.textContent === 'Ver',
  );
  expect(ver).toBeDefined();
  act(() => ver?.click());
  expect(abiertas).toHaveLength(1);
  expect(abiertas[0]).toEndWith(`/contacts/${ID}`);

  // Segunda vuelta: ya no hay «Añadir», sino «Ver», y no se crea nada más.
  expect(opcion(host, 'Añadir a Contactos')).toBeUndefined();
  const verEn = opcion(host, 'Ver en Contactos');
  expect(verEn).toBeDefined();
  act(() => verEn?.click());
  expect(abiertas).toHaveLength(2);
  expect(llamadas.filter((l) => l.metodo === 'POST')).toHaveLength(1);
});

test('un remitente que ya está en la libreta ofrece «Ver en Contactos» de entrada', async () => {
  libreta.set('ana@example.eu', ID);
  const host = await montar('ana@example.eu');
  expect(llamadas.some((l) => l.url === '/api/contacts?email=ana%40example.eu')).toBe(true);
  expect(opcion(host, 'Añadir a Contactos')).toBeUndefined();
  const ver = opcion(host, 'Ver en Contactos');
  expect(ver).toBeDefined();
  act(() => ver?.click());
  expect(abiertas[0]).toEndWith(`/contacts/${ID}`);
});

test('ni en Spam, ni a un no-reply, ni a uno mismo: ni se ofrece ni se pregunta', async () => {
  for (const [de, enSpam] of [
    ['ana@example.eu', true],
    ['no-reply@banco.eu', false],
    ['Alias@sytue.eu', false],
  ] as const) {
    const host = await montar(de, { enSpam });
    expect(opcion(host, 'Añadir a Contactos')).toBeUndefined();
    expect(opcion(host, 'Ver en Contactos')).toBeUndefined();
    document.body.innerHTML = '';
  }
  expect(llamadas.filter((l) => l.url.startsWith('/api/contacts'))).toEqual([]);
});
