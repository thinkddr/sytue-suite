/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { AsistenteFacturas } from './AsistenteFacturas.js';
import { useT as traducir } from '../i18n/index.js';
import type { Modelo, Registro } from './tipos.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const t = traducir('es');
const compra: Modelo = {
  clave: 'factura_compra',
  modulo: 'facturacion',
  etiqueta: 'Compras',
  numerado: true,
  contable: true,
  recibe: true,
  campos: [],
  lista: [],
  estados: [{ valor: 'confirmado', etiqueta: 'Confirmada' }],
};
const venta: Modelo = {
  ...compra,
  clave: 'factura_venta',
  etiqueta: 'Ventas',
  recibe: false,
  cobro: true,
};
const reg: Registro = {
  id: 'compra-1',
  modelo: 'factura_compra',
  etiqueta: 'Proveedor F-1',
  ref: 'FC-1',
  estado: 'borrador',
  data: { total: 1210 },
  creado_en: '2026-10-03',
  editado_en: '2026-10-03',
};
const original = globalThis.fetch;
let host: HTMLDivElement;
let formularios = 0;
let subidas = 0;
let rutas: string[] = [];
let existente = false;
beforeEach(() => {
  formularios = 0;
  subidas = 0;
  rutas = [];
  existente = false;
  host = document.createElement('div');
  document.body.appendChild(host);
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'https://erp.example.test');
    rutas.push(url.pathname);
    const response = (status: number, data: unknown) =>
      new Response(JSON.stringify(data), {
        status,
        headers: { 'content-type': 'application/json' },
      });
    if (url.pathname === '/api/factura_compra/asistencia/leer')
      return response(200, {
        data: {
          fecha: '2026-10-03',
          base: 1000,
          iva: 210,
          total: 1210,
          externo_id: 'B12345678/F-1',
        },
        etiquetas: {},
        ...(existente ? { existente: { ...reg, estado: 'confirmado' } } : {}),
      });
    if (url.pathname === '/api/adjuntos' && init?.method === 'POST') {
      subidas++;
      return subidas === 1
        ? response(500, { error: { message: 'Adjunto no disponible' } })
        : response(200, { tareas_hechas: 0 });
    }
    if (url.pathname === '/api/adjuntos') return response(200, { adjuntos: [] });
    throw new Error('Petición inesperada ' + url.pathname);
  }) as typeof fetch;
});
afterEach(async () => {
  await act(async () => render(null, host));
  host.remove();
  document.body.innerHTML = '';
  globalThis.fetch = original;
});
async function esperar(condicion: () => boolean): Promise<void> {
  for (let i = 0; i < 40; i++) {
    if (condicion()) return;
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  }
  expect(condicion()).toBe(true);
}
async function pulsar(b: HTMLElement): Promise<void> {
  await act(async () => b.click());
}
function boton(texto: string): HTMLButtonElement {
  const b = [...document.querySelectorAll('button')].find((x) => x.textContent?.trim() === texto);
  if (!b) throw new Error('Falta botón ' + texto);
  return b;
}
async function montar(): Promise<void> {
  await act(async () =>
    render(
      <AsistenteFacturas
        t={t}
        locale="es"
        modelos={[compra, venta]}
        puedeCrear={() => true}
        puedeAccion={() => true}
        onCerrar={() => undefined}
        onAbrir={() => undefined}
        renderFormulario={(p) => {
          formularios++;
          return <button onClick={() => p.onCreado(reg)}>Guardar ficha de prueba</button>;
        }}
      />,
      host,
    ),
  );
}
async function elegirXML(): Promise<void> {
  await pulsar([...document.querySelectorAll('button')].at(-1)!);
  const input = document.querySelector<HTMLInputElement>('#asistente-factura-original')!;
  const fichero = new File(['<Invoice/>'], 'factura.xml', { type: 'application/xml' });
  Object.defineProperty(input, 'files', { value: [fichero], configurable: true });
  await act(async () => {
    input.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await pulsar(boton(t('process.prepare')));
}
test('fallar el adjunto conserva la factura creada y solo reintenta subir el original', async () => {
  await montar();
  await elegirXML();
  await esperar(() =>
    Boolean(
      [...document.querySelectorAll('button')].find(
        (b) => b.textContent === t('invoiceAssist.review'),
      ),
    ),
  );
  await pulsar(boton(t('invoiceAssist.review')));
  await pulsar(boton('Guardar ficha de prueba'));
  expect(formularios).toBe(1);
  await pulsar(boton(t('docs.upload')));
  await esperar(
    () => document.querySelector('[role="alert"]')?.textContent === 'Adjunto no disponible',
  );
  await pulsar(boton(t('docs.upload')));
  await esperar(
    () =>
      Boolean(document.querySelector('#docs-factura_compra-compra-1')) ||
      document.body.textContent?.includes(t('invoiceAssist.postHint')) === true,
  );
  expect(formularios).toBe(1);
  expect(subidas).toBe(2);
  expect(rutas.some((r) => r.includes('verifactu'))).toBe(false);
  expect(rutas.some((r) => r.includes('asentar'))).toBe(false);
});
test('una compra ya registrada abre el paso de adjuntos sin volver a crear ni contabilizar', async () => {
  existente = true;
  await montar();
  await elegirXML();
  await esperar(() => document.body.textContent?.includes(t('invoiceAssist.attachHint')) === true);
  expect(formularios).toBe(0);
  await pulsar(boton(t('docs.upload')));
  await esperar(() => Boolean(document.querySelector('[role="alert"]')));
  await pulsar(boton(t('docs.upload')));
  await esperar(() => document.body.textContent?.includes('Confirmada') === true);
  expect(formularios).toBe(0);
  expect(rutas.some((r) => r.includes('asentar') || r.includes('verifactu'))).toBe(false);
});
