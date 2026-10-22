/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/import  (multipart/form-data, campo `file`)
 *
 * Abre/convierte un fichero ofimático (docx/xlsx/pptx/csv/ods/odt/odp/md/html)
 * reenviándolo al servicio Go propio (POST /api/v1/import), que lo parsea
 * clean-room y devuelve { kind, title, content, comments } — el valor inicial del registro
 * CONTENT_KEY[kind] del editor. El cliente (DocList) crea el documento del kind
 * correcto y siembra `content` con una op `set`. El token de sesión NUNCA llega
 * al navegador: se añade aquí (mismo patrón proxy que el resto del BFF).
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext, docsBaseUrl } from '../../lib/auth';

export const prerender = false;

// Holgura sobre el tope de 25 MiB del servicio Go: rechazamos antes de bufferizar.
const MAX_BYTES = 26 * 1024 * 1024;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const len = Number(ctx.request.headers.get('content-length') ?? '0');
  if (Number.isFinite(len) && len > MAX_BYTES) return json({ error: 'too_large' }, 413);

  // El navegador sube el fichero como cuerpo CRUDO (application/octet-stream) con
  // el nombre en ?name= — así NO dispara el checkOrigin (CSRF) de Astro, que
  // bloquea los content-type "de formulario" (multipart/urlencoded/text-plain).
  // Mismo patrón que el upload de Drive (pages/api/content.ts). Reconstruimos el
  // multipart aquí (server→server, sin CSRF) porque el servicio Go espera `file`.
  const name = new URL(ctx.request.url).searchParams.get('name') || 'archivo';
  const bytes = await ctx.request.arrayBuffer();
  if (bytes.byteLength === 0) return json({ error: 'missing_file' }, 400);
  if (bytes.byteLength > MAX_BYTES) return json({ error: 'too_large' }, 413);

  const out = new FormData();
  out.append('file', new Blob([bytes]), name);

  let upstream: Response;
  try {
    // `fetch` reconstruye el multipart con un boundary nuevo (no reenviamos el crudo).
    // `comments=1`: el cliente guarda los comentarios del fichero al crear el documento
    // (DocList), así que el servicio puede dejar su ancla en el HTML.
    // `locale`: los avisos que el servicio escribe dentro del documento (un .pages que no se
    // puede convertir, por ejemplo) salen en el idioma de quien lo abre.
    upstream = await fetch(
      `${docsBaseUrl()}/api/v1/import?comments=1&locale=${encodeURIComponent(ctx.locals.locale)}`,
      {
        method: 'POST',
        headers: { authorization: `Bearer ${session.tokens.accessToken}` },
        body: out,
      },
    );
  } catch {
    return json({ error: 'upstream_unreachable' }, 502);
  }

  const body = upstream.status === 204 ? null : await upstream.text();
  return new Response(body, {
    status: upstream.status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
