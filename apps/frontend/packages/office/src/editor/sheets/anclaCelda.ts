/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El ANCLA DE CELDA de un comentario de una hoja: `{ hoja, fila, col, text }`, con el id de
 * la hoja (idhoja.ts) y la identidad de su fila y de su columna (anclas.ts), más la
 * referencia que se ve («B4»). Como Excel y Sheets, un comentario de una hoja va pegado a una
 * celda y la rejilla lo marca con un triángulo en su esquina. La referencia no sirve de
 * identidad: insertar una fila encima convierte B4 en B5, y el comentario tiene que ir con
 * su celda. `text` es solo lo que se enseña citado (y lo que lee una versión anterior).
 *
 * Es la misma forma que guarda el servicio al importar las notas de un .xlsx o un .ods
 * (`apps/nucleo/office/model/sheet_notas.go`) y la que valida `validarAncla`
 * (`services/docs/internal/http/comments.go`).
 *
 * Módulo puro: sin DOM, para probarse solo.
 */
import type { CommentItem, SheetData, SheetModel } from '../types';
import { ejeColumnas, ejeFilas, idDe, indiceDe } from './anclas';
import { numToCol } from '@sytue/formula/ligero';

export interface AnclaCelda {
  hoja: string;
  fila: string;
  col: string;
  text: string;
}

/** Lo que admite el servicio en el id de una fila o una columna (`reIDAncla`). */
const ID_EJE = /^[A-Za-z0-9_-]{1,64}$/;
/** Tope del id de hoja y de la referencia citada (`validarAncla`). */
const MAX_TEXTO = 300;

/** Referencia A1 («B4») de una posición 0-based. */
const refDe = (c: number, r: number): string => `${numToCol(c)}${r + 1}`;

/** El ancla de la celda (c, r) de la hoja `s`, o null si la hoja aún no tiene id. */
export function anclaDeCelda(
  s: Pick<SheetData, 'id' | 'rowIds' | 'rowBase' | 'colIds' | 'colBase'>,
  c: number,
  r: number,
): AnclaCelda | null {
  if (!s.id || s.id.length > MAX_TEXTO) return null;
  return {
    hoja: s.id,
    fila: idDe(ejeFilas(s), r),
    col: idDe(ejeColumnas(s), c),
    text: refDe(c, r),
  };
}

/** El ancla de celda de un comentario, o null si no lo es (texto del folio, dibujo…) o está
 *  mal formada. */
export function leerAnclaCelda(raw: unknown): AnclaCelda | null {
  if (raw === null || typeof raw !== 'object') return null;
  const o = raw as Record<string, unknown>;
  const { hoja, fila, col, text } = o;
  if (typeof hoja !== 'string' || hoja === '' || hoja.length > MAX_TEXTO) return null;
  if (typeof fila !== 'string' || !ID_EJE.test(fila)) return null;
  if (typeof col !== 'string' || !ID_EJE.test(col)) return null;
  return { hoja, fila, col, text: typeof text === 'string' ? text.slice(0, MAX_TEXTO) : '' };
}

/** Dónde está HOY la celda de un ancla: la hoja (índice) y la posición, con su referencia.
 *  null si la hoja, la fila o la columna ya no existen: el comentario no se borra, solo deja
 *  de marcar una celda (la conversación sigue valiendo). */
export function celdaDeAncla(
  m: Pick<SheetModel, 'sheets'>,
  a: AnclaCelda,
): { hoja: number; c: number; r: number; ref: string } | null {
  const hoja = m.sheets.findIndex((s) => s.id === a.hoja);
  const s = m.sheets[hoja];
  if (!s) return null;
  const r = indiceDe(ejeFilas(s), a.fila);
  const c = indiceDe(ejeColumnas(s), a.col);
  if (r === null || c === null) return null;
  return { hoja, c, r, ref: refDe(c, r) };
}

/** Quién escribió un comentario: su nombre del directorio, o el principio del `sub` si no se
 *  resolvió. Lo enseñan el panel de comentarios y el `title` de la marca de la celda. */
export function autorDe(c: Pick<CommentItem, 'author_sub' | 'author_name'>): string {
  return c.author_name || c.author_sub.slice(0, 8);
}

/** Lo que dice la marca de una celda: cuántos hilos abiertos tiene y el primero (el más
 *  antiguo), que es lo que enseña su `title`. */
export interface MarcaComentario {
  hilos: number;
  primero: CommentItem;
}

/**
 * Las celdas de la hoja `indice` que llevan marca: las de los hilos ABIERTOS (resolver uno
 * quita su marca) anclados a ellas. Solo cuentan las raíces: una respuesta no marca nada.
 */
export function marcasDeComentarios(
  m: Pick<SheetModel, 'sheets'>,
  indice: number,
  comentarios: readonly CommentItem[],
): Map<string, MarcaComentario> {
  const out = new Map<string, MarcaComentario>();
  const id = m.sheets[indice]?.id;
  if (!id) return out;
  for (const k of comentarios) {
    if (k.resolved || k.parent_id) continue;
    const a = leerAnclaCelda(k.anchor);
    if (!a || a.hoja !== id) continue;
    const celda = celdaDeAncla(m, a);
    if (!celda) continue;
    const ya = out.get(celda.ref);
    if (!ya) out.set(celda.ref, { hilos: 1, primero: k });
    else {
      ya.hilos += 1;
      if (k.created_at < ya.primero.created_at) ya.primero = k;
    }
  }
  return out;
}
