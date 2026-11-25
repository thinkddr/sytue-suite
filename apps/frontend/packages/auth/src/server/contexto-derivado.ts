/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL CONTEXTO DE ASTRO CON OTROS `params`, SIN COPIARLO.
 *
 * Varias rutas reenvían la petición a otro manejador cambiando solo la `ruta`
 * (`{ ...ctx, params: { …, ruta } }`). Copiar el contexto con `...` LEE todas sus
 * propiedades, y varias son getters con efectos: `csp` en concreto, que en una app
 * sin `security.csp` escribe en el log «context.csp was used when rendering the route
 * …, but CSP was not configured» en CADA petición a esas rutas —el aviso que salía en
 * producción sin que ninguna línea nuestra tocara `csp`—. Y si la app sí la tiene,
 * cada copia construye un objeto de CSP que nadie usa.
 *
 * Aquí el derivado HEREDA del contexto (prototipo) y solo pone lo que cambia: los
 * getters se evalúan si alguien los pide, en el contexto original, y nunca antes.
 */
import type { APIContext } from 'astro';

/** `ctx` con `params` (y, si hace falta, `request`) sustituidos; el resto, heredado. */
export function contextoDerivado<C extends Pick<APIContext, 'params'>>(
  ctx: C,
  params: C['params'],
  extra: Partial<Pick<APIContext, 'request'>> = {},
): C {
  return Object.assign(Object.create(ctx) as C, { params, ...extra });
}
