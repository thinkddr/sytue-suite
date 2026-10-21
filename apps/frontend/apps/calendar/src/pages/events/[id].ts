/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Calendar — ENLACE CANÓNICO de un evento: GET /events/{id}.
 *
 * Lo componen los resultados de Calendar en el buscador global de la suite y los
 * intents del Hub (services/hub/internal/connectors/calendar.go, intents/intents.go),
 * y NO existía: cada enlace llevaba a un 404. Mismo fallo y mismo arreglo que
 * /files/{id} en Drive.
 *
 * La vista navega por DÍAS, no por evento: aquí se resuelve el evento contra el
 * servicio y se redirige a la vista con `?date=<dtstart>` (y `all_day=1`), que la isla
 * abre en ese día (fechaDelEnlace). Va en el servidor porque el access token vive en la
 * sesión httpOnly.
 *
 * Si el evento no es de quien lo pide o ya no existe, se manda a la vista SIN distinguir
 * cuál fue: un 404 propio delataría qué ids existen.
 */
import type { APIRoute } from 'astro';
import { returnToCanonico, rutaConOrg } from '@sytue/config';
import { bff, toBffContext } from '../../lib/bff.js';
import { CALENDAR_BASE_URL } from '../../lib/caldav.js';

export const prerender = false;

/** La vista, CON el prefijo de la app: `/` a secas es el lanzador de la suite. */
const HOME = import.meta.env.BASE_URL;

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/**
 * Resuelve a dónde mandar al visitante; la vista si el evento no lleva a ningún
 * sitio. Con org (la del path, ADR 0056, que el middleware dejó en `locals.org`)
 * el destino sale canónico: sin org también vale, pero el middleware la añadiría
 * con OTRO salto.
 */
async function destino(id: string, accessToken: string, org?: string): Promise<string> {
  if (!UUID.test(id)) return rutaConOrg(HOME, org, '/');
  try {
    const res = await fetch(`${CALENDAR_BASE_URL}/api/v1/events/${id}`, {
      headers: { authorization: `Bearer ${accessToken}`, accept: 'application/json' },
    });
    if (!res.ok) return rutaConOrg(HOME, org, '/');
    const e = (await res.json()) as { dtstart?: string; all_day?: boolean };
    if (!e.dtstart) return rutaConOrg(HOME, org, '/');
    const qs = new URLSearchParams({ date: e.dtstart });
    if (e.all_day) qs.set('all_day', '1');
    return rutaConOrg(HOME, org, `/?${qs}`);
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
