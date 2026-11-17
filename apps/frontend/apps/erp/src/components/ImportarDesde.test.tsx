/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { AsistenteImportarDesde, PROVEEDORES } from './ImportarDesde.js';
import { Bienvenida } from './ErpApp.js';
import { useT as traducir } from '../i18n/index.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const t = traducir('es');

const INSTANCIA = '6f1c2a4e-1b2c-4d5e-8f90-123456789abc';
const original = globalThis.fetch;
let host: HTMLDivElement;
let peticiones: { ruta: string; cuerpo: Record<string, unknown> }[];
let pruebaFalla: boolean;
let importados: number;
let leidas: boolean;

beforeEach(() => {
  peticiones = [];
  pruebaFalla = false;
  importados = 0;
  leidas = false;
  host = document.createElement('div');
  document.body.appendChild(host);
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'https://erp.example.test');
    const responder = (status: number, data: unknown) =>
      new Response(JSON.stringify(data), {
        status,
        headers: { 'content-type': 'application/json' },
      });
    const cuerpo = init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : {};
    if (init?.method === 'POST') peticiones.push({ ruta: url.pathname, cuerpo });
    switch (url.pathname) {
      case '/api/conexiones':
        leidas = true;
        return responder(200, { conexiones: [{ proveedor: 'odoo', instancia: INSTANCIA }] });
      case '/api/conexiones/holded/probar':
        return pruebaFalla
          ? responder(422, { error: { code: 'unprocessable', message: 'gateway (401)' } })
          : responder(200, { ok: true, objeto: 'contacts', filas: 1 });
      case '/api/conexiones/holded':
        return new Response(null, { status: 204 });
      case '/api/importar/holded':
        return responder(200, {
          importado: [
            { objeto: 'contacts', modelo: 'contacto', creados: 12, actualizados: 3 },
            {
              objeto: 'invoices',
              modelo: 'factura_venta',
              creados: 4,
              actualizados: 0,
              fallos: ['F-1: sin cliente'],
            },
          ],
        });
      // Lo que pide la portada no interesa aquí: sin datos, pinta lo neutro.
      default:
        if (url.pathname.startsWith('/api/')) return responder(404, {});
    }
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
function boton(texto: string): HTMLButtonElement {
  const b = [...document.querySelectorAll('button')].find((x) => x.textContent?.trim() === texto);
  if (!b) throw new Error('Falta botón ' + texto);
  return b;
}
async function pulsar(b: HTMLElement): Promise<void> {
  await act(async () => b.click());
}
async function escribir(selector: string, valor: string): Promise<void> {
  const input = document.querySelector<HTMLInputElement>(selector)!;
  await act(async () => {
    input.value = valor;
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}
function avanzar(): HTMLButtonElement {
  return [...document.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')].at(-1)!;
}
async function montar(): Promise<void> {
  await act(async () =>
    render(
      <AsistenteImportarDesde t={t} onCerrar={() => undefined} onImportado={() => importados++} />,
      host,
    ),
  );
  await esperar(() => leidas && document.body.textContent?.includes('Holded') === true);
}

test('elegir Holded, probar, guardar e importar solo lo marcado', async () => {
  await montar();
  expect(avanzar().disabled).toBe(true);
  await pulsar(boton('Holded'));
  // Qué entra en Sytue One se cuenta antes de conectar nada.
  expect(document.body.textContent).toContain('Holded');
  expect(boton('Holded').getAttribute('aria-checked')).toBe('true');
  await pulsar(avanzar());

  await esperar(() => Boolean(document.querySelector('#erp-importar-instancia')));
  // Sin probar no se avanza: no se guarda una conexión que no responde.
  expect(avanzar().disabled).toBe(true);
  await escribir('#erp-importar-instancia', 'no-es-un-id');
  await pulsar(boton(t('migrate.test')));
  expect(document.body.textContent).toContain(t('migrate.invalidId'));
  expect(peticiones).toHaveLength(0);

  pruebaFalla = true;
  await escribir('#erp-importar-instancia', INSTANCIA);
  await pulsar(boton(t('migrate.test')));
  await esperar(() => document.body.textContent?.includes(t('migrate.testFailed')) === true);
  expect(avanzar().disabled).toBe(true);

  pruebaFalla = false;
  await pulsar(boton(t('migrate.test')));
  await esperar(
    () => document.body.textContent?.includes(t('migrate.testOk', { name: 'Holded' })) === true,
  );
  await pulsar(avanzar());
  await esperar(() => document.body.textContent?.includes(t('migrate.whatHeading')) === true);
  expect(peticiones.map((p) => p.ruta)).toEqual([
    '/api/conexiones/holded/probar',
    '/api/conexiones/holded/probar',
    '/api/conexiones/holded',
  ]);
  expect(peticiones[2]!.cuerpo).toEqual({ instancia: INSTANCIA, activo: true });

  // Desmarcar las compras: no se piden.
  const compras = [...document.querySelectorAll('label')].find((l) =>
    l.textContent?.includes(t('migrate.obj.purchaseInvoices')),
  )!;
  await act(async () => compras.querySelector('input')!.click());
  await pulsar(boton(t('migrate.run')));
  await esperar(() => document.body.textContent?.includes(t('migrate.doneHeading')) === true);
  expect(peticiones.at(-1)).toEqual({
    ruta: '/api/importar/holded',
    cuerpo: { objetos: ['contacts', 'products', 'invoices'] },
  });
  const filas = [...document.querySelectorAll('tbody tr')].map((tr) => tr.textContent);
  expect(filas).toEqual([
    t('migrate.obj.contacts') + '1230',
    t('migrate.obj.salesInvoices') + '401',
  ]);
  expect(importados).toBe(1);
});

test('una conexión guardada rellena su identificador', async () => {
  await montar();
  await pulsar(boton('Odoo'));
  await pulsar(avanzar());
  await esperar(() => Boolean(document.querySelector('#erp-importar-instancia')));
  expect(document.querySelector<HTMLInputElement>('#erp-importar-instancia')!.value).toBe(
    INSTANCIA,
  );
});

// Las claves de objeto del asistente son las de los mapeos del servicio: una que no
// existiera allí haría que `/importar/{proveedor}` rechazara la importación entera.
test('los objetos del asistente existen en los mapeos de SyERP', () => {
  const fichero: Record<string, string> = {
    holded: 'holded.go',
    odoo: 'odoo.go',
    salesforce: 'salesforce.go',
    'business-central': 'businesscentral.go',
  };
  for (const p of PROVEEDORES) {
    const go = readFileSync(
      new URL(
        `../../../../../../services/syerp/internal/conectores/${fichero[p.id]}`,
        import.meta.url,
      ),
      'utf8',
    );
    // `Clave()` es el `Nombre` o, sin él, el `Objeto`.
    const claves = [...go.matchAll(/\b(?:Nombre|Objeto):\s*"([^"]+)"/g)].map((m) => m[1]);
    for (const o of p.objetos) expect(claves).toContain(o.clave);
  }
});

test('la portada ofrece importar solo a quien administra', async () => {
  const props = {
    modulos: [],
    activos: [],
    recientes: [],
    onCambiar: async () => undefined,
    onAbrirModelo: () => undefined,
    onNuevaFicha: () => undefined,
    onVista: () => undefined,
    onPaleta: () => undefined,
    userName: '',
    organizacion: null,
    onConfigurar: () => undefined,
    tareas: 0,
    verObligaciones: false,
    t,
  };
  await act(async () => render(<Bienvenida {...props} />, host));
  expect(document.body.textContent).not.toContain(t('migrate.cta'));
  let abierto = false;
  await act(async () =>
    render(<Bienvenida {...props} onImportar={() => (abierto = true)} />, host),
  );
  expect(document.body.textContent).toContain(t('migrate.cta'));
  await pulsar(boton(t('migrate.title')));
  expect(abierto).toBe(true);
});
