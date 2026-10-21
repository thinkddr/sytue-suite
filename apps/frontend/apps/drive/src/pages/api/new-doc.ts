/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — CREAR UN DOCUMENTO DE SYTUE DOCS sin salir de Drive.
 *
 *   POST /api/new-doc  { kind: 'document'|'spreadsheet'|'presentation', title? }
 *   → { id, kind, href }   href = URL del editor, lista para abrir
 *
 * Por qué vive en el BFF de Drive y no redirige a Docs: el access token de la sesión es
 * httpOnly y se resuelve en el servidor; mandar al navegador a que lo pida en otro
 * dominio obligaría a un rebote de sesión visible para el usuario. Así, «Nuevo →
 * Documento» crea y abre, sin pasos intermedios.
 *
 * LÍMITE QUE HAY QUE CONOCER: un documento de Docs NO es un fichero de Drive. Son dos
 * almacenes distintos y `Document` no tiene carpeta, así que el documento nuevo NO
 * aparece en la carpeta donde estabas. Antes eso lo hacía desaparecer de la vista; ahora
 * sale en la sección «Documentos» de la sidebar (GET /api/documents). Anclarlo de verdad
 * a una carpeta exige que Docs conozca las carpetas de Drive: es otra ola.
 */
import type { APIRoute } from 'astro';
import { enlaceDocumento } from '@sytue/config';
import { serverEnv } from '@sytue/auth/server';
import { DocsClient, bearer, type DocKind } from '@sytue/api-clients';
import { bff, toBffContext } from '../../lib/auth';

export const prerender = false;

/** Títulos por defecto: los mismos que usa Sytue Docs al crear desde su propia lista. */
const DEFAULT_TITLE: Record<DocKind, string> = {
  document: 'Documento sin título',
  spreadsheet: 'Hoja sin título',
  presentation: 'Presentación sin título',
};

function isKind(v: unknown): v is DocKind {
  return v === 'document' || v === 'spreadsheet' || v === 'presentation';
}

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

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { kind?: unknown; title?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const kind = body.kind;
  if (!isKind(kind)) return json({ error: 'invalid_kind' }, 400);
  const title =
    typeof body.title === 'string' && body.title.trim() ? body.title.trim() : DEFAULT_TITLE[kind];

  try {
    const client = new DocsClient({
      baseUrl: docsBaseUrl(),
      auth: bearer(session.tokens.accessToken),
    });
    const doc = await client.create({ kind, title });
    return json(
      {
        id: doc.id,
        kind: doc.kind,
        // La org en el href (ADR 0056): sin ella, `/docs/edit` —ruta en `sinOrg`
        // de Docs— se serviría siempre sin org y el editor hidrataría creyendo
        // que `edit` es la organización. El documento nace del token de la
        // sesión, así que la canónica lleva SU organización.
        href: enlaceDocumento(doc.id, doc.kind, session.user.tenant),
      },
      201,
    );
  } catch {
    // No se distingue el motivo hacia el navegador: crear un documento no revela nada
    // que ayude a quien no debería poder hacerlo.
    return json({ error: 'create_failed' }, 502);
  }
};
