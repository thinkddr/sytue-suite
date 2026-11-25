/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los bloques del editor (panel, sección que se pliega, estado, decisiones, fecha,
 * columnas, código con lenguaje) y dónde está el cursor respecto a ellos.
 *
 * QUÉ HTML ESCRIBEN: el compartido con Fyne y con el saneador del servicio
 * (`BLOQUES_DE_NOTAS`). Se insertan con `insertHTML`, que entra en la pila de deshacer;
 * probado en Chromium: el bloque se coloca entre los párrafos sin romperlos. El texto
 * de las plantillas (el título de una sección, el de un estado) va escapado.
 *
 * `data-nuevo` es una marca de un instante: dice dónde poner el cursor después de
 * insertar y se quita en seguida (y el saneador la tiraría igual).
 */
import { BLOQUES_DE_NOTAS } from '@sytue/ui/browser/bloques-de-notas';
import { escapar } from '../client/captura.js';

export type TipoDePanel = 'info' | 'nota' | 'exito' | 'aviso' | 'error';
export const TIPOS_DE_PANEL = BLOQUES_DE_NOTAS.enumerados['div']![
  'data-panel'
] as readonly TipoDePanel[];
export type ColorDeBloque = (typeof BLOQUES_DE_NOTAS.coloresDeBloque)[number];
export const COLORES_DE_BLOQUE = BLOQUES_DE_NOTAS.coloresDeBloque;
export const LENGUAJES = BLOQUES_DE_NOTAS.enumerados['code']!['data-lenguaje']!;

const MARCA = 'data-nuevo';

/** El día de hoy en `AAAA-MM-DD`, en la hora de quien escribe. */
export function hoyISO(ahora = new Date()): string {
  const d = new Date(ahora.getTime() - ahora.getTimezoneOffset() * 60_000);
  return d.toISOString().slice(0, 10);
}

/** Cómo se lee una fecha `AAAA-MM-DD` en el idioma de la página («24 nov 2026»). */
export function fechaLegible(iso: string, idioma?: string): string {
  const [a, m, d] = iso.split('-').map(Number);
  const fecha = new Date(a!, (m ?? 1) - 1, d ?? 1);
  try {
    return fecha.toLocaleDateString(idioma || undefined, { dateStyle: 'medium' });
  } catch {
    return iso;
  }
}

export const plantillaPanel = (tipo: TipoDePanel): string =>
  `<div data-panel="${tipo}" ${MARCA}=""><p><br></p></div>`;

export const plantillaExpandir = (titulo: string): string =>
  `<details data-expandir="" open="" ${MARCA}=""><summary>${escapar(titulo)}</summary><p><br></p></details>`;

export const plantillaEstado = (texto: string, color: ColorDeBloque = 'gris'): string =>
  `<span data-estado="${color}" ${MARCA}="">${escapar(texto)}</span>&nbsp;`;

export const plantillaDecisiones = (): string =>
  `<ul data-tipo="decisiones" ${MARCA}=""><li><br></li></ul>`;

export const plantillaFecha = (iso: string, idioma?: string): string =>
  `<time datetime="${iso}" ${MARCA}="">${escapar(fechaLegible(iso, idioma))}</time>&nbsp;`;

export const plantillaColumnas = (n: 2 | 3): string =>
  `<div data-columnas="${n}" ${MARCA}="">${'<div data-columna=""><p><br></p></div>'.repeat(n)}</div>`;

/**
 * Pone el cursor dentro de lo recién insertado: en el título de una sección o en el texto
 * de un estado, seleccionado (para escribir encima); en lo demás, en su primer párrafo.
 */
export function entrarEnNuevo(raiz: HTMLElement): HTMLElement | null {
  const nuevo = raiz.querySelector<HTMLElement>(`[${MARCA}]`);
  if (!nuevo) return null;
  nuevo.removeAttribute(MARCA);
  const sel = document.getSelection();
  if (!sel) return nuevo;
  const r = document.createRange();
  const seleccionar = nuevo.matches('details')
    ? nuevo.querySelector('summary')
    : nuevo.matches('[data-estado]')
      ? nuevo
      : null;
  if (seleccionar) r.selectNodeContents(seleccionar);
  else if (nuevo.matches('time')) r.setStartAfter(nuevo.nextSibling ?? nuevo);
  else {
    const destino = nuevo.querySelector('p, li, code') ?? nuevo;
    r.selectNodeContents(destino);
    r.collapse(true);
  }
  sel.removeAllRanges();
  sel.addRange(r);
  return nuevo;
}

/** Lo que tiene barra propia, del más pequeño al más grande. */
export type Contexto =
  | { tipo: 'estado'; el: HTMLElement }
  | { tipo: 'fecha'; el: HTMLTimeElement }
  | { tipo: 'codigo'; el: HTMLPreElement }
  | { tipo: 'tabla'; el: HTMLTableElement; celda: HTMLTableCellElement }
  | { tipo: 'panel'; el: HTMLElement }
  | { tipo: 'expandir'; el: HTMLDetailsElement };

/** El bloque con barra propia en el que está `nodo`, dentro de `raiz`; o null. */
export function contextoDe(nodo: Node | null, raiz: HTMLElement): Contexto | null {
  const el = nodo?.nodeType === Node.TEXT_NODE ? nodo.parentElement : (nodo as Element | null);
  if (!el || !raiz.contains(el)) return null;
  const dentro = <E extends Element>(sel: string) => {
    const x = el.closest<E>(sel);
    return x && raiz.contains(x) && (x as Element) !== raiz ? x : null;
  };
  const estado = dentro<HTMLElement>('[data-estado]');
  if (estado) return { tipo: 'estado', el: estado };
  const fecha = dentro<HTMLTimeElement>('time');
  if (fecha) return { tipo: 'fecha', el: fecha };
  const pre = dentro<HTMLPreElement>('pre');
  if (pre) return { tipo: 'codigo', el: pre };
  const celda = dentro<HTMLTableCellElement>('td, th');
  const tabla = celda?.closest('table');
  if (celda && tabla) return { tipo: 'tabla', el: tabla, celda };
  const panel = dentro<HTMLElement>('[data-panel]');
  if (panel) return { tipo: 'panel', el: panel };
  const expandir = dentro<HTMLDetailsElement>('details');
  if (expandir) return { tipo: 'expandir', el: expandir };
  return null;
}

/** El `<code>` de un bloque de código, creándolo si el `<pre>` aún no lo tiene. */
export function codigoDe(pre: HTMLPreElement): HTMLElement {
  const hijo = pre.firstElementChild;
  if (hijo?.tagName === 'CODE' && pre.childNodes.length === 1) return hijo as HTMLElement;
  const code = document.createElement('code');
  while (pre.firstChild) code.append(pre.firstChild);
  pre.append(code);
  return code;
}

/** Cuántas líneas tiene un bloque de código (lo que numera `data-lineas`). */
export function lineasDe(pre: HTMLElement): number {
  const texto = (pre.innerText ?? pre.textContent ?? '').replace(/\n$/, '');
  return Math.max(1, texto.split('\n').length);
}

/**
 * Los números de línea de los bloques de código que los llevan, en la LECTURA: un
 * `<span>` al principio de cada `<pre data-lineas>`, fuera del alcance del lector de
 * pantalla y de la selección. Es la vista, no la nota: se rehace cada vez que se pinta.
 * En el editor van en una capa aparte (RichText), porque ahí el `<pre>` se guarda tal cual.
 */
export function numerarCodigo(raiz: HTMLElement | null): void {
  if (!raiz) return;
  for (const pre of raiz.querySelectorAll<HTMLElement>('pre[data-lineas]')) {
    pre.querySelector(':scope > .notes-lineas')?.remove();
    const n = lineasDe(pre);
    const gutter = document.createElement('span');
    gutter.className = 'notes-lineas';
    gutter.setAttribute('aria-hidden', 'true');
    gutter.textContent = Array.from({ length: n }, (_, k) => k + 1).join('\n');
    pre.prepend(gutter);
  }
}
