/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * Expresiones regulares en tiempo lineal para REGEXTEST, REGEXEXTRACT y REGEXREPLACE.
 *
 * Con el RegExp de JavaScript, una fórmula como `=REGEXTEST(A1; "(a+)+$")` congelaba con
 * vuelta atrás la pestaña de cada persona que abría la hoja, también la de quien tendría
 * que borrarla (WEB-13). Esto es una máquina de Pike —la de Thompson con capturas, la de
 * RE2—: recorre el texto una vez, con todos los caminos del patrón a la vez, y su coste
 * es el largo del texto por el tamaño del patrón, sin vuelta atrás.
 *
 * La sintaxis es la de JavaScript sin lo que obliga a volver atrás: las referencias hacia
 * atrás y el lookaround dan #VALUE!, como en el motor nativo (apps/nucleo/formula, con el
 * regexp de Go, que es RE2). Las coincidencias también son las de RE2: la que empieza más
 * a la izquierda y, de las que empiezan ahí, la que prefieren las alternativas y los
 * cuantificadores; un grupo dentro de una repetición recuerda su última vuelta.
 */

type Pred = (c: number) => boolean;
type Aserto = 'ini' | 'fin' | 'limite' | 'nolimite';

type Nodo =
  | { k: 'car'; p: Pred }
  | { k: 'cat'; hijos: Nodo[] }
  | { k: 'alt'; hijos: Nodo[] }
  | { k: 'grupo'; n: number; hijo: Nodo }
  | { k: 'rep'; hijo: Nodo; min: number; max: number; codicioso: boolean }
  | { k: 'aserto'; a: Aserto };

type Inst =
  | { op: 'car'; p: Pred }
  | { op: 'div'; x: number; y: number }
  | { op: 'salto'; x: number }
  | { op: 'marca'; n: number }
  | { op: 'aserto'; a: Aserto }
  | { op: 'fin' };

/** Un patrón compilado: las instrucciones y cuántos grupos de captura tiene. */
export interface Programa {
  readonly inst: readonly Inst[];
  readonly grupos: number;
}

/** Una coincidencia: inicio y fin de la entera y de cada grupo; -1 si no participó. */
export type Coincidencia = number[];

// Topes: una repetición no pasa de 1000 vueltas (el de RE2) y el programa no pasa de 2000
// instrucciones, que con el texto de una celda (~32k) deja el peor caso en décimas.
const MAX_REPETICION = 1000;
const MAX_INSTRUCCIONES = 2000;

class NoAdmitido extends Error {}

const esDigito: Pred = (c) => c >= 48 && c <= 57;
const esPalabra: Pred = (c) =>
  esDigito(c) || (c >= 65 && c <= 90) || (c >= 97 && c <= 122) || c === 95;
const esEspacio: Pred = (c) =>
  (c >= 9 && c <= 13) ||
  c === 32 ||
  c === 0xa0 ||
  c === 0x1680 ||
  (c >= 0x2000 && c <= 0x200a) ||
  c === 0x2028 ||
  c === 0x2029 ||
  c === 0x202f ||
  c === 0x205f ||
  c === 0x3000 ||
  c === 0xfeff;
const finDeLinea: Pred = (c) => c === 10 || c === 13 || c === 0x2028 || c === 0x2029;
const no =
  (p: Pred): Pred =>
  (c) =>
    !p(c);

/** Canonicalize de ECMAScript sin `u`: la mayúscula simple, si es un solo carácter. */
function canonica(c: number): number {
  if (c < 128) return c >= 97 && c <= 122 ? c - 32 : c;
  const m = String.fromCharCode(c).toUpperCase();
  if (m.length !== 1) return c;
  const u = m.charCodeAt(0);
  return u < 128 ? c : u;
}

class Lector {
  private i = 0;
  grupos = 0;

  constructor(
    private readonly src: string,
    private readonly sinCaja: boolean,
  ) {}

  leer(): Nodo {
    const n = this.alternativa();
    if (this.i < this.src.length) throw new NoAdmitido(); // un `)` sin abrir
    return n;
  }

  private ver(d = 0): string {
    return this.src.charAt(this.i + d);
  }

  private alternativa(): Nodo {
    const ramas = [this.secuencia()];
    while (this.ver() === '|') {
      this.i++;
      ramas.push(this.secuencia());
    }
    return ramas.length === 1 ? ramas[0]! : { k: 'alt', hijos: ramas };
  }

  private secuencia(): Nodo {
    const hijos: Nodo[] = [];
    while (this.i < this.src.length && this.ver() !== '|' && this.ver() !== ')') {
      hijos.push(this.cuantificado(this.atomo()));
    }
    return { k: 'cat', hijos };
  }

  /** `{n}`, `{n,}` o `{n,m}` en la posición actual, o null si `{` es un carácter. */
  private llaves(): [number, number, number] | null {
    const m = /^\{(\d+)(,(\d*))?\}/.exec(this.src.slice(this.i));
    if (!m) return null;
    const min = Number(m[1]);
    const max = m[2] === undefined ? min : m[3] === '' ? Infinity : Number(m[3]);
    return [min, max, m[0].length];
  }

  private cuantificado(atomo: Nodo): Nodo {
    let min: number;
    let max: number;
    const c = this.ver();
    if (c === '*' || c === '+' || c === '?') {
      [min, max] = c === '*' ? [0, Infinity] : c === '+' ? [1, Infinity] : [0, 1];
      this.i++;
    } else if (c === '{' && this.llaves()) {
      const [a, b, largo] = this.llaves()!;
      [min, max] = [a, b];
      this.i += largo;
    } else {
      return atomo;
    }
    if (
      atomo.k === 'aserto' ||
      min > max ||
      min > MAX_REPETICION ||
      (max !== Infinity && max > MAX_REPETICION)
    ) {
      throw new NoAdmitido();
    }
    let codicioso = true;
    if (this.ver() === '?') {
      codicioso = false;
      this.i++;
    }
    const sig = this.ver();
    if (sig === '*' || sig === '+' || sig === '?' || (sig === '{' && this.llaves()))
      throw new NoAdmitido();
    return { k: 'rep', hijo: atomo, min, max, codicioso };
  }

  private atomo(): Nodo {
    const c = this.ver();
    switch (c) {
      case '(':
        return this.grupo();
      case '[':
        return { k: 'car', p: this.clase() };
      case '.':
        this.i++;
        return { k: 'car', p: no(finDeLinea) };
      case '^':
        this.i++;
        return { k: 'aserto', a: 'ini' };
      case '$':
        this.i++;
        return { k: 'aserto', a: 'fin' };
      case '\\':
        return this.escape();
      case '*':
      case '+':
      case '?':
        throw new NoAdmitido(); // nada que repetir
      case '{':
        if (this.llaves()) throw new NoAdmitido();
        break;
    }
    this.i++;
    return { k: 'car', p: this.igual(c.charCodeAt(0)) };
  }

  private grupo(): Nodo {
    this.i++; // (
    let n = -1;
    if (this.ver() === '?') {
      if (this.ver(1) === ':') {
        this.i += 2;
      } else if (this.ver(1) === '<' && this.ver(2) !== '=' && this.ver(2) !== '!') {
        const m = /^\?<([A-Za-z_$][\w$]*)>/.exec(this.src.slice(this.i));
        if (!m) throw new NoAdmitido();
        this.i += m[0].length;
        n = ++this.grupos;
      } else {
        throw new NoAdmitido(); // lookaround y lo que JavaScript no admite
      }
    } else {
      n = ++this.grupos;
    }
    const hijo = this.alternativa();
    if (this.ver() !== ')') throw new NoAdmitido();
    this.i++;
    return { k: 'grupo', n, hijo };
  }

  private escape(): Nodo {
    const c = this.ver(1);
    if (c === 'b' || c === 'B') {
      this.i += 2;
      return { k: 'aserto', a: c === 'b' ? 'limite' : 'nolimite' };
    }
    // Una referencia hacia atrás obliga a volver atrás: no se admite (como en RE2).
    if ((c >= '1' && c <= '9') || (c === 'k' && this.ver(2) === '<')) throw new NoAdmitido();
    const e = this.escapeComun();
    return { k: 'car', p: typeof e === 'number' ? this.igual(e) : e };
  }

  /** Un escape de dentro o de fuera de una clase: un carácter o una familia. */
  private escapeComun(): number | Pred {
    this.i++; // \
    const c = this.ver();
    if (c === '') throw new NoAdmitido();
    this.i++;
    switch (c) {
      case 'd':
        return esDigito;
      case 'D':
        return no(esDigito);
      case 'w':
        return esPalabra;
      case 'W':
        return no(esPalabra);
      case 's':
        return esEspacio;
      case 'S':
        return no(esEspacio);
      case 't':
        return 9;
      case 'n':
        return 10;
      case 'v':
        return 11;
      case 'f':
        return 12;
      case 'r':
        return 13;
      case '0':
        if (esDigito(this.src.charCodeAt(this.i))) throw new NoAdmitido(); // octal
        return 0;
      case 'x':
      case 'u': {
        const largo = c === 'x' ? 2 : 4;
        const hex = this.src.slice(this.i, this.i + largo);
        if (hex.length === largo && /^[0-9A-Fa-f]+$/.test(hex)) {
          this.i += largo;
          return parseInt(hex, 16);
        }
        return c.charCodeAt(0); // escape de identidad, como JavaScript
      }
      case 'c': {
        const l = this.src.charCodeAt(this.i);
        if ((l >= 65 && l <= 90) || (l >= 97 && l <= 122)) {
          this.i++;
          return l % 32;
        }
        throw new NoAdmitido();
      }
      default:
        if (esDigito(c.charCodeAt(0))) throw new NoAdmitido();
        return c.charCodeAt(0);
    }
  }

  private clase(): Pred {
    this.i++; // [
    let negada = false;
    if (this.ver() === '^') {
      negada = true;
      this.i++;
    }
    const partes: Pred[] = [];
    for (;;) {
      if (this.i >= this.src.length) throw new NoAdmitido();
      if (this.ver() === ']') {
        this.i++;
        break;
      }
      const a = this.elementoDeClase();
      if (
        typeof a === 'number' &&
        this.ver() === '-' &&
        this.ver(1) !== ']' &&
        this.i + 1 < this.src.length
      ) {
        this.i++; // -
        const b = this.elementoDeClase();
        if (typeof b !== 'number' || a > b) throw new NoAdmitido();
        partes.push((c) => c >= a && c <= b);
      } else {
        partes.push(typeof a === 'number' ? (c) => c === a : a);
      }
    }
    const enClase: Pred = (c) => partes.some((p) => p(c));
    const p = this.sinCaja ? this.plegada(enClase) : enClase;
    return negada ? no(p) : p;
  }

  private elementoDeClase(): number | Pred {
    if (this.ver() !== '\\') {
      return this.src.charCodeAt(this.i++);
    }
    if (this.ver(1) === 'b') {
      this.i += 2;
      return 8; // dentro de una clase, \b es el retroceso
    }
    return this.escapeComun();
  }

  private igual(code: number): Pred {
    if (!this.sinCaja) return (c) => c === code;
    const k = canonica(code);
    return (c) => c === code || canonica(c) === k;
  }

  /** Una clase sin distinguir mayúsculas: casa si casa el carácter o alguna de sus cajas. */
  private plegada(p: Pred): Pred {
    return (c) => {
      if (p(c)) return true;
      const s = String.fromCharCode(c);
      const may = s.toUpperCase();
      const min = s.toLowerCase();
      return (
        (may.length === 1 && p(may.charCodeAt(0))) || (min.length === 1 && p(min.charCodeAt(0)))
      );
    };
  }
}

function emitir(n: Nodo, prog: Inst[]): void {
  switch (n.k) {
    case 'car':
      prog.push({ op: 'car', p: n.p });
      break;
    case 'aserto':
      prog.push({ op: 'aserto', a: n.a });
      break;
    case 'cat':
      for (const h of n.hijos) emitir(h, prog);
      break;
    case 'grupo':
      if (n.n >= 0) prog.push({ op: 'marca', n: 2 * n.n });
      emitir(n.hijo, prog);
      if (n.n >= 0) prog.push({ op: 'marca', n: 2 * n.n + 1 });
      break;
    case 'alt': {
      const salidas: number[] = [];
      for (let k = 0; k < n.hijos.length - 1; k++) {
        const div = prog.length;
        prog.push({ op: 'div', x: div + 1, y: -1 });
        emitir(n.hijos[k]!, prog);
        salidas.push(prog.length);
        prog.push({ op: 'salto', x: -1 });
        prog[div] = { op: 'div', x: div + 1, y: prog.length };
      }
      emitir(n.hijos[n.hijos.length - 1]!, prog);
      for (const s of salidas) prog[s] = { op: 'salto', x: prog.length };
      break;
    }
    case 'rep': {
      for (let k = 0; k < n.min; k++) emitir(n.hijo, prog);
      const ramas: number[] = [];
      if (n.max === Infinity) {
        const div = prog.length;
        prog.push({ op: 'div', x: -1, y: -1 });
        emitir(n.hijo, prog);
        prog.push({ op: 'salto', x: div });
        ramas.push(div);
      } else {
        for (let k = n.min; k < n.max; k++) {
          ramas.push(prog.length);
          prog.push({ op: 'div', x: -1, y: -1 });
          emitir(n.hijo, prog);
        }
      }
      const fuera = prog.length;
      for (const d of ramas) {
        prog[d] = n.codicioso
          ? { op: 'div', x: d + 1, y: fuera }
          : { op: 'div', x: fuera, y: d + 1 };
      }
      break;
    }
  }
  if (prog.length > MAX_INSTRUCCIONES) throw new NoAdmitido();
}

/** Compila un patrón; null si no es válido, si no se admite o si es desmesurado. */
export function compilarRegex(patron: string, sinCaja: boolean): Programa | null {
  try {
    const lector = new Lector(patron, sinCaja);
    const arbol = lector.leer();
    const inst: Inst[] = [{ op: 'marca', n: 0 }];
    emitir(arbol, inst);
    inst.push({ op: 'marca', n: 1 }, { op: 'fin' });
    if (inst.length > MAX_INSTRUCCIONES) return null;
    return { inst, grupos: lector.grupos };
  } catch (e) {
    if (e instanceof NoAdmitido) return null;
    throw e;
  }
}

interface Hilo {
  pc: number;
  caps: number[];
}

/** Más pasos de los que se dan a una fórmula: el patrón y el texto juntos son desmesurados. */
export class Desmesurado extends Error {}

// Pasos de la máquina por llamada a una función: una búsqueda normal en una celda gasta
// miles; el tope deja el peor caso (todas las coincidencias de un patrón que sigue vivo
// hasta el final del texto, cuadrático como en RE2) en décimas de segundo.
const PRESUPUESTO = 10_000_000;

function cumple(a: Aserto, t: string, pos: number): boolean {
  switch (a) {
    case 'ini':
      return pos === 0;
    case 'fin':
      return pos === t.length;
    default: {
      const antes = pos > 0 && esPalabra(t.charCodeAt(pos - 1));
      const despues = pos < t.length && esPalabra(t.charCodeAt(pos));
      return (antes !== despues) === (a === 'limite');
    }
  }
}

/** Un patrón compilado ante un texto. Cada búsqueda gasta del mismo presupuesto. */
export class Maquina {
  // Sello de la posición en que cada instrucción entró en la lista: `base + pos + 1`. La
  // base cambia en cada búsqueda para no confundir los sellos de una con los de otra.
  private readonly visto: Float64Array;
  private base = 0;
  private pasos = 0;

  constructor(
    private readonly prog: Programa,
    private readonly t: string,
  ) {
    this.visto = new Float64Array(prog.inst.length);
  }

  private anadir(lista: Hilo[], pc0: number, caps0: number[], pos: number): void {
    const { inst } = this.prog;
    const sello = this.base + pos + 1;
    const pila: Hilo[] = [{ pc: pc0, caps: caps0 }];
    while (pila.length > 0) {
      const { pc, caps } = pila.pop()!;
      if (this.visto[pc] === sello) continue;
      this.visto[pc] = sello;
      if (++this.pasos > PRESUPUESTO) throw new Desmesurado();
      const i = inst[pc]!;
      switch (i.op) {
        case 'salto':
          pila.push({ pc: i.x, caps });
          break;
        case 'div':
          pila.push({ pc: i.y, caps }, { pc: i.x, caps }); // x primero: es la preferida
          break;
        case 'marca': {
          const otra = caps.slice();
          otra[i.n] = pos;
          pila.push({ pc: pc + 1, caps: otra });
          break;
        }
        case 'aserto':
          if (cumple(i.a, this.t, pos)) pila.push({ pc: pc + 1, caps });
          break;
        default:
          lista.push({ pc, caps });
      }
    }
  }

  /**
   * La primera coincidencia que empieza en `desde` o después, o null. Los hilos van en
   * orden de preferencia; el que llega a `fin` corta a los que van detrás, y uno de
   * delante que llegue después lo sustituye: así gana la coincidencia más a la izquierda
   * y, de las que empiezan ahí, la preferida.
   */
  buscar(desde: number): Coincidencia | null {
    const { inst } = this.prog;
    const t = this.t;
    this.base += t.length + 2;
    const vacias = new Array<number>(2 * (this.prog.grupos + 1)).fill(-1);
    let actual: Hilo[] = [];
    let hallada: Coincidencia | null = null;
    for (let pos = desde; pos <= t.length; pos++) {
      if (hallada === null) this.anadir(actual, 0, vacias, pos);
      else if (actual.length === 0) break; // ya no queda nadie que pueda mejorarla
      const c = pos < t.length ? t.charCodeAt(pos) : -1;
      const siguiente: Hilo[] = [];
      for (const h of actual) {
        const i = inst[h.pc]!;
        if (i.op === 'fin') {
          hallada = h.caps;
          break;
        }
        if (i.op === 'car' && c >= 0 && i.p(c)) this.anadir(siguiente, h.pc + 1, h.caps, pos + 1);
      }
      actual = siguiente;
    }
    return hallada;
  }

  /**
   * Todas las coincidencias, sin solaparse, como `FindAll` de Go: una vacía pegada al
   * final de la anterior no cuenta.
   */
  todas(): Coincidencia[] {
    const t = this.t;
    const out: Coincidencia[] = [];
    let pos = 0;
    let finAnterior = -1;
    while (pos <= t.length) {
      const m = this.buscar(pos);
      if (m === null) break;
      let vale = true;
      if (m[1] === pos) {
        if (m[0] === finAnterior) vale = false;
        const c = t.charCodeAt(pos);
        const par =
          c >= 0xd800 &&
          c <= 0xdbff &&
          pos + 1 < t.length &&
          (t.charCodeAt(pos + 1) & 0xfc00) === 0xdc00;
        pos += par ? 2 : 1;
      } else {
        pos = m[1]!;
      }
      finAnterior = m[1]!;
      if (vale) out.push(m);
    }
    return out;
  }
}

/** El texto del grupo `n` de una coincidencia, o '' si no participó. */
export function grupo(t: string, m: Coincidencia, n: number): string {
  const ini = m[2 * n];
  const fin = m[2 * n + 1];
  return ini === undefined || ini < 0 || fin === undefined || fin < 0 ? '' : t.slice(ini, fin);
}
