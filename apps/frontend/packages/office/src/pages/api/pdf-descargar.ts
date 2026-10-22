/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/pdf-descargar?path=… — el documento original, tal cual está en Drive.
 *
 * Hace falta para firmar con la aplicación oficial del país: hay que bajar el
 * PDF, firmarlo fuera y devolverlo. Antes no había forma de sacarlo de aquí —el
 * visor lo lee por el servidor y nunca baja el fichero entero al navegador— así
 * que la única salida era ir a Drive a buscarlo, que es mandar a la persona a
 * otra aplicación en mitad de una tarea.
 *
 * El token no toca el navegador: el fichero lo descarga este puente con la
 * sesión y lo reenvía en streaming, sin leerlo a memoria. Un PDF escaneado son
 * decenas de megas y bufferizarlo multiplicaría la memoria de la réplica por
 * cada descarga en curso.
 */
import type { APIRoute } from 'astro';
import { DriveClient, bearer } from '@sytue/api-clients';
import { bff, toBffContext, driveBaseUrl } from '../../lib/auth';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** El nombre del fichero, para que se guarde con el suyo y no con el de la ruta. */
function nombreDe(ruta: string): string {
  const n = ruta.split('/').filter(Boolean).pop() ?? 'documento.pdf';
  return n.toLowerCase().endsWith('.pdf') ? n : `${n}.pdf`;
}

export const GET: APIRoute = async (ctx) => {
  const guard = await bff.getSession(toBffContext(ctx));
  if (!guard) return json({ error: 'unauthenticated' }, 401);

  const ruta = (new URL(ctx.request.url).searchParams.get('path') ?? '').trim();
  if (!ruta) return json({ error: 'missing_path' }, 400);

  const client = new DriveClient({
    baseUrl: driveBaseUrl(),
    auth: bearer(guard.tokens.accessToken),
  });
  let original: Response;
  try {
    original = await client.openDownload(ruta);
  } catch {
    return json({ error: 'download_failed' }, 404);
  }
  const nombre = nombreDe(ruta);
  return new Response(original.body, {
    status: 200,
    headers: {
      'content-type': 'application/pdf',
      // `attachment`: esto es para llevárselo a otro programa, no para verlo
      // otra vez en una pestaña.
      'content-disposition': `attachment; filename="${encodeURIComponent(nombre)}"`,
      'cache-control': 'no-store',
      'x-content-type-options': 'nosniff',
    },
  });
};
