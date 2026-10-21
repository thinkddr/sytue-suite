/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Calcula el corpus del suavizado exponencial (ets-corpus.ts) con LibreOffice y guarda lo que
 * contestó en `ets-esperado-libreoffice.json`. Lo contrastan `ets.test.ts` (motor de
 * TypeScript) y `apps/nucleo/formula/ets_compat_test.go` (motor de Go), con las tolerancias
 * de `ets-tolerancias.json`: el algoritmo de LibreOffice optimiza los parámetros por
 * bisección a 0,001 y el nuestro con una búsqueda por patrones a 1e-4, así que NO coinciden a
 * 1e-9 como el resto del corpus (docs/architecture.md#calcus-compatibilidad).
 *
 *   cd apps/frontend/packages/formula && bun compat/ets-libreoffice.ts
 */
import { writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { DATOS_ETS, FORMULAS_ETS } from './ets-corpus';
import { calcular } from './libreoffice';

/** Las fórmulas de intervalo: LibreOffice las calcula por SIMULACIÓN y cada ejecución da otro
 *  valor (±15 % medido), así que se repiten y se guarda la media y el rango. */
export const esIntervalo = (f: string): boolean => /\.PI\.|\.CONFINT\(/.test(f);

/** Cuántas veces se calcula cada intervalo para tener su media y su rango. */
const REPETICIONES = 25;

if (import.meta.main) {
  const e = await calcular(FORMULAS_ETS, DATOS_ETS);
  const pi = FORMULAS_ETS.filter(esIntervalo);
  const muestras = new Map<string, number[]>(pi.map((f) => [f, []]));
  for (let i = 0; i < REPETICIONES; i++) {
    const r = await calcular(pi, DATOS_ETS);
    for (const c of r.casos) if (typeof c.lo === 'number') muestras.get(c.f)!.push(c.lo);
  }
  const intervalos: Record<string, { media: number; min: number; max: number; n: number }> = {};
  for (const [f, v] of muestras)
    if (v.length > 0)
      intervalos[f] = {
        media: v.reduce((a, b) => a + b, 0) / v.length,
        min: Math.min(...v),
        max: Math.max(...v),
        n: v.length,
      };
  const ruta = new URL('./ets-esperado-libreoffice.json', import.meta.url);
  writeFileSync(ruta, JSON.stringify({ ...e, solo365: undefined, intervalos }, null, 1) + '\n');
  Bun.spawnSync(['bunx', 'prettier@3.9.9', '--write', fileURLToPath(ruta)]);
  process.stdout.write(`${e.casos.length} fórmulas calculadas con ${e.libreoffice}\n`);
}
