/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las rutas de Sytue Web, con y sin organización (ADR 0056).
 *
 * `middlewareDeOrganizacion` lee como organización CUALQUIER primer segmento con
 * forma de slug, así que cada página de Pages tiene que declararse «sin org»: si no,
 * `/web/edit/<id>` se leería como «la organización edit».
 */
import { orgActual, rutaConOrg } from '@sytue/config';
import { withBase } from '@sytue/ui';

export const PAGINAS_SIN_ORG = ['new', 'edit', 'site', 'settings'] as const;

const BASE = import.meta.env.BASE_URL ?? '/web';

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

/** Ruta pública: nunca lleva organización. */
export function rutaPublica(camino: string): string {
  return withBase(BASE, camino.startsWith('/') ? camino : `/${camino}`);
}
