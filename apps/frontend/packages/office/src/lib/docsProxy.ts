/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Proxy same-origin al servicio Go de Sytue Docs (BLUEPRINT.md §4.1). El
 * navegador no puede hablar con docs-api directamente (sin CORS ni WS): estas
 * rutas BFF resuelven la sesión httpOnly, añaden Authorization: Bearer y
 * reenvían la petición VERBATIM (los cuerpos Go llegan en snake_case al
 * cliente; los tipos de src/editor/types.ts lo reflejan).
 */
import type { APIContext } from 'astro';
import type { DocKind } from '@sytue/api-clients';
import { bff, toBffContext, docsBaseUrl } from './auth';
import { esKind } from './producto';
import { esUuid } from './uuid';

export { esUuid };

/** Respuesta JSON con las cabeceras estándar del BFF. */
export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/**
 * Reenvía `path` (relativo a /api/v1) al servicio Go con el token de la sesión.
 * Devuelve una Response con el MISMO status y cuerpo textual (204 → cuerpo null).
 * Todo parámetro interpolado en `path` debe pasar por encodeURIComponent
 * (frontera de confianza) ANTES de llamar aquí.
 */
export async function proxyDocs(
  ctx: APIContext,
  path: string,
  init?: RequestInit,
): Promise<Response> {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let upstream: Response;
  try {
    upstream = await fetch(`${docsBaseUrl()}/api/v1${path}`, {
      ...init,
      headers: {
        ...(init?.headers as Record<string, string> | undefined),
        authorization: `Bearer ${session.tokens.accessToken}`,
      },
    });
  } catch {
    return json({ error: 'upstream_unreachable' }, 502);
  }

  const body = upstream.status === 204 ? null : await upstream.text();
  return new Response(body, {
    status: upstream.status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** Lee y valida el parámetro obligatorio `id` de la query. */
/**
 * El tipo de un documento, para que el título y el icono de la pestaña salgan
 * del producto correcto (Docus/Calcus/Diapos) desde el primer pintado. `null`
 * si no se sabe a tiempo: el editor no espera por esto, y al cargar el
 * documento el cliente corrige la marca (lib/producto.ts).
 */
export async function kindDeDocumento(accessToken: string, id: string): Promise<DocKind | null> {
  try {
    const r = await fetch(`${docsBaseUrl()}/api/v1/documents/${encodeURIComponent(id)}`, {
      headers: { authorization: `Bearer ${accessToken}` },
      signal: AbortSignal.timeout(800),
    });
    if (!r.ok) return null;
    const body = (await r.json()) as { document?: { kind?: unknown } };
    return esKind(typeof body.document?.kind === 'string' ? body.document.kind : null)
      ? (body.document!.kind as DocKind)
      : null;
  } catch {
    return null;
  }
}

export function requireId(ctx: APIContext): string | null {
  const id = new URL(ctx.request.url).searchParams.get('id');
  return id && esUuid(id) ? id : null;
}
