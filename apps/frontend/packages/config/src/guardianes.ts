/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LEER EL CÓDIGO DE LA SUITE EN UN TEST SIN QUE EL VERDE MIENTA.
 *
 * Los guardianes que miran el código fuente —la plomería copiada, el `role="toolbar"`
 * escrito a mano, el aro de foco, el HTML crudo— comparten un fallo que no se ve: si
 * su patrón no casa nada, no fallan, simplemente no miran. Con el Bun fijado (1.4.4),
 * unas llaves cuyas alternativas cruzan un `/` (`{apps/…,packages/office/…}`) no
 * casan NADA, y así pasaron en verde los de la plomería, los iconos y la barra sin
 * haber leído un fichero (2026-11-25). Las llaves dentro de un solo segmento
 * (`*.{ts,tsx}`, `{apps,packages}/…`) sí funcionan.
 *
 * Medido ese día, dejando a cero todo `Bun.Glob#scan`: de los dieciocho ficheros de
 * test que recorren el código, cuatro seguían en verde (el aro de foco, el color por
 * prefijo, el viewport y el HTML crudo de Pages). Los demás caían por otro lado —una lista de
 * excepciones que ya no se encontraba, un fichero que se esperaba ver—, pero eso es
 * suerte, no diseño. Por eso aquí el recuento es parte del contrato: quien recorre
 * dice cuánto tiene que salir como poco, y si sale menos, el test cae diciendo qué
 * patrón no mira (CONVENTIONS §13.4, «contar siempre también los que pasaron»).
 *
 * Solo para tests: usa `Bun`, que la compilación de la librería no conoce (va con
 * `types: ["node"]`), y por eso `tsconfig.lib.json` lo deja fuera, como a los tests.
 */
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

/** Un recorrido de `Bun.Glob` y lo mínimo que tiene que devolver. */
export interface Recorrido {
  /** Patrón relativo a `cwd`. Sin llaves que crucen un `/`: no casan nada. */
  glob: string;
  /** Desde dónde se resuelve: ruta absoluta o URL de fichero (`new URL('../', import.meta.url)`). */
  cwd: string | URL;
  /**
   * Cuántos ficheros tienen que salir, como poco, DESPUÉS de `filtro`. Va muy por debajo
   * de lo medido, para que borrar pantallas no lo rompa y un patrón roto sí.
   */
  minimo: number;
  /** Qué rutas (relativas a `cwd`) se leen. Sin él, todas. */
  filtro?: (rel: string) => boolean;
}

/**
 * Cada fichero que casa, con su texto, ordenado por ruta. LANZA si salen menos de
 * `minimo`: dentro de un `test` eso es un rojo que nombra el patrón, en vez del verde
 * de un guardián que no ha leído nada.
 */
export async function leerFuentes({
  glob,
  cwd,
  minimo,
  filtro,
}: Recorrido): Promise<[rel: string, texto: string][]> {
  const base = typeof cwd === 'string' ? cwd : fileURLToPath(cwd);
  const rutas: string[] = [];
  for await (const rel of new Bun.Glob(glob).scan({ cwd: base })) {
    if (!filtro || filtro(rel)) rutas.push(rel);
  }
  if (rutas.length < minimo) {
    throw new Error(
      `«${glob}» desde ${base} solo da ${rutas.length} ficheros y tendrían que ser ${minimo} como poco: ` +
        'el patrón no casa y el guardián no mira. Si lleva llaves que cruzan un «/», sepáralo en un glob por raíz.',
    );
  }
  // De uno en uno, como hacían los tests: tres mil lecturas a la vez agotan los
  // descriptores de un runner (EMFILE) antes que el tiempo.
  const leidas: [rel: string, texto: string][] = [];
  for (const rel of rutas.sort()) leidas.push([rel, await Bun.file(join(base, rel)).text()]);
  return leidas;
}
