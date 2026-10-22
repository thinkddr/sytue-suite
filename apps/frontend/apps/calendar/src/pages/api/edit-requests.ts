/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Calendar — «Pedir acceso de edición» de un calendario compartido y la
 * respuesta del dueño (services/calendar/internal/http/peticiones.go), con el BFF común
 * de las peticiones (`rutasDePeticiones`, @sytue/auth/server).
 */
import { rutasDePeticiones } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import { CALENDAR_BASE_URL } from '../../lib/caldav.js';

export const prerender = false;

export const { GET, POST, DELETE } = rutasDePeticiones({
  sesion: getApiSession,
  servicio: () => `${CALENDAR_BASE_URL}/api/v1`,
  base: (q) => {
    const id = q.get('calendar');
    return id ? `/calendars/${encodeURIComponent(id)}/edit-requests` : null;
  },
});
