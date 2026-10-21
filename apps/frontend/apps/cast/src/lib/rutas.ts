/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las rutas de Sytue Cast, con y sin organización (ADR 0056).
 *
 * `middlewareDeOrganizacion` lee como organización CUALQUIER primer segmento con
 * forma de slug, así que cada página de Cast tiene que declararse «sin org»: si no,
 * `/cast/watch/<id>` se leería como «la organización watch». La lista vive aquí y
 * la usan el middleware y los enlaces, para que no puedan discrepar.
 *
 * Qué queda con organización: la portada de trabajo (`/cast/<org>/`). El resto
 * acepta la forma con org (`/cast/<org>/upload`, que el middleware reescribe) y la
 * forma sin ella. Los enlaces que se COMPARTEN (ver, incrustar, canal) salen siempre
 * sin org: quien los recibe puede ser de otra organización o de ninguna.
 */
import { orgActual, rutaConOrg } from '@sytue/config';
import { withBase } from '@sytue/ui';

export const PAGINAS_SIN_ORG = [
  'explore',
  'c',
  'watch',
  'embed',
  'oembed',
  'media',
  'upload',
  'studio',
  'manage',
  'moderation',
] as const;

const BASE = import.meta.env.BASE_URL;

/**
 * La organización de la URL del navegador, o null. `orgActual` toma el primer
 * segmento con forma de slug; aquí se descartan los nombres de página.
 */
export function orgDeLaUrl(): string | null {
  const org = orgActual(BASE);
  return org && !(PAGINAS_SIN_ORG as readonly string[]).includes(org) ? org : null;
}

/** Ruta interna de trabajo, con la organización de la URL si la hay. */
export function ruta(camino: string): string {
  return withBase(BASE, rutaConOrg('', orgDeLaUrl(), camino));
}

/** Ruta pública y compartible: nunca lleva organización. */
export function rutaPublica(camino: string): string {
  return withBase(BASE, camino.startsWith('/') ? camino : `/${camino}`);
}

/**
 * Un fichero de `/media/…` del servicio visto desde el navegador: por el proxy de
 * esta app (`/cast/media/…`), que funciona en `cast.<dominio>` y en
 * `<org>.<dominio>`, donde `/media` en la raíz no existe.
 */
export function urlDeMedia(relativa: string | null | undefined): string | null {
  if (!relativa) return null;
  if (/^https?:\/\//.test(relativa)) return relativa;
  return rutaPublica(relativa.startsWith('/') ? relativa : `/${relativa}`);
}
