/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de SyERP — «Pedir acceso de edición» de un módulo en lectura y la respuesta de la
 * administración (services/syerp/internal/http/peticiones.go), con el BFF común de las
 * peticiones (`rutasDePeticiones`, @sytue/auth/server). Va aparte del proxy genérico
 * porque la franja habla el contrato común (`&grant=`, `&sub=`), no rutas del servicio.
 */
import { rutasDePeticiones } from '@sytue/auth/server';
import { apiUrl, getApiSession } from '../../lib/bff.js';

export const prerender = false;

export const { GET, POST, DELETE } = rutasDePeticiones({
  sesion: getApiSession,
  servicio: () => `${apiUrl()}/api/v1`,
  base: (q) => {
    const m = q.get('modulo');
    return m && /^[a-z_]+$/.test(m) ? `/modulos/${m}/edit-requests` : null;
  },
});
