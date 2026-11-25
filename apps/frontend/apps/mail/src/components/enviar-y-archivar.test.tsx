/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Enviar y archivar» y las fechas fijas de «Programar envío» en el redactor.
 *
 * - Contestando sale «Enviar y archivar»: envía y, ya enviado, pide al padre que
 *   archive la conversación del correo contestado. «Enviar» a secas no archiva, y un
 *   mensaje nuevo no ofrece el botón. Fyne: `TestEnviarYArchivar`.
 * - El panel de programar ofrece «Mañana» (las fechas de posponer) y pulsarla rellena
 *   el campo de fecha y hora. Fyne: `TestProgramarConUnaFechaFija`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { en } from '../i18n/messages/en.js';

if (typeof document === 'undefined') GlobalRegistrator.register({ url: 'https://sytue.test/mail' });
else
  (window as unknown as { happyDOM?: { setURL: (url: string) => void } }).happyDOM?.setURL(
    'https://sytue.test/mail',
  );

const { ComposeModal } = await import('./ComposeModal.js');
const { toLocalInput } = await import('./utils.js');

const enviados: string[] = [];

beforeEach(() => {
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith('/api/send')) enviados.push(url);
    return new Response(JSON.stringify({ id: 'd1' }), {
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
});
afterEach(() => {
  render(null, document.body);
  document.body.innerHTML = '';
  enviados.length = 0;
});

async function esperar(): Promise<void> {
  for (let i = 0; i < 8; i++)
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
}

function boton(etiqueta: string): HTMLButtonElement | undefined {
  return [...document.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.getAttribute('aria-label') === etiqueta || b.textContent?.trim() === etiqueta,
  );
}

async function redactar(opciones: {
  answers?: string;
  onEnviarYArchivar?: (id: string) => void;
}): Promise<void> {
  await act(async () => {
    render(
      <ComposeModal
        locale="en"
        open
        ownerId="ana"
        initial={{
          to: 'luis@example.com',
          subject: 'Re: Hola',
          body: 'Ok',
          ...(opciones.answers ? { answers: opciones.answers } : {}),
        }}
        draftMailboxId="drafts"
        onClose={() => undefined}
        onEnviarYArchivar={opciones.onEnviarYArchivar}
      />,
      document.body,
    );
  });
  await esperar();
}

test('«Enviar y archivar» envía y, después, archiva la conversación contestada', async () => {
  const archivadas: string[] = [];
  await redactar({ answers: 'm1', onEnviarYArchivar: (id) => archivadas.push(id) });
  const b = boton(en['compose.sendAndArchive']);
  expect(b).toBeDefined();
  await act(async () => b!.click());
  await esperar();
  expect(enviados).toHaveLength(1);
  expect(archivadas).toEqual(['m1']);
});

test('«Enviar» a secas no archiva, y sin contestar a nadie no hay botón', async () => {
  const archivadas: string[] = [];
  await redactar({ answers: 'm1', onEnviarYArchivar: (id) => archivadas.push(id) });
  await act(async () => boton(en['compose.send'])!.click());
  await esperar();
  expect(enviados).toHaveLength(1);
  expect(archivadas).toEqual([]);

  render(null, document.body);
  await redactar({ onEnviarYArchivar: (id) => archivadas.push(id) });
  expect(boton(en['compose.sendAndArchive'])).toBeUndefined();
});

test('«Programar envío» ofrece «Mañana» y rellena el campo', async () => {
  await redactar({});
  await act(async () => boton(en['compose.schedule'])!.click());
  await esperar();
  const manana = boton(en['snooze.tomorrow']);
  expect(manana).toBeDefined();
  await act(async () => manana!.click());
  await esperar();
  const campo = document.getElementById('compose-schedule-at') as HTMLInputElement;
  const quiero = new Date();
  quiero.setDate(quiero.getDate() + 1);
  quiero.setHours(8, 0, 0, 0);
  expect(campo.value).toBe(toLocalInput(quiero));
  expect(manana!.getAttribute('aria-pressed')).toBe('true');
});
