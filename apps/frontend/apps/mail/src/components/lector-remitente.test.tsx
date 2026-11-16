/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El lector resume lo que el MX comprobó del remitente (SPF/DKIM/DMARC): la
 * insignia «Verificado» junto a la dirección cuando DMARC pasa, y un aviso antes
 * del cuerpo cuando falla. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { EmailSummary, SenderAuth } from '@sytue/api-clients';

if (typeof document === 'undefined') GlobalRegistrator.register();
process.env.BASE_URL ??= '/mail/';

const { ThemeProvider } = await import('@sytue/ui');
const { MessageReader } = await import('./MessageReader.js');
const { groupByThread } = await import('./utils.js');

globalThis.fetch = (async () =>
  new Response(JSON.stringify({}), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  })) as unknown as typeof fetch;

function correo(senderAuth?: SenderAuth): EmailSummary {
  return {
    id: 'm1',
    threadId: 'h1',
    subject: 'Factura',
    from: [{ email: 'cobros@banco.eu' }],
    to: [{ email: 'yo@ejemplo.eu' }],
    cc: [],
    preview: 'Hola',
    receivedAt: '2026-11-05T10:00:00Z',
    keywords: { $seen: true },
    hasAttachment: false,
    ...(senderAuth ? { senderAuth } : {}),
  };
}

afterEach(() => {
  document.body.innerHTML = '';
});

async function montar(mensaje: EmailSummary): Promise<HTMLElement> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const nada = (): void => {};
  await act(async () => {
    render(
      <ThemeProvider>
        <MessageReader
          locale="es"
          conversation={groupByThread([mensaje])[0] ?? null}
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
          moveTargets={[]}
          labels={[]}
          onToggleLabel={nada}
        />
      </ThemeProvider>,
      host,
    );
  });
  return host;
}

test('DMARC pass pinta «Verificado» con el detalle técnico en el title', async () => {
  const host = await montar(correo({ spf: 'pass', dkim: 'pass', dmarc: 'pass' }));
  const insignia = Array.from(host.querySelectorAll('span')).find(
    (el) => el.textContent?.trim() === 'Verificado',
  );
  expect(insignia?.getAttribute('title')).toBe('SPF pass · DKIM pass · DMARC pass');
  expect(host.textContent).not.toContain('No hemos podido comprobar');
});

test('DMARC fail avisa antes del cuerpo con el dominio del remitente', async () => {
  const host = await montar(correo({ spf: 'pass', dmarc: 'fail' }));
  expect(host.textContent).toContain('venga de verdad de banco.eu');
  expect(host.textContent).not.toContain('Verificado');
});

test('sin veredictos no dice nada', async () => {
  const host = await montar(correo());
  expect(host.textContent).not.toContain('Verificado');
  expect(host.textContent).not.toContain('No hemos podido comprobar');
});
