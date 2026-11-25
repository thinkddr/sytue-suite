/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * Lo que Drive admite en una petición de `/batch/zip` (su `maxBatchItems`). Más
 * allá contesta 400 y no bajaría nada: mejor recortar y decir cuántas quedan fuera.
 */
export const TOPE_ZIP = 1000;

export interface PlanDeZip {
  /** IDs de Drive de las fotos que entran en el ZIP, en el orden de selección. */
  ficheros: string[];
  /** Fotos marcadas que no entran: pasan del tope o no están cargadas en la lista. */
  fuera: number;
}

/**
 * Traduce la selección (ids de Fotos) a los ficheros de Drive que la respaldan. El
 * ZIP lo arma Drive, que solo conoce SUS ids; una foto sin original conocido no se
 * puede pedir y se cuenta fuera en vez de romper la petición entera.
 */
export function planDeZip(
  seleccion: readonly string[],
  fotos: readonly { id: string; driveFileId: string }[],
): PlanDeZip {
  const porId = new Map(fotos.map((f) => [f.id, f.driveFileId.trim()]));
  const vistos = new Set<string>();
  const ficheros: string[] = [];
  let validas = 0;
  for (const id of seleccion) {
    const drive = porId.get(id);
    if (!drive || vistos.has(drive)) continue;
    vistos.add(drive);
    validas++;
    if (ficheros.length < TOPE_ZIP) ficheros.push(drive);
  }
  return { ficheros, fuera: seleccion.length - validas + (validas - ficheros.length) };
}

/**
 * El nombre del ZIP de un álbum entero: su título, sin lo que un sistema de ficheros
 * no acepta en un nombre (`/`, `\`, `:`…), y el respaldo («Fotos») si se queda en nada.
 */
export function prefijoDeAlbum(titulo: string, respaldo: string): string {
  const limpio = titulo
    // eslint-disable-next-line no-control-regex -- los de control tampoco valen en un nombre
    .replace(/[\\/:*?"<>|\u0000-\u001f]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 80);
  return limpio || respaldo;
}

/** Nombre del ZIP sin extensión (Drive añade «.zip»): «Fotos 2026-11-10». */
export function nombreDeZip(prefijo: string, hoy: Date): string {
  const d = (n: number) => String(n).padStart(2, '0');
  return `${prefijo} ${hoy.getFullYear()}-${d(hoy.getMonth() + 1)}-${d(hoy.getDate())}`;
}

/** Consulta para la ruta `api/zip` del BFF, que reenvía el ZIP de Drive en streaming. */
export function consultaDeZip(ficheros: readonly string[], nombre: string): string {
  return new URLSearchParams({ file_ids: ficheros.join(','), name: nombre }).toString();
}
