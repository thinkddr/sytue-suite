/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — «Pedir acceso de edición» de algo compartido en lectura y la
 * respuesta de quien lo compartió (services/drive/internal/http/peticiones.go), con el
 * BFF común de las peticiones (`rutasDePeticiones`, @sytue/auth/server).
 */
import { rutasDePeticiones, sesionApi } from '@sytue/auth/server';
import { bff, driveBaseUrl } from '../../lib/auth';

export const prerender = false;

export const { GET, POST, DELETE } = rutasDePeticiones({
  sesion: sesionApi(bff),
  servicio: () => `${driveBaseUrl()}/api/v1`,
  base: (q) => {
    const id = q.get('id');
    return id ? `/shared/${encodeURIComponent(id)}/edit-requests` : null;
  },
});
