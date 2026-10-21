/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que repiten las páginas SSR de Cast: la sesión (si la hay), el Hub y el
 * redirect al login con el returnTo canónico (ADR 0056).
 */
import type { AstroGlobal } from 'astro';
import { returnToCanonico } from '@sytue/config';
import { serverEnv } from '@sytue/auth/server';
import { bff, toBffContext } from './bff.js';
import type { TipoDeFallo } from './fallo.js';

export const hubBaseUrl = (): string =>
  serverEnv(import.meta.env).SYTUE_HUB_BASE_URL ?? 'https://hub.sytue.eu';

export const sesionDe = (astro: AstroGlobal) => {
  // La sesión también decide la cabecera de páginas públicas. Ni la cabecera
  // anónima ni los datos de una cuenta deben reutilizarse para otra petición.
  astro.response.headers.set('cache-control', 'private, no-store');
  return bff.getSession(toBffContext(astro));
};

/** El redirect al login, sin caché, con el returnTo en la forma canónica. */
export function alLogin(astro: AstroGlobal): Response {
  const returnTo = returnToCanonico(astro.url, bff.basePath);
  const r = astro.redirect(`${bff.basePath}/auth/login?returnTo=${encodeURIComponent(returnTo)}`);
  r.headers.set('cache-control', 'no-store');
  return r;
}

/** El estado de una página a partir del código HTTP del servicio. */
export function falloDeStatus(status: number): TipoDeFallo {
  return status === 403 || status === 404 ? 'sinPermiso' : 'error';
}

/** Un identificador de vídeo con forma de UUID: lo demás es un 404 sin preguntar. */
export const RE_UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
