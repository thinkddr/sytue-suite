/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Mail — ENLACE CANÓNICO de un correo: GET /messages/{id}.
 *
 * Es la ruta que compone el buscador global de la suite para cada correo que
 * encuentra (services/hub/internal/connectors/mail.go), y NO existía: la app solo
 * tiene su vista principal, así que cada resultado de Mail llevaba a un 404. Es el
 * mismo fallo que tuvo Drive con /files/{id} y se resuelve igual.
 *
 * La vista abre CONVERSACIONES, no mensajes sueltos: aquí se resuelve el mensaje a su
 * hilo contra el servicio y se redirige a la vista con `?thread=…`, que la isla abre
 * al cargar. Va en el servidor porque el access token vive en la sesión httpOnly.
 *
 * Si el correo no es de quien lo pide o ya no existe, se manda a la bandeja SIN
 * distinguir cuál fue: un 404 propio delataría qué ids existen.
 */
import type { APIRoute } from 'astro';
import { returnToCanonico, rutaConOrg } from '@sytue/config';
import { bff, toBffContext } from '../../lib/bff.js';
import { mailGet } from '../../lib/mailrest.js';

export const prerender = false;

/** La bandeja, CON el prefijo de la app: `/` a secas es el lanzador de la suite. */
const HOME = import.meta.env.BASE_URL;

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/**
 * Resuelve a dónde mandar al visitante; la bandeja si el correo no lleva a ningún
 * sitio. Con org (la del path, ADR 0056, que el middleware dejó en `locals.org`)
 * el destino sale canónico: sin org también vale, pero el middleware la añadiría
 * con OTRO salto.
 */
async function destino(id: string, accessToken: string, org?: string): Promise<string> {
  if (!UUID.test(id)) return rutaConOrg(HOME, org, '/');
  try {
    const res = await mailGet(accessToken, `/api/v1/messages/${encodeURIComponent(id)}`);
    if (!res.ok) return rutaConOrg(HOME, org, '/');
    const m = (await res.json()) as { thread_id?: string };
    // Un mensaje sin hilo se abre como hilo de uno, con la misma clave que la lista.
    const hilo = m.thread_id && UUID.test(m.thread_id) ? m.thread_id : `msg:${id}`;
    return rutaConOrg(HOME, org, `/?${new URLSearchParams({ thread: hilo })}`);
  } catch {
    return rutaConOrg(HOME, org, '/');
  }
}

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) {
    // La petición puede llegar REESCRITA por dentro (entrada org-ful con la marca
    // `?__org=`, ADR 0056): el returnTo sale en la forma canónica, no en la
    // interna, o tras el login la barra se quedaría con la marca.
    const returnTo = returnToCanonico(ctx.url, bff.basePath);
    return ctx.redirect(`${bff.basePath}/auth/login?returnTo=${encodeURIComponent(returnTo)}`);
  }
  return ctx.redirect(
    await destino(
      ctx.params.id ?? '',
      session.tokens.accessToken,
      ctx.locals.org ?? session.user.tenant,
    ),
  );
};
