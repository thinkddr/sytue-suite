/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Un cobro sin respuesta debe recuperarse con la misma petición, incluso al
 * recargar. Los rechazos confirmados dejan corregir y los permisos de ventas
 * permiten preparar un presupuesto sin dar acceso a emitir una factura.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeEach, expect, mock, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { Altas } from './AltasContext.js';
import type { Registro } from './tipos.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

// El doble sustituye IndexedDB, no los componentes ni la API. Conserva el
// aislamiento por cuenta y la copia estructurada del almacén compartido.
const almacen = new Map<string, unknown>();
const clave = (cuenta: string, espacio: string, id: string) =>
  JSON.stringify([cuenta, espacio, id]);
void mock.module('@sytue/offline', () => ({
  readOfflineValue: async <T,>(cuenta: string, espacio: string, id: string): Promise<T | null> => {
    const valor = almacen.get(clave(cuenta, espacio, id));
    return valor === undefined ? null : (structuredClone(valor) as T);
  },
  mutateOfflineValue: async <T,>(
    cuenta: string,
    espacio: string,
    id: string,
    actualizar: (anterior: T | null) => T | null,
  ): Promise<void> => {
    const k = clave(cuenta, espacio, id);
    const valor = almacen.get(k);
    const siguiente = actualizar(valor === undefined ? null : (structuredClone(valor) as T));
    if (siguiente === null) almacen.delete(k);
    else almacen.set(k, structuredClone(siguiente));
  },
}));

const { PuntoVenta } = await import('./PuntoVenta.js');
const { AltasContext } = await import('./AltasContext.js');
const { useT: traducir } = await import('../i18n/index.js');
const t = traducir('es');
const fetchOriginal = globalThis.fetch;

const cuentaPrincipal = 'organizacion-uno:persona-uno';
const otraCuenta = 'organizacion-dos:persona-uno';
const idProducto = '00000000-0000-4000-8000-000000000001';
const idSesion = '00000000-0000-4000-8000-000000000002';
const idCliente = '00000000-0000-4000-8000-000000000003';
const instante = '2026-10-03T10:00:00Z';
function registro(modelo: string, id: string, etiqueta: string, data: Registro['data']): Registro {
  return { modelo, id, etiqueta, data, creado_en: instante, editado_en: instante };
}
const producto = (precio = 1210) =>
  registro('producto', idProducto, 'Café de finca', {
    codigo: 'CAF-001',
    categoria: 'Café',
    precio_con_iva: precio,
    tipo_iva: '21',
  });
const sesion = registro('sesion_tpv', idSesion, 'Caja principal', { caja: 'Principal' });
const cliente = registro('contacto', idCliente, 'Aurora SL', { nombre: 'Aurora SL' });
const presupuesto = registro('presupuesto', 'presupuesto-uno', 'Oferta Aurora', {
  cliente: idCliente,
  total: 1210,
});

interface CuerpoCobro {
  operacion: string;
  sesion: string;
  cliente: string;
  medio: 'efectivo' | 'tarjeta';
  recibido: number;
  lineas: { producto: string; cantidad: number; precio: number; tipo_iva: string }[];
}
interface Llamada {
  ruta: string;
  metodo: string;
  cuerpo: unknown;
  consulta: string;
}
let llamadas: Llamada[] = [];
let inventario: Registro[] = [];
let imagenesGuardadas: Record<string, string> = {};
let respuestasCobro: (Response | Error)[] = [];
let ofertas: Registro[] = [];
let host: HTMLDivElement | null = null;
const json = (status: number, cuerpo: unknown) =>
  new Response(JSON.stringify(cuerpo), {
    status,
    headers: { 'content-type': 'application/json' },
  });
const cobros = (): CuerpoCobro[] =>
  llamadas
    .filter((l) => l.ruta === '/api/tpv/cobrar' && l.metodo === 'POST')
    .map((l) => l.cuerpo as CuerpoCobro);

beforeEach(() => {
  almacen.clear();
  llamadas = [];
  respuestasCobro = [];
  ofertas = [];
  inventario = [producto()];
  imagenesGuardadas = {};
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'https://erp.example.test');
    const metodo = init?.method ?? 'GET';
    const cuerpo: unknown =
      typeof init?.body === 'string' ? JSON.parse(init.body) : (init?.body ?? null);
    llamadas.push({ ruta: url.pathname, metodo, cuerpo, consulta: url.search });
    if (metodo === 'GET') {
      if (url.pathname === '/api/productos/imagenes')
        return json(200, { imagenes: imagenesGuardadas });
      if (url.pathname === '/api/producto') return json(200, { registros: inventario });
      if (url.pathname === '/api/sesion_tpv') return json(200, { registros: [sesion] });
      if (url.pathname === '/api/contacto') return json(200, { registros: [cliente] });
    }
    if (metodo === 'POST' && url.pathname === '/api/adjuntos') {
      imagenesGuardadas[url.searchParams.get('registro') ?? ''] =
        '00000000-0000-4000-8000-000000000099';
      return json(201, {
        adjunto: { id: '00000000-0000-4000-8000-000000000099' },
        tareas_hechas: 0,
      });
    }
    if (metodo === 'POST' && url.pathname === '/api/tpv/cobrar') {
      const respuesta = respuestasCobro.shift();
      if (respuesta instanceof Error) throw respuesta;
      if (respuesta) return respuesta;
      const cobro = cuerpo as CuerpoCobro;
      const total = cobro.lineas.reduce((suma, l) => suma + l.precio * l.cantidad, 0);
      return json(201, {
        factura: {
          ...registro('factura_venta', 'factura-uno', 'Factura', { total }),
          ref: 'FV-0001',
        },
        cambio: cobro.medio === 'efectivo' ? cobro.recibido - total : 0,
      });
    }
    if (metodo === 'POST' && url.pathname === '/api/tpv/presupuesto') return json(201, presupuesto);
    throw new Error(`Petición inesperada: ${metodo} ${url.pathname}`);
  }) as unknown as typeof fetch;
});

async function desmontar(): Promise<void> {
  if (!host) return;
  const actual = host;
  await act(async () => render(null, actual));
  actual.remove();
  host = null;
}
afterEach(async () => {
  await desmontar();
  document.body.innerHTML = '';
  globalThis.fetch = fetchOriginal;
});
afterAll(() => mock.restore());

// Los efectos reales de Preact y sus temporizadores terminan dentro de act;
// esperar una condición evita fijar pausas largas que escondan carreras.
async function esperar(condicion: () => boolean): Promise<void> {
  for (let i = 0; i < 40; i++) {
    if (condicion()) return;
    await act(async () => {
      await new Promise((resolver) => setTimeout(resolver, 0));
    });
  }
  expect(condicion()).toBe(true);
}
async function montar({
  cuenta = cuentaPrincipal,
  soloLectura = false,
  permitePresupuesto = false,
  puedeEditarProductos = false,
} = {}): Promise<void> {
  await desmontar();
  host = document.createElement('div');
  document.body.appendChild(host);
  const altas: Altas = {
    puedeCrear: (modelo) => permitePresupuesto && modelo === 'presupuesto',
    crear: () => {
      throw new Error('Esta prueba no solicita un alta rápida');
    },
  };
  await act(async () => {
    render(
      <AltasContext.Provider value={altas}>
        <PuntoVenta
          locale="es"
          t={t}
          vendedor="persona-uno"
          cuenta={cuenta}
          soloLectura={soloLectura}
          puedeEditarProductos={puedeEditarProductos}
          revision={0}
          onFactura={() => undefined}
          onProductos={() => undefined}
          onPresupuesto={(r) => ofertas.push(r)}
        />
      </AltasContext.Provider>,
      host!,
    );
  });
  await esperar(
    () =>
      Boolean(document.querySelector('.erp-tpv-product')) &&
      Boolean(document.querySelector('select[name="sesion"] option')),
  );
}
function obtener<E extends Element>(selector: string): E {
  const elemento = document.querySelector<E>(selector);
  if (!elemento) throw new Error(`Falta el control ${selector}`);
  return elemento;
}
function boton(texto: string): HTMLButtonElement {
  const encontrado = [...document.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === texto,
  );
  if (!encontrado) throw new Error(`Falta el botón «${texto}»`);
  return encontrado;
}
async function pulsar(elemento: HTMLElement): Promise<void> {
  await act(async () => {
    elemento.click();
  });
}
async function anadirProducto(): Promise<void> {
  const b = obtener<HTMLButtonElement>('.erp-tpv-product');
  await esperar(() => !b.disabled);
  await pulsar(b);
}
function checkout(): HTMLButtonElement {
  return obtener('.erp-tpv-payment button[type="submit"]');
}
async function enviar(): Promise<void> {
  await act(async () => {
    obtener<HTMLFormElement>('.erp-tpv-payment').dispatchEvent(
      new Event('submit', { bubbles: true, cancelable: true }),
    );
  });
}
async function elegirCliente(): Promise<void> {
  await pulsar(obtener<HTMLButtonElement>('#pos-cliente'));
  await esperar(() => Boolean(document.querySelector('[role="option"] button')));
  await pulsar(boton('Aurora SL'));
}
async function efectivo(): Promise<void> {
  await pulsar(obtener<HTMLInputElement>('input[name="medio"][value="efectivo"]'));
  await act(async () => {
    const entrada = obtener<HTMLInputElement>('input[name="recibido"]');
    entrada.value = '1000';
    entrada.dispatchEvent(new Event('input', { bubbles: true }));
  });
}
async function esperarVenta(): Promise<void> {
  await esperar(() => Boolean(document.querySelector('.erp-tpv-success')) && almacen.size === 0);
  expect(document.querySelector('[role="status"]')?.textContent).toContain(t('pos.recorded'));
}
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

test('una respuesta perdida mantiene la petición y el UUID al reintentar el cobro', async () => {
  respuestasCobro.push(new TypeError('Respuesta perdida'));
  await montar();
  await anadirProducto();
  expect(checkout().disabled).toBe(false);
  await enviar();
  await esperar(
    () =>
      cobros().length === 1 &&
      !checkout().disabled &&
      Boolean(document.querySelector('[role="alert"]')),
  );
  const primera = cobros()[0]!;
  expect(primera.operacion).toMatch(uuid);
  expect(primera).toEqual({
    operacion: primera.operacion,
    sesion: idSesion,
    cliente: '',
    medio: 'tarjeta',
    recibido: 1210,
    lineas: [{ producto: idProducto, cantidad: 1, precio: 1210, tipo_iva: '21' }],
  });
  expect(almacen.size).toBe(1);
  expect(obtener<HTMLButtonElement>('.erp-tpv-product').disabled).toBe(true);
  expect(obtener<HTMLSelectElement>('select[name="sesion"]').disabled).toBe(true);
  expect(obtener<HTMLButtonElement>('#pos-cliente').closest('fieldset')?.disabled).toBe(true);
  expect(document.querySelector('[role="status"]')?.textContent).toBe(t('pos.retryHint'));
  await enviar();
  await esperarVenta();
  expect(cobros()).toHaveLength(2);
  expect(cobros()[1]).toEqual(primera);
});

test('al remontar se recupera el cobro pendiente, sin exponerlo a otra cuenta', async () => {
  respuestasCobro.push(new TypeError('Respuesta perdida'));
  await montar();
  await anadirProducto();
  await enviar();
  await esperar(() => almacen.size === 1 && !checkout().disabled);
  const primera = cobros()[0]!;
  await montar({ cuenta: otraCuenta });
  expect(checkout().disabled).toBe(true);
  expect(obtener<HTMLButtonElement>('.erp-tpv-product').disabled).toBe(false);
  expect(document.querySelector('.erp-tpv-lines')?.textContent).toContain(t('pos.addProducts'));
  expect(document.body.textContent).not.toContain(t('pos.retryHint'));
  expect(cobros()).toHaveLength(1);
  await montar();
  await esperar(
    () => document.querySelector('[role="status"]')?.textContent === t('pos.retryHint'),
  );
  expect(document.querySelector('.erp-tpv-lines')?.textContent).toContain('Café de finca');
  expect(checkout().disabled).toBe(false);
  await enviar();
  await esperarVenta();
  expect(cobros()).toHaveLength(2);
  expect(cobros()[1]).toEqual(primera);
});

test('un 422 permite corregir la cesta y cobrar con un UUID nuevo', async () => {
  respuestasCobro.push(json(422, { error: { code: 'validation', message: 'Precio actualizado' } }));
  await montar();
  await anadirProducto();
  await enviar();
  await esperar(
    () =>
      document.querySelector('[role="alert"]')?.textContent === 'Precio actualizado' &&
      !obtener<HTMLButtonElement>('.erp-tpv-product').disabled,
  );
  expect(almacen.size).toBe(0);
  const primera = cobros()[0]!;
  await pulsar(boton('+'));
  await enviar();
  await esperarVenta();
  expect(cobros()).toHaveLength(2);
  const segunda = cobros()[1]!;
  expect(segunda.operacion).toMatch(uuid);
  expect(segunda.operacion).not.toBe(primera.operacion);
  expect(segunda.lineas[0]?.cantidad).toBe(2);
  expect(segunda.recibido).toBe(2420);
});

for (const estado of [401, 403, 429, 422]) {
  test(`un ${estado} después de perder la respuesta conserva el cobro original`, async () => {
    respuestasCobro.push(
      new TypeError('Respuesta perdida'),
      json(estado, { error: { code: 'denied', message: 'Reintenta más tarde' } }),
    );
    await montar();
    await anadirProducto();
    await enviar();
    await esperar(() => almacen.size === 1 && !checkout().disabled);
    const primera = cobros()[0]!;
    await enviar();
    await esperar(() => cobros().length === 2 && !checkout().disabled);
    expect(almacen.size).toBe(1);
    expect(obtener<HTMLButtonElement>('.erp-tpv-product').disabled).toBe(true);
    await montar();
    await esperar(() => !checkout().disabled);
    await enviar();
    await esperarVenta();
    expect(cobros()[1]).toEqual(primera);
    expect(cobros()[2]).toEqual(primera);
  });
}

test('se puede crear un presupuesto con permiso de ventas y sin permiso para facturar', async () => {
  await montar({ soloLectura: true, permitePresupuesto: true });
  await anadirProducto();
  await elegirCliente();
  expect(checkout().disabled).toBe(true);
  expect(boton(t('pos.quote')).disabled).toBe(false);
  await pulsar(boton(t('pos.quote')));
  await esperar(() => ofertas.length === 1);
  expect(ofertas).toEqual([presupuesto]);
  expect(cobros()).toHaveLength(0);
  expect(almacen.size).toBe(0);
  const peticion = llamadas.find((l) => l.ruta === '/api/tpv/presupuesto' && l.metodo === 'POST');
  expect(peticion?.cuerpo).toMatchObject({
    cliente: idCliente,
    lineas: [{ producto: idProducto, cantidad: 1, precio: 1210, tipo_iva: '21' }],
  });
});

for (const precio of [99999, 100000]) {
  test(`el efectivo ${precio === 99999 ? 'admite 999,99 €' : 'bloquea exactamente 1.000 €'}`, async () => {
    inventario = [producto(precio)];
    await montar();
    await anadirProducto();
    await elegirCliente();
    await efectivo();
    expect(checkout().disabled).toBe(precio === 100000);
    await enviar();
    if (precio === 100000) {
      expect(cobros()).toHaveLength(0);
      expect(almacen.size).toBe(0);
      expect(document.body.textContent).toContain(t('pos.cashLimit'));
    } else {
      await esperarVenta();
      expect(cobros()).toHaveLength(1);
      expect(cobros()[0]).toMatchObject({
        medio: 'efectivo',
        recibido: 100000,
        cliente: idCliente,
      });
    }
  });
}

test('el lector ve la imagen del servicio sin recibir controles de subida', async () => {
  const servicio = producto();
  servicio.data.tipo = 'servicio';
  inventario = [servicio];
  imagenesGuardadas[idProducto] = '00000000-0000-4000-8000-000000000099';
  await montar({ soloLectura: true });
  await esperar(() => Boolean(host?.querySelector('.erp-tpv-product-photo')));
  expect(host?.querySelector('.erp-tpv-product-photo')?.getAttribute('src')).toContain(
    '/api/adjuntos/00000000-0000-4000-8000-000000000099?ver=1',
  );
  expect(host?.querySelector('input[type="file"]')).toBeNull();
  expect(host?.querySelector('.erp-tpv-product-image-action')).toBeNull();
});

test('el editor de productos sube la imagen al registro sin modificar la venta', async () => {
  await montar({ soloLectura: true, puedeEditarProductos: true });
  expect(host?.querySelector('.erp-tpv-product-image-action')).toBeNull();
  const catalogo = [...host!.querySelectorAll<HTMLButtonElement>('button')].find((b) =>
    b.textContent?.includes(t('pos.catalogue')),
  )!;
  await act(async () => catalogo.click());
  const modo = [...document.querySelectorAll<HTMLButtonElement>('button')].find((b) =>
    b.textContent?.includes(t('pos.images')),
  )!;
  await act(async () => modo.click());
  await esperar(() => Boolean(host?.querySelector('.erp-tpv-product-image-action')));
  const subir = host!.querySelector<HTMLButtonElement>('.erp-tpv-product-image-action')!;
  expect(subir.disabled).toBe(false);
  await act(async () => subir.click());
  const input = host!.querySelector<HTMLInputElement>('input[type="file"]')!;
  const fichero = new File([new Uint8Array([137, 80, 78, 71])], 'servicio.png', {
    type: 'image/png',
  });
  Object.defineProperty(input, 'files', { configurable: true, value: [fichero] });
  await act(async () => {
    input.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await esperar(() => Boolean(host?.querySelector('.erp-tpv-product-photo')));
  const envio = llamadas.find((r) => r.ruta === '/api/adjuntos' && r.metodo === 'POST');
  const destino = new URLSearchParams(envio?.consulta);
  expect(destino.get('modelo')).toBe('producto');
  expect(destino.get('registro')).toBe(idProducto);
  expect(destino.get('nombre')).toBe('servicio.png');
  expect(host?.querySelector('.erp-tpv-quantity')).toBeNull();
  expect(cobros()).toHaveLength(0);
});
