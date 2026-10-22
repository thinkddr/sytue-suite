/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Reescribir las referencias de una fórmula cuando se insertan o se borran
 * filas y columnas.
 *
 * POR QUÉ EXISTE, Y POR QUÉ ES LO MÁS GRAVE QUE HABÍA EN LA HOJA. Hasta ahora,
 * insertar una fila movía las celdas de sitio y dejaba las fórmulas apuntando a
 * donde ya no está su dato: `=SUMA(A1:A5)` seguía sumando cinco filas cuando ya
 * eran seis. No fallaba nada, no salía ningún aviso — simplemente el resultado
 * pasaba a estar mal. Un error visible se corrige; uno silencioso se firma, se
 * manda al cliente y se descubre tres meses después.
 *
 * LAS REGLAS SON LAS DE EXCEL, y no por imitar: son las que espera cualquiera
 * que haya usado una hoja de cálculo, y cambiarlas sería sorprender a la gente
 * con su propio trabajo.
 *
 *   · Lo que está ANTES del punto de inserción no se mueve.
 *   · Lo que está en el punto o después se desplaza.
 *   · Al borrar, una celda que estaba DENTRO de lo borrado ya no existe: la
 *     fórmula pasa a `#¡REF!`. Mentir con un cero sería peor.
 *   · Un RANGO al que se le borra un trozo se encoge; si se lo borran entero,
 *     `#¡REF!`.
 *   · Los `$` NO eximen. Marcan la referencia como absoluta al COPIAR, que es
 *     otra cosa: `$A$1` con una fila insertada encima pasa a `$A$2`, porque el
 *     dato se ha movido de verdad. Confundir las dos cosas es el error clásico.
 *
 * Se apoya en `extractRefs` (formula-edit.ts), que ya localiza cada referencia
 * con su posición exacta en el texto. Aquí solo se decide a dónde va cada una y
 * se recompone conservando lo que el usuario escribió: sus `$`, sus mayúsculas y
 * su prefijo de hoja.
 *
 * Self-test: packages/office/test/sheets-refshift.test.ts (bun test).
 */
import type { SheetAutoFilter, SheetData } from '../types';
import type { FilterView } from './filtro';
import { extractRefs } from './formula-edit';
import { escapeHtml, isFormula, parseCell, rawText, renderCellHtml } from './format';

/** Lo que le pasa a la rejilla: un tramo que se abre o que se cierra. */
export interface Desplazamiento {
  /** Sobre qué eje. */
  eje: 'fila' | 'columna';
  /** Índice 0-based donde empieza el tramo. */
  desde: number;
  /** Cuántas se insertan (>0) o se borran (<0). */
  cantidad: number;
  /** Tope de la rejilla en ese eje (filas o columnas). Lo que rebosa deja de existir. */
  tope: number;
}

/** Lo que devuelve `#¡REF!` en el texto de una fórmula. */
export const REF_ROTA = '#¡REF!';

/**
 * Nuevo índice de una posición, o null si deja de existir.
 *
 * `dentro` distingue los dos usos: para una celda suelta, caer en lo borrado
 * significa desaparecer; para el extremo de un rango, significa pegarse al
 * borde de lo que queda — que es como el rango se encoge en vez de romperse.
 */
function mover(i: number, d: Desplazamiento, dentro: 'muere' | 'sujeta'): number | null {
  if (d.cantidad > 0) {
    if (i < d.desde) return i;
    const nuevo = i + d.cantidad;
    // Fuera de la rejilla: la celda ya no está, y decirlo es más honesto que
    // dejar la referencia apuntando al último borde.
    return nuevo < d.tope ? nuevo : null;
  }
  const n = -d.cantidad;
  if (i < d.desde) return i;
  if (i < d.desde + n) return dentro === 'sujeta' ? d.desde : null;
  return i - n;
}

/** Un extremo de referencia descompuesto: `$A$12` → col A (fijada), fila 12. */
interface Extremo {
  colDolar: string;
  col: string;
  filaDolar: string;
  fila: string;
}

const EXTREMO = /^(\$?)([A-Za-z]*)(\$?)(\d*)$/;

function partirExtremo(txt: string): Extremo | null {
  const m = EXTREMO.exec(txt);
  if (!m || (!m[2] && !m[4])) return null;
  return { colDolar: m[1] ?? '', col: m[2] ?? '', filaDolar: m[3] ?? '', fila: m[4] ?? '' };
}

/** Letras de columna → índice 0-based ('A' → 0, 'AA' → 26). */
function colAIndice(letras: string): number {
  let n = 0;
  for (const c of letras.toUpperCase()) n = n * 26 + (c.charCodeAt(0) - 64);
  return n - 1;
}

/** Índice 0-based → letras ('0' → 'A'). Conserva las mayúsculas del original. */
function indiceACol(n: number, comoElOriginal: string): string {
  let s = '';
  let x = n + 1;
  while (x > 0) {
    const r = (x - 1) % 26;
    s = String.fromCharCode(65 + r) + s;
    x = Math.floor((x - 1) / 26);
  }
  // Si quien escribió la fórmula usó minúsculas, se las respetamos: reescribirle
  // el texto en mayúsculas es un cambio que él no ha pedido.
  return comoElOriginal && comoElOriginal === comoElOriginal.toLowerCase() ? s.toLowerCase() : s;
}

/**
 * Desplaza un extremo. Devuelve null si desaparece.
 *
 * Un extremo puede no tener columna (`5`, fila entera) o no tener fila (`A`,
 * columna entera): en ese caso el eje ausente no se toca, porque una referencia
 * a la columna A sigue siendo la columna A por muchas filas que se inserten.
 */
function moverExtremo(e: Extremo, d: Desplazamiento, dentro: 'muere' | 'sujeta'): Extremo | null {
  if (d.eje === 'fila') {
    if (!e.fila) return e;
    const i = mover(Number(e.fila) - 1, d, dentro);
    return i === null ? null : { ...e, fila: String(i + 1) };
  }
  if (!e.col) return e;
  const i = mover(colAIndice(e.col), d, dentro);
  return i === null ? null : { ...e, col: indiceACol(i, e.col) };
}

const pintar = (e: Extremo): string => `${e.colDolar}${e.col}${e.filaDolar}${e.fila}`;

/**
 * Reescribe una referencia suelta (`A1`, `$B$4:C9`, `A:A`) tras el
 * desplazamiento. Devuelve `#¡REF!` si deja de existir.
 */
export function moverReferencia(ref: string, d: Desplazamiento): string {
  const bang = ref.lastIndexOf('!');
  const hoja = bang >= 0 ? ref.slice(0, bang + 1) : '';
  const a1 = bang >= 0 ? ref.slice(bang + 1) : ref;
  const partes = a1.split(':');

  if (partes.length === 1) {
    const e = partirExtremo(partes[0] ?? '');
    if (!e) return ref;
    const m = moverExtremo(e, d, 'muere');
    return m ? hoja + pintar(m) : REF_ROTA;
  }
  if (partes.length !== 2) return ref;

  const [aTxt, bTxt] = partes;
  const a = partirExtremo(aTxt ?? '');
  const b = partirExtremo(bTxt ?? '');
  if (!a || !b) return ref;

  // Un rango solo muere si se lleva por delante sus DOS extremos, y para saberlo
  // hay que preguntar con 'muere'; para colocarlo, con 'sujeta'. Con solo una de
  // las dos, o el rango se rompe cuando debería encogerse, o sobrevive vacío.
  if (moverExtremo(a, d, 'muere') === null && moverExtremo(b, d, 'muere') === null) {
    return REF_ROTA;
  }
  const na = moverExtremo(a, d, 'sujeta');
  const nb = moverExtremo(b, d, 'sujeta');
  if (!na || !nb) return REF_ROTA;
  return `${hoja}${pintar(na)}:${pintar(nb)}`;
}

/**
 * Reescribe TODAS las referencias de una fórmula.
 *
 * Se sustituye de derecha a izquierda para que las posiciones que devolvió
 * `extractRefs` sigan siendo válidas mientras se reemplaza: hacerlo al revés
 * desplaza el texto bajo los pies de las sustituciones siguientes.
 */
export function reescribirFormula(texto: string, d: Desplazamiento): string {
  if (!texto.startsWith('=')) return texto;
  const refs = extractRefs(texto);
  if (refs.length === 0) return texto;
  let out = texto;
  for (const r of [...refs].sort((x, y) => y.start - x.start)) {
    const nuevo = moverReferencia(r.text, d);
    if (nuevo !== r.text) out = out.slice(0, r.start) + nuevo + out.slice(r.end);
  }
  return out;
}

/**
 * Aplica el desplazamiento al HTML de una celda, conservando su formato.
 *
 * Gemela de `shiftCellHtml` (format.ts), que hace lo MISMO para la otra clase de
 * desplazamiento: el de copiar y autorrellenar, donde `$` sí exime porque ahí la
 * referencia no se ha movido, se ha vuelto a escribir en otro sitio. Son dos
 * reglas distintas para dos cosas distintas, y por eso son dos funciones.
 */
export function reescribirCelda(
  html: string,
  d: Desplazamiento | ((formula: string) => string),
): string {
  if (html === '' || !isFormula(html)) return html;
  const p = parseCell(html);
  const raw = rawText(html);
  // Una función: otro reescrito de la fórmula (p. ej. renombrar un nombre definido).
  p.inner = escapeHtml(typeof d === 'function' ? d(raw) : reescribirFormula(raw, d));
  return renderCellHtml(p);
}

/** Índice 0-based de una columna que se desplaza, o null si deja de existir. */
function moverColumna(c: number, d: Desplazamiento): number | null {
  if (d.eje !== 'columna') return c;
  const m = /^([A-Z]+)/.exec(moverReferencia(`${indiceACol(c, 'A')}1`, d));
  return m ? colAIndice(m[1]!) : null;
}

/** Índice 0-based de una fila que se desplaza, o null si deja de existir. */
function moverFila(r: number, d: Desplazamiento): number | null {
  if (d.eje !== 'fila') return r;
  const m = /(\d+)$/.exec(moverReferencia(`A${r + 1}`, d));
  return m ? Number(m[1]) - 1 : null;
}

/** ¿El rango «A1:B2» es en realidad una sola celda? */
function esUnaCelda(r: string): boolean {
  const [a, b] = r.split(':');
  return a === b;
}

/**
 * Los criterios de un filtro (valores excluidos y condiciones, por columna) siguen a
 * sus columnas: una columna insertada los empuja, una borrada se lleva los suyos. Lo
 * usan el autofiltro y las vistas de filtro de cada persona (que no viven en el modelo).
 */
export function moverCriterios(
  c: Pick<SheetAutoFilter, 'excl' | 'cond'>,
  d: Desplazamiento,
): Pick<SheetAutoFilter, 'excl' | 'cond'> {
  const out: Pick<SheetAutoFilter, 'excl' | 'cond'> = {};
  if (c.excl) {
    const excl: Record<string, string[]> = {};
    for (const k in c.excl) {
      const col = moverColumna(Number(k), d);
      if (col !== null && c.excl[k]!.length > 0) excl[String(col)] = c.excl[k]!;
    }
    if (Object.keys(excl).length > 0) out.excl = excl;
  }
  if (c.cond) {
    const cond: NonNullable<SheetAutoFilter['cond']> = {};
    for (const k in c.cond) {
      const col = moverColumna(Number(k), d);
      if (col !== null) cond[String(col)] = c.cond[k]!;
    }
    if (Object.keys(cond).length > 0) out.cond = cond;
  }
  return out;
}

/** Una vista de filtro con sus criterios movidos (id y nombre, los suyos). */
export function moverVista(v: FilterView, d: Desplazamiento): FilterView {
  return { id: v.id, name: v.name, ...moverCriterios(v, d) };
}

/**
 * Los OBJETOS de la hoja siguen a sus celdas al insertar o borrar filas y columnas,
 * con las mismas reglas que las fórmulas: una tabla, un minigráfico o un gráfico
 * cuyo rango queda dentro de lo insertado crece; si se le borra un trozo, encoge; y
 * si se le borra entero, desaparece (su dato ya no existe). El autofiltro y sus
 * exclusiones, y la columna de cada segmentación, se mueven igual.
 *
 * Antes conservaban su rango y se quedaban mirando datos que ya estaban en otro
 * sitio: la tabla pintaba sus bandas sobre filas ajenas y el gráfico cambiaba de
 * serie sin que nadie lo pidiera. Devuelve la hoja con sus objetos movidos (las
 * celdas, tal cual); la clave de lo que se queda vacío se quita, para no ensuciar el
 * JSON.
 */
export function moverObjetos(s: SheetData, d: Desplazamiento): SheetData {
  const out: SheetData = { ...s };
  const vivo = (ref: string): string | null => {
    const r = moverReferencia(ref, d);
    return r === REF_ROTA ? null : r;
  };
  if (s.tables) {
    const tables = s.tables.flatMap((t) => {
      const range = vivo(t.range);
      return range ? [{ ...t, range }] : [];
    });
    if (tables.length > 0) out.tables = tables;
    else delete out.tables;
  }
  if (s.merges) {
    const merges = s.merges.flatMap((m) => {
      const r = vivo(m);
      // Una combinada que queda en una sola celda ya no combina nada.
      return r && /[A-Z]+\d+:[A-Z]+\d+/.test(r) && !esUnaCelda(r) ? [r] : [];
    });
    if (merges.length > 0) out.merges = merges;
    else delete out.merges;
  }
  const medidas = (m: Record<string, number> | undefined, eje: 'fila' | 'columna') => {
    if (!m || d.eje !== eje) return m;
    const out2: Record<string, number> = {};
    for (const k in m) {
      const i = eje === 'columna' ? moverColumna(Number(k), d) : moverFila(Number(k), d);
      if (i !== null) out2[String(i)] = m[k]!;
    }
    return Object.keys(out2).length > 0 ? out2 : undefined;
  };
  const cw = medidas(s.colWidths, 'columna');
  if (cw) out.colWidths = cw;
  else delete out.colWidths;
  const rh = medidas(s.rowHeights, 'fila');
  if (rh) out.rowHeights = rh;
  else delete out.rowHeights;
  if (s.sparklines) {
    const sparklines = s.sparklines.flatMap((sp) => {
      const cell = vivo(sp.cell);
      const range = vivo(sp.range);
      return cell && range ? [{ ...sp, cell, range }] : [];
    });
    if (sparklines.length > 0) out.sparklines = sparklines;
    else delete out.sparklines;
  }
  if (s.charts) {
    const charts = s.charts.flatMap((ch) => {
      const range = vivo(ch.range);
      return range ? [{ ...ch, range }] : [];
    });
    if (charts.length > 0) out.charts = charts;
    else delete out.charts;
  }
  if (s.slicers) {
    const slicers = s.slicers.flatMap((sl) => {
      if (sl.pivot) return [sl]; // su `col` es un campo del origen, no una columna
      const col = moverColumna(sl.col, d);
      return col === null ? [] : [{ ...sl, col }];
    });
    if (slicers.length > 0) out.slicers = slicers;
    else delete out.slicers;
  }
  if (s.autofilter) {
    const af = s.autofilter;
    const a1 = `${indiceACol(af.c1, 'A')}${af.row + 1}:${indiceACol(af.c2, 'A')}${af.row + 1}`;
    const m = /^([A-Z]+)(\d+):([A-Z]+)\d+$/.exec(vivo(a1) ?? '');
    if (!m) {
      delete out.autofilter;
    } else {
      const nuevo: SheetAutoFilter = {
        row: Number(m[2]) - 1,
        c1: colAIndice(m[1]!),
        c2: colAIndice(m[3]!),
      };
      Object.assign(nuevo, moverCriterios(af, d));
      out.autofilter = nuevo;
    }
  }
  return out;
}
