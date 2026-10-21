/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Drive — ENLACE CANÓNICO de un fichero: GET /files/{id}.
 *
 * Esta ruta NO existía y TRES sitios la componían, así que todos daban 404:
 *   · los resultados de Drive en la búsqueda global (services/hub/…/connectors/drive.go)
 *   · la actividad reciente del launcher (el mismo conector)
 *   · «Compartir a Rooms» del menú de un fichero (FileBrowser), que publicaba en un canal
 *     un enlace que no llevaba a ninguna parte
 *
 * El explorador navega por RUTA, no por id, así que aquí se resuelve id → ruta contra el
 * servicio y se redirige a la vista con el fichero ya abierto (`?path=…&preview=…`). Va en
 * el servidor porque el access token vive en la sesión httpOnly.
 *
 * Es un endpoint y no una página porque lo único que hace es redirigir: no hay nada que
 * pintar, y una página con frontmatter vacío confunde a `astro check`.
 *
 * La autorización es la del servicio: si quien pide no puede ver el fichero responde 403
 * o 404, y aquí se manda a la raíz SIN distinguir cuál fue —un 404 propio delataría qué
 * ids existen—.
 */
import type { APIRoute } from 'astro';
import { returnToCanonico, rutaConOrg } from '@sytue/config';
import { bff, toBffContext, driveBaseUrl } from '../../lib/auth';

export const prerender = false;

/**
 * Raíz del explorador: el destino cuando el id no lleva a ningún sitio visible.
 *
 * CON EL PREFIJO DE LA APP, y ahí estaba la avería: era `'/'` a secas. Toda la suite
 * comparte host —`corporate.sytue.eu/drive`, `/mail`, `/docs`…— así que `/` no es el
 * explorador, es el LANZADOR. Cada reciente de Drive del Hub llevaba a la portada de
 * Sytue One con `?path=…&preview=…` colgando de la URL, donde no los lee nadie, y el
 * efecto que se leía era «los archivos recientes no se abren». Medido el 2026-09-13:
 * `/drive/files/<id>` acababa en `corporate.sytue.eu/?path=%2FFotos&preview=…` con el
 * título «Sytue One — Inicio».
 */
const HOME = import.meta.env.BASE_URL;

/**
 * Resuelve a dónde mandar al visitante; HOME si el fichero no es suyo o no existe.
 *
 * Con org —la del path (ADR 0056), que el middleware dejó en `locals.org`— el
 * destino sale canónico y se ahorra un salto: sin org también vale, pero el
 * middleware de la propia app volvería a añadirla con OTRO 302. Es el caso de los
 * enlaces canónicos (`driveFileUrl`) que llegan sin org y la ganan al entrar, y
 * de cualquiera que escriba la URL con el segmento.
 */
async function destino(id: string, accessToken: string, org?: string): Promise<string> {
  try {
    const res = await fetch(`${driveBaseUrl()}/api/v1/files/${encodeURIComponent(id)}`, {
      headers: { authorization: `Bearer ${accessToken}`, accept: 'application/json' },
    });
    if (!res.ok) return rutaConOrg(HOME, org, '/');
    const f = (await res.json()) as { path?: string };
    if (!f.path) return rutaConOrg(HOME, org, '/');
    const parent = f.path.slice(0, f.path.lastIndexOf('/')) || '/';
    const qs = new URLSearchParams();
    if (parent !== '/') qs.set('path', parent);
    qs.set('preview', f.path);
    return rutaConOrg(HOME, org, `/?${qs}`);
  } catch {
    // Drive no responde: a la raíz. Un fallo aquí no debe dejar una pantalla rota cuando
    // el explorador entero sigue funcionando.
    return rutaConOrg(HOME, org, '/');
  }
}

export const GET: APIRoute = async (ctx) => {
  const id = ctx.params.id ?? '';
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) {
    // La petición puede llegar REESCRITA por dentro (entrada org-ful con la marca
    // `?__org=`, ADR 0056): el returnTo sale en la forma canónica, no en la
    // interna, o tras el login la barra se quedaría con la marca.
    const returnTo = returnToCanonico(ctx.url, bff.basePath);
    return ctx.redirect(`${bff.basePath}/auth/login?returnTo=${encodeURIComponent(returnTo)}`);
  }
  return ctx.redirect(
    id
      ? await destino(id, session.tokens.accessToken, ctx.locals.org ?? session.user.tenant)
      : rutaConOrg(HOME, ctx.locals.org ?? session.user.tenant, '/'),
  );
};
