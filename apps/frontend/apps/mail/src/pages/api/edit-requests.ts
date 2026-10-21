/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Mail — «Pedir acceso» a un buzón compartido en `read` y la respuesta de
 * quien lo gestiona (services/mail/internal/jmap/peticiones.go), con el BFF común de
 * las peticiones (`rutasDePeticiones`, @sytue/auth/server).
 */
import { rutasDePeticiones } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import { mailOrigin } from '../../lib/mailrest.js';

export const prerender = false;

export const { GET, POST, DELETE } = rutasDePeticiones({
  sesion: getApiSession,
  servicio: () => `${mailOrigin()}/api/v1`,
  base: (q) => {
    const id = q.get('account');
    return id ? `/shared-mailboxes/${encodeURIComponent(id)}/edit-requests` : null;
  },
});
