/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El cliente de la API de Bases desde el NAVEGADOR: habla con el proxy del BFF de esta
 * misma app (`/bases/api`), que cambia la cookie por el token por dentro. Solo se
 * crea en manejadores y efectos —necesita `window.location`—, nunca al renderizar.
 */
import { BasesClient } from '@sytue/api-clients/bases';
import { HttpError } from '@sytue/api-clients';
import { ruta } from './rutas.js';
import type { T } from '../i18n/index.js';

export function cliente(): BasesClient {
  return new BasesClient({ baseUrl: `${window.location.origin}${ruta('/api')}` });
}

/** El mensaje, traducido, de un fallo de la API. Nunca el texto crudo del servidor. */
export function mensajeDeError(e: unknown, t: T): string {
  if (e instanceof HttpError) {
    if (e.status === 401) return t('err.session');
    if (e.status === 403) return t('err.forbidden');
    if (e.status === 404) return t('err.notFound');
    if (e.code === 'rev_conflict') return t('err.conflict');
    if (e.code === 'not_a_member') return t('err.notMember');
    if (e.code === 'is_member') return t('err.isMember');
    if (e.code === 'referenced') return t('err.referenced');
    if (e.code === 'limit_reached') return t('err.limit');
    if (e.status === 422 || e.status === 400) return t('err.invalid');
  }
  return t('err.generic');
}
