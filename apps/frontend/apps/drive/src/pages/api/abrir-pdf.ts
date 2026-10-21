/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/abrir-pdf?path=<ruta>  →  302 a Sytue PDF con ese fichero abierto.
 *
 * Existe para que la puerta la abra el SERVIDOR y no el navegador: la URL pública
 * de Sytue Docs sale de la configuración del despliegue, y hacérsela llegar al
 * cliente obligaría a propagarla por props hasta cada fila de la lista de
 * ficheros. Un redirect resuelve lo mismo con una línea y sin exponer nada.
 *
 * La verja es la sesión. La del FICHERO la aplica Sytue PDF al leerlo de Drive:
 * aquí solo se compone un enlace, y componerlo no da acceso a nada.
 *
 * LA URL LA COMPONE `appUrl` Y NO UNA VARIABLE DE ENTORNO. Escrita a mano le
 * faltaba el PREFIJO del producto —mandaba a `docs.sytue.eu/pdf` cuando la
 * página vive en `/docs/pdf`—, así que esta puerta llevaba abriendo un 404
 * desde que existe. No se parece a un enlace roto: el host resuelve, el ingress
 * enruta y lo que contesta es el 404 de Traefik, que se lee como «CertiPDF
 * está caído». `appUrl` conoce host Y prefijo de cada producto.
 */
import type { APIRoute } from 'astro';
import { appUrl, returnToCanonico } from '@sytue/config';
import { bff, toBffContext } from '../../lib/auth';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) {
    // El returnTo canónico (ADR 0056) y NO la petición cruda: `ctx.request.url`
    // es ABSOLUTA y `safeReturnTo` la tira a `/` tras el login — se perdía la
    // apertura. Con el helper sale la ruta de la app y, al volver con sesión,
    // este mismo endpoint termina el trabajo.
    const returnTo = returnToCanonico(ctx.url, bff.basePath);
    return ctx.redirect(`${bff.basePath}/auth/login?returnTo=${encodeURIComponent(returnTo)}`, 302);
  }
  const path = new URL(ctx.request.url).searchParams.get('path');
  if (!path || !path.startsWith('/')) {
    // Una ruta que no empieza por «/» no es de Drive, y concatenarla produciría
    // una URL a un sitio que no elegimos nosotros. Se vuelve al explorador CON su
    // prefijo: `/` a secas, en un host que comparten todas las apps, es el
    // lanzador de la suite y no Drive.
    return ctx.redirect(bff.basePath, 302);
  }
  // La org va al destino (ADR 0056): `/certipdf/pdf` sin org hace que el visor
  // confunda la RUTA `pdf` con la organización y componga `/certipdf/pdf/pdf?path=…`
  // (404). Este endpoint vive bajo `/api/`, que ninguna app escopia, así que
  // `locals.org` no llega por el path: manda la org de la sesión.
  return ctx.redirect(
    appUrl('pdf', `pdf?path=${encodeURIComponent(path)}`, ctx.locals.org ?? session.user.tenant),
    302,
  );
};
