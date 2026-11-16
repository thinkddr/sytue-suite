/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * IMPRIMIR ES IMPRIMIR EL DOCUMENTO ENTERO.
 *
 * Las hojas se dibujan por lotes según se llega a ellas, así que Ctrl+P a secas sacaba
 * la página del navegador con las cuatro primeras y la cinta encima. Lo que se fija
 * aquí: que antes de llamar a `print` se piden al motor las que faltan —en lotes, sin
 * repetir las que ya están— y que en el contenedor de papel están TODAS; que Ctrl+P
 * hace lo mismo que el botón; y que lo que el documento prohíbe no se imprime ni con
 * el atajo.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { docsT } from '../i18n';
import { rangosQueFaltan, type Contenido, type Permisos } from './api';
import { PdfApp } from './PdfApp';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = docsT('es');
const tick = (ms = 0): Promise<void> => new Promise((r) => setTimeout(r, ms));

const TODO: Permisos = {
  Imprimir: true,
  Modificar: true,
  Copiar: true,
  Anotar: true,
  RellenarCampos: true,
  Accesibilidad: true,
  Ensamblar: true,
  ImprimirEnAlta: true,
  SoloConClaveDeDueno: false,
};

function contenido(total: number, permisos: Permisos = TODO): Contenido {
  return {
    paginas: Array.from({ length: total }, (_, i) => ({
      numero: i + 1,
      ancho: 595,
      alto: 842,
      rotacion: 0,
      caja: [0, 0, 595, 842] as [number, number, number, number],
    })),
    total,
    indice: null,
    enlaces: null,
    adjuntos: null,
    anotaciones: null,
    campos: null,
    firmas: null,
    cifrado: false,
    permisos,
    huella: {
      Autor: '',
      Creador: '',
      Productor: '',
      Titulo: '',
      Asunto: '',
      Palabras: '',
      Creado: '',
      Modificado: '',
      ConXMP: false,
      ConAdjuntos: false,
      ConJS: false,
    },
  };
}

/** Los rangos que se han pedido al motor, en orden. */
let pedidos: string[] = [];
/** Cuántas hojas había en el contenedor de papel cada vez que se llamó a `print`. */
let alImprimir: number[] = [];
let doc: Contenido;
let montado: HTMLElement | null = null;
const fetchOriginal = globalThis.fetch;
const printOriginal = window.print;

function json(cuerpo: unknown): Response {
  return new Response(JSON.stringify(cuerpo), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  });
}

beforeEach(() => {
  pedidos = [];
  alImprimir = [];
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    if (String(url).includes('pdf-firmas')) return json({ received: [], sent: [] });
    const c = JSON.parse(String(init?.body ?? '{}')) as { op: string; paginas?: string };
    if (c.op === 'contenido') return json(doc);
    if (c.op === 'render') {
      pedidos.push(c.paginas!);
      const [a, b] = c.paginas!.split('-').map(Number) as [number, number];
      const paginas: Record<string, string> = {};
      for (let n = a; n <= b; n++)
        paginas[n] =
          `<svg data-n="${n}"><clipPath id="sypdf-h-p${n}-rec1"/><g clip-path="url(#sypdf-h-p${n}-rec1)"/></svg>`;
      return json({ paginas, total: doc.total });
    }
    return json({});
  }) as typeof fetch;
  // El espía cuenta lo que ESTÁ MONTADO en el momento de imprimir: imprimir antes de
  // tener las hojas es exactamente el fallo que se corrige.
  window.print = () => {
    alImprimir.push(document.querySelectorAll('[data-impresion] [data-hoja-impresa]').length);
  };
});

afterEach(() => {
  globalThis.fetch = fetchOriginal;
  window.print = printOriginal;
  // Desmontar de verdad: el atajo de teclado de una app que se queda colgada del
  // `document` imprimiría también en la prueba siguiente.
  if (montado) render(null, montado);
  montado = null;
  document.body.innerHTML = '';
});

async function abrir(d: Contenido): Promise<HTMLElement> {
  doc = d;
  const host = document.createElement('div');
  document.body.appendChild(host);
  montado = host;
  render(<PdfApp path="/contrato.pdf" locale="es" autor="Ana" />, host);
  // Abierto de verdad: con la cinta pintada y el primer lote ya en pantalla.
  for (let i = 0; i < 200 && !host.querySelector('[data-pagina="1"] svg[data-n]'); i++)
    await tick(5);
  return host;
}

async function irAOrganizar(host: HTMLElement): Promise<void> {
  const pestana = [...host.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find(
    (x) => x.textContent?.trim() === t('pdf.menu.organize'),
  )!;
  pestana.click();
  await tick();
}

const boton = (host: HTMLElement, texto: string): HTMLButtonElement | undefined =>
  [...host.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.textContent?.trim() === texto,
  );

async function hastaImprimir(): Promise<void> {
  for (let i = 0; i < 200 && alImprimir.length === 0; i++) await tick(5);
}

test('«Imprimir» pide las páginas que faltan y monta las veinte antes de llamar a print', async () => {
  const host = await abrir(contenido(20));
  // Abierto en la primera: el motor solo ha dibujado el primer lote.
  expect(pedidos).toEqual(['1-4']);
  await irAOrganizar(host);
  boton(host, t('pdf.menu.print'))!.click();
  await hastaImprimir();

  expect(pedidos).toEqual(['1-4', '5-8', '9-12', '13-16', '17-20']);
  expect(alImprimir).toEqual([20]);
  const hojas = [...document.querySelectorAll<HTMLElement>('[data-hoja-impresa]')];
  expect(hojas.map((h) => Number(h.dataset.hojaImpresa))).toEqual(
    Array.from({ length: 20 }, (_, i) => i + 1),
  );
  expect(hojas[19]!.querySelector('svg')?.getAttribute('data-n')).toBe('20');
  // Fuera del marco de la app, que en papel recorta al primer folio.
  expect(document.querySelector('[data-impresion]')?.parentElement).toBe(document.body);
  // Sus recursos son suyos: los de la copia de pantalla están apagados en papel.
  expect(hojas[19]!.querySelector('clipPath')?.id).toBe('sypdf-papel-h-p20-rec1');
  expect(hojas[19]!.querySelector('g')?.getAttribute('clip-path')).toBe(
    'url(#sypdf-papel-h-p20-rec1)',
  );
  // Lo de pantalla no va al papel: el panel y el documento a medio dibujar.
  expect(host.querySelector('#pdf-panel')?.closest('.sytue-no-print')).not.toBeNull();
  expect(document.querySelector('[data-impresion]')?.closest('.sytue-no-print')).toBeNull();

  // Al cerrar el diálogo de impresión las hojas se desmontan.
  window.dispatchEvent(new Event('afterprint'));
  await tick();
  expect(document.querySelector('[data-impresion]')).toBeNull();
});

test('Ctrl+P imprime el documento entero, no la página del navegador', async () => {
  await abrir(contenido(20));
  const e = new KeyboardEvent('keydown', { key: 'p', ctrlKey: true, cancelable: true });
  document.dispatchEvent(e);
  expect(e.defaultPrevented).toBe(true);
  await hastaImprimir();
  expect(alImprimir).toEqual([20]);
});

test('lo que el documento prohíbe imprimir no se imprime: ni el botón ni el atajo', async () => {
  const host = await abrir(contenido(6, { ...TODO, Imprimir: false, SoloConClaveDeDueno: true }));
  await irAOrganizar(host);
  const b = boton(host, t('pdf.menu.print'))!;
  expect(b.disabled).toBe(true);
  expect(b.title).toBe(t('pdf.blocked.print'));

  const e = new KeyboardEvent('keydown', { key: 'p', ctrlKey: true, cancelable: true });
  document.dispatchEvent(e);
  expect(e.defaultPrevented).toBe(true);
  await tick(20);
  expect(alImprimir).toEqual([]);
  expect(pedidos).toEqual(['1-4']);
  expect(host.querySelector('[role="alert"]')?.textContent).toContain(t('pdf.blocked.print'));
});

test('«Descargar» está en la cinta y baja el original de Drive', async () => {
  const host = await abrir(contenido(3));
  await irAOrganizar(host);
  const a = [...host.querySelectorAll<HTMLAnchorElement>('a')].find(
    (x) => x.textContent?.trim() === t('pdf.menu.download'),
  );
  expect(a).toBeDefined();
  expect(a!.getAttribute('href')).toContain(
    `/api/pdf-descargar?path=${encodeURIComponent('/contrato.pdf')}`,
  );
  expect(a!.hasAttribute('download')).toBe(true);
});

test('los rangos que faltan van en lotes y no repiten lo que ya está dibujado', () => {
  const ya = (ns: number[]): Record<number, string> =>
    Object.fromEntries(ns.map((n) => [n, '<svg/>']));
  expect(rangosQueFaltan(ya([1, 2, 3, 4]), 10, 4)).toEqual([
    [5, 8],
    [9, 10],
  ]);
  // Un hueco en medio (se pasó por la 6) corta el tramo.
  expect(rangosQueFaltan(ya([1, 6]), 8, 4)).toEqual([
    [2, 5],
    [7, 8],
  ]);
  // Una que el motor no supo dibujar ('') cuenta como dibujada: no se reintenta.
  expect(rangosQueFaltan({ 1: '', 2: '<svg/>' }, 2, 4)).toEqual([]);
  expect(rangosQueFaltan({}, 0, 4)).toEqual([]);
});
