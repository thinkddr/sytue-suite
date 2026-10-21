/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Notepads — «Pedir acceso de edición» de un cuaderno ajeno que se lee y la
 * respuesta de su dueño (services/notes/internal/http/peticiones.go), con el BFF común de
 * las peticiones (`rutasDePeticiones`, @sytue/auth/server).
 */
import { rutasDePeticiones } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import { notesApiUrl } from '../../lib/notes.js';

export const prerender = false;

export const { GET, POST, DELETE } = rutasDePeticiones({
  sesion: getApiSession,
  servicio: () => `${notesApiUrl()}/api/v1`,
  base: (q) => {
    const id = q.get('id');
    return id ? `/notebooks/${encodeURIComponent(id)}/edit-requests` : null;
  },
});
