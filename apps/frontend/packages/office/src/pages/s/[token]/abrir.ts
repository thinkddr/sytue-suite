/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /s/{token}/abrir — «Abrir en el editor» de la vista pública (ADR 0052).
 *
 * Canjea el enlace de editor o de comentarista por un permiso de quien tiene la
 * sesión —también si es de otra organización— y lleva al editor. Sin sesión, al
 * login, y de vuelta a la vista pública: el canje lo decide un clic, no una URL.
 *
 * POST, no GET: canjear escribe un permiso, y un enlace o una imagen de otro sitio
 * no deben poder dejar a nadie con acceso a un documento que no ha elegido abrir.
 * El origen del POST lo comprueba Astro (`security.checkOrigin`, activo en la
 * suite: `@sytue/config/astro-security`), y la cookie de sesión no viaja en un POST
 * de otro sitio.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext, docsBaseUrl } from '../../../lib/auth';
import { leerPrueba } from '../../../lib/enlaceClave';
import { r } from '../../../lib/rutas.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const token = ctx.params.token ?? '';
  const vista = r(`/s/${encodeURIComponent(token)}`);

  const session = await bff.getSession(toBffContext(ctx));
  if (!session) {
    return ctx.redirect(`${bff.basePath}/auth/login?returnTo=${encodeURIComponent(vista)}`, 303);
  }
  let res: Response;
  try {
    // El token en el cuerpo, no en la ruta: la ruta va al log de peticiones.
    res = await fetch(`${docsBaseUrl().replace(/\/$/, '')}/api/v1/share/redeem`, {
      method: 'POST',
      headers: {
        authorization: `Bearer ${session.tokens.accessToken}`,
        'content-type': 'application/json',
      },
      // Con la prueba de la contraseña, si el enlace la pide y ya se dio en la vista.
      body: JSON.stringify({ token, proof: leerPrueba(ctx, token) }),
    });
  } catch {
    return ctx.redirect(vista, 303);
  }
  if (!res.ok) return ctx.redirect(vista, 303); // caducado, de lector…: la vista lo dice
  const { document_id: id } = (await res.json()) as { document_id?: string };
  if (!id) return ctx.redirect(vista, 303);
  // La org al destino (ADR 0056): `edit` es ruta en `sinOrg`, así que un
  // `/docs/edit?id=…` sin org no recibe el salto del middleware y el editor
  // hidrata creyendo que `edit` es la organización. El canje acaba de dar el
  // permiso a la sesión, así que la canónica lleva SU organización.
  return ctx.redirect(r(`/edit?id=${encodeURIComponent(id)}`, session.user.tenant), 303);
};
