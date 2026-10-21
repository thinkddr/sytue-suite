/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/create  { kind: 'document'|'spreadsheet'|'presentation', title? }
 *                    { template: 'carta'|…|'org:<id>', template_version?, title? }
 *
 * Crea un documento vacio en el servicio Go propio (docs.sytue.eu) usando el SDK
 * DocsClient (@sytue/api-clients) con el access token de la sesion. Devuelve el id
 * para abrir el editor nativo. Implementacion clean-room: SIN motores de terceros.
 *
 * Con `template`, el documento nace de una plantilla de la galería: el SERVICIO
 * pone el tipo, el título (si no llega otro) y el contenido, en el idioma de la
 * sesión y en la misma petición. Aquí no se siembra nada.
 */
import type { APIRoute } from 'astro';
import { DocsClient, bearer, type DocKind } from '@sytue/api-clients';
import { bff, toBffContext, docsBaseUrl } from '../../lib/auth';
import { esUuid } from '../../lib/uuid';

export const prerender = false;

const DEFAULT_TITLE: Record<DocKind, string> = {
  document: 'Documento sin título',
  spreadsheet: 'Hoja sin título',
  presentation: 'Presentación sin título',
};

function isKind(value: unknown): value is DocKind {
  return value === 'document' || value === 'spreadsheet' || value === 'presentation';
}

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: {
    kind?: unknown;
    title?: unknown;
    template?: unknown;
    template_version?: unknown;
    theme?: unknown;
    brand_version?: unknown;
  };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const template = typeof body.template === 'string' ? body.template.trim() : '';
  const kind = body.kind ?? 'document';
  if (!isKind(kind)) return json({ error: 'invalid_kind' }, 400);

  // Con una plantilla de la organización («org:<id>»), la versión que se copia.
  const templateVersion =
    typeof body.template_version === 'string' && esUuid(body.template_version)
      ? body.template_version
      : '';

  // El tema (catálogo común o «marca») que se aplica al contenido de la plantilla.
  const theme =
    typeof body.theme === 'string' && /^[a-z0-9-]{1,24}$/.test(body.theme) ? body.theme : '';
  const brandVersion =
    typeof body.brand_version === 'number' && Number.isInteger(body.brand_version)
      ? body.brand_version
      : 0;

  const rawTitle = typeof body.title === 'string' ? body.title.trim() : '';
  // Sin título, el de la plantilla (lo decide el servicio, ya traducido).
  const title = rawTitle || (template ? '' : DEFAULT_TITLE[kind]);

  const client = new DocsClient({
    baseUrl: docsBaseUrl(),
    auth: bearer(session.tokens.accessToken),
  });

  try {
    const doc = await client.create(
      template
        ? {
            kind,
            title,
            template,
            locale: ctx.locals.locale,
            ...(templateVersion ? { templateVersion } : {}),
            ...(theme ? { theme } : {}),
            ...(brandVersion > 0 ? { brandVersion } : {}),
          }
        : { kind, title },
    );
    return json({ id: doc.id, title: doc.title, kind: doc.kind }, 201);
  } catch (e) {
    // EL ERROR SE REGISTRA, no se descarta. Este `catch` estaba vacío y convertía
    // cualquier fallo del servicio —un SQL que revienta, un token caducado, la red—
    // en el mismo 502 mudo: en el navegador solo se veía «502» y en el servidor,
    // nada. Diagnosticar «no se crea el segundo documento» exigía leer el código en
    // vez de un log, que es justo lo que un log existe para evitar.
    //
    // El detalle NO viaja al cliente: puede llevar dentro la consulta o el DSN. El
    // navegador sigue recibiendo `create_failed` y el porqué se queda aquí.
    console.error('[docs] /api/create falló', {
      kind,
      template,
      title,
      error: e instanceof Error ? e.message : String(e),
    });
    return json({ error: 'create_failed' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
