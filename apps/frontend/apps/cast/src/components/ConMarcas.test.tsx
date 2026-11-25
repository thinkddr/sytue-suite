/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las marcas de tiempo pulsables de una descripción o un comentario, los capítulos y
 * «Empezar en…» de Compartir. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { readFileSync } from 'node:fs';
import type { Video } from '../lib/tipos.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { Capitulos, ConMarcas } = await import('./ConMarcas.js');
const { ShareDialog } = await import('./ShareDialog.js');
const { ToastProvider } = await import('@sytue/ui');
const { capitulosDe } = await import('../lib/marcas.js');
// Alias: `useT` no es un hook de verdad, y aquí se llama fuera de un componente.
const { useT: traductor } = await import('../i18n/index.js');
const t = traductor('es');

let host: HTMLElement;
const montar = async (nodo: preact.ComponentChild): Promise<HTMLElement> => {
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(() => render(<ToastProvider>{nodo}</ToastProvider>, host));
  return host;
};
afterEach(() => {
  render(null, host);
  host.remove();
});

test('una marca del texto es un botón que lleva el vídeo allí; sin `saltar`, texto', async () => {
  const saltos: number[] = [];
  const h = await montar(
    <p>
      <ConMarcas
        t={t}
        texto="Lo bueno empieza en 12:05, de verdad"
        saltar={(s) => saltos.push(s)}
      />
    </p>,
  );
  expect(h.textContent).toBe('Lo bueno empieza en 12:05, de verdad');
  const marca = h.querySelector('button')!;
  expect(marca.textContent).toBe('12:05');
  expect(marca.getAttribute('aria-label')).toBe('Ir a 12:05');
  await act(() => marca.click());
  expect(saltos).toEqual([725]);

  render(null, host);
  await act(() =>
    render(
      <p>
        <ConMarcas t={t} texto="en 12:05" />
      </p>,
      host,
    ),
  );
  expect(host.querySelector('button')).toBeNull();
});

test('los capítulos de la descripción, cada uno con su marca, llevan a su segundo', async () => {
  const saltos: number[] = [];
  const caps = capitulosDe('0:00 Presentación\n1:30 Demo\n1:02:03 Preguntas');
  const h = await montar(<Capitulos t={t} capitulos={caps} saltar={(s) => saltos.push(s)} />);
  expect(h.querySelector('nav')?.getAttribute('aria-label')).toBe('Capítulos');
  const filas = Array.from(h.querySelectorAll('li button'));
  expect(filas.map((b) => b.textContent)).toEqual([
    '0:00Presentación',
    '1:30Demo',
    '1:02:03Preguntas',
  ]);
  await act(() => (filas[1] as HTMLButtonElement).click());
  expect(saltos).toEqual([90]);
});

test('Compartir ofrece «Empezar en…» por donde iba el vídeo, y el enlace lleva «?t=»', async () => {
  const video = { id: 'v1', title: 'Charla', visibility: 'public', width: 1920, height: 1080 };
  const h = await montar(
    <ShareDialog t={t} video={video as Video} segundo={83.6} abierto cerrar={() => undefined} />,
  );
  const enlace = (): string => document.querySelector<HTMLInputElement>('input[readonly]')!.value;
  expect(enlace().endsWith('/watch/v1')).toBe(true);
  const casilla = Array.from(document.querySelectorAll('label')).find((l) =>
    l.textContent?.includes('Empezar en 1:23'),
  );
  expect(casilla).toBeDefined();
  const input = casilla!.querySelector('input') ?? document.getElementById(casilla!.htmlFor);
  await act(() => (input as HTMLInputElement).click());
  expect(enlace().endsWith('/watch/v1?t=83')).toBe(true);
  void h;
});

test('sin haber empezado no hay «Empezar en…»', async () => {
  const video = { id: 'v1', title: 'Charla', visibility: 'public', width: 1920, height: 1080 };
  await montar(<ShareDialog t={t} video={video as Video} abierto cerrar={() => undefined} />);
  expect(document.body.textContent).not.toContain('Empezar en');
});

test('la página de ver respeta «?t=» y pasa `saltar` a la descripción y a los comentarios', () => {
  const fuente = readFileSync(new URL('../islands/WatchApp.tsx', import.meta.url), 'utf8');
  expect(fuente).toContain("inicioDeUrl(new URL(location.href).searchParams.get('t'))");
  expect(fuente).toContain('inicio={inicio}');
  expect(fuente).toMatch(/<Descripcion[^>]*saltar=/);
  expect(fuente).toMatch(/<Comments[^>]*saltar=\{play && !falloPlay \? saltar : undefined\}/);
});
