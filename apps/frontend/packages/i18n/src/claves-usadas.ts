/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NINGUNA CLAVE DEL CATÁLOGO SIN USAR: la mecánica común a las apps que lo vigilan
 * (www desde el 2026-09-26, la consola desde el 2026-09-28).
 *
 * Una clave que ninguna página pide se sigue traduciendo 26 veces y nadie la ve. Se
 * compara el catálogo inglés —la fuente de verdad; los otros 25 no pueden tener claves
 * de más porque su tipo sale de él— con lo que el código pide de verdad:
 *
 *   - Una clave LITERAL cuenta como usada si aparece entre comillas en el código de la
 *     app (no en los catálogos ni en los tests).
 *   - Una clave DINÁMICA (`t(\`product.${key}.tagline\`)`) no aparece nunca escrita, así
 *     que la app la lista con las claves EXACTAS que produce (`dinamicas`), y
 *     `plantillasDinamicas` saca del código las que hay para que el test compare las dos.
 *
 * Solo funciones: cada app pone sus `expect` en su propio test.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

const EXCLUIDOS = new Set(['node_modules', 'dist', '.astro', 'messages']);

function fuentes(dir: string): string[] {
  return readdirSync(dir).flatMap((nombre) => {
    if (EXCLUIDOS.has(nombre)) return [];
    const ruta = join(dir, nombre);
    if (statSync(ruta).isDirectory()) return fuentes(ruta);
    return /\.(astro|tsx?|mjs)$/.test(nombre) && !/\.test\.tsx?$/.test(nombre) ? [ruta] : [];
  });
}

/** Todo el código de la app bajo `raiz`, sin catálogos ni tests, en una cadena. */
export function codigoDe(raiz: string): string {
  return fuentes(raiz)
    .map((f) => readFileSync(f, 'utf8'))
    .join('\n');
}

/** Cada `t(\`…${…}…\`)` del código, tal cual está escrito, sin repetir y ordenado. */
export function plantillasDinamicas(codigo: string): string[] {
  const vistas = [...codigo.matchAll(/\bt\(\s*`([^`]*\$\{[^`]*)`/g)].map((m) => m[1] ?? '');
  return [...new Set(vistas)].sort((a, b) => a.localeCompare(b));
}

/** Claves de `claves` que el código no pide ni literales ni por una plantilla listada. */
export function clavesSinUsar(
  claves: readonly string[],
  codigo: string,
  dinamicas: Record<string, readonly string[]>,
): string[] {
  const producidas = new Set(Object.values(dinamicas).flat());
  return claves.filter(
    (k) => !producidas.has(k) && !codigo.includes(`'${k}'`) && !codigo.includes(`"${k}"`),
  );
}
