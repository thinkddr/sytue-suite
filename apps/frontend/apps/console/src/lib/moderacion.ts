/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Qué reenvía el BFF de la cola de denuncias del operador
 * (`pages/api/platform-moderation/[...path].ts`), y a qué servicio. Aparte para
 * poder probarlo sin levantar Astro.
 */

const UUID = '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}';

/** Ancladas, con su método: nada que no sea exactamente esto llega al servicio. */
const RUTAS: { re: RegExp; metodo: string; servicio: 'cast' | 'rooms' }[] = [
  { re: /^cast\/reports$/, metodo: 'GET', servicio: 'cast' },
  { re: new RegExp(`^cast/reports/${UUID}$`), metodo: 'POST', servicio: 'cast' },
  { re: /^rooms\/reports$/, metodo: 'GET', servicio: 'rooms' },
  { re: new RegExp(`^rooms/reports/${UUID}/resolve$`), metodo: 'POST', servicio: 'rooms' },
];

/** Lo único que se reenvía de la query. */
export const CONSULTA_DE_MODERACION = ['status', 'cursor', 'limit', 'all'];

/**
 * El servicio y la ruta bajo `/api/v1/platform/moderation/` a la que va una
 * petición del navegador, o `null` si no se reenvía.
 */
export function destinoDeModeracion(
  metodo: string,
  path: string,
): { servicio: 'cast' | 'rooms'; resto: string } | null {
  const ruta = RUTAS.find((x) => x.metodo === metodo && x.re.test(path));
  return ruta ? { servicio: ruta.servicio, resto: path.slice(ruta.servicio.length + 1) } : null;
}
