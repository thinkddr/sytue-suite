/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/groups — los grupos de Sytue ID a los que pertenece quien pregunta,
 * para el selector de «Compartir con un grupo». Ya viajan en la sesión (claim
 * `groups` del token, ver @sytue/auth: `SytueUser.groups`) — son los MISMOS
 * nombres legibles que Drive matchea contra sus comparticiones `kind=group`, así
 * que no hace falta pedirle nada a Sytue ID.
 *
 * Solo los propios: compartir con un grupo al que no se pertenece no es un caso
 * de uso ni lo pide el servicio (`subject` es el nombre del grupo, no requiere
 * membresía para crear el share, pero ofrecer una lista de TODOS los grupos de
 * la organización exigiría un endpoint admin que este BFF no tiene motivo para
 * llamar en nombre de un usuario cualquiera).
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  // El pseudo-grupo de tenancy (`org/<slug>` o `/org/<slug>`) no es un grupo con
  // el que se pueda compartir: es cómo se deriva a qué organización pertenece la
  // sesión (ver `userFromClaims` en @sytue/auth).
  const groups = session.user.groups.filter((g) => !/^\/?org\//.test(g));
  return json({ groups });
};
