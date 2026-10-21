/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — descarga/exportación (BLUEPRINT.md §4.2). Dos pasos
 * upstream vía DocsClient, y PROXY de los bytes al navegador:
 *
 *   POST /api/export?id=&filename=  body {format}
 *     1) createExport(id, format)      → materializa el blob en S3 (201)
 *     2) downloadExport(id, blobId)    → bytes por streaming inline del servicio
 *   Se devuelven los bytes con Content-Disposition: attachment. El navegador NO
 *   puede seguir la URL prefirmada de S3 (apunta a minio interno) ni hablar con
 *   el servicio Go directamente (sin CORS): por eso el BFF hace de proxy.
 *
 * Formatos: 'pdf' | 'html' | 'markdown' | 'odf' | 'ooxml'.
 *
 * PDF estaba fuera por un comentario que decía que el backend no lo serializa, y sí
 * lo hace: `services/docs/internal/domain/export_pdf.go` es un motor propio con solo
 * la stdlib y `Export()` lo despacha para los tres tipos de documento. El menú del
 * editor ya ofrecía «PDF»; esta lista lo rechazaba con `bad_format`, así que la
 * entrada más buscada del menú devolvía un error.
 */
import type { APIRoute } from 'astro';
import { DocsClient, bearer } from '@sytue/api-clients';
import { requireId, json } from '../../lib/docsProxy';
import { bff, toBffContext, docsBaseUrl } from '../../lib/auth';
import { EXPORT_FORMATS } from '../../editor/types';

export const prerender = false;

// La lista vive en el editor (`editor/types.ts`) y aquí solo se consume: así el menú
// y esta verja no pueden decir cosas distintas, que es como PDF acabó dando error.
const FORMATS = new Set<string>(EXPORT_FORMATS);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);

  let payload: { format?: unknown; template?: unknown; accessible?: unknown };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'bad_request' }, 400);
  }
  const format = typeof payload.format === 'string' ? payload.format : '';
  if (!FORMATS.has(format)) return json({ error: 'bad_format' }, 400);
  // Como plantilla de Office u OpenDocument (.dotx .xltx .potx · .ott .ots .otp): solo
  // los dos paquetes que tienen plantilla.
  const template = payload.template === true;
  if (template && format !== 'ooxml' && format !== 'odf') return json({ error: 'bad_format' }, 400);

  const client = new DocsClient({
    baseUrl: docsBaseUrl(),
    auth: bearer(session.tokens.accessToken),
  });
  try {
    // El PDF sale accesible (PDF/UA) salvo que el interruptor del menú lo apague.
    const meta = await client.createExport(id, format, template, payload.accessible !== false);
    const { bytes, contentType } = await client.downloadExport(id, meta.id);
    const name = downloadName(new URL(ctx.request.url).searchParams.get('filename'), format);
    return new Response(bytes, {
      status: 200,
      headers: {
        'content-type': meta.contentType || contentType,
        // filename (fallback ASCII) + filename* (RFC 5987, UTF-8) para títulos con tildes.
        'content-disposition': `attachment; filename="${name.ascii}"; filename*=UTF-8''${name.utf8}`,
        'cache-control': 'no-store',
      },
    });
  } catch {
    return json({ error: 'export_failed' }, 502);
  }
};

/**
 * Nombre de descarga saneado a partir del `filename` del cliente (frontera de
 * confianza: se elimina lo que pueda inyectar cabeceras o rutas —control chars,
 * comillas, barras— conservando espacios y tildes). Si falta, `export.<ext>`.
 */
function downloadName(raw: string | null, format: string): { ascii: string; utf8: string } {
  const ext = format === 'markdown' ? 'md' : format;
  const cleaned = (raw ?? '')
    .split('')
    .filter((c) => c >= ' ' && c !== '"' && c !== '\\' && c !== '/')
    .join('')
    .trim()
    .slice(0, 160);
  const name = cleaned || `export.${ext}`;
  return { ascii: name.replace(/[^ -~]/g, '_'), utf8: encodeURIComponent(name) };
}
