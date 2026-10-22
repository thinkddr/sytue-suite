/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El diálogo de denuncia: los motivos son un grupo de opciones con su leyenda, no se
 * envía sin elegir uno, y lo que sale es lo que se eligió.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { ObjetivoDeDenuncia } from '../lib/tipos.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { ReportDialog } = await import('./ReportDialog.js');
const { ToastProvider } = await import('@sytue/ui');
const { useT: traductor } = await import('../i18n/index.js');
const t = traductor('es');

const enviadas: unknown[] = [];

let host: HTMLElement;
afterEach(() => {
  render(null, host);
  host.remove();
  enviadas.length = 0;
});
async function abrir(objetivo: ObjetivoDeDenuncia): Promise<HTMLElement> {
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(
      <ToastProvider>
        <ReportDialog
          t={t}
          locale="es"
          objetivo={objetivo}
          enviar={(datos) => {
            enviadas.push(datos);
            return Promise.resolve();
          }}
          cerrar={() => render(null, host)}
        />
      </ToastProvider>,
      host,
    );
  });
  const dialogo = document.querySelector<HTMLElement>('[role="dialog"]');
  if (!dialogo) throw new Error('el diálogo no se abrió');
  return dialogo;
}

test('los motivos son un grupo con leyenda y cada uno tiene su rótulo', async () => {
  const d = await abrir({ kind: 'video', videoId: 'v1' });
  expect(d.textContent).toContain('Denunciar este vídeo');
  const grupo = d.querySelector('fieldset');
  expect(grupo?.querySelector('legend')?.textContent).toBe('¿Cuál es el problema?');
  const radios = [...(grupo?.querySelectorAll<HTMLInputElement>('input[type="radio"]') ?? [])];
  expect(radios.map((r) => r.value)).toEqual([
    'harassment',
    'sexual',
    'violence',
    'spam',
    'copyright',
    'other',
  ]);
  // Un solo grupo: las flechas del teclado recorren los seis.
  expect(new Set(radios.map((r) => r.name)).size).toBe(1);
  for (const r of radios) expect(r.closest('label')?.textContent?.trim()).not.toBe('');
  // El campo de detalles tiene su etiqueta.
  const area = d.querySelector('textarea');
  expect(d.querySelector(`label[for="${area?.id}"]`)?.textContent).toBe('Detalles (opcional)');
});

test('sin motivo no se envía; con él sale lo elegido', async () => {
  const d = await abrir({ kind: 'comment', videoId: 'v1', target: 'c9' });
  const enviar = d.querySelector<HTMLButtonElement>('button[type="submit"]');
  expect(enviar?.disabled).toBe(true);
  await act(async () => {
    d.querySelector<HTMLInputElement>('input[value="spam"]')?.click();
  });
  expect(enviar?.disabled).toBe(false);
  await act(async () => {
    d.querySelector('form')?.dispatchEvent(new Event('submit', { cancelable: true }));
    await Promise.resolve();
  });
  expect(enviadas).toEqual([{ reason: 'spam', details: '' }]);
  // Enviada, el diálogo se cierra.
  expect(document.querySelector('[role="dialog"]')).toBeNull();
});
