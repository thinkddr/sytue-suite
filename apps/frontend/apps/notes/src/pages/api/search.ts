/**
 * GET /api/search?q= — busca en las notas indexables visibles. Sin `q`, las
 * recientes. Lo que el usuario dejó fuera del índice no sale de aquí: el filtro
 * lo aplica el servicio, no esta capa.
 */
import type { APIRoute } from 'astro';
import { consulta } from '@sytue/auth/server';
import { conCliente, fallo, json } from '../../lib/notes.js';

export const prerender = false;

const buscar: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  const p = await consulta(ctx.request, ctx.url);
  try {
    // `archived=1`: el filtro «Incluir archivados»; sin él lo archivado no sale.
    return json({ notes: await c.client.search(p.get('q') ?? '', 30, p.get('archived') === '1') });
  } catch (err) {
    return fallo(err);
  }
};

// GET y QUERY, el mismo handler. QUERY es el preferente del cliente y GET el
// respaldo para quien no lo hable; ver `consulta` en @sytue/auth/server.
export const GET: APIRoute = buscar;
export const QUERY: APIRoute = buscar;
