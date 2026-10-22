/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La contraseña de un enlace público (`/s/{token}`), del lado del BFF.
 *
 * Quien acierta la contraseña recibe del servicio una PRUEBA; aquí se guarda en una
 * cookie httpOnly de esa dirección y se presenta en cada petición. La contraseña no se
 * guarda en ningún sitio y la decisión es siempre del servicio: esto solo transporta.
 */
import { createHash } from 'node:crypto';
import type { APIContext, AstroGlobal } from 'astro';
import { withBase } from '@sytue/ui';

/** Lo que dura la prueba en el navegador: una jornada. */
const VIDA_S = 12 * 60 * 60;

/** Cabecera con la que la prueba viaja al servicio en las lecturas sin sesión. */
export const CABECERA_PRUEBA = 'x-share-proof';

type Ctx = Pick<APIContext | AstroGlobal, 'cookies' | 'request'>;

/**
 * Nombre de la cookie de un enlace. Sale de un hash del token y no del token: el nombre
 * de una cookie acaba en registros y herramientas donde el token no debe estar.
 */
function cookieDe(token: string): string {
  return `sytue_docs_enlace_${createHash('sha256').update(token).digest('hex').slice(0, 16)}`;
}

/** La dirección de la vista pública de un enlace, que es también el ámbito de su cookie. */
export function rutaPublica(token: string, sufijo = ''): string {
  return withBase(import.meta.env.BASE_URL, `/s/${encodeURIComponent(token)}${sufijo}`);
}

export function leerPrueba(ctx: Ctx, token: string): string {
  return ctx.cookies.get(cookieDe(token))?.value ?? '';
}

export function guardarPrueba(ctx: Ctx, token: string, prueba: string): void {
  ctx.cookies.set(cookieDe(token), prueba, {
    httpOnly: true,
    secure: !import.meta.env.DEV,
    sameSite: 'lax',
    path: rutaPublica(token),
    maxAge: VIDA_S,
  });
}

/**
 * Las cabeceras hacia el servicio: la prueba, si la hay, y la cadena de proxies tal como
 * llegó, para que el servicio limite los intentos por la IP de quien llama y no por la
 * del BFF. El servicio solo se la cree a sus proxies de confianza.
 */
export function cabecerasDeEnlace(ctx: Ctx, token: string): Record<string, string> {
  const prueba = leerPrueba(ctx, token);
  const xff = ctx.request.headers.get('x-forwarded-for');
  return {
    ...(prueba ? { [CABECERA_PRUEBA]: prueba } : {}),
    ...(xff ? { 'x-forwarded-for': xff } : {}),
  };
}
