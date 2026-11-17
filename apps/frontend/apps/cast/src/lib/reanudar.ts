/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Seguir donde se dejó un vídeo bajo demanda, sin DOM: qué posición merece guardarse,
 * cuánto dura el recuerdo y cuántos vídeos se recuerdan. La misma regla vive en el
 * escritorio (apps/fyne/apps/cast/reanudar.go), con el mismo formato.
 *
 * Es una comodidad de cada dispositivo, no un dato de la cuenta: va en `localStorage`
 * bajo una clave por persona (quien comparte el navegador no hereda lo que vio otra) y
 * no se sincroniza; eso sería un dato del servicio. Sin sesión no se recuerda nada.
 *
 * - Por debajo de 10 s no se guarda: es haber abierto el vídeo, no haberlo empezado.
 * - Desde el 95 % se da por visto y se olvida: volver a él es verlo otra vez, no ver
 *   los créditos.
 * - Lo que lleva 90 días sin tocarse caduca, y se recuerdan como mucho 200 vídeos (los
 *   más recientes): el almacén es pequeño y compartido con el resto de la app.
 */

/** Por debajo de esto no se guarda (segundos). */
export const MIN_S = 10;
/** Desde esta fracción del vídeo se da por visto. */
export const MAX_FRACCION = 0.95;
/** Cada cuánto se guarda mientras suena (ms). */
export const CADA_MS = 5_000;
/** Lo que dura el recuerdo sin volver al vídeo (ms). */
export const CADUCIDAD_MS = 90 * 24 * 60 * 60 * 1000;
/** Cuántos vídeos se recuerdan como mucho. */
export const MAX_ENTRADAS = 200;

const CLAVE = 'sytue.cast.reanudar';

/** Dónde se quedó un vídeo: segundo, duración (s) y cuándo se guardó (ms de época). */
export interface Punto {
  s: number;
  t: number;
  en: number;
}

/** Los puntos de una persona, por id de vídeo. */
export type Puntos = Readonly<Record<string, Punto>>;

/** Si una posición merece recordarse: pasados 10 s y sin llegar al 95 %. */
export function merece(s: number, total: number): boolean {
  return (
    Number.isFinite(s) &&
    Number.isFinite(total) &&
    total > 0 &&
    s >= MIN_S &&
    s / total < MAX_FRACCION
  );
}

/**
 * Los puntos con la posición de ese vídeo: si merece, se guarda; si no (se acaba de
 * abrir, o ya está casi visto), se olvida la que hubiera. Y de paso se poda.
 */
export function anotar(
  puntos: Puntos,
  id: string,
  s: number,
  total: number,
  ahora: number,
): Puntos {
  const nuevos: Record<string, Punto> = { ...puntos };
  if (merece(s, total)) nuevos[id] = { s, t: total, en: ahora };
  else delete nuevos[id];
  return podar(nuevos, ahora);
}

/** Sin los caducados ni los que no caben (se quedan los más recientes). */
export function podar(puntos: Puntos, ahora: number): Puntos {
  const vivos = Object.entries(puntos)
    .filter(([, p]) => valido(p) && ahora - p.en < CADUCIDAD_MS)
    .sort(([, a], [, b]) => b.en - a.en)
    .slice(0, MAX_ENTRADAS);
  return Object.fromEntries(vivos);
}

/** Desde dónde seguir ese vídeo (s), o null si no hay que seguir. */
export function desde(puntos: Puntos, id: string, ahora: number): number | null {
  const p = puntos[id];
  if (!p || !valido(p) || ahora - p.en >= CADUCIDAD_MS || !merece(p.s, p.t)) return null;
  return p.s;
}

/** Lo visto de ese vídeo (de 0 a 1) para la barra de la tarjeta, o null. */
export function visto(puntos: Puntos, id: string, ahora: number): number | null {
  const s = desde(puntos, id, ahora);
  const p = puntos[id];
  return s === null || !p ? null : s / p.t;
}

function valido(p: unknown): p is Punto {
  if (!p || typeof p !== 'object') return false;
  const { s, t, en } = p as Record<string, unknown>;
  return typeof s === 'number' && typeof t === 'number' && typeof en === 'number';
}

// ── El almacén ─────────────────────────────────────────────────────────────────────

/** Lo mínimo de `Storage` que se usa: los tests ponen uno en memoria. */
export type Almacen = Pick<Storage, 'getItem' | 'setItem'>;

/**
 * `localStorage`, si se deja. En una ventana privada o con los datos del sitio
 * bloqueados, acceder a él lanza: entonces no se recuerda nada y el vídeo empieza de
 * cero, que es lo que pasaba antes.
 */
function almacenPorDefecto(): Almacen | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage;
  } catch {
    return null;
  }
}

const clave = (persona: string): string => `${CLAVE}:${persona}`;

/** Los puntos de esa persona; vacíos sin persona, sin almacén o con basura dentro. */
export function leer(persona: string | null | undefined, almacen = almacenPorDefecto()): Puntos {
  if (!persona || !almacen) return {};
  try {
    const crudo = almacen.getItem(clave(persona));
    const datos: unknown = crudo ? JSON.parse(crudo) : null;
    if (!datos || typeof datos !== 'object' || Array.isArray(datos)) return {};
    return podar(datos as Record<string, Punto>, Date.now());
  } catch {
    return {};
  }
}

/** Guarda los puntos de esa persona. Si no se puede (cuota, bloqueo), no pasa nada. */
export function guardar(
  persona: string | null | undefined,
  puntos: Puntos,
  almacen = almacenPorDefecto(),
): void {
  if (!persona || !almacen) return;
  try {
    almacen.setItem(clave(persona), JSON.stringify(puntos));
  } catch {
    // Sin sitio o sin permiso: el recuerdo es una comodidad, no un dato.
  }
}

/** Anota la posición de un vídeo de esa persona y la guarda. */
export function recordar(
  persona: string | null | undefined,
  id: string,
  s: number,
  total: number,
  almacen = almacenPorDefecto(),
): void {
  if (!persona || !almacen) return;
  guardar(persona, anotar(leer(persona, almacen), id, s, total, Date.now()), almacen);
}
