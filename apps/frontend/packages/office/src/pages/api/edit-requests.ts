/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — «Pedir acceso de edición» de un documento y la respuesta del
 * dueño (services/docs/internal/http/peticiones.go), con el BFF común de las
 * peticiones (`rutasDePeticiones`, @sytue/auth/server).
 */
import { rutasDePeticiones, sesionApi } from '@sytue/auth/server';
import { bff, docsBaseUrl } from '../../lib/auth';

export const prerender = false;

export const { GET, POST, DELETE } = rutasDePeticiones({
  sesion: sesionApi(bff),
  servicio: () => `${docsBaseUrl()}/api/v1`,
  base: (q) => {
    const id = q.get('id');
    return id ? `/documents/${encodeURIComponent(id)}/edit-requests` : null;
  },
});
