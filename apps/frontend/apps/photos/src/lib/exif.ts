/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo poco del EXIF que la galería cifrada necesita (ADR 0086): la fecha de captura
 * y la orientación, leídas EN EL NAVEGADOR. Con la foto en la bóveda el servidor no
 * puede leerlas, y no se le dan.
 *
 * Es el mismo criterio que el lector de Go (`apps/nucleo/photos/exif`): la fecha es
 * DateTimeOriginal, con OffsetTimeOriginal si viene y como UTC si no (no se sabe
 * dónde se tomó, y así el orden es estable); una fecha antes de 1900 es la de una
 * cámara sin reloj y no cuenta. Los bytes vienen de fuera: cada desplazamiento se
 * comprueba contra el tamaño real y nada se sigue sin comprobarlo.
 */

/** Cuánto del principio del fichero hace falta: el APP1 va delante de la imagen. */
export const BYTES_DE_CABECERA = 128 * 1024;

export interface ExifLeido {
  /** ISO 8601 en UTC, o `null` sin fecha válida. */
  tomada: string | null;
  /** 1..8, o 0 sin etiqueta. */
  orientacion: number;
}

const NADA: ExifLeido = { tomada: null, orientacion: 0 };
const MAX_ENTRADAS = 1024;

function tiffDeJpeg(b: Uint8Array): DataView | null {
  if (b.length < 4 || b[0] !== 0xff || b[1] !== 0xd8) return null;
  let i = 2;
  while (i + 4 <= b.length) {
    if (b[i] !== 0xff) return null;
    const marca = b[i + 1]!;
    // Comienzo de la imagen o fin: ya no hay EXIF.
    if (marca === 0xda || marca === 0xd9) return null;
    const largo = (b[i + 2]! << 8) | b[i + 3]!;
    if (largo < 2 || i + 2 + largo > b.length) return null;
    if (
      marca === 0xe1 &&
      largo >= 8 &&
      String.fromCharCode(...b.subarray(i + 4, i + 10)) === 'Exif\0\0'
    ) {
      return new DataView(b.buffer, b.byteOffset + i + 10, largo - 8);
    }
    i += 2 + largo;
  }
  return null;
}

interface Campo {
  tag: number;
  tipo: number;
  cuenta: number;
  /** Desplazamiento del valor dentro del TIFF (o del propio campo si cabe en 4 bytes). */
  valor: number;
}

function leerIFD(t: DataView, le: boolean, off: number): Campo[] {
  if (off < 8 || off + 2 > t.byteLength) return [];
  const n = t.getUint16(off, le);
  if (n > MAX_ENTRADAS || off + 2 + n * 12 > t.byteLength) return [];
  const campos: Campo[] = [];
  for (let k = 0; k < n; k++) {
    const p = off + 2 + k * 12;
    const tipo = t.getUint16(p + 2, le);
    const cuenta = t.getUint32(p + 4, le);
    const ancho = tipo === 3 ? 2 : tipo === 4 ? 4 : 1;
    const bytes = ancho * cuenta;
    campos.push({
      tag: t.getUint16(p, le),
      tipo,
      cuenta,
      valor: bytes <= 4 ? p + 8 : t.getUint32(p + 8, le),
    });
  }
  return campos;
}

function entero(t: DataView, le: boolean, c: Campo | undefined): number | null {
  if (!c || c.cuenta < 1) return null;
  if (c.tipo === 3 && c.valor + 2 <= t.byteLength) return t.getUint16(c.valor, le);
  if (c.tipo === 4 && c.valor + 4 <= t.byteLength) return t.getUint32(c.valor, le);
  return null;
}

function texto(t: DataView, c: Campo | undefined): string {
  if (!c || c.tipo !== 2 || c.cuenta > 256 || c.valor + c.cuenta > t.byteLength) return '';
  let s = '';
  for (let k = 0; k < c.cuenta; k++) {
    const ch = t.getUint8(c.valor + k);
    if (ch === 0) break;
    s += String.fromCharCode(ch);
  }
  return s.trim();
}

/** «2026:07:14 18:32:05» (+ «+02:00») a ISO en UTC; `null` si no vale. */
export function fechaExif(crudo: string, desfase = ''): string | null {
  const m = /^(\d{4}):(\d{2}):(\d{2}) (\d{2}):(\d{2}):(\d{2})$/.exec(crudo.trim());
  if (!m) return null;
  const [a = 0, mes = 0, d = 0, h = 0, mi = 0, s = 0] = m.slice(1).map(Number);
  if (a < 1900 || mes < 1 || mes > 12 || d < 1 || d > 31 || h > 23 || mi > 59 || s > 60)
    return null;
  let ms = Date.UTC(a, mes - 1, d, h, mi, s);
  const z = /^([+-])(\d{2}):(\d{2})$/.exec(desfase.trim());
  if (z) ms -= (z[1] === '-' ? -1 : 1) * (Number(z[2]) * 60 + Number(z[3])) * 60_000;
  const fecha = new Date(ms);
  return Number.isNaN(fecha.getTime()) ? null : fecha.toISOString();
}

/** Lee fecha y orientación del principio de un JPEG; sin EXIF (o roto), nada. */
export function leerExif(cabecera: Uint8Array): ExifLeido {
  try {
    const t = tiffDeJpeg(cabecera);
    if (!t || t.byteLength < 8) return NADA;
    const orden = t.getUint16(0, false);
    if (orden !== 0x4949 && orden !== 0x4d4d) return NADA;
    const le = orden === 0x4949;
    if (t.getUint16(2, le) !== 42) return NADA;
    const ifd0 = leerIFD(t, le, t.getUint32(4, le));
    const porTag = (cs: Campo[], tag: number) => cs.find((c) => c.tag === tag);
    const orientacion = entero(t, le, porTag(ifd0, 0x0112)) ?? 0;
    const puntero = entero(t, le, porTag(ifd0, 0x8769));
    const exif = puntero ? leerIFD(t, le, puntero) : [];
    const tomada = fechaExif(texto(t, porTag(exif, 0x9003)), texto(t, porTag(exif, 0x9011)));
    return { tomada, orientacion: orientacion >= 1 && orientacion <= 8 ? orientacion : 0 };
  } catch {
    return NADA;
  }
}
