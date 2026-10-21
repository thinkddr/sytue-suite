/**
 * /api/categories — categorías del sidebar del usuario (proxy BFF hacia el servicio
 * Go Sytue Chat, autorizado por cookie; el access token no toca el navegador).
 *
 *   GET  → lista mis categorías.
 *   POST → crea / actualiza / borra según el cuerpo (JSON, no form, contra el CSRF de
 *          Astro dev; el borrado va por POST {delete:true}, mismo patrón que /api/pin):
 *            { name }                              → crear
 *            { id, name?, sortOrder?, collapsed? } → actualizar
 *            { id, delete: true }                  → borrar (los canales recaen a la
 *                                                     categoría por defecto)
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const categories = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).listCategories();
    return json({ categories });
  } catch (err) {
    return json({ error: 'chat_error', message: String(err) }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: {
    id?: string;
    name?: string;
    sortOrder?: number;
    collapsed?: boolean;
    delete?: boolean;
  };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
  try {
    const id = payload.id?.trim();
    if (id && payload.delete === true) {
      await client.deleteCategory(id);
      return json({ ok: true });
    }
    if (id) {
      const category = await client.updateCategory(id, {
        ...(payload.name !== undefined ? { name: payload.name } : {}),
        ...(payload.sortOrder !== undefined ? { sortOrder: payload.sortOrder } : {}),
        ...(payload.collapsed !== undefined ? { collapsed: payload.collapsed } : {}),
      });
      return json({ category });
    }
    const name = payload.name?.trim();
    if (!name) return json({ error: 'missing_name' }, 400);
    const category = await client.createCategory(name);
    return json({ category }, 201);
  } catch (err) {
    return json({ error: 'chat_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
