/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ⌘/Ctrl+Intro publica el comentario (Intro solo sigue siendo un salto de línea), y las
 * marcas de tiempo de un comentario llevan el vídeo allí. Sobre un `fetch` de mentira:
 * `mock.module` del cliente choca con el de `Diapositivas.test.tsx` en la misma pasada.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';

if (typeof document === 'undefined') GlobalRegistrator.register();

const publicados: string[] = [];
const comentario = (id: string, body: string, sub: string, name: string) => ({
  id,
  body,
  author: { sub, name },
  created_at: '2026-11-20T10:00:00Z',
  can: { delete: sub === 'yo' },
});
const fetchReal = globalThis.fetch;
globalThis.fetch = (async (_url: RequestInfo | URL, init?: RequestInit) => {
  if (init?.method === 'POST') {
    const { body } = JSON.parse(String(init.body)) as { body: string };
    publicados.push(body);
    return Response.json(comentario(`n${publicados.length}`, body, 'yo', 'Yo'));
  }
  return Response.json({ comments: [comentario('c1', 'Lo mejor, en el 3:15', 'otra', 'Ana')] });
}) as typeof fetch;

const { Comments } = await import('./Comments.js');
const { ToastProvider } = await import('@sytue/ui');
const { useT: traductor } = await import('../i18n/index.js');
const t = traductor('es');

let host: HTMLElement;
afterEach(() => {
  render(null, host);
  host.remove();
  globalThis.fetch = fetchReal;
});

const tecla = (el: Element, init: KeyboardEventInit): Promise<void> =>
  act(() => {
    el.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, ...init }));
  });

test('⌘/Ctrl+Intro publica; Intro a secas no; y «3:15» salta', async () => {
  const saltos: number[] = [];
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(() =>
    render(
      <ToastProvider>
        <Comments
          t={t}
          locale="es"
          videoId="v1"
          conSesion
          activos
          entrar={() => undefined}
          miSub="yo"
          moderacion={{
            denunciar: () => undefined,
            bloquear: () => Promise.resolve(false),
            bloqueados: new Set(),
            dialogos: <span />,
          }}
          saltar={(s) => saltos.push(s)}
        />
      </ToastProvider>,
      host,
    ),
  );
  for (let i = 0; i < 20 && !host.querySelector('li'); i++)
    await act(async () => {
      await new Promise((r) => setTimeout(r, 5));
    });
  const marca = Array.from(host.querySelectorAll('button')).find((b) => b.textContent === '3:15');
  expect(marca?.getAttribute('aria-label')).toBe('Ir a 3:15');
  await act(() => marca!.click());
  expect(saltos).toEqual([195]);

  const campo = host.querySelector('textarea')!;
  expect(campo.getAttribute('aria-keyshortcuts')).toBe('Control+Enter Meta+Enter');
  await act(() => {
    campo.value = 'Muy bueno';
    campo.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await tecla(campo, { key: 'Enter' });
  expect(publicados).toEqual([]);
  await tecla(campo, { key: 'Enter', ctrlKey: true });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 5));
  });
  expect(publicados).toEqual(['Muy bueno']);
  expect(host.querySelector('textarea')!.value).toBe('');
});
