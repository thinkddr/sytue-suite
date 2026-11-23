/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DER mínimo (X.690) para firmar en el dispositivo: leer un `.p12`, un certificado
 * y un CMS, y escribir el CMS de la firma. Solo lo que hace falta y con longitudes
 * DEFINIDAS: DER no admite otras, y un `.p12` con longitudes indefinidas (BER) se
 * acepta al leer porque hay generadores que las escriben así.
 *
 * Sin dependencias, como en el servidor (`apps/nucleo/pdf/pdfsign.go`): es el camino
 * de una clave privada, y es el último sitio donde se quiere código de fuera.
 */

/** Un nodo leído: etiqueta, contenido y el tramo exacto que ocupa en el origen. */
export interface Nodo {
  /** El primer byte de la etiqueta (clase, construido y número < 31). */
  etiqueta: number;
  /** El contenido, sin cabecera. */
  valor: Uint8Array<ArrayBuffer>;
  /** El nodo entero tal y como venía, con su cabecera. */
  bruto: Uint8Array<ArrayBuffer>;
}

export class ErrorDER extends Error {
  constructor(motivo: string) {
    super(`der: ${motivo}`);
    this.name = 'ErrorDER';
  }
}

export const ETIQUETA = {
  entero: 0x02,
  bits: 0x03,
  octetos: 0x04,
  nulo: 0x05,
  oid: 0x06,
  utf8: 0x0c,
  imprimible: 0x13,
  ia5: 0x16,
  bmp: 0x1e,
  secuencia: 0x30,
  conjunto: 0x31,
} as const;

/** `[n]` de contexto, construido. */
export const contexto = (n: number): number => 0xa0 | n;

function leerCabecera(
  d: Uint8Array,
  pos: number,
): { etiqueta: number; largo: number; ini: number } {
  if (pos + 2 > d.length) throw new ErrorDER('cabecera cortada');
  const etiqueta = d[pos]!;
  if ((etiqueta & 0x1f) === 0x1f) throw new ErrorDER('etiqueta larga no soportada');
  let largo = d[pos + 1]!;
  let ini = pos + 2;
  if (largo === 0x80) return { etiqueta, largo: -1, ini };
  if (largo & 0x80) {
    const n = largo & 0x7f;
    if (n === 0 || n > 4 || ini + n > d.length) throw new ErrorDER('longitud inválida');
    largo = 0;
    for (let i = 0; i < n; i++) largo = largo * 256 + d[ini + i]!;
    ini += n;
  }
  return { etiqueta, largo, ini };
}

/** Lee un nodo en `pos`. Con longitud indefinida (BER) busca su fin recorriendo los hijos. */
export function leerNodo(d: Uint8Array<ArrayBuffer>, pos = 0): Nodo & { fin: number } {
  const { etiqueta, largo, ini } = leerCabecera(d, pos);
  if (largo >= 0) {
    const fin = ini + largo;
    if (fin > d.length) throw new ErrorDER('contenido cortado');
    return { etiqueta, valor: d.subarray(ini, fin), bruto: d.subarray(pos, fin), fin };
  }
  // BER indefinida: solo en nodos construidos, y acaba en 00 00.
  if (!(etiqueta & 0x20)) throw new ErrorDER('longitud indefinida en un primitivo');
  let p = ini;
  const partes: Uint8Array[] = [];
  for (;;) {
    if (p + 2 > d.length) throw new ErrorDER('fin de contenido ausente');
    if (d[p] === 0 && d[p + 1] === 0) break;
    const hijo = leerNodo(d, p);
    partes.push(hijo.bruto);
    p = hijo.fin;
  }
  // El contenido se rehace DEFINIDO: quien lo lea después no tiene que saber de BER.
  const valor = concatenar(...partes);
  return { etiqueta, valor, bruto: d.subarray(pos, p + 2), fin: p + 2 };
}

/** Los hijos de un nodo construido. */
export function hijos(n: Nodo): Nodo[] {
  const out: Nodo[] = [];
  let p = 0;
  while (p < n.valor.length) {
    const h = leerNodo(n.valor, p);
    out.push({ etiqueta: h.etiqueta, valor: h.valor, bruto: h.bruto });
    p = h.fin;
  }
  return out;
}

/** El hijo `i`, que tiene que existir (y, si se dice, con esa etiqueta). */
export function hijo(n: Nodo, i: number, etiqueta?: number): Nodo {
  const h = hijos(n)[i];
  if (!h) throw new ErrorDER(`falta el elemento ${i}`);
  if (etiqueta !== undefined && h.etiqueta !== etiqueta)
    throw new ErrorDER(`etiqueta ${h.etiqueta.toString(16)} en vez de ${etiqueta.toString(16)}`);
  return h;
}

/**
 * El contenido de una OCTET STRING, juntando los trozos si viene CONSTRUIDA (BER):
 * es como guardan sus bolsas algunos `.p12` de Windows.
 */
export function octetos(n: Nodo): Uint8Array<ArrayBuffer> {
  if (n.etiqueta === ETIQUETA.octetos) return n.valor;
  if (n.etiqueta === (ETIQUETA.octetos | 0x20)) return concatenar(...hijos(n).map(octetos));
  throw new ErrorDER('se esperaba una OCTET STRING');
}

/** Un OID en su forma de puntos. */
export function oid(n: Nodo): string {
  if (n.etiqueta !== ETIQUETA.oid) throw new ErrorDER('se esperaba un OID');
  const v = n.valor;
  if (v.length === 0) throw new ErrorDER('OID vacío');
  const partes: number[] = [];
  let acc = 0;
  for (let i = 0; i < v.length; i++) {
    acc = acc * 128 + (v[i]! & 0x7f);
    if (!(v[i]! & 0x80)) {
      if (partes.length === 0) {
        const primero = acc < 80 ? Math.floor(acc / 40) : 2;
        partes.push(primero, acc - primero * 40);
      } else partes.push(acc);
      acc = 0;
    }
  }
  return partes.join('.');
}

/** Un INTEGER pequeño (iteraciones, versiones). */
export function entero(n: Nodo): number {
  if (n.etiqueta !== ETIQUETA.entero || n.valor.length === 0 || n.valor.length > 6)
    throw new ErrorDER('se esperaba un entero pequeño');
  let v = n.valor[0]! & 0x80 ? -1 : 0;
  for (const b of n.valor) v = v * 256 + b;
  return v;
}

/* ---------------------------------------------------------------- Escribir */

export function concatenar(...partes: Uint8Array[]): Uint8Array<ArrayBuffer> {
  let total = 0;
  for (const p of partes) total += p.length;
  const out = new Uint8Array(total);
  let pos = 0;
  for (const p of partes) {
    out.set(p, pos);
    pos += p.length;
  }
  return out;
}

function longitud(n: number): Uint8Array {
  if (n < 0x80) return Uint8Array.of(n);
  const bytes: number[] = [];
  while (n > 0) {
    bytes.unshift(n & 0xff);
    n = Math.floor(n / 256);
  }
  return Uint8Array.of(0x80 | bytes.length, ...bytes);
}

/** Etiqueta, longitud y contenido. */
export function tlv(etiqueta: number, ...contenido: Uint8Array[]): Uint8Array<ArrayBuffer> {
  const valor = concatenar(...contenido);
  return concatenar(Uint8Array.of(etiqueta), longitud(valor.length), valor);
}

export const secuencia = (...c: Uint8Array[]) => tlv(ETIQUETA.secuencia, ...c);
export const cadenaOctetos = (b: Uint8Array) => tlv(ETIQUETA.octetos, b);
export const nulo = () => Uint8Array.of(0x05, 0x00);

/**
 * SET OF en DER: los elementos ORDENADOS por su codificación. No es pedantería:
 * un validador que vuelve a codificar el conjunto antes de comprobar la firma
 * (BouncyCastle, el del validador de la Comisión) lo reordena, y si no se firmó ya
 * ordenado su resumen sale distinto (`ordenarComoDER` en Go).
 */
export function conjunto(...elementos: Uint8Array[]): Uint8Array<ArrayBuffer> {
  const orden = [...elementos].sort(comparar);
  return tlv(ETIQUETA.conjunto, ...orden);
}

/** El orden de bytes de DER (lexicográfico, el más corto antes si es prefijo). */
export function comparar(a: Uint8Array, b: Uint8Array): number {
  const n = Math.min(a.length, b.length);
  for (let i = 0; i < n; i++) if (a[i] !== b[i]) return a[i]! - b[i]!;
  return a.length - b.length;
}

/** Un OID desde su forma de puntos. */
export function oidDER(texto: string): Uint8Array<ArrayBuffer> {
  const partes = texto.split('.').map(Number);
  const [a = 0, b = 0, ...resto] = partes;
  const bytes: number[] = [];
  for (const v of [a * 40 + b, ...resto]) {
    const trozo: number[] = [v & 0x7f];
    let x = Math.floor(v / 128);
    while (x > 0) {
      trozo.unshift((x & 0x7f) | 0x80);
      x = Math.floor(x / 128);
    }
    bytes.push(...trozo);
  }
  return tlv(ETIQUETA.oid, Uint8Array.from(bytes));
}

/** Un INTEGER pequeño no negativo. */
export function enteroDER(v: number): Uint8Array<ArrayBuffer> {
  const bytes: number[] = [];
  do {
    bytes.unshift(v & 0xff);
    v = Math.floor(v / 256);
  } while (v > 0);
  if (bytes[0]! & 0x80) bytes.unshift(0);
  return tlv(ETIQUETA.entero, Uint8Array.from(bytes));
}

/**
 * Un INTEGER desde sus bytes sin signo (r y s de ECDSA): sin ceros de sobra a la
 * izquierda y con uno delante si el primer bit quedaría a 1.
 */
export function enteroSinSigno(b: Uint8Array): Uint8Array<ArrayBuffer> {
  let i = 0;
  while (i < b.length - 1 && b[i] === 0) i++;
  const cuerpo = b.subarray(i);
  return tlv(ETIQUETA.entero, cuerpo[0]! & 0x80 ? concatenar(Uint8Array.of(0), cuerpo) : cuerpo);
}
