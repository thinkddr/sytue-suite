/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * /notebooks/{id} — la dirección CANÓNICA de un cuaderno.
 *
 * Es la URL que componen la búsqueda federada del Hub y los enlaces que se
 * comparten en Rooms, así que tiene que existir aunque la aplicación sea una
 * sola pantalla. Redirige a la raíz con el cuaderno en la query, que es lo que
 * la isla sabe leer: una única superficie que montar y un único sitio donde
 * arreglar lo que falle.
 *
 * Es un endpoint y no una página `.astro`: su única respuesta es el 302. Una
 * página sin cuerpo hace que Astro descarte el frontmatter al generar las
 * comprobaciones, con lo que no representa la respuesta que sirve en runtime.
 */
import type { APIRoute } from 'astro';
import { rutaConOrg } from '@sytue/config';

export const prerender = false;

export const GET: APIRoute = (ctx) => {
  const nota = ctx.url.searchParams.get('note');
  const q = new URLSearchParams({ notebook: ctx.params.id ?? '' });
  if (nota) q.set('note', nota);

  // Con org en el path, el middleware la deja en `locals.org`; sin ella, el
  // middleware canónico hará el único salto adicional tras el login.
  return ctx.redirect(
    rutaConOrg(import.meta.env.BASE_URL, ctx.locals.org, `/?${q.toString()}`),
    302,
  );
};
