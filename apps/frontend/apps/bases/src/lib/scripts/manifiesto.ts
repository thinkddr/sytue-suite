/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Manifiesto de permisos de un script de Sytue Bases (ADR 0069): qué operaciones de la
 * superficie pide, sobre qué tablas de la base, si escribe y con qué cotas.
 *
 * Es el espejo EXACTO de `domain.ParseManifiesto` (services/bases): mismas reglas, mismas
 * cotas, mismo JSON canónico. El servidor lo guarda normalizado y es ese el que se enseña
 * y se ejecuta; aquí se valida antes de mandarlo y, sobre todo, ANTES DE EJECUTAR lo que
 * el servidor devuelve. Módulo PURO, sin DOM.
 */
import { sha256Hex } from '@sytue/scripts';

import { OPERACIONES, escribe, esOperacion } from './superficie.js';
import type { Operacion } from './superficie.js';

export const MAX_TABLAS = 50;
export const TOPE_OPS = 500;
export const TOPE_MS = 30_000;
export const OPS_POR_DEFECTO = 100;
export const MS_POR_DEFECTO = 5_000;

export interface ManifiestoScript {
  v: 1;
  ops: Operacion[];
  /** Ids de tabla. Vacío = todas las de la base que la persona lea. */
  tables: string[];
  write: boolean;
  max_ops: number;
  max_ms: number;
}

export type ResultadoManifiesto =
  { ok: true; manifiesto: ManifiestoScript } | { ok: false; error: string };

const CLAVES = new Set(['v', 'ops', 'tables', 'write', 'max_ops', 'max_ms']);
export const RE_UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

const ordenado = <S extends string>(l: readonly S[]): S[] => [...new Set(l)].sort();
const lista = (x: unknown, max: number): string[] | null =>
  Array.isArray(x) && x.length <= max && x.every((s) => typeof s === 'string') ? x : null;

/** Valida y normaliza un manifiesto de origen no fiable. Nunca lanza. */
export function validarManifiesto(x: unknown): ResultadoManifiesto {
  const mal = (error: string): ResultadoManifiesto => ({ ok: false, error });
  if (typeof x !== 'object' || x === null || Array.isArray(x)) return mal('objeto');
  const m = x as Record<string, unknown>;
  for (const k of Object.keys(m)) if (!CLAVES.has(k)) return mal(`campo:${k}`);
  if (m.v !== 1) return mal('version');
  if (typeof m.write !== 'boolean') return mal('write');
  const ops = lista(m.ops ?? [], 2 * OPERACIONES.length);
  if (!ops || !ops.every(esOperacion)) return mal('ops');
  // Una operación que escribe sin declarar escritura es un manifiesto que dice dos cosas.
  if (!m.write && ops.some(escribe)) return mal('write');
  const tablas = lista(m.tables ?? [], MAX_TABLAS)?.map((t) => t.toLowerCase());
  if (!tablas || !tablas.every((t) => RE_UUID.test(t))) return mal('tables');
  const maxOps = m.max_ops === undefined || m.max_ops === 0 ? OPS_POR_DEFECTO : m.max_ops;
  const maxMs = m.max_ms === undefined || m.max_ms === 0 ? MS_POR_DEFECTO : m.max_ms;
  if (
    typeof maxOps !== 'number' ||
    typeof maxMs !== 'number' ||
    !Number.isInteger(maxOps) ||
    !Number.isInteger(maxMs) ||
    maxOps < 1 ||
    maxOps > TOPE_OPS ||
    maxMs < 100 ||
    maxMs > TOPE_MS
  )
    return mal('limites');
  return {
    ok: true,
    manifiesto: {
      v: 1,
      ops: ordenado(ops),
      tables: ordenado(tablas),
      write: m.write,
      max_ops: maxOps,
      max_ms: maxMs,
    },
  };
}

/** JSON canónico (claves en el orden del servidor): lo que se guarda, se compara y se resume. */
export const canonico = (m: ManifiestoScript): string =>
  JSON.stringify({
    v: m.v,
    ops: m.ops,
    tables: m.tables,
    write: m.write,
    max_ops: m.max_ops,
    max_ms: m.max_ms,
  });

/** SHA-256 del manifiesto canónico: el consentimiento se da sobre ESTE hash. */
export const hashManifiesto = (m: ManifiestoScript): Promise<string> => sha256Hex(canonico(m));

/** Una línea del diálogo de consentimiento: clave de texto y parámetros. */
export interface LineaConsentimiento {
  clave:
    | 'scripts.consent.write'
    | 'scripts.consent.readOnly'
    | 'scripts.consent.ops'
    | 'scripts.consent.scope'
    | 'scripts.consent.allTables'
    | 'scripts.consent.limits';
  params?: Record<string, string | number>;
}

/**
 * Lo que el manifiesto pide, en el orden en que importa a quien lo lee. `nombreDeTabla`
 * pone el nombre a cada id (una tabla que ya no existe sale con su id).
 */
export function describirManifiesto(
  m: ManifiestoScript,
  nombreDeTabla: (id: string) => string | undefined,
): LineaConsentimiento[] {
  const l: LineaConsentimiento[] = [
    { clave: m.write ? 'scripts.consent.write' : 'scripts.consent.readOnly' },
  ];
  if (m.ops.length > 0)
    l.push({ clave: 'scripts.consent.ops', params: { list: m.ops.join(', ') } });
  l.push(
    m.tables.length > 0
      ? {
          clave: 'scripts.consent.scope',
          params: { scope: m.tables.map((t) => nombreDeTabla(t) ?? t).join(', ') },
        }
      : { clave: 'scripts.consent.allTables' },
  );
  l.push({
    clave: 'scripts.consent.limits',
    params: { ops: m.max_ops, seconds: Math.round(m.max_ms / 100) / 10 },
  });
  return l;
}
