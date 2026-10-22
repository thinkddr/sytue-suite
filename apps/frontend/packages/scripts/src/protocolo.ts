/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Protocolo de mensajes entre el anfitrión (esta página) y el sandbox de scripts
 * (iframe sin `allow-same-origin` + Worker). Es la ÚNICA superficie por la que el código
 * del usuario toca algo: lo que no cabe en estos mensajes no existe.
 *
 * Todo lo que llega del sandbox es HOSTIL, incluido lo que reenvía el iframe (que es
 * código nuestro, pero está al otro lado de la frontera de confianza): se valida con
 * `leerMensaje` y NUNCA se copia ni se fusiona en otro objeto; se construye uno nuevo con
 * los campos ya comprobados. Un objeto con `__proto__`, `constructor` o cualquier clave
 * fuera del conjunto exacto de su tipo se rechaza (prototype pollution). PURO.
 *
 * Es COMÚN a los productos (Docs, Bases): lo único que cambia de uno a otro es la lista
 * cerrada de operaciones, que cada producto pasa a `leerMensaje`. No hay lista por
 * defecto a propósito: una operación que el producto no nombra no existe.
 */

export const LIMITES = {
  /** Bytes (aprox., UTF-16 como caracteres) de un mensaje serializado. */
  maxMensaje: 64 * 1024,
  /** Argumentos de una llamada. */
  maxArgs: 4,
  /** Caracteres de un argumento cadena. */
  maxCadena: 16 * 1024,
  /** Caracteres de una línea de salida. */
  maxLinea: 4096,
  /** Caracteres de salida de una ejecución. */
  maxSalida: 64 * 1024,
  /** Mensajes por segundo desde el sandbox. */
  maxPorSegundo: 200,
  /** Llamadas sin contestar a la vez. */
  maxPendientes: 32,
  /** Celdas en una lectura de rango. */
  maxCeldas: 10_000,
  /** Caracteres de un dato que se devuelve al script. */
  maxDato: 256 * 1024,
} as const;

export type Escalar = string | number | boolean | null;

/** Del sandbox al anfitrión. */
export type MensajeDeSandbox<Op extends string = string> =
  | { t: 'ready' }
  | { t: 'call'; seq: number; op: Op; args: Escalar[] }
  | { t: 'log'; text: string }
  | { t: 'done' }
  | { t: 'fail'; name: string; message: string };

const CLAVES: Readonly<Record<string, readonly string[]>> = {
  ready: ['t'],
  call: ['t', 'seq', 'op', 'args'],
  log: ['t', 'text'],
  done: ['t'],
  fail: ['t', 'name', 'message'],
};

const esEscalar = (x: unknown): x is Escalar =>
  x === null ||
  typeof x === 'boolean' ||
  (typeof x === 'number' && Number.isFinite(x)) ||
  (typeof x === 'string' && x.length <= LIMITES.maxCadena);

/** Objeto llano: prototipo de Object o nulo, nada de clases ni de proxies exóticos. */
const llano = (x: unknown): x is Record<string, unknown> => {
  if (typeof x !== 'object' || x === null || Array.isArray(x)) return false;
  const p = Object.getPrototypeOf(x) as unknown;
  return p === Object.prototype || p === null;
};

/**
 * Valida un mensaje del sandbox y devuelve una COPIA nueva, o el motivo del rechazo.
 * `grande` es lo que pasa del tope de tamaño; `forma`, todo lo demás. `operaciones` es la
 * lista cerrada del producto: una llamada a cualquier otra cosa es `forma`.
 */
export function leerMensaje<Op extends string>(
  crudo: unknown,
  operaciones: readonly Op[],
): { ok: true; mensaje: MensajeDeSandbox<Op> } | { ok: false; motivo: 'forma' | 'grande' } {
  if (!llano(crudo)) return { ok: false, motivo: 'forma' };
  let tamano: number;
  try {
    tamano = JSON.stringify(crudo).length;
  } catch {
    return { ok: false, motivo: 'forma' };
  }
  if (tamano > LIMITES.maxMensaje) return { ok: false, motivo: 'grande' };
  const t = crudo.t;
  if (typeof t !== 'string' || !Object.hasOwn(CLAVES, t)) return { ok: false, motivo: 'forma' };
  const claves = Object.keys(crudo);
  const esperadas = CLAVES[t]!;
  if (claves.length !== esperadas.length || !claves.every((k) => esperadas.includes(k)))
    return { ok: false, motivo: 'forma' };
  switch (t) {
    case 'ready':
    case 'done':
      return { ok: true, mensaje: { t } };
    case 'log':
      if (typeof crudo.text !== 'string') return { ok: false, motivo: 'forma' };
      return crudo.text.length <= LIMITES.maxLinea
        ? { ok: true, mensaje: { t, text: crudo.text } }
        : { ok: false, motivo: 'grande' };
    case 'fail':
      return typeof crudo.name === 'string' &&
        typeof crudo.message === 'string' &&
        crudo.name.length <= 64 &&
        crudo.message.length <= 500
        ? { ok: true, mensaje: { t, name: crudo.name, message: crudo.message } }
        : { ok: false, motivo: 'forma' };
    default: {
      const { seq, op, args } = crudo;
      if (
        typeof seq !== 'number' ||
        !Number.isSafeInteger(seq) ||
        seq < 1 ||
        typeof op !== 'string' ||
        !(operaciones as readonly string[]).includes(op) ||
        !Array.isArray(args) ||
        args.length > LIMITES.maxArgs ||
        !args.every(esEscalar)
      )
        return { ok: false, motivo: 'forma' };
      return {
        ok: true,
        mensaje: { t: 'call', seq, op: op as Op, args: [...args] },
      };
    }
  }
}

/** Respuesta del anfitrión a una llamada. */
export type Respuesta =
  | { t: 'ret'; seq: number; ok: true; value: unknown }
  | { t: 'ret'; seq: number; ok: false; code: string; message: string };

/**
 * Datos devueltos al script: solo escalares, listas y objetos llanos de poca hondura, de
 * tamaño acotado. Construye una estructura NUEVA (sin aliasing con el estado del editor);
 * lo que no cabe se rechaza con `undefined`.
 */
export function sanearDato(valor: unknown): unknown {
  let presupuesto: number = LIMITES.maxDato;
  const ir = (v: unknown, hondo: number): unknown => {
    if (presupuesto < 0 || hondo > 5) throw new Error('dato');
    if (v === null || typeof v === 'boolean') return v;
    if (typeof v === 'number') return Number.isFinite(v) ? v : null;
    if (typeof v === 'string') {
      presupuesto -= v.length;
      if (presupuesto < 0) throw new Error('dato');
      return v;
    }
    if (Array.isArray(v)) {
      presupuesto -= v.length;
      return v.map((x) => ir(x, hondo + 1));
    }
    if (typeof v === 'object' && v !== undefined) {
      const out: Record<string, unknown> = Object.create(null) as Record<string, unknown>;
      for (const [k, x] of Object.entries(v)) {
        if (k === '__proto__') continue;
        presupuesto -= k.length;
        out[k] = ir(x, hondo + 1);
      }
      return out;
    }
    return null; // undefined, funciones, símbolos
  };
  try {
    return ir(valor, 0);
  } catch {
    return undefined;
  }
}
