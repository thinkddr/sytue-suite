/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { ToastProvider } from '@sytue/ui';
import { Aprobaciones, puedeResolver } from './Aprobaciones.js';
import { Ficha } from './ErpApp.js';
import type { Aprobacion, Modelo, Registro } from './tipos.js';
import { useT as traducir } from '../i18n/index.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const t = traducir('es');
const original = globalThis.fetch;
let host: HTMLDivElement;
/** Lo que se pidió al servicio: método, ruta y cuerpo. */
let pedidas: { metodo: string; ruta: string; cuerpo: string }[] = [];
let responder: (metodo: string, ruta: string) => Response;

beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
  pedidas = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const ruta = String(input);
    const metodo = init?.method ?? 'GET';
    pedidas.push({ metodo, ruta, cuerpo: String(init?.body ?? '') });
    return responder(metodo, ruta);
  }) as typeof fetch;
});
afterEach(async () => {
  await act(async () => render(null, host));
  host.remove();
  document.body.innerHTML = '';
  globalThis.fetch = original;
});

const json = (cuerpo: unknown, status = 200): Response =>
  new Response(JSON.stringify(cuerpo), {
    status,
    headers: { 'content-type': 'application/json' },
  });
const esperar = (): Promise<void> =>
  act(async () => {
    for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0));
  });
const boton = (texto: string): HTMLButtonElement[] =>
  [...host.querySelectorAll('button')].filter((b) => b.textContent?.trim() === texto);

function solicitud(id: string, por: string, creada = 'otra'): Aprobacion {
  return {
    id,
    modelo: 'factura_compra',
    registro_id: `r-${id}`,
    importe: 5000000,
    moneda: 'EUR',
    estado: 'pendiente',
    solicitada_por: por,
    solicitada_en: '2026-11-09T10:00:00Z',
    creada_por: creada,
  };
}

test('cuatro ojos: no se ofrece resolver lo que una pidió o creó', () => {
  expect(puedeResolver(solicitud('a', 'luis'), 'yo')).toBe(true);
  expect(puedeResolver(solicitud('a', 'yo'), 'yo')).toBe(false);
  expect(puedeResolver(solicitud('a', 'luis', 'yo'), 'yo')).toBe(false);
  expect(puedeResolver({ ...solicitud('a', 'luis'), estado: 'aprobada' }, 'yo')).toBe(false);
});

async function pintarCola(puedeAdministrar = false): Promise<void> {
  await act(async () =>
    render(
      <ToastProvider>
        <Aprobaciones
          locale="es"
          t={t}
          yo="yo"
          puedeAprobar
          puedeAdministrar={puedeAdministrar}
          rotuloModelo={() => 'Facturas de compra'}
          onAbrir={() => {}}
          onCambio={() => {}}
        />
      </ToastProvider>,
      host,
    ),
  );
  await esperar();
}

test('la cola ofrece aprobar lo de otras personas y aprueba con un clic', async () => {
  responder = (metodo, ruta) =>
    metodo === 'POST'
      ? json({ ...solicitud('a1', 'luis'), estado: 'aprobada' })
      : ruta.includes('aprobaciones')
        ? json({ aprobaciones: [solicitud('a1', 'luis'), solicitud('a2', 'yo')] })
        : json({});
  await pintarCola();
  expect(pedidas[0]?.ruta).toContain('aprobaciones?estado=pendiente');
  // Dos filas, pero solo una se puede aprobar: la otra la pedí yo.
  expect(host.querySelectorAll('li').length).toBe(2);
  expect(boton(t('approvals.approve')).length).toBe(1);
  expect(host.textContent).toContain(t('approvals.waiting'));
  expect(host.textContent).toContain(t('approvals.byYou'));

  await act(async () => boton(t('approvals.approve'))[0]?.click());
  await esperar();
  expect(
    pedidas.some((p) => p.metodo === 'POST' && p.ruta.endsWith('aprobaciones/a1/aprobar')),
  ).toBe(true);
});

test('los umbrales se guardan en céntimos, y en blanco se quitan', async () => {
  responder = (metodo, ruta) =>
    metodo === 'PUT'
      ? new Response(null, { status: 204 })
      : ruta.includes('admin/umbrales')
        ? json({
            umbrales: [{ modulo: 'compras', importe: 1000000, moneda: 'EUR' }],
            moneda_por_defecto: 'EUR',
            modulos: [
              { clave: 'obligaciones', rotulo: 'Obligaciones' },
              { clave: 'compras', rotulo: 'Compras' },
              { clave: 'ventas', rotulo: 'Ventas' },
            ],
          })
        : json({ aprobaciones: [] });
  await pintarCola(true);
  // «Obligaciones» no admite umbral: no se ofrece.
  expect(host.querySelector('#umbral-obligaciones')).toBeNull();
  const ventas = host.querySelector<HTMLInputElement>('#umbral-ventas');
  const compras = host.querySelector<HTMLInputElement>('#umbral-compras');
  expect(compras?.value).toBe('10000.00');

  await act(async () => {
    ventas!.value = '2500,5';
    ventas!.dispatchEvent(new Event('input', { bubbles: true }));
  });
  const guardar = boton(t('action.save'));
  await act(async () => guardar[1]?.click());
  await esperar();
  const ponerVentas = pedidas.find((p) => p.metodo === 'PUT' && p.ruta.endsWith('umbrales/ventas'));
  expect(JSON.parse(ponerVentas?.cuerpo ?? '{}')).toEqual({ importe: 250050, moneda: 'EUR' });

  await act(async () => {
    compras!.value = '';
    compras!.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await act(async () => boton(t('action.save'))[0]?.click());
  await esperar();
  const quitar = pedidas.find((p) => p.metodo === 'PUT' && p.ruta.endsWith('umbrales/compras'));
  expect(JSON.parse(quitar?.cuerpo ?? '{}').importe).toBeLessThan(0);
});

const modelo: Modelo = {
  clave: 'factura_compra',
  modulo: 'compras',
  etiqueta: 'Facturas de compra',
  numerado: true,
  contable: true,
  campos: [{ clave: 'proveedor', tipo: 'texto', etiqueta: 'Proveedor' }],
  lista: ['proveedor'],
};
const registro: Registro = {
  id: 'f1',
  modelo: 'factura_compra',
  ref: 'FC-1',
  etiqueta: 'Proveedor grande',
  data: { proveedor: 'Proveedor grande' },
  creado_en: '2026-11-01T00:00:00Z',
  editado_en: '2026-11-01T00:00:00Z',
};

test('al asentar por encima del umbral, la ficha ofrece pedir aprobación y la pide', async () => {
  responder = (_metodo, ruta) =>
    ruta.endsWith('/asentar')
      ? json({ error: { code: 'requiere_aprobacion', message: 'Over threshold' } }, 409)
      : ruta.endsWith('/aprobacion')
        ? json(solicitud('n1', 'yo'), 201)
        : json({ asientos: [] });
  await act(async () =>
    render(
      <ToastProvider>
        <Ficha
          modelo={modelo}
          registro={registro}
          semilla={null}
          etiquetas={{}}
          locale="es"
          enPapelera={false}
          facturae={false}
          requisitos={null}
          soloLectura={false}
          onTareas={() => {}}
          onCerrar={() => {}}
          onDuplicar={() => {}}
          onConvertido={() => {}}
          onGuardado={() => {}}
          onInvalidar={() => {}}
          t={t}
        />
      </ToastProvider>,
      host,
    ),
  );
  await esperar();
  expect(boton(t('form.requestApproval')).length).toBe(0);
  await act(async () => boton(t('action.post'))[0]?.click());
  await esperar();
  expect(host.textContent).toContain(t('form.approvalNeeded'));
  await act(async () => boton(t('form.requestApproval'))[0]?.click());
  await esperar();
  expect(
    pedidas.some((p) => p.metodo === 'POST' && p.ruta.endsWith('factura_compra/f1/aprobacion')),
  ).toBe(true);
  expect(host.textContent).toContain(t('form.approvalRequested'));
  expect(boton(t('form.requestApproval')).length).toBe(0);
});
