/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Evalúa el corpus del suavizado exponencial con el motor de Calcus y lo compara con lo que
 * contestó LibreOffice (`ets-esperado-libreoffice.json`). Es lo que usa `ets.test.ts` y lo que
 * imprime el informe (mediana y máximo por serie y por familia):
 *
 *   cd apps/frontend/packages/formula && bun compat/ets-comparar.ts
 *
 * A diferencia del corpus principal, aquí NO se exige 1e-9: ni Excel ni LibreOffice publican
 * su optimizador, el de LibreOffice biseca los parámetros a 0,001 y sus intervalos salen de
 * una simulación que cambia entre ejecuciones. Cada familia tiene su tolerancia relativa, con
 * su motivo, en `ets-divergencias.json`; lo que la supera se declara allí fórmula a fórmula.
 */
import { Engine } from '../src/engine';
import { FormulaError } from '../src/values';
import { DATOS_ETS, SERIES, rangos } from './ets-corpus';
import type { Esperado, Resultado } from './libreoffice';

/** El JSON de LibreOffice del corpus ETS: los intervalos llevan media y rango de N ejecuciones. */
export interface EsperadoEts extends Omit<Esperado, 'solo365'> {
  intervalos: Record<string, { media: number; min: number; max: number; n: number }>;
}

/** Familias con tolerancia propia. */
export type Familia =
  'pronostico' | 'intervalo' | 'estacionalidad' | 'parametro' | 'error' | 'paso';

export const FAMILIAS: readonly Familia[] = [
  'pronostico',
  'intervalo',
  'estacionalidad',
  'parametro',
  'error',
  'paso',
];

export interface Tolerancias {
  /** Tolerancia relativa por familia (null: no se compara) y por qué. */
  tolerancias: Record<Familia, { tol: number | null; motivo: string }>;
  /** Las fórmulas que superan la tolerancia de su familia, o cuyo error difiere. */
  divergencias: Record<string, { quien: string; motivo: string }>;
}

export function evaluarEts(formulas: readonly string[]): Resultado[] {
  const e = new Engine({ now: () => new Date(Date.UTC(2026, 9, 2)), random: () => 0.5 });
  for (const [ref, v] of Object.entries(DATOS_ETS)) e.setCell('Hoja1', ref, String(v));
  formulas.forEach((f, i) => e.setCell('Hoja1', `P${i + 1}`, f));
  return formulas.map((_, i) => {
    const v = e.getValue('Hoja1', `P${i + 1}`);
    return v instanceof FormulaError ? { error: v.code } : v;
  });
}

/** Familia de una fórmula del corpus. */
export function familia(f: string): Familia {
  if (/\.PI\.|\.CONFINT\(/.test(f)) return 'intervalo';
  if (f.includes('.SEASONALITY(')) return 'estacionalidad';
  const t = /\.STAT[A-Z.]*\([^,]+,[^,]+,(\d)/.exec(f);
  if (!t) return 'pronostico';
  const k = Number(t[1]);
  return k >= 1 && k <= 3 ? 'parametro' : k === 8 ? 'paso' : 'error';
}

/** Nombre de la serie del corpus que lee la fórmula (por su rango de valores). */
export function serieDe(f: string): string {
  const i = SERIES.findIndex((_, k) => f.includes(`${rangos(k).v},`));
  return i < 0 ? '?' : SERIES[i]!.nombre;
}

export type Veredicto = 'igual' | 'difiere' | 'sin-contraste';

export interface CasoEts {
  f: string;
  familia: Familia;
  serie: string;
  lo: Resultado;
  nuestro: Resultado;
  /** Diferencia relativa (solo entre dos números). */
  rel: number | undefined;
  veredicto: Veredicto;
}

export function compararEts(esperado: EsperadoEts, tol: Tolerancias): CasoEts[] {
  const nuestros = evaluarEts(esperado.casos.map((c) => c.f));
  return esperado.casos.map((c, i) => {
    const nuestro = nuestros[i]!;
    const fam = familia(c.f);
    // Los intervalos se comparan con la MEDIA de las ejecuciones de LibreOffice.
    const media = fam === 'intervalo' ? esperado.intervalos[c.f]?.media : undefined;
    const lo: Resultado = media ?? c.lo;
    let rel: number | undefined;
    let veredicto: Veredicto;
    if (typeof lo === 'number' && typeof nuestro === 'number') {
      rel = Math.abs(nuestro - lo) / Math.max(Math.abs(lo), 1e-9);
      const t = tol.tolerancias[fam].tol;
      veredicto = t === null ? 'sin-contraste' : rel <= t ? 'igual' : 'difiere';
    } else if (typeof lo === 'object' && typeof nuestro === 'object') {
      veredicto = lo.error === nuestro.error ? 'igual' : 'difiere';
    } else veredicto = 'difiere';
    return { f: c.f, familia: fam, serie: serieDe(c.f), lo, nuestro, rel, veredicto };
  });
}

/** Mediana y máximo de una lista (0 si está vacía). */
export function resumen(v: number[]): { n: number; mediana: number; maximo: number } {
  const s = [...v].sort((a, b) => a - b);
  return { n: s.length, mediana: s[s.length >> 1] ?? 0, maximo: s[s.length - 1] ?? 0 };
}

/** ¿Es un pronóstico con todo por defecto (objetivo, valores, tiempo)? */
export const porDefecto = (f: string): boolean => /^=[A-Z.]+\(\d+,[A-Z0-9:]+,[A-Z0-9:]+\)$/.test(f);

if (import.meta.main) {
  const leer = async <T>(n: string): Promise<T> =>
    (await Bun.file(new URL(n, import.meta.url)).json()) as T;
  const esperado = await leer<EsperadoEts>('./ets-esperado-libreoffice.json');
  const tol = await leer<Tolerancias>('./ets-divergencias.json');
  const casos = compararEts(esperado, tol);
  const salida = (t: string): void => void process.stdout.write(`${t}\n`);
  const pct = (x: number): string => `${(100 * x).toFixed(2)} %`;
  const todo = process.argv.includes('--todo');
  salida(`${esperado.libreoffice} · ${casos.length} fórmulas`);
  for (const v of ['igual', 'difiere', 'sin-contraste'] as const)
    salida(`${v}: ${casos.filter((c) => c.veredicto === v).length}`);
  salida('\nPronóstico por serie (diferencia relativa con LibreOffice, todo por defecto):');
  for (const s of SERIES) {
    const r = resumen(
      casos
        .filter((c) => c.familia === 'pronostico' && c.serie === s.nombre && c.rel !== undefined)
        .filter((c) => porDefecto(c.f))
        .map((c) => c.rel!),
    );
    salida(`  ${s.nombre.padEnd(15)} n=${r.n}  mediana ${pct(r.mediana)}  máximo ${pct(r.maximo)}`);
  }
  salida('\nPor familia (todas las fórmulas con número en los dos lados):');
  for (const fam of FAMILIAS) {
    const r = resumen(
      casos.filter((c) => c.familia === fam && c.rel !== undefined).map((c) => c.rel!),
    );
    salida(`  ${fam.padEnd(15)} n=${r.n}  mediana ${pct(r.mediana)}  máximo ${pct(r.maximo)}`);
  }
  const mostrar = (r: Resultado): string => (typeof r === 'object' ? r.error : String(r));
  for (const c of casos.filter((x) => todo || x.veredicto === 'difiere'))
    salida(
      `${c.veredicto.toUpperCase()} [${c.familia}/${c.serie}] ${c.f}\n   nuestro ${mostrar(c.nuestro)}   LibreOffice ${mostrar(c.lo)}   rel ${c.rel === undefined ? '-' : pct(c.rel)}`,
    );
}
