/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA HOJA DE «?» ENSEÑA EXACTAMENTE LO QUE EL VISOR ATIENDE.
 *
 * El visor tenía Ctrl+F, el zoom, pasar de hoja, Escape y Ctrl+P, y no los enseñaba en
 * ninguna parte. Lo que se fija aquí: que «?» fuera de un campo abre la hoja; que cada
 * tecla de la hoja la atiende el visor montado y ninguna otra (la hoja y el oyente salen
 * de la misma tabla, y aquí se comprueba contra el oyente de verdad); y que la hoja está
 * en el idioma de quien lee, sin claves sin traducir.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { docsT } from '../i18n';
import type { Contenido } from './api';
import { GRUPOS_VISOR, accionDeTecla, gruposDeAtajos } from './atajos';
import { PdfApp } from './PdfApp';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = docsT('es');
const tick = (ms = 0): Promise<void> => new Promise((r) => setTimeout(r, ms));

const doc = {
  paginas: Array.from({ length: 6 }, (_, i) => ({
    numero: i + 1,
    ancho: 595,
    alto: 842,
    rotacion: 0,
    caja: [0, 0, 595, 842],
  })),
  total: 6,
  permisos: {
    Imprimir: true,
    Modificar: true,
    Copiar: true,
    Anotar: true,
    RellenarCampos: true,
    Accesibilidad: true,
    Ensamblar: true,
    ImprimirEnAlta: true,
    SoloConClaveDeDueno: false,
  },
  huella: {},
} as unknown as Contenido;

let montado: HTMLElement | null = null;
const fetchOriginal = globalThis.fetch;
const printOriginal = window.print;

const json = (cuerpo: unknown): Response =>
  new Response(JSON.stringify(cuerpo), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  });

beforeEach(() => {
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    if (String(url).includes('pdf-firmas')) return json({ received: [], sent: [] });
    const c = JSON.parse(String(init?.body ?? '{}')) as { op: string; paginas?: string };
    if (c.op === 'contenido') return json(doc);
    if (c.op === 'render') {
      const [a, b] = c.paginas!.split('-').map(Number) as [number, number];
      const paginas: Record<string, string> = {};
      for (let n = a; n <= b; n++) paginas[n] = `<svg data-n="${n}"></svg>`;
      return json({ paginas, total: doc.total });
    }
    return json({});
  }) as typeof fetch;
  window.print = () => undefined;
});

afterEach(() => {
  globalThis.fetch = fetchOriginal;
  window.print = printOriginal;
  if (montado) render(null, montado);
  montado = null;
  document.body.innerHTML = '';
});

async function abrir(): Promise<HTMLElement> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  montado = host;
  render(<PdfApp path="/contrato.pdf" locale="es" autor="Ana" />, host);
  for (let i = 0; i < 200 && !host.querySelector('[data-pagina="1"] svg[data-n]'); i++)
    await tick(5);
  return host;
}

function pulsar(key: string, ctrl = false, destino: EventTarget = document.body): KeyboardEvent {
  const e = new KeyboardEvent('keydown', { key, ctrlKey: ctrl, cancelable: true, bubbles: true });
  destino.dispatchEvent(e);
  return e;
}

const hoja = (): HTMLElement | null =>
  [...document.querySelectorAll<HTMLElement>('[role="dialog"]')].find((d) =>
    d.textContent?.includes(t('pdf.shortcuts.title')),
  ) ?? null;

test('«?» fuera de un campo abre la hoja con los tres grupos y sus teclas', async () => {
  await abrir();
  expect(hoja()).toBeNull();
  const e = pulsar('?');
  expect(e.defaultPrevented).toBe(true);
  await tick();
  const d = hoja();
  expect(d).not.toBeNull();
  for (const g of gruposDeAtajos(t)) {
    expect(d!.textContent).toContain(g.titulo);
    for (const a of g.atajos) expect(d!.textContent).toContain(a.descripcion);
  }
  const teclas = [...d!.querySelectorAll('kbd')].map((k) => k.textContent);
  expect(teclas).toContain('Ctrl+F');
  expect(teclas).toContain(t('slides.present.key.pageDown'));
  expect(teclas).toContain('?');
});

test('«?» escrito en un campo es un carácter, no la hoja', async () => {
  const host = await abrir();
  const campo = document.createElement('input');
  host.appendChild(campo);
  const e = pulsar('?', false, campo);
  await tick();
  expect(e.defaultPrevented).toBe(false);
  expect(hoja()).toBeNull();
});

test('cada tecla de la hoja la atiende el visor montado, y ninguna más', async () => {
  await abrir();
  const enganchadas = GRUPOS_VISOR.flatMap((g) =>
    g.atajos.flatMap((a) => a.combinaciones.map((c) => c.combinacion)),
  );
  for (const c of enganchadas) {
    // `?` abre la hoja, que es una capa: se cierra para que no tape la siguiente.
    const e = pulsar(c.key, c.ctrl);
    // Escape no se cancela a propósito (es también de los diálogos); se ve en su efecto.
    if (c.key !== 'Escape') expect(`${c.key} ${e.defaultPrevented}`).toBe(`${c.key} true`);
    await tick();
    if (c.key === '?') hoja()?.querySelector<HTMLButtonElement>('button')?.click();
    await tick();
  }
  // Lo que no está en la hoja sigue siendo del navegador.
  for (const [key, ctrl] of [
    ['s', true],
    ['j', false],
    ['ArrowDown', false],
    [' ', false],
    ['c', true],
  ] as const)
    expect(`${key} ${pulsar(key, ctrl).defaultPrevented}`).toBe(`${key} false`);
});

test('las filas sin combinación son las que hace el navegador: flechas, Espacio y Ctrl+C', () => {
  const delNavegador = GRUPOS_VISOR.flatMap((g) => g.atajos)
    .filter((a) => a.combinaciones.length === 0)
    .map((a) => a.descripcion);
  expect(delNavegador).toEqual([
    'pdf.shortcuts.scroll',
    'pdf.shortcuts.screen',
    'pdf.shortcuts.copy',
  ]);
});

test('accionDeTecla: Ctrl vale en un campo, las sueltas no, y con Alt nada', () => {
  const k = (key: string, o: Partial<{ ctrlKey: boolean; metaKey: boolean; altKey: boolean }>) => ({
    key,
    ctrlKey: false,
    metaKey: false,
    altKey: false,
    ...o,
  });
  expect(accionDeTecla(k('F', { ctrlKey: true }), true)).toBe('buscar');
  expect(accionDeTecla(k('=', { metaKey: true }), false)).toBe('acercar');
  expect(accionDeTecla(k('Home', {}), true)).toBeNull();
  expect(accionDeTecla(k('Home', {}), false)).toBe('primera');
  expect(accionDeTecla(k('?', {}), false)).toBe('ayuda');
  expect(accionDeTecla(k('0', { ctrlKey: true, altKey: true }), false)).toBeNull();
  // Sin Ctrl, «f» es una letra.
  expect(accionDeTecla(k('f', {}), false)).toBeNull();
});

test('la hoja está traducida en los 26 idiomas', () => {
  for (const l of [
    'bg',
    'cs',
    'da',
    'de',
    'el',
    'en',
    'es',
    'et',
    'fi',
    'fr',
    'ga',
    'hr',
    'hu',
    'is',
    'it',
    'lt',
    'lv',
    'mt',
    'nb',
    'nl',
    'pl',
    'pt',
    'ro',
    'sk',
    'sl',
    'sv',
  ] as const) {
    const tl = docsT(l);
    const textos = gruposDeAtajos(tl).flatMap((g) => [
      g.titulo,
      ...g.atajos.flatMap((a) => [a.descripcion, ...a.teclas]),
    ]);
    for (const x of [tl('pdf.shortcuts.title'), ...textos])
      expect(x).not.toMatch(/^(pdf|slides)\./);
  }
});
