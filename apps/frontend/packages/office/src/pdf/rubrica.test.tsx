/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «RECORDAR MI FIRMA».
 *
 * Lo que se fija: que sin marcar la casilla no se guarda nada; que marcada se
 * guarda lo dibujado (redondeado y dentro del lienzo); que una firma guardada se
 * pone sola en el lienzo con la casilla marcada; y que desmarcarla y firmar la
 * olvida. Y que si el BFF falla se firma igual.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { docsT } from '../i18n';
import { redondear, type Pagina } from './api';
import { Firmar } from './Paneles';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = docsT('es');
const A4: Pagina = { numero: 1, ancho: 595, alto: 842, rotacion: 0, caja: [0, 0, 595, 842] };
const GUARDADA: [number, number][][] = [
  [
    [5, 5],
    [60, 40],
  ],
];

let host: HTMLElement;
let enviados: Record<string, unknown>[];
let pedidas: { metodo: string; cuerpo: unknown }[];
let respuestaGet: () => Response;
const fetchReal = globalThis.fetch;

beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
  enviados = [];
  pedidas = [];
  respuestaGet = () => new Response(null, { status: 404 });
  globalThis.fetch = ((url: string, init?: RequestInit) => {
    if (!String(url).includes('/api/pdf-rubrica')) return Promise.reject(new Error('fuera'));
    const metodo = init?.method ?? 'GET';
    pedidas.push({ metodo, cuerpo: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (metodo === 'GET') return Promise.resolve(respuestaGet());
    return Promise.resolve(new Response('{"ok":true}', { status: 200 }));
  }) as unknown as typeof fetch;
});
afterEach(() => {
  act(() => render(null, host));
  host.remove();
  globalThis.fetch = fetchReal;
});

async function montar(): Promise<void> {
  await act(async () => {
    render(
      <Firmar
        t={t}
        path="/contrato.pdf"
        paginas={[A4]}
        svgs={{}}
        onAplicar={(cuerpo) => {
          enviados.push(cuerpo);
          return Promise.resolve(true);
        }}
        onCerrar={() => undefined}
        trabajando={false}
      />,
      host,
    );
  });
  // La lectura de la guardada es asíncrona: se deja que conteste.
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

function dibujar(): void {
  const lienzo = host.querySelector('canvas')!;
  lienzo.setPointerCapture = () => undefined;
  lienzo.getBoundingClientRect = () => ({ left: 0, top: 0, width: 480, height: 148 }) as DOMRect;
  act(() => {
    lienzo.dispatchEvent(new PointerEvent('pointerdown', { clientX: 10.04, clientY: 10 }));
  });
  act(() => {
    // Fuera del lienzo por la derecha: el puntero capturado puede salirse.
    lienzo.dispatchEvent(new PointerEvent('pointermove', { clientX: 500, clientY: 30 }));
  });
}

const casilla = () => host.querySelector<HTMLInputElement>('input[data-recordar-firma]')!;
const escrituras = () => pedidas.filter((p) => p.metodo !== 'GET');

function enviar(): void {
  act(() => {
    host.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }));
  });
  expect(enviados).toHaveLength(1);
}

test('sin marcar la casilla no se guarda nada', async () => {
  await montar();
  expect(casilla().checked).toBe(false);
  dibujar();
  enviar();
  expect(escrituras()).toEqual([]);
});

test('con la casilla, se guarda lo dibujado redondeado y dentro del lienzo', async () => {
  await montar();
  dibujar();
  act(() => casilla().click());
  enviar();
  expect(escrituras()).toEqual([
    {
      metodo: 'PUT',
      cuerpo: {
        trazos: [
          [
            [10, 10],
            [480, 30],
          ],
        ],
      },
    },
  ]);
});

test('la guardada se pone sola en el lienzo, y desmarcar y firmar la olvida', async () => {
  respuestaGet = () => new Response(JSON.stringify({ trazos: GUARDADA }), { status: 200 });
  await montar();
  expect(casilla().checked).toBe(true);
  // Se puede firmar sin dibujar: los trazos ya están.
  act(() => casilla().click());
  enviar();
  const cuerpo = enviados[0] as { datos: { trazos: [number, number][][] } };
  expect(cuerpo.datos.trazos[0]).toHaveLength(2);
  expect(escrituras()).toEqual([{ metodo: 'DELETE', cuerpo: undefined }]);
});

test('si el BFF no contesta se firma igual', async () => {
  globalThis.fetch = (() => Promise.reject(new Error('caído'))) as unknown as typeof fetch;
  await montar();
  dibujar();
  act(() => casilla().click());
  enviar();
});

test('redondear deja una décima y descarta los trazos vacíos', () => {
  expect(
    redondear([
      [
        [1.234, -3],
        [600, 147.96],
      ],
      [],
    ]),
  ).toEqual([
    [
      [1.2, 0],
      [480, 148],
    ],
  ]);
});
