/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Manifiesto de permisos de un script de usuario (ADR 0069). Declara lo que el script
 * necesita: comandos del registro común, familias de lectura, escritura, ámbito (hojas y
 * rangos), cotas de operaciones y tiempo, y si puede correr sin que nadie lo pulse.
 *
 * Es el espejo EXACTO de `domain.ParseManifiesto` (services/docs): mismas cotas, mismo
 * JSON canónico. El servidor lo guarda normalizado y es ese el que se enseña y se
 * ejecuta; aquí además se exige que cada comando exista en el registro y sea
 * reproducible, que el servidor no puede saber. Módulo PURO, sin DOM.
 */
import { sha256Hex } from '@sytue/scripts';

import { esComandoDeScript } from '../comandos';
import { NON_REPLAYABLE } from '../macros';

export const MANIFIESTO_VERSION = 1;
export const MAX_COMANDOS = 100;
export const MAX_AMBITO = 50;
export const TOPE_OPS = 10_000;
export const TOPE_MS = 30_000;
export const OPS_POR_DEFECTO = 1_000;
export const MS_POR_DEFECTO = 5_000;

export const LECTURAS = ['cells', 'selection', 'slides', 'text', 'title'] as const;
export type Lectura = (typeof LECTURAS)[number];

export interface AmbitoManifiesto {
  sheets?: string[];
  ranges?: string[];
}

export interface ManifiestoScript {
  v: 1;
  commands: string[];
  read: Lectura[];
  write: boolean;
  scope: AmbitoManifiesto;
  max_ops: number;
  max_ms: number;
  unattended: boolean;
}

export type ResultadoManifiesto =
  { ok: true; manifiesto: ManifiestoScript } | { ok: false; error: string };

const CLAVES = new Set([
  'v',
  'commands',
  'read',
  'write',
  'scope',
  'max_ops',
  'max_ms',
  'unattended',
]);
const RE_RANGO = /^[A-Z]{1,3}\d{1,7}(?::[A-Z]{1,3}\d{1,7})?$/;

const esObjeto = (x: unknown): x is Record<string, unknown> =>
  typeof x === 'object' && x !== null && !Array.isArray(x);

const ordenado = (l: readonly string[]): string[] => [...new Set(l)].sort();

const listaDeCadenas = (x: unknown, max: number): string[] | null =>
  Array.isArray(x) && x.length <= max && x.every((s) => typeof s === 'string') ? x : null;

/** Valida y normaliza un manifiesto de origen no fiable. Nunca lanza. */
export function validarManifiesto(x: unknown): ResultadoManifiesto {
  const mal = (error: string): ResultadoManifiesto => ({ ok: false, error });
  if (!esObjeto(x)) return mal('El manifiesto no es un objeto.');
  for (const k of Object.keys(x)) if (!CLAVES.has(k)) return mal(`Campo desconocido: ${k}.`);
  if (x.v !== MANIFIESTO_VERSION) return mal('Versión de manifiesto no soportada.');

  const comandos = listaDeCadenas(x.commands ?? [], MAX_COMANDOS);
  if (!comandos) return mal('La lista de comandos no es válida.');
  for (const c of comandos) {
    if (!/^[a-zA-Z][a-zA-Z0-9]{0,31}$/.test(c)) return mal('Nombre de comando no válido.');
    if (!esComandoDeScript(c) || NON_REPLAYABLE.has(c))
      return mal(`Comando no disponible para scripts: ${c}.`);
  }
  const lecturas = listaDeCadenas(x.read ?? [], LECTURAS.length * 2);
  if (!lecturas || !lecturas.every((l) => (LECTURAS as readonly string[]).includes(l)))
    return mal('Lectura desconocida.');
  if (typeof x.write !== 'boolean') return mal('«write» debe ser verdadero o falso.');
  if (x.unattended !== undefined && typeof x.unattended !== 'boolean')
    return mal('«unattended» debe ser verdadero o falso.');

  const ambito = x.scope ?? {};
  if (!esObjeto(ambito) || Object.keys(ambito).some((k) => k !== 'sheets' && k !== 'ranges'))
    return mal('Ámbito no válido.');
  const hojas = listaDeCadenas(ambito.sheets ?? [], MAX_AMBITO);
  const rangos = listaDeCadenas(ambito.ranges ?? [], MAX_AMBITO);
  if (!hojas || hojas.some((h) => h === '' || Array.from(h).length > 64))
    return mal('Hoja no válida.');
  if (!rangos || rangos.some((r) => !RE_RANGO.test(r))) return mal('Rango no válido.');

  const ops = x.max_ops === undefined || x.max_ops === 0 ? OPS_POR_DEFECTO : x.max_ops;
  const ms = x.max_ms === undefined || x.max_ms === 0 ? MS_POR_DEFECTO : x.max_ms;
  if (
    !Number.isInteger(ops) ||
    !Number.isInteger(ms) ||
    (ops as number) < 1 ||
    (ops as number) > TOPE_OPS ||
    (ms as number) < 100 ||
    (ms as number) > TOPE_MS
  )
    return mal('Límites fuera de rango.');

  const scope: AmbitoManifiesto = {};
  if (hojas.length > 0) scope.sheets = ordenado(hojas);
  if (rangos.length > 0) scope.ranges = ordenado(rangos);
  return {
    ok: true,
    manifiesto: {
      v: 1,
      commands: ordenado(comandos),
      read: ordenado(lecturas) as Lectura[],
      write: x.write,
      scope,
      max_ops: ops as number,
      max_ms: ms as number,
      unattended: x.unattended === true,
    },
  };
}

/** JSON canónico (claves en orden fijo): lo que se guarda, se compara y se resume. */
export const canonico = (m: ManifiestoScript): string =>
  JSON.stringify({
    v: m.v,
    commands: m.commands,
    read: m.read,
    write: m.write,
    scope: {
      ...(m.scope.sheets ? { sheets: m.scope.sheets } : {}),
      ...(m.scope.ranges ? { ranges: m.scope.ranges } : {}),
    },
    max_ops: m.max_ops,
    max_ms: m.max_ms,
    unattended: m.unattended,
  });

/** SHA-256 hexadecimal del manifiesto canónico: el consentimiento se da sobre ESTE hash. */
export const hashManifiesto = (m: ManifiestoScript): Promise<string> => sha256Hex(canonico(m));

/* ---------- Ámbito: hojas y rangos ---------- */

const colNum = (letras: string): number => {
  let n = 0;
  for (const c of letras) n = n * 26 + (c.charCodeAt(0) - 64);
  return n;
};

interface Caja {
  c1: number;
  r1: number;
  c2: number;
  r2: number;
}

/** «B2» o «A1:C9» (mayúsculas) a caja; `null` si no es una referencia. */
export function cajaDe(ref: string): Caja | null {
  if (!RE_RANGO.test(ref)) return null;
  const [a, b = a] = ref.split(':') as [string, string?];
  const p = (s: string): [number, number] => {
    const m = /^([A-Z]+)(\d+)$/.exec(s)!;
    return [colNum(m[1]!), Number(m[2])];
  };
  const [c1, r1] = p(a);
  const [c2, r2] = p(b ?? a);
  return { c1: Math.min(c1, c2), r1: Math.min(r1, r2), c2: Math.max(c1, c2), r2: Math.max(r1, r2) };
}

/** Celdas que abarca una referencia (0 si no es válida). */
export const celdasDe = (ref: string): number => {
  const c = cajaDe(ref);
  return c ? (c.c2 - c.c1 + 1) * (c.r2 - c.r1 + 1) : 0;
};

/**
 * ¿Cae `ref` en `hoja` dentro del ámbito? Un ámbito vacío es todo el documento; con hojas,
 * la hoja ha de figurar; con rangos, la referencia ha de caber ENTERA en alguno.
 */
export function dentroDelAmbito(hoja: string, ref: string, ambito: AmbitoManifiesto): boolean {
  if (ambito.sheets && !ambito.sheets.includes(hoja)) return false;
  if (!ambito.ranges) return true;
  const q = cajaDe(ref);
  if (!q) return false;
  return ambito.ranges.some((r) => {
    const a = cajaDe(r);
    return a !== null && q.c1 >= a.c1 && q.r1 >= a.r1 && q.c2 <= a.c2 && q.r2 <= a.r2;
  });
}
