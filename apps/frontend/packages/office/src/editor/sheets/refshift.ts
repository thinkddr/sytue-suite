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
import type { SheetAutoFilter, SheetData, SheetGroup } from '../types';
import type { FilterView } from './filtro';
import { extractRefs } from './formula-edit';
import {
  escapeHtml,
  isFormula,
  isPermissionGranted,
  parseCell,
  parseRangeRef,
  rawText,
  renderCellHtml,
  shiftRefs,
} from './format';
import { moveSpans } from './ocultas';

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
 * `dentro` distingue los usos: para una celda suelta, caer en lo borrado
 * significa desaparecer; para el extremo de un rango, significa pegarse al
 * borde de lo que queda — que es como el rango se encoge en vez de romperse.
 * El INICIO se pega a la primera que sigue al corte y el FIN, a la última que
 * lo precede: `A1:A5` sin las filas 3-5 es `A1:A2`, no `A1:A3`, que ya sería
 * la antigua fila 6, un dato ajeno.
 */
type Dentro = 'muere' | 'sujeta' | 'sujetaFin';

function mover(i: number, d: Desplazamiento, dentro: Dentro): number | null {
  if (d.cantidad > 0) {
    if (i < d.desde) return i;
    const nuevo = i + d.cantidad;
    // Fuera de la rejilla: la celda ya no está, y decirlo es más honesto que
    // dejar la referencia apuntando al último borde.
    return nuevo < d.tope ? nuevo : null;
  }
  const n = -d.cantidad;
  if (i < d.desde) return i;
  if (i < d.desde + n) {
    if (dentro === 'muere') return null;
    return dentro === 'sujeta' ? d.desde : d.desde - 1;
  }
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
function moverExtremo(e: Extremo, d: Desplazamiento, dentro: Dentro): Extremo | null {
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
  // Cuál es el fin en el eje que se mueve: quien escribe `A5:A1` también tiene
  // derecho a que su rango se encoja por el lado bueno.
  const pos = (e: Extremo): number =>
    d.eje === 'fila' ? Number(e.fila || 0) : e.col ? colAIndice(e.col) : 0;
  const aEsFin = pos(a) > pos(b);
  const na = moverExtremo(a, d, aEsFin ? 'sujetaFin' : 'sujeta');
  const nb = moverExtremo(b, d, aEsFin ? 'sujeta' : 'sujetaFin');
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

/** Un rango que se ha quedado en una celda se escribe como la celda: «B5», no «B5:B5». */
function compacto(r: string): string {
  return esUnaCelda(r) ? r.split(':')[0]! : r;
}

/**
 * La fórmula de una regla de formato condicional o de una validación tras el
 * desplazamiento. Está escrita respecto a la celda de ARRIBA A LA IZQUIERDA de su
 * rango, así que sus referencias se mueven como las de cualquier fórmula… salvo
 * cuando lo borrado se lleva esa celda: entonces la de arriba a la izquierda pasa a
 * ser otra, la primera superviviente, y la fórmula tiene que ser la que le tocaba a
 * ella. Primero se «copia» la fórmula a esa celda (como el autorrelleno, donde los
 * `$` sí eximen) y luego se aplica el desplazamiento. Sin esto, borrar la primera
 * fila de una regla `=B2>5` sobre `B2:B10` la dejaría en `=#¡REF!>5`, apagada.
 */
function moverFormulaAnclada(formula: string, rango: string, d: Desplazamiento): string {
  const r = parseRangeRef(rango);
  if (!r || !formula.startsWith('=')) return reescribirFormula(formula, d);
  const n = -d.cantidad;
  const ancla = d.eje === 'fila' ? r.r1 : r.c1;
  if (n > 0 && ancla >= d.desde && ancla < d.desde + n) {
    const salto = d.desde + n - ancla;
    const copiada = shiftRefs(
      formula,
      d.eje === 'columna' ? salto : 0,
      d.eje === 'fila' ? salto : 0,
    );
    // Una copia hacia abajo o a la derecha no se sale nunca por arriba ni por la
    // izquierda; si la fórmula no parsea, `shiftRefs` la devuelve tal cual.
    if (copiada.startsWith('=')) return reescribirFormula(copiada, d);
  }
  return reescribirFormula(formula, d);
}

/**
 * Un grupo de esquema tras el desplazamiento, con las reglas de un rango: crece si
 * se inserta DENTRO de su detalle, encoge si se le borra un trozo y desaparece si se
 * le borra entero. Insertar justo en su primera fila lo baja entero, como a una
 * fórmula; y su fila de resumen (end+1) queda fuera, así que insertar ahí no lo
 * estira.
 */
function moverGrupo(g: SheetGroup, d: Desplazamiento): SheetGroup | null {
  const ref =
    d.eje === 'fila'
      ? `A${g.start + 1}:A${g.end + 1}`
      : `${indiceACol(g.start, 'A')}1:${indiceACol(g.end, 'A')}1`;
  const r = parseRangeRef(moverReferencia(ref, d));
  if (!r) return null;
  const [start, end] = d.eje === 'fila' ? [r.r1, r.r2] : [r.c1, r.c2];
  return { ...g, start, end };
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
 * con las mismas reglas que las fórmulas: una tabla, un minigráfico, un gráfico, una
 * regla de formato condicional, una validación o una combinada cuyo rango queda
 * dentro de lo insertado crece; si se le borra un trozo, encoge; y si se le borra
 * entero, desaparece (su dato ya no existe). El autofiltro y sus exclusiones, la
 * columna de cada segmentación, los grupos de esquema y los anchos y altos se mueven
 * igual.
 *
 * Con la hoja PROTEGIDA, lo que gobierna un permiso no concedido no se toca: el
 * formato condicional, las validaciones, los gráficos, los minigráficos y las
 * segmentaciones sin `objects`, y el autofiltro sin `autoFilter`. Es lo que veta el
 * servidor (services/docs/internal/domain/proteccion.go, `camposPorPermiso`):
 * moverlos haría que rechazara el guardado entero, y con él las celdas movidas. Que
 * la regla se quede donde estaba es el mal menor, y es lo que hace Excel.
 *
 * Los rangos editables por persona o grupo (`protect.ranges`) SÍ viajan con sus celdas:
 * si se quedaran atrás, el rango de alguien pasaría a cubrir el contenido bloqueado que
 * ha llegado a sus filas y dejaría fuera parte del suyo. Crecen, encogen o desaparecen
 * como cualquier rango. El servidor solo lo acepta de quien no es propietario si
 * reproduce el mismo desplazamiento con las celdas movidas igual
 * (services/docs/internal/domain/proteccion_desplazar.go).
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
  const objetos = isPermissionGranted(s.protect, 'objects');
  if (s.protect?.ranges) {
    const ranges = s.protect.ranges.flatMap((g) => {
      const ref = vivo(g.ref);
      return ref ? [{ ...g, ref: compacto(ref) }] : [];
    });
    const { ranges: _previos, ...resto } = s.protect;
    out.protect = ranges.length > 0 ? { ...resto, ranges } : resto;
  }
  if (s.condFormats && objetos) {
    const condFormats = s.condFormats.flatMap((c) => {
      const range = vivo(c.range);
      if (!range) return [];
      const regla = { ...c, range: compacto(range) };
      if (c.formula !== undefined) regla.formula = moverFormulaAnclada(c.formula, c.range, d);
      return [regla];
    });
    if (condFormats.length > 0) out.condFormats = condFormats;
    else delete out.condFormats;
  }
  if (s.validations && objetos) {
    const validations = s.validations.flatMap((v) => {
      const range = vivo(v.range);
      if (!range) return [];
      const regla = { ...v, range: compacto(range) };
      if (v.formula !== undefined) regla.formula = moverFormulaAnclada(v.formula, v.range, d);
      return [regla];
    });
    if (validations.length > 0) out.validations = validations;
    else delete out.validations;
  }
  const grupos = (
    gs: SheetGroup[] | undefined,
    eje: Desplazamiento['eje'],
    clave: 'rowGroups' | 'colGroups',
  ) => {
    if (!gs || d.eje !== eje) return;
    const movidos = gs.flatMap((g) => {
      const m = moverGrupo(g, d);
      return m ? [m] : [];
    });
    if (movidos.length > 0) out[clave] = movidos;
    else delete out[clave];
  };
  grupos(s.rowGroups, 'fila', 'rowGroups');
  grupos(s.colGroups, 'columna', 'colGroups');
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
  // Las filas y columnas ocultas a mano siguen a su fila o columna (sheets/ocultas.ts).
  const campoOcultas = d.eje === 'fila' ? 'hiddenRows' : 'hiddenCols';
  if (s[campoOcultas]) {
    const ocultas = moveSpans(s[campoOcultas], d.desde, d.cantidad, d.tope);
    if (ocultas.length > 0) out[campoOcultas] = ocultas;
    else delete out[campoOcultas];
  }
  if (s.sparklines && objetos) {
    const sparklines = s.sparklines.flatMap((sp) => {
      const cell = vivo(sp.cell);
      const range = vivo(sp.range);
      return cell && range ? [{ ...sp, cell, range }] : [];
    });
    if (sparklines.length > 0) out.sparklines = sparklines;
    else delete out.sparklines;
  }
  if (s.charts && objetos) {
    const charts = s.charts.flatMap((ch) => {
      const range = vivo(ch.range);
      return range ? [{ ...ch, range }] : [];
    });
    if (charts.length > 0) out.charts = charts;
    else delete out.charts;
  }
  if (s.slicers && objetos) {
    const slicers = s.slicers.flatMap((sl) => {
      if (sl.pivot) return [sl]; // su `col` es un campo del origen, no una columna
      const col = moverColumna(sl.col, d);
      return col === null ? [] : [{ ...sl, col }];
    });
    if (slicers.length > 0) out.slicers = slicers;
    else delete out.slicers;
  }
  if (s.autofilter && isPermissionGranted(s.protect, 'autoFilter')) {
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
