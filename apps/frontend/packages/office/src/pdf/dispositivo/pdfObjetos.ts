/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo justo de un PDF para FIRMARLO ANEXANDO en el dispositivo (§7.5.6): leer la
 * cadena de tablas de referencias —clásicas y en flujo (§7.5.8), con objetos dentro
 * de flujos de objetos (§7.5.7)—, resolver el catálogo, la primera página y el
 * formulario, y volver a escribir esos diccionarios tal cual con un campo más.
 *
 * NO es un motor de PDF: no dibuja, no descifra y no repara. Lo que no sepa leer con
 * seguridad lo rechaza (`ErrorPdf`) en vez de escribir un anexo que apunte a basura;
 * el motor completo es el del servidor y el nativo (`apps/nucleo/pdf`), que aquí no
 * pueden ver el documento.
 *
 * Los valores se guardan SIN reinterpretar —números y nombres con su texto, cadenas
 * con sus bytes— para que reescribir un diccionario no cambie lo que dice.
 */

/** Por qué no se puede firmar este PDF en el dispositivo. */
export type MotivoPdf =
  /** No es un PDF, o su estructura no se puede leer con seguridad. */
  | 'ilegible'
  /** Está protegido con contraseña: el anexo tendría que cifrarse con su clave. */
  | 'cifrado'
  /** Certificado con /DocMDP P=1: no admite NINGÚN cambio, tampoco otra firma. */
  | 'bloqueado'
  /** La firma visible no cabe donde se pidió: la página no existe o el recuadro se sale. */
  | 'pagina'
  /** Se pidió certificar (/DocMDP) un documento que ya lleva firmas: solo la primera puede. */
  | 'certificarFirmado';

export class ErrorPdf extends Error {
  constructor(
    readonly motivo: MotivoPdf,
    detalle = '',
  ) {
    super(`pdf: ${motivo}${detalle ? `: ${detalle}` : ''}`);
    this.name = 'ErrorPdf';
  }
}

export interface Nombre {
  t: 'nombre';
  /** Tal cual va en el fichero, sin la barra (con sus `#xx`). */
  v: string;
}
export interface Numero {
  t: 'num';
  /** El texto original: «0.50» se reescribe «0.50». */
  v: string;
}
export interface Cadena {
  t: 'cadena';
  v: Uint8Array;
}
export interface Ref {
  t: 'ref';
  n: number;
  g: number;
}
export interface Flujo {
  t: 'flujo';
  dict: Dict;
  /** Los bytes del flujo SIN decodificar. */
  datos: Uint8Array;
}
export type Dict = Map<string, Obj>;
export type Obj = null | boolean | Nombre | Numero | Cadena | Ref | Obj[] | Dict | Flujo;

export const nombre = (v: string): Nombre => ({ t: 'nombre', v });
export const num = (v: number): Numero => ({ t: 'num', v: String(v) });
export const ref = (n: number, g = 0): Ref => ({ t: 'ref', n, g });
export const cadena = (v: Uint8Array): Cadena => ({ t: 'cadena', v });

export const esDict = (o: Obj | undefined): o is Dict => o instanceof Map;
export const esRef = (o: Obj | undefined): o is Ref =>
  !!o && typeof o === 'object' && 't' in o && o.t === 'ref';
const esNumero = (o: Obj | undefined): o is Numero =>
  !!o && typeof o === 'object' && 't' in o && o.t === 'num';
export const esNombre = (o: Obj | undefined, v?: string): o is Nombre =>
  !!o && typeof o === 'object' && 't' in o && o.t === 'nombre' && (v === undefined || o.v === v);
export const esCadena = (o: Obj | undefined): o is Cadena =>
  !!o && typeof o === 'object' && 't' in o && o.t === 'cadena';

/** El valor entero de un número (sin decimales), o `undefined`. */
export function entero(o: Obj | undefined): number | undefined {
  if (!esNumero(o)) return undefined;
  const n = Number(o.v);
  return Number.isSafeInteger(n) ? n : undefined;
}

/** El valor de un número, con decimales, o `undefined`. */
export function real(o: Obj | undefined): number | undefined {
  if (!esNumero(o)) return undefined;
  const n = Number(o.v);
  return Number.isFinite(n) ? n : undefined;
}

/** Una página: su objeto y su /MediaBox (heredada si no la tiene), en puntos. */
export interface PaginaPdf {
  n: number;
  caja: [number, number, number, number];
}

/** A4 vertical, lo que dice la norma cuando no hay /MediaBox en ningún nivel. */
const CAJA_POR_DEFECTO: [number, number, number, number] = [0, 0, 595, 842];

/* ------------------------------------------------------------------ Léxico */

const BLANCO = new Set([0, 9, 10, 12, 13, 32]);
const DELIM = new Set(Array.from('()<>[]{}/%', (c) => c.charCodeAt(0)));

const latin1 = (d: Uint8Array, a: number, b: number) => {
  let s = '';
  for (let i = a; i < b; i++) s += String.fromCharCode(d[i]!);
  return s;
};

/** Un lector de objetos sobre los bytes del fichero, desde una posición. */
export class Lector {
  constructor(
    readonly d: Uint8Array,
    public p = 0,
  ) {}

  saltarBlancos(): void {
    const d = this.d;
    while (this.p < d.length) {
      const c = d[this.p]!;
      if (BLANCO.has(c)) this.p++;
      else if (c === 0x25) {
        while (this.p < d.length && d[this.p] !== 10 && d[this.p] !== 13) this.p++;
      } else break;
    }
  }

  /** Una palabra suelta (número, `obj`, `R`, `true`…). */
  palabra(): string {
    this.saltarBlancos();
    const ini = this.p;
    while (this.p < this.d.length && !BLANCO.has(this.d[this.p]!) && !DELIM.has(this.d[this.p]!))
      this.p++;
    return latin1(this.d, ini, this.p);
  }

  /** ¿Empieza aquí (tras blancos) `texto`? No avanza. */
  mira(texto: string): boolean {
    this.saltarBlancos();
    for (let i = 0; i < texto.length; i++)
      if (this.d[this.p + i] !== texto.charCodeAt(i)) return false;
    return true;
  }

  /** Un objeto directo. `N G R` se lee como referencia. */
  objeto(profundidad = 0): Obj {
    if (profundidad > 64) throw new ErrorPdf('ilegible', 'anidamiento');
    this.saltarBlancos();
    const d = this.d;
    const c = d[this.p];
    if (c === undefined) throw new ErrorPdf('ilegible', 'fin inesperado');
    if (c === 0x2f) {
      this.p++;
      const ini = this.p;
      while (this.p < d.length && !BLANCO.has(d[this.p]!) && !DELIM.has(d[this.p]!)) this.p++;
      return nombre(latin1(d, ini, this.p));
    }
    if (c === 0x3c && d[this.p + 1] === 0x3c) {
      this.p += 2;
      const dict: Dict = new Map();
      for (;;) {
        this.saltarBlancos();
        if (d[this.p] === 0x3e && d[this.p + 1] === 0x3e) {
          this.p += 2;
          break;
        }
        const k = this.objeto(profundidad + 1);
        if (!esNombre(k)) throw new ErrorPdf('ilegible', 'clave de diccionario');
        dict.set(k.v, this.objeto(profundidad + 1));
      }
      return dict;
    }
    if (c === 0x3c) return cadena(this.hex());
    if (c === 0x28) return cadena(this.literal());
    if (c === 0x5b) {
      this.p++;
      const arr: Obj[] = [];
      for (;;) {
        this.saltarBlancos();
        if (d[this.p] === 0x5d) {
          this.p++;
          return arr;
        }
        arr.push(this.objeto(profundidad + 1));
      }
    }
    const w = this.palabra();
    if (w === '') throw new ErrorPdf('ilegible', `byte ${c} inesperado`);
    if (w === 'true') return true;
    if (w === 'false') return false;
    if (w === 'null') return null;
    if (!/^[+-]?(\d+\.?\d*|\.\d+)$/.test(w)) throw new ErrorPdf('ilegible', `palabra ${w}`);
    // ¿«N G R»? Se mira sin consumir: si no lo es, se vuelve atrás.
    if (/^\d+$/.test(w)) {
      const antes = this.p;
      const g = this.palabra();
      if (/^\d+$/.test(g)) {
        const r = this.palabra();
        if (r === 'R') return ref(Number(w), Number(g));
      }
      this.p = antes;
    }
    return { t: 'num', v: w };
  }

  private hex(): Uint8Array {
    this.p++;
    const nibbles: number[] = [];
    while (this.p < this.d.length && this.d[this.p] !== 0x3e) {
      const c = this.d[this.p++]!;
      if (BLANCO.has(c)) continue;
      const v = parseInt(String.fromCharCode(c), 16);
      if (Number.isNaN(v)) throw new ErrorPdf('ilegible', 'cadena hexadecimal');
      nibbles.push(v);
    }
    this.p++;
    if (nibbles.length % 2) nibbles.push(0);
    const out = new Uint8Array(nibbles.length / 2);
    for (let i = 0; i < out.length; i++) out[i] = (nibbles[2 * i]! << 4) | nibbles[2 * i + 1]!;
    return out;
  }

  private literal(): Uint8Array {
    const d = this.d;
    this.p++;
    const out: number[] = [];
    let nivel = 1;
    while (this.p < d.length) {
      const c = d[this.p++]!;
      if (c === 0x5c) {
        const e = d[this.p++];
        if (e === undefined) break;
        const esc: Record<number, number> = { 110: 10, 114: 13, 116: 9, 98: 8, 102: 12 };
        const simple = esc[e];
        if (simple !== undefined) out.push(simple);
        else if (e >= 0x30 && e <= 0x37) {
          let v = e - 0x30;
          for (let i = 0; i < 2 && d[this.p]! >= 0x30 && d[this.p]! <= 0x37; i++)
            v = v * 8 + (d[this.p++]! - 0x30);
          out.push(v & 0xff);
        } else if (e === 13) {
          if (d[this.p] === 10) this.p++;
        } else if (e !== 10) out.push(e);
        continue;
      }
      if (c === 0x28) nivel++;
      if (c === 0x29 && --nivel === 0) return Uint8Array.from(out);
      out.push(c);
    }
    throw new ErrorPdf('ilegible', 'cadena sin cerrar');
  }

  /**
   * «N G obj … endobj» en la posición actual. Si es un flujo, sus bytes: por /Length
   * si es directo y cuadra con `endstream`, y si no, buscando `endstream`.
   */
  objetoIndirecto(): { n: number; g: number; valor: Obj } {
    const n = Number(this.palabra());
    const g = Number(this.palabra());
    if (!Number.isSafeInteger(n) || !Number.isSafeInteger(g) || this.palabra() !== 'obj')
      throw new ErrorPdf('ilegible', 'objeto indirecto');
    const valor = this.objeto();
    if (esDict(valor) && this.mira('stream')) {
      this.p += 'stream'.length;
      if (this.d[this.p] === 13) this.p++;
      if (this.d[this.p] === 10) this.p++;
      const ini = this.p;
      const largo = entero(valor.get('Length'));
      let fin = -1;
      if (largo !== undefined && largo >= 0 && ini + largo <= this.d.length) {
        const l = new Lector(this.d, ini + largo);
        if (l.mira('endstream')) fin = ini + largo;
      }
      if (fin < 0) {
        fin = buscar(this.d, 'endstream', ini);
        if (fin < 0) throw new ErrorPdf('ilegible', 'flujo sin cerrar');
        // El salto de línea antes de `endstream` no es del flujo.
        if (this.d[fin - 1] === 10) fin--;
        if (this.d[fin - 1] === 13) fin--;
      }
      this.p = fin;
      if (!this.mira('endstream')) throw new ErrorPdf('ilegible', 'flujo sin cerrar');
      this.p += 'endstream'.length;
      return { n, g, valor: { t: 'flujo', dict: valor, datos: this.d.subarray(ini, fin) } };
    }
    return { n, g, valor };
  }
}

/** La posición de `texto` en `d` desde `desde`, o -1. */
export function buscar(d: Uint8Array, texto: string, desde = 0, hasta = d.length): number {
  const c0 = texto.charCodeAt(0);
  for (let i = desde; i <= hasta - texto.length; i++) {
    if (d[i] !== c0) continue;
    let ok = true;
    for (let j = 1; j < texto.length; j++)
      if (d[i + j] !== texto.charCodeAt(j)) {
        ok = false;
        break;
      }
    if (ok) return i;
  }
  return -1;
}

/** La última posición de `texto` en `d` antes de `hasta`, o -1. */
export function buscarAtras(d: Uint8Array, texto: string, hasta = d.length): number {
  for (let i = hasta - texto.length; i >= 0; i--) {
    let ok = true;
    for (let j = 0; j < texto.length; j++)
      if (d[i + j] !== texto.charCodeAt(j)) {
        ok = false;
        break;
      }
    if (ok) return i;
  }
  return -1;
}

/* ------------------------------------------------------------- Escritura */

const HEX = '0123456789ABCDEF';
export function aHex(b: Uint8Array): string {
  let s = '';
  for (const x of b) s += HEX[x >> 4]! + HEX[x & 15]!;
  return s;
}

/** Serializa un objeto directo. Las cadenas, siempre en hexadecimal: no pierden nada. */
export function serializar(o: Obj): string {
  if (o === null) return 'null';
  if (o === true) return 'true';
  if (o === false) return 'false';
  if (Array.isArray(o)) return `[${o.map(serializar).join(' ')}]`;
  if (o instanceof Map) {
    let s = '<<';
    for (const [k, v] of o) s += `/${k} ${serializar(v)}`;
    return `${s}>>`;
  }
  switch (o.t) {
    case 'nombre':
      return `/${o.v}`;
    case 'num':
      return o.v;
    case 'cadena':
      return `<${aHex(o.v)}>`;
    case 'ref':
      return `${o.n} ${o.g} R`;
    case 'flujo':
      throw new ErrorPdf('ilegible', 'un flujo no se reescribe');
  }
}

/** Texto de PDF: UTF-16BE con su marca, la única forma de que sobreviva una tilde. */
export function textoPDF(s: string): Cadena {
  const out = [0xfe, 0xff];
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    out.push(c >> 8, c & 0xff);
  }
  return cadena(Uint8Array.from(out));
}

/** Lee un texto de PDF: UTF-16BE con marca o, si no, PDFDocEncoding (≈ Latin-1). */
export function leerTextoPDF(o: Obj | undefined): string {
  if (!esCadena(o)) return '';
  const v = o.v;
  if (v[0] === 0xfe && v[1] === 0xff) {
    let s = '';
    for (let i = 2; i + 1 < v.length; i += 2) s += String.fromCharCode((v[i]! << 8) | v[i + 1]!);
    return s;
  }
  return latin1(v, 0, v.length);
}

/* ----------------------------------------------------------- Decodificar */

async function inflar(datos: Uint8Array): Promise<Uint8Array> {
  const flujo = new Blob([datos.slice()]).stream().pipeThrough(new DecompressionStream('deflate'));
  return new Uint8Array(await new Response(flujo).arrayBuffer());
}

/** Deshace un predictor PNG (§7.4.4.4): filas de `1 + columnas` bytes. */
function predictorPNG(d: Uint8Array, columnas: number): Uint8Array {
  const fila = columnas + 1;
  if (columnas <= 0 || d.length % fila) throw new ErrorPdf('ilegible', 'predictor');
  const filas = d.length / fila;
  const out = new Uint8Array(filas * columnas);
  for (let f = 0; f < filas; f++) {
    const tipo = d[f * fila]!;
    for (let c = 0; c < columnas; c++) {
      const x = d[f * fila + 1 + c]!;
      const a = c > 0 ? out[f * columnas + c - 1]! : 0;
      const b = f > 0 ? out[(f - 1) * columnas + c]! : 0;
      const cc = f > 0 && c > 0 ? out[(f - 1) * columnas + c - 1]! : 0;
      let v: number;
      switch (tipo) {
        case 0:
          v = x;
          break;
        case 1:
          v = x + a;
          break;
        case 2:
          v = x + b;
          break;
        case 3:
          v = x + ((a + b) >> 1);
          break;
        case 4: {
          const p = a + b - cc;
          const pa = Math.abs(p - a),
            pb = Math.abs(p - b),
            pc = Math.abs(p - cc);
          v = x + (pa <= pb && pa <= pc ? a : pb <= pc ? b : cc);
          break;
        }
        default:
          throw new ErrorPdf('ilegible', 'predictor');
      }
      out[f * columnas + c] = v & 0xff;
    }
  }
  return out;
}

/** Decodifica un flujo con FlateDecode (y predictor PNG) o sin filtro. Nada más. */
export async function decodificar(f: Flujo): Promise<Uint8Array> {
  let filtro = f.dict.get('Filter');
  let parms = f.dict.get('DecodeParms');
  if (Array.isArray(filtro)) {
    if (filtro.length > 1) throw new ErrorPdf('ilegible', 'filtros encadenados');
    filtro = filtro[0];
    parms = Array.isArray(parms) ? parms[0] : parms;
  }
  if (filtro === undefined || filtro === null) return f.datos;
  if (!esNombre(filtro, 'FlateDecode')) throw new ErrorPdf('ilegible', 'filtro');
  let d: Uint8Array;
  try {
    d = await inflar(f.datos);
  } catch {
    throw new ErrorPdf('ilegible', 'deflate');
  }
  if (esDict(parms)) {
    const pred = entero(parms.get('Predictor')) ?? 1;
    if (pred >= 10) d = predictorPNG(d, entero(parms.get('Columns')) ?? 1);
    else if (pred !== 1) throw new ErrorPdf('ilegible', 'predictor');
  }
  return d;
}

/* ---------------------------------------------------------------- Xref */

type Entrada = { tipo: 1; off: number; g: number } | { tipo: 2; flujo: number; indice: number };

/** El documento tal y como hace falta para anexarle una firma. */
export class DocumentoPdf {
  readonly entradas = new Map<number, Entrada>();
  readonly trailer: Dict = new Map();
  /** Dónde empieza la última tabla (el `startxref` final): el /Prev del anexo. */
  ultimoXref = 0;
  /** ¿La última tabla es clásica? Si no, el anexo también va en flujo. */
  clasica = true;
  private readonly flujosDeObjetos = new Map<
    number,
    { datos: Uint8Array; offsets: Map<number, number> }
  >();

  private constructor(readonly bytes: Uint8Array) {}

  static async abrir(bytes: Uint8Array): Promise<DocumentoPdf> {
    if (buscar(bytes, '%PDF-', 0, Math.min(bytes.length, 1024)) < 0)
      throw new ErrorPdf('ilegible', 'no es un PDF');
    const doc = new DocumentoPdf(bytes);
    await doc.leerXrefs();
    if (doc.trailer.has('Encrypt')) throw new ErrorPdf('cifrado');
    if (!esRef(doc.trailer.get('Root'))) throw new ErrorPdf('ilegible', 'sin /Root');
    return doc;
  }

  private async leerXrefs(): Promise<void> {
    const d = this.bytes;
    const i = buscarAtras(d, 'startxref', d.length);
    if (i < 0 || d.length - i > 4096) throw new ErrorPdf('ilegible', 'sin startxref');
    const l = new Lector(d, i + 'startxref'.length);
    const off = Number(l.palabra());
    if (!Number.isSafeInteger(off) || off <= 0 || off >= d.length)
      throw new ErrorPdf('ilegible', 'startxref');
    this.ultimoXref = off;
    this.clasica = new Lector(d, off).mira('xref');
    const vistos = new Set<number>();
    const pendientes = [off];
    while (pendientes.length) {
      const p = pendientes.shift()!;
      if (vistos.has(p)) continue;
      if (vistos.size > 1000) throw new ErrorPdf('ilegible', 'demasiadas tablas');
      vistos.add(p);
      if (p <= 0 || p >= d.length) throw new ErrorPdf('ilegible', '/Prev');
      const trailer = new Lector(d, p).mira('xref')
        ? this.leerTablaClasica(p)
        : await this.leerTablaEnFlujo(p);
      // Lo más nuevo manda: una clave del trailer o una entrada ya vista no se pisa.
      for (const [k, v] of trailer)
        if (!this.trailer.has(k) && k !== 'Prev' && k !== 'XRefStm') this.trailer.set(k, v);
      // Un fichero híbrido (§7.5.8.4) lleva además su tabla en flujo.
      const stm = entero(trailer.get('XRefStm'));
      if (stm !== undefined) pendientes.unshift(stm);
      const prev = entero(trailer.get('Prev'));
      if (prev !== undefined) pendientes.push(prev);
    }
  }

  private poner(n: number, e: Entrada): void {
    if (!this.entradas.has(n)) this.entradas.set(n, e);
  }

  private leerTablaClasica(p: number): Dict {
    const l = new Lector(this.bytes, p);
    l.palabra(); // xref
    for (;;) {
      if (l.mira('trailer')) {
        l.p += 'trailer'.length;
        const t = l.objeto();
        if (!esDict(t)) throw new ErrorPdf('ilegible', 'trailer');
        return t;
      }
      const ini = Number(l.palabra());
      const cuantos = Number(l.palabra());
      if (!Number.isSafeInteger(ini) || !Number.isSafeInteger(cuantos) || cuantos < 0)
        throw new ErrorPdf('ilegible', 'subsección de xref');
      for (let k = 0; k < cuantos; k++) {
        const off = Number(l.palabra());
        const g = Number(l.palabra());
        const tipo = l.palabra();
        // Las libres («f») también se marcan, para que una tabla más vieja no las reviva.
        if (tipo === 'n' && off > 0) this.poner(ini + k, { tipo: 1, off, g });
        else if (tipo === 'f' || tipo === 'n') this.poner(ini + k, { tipo: 1, off: 0, g });
        else throw new ErrorPdf('ilegible', 'entrada de xref');
      }
    }
  }

  private async leerTablaEnFlujo(p: number): Promise<Dict> {
    const { valor } = new Lector(this.bytes, p).objetoIndirecto();
    if (!valor || typeof valor !== 'object' || !('t' in valor) || valor.t !== 'flujo')
      throw new ErrorPdf('ilegible', 'tabla en flujo');
    const dict = valor.dict;
    if (!esNombre(dict.get('Type'), 'XRef')) throw new ErrorPdf('ilegible', 'tabla en flujo');
    const W = dict.get('W');
    const w = Array.isArray(W) ? W.map(entero) : [];
    if (w.length !== 3 || w.some((x) => x === undefined || x < 0 || x > 8))
      throw new ErrorPdf('ilegible', '/W');
    const [w0, w1, w2] = w as [number, number, number];
    const tam = entero(dict.get('Size')) ?? 0;
    const I = dict.get('Index');
    const indice = Array.isArray(I) ? I.map(entero) : [0, tam];
    const datos = await decodificar(valor);
    const leer = (pos: number, n: number) => {
      let v = 0;
      for (let i = 0; i < n; i++) v = v * 256 + datos[pos + i]!;
      return v;
    };
    let pos = 0;
    for (let k = 0; k + 1 < indice.length; k += 2) {
      const ini = indice[k],
        cuantos = indice[k + 1];
      if (ini === undefined || cuantos === undefined) throw new ErrorPdf('ilegible', '/Index');
      for (let j = 0; j < cuantos; j++) {
        if (pos + w0 + w1 + w2 > datos.length) throw new ErrorPdf('ilegible', 'tabla corta');
        const tipo = w0 ? leer(pos, w0) : 1;
        const a = leer(pos + w0, w1);
        const b = leer(pos + w0 + w1, w2);
        pos += w0 + w1 + w2;
        if (tipo === 1) this.poner(ini + j, { tipo: 1, off: a, g: b });
        else if (tipo === 2) this.poner(ini + j, { tipo: 2, flujo: a, indice: b });
        else this.poner(ini + j, { tipo: 1, off: 0, g: b });
      }
    }
    return dict;
  }

  /** La generación con la que está vivo el objeto `n` (0 dentro de un flujo de objetos). */
  generacion(n: number): number | undefined {
    const e = this.entradas.get(n);
    if (!e) return undefined;
    return e.tipo === 1 ? e.g : 0;
  }

  /** El número más alto en uso: /Size corto lo escriben algunos generadores. */
  maxObjeto(): number {
    let mayor = (entero(this.trailer.get('Size')) ?? 1) - 1;
    for (const n of this.entradas.keys()) if (n > mayor) mayor = n;
    return mayor;
  }

  /** El objeto `n`, o `null` si no existe. */
  async objeto(n: number): Promise<Obj> {
    const e = this.entradas.get(n);
    if (!e) return null;
    if (e.tipo === 1) {
      if (e.off <= 0 || e.off >= this.bytes.length) return null;
      const o = new Lector(this.bytes, e.off).objetoIndirecto();
      if (o.n !== n) throw new ErrorPdf('ilegible', `el objeto ${n} no está donde dice la tabla`);
      return o.valor;
    }
    let fo = this.flujosDeObjetos.get(e.flujo);
    if (!fo) {
      const f = await this.objeto(e.flujo);
      if (!f || typeof f !== 'object' || !('t' in f) || f.t !== 'flujo')
        throw new ErrorPdf('ilegible', 'flujo de objetos');
      const datos = await decodificar(f);
      const N = entero(f.dict.get('N')) ?? 0;
      const primero = entero(f.dict.get('First')) ?? 0;
      const cab = new Lector(datos, 0);
      const offsets = new Map<number, number>();
      for (let i = 0; i < N; i++) {
        const num = Number(cab.palabra());
        const off = Number(cab.palabra());
        if (!Number.isSafeInteger(num) || !Number.isSafeInteger(off))
          throw new ErrorPdf('ilegible', 'cabecera del flujo de objetos');
        offsets.set(num, primero + off);
      }
      fo = { datos, offsets };
      this.flujosDeObjetos.set(e.flujo, fo);
    }
    const off = fo.offsets.get(n);
    if (off === undefined) return null;
    return new Lector(fo.datos, off).objeto();
  }

  /** Sigue las referencias hasta un valor directo. */
  async resolver(o: Obj | undefined): Promise<Obj> {
    let x: Obj = o ?? null;
    for (let i = 0; esRef(x) && i < 32; i++) x = await this.objeto(x.n);
    return esRef(x) ? null : x;
  }

  /** El número de objeto de la primera página (el árbol se recorre en orden). */
  async primeraPagina(): Promise<number> {
    const raiz = await this.resolver(this.trailer.get('Root'));
    if (!esDict(raiz)) throw new ErrorPdf('ilegible', 'catálogo');
    let nodo = raiz.get('Pages');
    for (let i = 0; i < 64; i++) {
      if (!esRef(nodo)) throw new ErrorPdf('ilegible', 'árbol de páginas');
      const d = await this.resolver(nodo);
      if (!esDict(d)) throw new ErrorPdf('ilegible', 'árbol de páginas');
      if (esNombre(d.get('Type'), 'Page') || !d.has('Kids')) return nodo.n;
      const kids = await this.resolver(d.get('Kids'));
      if (!Array.isArray(kids) || kids.length === 0) throw new ErrorPdf('ilegible', 'sin páginas');
      nodo = kids[0];
    }
    throw new ErrorPdf('ilegible', 'árbol de páginas');
  }

  /**
   * Las páginas en orden, con su /MediaBox (la heredada de /Pages si no tienen),
   * para colocar una firma VISIBLE. Hasta `maximo`: un árbol mayor o con ciclos se
   * rechaza en vez de recorrerlo sin fin.
   */
  async paginas(maximo = 10000): Promise<PaginaPdf[]> {
    const raiz = await this.resolver(this.trailer.get('Root'));
    if (!esDict(raiz)) throw new ErrorPdf('ilegible', 'catálogo');
    const out: PaginaPdf[] = [];
    const vistos = new Set<number>();
    const caja = async (d: Dict, heredada: PaginaPdf['caja']): Promise<PaginaPdf['caja']> => {
      const mb = await this.resolver(d.get('MediaBox'));
      if (!Array.isArray(mb) || mb.length !== 4) return heredada;
      const v = await Promise.all(mb.map(async (x) => real(await this.resolver(x))));
      if (v.some((x) => x === undefined)) return heredada;
      const [a, b, c, e] = v as number[];
      return [Math.min(a!, c!), Math.min(b!, e!), Math.max(a!, c!), Math.max(b!, e!)];
    };
    const recorrer = async (nodo: Obj | undefined, heredada: PaginaPdf['caja'], nivel: number) => {
      if (!esRef(nodo) || nivel > 64 || vistos.has(nodo.n))
        throw new ErrorPdf('ilegible', 'árbol de páginas');
      vistos.add(nodo.n);
      const d = await this.resolver(nodo);
      if (!esDict(d)) throw new ErrorPdf('ilegible', 'árbol de páginas');
      const propia = await caja(d, heredada);
      if (esNombre(d.get('Type'), 'Page') || !d.has('Kids')) {
        if (out.length >= maximo) throw new ErrorPdf('ilegible', 'demasiadas páginas');
        out.push({ n: nodo.n, caja: propia });
        return;
      }
      const kids = await this.resolver(d.get('Kids'));
      if (!Array.isArray(kids)) throw new ErrorPdf('ilegible', 'árbol de páginas');
      for (const k of kids) await recorrer(k, propia, nivel + 1);
    };
    await recorrer(raiz.get('Pages'), CAJA_POR_DEFECTO, 0);
    if (out.length === 0) throw new ErrorPdf('ilegible', 'sin páginas');
    return out;
  }
}
