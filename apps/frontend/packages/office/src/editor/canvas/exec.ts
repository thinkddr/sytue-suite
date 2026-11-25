/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * exec — comandos de edición del lienzo (BLUEPRINT.md §3). El HTML del
 * contenteditable ES el modelo: cada comando actúa sobre la selección con
 * el adaptador compartido de edición nativa y, donde éste no llega (interlineado,
 * tablas, índice), con Selection/Range a mano. Módulo sin componentes; su único estado global es
 * el root editable activo y el último Range conocido dentro de él.
 */
import { rangos } from '@sytue/i18n';
import { EMPTY_FORMAT } from '../types';
import type { EditorCommand, FormatState } from '../types';
import {
  editingCommandEnabled,
  editingCommandState,
  editingCommandValue,
  executeEditingCommand,
} from '@sytue/ui/browser/editing';
import { recordStep } from '../macros';
import {
  acceptTrackedChanges,
  deletionHtml,
  insertionHtml,
  rejectTrackedChanges,
  resolveTrackedChange,
  type TrackAuthor,
} from '../trackchanges';
import { getSelectedImage } from './imageDom';
import { cambioDeMayusculas, esModoMayusculas, type ModoMayusculas } from '../mayusculas';
import { editorT } from '../../i18n';
import {
  dejarSeleccionEnCasillas,
  elementoDeCasilla,
  marcarComoCasillas,
  quitarCasillas,
} from './casillas';

// ponytail: execCommand está deprecado pero es interoperable y sin dependencias;
// migrar a un modelo propio de Range solo si un navegador soportado lo retira.

let root: HTMLElement | null = null;
let savedRange: Range | null = null;

/* Memoria de selección (§3): si la selección vive dentro del root se clona el
 * Range para poder restaurarlo cuando un control del ribbon robe el foco.
 * Guardado por SSR: el módulo también se importa al renderizar en servidor. */
if (typeof document !== 'undefined') {
  document.addEventListener('selectionchange', () => {
    const rootEl = root;
    if (!rootEl) return;
    const sel = document.getSelection();
    if (sel && sel.rangeCount > 0 && sel.anchorNode && rootEl.contains(sel.anchorNode)) {
      savedRange = sel.getRangeAt(0).cloneRange();
    }
  });
}

/**
 * Registra el root editable activo. PageCanvas lo llama al montar (y con null
 * al desmontar); SheetCanvas/SlidesCanvas registran la celda/caja enfocada.
 */
export function setEditorRoot(el: HTMLElement | null): void {
  if (el === root) return;
  root = el;
  savedRange = null; // un Range del root anterior ya no vale
  if (el) {
    // Estilos como CSS inline y párrafo <p> por defecto (§3, obligatorio).
    try {
      executeEditingCommand('styleWithCSS', 'true');
    } catch {
      /* navegador sin soporte: no-op */
    }
    try {
      executeEditingCommand('defaultParagraphSeparator', 'p');
    } catch {
      /* ídem */
    }
  }
}

export function getEditorRoot(): HTMLElement | null {
  return root;
}

/** Reemplaza (o quita, con html vacío) una capa del folio —la página, la marca de agua, el
 *  borde— y emite `input` para el autosave. La capa va como PRIMER hijo (fuera del flujo,
 *  no molesta al caret). La usan la cinta (Disposición) y la barra de estado (el idioma). */
export function ponerCapaDelFolio(selector: string, html: string): void {
  if (!root) return;
  root.querySelector(selector)?.remove();
  if (html) root.insertAdjacentHTML('afterbegin', html);
  dispatchInput();
}

/* ---------- utilidades internas ---------- */

/** execCommand con red de seguridad; false = el navegador no lo aplicó. */
function cmd(name: string, value?: string): boolean {
  return executeEditingCommand(name, value);
}

/** Los caminos manuales (Range) no emiten `input` nativo: se emite a mano. */
function dispatchInput(): void {
  root?.dispatchEvent(new Event('input', { bubbles: true }));
}

/** Range vivo de la selección si cae dentro del root; null en caso contrario. */
function selectionRange(): Range | null {
  const rootEl = root;
  if (!rootEl) return null;
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0) return null;
  const range = sel.getRangeAt(0);
  return rootEl.contains(range.commonAncestorContainer) ? range : null;
}

/** Devuelve el foco (y el último Range) al lienzo si un control se lo llevó. */
function restoreSelection(): void {
  const rootEl = root;
  if (!rootEl) return;
  const active = document.activeElement;
  if (active && rootEl.contains(active)) return; // el foco sigue dentro
  rootEl.focus({ preventScroll: true });
  const range = savedRange;
  if (range && rootEl.contains(range.commonAncestorContainer)) {
    const sel = document.getSelection();
    if (sel) {
      sel.removeAllRanges();
      sel.addRange(range);
    }
  }
}

/** closest() acotado al root: los ancestros fuera del lienzo no cuentan. */
function closestInRoot(node: Node | null | undefined, selector: string): HTMLElement | null {
  const rootEl = root;
  if (!rootEl || !node) return null;
  const el = node instanceof Element ? node : node.parentElement;
  const found = el?.closest<HTMLElement>(selector) ?? null;
  return found && rootEl.contains(found) ? found : null;
}

/** Coloca el caret (colapsado) al principio de un elemento tras mutar tablas. */
function placeCaretIn(el: Element | null | undefined): void {
  if (!el) return;
  const range = document.createRange();
  range.selectNodeContents(el);
  range.collapse(true);
  const sel = document.getSelection();
  if (sel) {
    sel.removeAllRanges();
    sel.addRange(range);
  }
  savedRange = range.cloneRange();
}

const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};

function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);
}

/* ---------- Cotejo de texto (Buscar y reemplazar, §6.4) ---------- */

export interface MatchOptions {
  /** Distinguir mayúsculas/minúsculas (por defecto: no). */
  caseSensitive?: boolean;
  /** Solo apariciones que son palabra completa (por defecto: no). */
  wholeWord?: boolean;
}

/** Carácter de palabra (Unicode: cubre acentos y ñ del español). */
const WORD_CHAR = /[\p{L}\p{N}_]/u;

/**
 * Índices `[inicio, fin)` de cada aparición NO solapada de `needle` en
 * `haystack` según las opciones. Función pura (sin DOM): findTextRanges la
 * aplica por nodo de texto y el self-test la cubre directamente.
 */
export function matchIndices(
  haystack: string,
  needle: string,
  opts: MatchOptions = {},
): [number, number][] {
  const out: [number, number][] = [];
  if (needle === '') return out;
  // Sin «coincidir mayúsculas», tampoco mira acentos: «reunion» encuentra «reunión»
  // (la regla de toda la suite, `rangos` de @sytue/i18n).
  const pares: [number, number][] = [];
  if (opts.caseSensitive) {
    for (
      let i = haystack.indexOf(needle);
      i !== -1;
      i = haystack.indexOf(needle, i + needle.length)
    ) {
      pares.push([i, i + needle.length]);
    }
  } else {
    pares.push(...rangos(haystack, needle));
  }
  const hay = haystack;
  for (const [i, end] of pares) {
    if (opts.wholeWord) {
      // ponytail: la frontera se evalúa dentro del nodo; en el borde se toma como
      // límite de palabra (coherente con el cotejo por nodo de findTextRanges, que
      // no ve coincidencias partidas entre nodos). Un matcher entre nodos llegará con la RGA.
      const before = hay[i - 1] ?? ''; // fuera de rango (borde) = frontera de palabra
      const after = hay[end] ?? '';
      if (WORD_CHAR.test(before) || WORD_CHAR.test(after)) continue;
    }
    out.push([i, end]);
  }
  return out;
}

/* ---------- Ayudas puras (sin DOM: las cubre el self-test) ---------- */

/**
 * Texto del campo de número de página (1-indexado, clamp ≥1).
 *
 * En el idioma de la interfaz, como el resto del editor: PageCanvas lo reescribe
 * en cada cambio de página, así que un literal castellano pisaba el rótulo ya
 * traducido que insertaba la cinta y dejaba «Página 1 de 3» dentro de un
 * documento en inglés, francés, alemán o italiano. La frase es la MISMA que la de
 * la barra de estado (`status.page`), que ya vive traducida en los cinco.
 */
export function pageFieldText(page: number, pages: number): string {
  const y = Math.max(1, Math.floor(pages) || 1);
  const x = Math.min(y, Math.max(1, Math.floor(page) || 1));
  return editorT()('status.page', { page: x, pages: y });
}

/** Posición de una celda en la rejilla de la tabla (fila/columna + spans). */
export interface GridRect {
  r: number;
  c: number;
  rs: number;
  cs: number;
}

/**
 * Rectángulo envolvente de un conjunto de celdas (por posición de rejilla) SOLO
 * si teselan un rectángulo sólido, sin huecos ni solapes (condición para poder
 * fusionar). null si el conjunto está vacío o no forma un rectángulo limpio.
 */
export function rectFromCells(
  cells: GridRect[],
): { r0: number; c0: number; r1: number; c1: number } | null {
  if (cells.length === 0) return null;
  let r0 = Infinity;
  let c0 = Infinity;
  let r1 = -Infinity;
  let c1 = -Infinity;
  for (const cell of cells) {
    r0 = Math.min(r0, cell.r);
    c0 = Math.min(c0, cell.c);
    r1 = Math.max(r1, cell.r + cell.rs - 1);
    c1 = Math.max(c1, cell.c + cell.cs - 1);
  }
  const rectArea = (r1 - r0 + 1) * (c1 - c0 + 1);
  // Cobertura exacta: cada casilla del rectángulo ocupada por UNA sola celda.
  const seen = new Set<string>();
  for (const cell of cells) {
    for (let r = cell.r; r < cell.r + cell.rs; r += 1) {
      for (let c = cell.c; c < cell.c + cell.cs; c += 1) {
        const k = `${r},${c}`;
        if (seen.has(k)) return null; // solape
        seen.add(k);
      }
    }
  }
  return seen.size === rectArea ? { r0, c0, r1, c1 } : null;
}

/** Etiqueta de leyenda: «Tabla N» / «Figura N» (n clamp ≥1). */
export type CaptionKind = 'tabla' | 'figura';
export function captionLabel(kind: CaptionKind, n: number): string {
  const etiqueta = editorT()(kind === 'figura' ? 'caption.figure' : 'caption.table');
  return `${etiqueta} ${Math.max(1, Math.floor(n) || 1)}`;
}

/** Bloques que admiten interlineado (§3.1: lineHeight por bloque). */
const BLOCK_SELECTOR = 'p,h1,h2,h3,blockquote,li,td,th';

/** Bloques alcanzados por la selección actual (uno solo si está colapsada). */
function blocksInSelection(): HTMLElement[] {
  const rootEl = root;
  if (!rootEl) return [];
  const range = selectionRange();
  if (!range) return [];
  if (!range.collapsed) {
    const hit = Array.from(rootEl.querySelectorAll<HTMLElement>(BLOCK_SELECTOR)).filter((el) =>
      range.intersectsNode(el),
    );
    if (hit.length > 0) return hit;
  }
  const single = closestInRoot(range.startContainer, BLOCK_SELECTOR);
  return single ? [single] : [];
}

/**
 * El peldaño de `fontSize` («1»…«7») de al lado: Ctrl+Mayús+. sube uno y Ctrl+Mayús+,
 * baja uno, como en Word y Docs de Google, sin pasar de los extremos. Indeterminado («»)
 * cuenta como la talla normal, «3». La app nativa hace lo mismo (`docus.pasoDeTalla`).
 */
export function pasoDeTalla(actual: string, dir: 1 | -1): string {
  const n = /^[1-7]$/.test(actual) ? Number(actual) : 3;
  return String(Math.min(7, Math.max(1, n + dir)));
}

/**
 * «Ordenar párrafos»: los bloques elegidos, de la A a la Z (o al revés) por su texto, con
 * la colación del idioma del documento. Solo si son HERMANOS —párrafos seguidos o
 * elementos de una misma lista—: mezclar una lista con los párrafos de alrededor la
 * rompería. Estable: los iguales quedan en el orden en que estaban. La app nativa lo hace
 * con `rico.OrdenarBloques`, con los mismos vectores (`docus-ordenar.json`).
 */
function ordenarParrafos(desc: boolean): void {
  const bloques = blocksInSelection();
  const padre = bloques[0]?.parentElement;
  if (bloques.length < 2 || !padre || bloques.some((b) => b.parentElement !== padre)) return;
  // El idioma del documento (el `lang` del folio, que es el suyo o el de la interfaz).
  const idioma = padre.closest<HTMLElement>('[lang]')?.lang || undefined;
  const orden = ordenDeTextos(
    bloques.map((b) => textoDeBloque(b)),
    desc,
    idioma,
  ).map((i) => bloques[i]!);
  if (orden.every((b, i) => b === bloques[i])) return;
  const ref = bloques[bloques.length - 1]!.nextSibling;
  for (const b of orden) padre.insertBefore(b, ref);
  // Lo ordenado sigue elegido: mover los nodos deja la selección en cualquier parte, y
  // así «de la Z a la A» justo después da la vuelta a lo mismo.
  const sel = document.getSelection();
  if (sel) {
    const r = document.createRange();
    r.setStartBefore(orden[0]!);
    r.setEndAfter(orden[orden.length - 1]!);
    sel.removeAllRanges();
    sel.addRange(r);
  }
  dispatchInput();
}

const textoDeBloque = (b: Element): string => (b.textContent ?? '').replace(/\s+/g, ' ').trim();

/** Los índices de `textos` en el orden de «Ordenar párrafos» (puro, para los vectores). */
export function ordenDeTextos(textos: readonly string[], desc: boolean, idioma?: string): number[] {
  const signo = desc ? -1 : 1;
  return textos
    .map((_, i) => i)
    .sort((a, b) => signo * textos[a]!.localeCompare(textos[b]!, idioma));
}

/** '<F>x<C>' → tabla F×C de celdas vacías + párrafo de escape posterior (§3.1). */
function tableHtml(spec: string): string | null {
  const m = /^(\d{1,2})\s*[x×]\s*(\d{1,2})$/i.exec(spec.trim());
  const rows = Math.min(Number(m?.[1] ?? 0), 20);
  const cols = Math.min(Number(m?.[2] ?? 0), 20);
  if (rows < 1 || cols < 1) return null;
  const row = `<tr>${'<td><br></td>'.repeat(cols)}</tr>`;
  return `<table><tbody>${row.repeat(rows)}</tbody></table><p><br></p>`;
}

/** Regenera el índice: ids estables `h-<n>` y un único <nav data-toc> (§3.1). */
function insertToc(): void {
  const rootEl = root;
  if (!rootEl) return;
  rootEl.querySelector('nav[data-toc]')?.remove();
  let n = 0;
  const items = Array.from(rootEl.querySelectorAll<HTMLElement>('h1,h2,h3')).map((head) => {
    if (!head.id) {
      // Contador con salto de ids ya ocupados: regenerar el índice no rompe anclas previas.
      do {
        n += 1;
      } while (rootEl.querySelector(`[id="h-${n}"]`) !== null);
      head.id = `h-${n}`;
    }
    const level = head.tagName === 'H1' ? '1' : head.tagName === 'H2' ? '2' : '3';
    return `<li data-level="${level}"><a href="#${escapeHtml(head.id)}">${escapeHtml(
      head.textContent ?? '',
    )}</a></li>`;
  });
  cmd(
    'insertHTML',
    `<nav data-toc><p><b>${editorT()('ref.toc')}</b></p><ol>${items.join('')}</ol></nav>`,
  );
  dispatchInput(); // la retirada del nav previo y los ids nuevos no emiten input solos
}

type TableCommand =
  | 'tableRowAbove'
  | 'tableRowBelow'
  | 'tableColLeft'
  | 'tableColRight'
  | 'tableDelRow'
  | 'tableDelCol'
  | 'tableDelete';

/* Capas decorativas del folio: marca de agua y borde de página (pagedecor.ts).
 * Van como primer hijo, fuera del flujo y contenteditable=false: nunca son
 * vecinos de caret válidos ni cuentan como «contenido» del documento. */
const DECOR_LAYER_SEL = '[data-sytue-watermark],[data-sytue-page-border]';
const CONTENT_CHILD_SEL = ':scope > :not([data-sytue-watermark]):not([data-sytue-page-border])';

/** Elimina la tabla entera dejando el caret en un vecino de CONTENIDO y el folio
 *  con al menos un párrafo editable. Ignora las capas decorativas (que no son
 *  vecinos ni «contenido») y las preserva: nunca borra el folio entero. */
function removeTable(table: HTMLTableElement): void {
  const isContent = (el: Element | null): el is HTMLElement =>
    el instanceof HTMLElement && !el.matches(DECOR_LAYER_SEL);
  const neighbour =
    [table.nextElementSibling, table.previousElementSibling].find(isContent) ?? null;
  table.remove();
  const rootEl = root;
  // Sin contenido (solo quedan capas, o nada): añade un párrafo editable SIN
  // borrar las capas decorativas (a diferencia de un innerHTML que las tiraría).
  if (rootEl && !rootEl.querySelector(CONTENT_CHILD_SEL)) {
    rootEl.insertAdjacentHTML('beforeend', '<p><br></p>');
  }
  placeCaretIn(neighbour ?? rootEl?.querySelector<HTMLElement>(CONTENT_CHILD_SEL));
}

/** Operaciones estructurales de tabla vía Range (§3.1). Sin selección en tabla → no-op. */
function tableOp(command: TableCommand): void {
  const rootEl = root;
  if (!rootEl) return;
  const range = selectionRange() ?? savedRange;
  const cell = closestInRoot(range?.startContainer, 'td,th') as HTMLTableCellElement | null;
  const rowEl = cell?.closest<HTMLTableRowElement>('tr') ?? null;
  const table = rowEl?.closest<HTMLTableElement>('table') ?? null;
  if (!cell || !rowEl || !table || !rootEl.contains(table)) return;

  switch (command) {
    case 'tableRowAbove':
    case 'tableRowBelow': {
      const tr = document.createElement('tr');
      for (let i = 0; i < rowEl.cells.length; i += 1) {
        const td = document.createElement('td');
        td.innerHTML = '<br>';
        tr.append(td);
      }
      rowEl.insertAdjacentElement(command === 'tableRowAbove' ? 'beforebegin' : 'afterend', tr);
      placeCaretIn(tr.cells[0]);
      break;
    }
    case 'tableColLeft':
    case 'tableColRight': {
      const at = cell.cellIndex + (command === 'tableColRight' ? 1 : 0);
      for (const row of Array.from(table.rows)) {
        const td = row.insertCell(Math.min(at, row.cells.length));
        td.innerHTML = '<br>';
      }
      placeCaretIn(cell);
      break;
    }
    case 'tableDelRow': {
      if (table.rows.length <= 1) {
        removeTable(table); // borrar la última fila elimina la tabla (§3.1)
        break;
      }
      const neighbour = rowEl.nextElementSibling ?? rowEl.previousElementSibling;
      rowEl.remove();
      placeCaretIn(neighbour instanceof HTMLTableRowElement ? neighbour.cells[0] : neighbour);
      break;
    }
    case 'tableDelCol': {
      if (rowEl.cells.length <= 1) {
        removeTable(table); // borrar la última columna elimina la tabla (§3.1)
        break;
      }
      const at = cell.cellIndex;
      for (const row of Array.from(table.rows)) {
        if (row.cells.length > 1) row.deleteCell(Math.min(at, row.cells.length - 1));
      }
      placeCaretIn(rowEl.cells[Math.min(at, rowEl.cells.length - 1)]);
      break;
    }
    case 'tableDelete':
      removeTable(table);
      break;
  }
  dispatchInput();
}

/* ---------- Combinar / dividir celdas + estilo de celda (§ tabla) ---------- */

interface CellPos extends GridRect {
  cell: HTMLTableCellElement;
}

/** Mapa celda→posición de rejilla, honrando colSpan/rowSpan (para fusión/división). */
function tableGrid(table: HTMLTableElement): Map<HTMLTableCellElement, CellPos> {
  const occ: boolean[][] = [];
  const positions = new Map<HTMLTableCellElement, CellPos>();
  const rows = Array.from(table.rows);
  for (let r = 0; r < rows.length; r += 1) {
    occ[r] ??= [];
    let c = 0;
    for (const cell of Array.from(rows[r]!.cells)) {
      while (occ[r]![c]) c += 1;
      const rs = Math.max(1, cell.rowSpan || 1);
      const cs = Math.max(1, cell.colSpan || 1);
      positions.set(cell, { cell, r, c, rs, cs });
      for (let dr = 0; dr < rs; dr += 1) {
        occ[r + dr] ??= [];
        for (let dc = 0; dc < cs; dc += 1) occ[r + dr]![c + dc] = true;
      }
      c += cs;
    }
  }
  return positions;
}

/** Celdas alcanzadas por la selección (dentro de UNA tabla); [] si no hay tabla. */
function selectedCells(): HTMLTableCellElement[] {
  const rootEl = root;
  const range = selectionRange() ?? savedRange;
  const anchor = closestInRoot(range?.startContainer, 'td,th') as HTMLTableCellElement | null;
  const table = anchor?.closest('table');
  if (!rootEl || !range || !anchor || !table || !rootEl.contains(table)) {
    return anchor ? [anchor] : [];
  }
  if (range.collapsed) return [anchor];
  const cells = Array.from(table.querySelectorAll<HTMLTableCellElement>('td,th')).filter((c) =>
    range.intersectsNode(c),
  );
  return cells.length > 0 ? cells : [anchor];
}

/** Fusiona las celdas seleccionadas si forman un rectángulo sólido (colSpan/rowSpan). */
function mergeCells(): void {
  const cells = selectedCells();
  if (cells.length < 2) return;
  const table = cells[0]!.closest<HTMLTableElement>('table');
  if (!table) return;
  const grid = tableGrid(table);
  const rects = cells.map((c) => grid.get(c)).filter((p): p is CellPos => p !== undefined);
  const rect = rectFromCells(rects);
  if (!rect) return; // no forman rectángulo limpio: no-op (evita corromper la tabla)
  const anchor = rects.find((p) => p.r === rect.r0 && p.c === rect.c0)?.cell;
  if (!anchor) return;
  for (const c of cells) {
    if (c === anchor) continue;
    const inner = c.innerHTML.trim();
    if (inner !== '' && inner !== '<br>') anchor.insertAdjacentHTML('beforeend', ` ${c.innerHTML}`);
    c.remove();
  }
  anchor.rowSpan = rect.r1 - rect.r0 + 1;
  anchor.colSpan = rect.c1 - rect.c0 + 1;
  placeCaretIn(anchor);
  dispatchInput();
}

/** Divide la celda fusionada bajo el cursor: reparte a 1×1 recreando las celdas vacías. */
function splitCell(): void {
  const anchor = selectedCells()[0];
  const table = anchor?.closest<HTMLTableElement>('table');
  if (!anchor || !table) return;
  const rs = Math.max(1, anchor.rowSpan || 1);
  const cs = Math.max(1, anchor.colSpan || 1);
  if (rs === 1 && cs === 1) return; // nada que dividir
  const start = tableGrid(table).get(anchor);
  if (!start) return;
  const tag = anchor.tagName.toLowerCase() as 'td' | 'th';
  anchor.rowSpan = 1;
  anchor.colSpan = 1;
  // Las columnas [c0..c0+cs-1] de las filas [r0..r0+rs-1] quedan libres y contiguas:
  // se rellenan de izquierda a derecha, recalculando el índice DOM en cada paso.
  for (let dr = 0; dr < rs; dr += 1) {
    const rowEl = table.rows[start.r + dr];
    if (!rowEl) continue;
    for (let dc = 0; dc < cs; dc += 1) {
      if (dr === 0 && dc === 0) continue; // el ancla ya ocupa su casilla
      const gc = start.c + dc;
      const grid = tableGrid(table); // refleja las celdas ya insertadas
      let domIdx = 0;
      for (const cell of Array.from(rowEl.cells)) {
        if ((grid.get(cell)?.c ?? Infinity) < gc) domIdx += 1;
      }
      const fresh = document.createElement(tag);
      fresh.innerHTML = '<br>';
      rowEl.insertBefore(fresh, rowEl.cells[domIdx] ?? null);
    }
  }
  placeCaretIn(anchor);
  dispatchInput();
}

/** Relleno o alineación vertical de las celdas seleccionadas ('transparent'/'' quita). */
function setCellProp(prop: 'background-color' | 'vertical-align', value: string): void {
  const cells = selectedCells();
  if (cells.length === 0) return;
  for (const c of cells) {
    if (value === '' || value === 'transparent') c.style.removeProperty(prop);
    else c.style.setProperty(prop, value);
  }
  dispatchInput();
}

/** Toggles inline que deben emitir etiqueta semántica (<b>/<i>/<u>/<strike>)
 *  en vez de <span> con CSS: exporta e interopera mejor y es lo que verifica
 *  el humo e2e. Con el caret colapsado Chromium decide el markup al TECLEAR,
 *  no al conmutar, así que styleWithCSS debe QUEDARSE apagado tras el toggle;
 *  cada exec() fija al entrar el modo que su comando necesita (el resto lo
 *  quiere encendido: la regla lee sangrías inline en px, §3). */
const SEMANTIC_INLINE = new Set([
  'bold',
  'italic',
  'underline',
  'strikeThrough',
  'superscript',
  'subscript',
]);

/** Comandos 1:1 con execCommand, sin valor. */
const PLAIN_COMMANDS: Partial<Record<EditorCommand, string>> = {
  bold: 'bold',
  italic: 'italic',
  underline: 'underline',
  strike: 'strikeThrough',
  superscript: 'superscript',
  subscript: 'subscript',
  clearFormat: 'removeFormat',
  alignLeft: 'justifyLeft',
  alignCenter: 'justifyCenter',
  alignRight: 'justifyRight',
  alignJustify: 'justifyFull',
  listBullet: 'insertUnorderedList',
  listNumber: 'insertOrderedList',
  indent: 'indent',
  outdent: 'outdent',
  insertHr: 'insertHorizontalRule',
  undo: 'undo',
  redo: 'redo',
};

/** Comandos 1:1 con execCommand que exigen valor (fuente, talla, color, imagen). */
const VALUE_COMMANDS: Partial<Record<EditorCommand, string>> = {
  font: 'fontName',
  size: 'fontSize',
  color: 'foreColor',
  insertImage: 'insertImage',
};

/**
 * Ejecuta un comando sobre la selección del lienzo (§3). Si el foco salió del
 * root (p. ej. hacia un select del ribbon), restaura primero el último Range
 * conocido. Los caminos manuales (Range) terminan emitiendo `input` sobre el
 * root; los de execCommand ya emiten input nativo.
 */
export function exec(command: EditorCommand, value?: string): void {
  if (!root) return;
  recordStep('document', command, value); // no-op salvo grabación de macro activa
  ejecutar(command, value);
}

/** exec sin grabar: los pasos internos de un comando compuesto («Copiar formato») no son
 *  pasos de la macro. */
function ejecutar(command: EditorCommand, value?: string): void {
  if (!root) return;
  restoreSelection();

  // Modo de markup del comando (ver SEMANTIC_INLINE): se deja puesto a propósito.
  const plain = PLAIN_COMMANDS[command];
  cmd('styleWithCSS', plain !== undefined && SEMANTIC_INLINE.has(plain) ? 'false' : 'true');

  if (plain) {
    cmd(plain);
    return;
  }
  const withValue = VALUE_COMMANDS[command];
  if (withValue) {
    if (value) cmd(withValue, value);
    return;
  }

  switch (command) {
    case 'highlight': {
      const colour = value || 'transparent'; // 'transparent' retira el resaltado
      if (!cmd('hiliteColor', colour)) cmd('backColor', colour); // fallback (§3.1)
      break;
    }
    case 'lineHeight': {
      if (!value) break;
      const blocks = blocksInSelection();
      for (const block of blocks) block.style.lineHeight = value;
      if (blocks.length > 0) dispatchInput();
      break;
    }
    case 'heading': {
      const tag = value && /^(h[1-3]|blockquote)$/i.test(value) ? value.toUpperCase() : 'P';
      cmd('formatBlock', `<${tag}>`);
      break;
    }
    case 'insertTable': {
      const html = tableHtml(value ?? '');
      if (html) cmd('insertHTML', html);
      break;
    }
    case 'insertLink': {
      if (!value) break;
      // Frontera de confianza: sin esquemas peligrosos (javascript:); lo demás se normaliza a https.
      const url = /^(https?:|mailto:|#)/i.test(value) ? value : `https://${value}`;
      const sel = document.getSelection();
      if (sel && !sel.isCollapsed && selectionRange()) {
        cmd('createLink', url);
      } else {
        // Selección colapsada: createLink no tiene qué envolver → ancla explícita (§3.1).
        cmd('insertHTML', `<a href="${escapeHtml(url)}">${escapeHtml(url)}</a>`);
      }
      break;
    }
    case 'insertToc':
      insertToc();
      break;
    case 'tableRowAbove':
    case 'tableRowBelow':
    case 'tableColLeft':
    case 'tableColRight':
    case 'tableDelRow':
    case 'tableDelCol':
    case 'tableDelete':
      tableOp(command);
      break;
    case 'tableMergeCells':
      mergeCells();
      break;
    case 'tableSplitCell':
      splitCell();
      break;
    case 'tableCellBg':
      setCellProp('background-color', value ?? '');
      break;
    case 'tableCellValign':
      setCellProp('vertical-align', value ?? '');
      break;
    case 'changeCase':
      if (esModoMayusculas(value)) cambiarMayusculas(value);
      break;
    case 'fontGrow':
    case 'fontShrink': {
      // Sin talla única bajo el cursor (una selección con dos) se parte de la normal.
      const actual = editingCommandValue('fontSize');
      const siguiente = pasoDeTalla(actual, command === 'fontGrow' ? 1 : -1);
      if (siguiente !== actual) cmd('fontSize', siguiente);
      break;
    }
    case 'sortParagraphs':
      ordenarParrafos(value === 'desc');
      break;
    case 'listCheck':
      listaDeCasillas();
      break;
    default:
      break; // el resto de EditorCommand lo cubren los mapas de arriba
  }
}

/* ---------- Lista de casillas (Inicio › Párrafo) ---------- */

/**
 * Pone o quita la lista de casillas, como los botones de viñetas y numerada: en una lista
 * de casillas la deshace en párrafos; en una de viñetas la pasa entera a casillas; en lo
 * demás (párrafos, una numerada) hace la lista y la marca. Los atributos no pasan por
 * execCommand: el `input` se emite a mano.
 */
function listaDeCasillas(): void {
  const rootEl = root;
  const range = selectionRange();
  if (!rootEl || !range) return;
  const li = closestInRoot(range.startContainer, 'li');
  const lista = li?.parentElement;
  if (li && lista && elementoDeCasilla(rootEl, li)) {
    quitarCasillas(lista);
    cmd('insertUnorderedList'); // de viñetas a párrafos
  } else if (lista?.tagName === 'UL') {
    marcarComoCasillas(lista, null, false);
  } else {
    cmd('insertUnorderedList');
    // Con varios párrafos seleccionados, todos sus elementos (no solo el del cursor).
    dejarSeleccionEnCasillas(rootEl, selectionRange());
  }
  dispatchInput();
}

/* ---------- Cambiar mayúsculas y minúsculas (Inicio › Fuente) ---------- */

/** Los bloques que abren párrafo para el cambio de mayúsculas: frase y palabra nuevas. */
const PARRAFO_SELECTOR = `${BLOCK_SELECTOR},h4,h5,h6,div`;

/** Un trozo de la selección: [desde, hasta) del texto de un nodo. */
interface TramoDeTexto {
  nodo: Text;
  desde: number;
  hasta: number;
}

/**
 * Transforma las mayúsculas del texto seleccionado sin tocar el marcado: cada nodo de
 * texto conserva su largo (`mayusculas.ts`), así que el formato de cada letra sigue en su
 * sitio y la selección sigue abarcando lo mismo. Lo no editable (citas, campos de
 * combinación) no se toca: su texto lo genera el editor.
 *
 * Con la selección dentro de UN nodo —una palabra, una frase sin cambios de formato, el
 * caso de casi siempre— va por `insertText`, que entra en el deshacer del navegador. Si
 * cruza nodos, cada uno se reescribe a mano, como el interlineado o las tablas: un
 * `insertText` por nodo serían tantos pasos de deshacer como trozos.
 */
function cambiarMayusculas(modo: ModoMayusculas): void {
  const rootEl = root;
  const range = selectionRange();
  if (!rootEl || !range || range.collapsed) return;
  const cambio = cambioDeMayusculas(modo);
  // El párrafo de donde arranca la selección, y su texto hasta ella (ver `previo`).
  const primerParrafo = closestInRoot(range.startContainer, PARRAFO_SELECTOR);
  const antes = document.createRange();
  antes.setStart(primerParrafo ?? rootEl, 0);
  antes.setEnd(range.startContainer, range.startOffset);
  const tramos: TramoDeTexto[] = [];
  const nuevos: string[] = [];
  let parrafo: HTMLElement | null | undefined;
  const comun = range.commonAncestorContainer;
  const walker = document.createTreeWalker(
    comun.nodeType === Node.TEXT_NODE ? (comun.parentNode ?? comun) : comun,
    NodeFilter.SHOW_TEXT | NodeFilter.SHOW_ELEMENT,
  );
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    if (!range.intersectsNode(n)) continue;
    const otro = closestInRoot(n, PARRAFO_SELECTOR);
    if (otro !== parrafo) {
      cambio.parrafo();
      if (parrafo === undefined && otro === primerParrafo) cambio.previo(antes.toString());
      parrafo = otro;
    }
    if (!(n instanceof Text)) {
      // Un salto de línea o una imagen separan palabras (y un salto, frases), como el
      // '\n' y el objeto de las runas de la app nativa.
      if (n.nodeName === 'BR') cambio.tramo('\n');
      else if (n.nodeName === 'IMG') cambio.tramo('￼');
      continue;
    }
    if (closestInRoot(n, '[contenteditable="false"]')) continue;
    const desde = n === range.startContainer ? range.startOffset : 0;
    const hasta = n === range.endContainer ? range.endOffset : n.length;
    if (desde >= hasta) continue;
    tramos.push({ nodo: n, desde, hasta });
    nuevos.push(cambio.tramo(n.data.slice(desde, hasta)));
  }
  const primero = tramos[0];
  const ultimo = tramos[tramos.length - 1];
  if (!primero || !ultimo) return;
  if (tramos.length === 1 && insertarEnTramo(primero, nuevos[0]!)) return;
  tramos.forEach((t, i) => {
    const nuevo = nuevos[i]!;
    if (nuevo !== t.nodo.data.slice(t.desde, t.hasta))
      t.nodo.replaceData(t.desde, t.hasta - t.desde, nuevo);
  });
  // replaceData encoge los rangos vivos que acababan dentro de lo sustituido: se repone la
  // selección entera, que mide lo mismo que antes.
  const sel = document.getSelection();
  if (sel) {
    const r = document.createRange();
    r.setStart(primero.nodo, primero.desde);
    r.setEnd(ultimo.nodo, ultimo.hasta);
    sel.removeAllRanges();
    sel.addRange(r);
    savedRange = r.cloneRange();
  }
  dispatchInput();
}

/** El camino con deshacer: selecciona el tramo y lo sustituye con `insertText`. */
function insertarEnTramo(t: TramoDeTexto, nuevo: string): boolean {
  const sel = document.getSelection();
  if (!sel) return false;
  const r = document.createRange();
  r.setStart(t.nodo, t.desde);
  r.setEnd(t.nodo, t.hasta);
  sel.removeAllRanges();
  sel.addRange(r);
  if (!cmd('insertText', nuevo)) return false;
  // El cursor queda tras lo escrito: se vuelve a seleccionar, como un procesador de textos.
  const fin = sel.rangeCount > 0 ? sel.getRangeAt(0) : null;
  if (fin && fin.startContainer instanceof Text && fin.startOffset >= nuevo.length) {
    sel.setBaseAndExtent(
      fin.startContainer,
      fin.startOffset - nuevo.length,
      fin.startContainer,
      fin.startOffset,
    );
  }
  return true;
}

/* ---------- Copiar formato (la brocha de Inicio) ---------- */

/** El formato de carácter que «Copiar formato» se lleva: lo que se VE, no las etiquetas. */
export interface FormatoDeCaracter {
  bold: boolean;
  italic: boolean;
  underline: boolean;
  strike: boolean;
  superscript: boolean;
  subscript: boolean;
  /** `fontName` tal cual lo da el navegador; '' si no lo sabe. */
  font: string;
  /** Peldaño '1'..'7' de `fontSize`; '' si no es ninguno. */
  size: string;
  /** Color del texto como `rgb(r, g, b)`; '' si no lo sabe. */
  color: string;
  /** Resaltado como `rgb(r, g, b)`; '' sin resaltado. */
  highlight: string;
}

type Marca = 'bold' | 'italic' | 'underline' | 'strike' | 'superscript' | 'subscript';
const MARCAS_DE_CARACTER: readonly Marca[] = [
  'bold',
  'italic',
  'underline',
  'strike',
  'superscript',
  'subscript',
];

/** Un color de queryCommandValue sin espacios; '' si es transparente (sin resaltado). */
export function normalizarColor(v: string): string {
  const c = v.replace(/\s+/g, '').toLowerCase();
  if (c === '' || c === 'transparent' || /^rgba\(.*,0(\.0*)?\)$/.test(c)) return '';
  return c.replace(/,/g, ', ');
}

/**
 * Los comandos que dejan `destino` (lo que hay tras «Borrar formato») como `fuente`.
 * Solo lo que difiere: un párrafo de un título ya es negrita y no hay que ponérsela, y
 * el color del folio no se escribe como un `<span>` de más. Las marcas son conmutadores:
 * primero se apagan las que sobran y luego se encienden las que faltan, para que el
 * superíndice no vuelva a encender el subíndice que acaba de apagar. Puro: lo prueba
 * docs-brocha.test.ts sin navegador.
 */
export function pasosDeFormato(
  fuente: FormatoDeCaracter,
  destino: FormatoDeCaracter,
): [EditorCommand, string?][] {
  const pasos: [EditorCommand, string?][] = [];
  for (const m of MARCAS_DE_CARACTER) if (destino[m] && !fuente[m]) pasos.push([m]);
  for (const m of MARCAS_DE_CARACTER) if (fuente[m] && !destino[m]) pasos.push([m]);
  const familia = (f: string): string => f.replace(/["']/g, '').trim().toLowerCase();
  if (fuente.font && familia(fuente.font) !== familia(destino.font))
    pasos.push(['font', fuente.font]);
  if (fuente.size && fuente.size !== destino.size) pasos.push(['size', fuente.size]);
  if (fuente.color && fuente.color !== destino.color) pasos.push(['color', fuente.color]);
  if (fuente.highlight !== destino.highlight)
    pasos.push(['highlight', fuente.highlight || 'transparent']);
  return pasos;
}

function leerFormatoDeCaracter(): FormatoDeCaracter {
  return {
    bold: editingCommandState('bold'),
    italic: editingCommandState('italic'),
    underline: editingCommandState('underline'),
    strike: editingCommandState('strikeThrough'),
    superscript: editingCommandState('superscript'),
    subscript: editingCommandState('subscript'),
    font: editingCommandValue('fontName'),
    size: editingCommandValue('fontSize'),
    color: normalizarColor(editingCommandValue('foreColor')),
    highlight: normalizarColor(
      editingCommandValue('hiliteColor') || editingCommandValue('backColor'),
    ),
  };
}

/** El formato de carácter al principio de la selección del lienzo; null sin lienzo. */
export function copiarFormato(): FormatoDeCaracter | null {
  if (!root || !(selectionRange() ?? savedRange)) return null;
  restoreSelection();
  return leerFormatoDeCaracter();
}

/**
 * Pone `formato` en la selección: «Borrar formato» y luego lo que falte para igualarlo.
 * Todo por execCommand, así que cada paso entra en el deshacer del navegador. Devuelve
 * false si no hay texto seleccionado (no hay dónde pintar).
 */
export function pegarFormato(formato: FormatoDeCaracter): boolean {
  if (!root) return false;
  restoreSelection();
  const range = selectionRange();
  if (!range || range.collapsed) return false;
  // Sin grabar: la macro no sabría reproducir «el formato de lo que se copió entonces».
  ejecutar('clearFormat');
  for (const [c, v] of pasosDeFormato(formato, leerFormatoDeCaracter())) ejecutar(c, v);
  return true;
}

/* ---------- Reemplazo de una coincidencia (Buscar y reemplazar, §6.4) ---------- */

/**
 * Sustituye el texto de `range` (una coincidencia del panel Buscar) por
 * `replacement` vía execCommand('insertText'), de modo que la sustitución entra
 * en el historial nativo de undo y emite `input` (autosave). El texto insertado
 * hereda el formato del entorno del rango. Cadena vacía = borrar. Devuelve false
 * si el rango no vive en el root. SearchPanel encadena varios (reemplazar todo)
 * en orden inverso de documento, para que los offsets previos sigan siendo válidos.
 */
export function replaceRange(range: Range, replacement: string): boolean {
  const rootEl = root;
  if (!rootEl || !rootEl.contains(range.commonAncestorContainer)) return false;
  const sel = document.getSelection();
  if (!sel) return false;
  if (!rootEl.contains(document.activeElement)) rootEl.focus({ preventScroll: true });
  sel.removeAllRanges();
  sel.addRange(range);
  return replacement === '' ? cmd('delete') : cmd('insertText', replacement);
}

/* ---------- Asistente de IA: leer selección / insertar texto ---------- */

/**
 * Texto de la selección del lienzo (restaura el último Range conocido si un panel
 * robó el foco); '' si está colapsada o no hay selección dentro del root. Solo
 * LEE: no muta el DOM ni mueve el caret. El panel de IA lo usa como contexto.
 */
export function getSelectionText(): string {
  const rootEl = root;
  if (!rootEl) return '';
  const range =
    selectionRange() ??
    (savedRange && rootEl.contains(savedRange.commonAncestorContainer) ? savedRange : null);
  return range && !range.collapsed ? range.toString() : '';
}

/**
 * Inserta `text` como TEXTO PLANO vía execCommand (entra en el undo nativo y emite
 * `input` → autosave; plano = la respuesta de la IA nunca inyecta HTML). Con
 * replace=true sustituye la selección; con replace=false la colapsa al final e
 * inserta tras ella (o en el caret). Devuelve false si no hay root editable.
 */
export function insertText(text: string, replace: boolean): boolean {
  const rootEl = root;
  if (!rootEl) return false;
  restoreSelection(); // el panel pudo robar el foco: recupera el último Range
  if (!replace) document.getSelection()?.collapseToEnd();
  return cmd('insertText', text);
}

/* ---------- Columnas de texto por sección (Disposición, §6.3) ---------- */

/** Ancestro de `node` que es hijo directo del folio (la unidad de «sección»). */
function topChild(rootEl: HTMLElement, node: Node | null): HTMLElement | null {
  let el: Element | null = node instanceof Element ? node : (node?.parentElement ?? null);
  while (el && el.parentElement && el.parentElement !== rootEl) el = el.parentElement;
  return el && el.parentElement === rootEl ? (el as HTMLElement) : null;
}

/**
 * Aplica columnas (n = 1..3) al tramo de bloques de nivel superior tocado por la
 * selección: los envuelve en un `<div class="sytue-cols">` con `column-count`
 * inline. n = 1 deshace (sube los hijos y quita el envoltorio). Va en el HTML del
 * folio —que ES el modelo—, así que persiste con el autosave y viaja en la
 * exportación; emite `input` para dispararlo (camino manual, como §3.1).
 * ponytail: sección = tramo contiguo de hijos directos del folio; una selección
 * que cruce niveles (dentro de una lista/tabla) sube a esos bloques superiores.
 */
export function setColumns(n: number): void {
  const rootEl = root;
  if (!rootEl) return;
  restoreSelection();
  const range = selectionRange() ?? savedRange;
  if (!range) return;
  const cols = Math.min(3, Math.max(1, Math.floor(n)));

  const kids = Array.from(rootEl.children) as HTMLElement[];
  const startEl = topChild(rootEl, range.startContainer);
  const endEl = topChild(rootEl, range.endContainer);
  if (!startEl || !endEl) return;
  const i = kids.indexOf(startEl);
  const j = kids.indexOf(endEl);
  if (i === -1 || j === -1) return;
  const run = kids.slice(Math.min(i, j), Math.max(i, j) + 1);
  const first = run[0];
  if (!first) return; // inalcanzable: i,j son índices válidos ⇒ el tramo no está vacío

  // ¿La sección ya ES un envoltorio de columnas? Entonces se ajusta o se deshace.
  const wrapper = run.length === 1 && first.classList.contains('sytue-cols') ? first : null;
  if (wrapper) {
    if (cols === 1) {
      const firstInner = wrapper.firstElementChild;
      const parent = wrapper.parentNode;
      while (wrapper.firstChild) parent?.insertBefore(wrapper.firstChild, wrapper);
      wrapper.remove();
      placeCaretIn(firstInner ?? rootEl.firstElementChild);
    } else {
      wrapper.style.columnCount = String(cols);
      wrapper.dataset.cols = String(cols);
      placeCaretIn(wrapper.firstElementChild ?? wrapper);
    }
    dispatchInput();
    return;
  }
  if (cols === 1) return; // sin columnas y se pide 1: nada que hacer

  const div = document.createElement('div');
  div.className = 'sytue-cols';
  div.dataset.cols = String(cols);
  div.style.columnCount = String(cols);
  div.style.columnGap = '2em';
  first.parentNode?.insertBefore(div, first);
  for (const el of run) div.appendChild(el);
  placeCaretIn(div.firstElementChild ?? div);
  dispatchInput();
}

/* ---------- estado de formato (§3.3) ---------- */

function qcs(name: string): boolean {
  return editingCommandState(name);
}

function qcv(name: string): string {
  return editingCommandValue(name);
}

function qce(name: string): boolean {
  return editingCommandEnabled(name);
}

/**
 * Estado de formato bajo el cursor, para reflejarlo en el ribbon;
 * EMPTY_FORMAT si no hay selección dentro del root (§3.3).
 */
export function getFormatState(): FormatState {
  const rootEl = root;
  if (!rootEl) return EMPTY_FORMAT;
  // La imagen se selecciona por clic (no como selección de texto): su estado va
  // aparte para que el grupo Imagen del ribbon aparezca aunque no haya rango.
  const imageSelected = getSelectedImage() !== null;
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0 || !sel.anchorNode || !rootEl.contains(sel.anchorNode)) {
    return { ...EMPTY_FORMAT, imageSelected };
  }
  const block = qcv('formatBlock').toLowerCase();
  const casillas = elementoDeCasilla(rootEl, sel.anchorNode) !== null;
  const align: FormatState['align'] = qcs('justifyCenter')
    ? 'center'
    : qcs('justifyRight')
      ? 'right'
      : qcs('justifyFull')
        ? 'justify'
        : 'left';
  return {
    bold: qcs('bold'),
    italic: qcs('italic'),
    underline: qcs('underline'),
    strike: qcs('strikeThrough'),
    font: qcv('fontName').replace(/["']/g, ''),
    size: qcv('fontSize'),
    align,
    bullet: qcs('insertUnorderedList') && !casillas,
    number: qcs('insertOrderedList'),
    checklist: casillas,
    heading:
      block === 'h1' || block === 'h2' || block === 'h3' || block === 'blockquote' ? block : 'p',
    inTable: closestInRoot(sel.anchorNode, 'table') !== null,
    imageSelected,
    canUndo: qce('undo'),
    canRedo: qce('redo'),
  };
}

/* ---------- Comentarios anclados a la selección ---------- */
// El ancla vive en el HTML del folio (que ES el modelo): un <mark data-comment-id>
// envuelve lo comentado y el autosave lo persiste. Sin CRDT por carácter: el
// resaltado se re-localiza por su id de marca al recargar (findTextRanges de
// respaldo para anclas antiguas que solo traían texto citado).

/**
 * Envuelve la selección viva del lienzo en `<mark data-comment-id=markId>` y
 * devuelve el texto citado (espacios normalizados), o null si no hay selección
 * de texto dentro del root. Emite `input` para que PageCanvas dispare el
 * autosave (camino manual: execCommand no interviene).
 */
export function wrapSelectionForComment(markId: string): string | null {
  const rootEl = root;
  if (!rootEl) return null;
  restoreSelection(); // el ribbon pudo robar el foco: recupera el último Range
  const range = selectionRange();
  if (!range || range.collapsed) return null;
  const text = range.toString().replace(/\s+/g, ' ').trim();
  if (text === '') return null;
  const mark = document.createElement('mark');
  mark.dataset.commentId = markId;
  try {
    range.surroundContents(mark);
  } catch {
    // surroundContents lanza si el rango parte un nodo no-Text (selección que
    // cruza fronteras de elementos): se envuelve el contenido extraído.
    mark.append(range.extractContents());
    range.insertNode(mark);
  }
  savedRange = null; // el Range guardado ya no describe el DOM tras envolver
  dispatchInput();
  return text;
}

/**
 * Retira el `<mark>` de un comentario (al resolver o al cancelar la creación):
 * sustituye el elemento por sus hijos y normaliza el texto. Emite `input` para
 * que el autosave persista el folio ya sin resaltado. No-op si no existe.
 */
export function removeCommentMark(markId: string): void {
  const rootEl = root;
  if (!rootEl) return;
  // markId es hex generado por nosotros; no necesita escape de selector.
  const mark = rootEl.querySelector<HTMLElement>(`mark[data-comment-id="${markId}"]`);
  const parent = mark?.parentNode;
  if (!mark || !parent) return;
  while (mark.firstChild) parent.insertBefore(mark.firstChild, mark);
  parent.removeChild(mark);
  if (parent instanceof Element) parent.normalize();
  dispatchInput();
}

/* ---------- Control de cambios (track changes) ---------- */
// Con el modo ACTIVO, la edición NO se aplica directa: se MARCA en el HTML (que
// ES el modelo). PageCanvas intercepta `beforeinput`, hace preventDefault y llama
// aquí. El texto tecleado/pegado se envuelve en <ins data-track> (trackchanges.ts,
// escape del autor incluido) y lo borrado en <del data-track> (nunca se quita: el
// caret lo salta). Las marcas contiguas del mismo autor se funden EN EL SITIO
// (coalesceMark: mueve hijos, O(1)) para no reserializar el bloque en cada tecla
// (evita O(n²) en párrafos largos). Cada operación emite `input` (autosave).
// Consolidar las marcas (aceptar/rechazar) es resolveTrackedChanges, cáscara
// sobre los helpers PUROS y probados accept/rejectTrackedChanges (trackchanges.ts).
//
// La fusión viva de coalesceMark es el espejo O(1) de mergeAdjacentTrackMarks (su
// especificación PURA, cubierta por test/trackchanges.test.ts); teclear→<ins> y
// borrar→<del> se apoyan en insertion/deletionHtml (también probados). ponytail:
// la coreografía de Range/Selection (mover el caret, StaticRange→Range) es
// irreduciblemente DOM y la suite de bun no monta un DOM (happy-dom solo es
// dev-dep de packages/ui, no resoluble desde aquí); un e2e de navegador queda como
// follow-up — cablea un DOM de test si aparece regresión.
//
// ponytail: MVP de teclear + borrar + pegar TEXTO. Siguen como follow-up (sin
// romper nada, solo sin rastrear): Intro/partir párrafos, formato mientras se
// rastrea, IME/composición, pegar con formato/imágenes y deshacer nativo (estas
// mutaciones por Range no entran en la pila de undo del navegador).

let trackAuthor: TrackAuthor | null = null;

/** Fija el autor de las marcas (EditorApp lo toma de la sesión). Sin sesión: reserva. */
export function setTrackAuthor(author: TrackAuthor | null): void {
  trackAuthor = author;
}

function currentAuthor(): TrackAuthor {
  return trackAuthor ?? { name: 'Autor', id: 'anon' };
}

/** Coloca el caret colapsado en (node, offset) y memoriza el Range. */
function setCaret(node: Node, offset: number): void {
  const range = document.createRange();
  range.setStart(node, offset);
  range.collapse(true);
  applyRange(range);
}

/** Aplica un Range como selección viva y lo guarda para restaurarlo. */
function applyRange(range: Range): void {
  const sel = document.getSelection();
  if (sel) {
    sel.removeAllRanges();
    sel.addRange(range);
  }
  savedRange = range.cloneRange();
}

/** Primer elemento del HTML dado (para hornear una marca ya escapada); null si vacío. */
function elementFromHtml(html: string): HTMLElement | null {
  if (html === '') return null;
  const tpl = document.createElement('template');
  tpl.innerHTML = html;
  const el = tpl.content.firstElementChild;
  return el instanceof HTMLElement ? el : null;
}

/** `<ins data-track>` del autor actual que contiene al nodo (su marca propia), o null. */
function ownInsAt(node: Node | null): HTMLElement | null {
  const ins = closestInRoot(node, 'ins[data-track]');
  return ins && ins.dataset.authorId === currentAuthor().id ? ins : null;
}

/** Marca de rastreo hermana contigua del mismo tipo (tagName) y autor, o null. */
function adjacentTrackMark(node: ChildNode | null, tag: string, id: string): HTMLElement | null {
  return node instanceof HTMLElement &&
    node.tagName === tag &&
    node.hasAttribute('data-track') &&
    (node.dataset.authorId ?? '') === id
    ? node
    : null;
}

/**
 * Funde `el` con sus marcas hermanas contiguas del mismo tipo y autor moviendo
 * sus hijos (sin reserializar el bloque; caret preservado porque referencia el
 * nodo de texto, que sobrevive al traslado). Devuelve la marca superviviente.
 */
function coalesceMark(el: HTMLElement): HTMLElement {
  const id = el.dataset.authorId ?? '';
  const prev = adjacentTrackMark(el.previousSibling, el.tagName, id);
  if (prev) {
    while (el.firstChild) prev.appendChild(el.firstChild);
    el.remove();
    el = prev;
  }
  const next = adjacentTrackMark(el.nextSibling, el.tagName, id);
  if (next) {
    while (next.firstChild) el.appendChild(next.firstChild);
    next.remove();
  }
  return el;
}

/**
 * Inserta `text` como cambio rastreado en el caret (o reemplaza la selección
 * borrándola antes). Si el caret ya está en una inserción propia, crece esa marca
 * (texto plano, sin anidar); si no, crea una `<ins>` nueva y la funde con las
 * contiguas del mismo autor. Cubre teclear (un carácter) y pegar texto (varios).
 */
export function trackInsert(text: string): void {
  const rootEl = root;
  if (!rootEl || text === '') return;
  let range = selectionRange();
  if (!range) return;
  if (!range.collapsed) {
    // Reemplazo: borra (rastreado) lo seleccionado y reobtén el caret colapsado.
    deleteLiveRange(range, false);
    range = selectionRange();
    if (!range || !range.collapsed) return;
  }
  const ownIns = ownInsAt(range.startContainer);
  let textNode: Text;
  if (ownIns) {
    textNode = document.createTextNode(text);
    range.insertNode(textNode);
  } else {
    // Si el caret cae DENTRO de un borrado, la inserción va TRAS él (no anida).
    const del = closestInRoot(range.startContainer, 'del[data-track]');
    if (del) {
      range = document.createRange();
      range.setStartAfter(del);
      range.collapse(true);
    }
    const el = elementFromHtml(insertionHtml(text, currentAuthor(), Date.now()));
    const first = el?.firstChild;
    if (!el || !(first instanceof Text)) return;
    textNode = first;
    range.insertNode(el);
    coalesceMark(el);
  }
  setCaret(textNode, textNode.length);
  dispatchInput();
}

/** Convierte un StaticRange (de beforeinput.getTargetRanges) en borrado rastreado. */
export function trackDelete(staticRange: StaticRange, backward: boolean): void {
  const rootEl = root;
  if (!rootEl) return;
  const range = document.createRange();
  try {
    range.setStart(staticRange.startContainer, staticRange.startOffset);
    range.setEnd(staticRange.endContainer, staticRange.endOffset);
  } catch {
    return; // contenedores inválidos (borde del documento): no-op seguro
  }
  if (!rootEl.contains(range.commonAncestorContainer)) return;
  deleteLiveRange(range, backward);
}

/**
 * Marca como borrado el contenido de `range`: lo envuelve en `<del data-track>`
 * (NO lo quita) y deja el caret al lado (backward⇒izquierda, forward⇒derecha) para
 * que el cursor «salte» lo borrado y el siguiente borrado continúe. Dos atajos:
 * una inserción propia aún sin consolidar se RETIRA (no se marca); un rango ya
 * dentro de un borrado solo mueve el caret (no re-marca).
 */
function deleteLiveRange(range: Range, backward: boolean): void {
  if (!root || range.collapsed) return;

  // Borrar la propia inserción sin consolidar = retirarla (paridad Word).
  const ownIns = ownInsAt(range.startContainer);
  if (ownIns && ownIns.contains(range.endContainer)) {
    range.deleteContents();
    const caret = range.cloneRange();
    caret.collapse(true);
    if (ownIns.textContent === '') {
      caret.setStartBefore(ownIns);
      caret.collapse(true);
      ownIns.remove();
    }
    applyRange(caret);
    dispatchInput();
    return;
  }

  // Rango ya borrado: el caret lo salta (no se re-marca ni se anida).
  const del = closestInRoot(range.startContainer, 'del[data-track]');
  if (del && del.contains(range.endContainer)) {
    const caret = document.createRange();
    if (backward) caret.setStartBefore(del);
    else caret.setStartAfter(del);
    caret.collapse(true);
    applyRange(caret);
    return;
  }

  // ponytail: si el rango cruza párrafos, range.toString() aplana el salto y
  // deleteContents funde los bloques: el texto se conserva (envuelto en un <del>)
  // pero el cambio de estructura NO queda rastreado (es irrechazable). Follow-up:
  // rastrear la fusión de bloques (marcar el <p> absorbido) cuando haga falta.
  const el = elementFromHtml(deletionHtml(range.toString(), currentAuthor(), Date.now()));
  range.deleteContents();
  if (!el) {
    dispatchInput();
    return;
  }
  range.insertNode(el);
  const survivor = coalesceMark(el);
  const caret = document.createRange();
  if (backward) caret.setStartBefore(survivor);
  else caret.setStartAfter(survivor);
  caret.collapse(true);
  applyRange(caret);
  dispatchInput();
}

/**
 * Consolida (accept=true) o descarta (accept=false) TODAS las marcas de rastreo
 * del folio de una vez: reescribe su HTML con el helper PURO correspondiente y
 * emite `input` (autosave). Devuelve si algo cambió (para un aviso veraz). No-op
 * si no hay folio o no había marcas. ponytail: resuelve el documento entero;
 * resolver solo la marca bajo el caret queda como follow-up. Nota: el swap de
 * innerHTML no entra en la pila de undo nativa (por eso la UI pide confirmación).
 */
export function resolveTrackedChanges(accept: boolean): boolean {
  const rootEl = root;
  if (!rootEl) return false;
  const next = (accept ? acceptTrackedChanges : rejectTrackedChanges)(rootEl.innerHTML);
  if (next === rootEl.innerHTML) return false; // sin marcas que resolver
  rootEl.innerHTML = next;
  savedRange = null; // el Range guardado ya no describe el DOM reescrito
  dispatchInput();
  return true;
}

/**
 * Consolida (accept) o descarta UNA sola marca rastreada por su índice de documento
 * —el ordinal de listTrackedChanges, que coincide con el de
 * querySelectorAll('ins[data-track],del[data-track]')— reescribiendo el HTML del
 * folio con el helper PURO y emitiendo `input` (autosave). Devuelve si algo cambió
 * (el panel de revisión lo usa para encadenar cambios). Como el swap de innerHTML no
 * entra en el deshacer nativo, se descarta el caret guardado (igual que
 * resolveTrackedChanges). ponytail: reescribe el folio entero por marca; sobra para
 * una revisión real (pocos clics). Si un folio-libro lo hiciera notar, resolver el
 * <ins>/<del> in situ por índice.
 */
export function resolveTrackedChangeAt(index: number, accept: boolean): boolean {
  const rootEl = root;
  if (!rootEl) return false;
  const next = resolveTrackedChange(rootEl.innerHTML, index, accept);
  if (next === rootEl.innerHTML) return false; // índice fuera de rango: sin cambios
  rootEl.innerHTML = next;
  savedRange = null; // el Range guardado ya no describe el DOM reescrito
  dispatchInput();
  return true;
}
