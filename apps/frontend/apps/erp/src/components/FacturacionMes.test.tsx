/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La tarjeta «Facturación por mes» de la portada: pide el informe al motor, pinta los
 * doce meses con su rótulo y desaparece si el servicio no puede dar los datos.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { FacturacionMes } from './FacturacionMes.js';
import { useT as traducir } from '../i18n/index.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const t = traducir('es');
const hoy = new Date(2026, 10, 12, 12);
const original = globalThis.fetch;
let host: HTMLDivElement;
let pedidos: { url: string; cuerpo: unknown }[];

function contestar(cuerpo: unknown, status = 200): void {
  pedidos = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    pedidos.push({ url: String(input), cuerpo: JSON.parse(String(init?.body ?? 'null')) });
    return new Response(JSON.stringify(cuerpo), { status });
  }) as typeof fetch;
}

async function pintar(): Promise<void> {
  // Los efectos (y con ellos la petición) corren al CERRAR el primer `act`; la
  // respuesta se espera en el segundo.
  await act(async () => render(<FacturacionMes locale="es" t={t} hoy={hoy} />, host));
  await act(async () => {
    for (let i = 0; i < 3; i++) await new Promise((r) => setTimeout(r, 0));
  });
}

function tarjeta(): HTMLElement | null {
  return host.querySelector('[data-grafico="facturacion-mes"]');
}

beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
});
afterEach(async () => {
  await act(async () => render(null, host));
  host.remove();
  globalThis.fetch = original;
});

test('pinta los doce meses con su rótulo y el total de la ventana', async () => {
  contestar({
    datos: [
      {
        columnas: [
          { clave: 'fecha', tipo: 'texto', etiqueta: 'Fecha' },
          { clave: 'valor', tipo: 'dinero', etiqueta: 'Total' },
        ],
        filas: [
          ['2026-10', 1000],
          ['2026-11', 210.5],
        ],
      },
    ],
  });
  await pintar();
  expect(pedidos).toHaveLength(1);
  expect(pedidos[0]!.url).toBe('/api/informes/datos');
  const t0 = tarjeta();
  expect(t0?.getAttribute('aria-busy')).toBe('false');
  expect(t0?.textContent).toContain(t('dashboard.monthlyTitle'));
  // Doce barras: la de un mes sin facturas sigue ahí.
  expect(t0?.querySelectorAll('rect')).toHaveLength(12);
  const eje = [...(t0?.querySelector('[data-bar-chart-axis]')?.children ?? [])];
  expect(eje).toHaveLength(12);
  expect(eje.at(-1)?.textContent).toMatch(/nov/i);
  // El total en euros, y cada mes en la tabla accesible con su importe.
  expect(t0?.textContent).toMatch(/1\.?210,50\s?€/);
  expect(t0?.querySelector('tbody tr:last-child td')?.textContent).toMatch(/210,50/);
});

test('sin facturas en la ventana lo dice, con las doce barras a cero', async () => {
  contestar({ datos: [{ columnas: [], filas: [] }] });
  await pintar();
  expect(tarjeta()?.textContent).toContain(t('dashboard.monthlyNone'));
  expect(tarjeta()?.querySelectorAll('rect')).toHaveLength(12);
});

test('mientras llega esboza las barras a su alto, sin cifras', async () => {
  globalThis.fetch = (() => new Promise<Response>(() => {})) as unknown as typeof fetch;
  await pintar();
  expect(tarjeta()?.getAttribute('aria-busy')).toBe('true');
  expect(tarjeta()?.querySelector('[data-esqueleto="barras"]')).not.toBeNull();
  expect(tarjeta()?.querySelector('svg')).toBeNull();
});

test('sin permiso la tarjeta no se pinta', async () => {
  contestar({ datos: [{ columnas: null, filas: null, error: 'sin_permiso' }] });
  await pintar();
  expect(tarjeta()).toBeNull();
});

test('con el servicio caído la tarjeta no se pinta', async () => {
  contestar({ error: { code: 'unavailable' } }, 503);
  await pintar();
  expect(tarjeta()).toBeNull();
});
