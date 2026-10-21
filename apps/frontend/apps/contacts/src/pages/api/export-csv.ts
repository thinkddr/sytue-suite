/**
 * Exportación CSV — proxy BFF autorizado por cookie. Descarga TODOS los contactos
 * del usuario como CSV (formato Google Contacts, el más portable). El servicio Go
 * neutraliza la inyección de fórmulas CSV al serializar.
 *
 * GET /api/export-csv -> GET /api/v1/export-csv  (text/csv, attachment)
 *
 * Es una lectura (GET): el navegador puede apuntar aquí con un enlace de descarga
 * y la cookie viaja same-origin. El access token nunca llega al cliente.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { CONTACTS_BASE_URL } from '../../lib/carddav.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) {
    return new Response(JSON.stringify({ error: 'unauthenticated' }), {
      status: 401,
      headers: { 'content-type': 'application/json; charset=utf-8' },
    });
  }

  const res = await fetch(`${CONTACTS_BASE_URL}/api/v1/export-csv`, {
    headers: { authorization: `Bearer ${session.accessToken}` },
  });
  if (!res.ok || !res.body) {
    return new Response(JSON.stringify({ error: 'export_failed' }), {
      status: res.status >= 500 ? 502 : res.status,
      headers: { 'content-type': 'application/json; charset=utf-8' },
    });
  }
  // Reenvía el CSV en streaming con las cabeceras de descarga.
  return new Response(res.body, {
    status: 200,
    headers: {
      'content-type': 'text/csv; charset=utf-8',
      'content-disposition': 'attachment; filename="contactos.csv"',
      'cache-control': 'no-store',
    },
  });
};
