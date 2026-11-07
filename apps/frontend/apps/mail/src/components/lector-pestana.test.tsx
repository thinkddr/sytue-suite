/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Crear evento» y «Compartir en Rooms» abren otra pestaña con el deep-link que
 * devuelve el Hub. Lo que falla no se ve: si la pestaña se abre DESPUÉS de esperar
 * al Hub, el navegador ya no la cuenta como fruto del clic y Safari la bloquea en
 * silencio. Por eso se monta el lector, se pulsa y se mira que la pestaña existe
 * antes de que el Hub conteste. Y los atajos «l»/«v» de MailApp encuentran los
 * disparadores que el lector pinta. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { EmailSummary } from '@sytue/api-clients';

if (typeof document === 'undefined') GlobalRegistrator.register();
// Lo que Astro pone en `import.meta.env` (en Bun, `process.env`): sin él no hay prefijo.
process.env.BASE_URL ??= '/mail/';

const { ThemeProvider } = await import('@sytue/ui');
const { MessageReader, disparadorDeAtajo } = await import('./MessageReader.js');
const { abrirTrasPeticion, groupByThread } = await import('./utils.js');

/** El Hub, que contesta cuando el test lo suelta. */
let soltarHub: (cuerpo: unknown) => void = () => {};
const intents: unknown[] = [];
globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
  const url = String(input);
  if (url.includes('/api/intent')) {
    intents.push(JSON.parse(String(init?.body ?? '{}')));
    const cuerpo = await new Promise((r) => (soltarHub = r));
    return new Response(JSON.stringify(cuerpo), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }
  return new Response(JSON.stringify({}), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  });
}) as unknown as typeof fetch;

interface PestanaFalsa {
  opener: unknown;
  cerrada: boolean;
  location: { href: string };
  close: () => void;
}
let pestanas: PestanaFalsa[] = [];
window.open = ((): PestanaFalsa => {
  const p: PestanaFalsa = {
    opener: window,
    cerrada: false,
    location: { href: 'about:blank' },
    close: () => {
      p.cerrada = true;
    },
  };
  pestanas.push(p);
  return p;
}) as unknown as typeof window.open;

const correo: EmailSummary = {
  id: 'm1',
  threadId: 'h1',
  subject: 'Presupuesto',
  from: [{ email: 'ana@ejemplo.eu' }],
  to: [{ email: 'yo@ejemplo.eu' }],
  cc: [],
  preview: 'Hola',
  receivedAt: '2026-11-05T10:00:00Z',
  keywords: { $seen: true },
  hasAttachment: false,
};

afterEach(() => {
  document.body.innerHTML = '';
  pestanas = [];
  intents.length = 0;
});

async function montar(): Promise<HTMLElement> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const nada = (): void => {};
  await act(async () => {
    render(
      <ThemeProvider>
        <MessageReader
          locale="es"
          conversation={groupByThread([correo])[0] ?? null}
          onReply={nada}
          onCompose={nada}
          onToggleFlag={nada}
          onMarkUnread={nada}
          onArchive={null}
          onTrash={null}
          onSpam={null}
          onNotSpam={null}
          onPurge={null}
          onMove={nada}
          moveTargets={[{ id: 'ar', name: 'Archive', role: 'archive' } as never]}
          labels={[]}
          onToggleLabel={nada}
        />
      </ThemeProvider>,
      host,
    );
  });
  return host;
}

function boton(host: ParentNode, texto: string): HTMLElement {
  const b = Array.from(host.querySelectorAll<HTMLElement>('button, [role="menuitem"]')).find(
    (el) => el.textContent?.trim() === texto,
  );
  if (!b) throw new Error(`no está «${texto}»`);
  return b;
}

async function esperar(): Promise<void> {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

test('«Crear evento» abre la pestaña en el clic y le da el deep-link al llegar', async () => {
  const host = await montar();
  await act(async () => boton(host, 'Crear evento').click());
  // El Hub aún no ha contestado y la pestaña ya está: sigue siendo fruto del clic.
  expect(pestanas).toHaveLength(1);
  expect(pestanas[0]?.opener).toBeNull();
  await esperar();
  expect(intents[0]).toMatchObject({ type: 'create-event-from-mail' });
  soltarHub({ url: 'https://calendar.sytue.eu/nuevo?x=1' });
  await esperar();
  expect(pestanas[0]?.location.href).toBe('https://calendar.sytue.eu/nuevo?x=1');
});

test('«Compartir en Rooms» pide share-to-chat desde mail y abre la pestaña en el clic', async () => {
  const host = await montar();
  await act(async () => host.querySelector<HTMLElement>('[aria-label="Más acciones"]')?.click());
  await act(async () => boton(document, 'Compartir en Rooms').click());
  expect(pestanas).toHaveLength(1);
  await esperar();
  expect(intents[0]).toMatchObject({
    type: 'share-to-chat',
    params: { source: 'mail', id: 'm1' },
  });
  soltarHub({ url: 'https://rooms.sytue.eu/share?t=1' });
  await esperar();
  expect(pestanas[0]?.location.href).toBe('https://rooms.sytue.eu/share?t=1');
});

test('sin deep-link, o con uno que no es http(s), la pestaña en blanco se cierra', async () => {
  const win = { open: window.open, location: window.location };
  expect(await abrirTrasPeticion(async () => undefined, win)).toBe(false);
  expect(await abrirTrasPeticion(async () => 'javascript:alert(1)', win)).toBe(false);
  expect(pestanas.map((p) => [p.cerrada, p.location.href])).toEqual([
    [true, 'about:blank'],
    [true, 'about:blank'],
  ]);
  await expect(
    abrirTrasPeticion(async () => {
      throw new Error('hub caído');
    }, win),
  ).rejects.toThrow('hub caído');
  expect(pestanas[2]?.cerrada).toBe(true);
});

test('si el navegador bloquea la pestaña, se navega en esta', async () => {
  const idas: string[] = [];
  const win = {
    open: () => null,
    location: { href: 'https://mail.sytue.eu/', assign: (u: string) => idas.push(u) },
  } as unknown as Pick<Window, 'open' | 'location'>;
  expect(await abrirTrasPeticion(async () => 'https://rooms.sytue.eu/share', win)).toBe(true);
  expect(idas).toEqual(['https://rooms.sytue.eu/share']);
});

test('«l» y «v» encuentran los disparadores de «Etiquetar» y «Mover a…»', async () => {
  const host = await montar();
  expect(host.querySelector(disparadorDeAtajo('l') ?? '#ninguno')?.textContent).toContain(
    'Etiquetar',
  );
  expect(host.querySelector(disparadorDeAtajo('v') ?? '#ninguno')?.textContent).toContain(
    'Mover a',
  );
  expect(disparadorDeAtajo('x')).toBeNull();
});
