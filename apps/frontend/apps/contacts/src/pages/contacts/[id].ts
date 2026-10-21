/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Contacts — ENLACE CANÓNICO de un contacto: GET /contacts/{id}.
 *
 * Lo componen los resultados de Contacts en el buscador global de la suite y los
 * intents del Hub (services/hub/internal/connectors/contacts.go, intents/intents.go),
 * y NO existía: cada enlace llevaba a un 404. Mismo fallo y mismo arreglo que
 * /files/{id} en Drive.
 *
 * La vista trabaja con CardDAV —libretas por URL, contactos por UID—, no con los ids de
 * la API: aquí se resuelve el contacto a su libreta (el slug, como resolveAddressBookId)
 * y a su UID, y se redirige a la vista con `?book=…&uid=…`, que la isla abre al cargar.
 * Va en el servidor porque el access token vive en la sesión httpOnly.
 *
 * Si el contacto no es de quien lo pide o ya no existe, se manda a la vista SIN
 * distinguir cuál fue: un 404 propio delataría qué ids existen.
 */
import type { APIRoute } from 'astro';
import { returnToCanonico, rutaConOrg } from '@sytue/config';
import { bff, toBffContext } from '../../lib/bff.js';
import { restJson } from '../../lib/rest.js';

export const prerender = false;

/** La vista, CON el prefijo de la app: `/` a secas es el lanzador de la suite. */
const HOME = import.meta.env.BASE_URL;

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/**
 * Resuelve a dónde mandar al visitante; la vista si el contacto no lleva a ningún
 * sitio. Con org (la del path, ADR 0056, que el middleware dejó en `locals.org`)
 * el destino sale canónico: sin org también vale, pero el middleware la añadiría
 * con OTRO salto.
 */
async function destino(id: string, accessToken: string, org?: string): Promise<string> {
  if (!UUID.test(id)) return rutaConOrg(HOME, org, '/');
  try {
    const contacto = await restJson<{ uid?: string; address_book_id?: string }>(
      accessToken,
      'GET',
      `/api/v1/contacts/${id}`,
    );
    const { address_books: libretas } = await restJson<{
      address_books: Array<{ id: string; name: string }>;
    }>(accessToken, 'GET', '/api/v1/address-books');
    const libreta = libretas.find((b) => b.id === contacto.address_book_id);
    if (!libreta || !contacto.uid) return rutaConOrg(HOME, org, '/');
    return rutaConOrg(
      HOME,
      org,
      `/?${new URLSearchParams({ book: libreta.name, uid: contacto.uid })}`,
    );
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
