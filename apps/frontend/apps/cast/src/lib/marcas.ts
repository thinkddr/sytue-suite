/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Marcas de tiempo: «1:23» en una descripción o un comentario se pulsa y el vídeo salta
 * allí; las líneas que empiezan por una son los capítulos; «?t=83» en el enlace es
 * «empieza aquí»; y las teclas del reproductor que llevan a un sitio (J y L, Inicio y
 * Fin, 0–9). La pareja nativa es `apps/fyne/apps/cast/marcas.go`, con los mismos casos.
 */

/** «1:23», «12:34» o «1:02:03»: sin dígitos ni «:» pegados delante o detrás. */
const MARCA = /(?<![\d:])(?:(\d{1,2}):)?(\d{1,2}):(\d{2})(?![\d:])/g;

function segundos(h: string | undefined, m: string, s: string): number | null {
  const hh = h === undefined ? 0 : Number(h);
  const mm = Number(m);
  const ss = Number(s);
  if (ss >= 60 || (h !== undefined && mm >= 60)) return null;
  return hh * 3600 + mm * 60 + ss;
}

/** Un trozo de texto: llano, o una marca con sus segundos. */
export interface Trozo {
  texto: string;
  segundos?: number;
}

/** El texto partido en lo llano y las marcas de tiempo, en orden. */
export function trozosDe(texto: string): Trozo[] {
  const out: Trozo[] = [];
  let desde = 0;
  for (const m of texto.matchAll(MARCA)) {
    const s = segundos(m[1], m[2]!, m[3]!);
    if (s === null) continue;
    if (m.index > desde) out.push({ texto: texto.slice(desde, m.index) });
    out.push({ texto: m[0], segundos: s });
    desde = m.index + m[0].length;
  }
  if (desde < texto.length) out.push({ texto: texto.slice(desde) });
  return out;
}

export interface Capitulo {
  segundos: number;
  titulo: string;
}

const LINEA_CAPITULO =
  /^\s*(?:[-–—•*]\s*)?[([]?(?:(\d{1,2}):)?(\d{1,2}):(\d{2})[)\]]?\s*(?:[-–—:|]\s*)?(\S.*?)\s*$/;

/**
 * Los capítulos de una descripción: las líneas que EMPIEZAN por una marca («0:00
 * Presentación», «- 1:30 – Demo»). Como en cualquier plataforma, solo cuentan si son
 * al menos dos, el primero en 0:00 y cada uno después del anterior; si no, es una
 * descripción con horas sueltas, no un índice.
 */
export function capitulosDe(descripcion: string): Capitulo[] {
  const out: Capitulo[] = [];
  for (const linea of descripcion.split('\n')) {
    const m = LINEA_CAPITULO.exec(linea);
    if (!m) continue;
    const s = segundos(m[1], m[2]!, m[3]!);
    if (s === null) continue;
    out.push({ segundos: s, titulo: m[4]! });
  }
  if (out.length < 2 || out[0]!.segundos !== 0) return [];
  for (let i = 1; i < out.length; i++) if (out[i]!.segundos <= out[i - 1]!.segundos) return [];
  return out;
}

/** El enlace con «?t=» en segundos enteros (o sin él, en el segundo 0). */
export function enlaceEn(enlace: string, segundo: number): string {
  const s = Math.floor(segundo);
  // A mano y no con `URL`: el enlace puede ser relativo (sin origen, en una prueba o
  // detrás de un proxy), y `new URL` sin base lo rechaza.
  const i = enlace.indexOf('?');
  const ruta = i < 0 ? enlace : enlace.slice(0, i);
  const q = new URLSearchParams(i < 0 ? '' : enlace.slice(i + 1));
  if (s > 0) q.set('t', String(s));
  else q.delete('t');
  const consulta = q.toString();
  return consulta ? `${ruta}?${consulta}` : ruta;
}

/** El «?t=» de un enlace: «83», «83s», «1m23s», «1h2m3s» o «1:23». null si no lo es. */
export function inicioDeUrl(valor: string | null): number | null {
  if (!valor) return null;
  const v = valor.trim();
  if (/^\d+s?$/.test(v)) return Number.parseInt(v, 10) || null;
  const hms = /^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/.exec(v);
  if (hms && (hms[1] || hms[2] || hms[3])) {
    const s = Number(hms[1] ?? 0) * 3600 + Number(hms[2] ?? 0) * 60 + Number(hms[3] ?? 0);
    return s || null;
  }
  const t = trozosDe(v);
  return t.length === 1 && t[0]!.segundos ? t[0]!.segundos : null;
}

/** Los segundos que mueven J y L: el doble que las flechas, como en cualquier reproductor. */
export const PASO_LARGO_S = 10;

/**
 * Adónde lleva una tecla del reproductor, o null si no lleva a ningún sitio: J y L
 * atrás y adelante 10 s, Inicio y Fin al principio y al final, y 0–9 al 0 %–90 %.
 */
export function destinoDeTecla(tecla: string, pos: number, dur: number): number | null {
  if (!Number.isFinite(dur) || dur <= 0) return null;
  const acotar = (s: number): number => Math.min(Math.max(s, 0), dur);
  switch (tecla) {
    case 'j':
    case 'J':
      return acotar(pos - PASO_LARGO_S);
    case 'l':
    case 'L':
      return acotar(pos + PASO_LARGO_S);
    case 'Home':
      return 0;
    case 'End':
      return dur;
    default:
      return /^[0-9]$/.test(tecla) ? (dur * Number(tecla)) / 10 : null;
  }
}
