/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cursores remotos en vivo (BLUEPRINT.md §4.4): lógica PURA (sin DOM) del estado
 * EFÍMERO de la posición de cursor/selección de cada participante. Serializa/
 * valida la posición por tipo de documento, asigna un color estable por peer y
 * mapea la posición de hoja/diapositiva a coordenadas. El transporte (WS) vive
 * en ./collab y la captura/render en cada lienzo. NO toca el modelo LWW.
 */
import { anclarRango, ejeColumnas, ejeFilas, resolverRango } from '../sheets/anclas';
import type { CursorDot, CursorPos, PeerCursor, SheetData, SlideBox } from '../types';

/** Nº de tonos de la paleta de avatares de marca (--sytue-avatar-1..N). */
const AVATAR_COUNT = 6;

/**
 * Color estable de un peer a partir de su identificador (sub): índice
 * determinista por hash en la paleta de avatares de marca (misma asignación que
 * los avatares de la cabecera, para que caret y avatar compartan color). Se
 * devuelve como var CSS lista para `color`/`background`.
 */
export const peerColor = (id: string): string => {
  let h = 0;
  for (let i = 0; i < id.length; i += 1) h = (h * 31 + id.charCodeAt(i)) >>> 0;
  return `var(--sytue-avatar-${(h % AVATAR_COUNT) + 1})`;
};

/** Nombre visible de un peer (reserva si viene vacío); se pinta como texto. */
export const peerName = (name: string): string => name.trim() || 'Colaborador';

/** Clave de posición para reiniciar la animación de la etiqueta al MOVERSE el
 *  peer (misma clave = misma posición = etiqueta que sigue desvaneciéndose). */
export const cursorPosKey = (pos: CursorPos): string =>
  pos.kind === 'sheet'
    ? `s${pos.sheet}:${pos.c1}:${pos.r1}:${pos.c2}:${pos.r2}`
    : pos.kind === 'doc'
      ? `d${pos.anchor}:${pos.focus}`
      : `p${pos.slide}:${pos.boxes.join(',')}`;

/** Entero finito o null (narrowing sin `as`, sin String(unknown)). */
const int = (v: unknown): number | null =>
  typeof v === 'number' && Number.isFinite(v) ? Math.trunc(v) : null;

/** Par de ids (ancla y extremo) o null. */
const par = (v: unknown): [string, string] | null =>
  Array.isArray(v) &&
  v.length === 2 &&
  v.every((x) => typeof x === 'string' && x.length > 0 && x.length <= 64)
    ? [v[0] as string, v[1] as string]
    : null;

/** Punto de la secuencia RGA recibido (JSON opaco) o null. */
const punto = (v: unknown): CursorDot | null => {
  if (typeof v !== 'object' || v === null) return null;
  const o = v as Record<string, unknown>;
  const c = int(o.c);
  return typeof o.a === 'string' && o.a.length <= 128 && c !== null && c >= 0
    ? { a: o.a, c }
    : null;
};

/**
 * Valida una posición de cursor recibida (JSON opaco `unknown`) a `CursorPos` o
 * null si no encaja. Estrecha por `kind`; descarta cualquier forma desconocida
 * (retrocompat: un emisor futuro con un kind nuevo se ignora sin romper).
 */
export const parseCursorPos = (raw: unknown): CursorPos | null => {
  if (typeof raw !== 'object' || raw === null) return null;
  const o = raw as Record<string, unknown>;
  switch (o.kind) {
    case 'sheet': {
      const sheet = int(o.sheet);
      const c1 = int(o.c1);
      const r1 = int(o.r1);
      const c2 = int(o.c2);
      const r2 = int(o.r2);
      if (sheet === null || c1 === null || r1 === null || c2 === null || r2 === null) return null;
      // Las anclas son opcionales (cliente anterior): sin ellas mandan los índices.
      const sheetId = typeof o.sheetId === 'string' && o.sheetId.length <= 128 ? o.sheetId : null;
      const rows = par(o.rows);
      const cols = par(o.cols);
      return {
        kind: 'sheet',
        sheet,
        c1,
        r1,
        c2,
        r2,
        ...(sheetId ? { sheetId } : {}),
        ...(rows ? { rows } : {}),
        ...(cols ? { cols } : {}),
      };
    }
    case 'doc': {
      const anchor = int(o.anchor);
      const focus = int(o.focus);
      if (anchor === null || focus === null) return null;
      // Los puntos son opcionales y van juntos: con uno solo no se puede anclar el rango.
      const anchorDot = punto(o.anchorDot);
      const focusDot = punto(o.focusDot);
      return anchorDot && focusDot
        ? { kind: 'doc', anchor, focus, anchorDot, focusDot }
        : { kind: 'doc', anchor, focus };
    }
    case 'slide': {
      const slide = int(o.slide);
      if (slide === null) return null;
      const boxes = Array.isArray(o.boxes)
        ? o.boxes.filter((b): b is string => typeof b === 'string')
        : [];
      return { kind: 'slide', slide, boxes };
    }
    default:
      return null;
  }
};

/**
 * Valida el objeto de presencia difundido por el servidor ({sub, name, cursor})
 * a un `PeerCursor` listo para render, o null si falta el sub o la posición no
 * es válida. El color se deriva del sub (estable).
 */
export const parsePeerCursor = (raw: unknown): PeerCursor | null => {
  if (typeof raw !== 'object' || raw === null) return null;
  const o = raw as Record<string, unknown>;
  const id = typeof o.sub === 'string' ? o.sub : '';
  if (id === '') return null;
  const pos = parseCursorPos(o.cursor);
  if (pos === null) return null;
  const name = typeof o.name === 'string' ? o.name : '';
  return { id, name, color: peerColor(id), pos };
};

/* ---------- Cursores anclados al texto ---------- */

/**
 * Dónde cae `offset` (un offset de texto de `antes`) en `despues`. El cursor de un
 * peer llega como offset y su dueño no lo reenvía cuando OTRA persona edita antes de
 * él, así que sin esto el caret se quedaba en el número y el texto se le iba de debajo.
 *
 * Se compara el texto de antes y el de después: lo que no cambió por delante (prefijo
 * común) ni por detrás (sufijo común) delimita la zona editada. Antes de ella el offset
 * no se mueve; después, se desplaza lo que creció o encogió el texto; dentro de una zona
 * borrada o reescrita se queda al principio de ella, que es donde el peer sigue estando.
 *
 * Es la red de seguridad: cuando el cursor trae los dots RGA de los caracteres a los que
 * está anclado (`anchorDot`/`focusDot`) y la secuencia local coincide con el folio, el
 * lienzo los resuelve con exactitud (state/coedicion.ts `offsetsDePuntos`) y esto no se usa.
 *
 * ponytail: es una heurística sobre el texto visible. Dos ediciones simultáneas lejanas
 * entre sí se resuelven como una sola zona (cubre las dos) y el caret entre ellas se
 * coloca al principio de la zona.
 */
export const anclarOffset = (offset: number, antes: string, despues: string): number => {
  if (antes === despues) return offset;
  const tope = Math.min(antes.length, despues.length);
  let pre = 0;
  while (pre < tope && antes.charCodeAt(pre) === despues.charCodeAt(pre)) pre += 1;
  let suf = 0;
  while (
    suf < tope - pre &&
    antes.charCodeAt(antes.length - 1 - suf) === despues.charCodeAt(despues.length - 1 - suf)
  ) {
    suf += 1;
  }
  // Un caret al final del texto sigue al final cuando se añade más detrás: con texto repetido
  // («aaaa» -> «aaaaa») el prefijo común se come todo el texto de antes y el caret se quedaba
  // atrás aunque la edición cayera a su espalda.
  if (suf === 0 && offset >= antes.length) return despues.length;
  if (offset <= pre) return offset;
  if (offset >= antes.length - suf) return offset + (despues.length - antes.length);
  return pre;
};

/** `anclarOffset` aplicado a ancla y foco de una posición de documento. */
export const anclarPos = (
  pos: Extract<CursorPos, { kind: 'doc' }>,
  antes: string,
  despues: string,
): Extract<CursorPos, { kind: 'doc' }> => ({
  ...pos,
  anchor: anclarOffset(pos.anchor, antes, despues),
  focus: anclarOffset(pos.focus, antes, despues),
});

/**
 * Qué ha cambiado en la presencia entre dos listas de participantes (por sub):
 * 'entra' si aparece alguien, 'sale' si alguien desaparece, null si es lo mismo.
 * Si hay a la vez una entrada y una salida, gana la entrada.
 */
export const cambioDePresencia = (
  antes: readonly string[],
  despues: readonly string[],
): 'entra' | 'sale' | null => {
  if (despues.some((s) => !antes.includes(s))) return 'entra';
  return antes.some((s) => !despues.includes(s)) ? 'sale' : null;
};

/* ---------- Mapeo posición → píxeles ---------- */

/** Geometría de la rejilla de hoja necesaria para situar un rango en píxeles. */
export interface SheetGeom {
  /** Borde izquierdo x de cada columna; longitud COLS+1 (colX[COLS] = borde derecho). */
  colX: readonly number[];
  /** Índices de fila VISIBLES en orden (salta filas ocultas por filtro/esquema). */
  visRows: readonly number[];
  /** Origen y del contenido (bajo la cabecera + canalón de esquema). */
  oy: number;
  /** Alto de fila en px de maqueta. */
  cellH: number;
  /** Borde superior de cada fila visible (+ el final de la última) cuando hay altos propios
   *  (`SheetData.rowHeights`); null o ausente = todas miden `cellH`. */
  rowTop?: readonly number[] | null;
}

/** Posición de hoja con las identidades de hoja, fila y columna añadidas. */
export const anclarPosHoja = (
  sheets: readonly SheetData[],
  sheet: number,
  rc: { c1: number; r1: number; c2: number; r2: number },
): Extract<CursorPos, { kind: 'sheet' }> => {
  const s = sheets[sheet];
  return {
    kind: 'sheet',
    sheet,
    ...rc,
    ...(s?.id ? { sheetId: s.id } : {}),
    ...(s
      ? {
          rows: anclarRango(ejeFilas(s), rc.r1, rc.r2),
          cols: anclarRango(ejeColumnas(s), rc.c1, rc.c2),
        }
      : {}),
  };
};

/**
 * Dónde está hoy el cursor de hoja de un peer: la hoja por su id (si la trae y existe), y
 * filas y columnas por su identidad. Lo que no se encuentra (borrado, o emisor anterior sin
 * anclas) cae a los índices anunciados. null = la hoja ya no existe.
 */
export const resolverPosHoja = (
  sheets: readonly SheetData[],
  pos: Extract<CursorPos, { kind: 'sheet' }>,
): { sheet: number; pos: Extract<CursorPos, { kind: 'sheet' }> } | null => {
  const porId = pos.sheetId ? sheets.findIndex((s) => s.id === pos.sheetId) : -1;
  const sheet = porId >= 0 ? porId : pos.sheetId ? -1 : pos.sheet;
  const s = sheets[sheet];
  if (!s) return null;
  const [r1, r2] = resolverRango(ejeFilas(s), pos.rows, pos.r1, pos.r2);
  const [c1, c2] = resolverRango(ejeColumnas(s), pos.cols, pos.c1, pos.c2);
  return { sheet, pos: { ...pos, sheet, c1, r1, c2, r2 } };
};

/** Caja en píxeles de maqueta (coordenadas de la rejilla). */
export interface PixelBox {
  left: number;
  top: number;
  width: number;
  height: number;
}

/**
 * Caja en píxeles del rango de un peer sobre la rejilla, o null si queda fuera
 * (alguna fila del rango está oculta por filtro/esquema colapsado). Reusa la
 * misma geometría (colX + índice visual de fila) que la selección propia.
 */
export const sheetPeerBox = (
  pos: Extract<CursorPos, { kind: 'sheet' }>,
  geom: SheetGeom,
): PixelBox | null => {
  const maxC = geom.colX.length - 2; // colX tiene COLS+1 entradas
  if (maxC < 0) return null;
  const c1 = Math.max(0, Math.min(pos.c1, pos.c2, maxC));
  const c2 = Math.max(0, Math.min(Math.max(pos.c1, pos.c2), maxC));
  const r1 = Math.min(pos.r1, pos.r2);
  const r2 = Math.max(pos.r1, pos.r2);
  const vi1 = geom.visRows.indexOf(r1);
  const vi2 = geom.visRows.indexOf(r2);
  if (vi1 < 0 || vi2 < 0) return null;
  const left = geom.colX[c1] ?? 0;
  const right = geom.colX[c2 + 1] ?? left;
  const top = geom.rowTop ? (geom.rowTop[vi1] ?? 0) : geom.oy + vi1 * geom.cellH;
  const bottom = geom.rowTop ? (geom.rowTop[vi2 + 1] ?? 0) : geom.oy + (vi2 + 1) * geom.cellH;
  return { left, top, width: right - left, height: bottom - top };
};

/** Caja seleccionada de un peer sobre el escenario (x/y/w/h en % 0..100) + su id. */
export interface SlidePeerBox {
  id: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

/**
 * Cajas (en % del escenario) que un peer tiene seleccionadas en la diapositiva
 * actual: cruza los ids anunciados con las cajas presentes (ignora los que ya no
 * existen, p. ej. tras borrarse). O(n) sobre las cajas; preserva su orden.
 */
export const slidePeerBoxes = (
  boxes: readonly SlideBox[],
  ids: readonly string[],
): SlidePeerBox[] => {
  if (ids.length === 0) return [];
  const want = new Set(ids);
  const out: SlidePeerBox[] = [];
  for (const b of boxes) if (want.has(b.id)) out.push({ id: b.id, x: b.x, y: b.y, w: b.w, h: b.h });
  return out;
};

/** Quién entra y quién sale entre dos listas de participantes (por sub). */
export const quienCambia = (
  antes: readonly string[],
  despues: readonly string[],
): { entran: string[]; salen: string[] } => ({
  entran: despues.filter((s) => !antes.includes(s)),
  salen: antes.filter((s) => !despues.includes(s)),
});
