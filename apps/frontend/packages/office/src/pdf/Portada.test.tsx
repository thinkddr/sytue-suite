/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA PORTADA DE CERTIPDF NO SE QUEDA SIN SALIDA.
 *
 * Si /api/pdf-recientes fallaba, la portada decía «No se pudo cargar» y no había más
 * que recargar la página; y mientras cargaba, una rueda en el hueco de las tarjetas.
 * Lo que se fija aquí: que mientras carga se ven esqueletos con la forma de la
 * tarjeta, que tras un fallo hay un «Reintentar» y que un fallo seguido de un éxito
 * pinta las tarjetas sin recargar.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { docsT } from '../i18n';
import { PortadaPdf } from './Portada';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = docsT('es');
const tick = (ms = 0): Promise<void> => new Promise((r) => setTimeout(r, ms));
// Preact corre los efectos tras pintar (requestAnimationFrame): se espera a que se
// cumpla la condición en lugar de adivinar cuántos milisegundos hacen falta.
async function hasta(cond: () => boolean, ms = 1000): Promise<void> {
  const fin = Date.now() + ms;
  while (!cond() && Date.now() < fin) await tick(5);
}

const fetchOriginal = globalThis.fetch;
// Cada llamada a /api/pdf-recientes toma la siguiente respuesta de la cola; lo demás
// (las peticiones de firma) contesta vacío para que no ocupe sitio.
let recientes: Array<() => Promise<Response>> = [];
let pedidas = 0;
let host: HTMLElement | null = null;

beforeEach(() => {
  recientes = [];
  pedidas = 0;
  globalThis.fetch = ((input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString();
    if (url.includes('/api/pdf-recientes')) {
      pedidas++;
      const siguiente = recientes.shift();
      return siguiente ? siguiente() : new Promise<Response>(() => undefined);
    }
    return Promise.resolve(Response.json({ received: [], sent: [], blocked: [] }));
  }) as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = fetchOriginal;
  // Desmontar de verdad: el efecto de una portada viva pediría la lista en el test
  // siguiente y se comería su respuesta.
  if (host) render(null, host);
  host = null;
  document.body.innerHTML = '';
});

function montar(): HTMLElement {
  document.body.innerHTML = '';
  const h = document.createElement('div');
  document.body.appendChild(h);
  render(<PortadaPdf locale="es" />, h);
  host = h;
  return h;
}

const botonReintentar = (h: HTMLElement): HTMLButtonElement | undefined =>
  [...h.querySelectorAll('button')].find((b) => b.textContent?.trim() === t('pdf.retry'));

test('mientras carga, seis esqueletos con la forma de la tarjeta y no una rueda', async () => {
  const raiz = montar();
  await tick();
  const cargando = raiz.querySelector('[data-recientes-cargando]');
  expect(cargando).not.toBeNull();
  expect(cargando?.getAttribute('aria-busy')).toBe('true');
  expect(cargando?.className).toContain('minmax(180px,1fr)');
  expect(cargando?.querySelectorAll('li[aria-hidden="true"]').length).toBe(6);
  expect(cargando?.querySelectorAll('.sytue-skeleton').length).toBe(18);
  expect(cargando?.textContent).toContain(t('list.loading'));
});

test('un fallo y luego un éxito pinta las tarjetas sin recargar', async () => {
  recientes.push(() => Promise.resolve(new Response('', { status: 502 })));
  recientes.push(() =>
    Promise.resolve(
      Response.json({
        items: [
          { name: 'Contrato.pdf', path: '/PDF/Contrato.pdf', modifiedAt: '2026-11-01T10:00:00Z' },
          { name: 'Factura.pdf', path: '/PDF/Factura.pdf' },
        ],
      }),
    ),
  );
  const raiz = montar();
  await hasta(() => botonReintentar(raiz) !== undefined);
  expect(raiz.textContent).toContain(t('pdf.list.loadFailed'));
  const boton = botonReintentar(raiz);
  expect(boton).toBeDefined();

  boton?.click();
  await hasta(() => raiz.querySelectorAll('a').length > 0);
  expect(pedidas).toBe(2);
  expect(raiz.textContent).not.toContain(t('pdf.list.loadFailed'));
  expect(botonReintentar(raiz)).toBeUndefined();
  const enlaces = [...raiz.querySelectorAll('a')].map((a) => a.textContent ?? '');
  expect(enlaces.some((x) => x.includes('Contrato'))).toBe(true);
  expect(enlaces.some((x) => x.includes('Factura'))).toBe(true);
});

test('al reintentar vuelve el esqueleto mientras llega la respuesta', async () => {
  recientes.push(() => Promise.reject(new Error('sin red')));
  const raiz = montar();
  await hasta(() => botonReintentar(raiz) !== undefined);
  botonReintentar(raiz)?.click();
  await tick();
  expect(raiz.querySelector('[data-recientes-cargando]')).not.toBeNull();
  expect(raiz.querySelector('[data-recientes-fallo]')).toBeNull();
});
