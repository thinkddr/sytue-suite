/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
/*
 * La cartera (Cartera.tsx y el mayor de Libro.tsx) contra un servicio de mentira: qué se
 * pide —con el día de quien mira— y qué se pinta. Sin respuesta, la pieza no aparece.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { AntiguedadCartera, BajoMinimo, EstadoCuentaContacto } from './Cartera.js';
import { MayorCuenta } from './Libro.js';
import { useT as traducir } from '../i18n/index.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const t = traducir('es');
const original = globalThis.fetch;
let host: HTMLDivElement;
let pedidas: string[] = [];
let respuestas: Record<string, unknown> = {};

beforeEach(() => {
  pedidas = [];
  respuestas = {};
  host = document.createElement('div');
  document.body.appendChild(host);
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = new URL(String(input), 'https://erp.example.test');
    pedidas.push(url.pathname + url.search);
    const datos = respuestas[url.pathname];
    return new Response(JSON.stringify(datos ?? {}), {
      status: datos ? 200 : 404,
      headers: { 'content-type': 'application/json' },
    });
  }) as typeof fetch;
});

afterEach(() => {
  render(null, host);
  host.remove();
  globalThis.fetch = original;
});

async function pintar(nodo: preact.ComponentChild): Promise<void> {
  await act(async () => {
    render(nodo, host);
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

test('la antigüedad pide los cobros con el día de hoy y pinta los cinco tramos', async () => {
  respuestas['/api/cobros/antiguedad'] = {
    modelo: 'factura_venta',
    hoy: '2026-11-25',
    tramos: [
      { tramo: 'al_dia', centimos: 10000, documentos: 1 },
      { tramo: '1_30', centimos: 20000, documentos: 1 },
      { tramo: '31_60', centimos: 0, documentos: 0 },
      { tramo: '61_90', centimos: 0, documentos: 0 },
      { tramo: 'mas_90', centimos: 40000, documentos: 1 },
    ],
    total: 70000,
    vencido: 60000,
  };
  await pintar(<AntiguedadCartera locale="es" t={t} conPagos />);
  expect(pedidas[0]).toMatch(/^\/api\/cobros\/antiguedad\?tipo=cobros&hoy=\d{4}-\d{2}-\d{2}$/);
  expect(document.querySelectorAll('[data-tramo]').length).toBe(5);
  expect(document.body.textContent).toContain(t('aging.over90'));
  expect(document.body.textContent).toContain('600,00');
});

test('sin servicio, ni la antigüedad ni el bajo mínimo se pintan', async () => {
  await pintar(
    <>
      <AntiguedadCartera locale="es" t={t} conPagos={false} />
      <BajoMinimo t={t} />
    </>,
  );
  expect(document.querySelector('[data-cartera]')).toBeNull();
});

test('el bajo mínimo enseña cada producto con su stock y su mínimo', async () => {
  respuestas['/api/inventario/bajo-minimo'] = {
    productos: [
      {
        id: 'p1',
        nombre: 'Tornillo',
        minimo: '100',
        stock: '50.5',
        faltan: '49.5',
        almacenes: 2,
        unidad: 'ud',
      },
    ],
  };
  await pintar(<BajoMinimo t={t} />);
  expect(document.querySelector('[data-cartera="bajo-minimo"]')?.textContent).toContain('Tornillo');
  expect(document.body.textContent).toContain(t('stock.levels', { stock: '50.5 ud', min: '100' }));
});

test('el estado de cuenta suma los dos lados y abre la factura pulsada', async () => {
  respuestas['/api/contacto/c1/estado-cuenta'] = {
    ventas: { facturado: 40000, pagado: 30000, pendiente: 10000, vencido: 10000 },
    compras: { facturado: 0, pagado: 0, pendiente: 0, vencido: 0 },
    documentos: [
      {
        id: 'f1',
        modelo: 'factura_venta',
        ref: 'FV-1',
        fecha: '2026-10-01',
        estado: 'confirmado',
        total: 10000,
        dias_vencida: 24,
      },
    ],
    recortado: false,
  };
  const abiertas: string[] = [];
  await pintar(
    <EstadoCuentaContacto
      contactoId="c1"
      locale="es"
      t={t}
      etiquetaEstado={(_m, e) => e}
      onAbrir={(m, id) => abiertas.push(`${m}/${id}`)}
    />,
  );
  expect(document.body.textContent).toContain(t('account.sales'));
  // Sin nada facturado por el contacto, ese lado no se pinta.
  expect(document.body.textContent).not.toContain(t('account.purchases'));
  expect(document.body.textContent).toContain(t('due.overdue', { n: 24 }));
  const fila = [...document.querySelectorAll('button')].find((b) =>
    b.textContent?.includes('FV-1'),
  );
  await act(async () => fila?.click());
  expect(abiertas).toEqual(['factura_venta/f1']);
});

test('el mayor de una cuenta enseña el saldo de entrada, cada apunte y el de salida', async () => {
  respuestas['/api/mayor'] = {
    cuenta: '430000',
    saldo_inicial: 5000,
    apuntes: [
      {
        asiento_id: 'a1',
        ref: 'VENTAS-1',
        fecha: '2026-11-02',
        concepto: 'Factura FV-1',
        debe: 12100,
        haber: 0,
        saldo: 17100,
      },
    ],
    debe: 12100,
    haber: 0,
    saldo_final: 17100,
    recortado: false,
  };
  await pintar(
    <MayorCuenta cuenta="430000" periodo="todo" locale="es" t={t} onCerrar={() => {}} />,
  );
  expect(pedidas[0]).toBe('/api/mayor?cuenta=430000');
  expect(document.body.textContent).toContain(t('ledger.accountTitle', { account: '430000' }));
  expect(document.body.textContent).toContain('Factura FV-1');
  expect(document.body.textContent).toContain('171,00');
});
