/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Bloquear a …» en el menú «⋯» del lector (MAIL-13): lleva el nombre del remitente,
 * entrega el hilo a quien bloquea, y no sale en Spam, ni a uno mismo, ni en un buzón
 * de solo lectura, ni sin quien lo atienda. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { EmailSummary } from '@sytue/api-clients';

if (typeof document === 'undefined') GlobalRegistrator.register();
process.env.BASE_URL ??= '/mail/';

// El lector pregunta a Contacts por el remitente (MAIL-12): aquí no importa.
globalThis.fetch = (async () => Response.json({ id: null })) as unknown as typeof fetch;

const { ThemeProvider, ToastProvider } = await import('@sytue/ui');
const { MessageReader } = await import('./MessageReader.js');
const { groupByThread } = await import('./utils.js');
type Conversation = ReturnType<typeof groupByThread>[number];

afterEach(() => {
  document.body.innerHTML = '';
});

function correo(de: string, nombre = 'Ana Ruiz'): EmailSummary {
  return {
    id: 'm1',
    threadId: 'h1',
    subject: 'Oferta',
    from: [{ name: nombre, email: de }],
    to: [{ email: 'yo@sytue.eu' }],
    cc: [],
    preview: 'Hola',
    receivedAt: '2026-11-05T10:00:00Z',
    keywords: { $seen: true },
    hasAttachment: false,
  };
}

const nada = (): void => {};

async function montar(
  de: string,
  opts: {
    enSpam?: boolean;
    readOnly?: boolean;
    nombre?: string;
    onBlock?: ((c: Conversation) => void) | null;
  } = {},
): Promise<HTMLElement> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(
      <ThemeProvider>
        <ToastProvider>
          <MessageReader
            locale="es"
            readOnly={opts.readOnly ?? false}
            conversation={groupByThread([correo(de, opts.nombre)])[0] ?? null}
            onReply={nada}
            onCompose={nada}
            onToggleFlag={nada}
            onMarkUnread={nada}
            onBlock={opts.onBlock === undefined ? nada : opts.onBlock}
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
            propias={['yo@sytue.eu']}
          />
        </ToastProvider>
      </ThemeProvider>,
      host,
    );
  });
  return host;
}

function bloquear(host: HTMLElement): HTMLElement | undefined {
  const mas = host.querySelector<HTMLButtonElement>('button[aria-label="Más acciones"]');
  act(() => mas?.click());
  return [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')].find((el) =>
    el.textContent?.startsWith('Bloquear a'),
  );
}

test('lleva el nombre del remitente y entrega el hilo a quien bloquea', async () => {
  const bloqueadas: Conversation[] = [];
  const host = await montar('ana@example.eu', { onBlock: (c) => bloqueadas.push(c) });
  const entrada = bloquear(host);
  expect(entrada?.textContent).toBe('Bloquear a Ana Ruiz');
  act(() => entrada?.click());
  expect(bloqueadas.map((c) => c.threadId)).toEqual(['h1']);
});

test('sin nombre, la dirección', async () => {
  const host = await montar('ana@example.eu', { nombre: '' });
  expect(bloquear(host)?.textContent).toBe('Bloquear a ana@example.eu');
});

test('ni en Spam, ni a uno mismo, ni en solo lectura, ni sin quien lo atienda', async () => {
  for (const [de, opts] of [
    ['ana@example.eu', { enSpam: true }],
    ['Yo@Sytue.eu', {}],
    ['ana@example.eu', { readOnly: true }],
    ['ana@example.eu', { onBlock: null }],
  ] as const) {
    const host = await montar(de, opts);
    expect(bloquear(host)).toBeUndefined();
    document.body.innerHTML = '';
  }
});
