/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — LISTA DE DOCUMENTOS DE SYTUE DOCS.
 *
 *   GET /api/documents?q=&cursor=   → { documents: [{id, title, kind, updatedAt, href}], nextCursor? }
 *
 * Por qué Drive necesita esto: un documento de Docs NO es un fichero de Drive. Son dos
 * almacenes distintos y `Document` no tiene carpeta, así que hasta ahora un documento
 * creado desde Drive («Nuevo → Documento») desaparecía de la vista: quedaba en Sytue Docs
 * y Drive no sabía de él. La sección «Documentos» lo muestra donde el usuario lo creó.
 *
 * NO se copia ni se sincroniza nada: se lista lo que Docs ya sabe. La autorización la
 * aplica el servicio de Docs sobre el token de la sesión — aquí no se filtra por dueño,
 * porque duplicar esa regla es exactamente cómo se acaba con dos verdades.
 *
 * El filtro `q` va AL SERVICIO (índice trigram sobre el título): nunca se trae todo para
 * filtrar en el navegador.
 */
import type { APIRoute } from 'astro';
import { enlaceDocumento } from '@sytue/config';
import { serverEnv } from '@sytue/auth/server';
import { DocsClient, bearer } from '@sytue/api-clients';
import { bff, toBffContext } from '../../lib/auth';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** URL del servicio Go de Sytue Docs. */
function docsBaseUrl(): string {
  const env = serverEnv(import.meta.env) as Record<string, string | undefined>;
  return env.PUBLIC_SYTUE_DOCS_BASE_URL ?? 'https://docs.sytue.eu';
}

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const params = new URL(ctx.request.url).searchParams;
  const q = params.get('q')?.trim() ?? '';
  const cursor = params.get('cursor')?.trim() ?? '';

  try {
    const client = new DocsClient({
      baseUrl: docsBaseUrl(),
      auth: bearer(session.tokens.accessToken),
    });
    const page = await client.list({
      ...(q ? { q } : {}),
      ...(cursor ? { cursor } : {}),
      limit: 50,
    });
    return json({
      documents: page.documents.map((d) => ({
        id: d.id,
        title: d.title,
        kind: d.kind,
        updatedAt: d.updatedAt,
        // La org en el href (ADR 0056): sin ella, `/docs/edit` —ruta en `sinOrg`
        // de Docs— se serviría siempre sin org y el editor hidrataría creyendo
        // que `edit` es la organización. La lista es del token de la sesión, así
        // que la canónica lleva SU organización.
        href: enlaceDocumento(d.id, d.kind, session.user.tenant),
      })),
      ...(page.nextCursor ? { nextCursor: page.nextCursor } : {}),
    });
  } catch {
    // Docs caído no debe romper Drive: la vista dirá que no pudo cargarlos y el resto
    // del explorador sigue funcionando.
    return json({ error: 'docs_unavailable' }, 502);
  }
};
