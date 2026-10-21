/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /abrir?file=<uuid de Drive>&volver=<carpeta>  →  la foto, abierta aquí.
 *
 * Es la puerta por la que Drive manda una imagen cuando alguien la pulsa en su
 * carpeta: el visor de Drive la enseñaba plana y sin nada que hacer con ella,
 * teniendo esta app al lado.
 *
 * NO COPIA NADA. Indexar es dar de alta en la galería el fichero que ya está en
 * Drive —el original se queda donde estaba, con su ACL y sus versiones—, y es
 * IDEMPOTENTE: abrir dos veces la misma foto lleva a la misma, no crea otra.
 *
 * LLEGA EL ID Y NO LA RUTA, igual que en `open-from-drive` de Docs: una ruta la
 * elige quien pulsa el enlace y habría que volver a comprobar de quién es; el id
 * se resuelve con el token de la sesión, así que si esa persona no puede ver el
 * fichero, el servicio contesta y aquí no hay nada que abrir.
 */
import type { APIRoute } from 'astro';
import { serverEnv, vieneDeOtroSitio } from '@sytue/auth/server';
import { returnToCanonico, rutaConOrg, volverA } from '@sytue/config';
import { bff, toBffContext } from '../lib/bff.js';

export const prerender = false;

const env = serverEnv(import.meta.env);
const destino = (env.SYTUE_PHOTOS_API_URL ?? 'http://syphotos:8097').replace(/\/$/, '');

export const GET: APIRoute = async (ctx) => {
  const url = new URL(ctx.request.url);
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) {
    // La petición puede llegar REESCRITA por dentro (entrada org-ful con la marca
    // `?__org=`, ADR 0056): el returnTo sale en la forma canónica, no en la
    // interna, o tras el login la barra se quedaría con la marca.
    const returnTo = returnToCanonico(url, bff.basePath);
    return ctx.redirect(`${bff.basePath}/auth/login?returnTo=${encodeURIComponent(returnTo)}`, 302);
  }

  /** Ruta dentro de esta app, con su prefijo Y la organización (ADR 0056).
   *
   * Prefija a secas era tirar la org incluso llegando canónico
   * (`/photos/<org>/abrir?file=…` la trae en el path y el middleware la deja en
   * `locals.org`): los destinos volvían a `/photos/edit?id=…` y, como `edit` es
   * ruta en `sinOrg`, ahí se quedaban para siempre — el visor componía la vuelta
   * a Drive con `orgActual('/photos')` = `edit` y caía en la carpeta equivocada.
   * Este endpoint vive bajo una ruta en `sinOrg`, así que si llega sin org el
   * path no la trae: manda la de la sesión. */
  const r = (p: string) =>
    rutaConOrg(import.meta.env.BASE_URL, ctx.locals.org ?? session.user.tenant, p);

  const file = (url.searchParams.get('file') ?? '').trim();
  // La vuelta solo puede ser una RUTA de Drive; `volverA` es quien lo decide y
  // por qué (packages/config: sería un redirector abierto).
  const vuelta = volverA(url.searchParams.get('volver'));
  const cola = vuelta ? `&volver=${encodeURIComponent(vuelta)}` : '';

  // Abrir DA DE ALTA la foto (FE-02): desde otra web no, aunque la cookie viaje.
  if (!file || vieneDeOtroSitio(ctx.request)) return ctx.redirect(r('/'), 302);

  const res = await fetch(`${destino}/api/v1/assets`, {
    method: 'POST',
    headers: {
      authorization: `Bearer ${session.tokens.accessToken}`,
      'content-type': 'application/json',
    },
    body: JSON.stringify({ driveFileId: file }),
  });
  // A la galería y no a un error: quien llega aquí venía de pulsar una foto en
  // Drive, y lo peor que le puede pasar es quedarse en una pantalla en blanco.
  if (!res.ok) return ctx.redirect(r('/'), 302);

  const foto = (await res.json()) as { id?: string };
  if (!foto.id) return ctx.redirect(r('/'), 302);

  return ctx.redirect(r(`/edit?id=${encodeURIComponent(foto.id)}${cola}`), 302);
};
