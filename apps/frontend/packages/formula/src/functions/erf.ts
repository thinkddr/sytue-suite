/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * erf y erfc a doble precisión (error ≲ 1e-15), compartidas por ERF/ERFC (engineering.ts)
 * y por toda la familia normal (stats.ts). Antes cada fichero tenía su aproximación
 * (Abramowitz–Stegun y Numerical Recipes, ≲ 1e-7) y NORM.DIST, GAUSS o ERF diferían de
 * LibreOffice en la octava cifra (docs/architecture.md#calcus-compatibilidad).
 *
 *  · |x| < 3: la serie erf(x) = (2/√π)·e^(−x²)·Σ 2ⁿx²ⁿ⁺¹ / (2n+1)!!, de términos positivos
 *    (sin la cancelación de la serie de Taylor alternada).
 *  · |x| ≥ 3: erfc por la fracción continua e^(−x²)/√π · 1/(x + ½/(x + 1/(x + 3/2/(x + …)))).
 */

const DOS_RAIZ_PI = 1.1283791670955126; // 2/√π
const RAIZ_PI = 1.7724538509055159;

/** erf(x). */
export function erf(x: number): number {
  const z = Math.abs(x);
  if (z >= 3) {
    const r = 1 - erfcPositivo(z);
    return x < 0 ? -r : r;
  }
  let term = z;
  let sum = z;
  for (let n = 0; n < 200; n++) {
    term *= (2 * z * z) / (2 * n + 3);
    sum += term;
    if (term < sum * 1e-17) break;
  }
  const r = DOS_RAIZ_PI * Math.exp(-z * z) * sum;
  return x < 0 ? -r : r;
}

/** erfc(x) = 1 − erf(x), sin perder cifras cuando es pequeña (x grande). */
export function erfc(x: number): number {
  if (x < 0) return 2 - erfc(-x);
  return x >= 3 ? erfcPositivo(x) : 1 - erf(x);
}

/** erfc(x) para x ≥ 3 por la fracción continua, evaluada de atrás hacia delante. */
function erfcPositivo(x: number): number {
  if (x > 27) return 0; // e^(−x²) ya es 0 en doble precisión
  let f = x;
  for (let k = 80; k >= 1; k--) f = x + k / 2 / f;
  return Math.exp(-x * x) / (RAIZ_PI * f);
}
