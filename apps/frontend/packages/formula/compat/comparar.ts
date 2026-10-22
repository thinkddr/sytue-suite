/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Evalúa el corpus con el motor de Calcus y lo compara con lo que contestó LibreOffice
 * (`esperado-libreoffice.json`, generado por libreoffice.ts). Es lo que usa
 * `compat.test.ts` y lo que imprime el informe:
 *
 *   cd apps/frontend/packages/formula && bun compat/comparar.ts
 */
import { catalogo } from '../src/catalogo';
import { Engine } from '../src/engine';
import { FormulaError } from '../src/values';
import { DATOS } from './corpus';
import type { Esperado, Resultado } from './libreoffice';

/** Cómo sale cada caso del contraste. */
export type Veredicto = 'igual' | 'difiere' | 'sin-contraste';

export interface Caso {
  f: string;
  lo: Resultado;
  nuestro: Resultado;
  veredicto: Veredicto;
}

/** Tolerancia relativa: LibreOffice escribe 15 cifras significativas en el CSV. */
const TOL = 1e-9;

/** ¿Dos números son el mismo a la precisión del CSV de LibreOffice? */
export function mismoNumero(a: number, b: number): boolean {
  return Math.abs(a - b) <= Math.max(1e-12, TOL * Math.max(Math.abs(a), Math.abs(b)));
}

/** Un complejo de Excel («3+4i»): LibreOffice lo escribe con 15 cifras y nosotros con 17. */
const COMPLEJO = /^([-+]?[\d.]+(?:E[-+]?\d+)?)([-+][\d.]*(?:E[-+]?\d+)?)([ij])$/i;

export function iguales(a: Resultado, b: Resultado): boolean {
  if (typeof a === 'number' && typeof b === 'number') return mismoNumero(a, b);
  if (typeof a === 'string' && typeof b === 'string') {
    const ca = COMPLEJO.exec(a);
    const cb = COMPLEJO.exec(b);
    if (ca && cb) {
      const im = (t: string): number => Number(t === '+' ? 1 : t === '-' ? -1 : t);
      return mismoNumero(Number(ca[1]), Number(cb[1])) && mismoNumero(im(ca[2]!), im(cb[2]!));
    }
  }
  if (typeof a === 'object' && typeof b === 'object') return a.error === b.error;
  return a === b;
}

/** Lo que LibreOffice no sabe contestar: no tiene la función o no entiende la sintaxis. */
export const sinContraste = (lo: Resultado): boolean =>
  typeof lo === 'object' && (lo.error === '#NAME?' || lo.error.startsWith('Err:'));

/** Las funciones de Excel 365: LibreOffice 7.4 no las tiene (llegaron a la 24.8), y las que
 *  intenta calcular con otro nombre dan un error que no es el de Excel. */
const SOLO_365 = new Set(
  catalogo()
    .filter((f) => f.compat === 'excel365')
    .map((f) => f.nombre),
);

/** ¿La fórmula usa alguna función que LibreOffice 7.4 no tiene? */
export const usaFuncion365 = (f: string): boolean =>
  [...f.matchAll(/([A-Z][A-Z0-9.]*)\(/g)].some((m) => SOLO_365.has(m[1]!));

/** El libro con los datos del corpus; el reloj y el azar son fijos. */
export function motor(): Engine {
  const e = new Engine({ now: () => new Date(Date.UTC(2026, 9, 2)), random: () => 0.5 });
  for (const [ref, v] of Object.entries(DATOS)) e.setCell('Hoja1', ref, String(v));
  return e;
}

/** Las fórmulas evaluadas una a una, cada una en P{i}. */
export function evaluar(formulas: readonly string[]): Resultado[] {
  const e = motor();
  formulas.forEach((f, i) => e.setCell('Hoja1', `P${i + 1}`, f));
  return formulas.map((_, i) => {
    const v = e.getValue('Hoja1', `P${i + 1}`);
    return v instanceof FormulaError ? { error: v.code } : v;
  });
}

export function comparar(esperado: Esperado): Caso[] {
  const nuestros = evaluar(esperado.casos.map((c) => c.f));
  return esperado.casos.map((c, i) => {
    const nuestro = nuestros[i]!;
    const veredicto: Veredicto =
      sinContraste(c.lo) || usaFuncion365(c.f)
        ? 'sin-contraste'
        : iguales(nuestro, c.lo)
          ? 'igual'
          : 'difiere';
    return { f: c.f, lo: c.lo, nuestro, veredicto };
  });
}

const mostrar = (r: Resultado): string => (typeof r === 'object' ? r.error : JSON.stringify(r));

if (import.meta.main) {
  const esperado = (await Bun.file(
    new URL('./esperado-libreoffice.json', import.meta.url),
  ).json()) as Esperado;
  const casos = comparar(esperado);
  const cuenta = (v: Veredicto): number => casos.filter((c) => c.veredicto === v).length;
  const salida = (t: string): void => void process.stdout.write(`${t}\n`);
  salida(`${esperado.libreoffice} · ${casos.length} fórmulas`);
  salida(
    `igual ${cuenta('igual')} · difiere ${cuenta('difiere')} · sin contraste ${cuenta('sin-contraste')}`,
  );
  for (const c of casos.filter((x) => x.veredicto === 'difiere'))
    salida(`DIFIERE ${c.f}\n   nuestro ${mostrar(c.nuestro)}\n   LibreOffice ${mostrar(c.lo)}`);
}
