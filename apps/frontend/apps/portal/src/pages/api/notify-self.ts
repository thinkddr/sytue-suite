/**
 * POST /api/notify-self — emite una notificación de prueba al PROPIO usuario.
 *
 * Afordancia de desarrollo (solo `astro dev`): proxy autenticado del POST
 * /notifications del Hub con el subject fijado a la sesión (nunca a terceros, así
 * que no requiere scope). Permite probar la campana de la shell end-to-end sin
 * depender de que otro servicio emita todavía. El access token JAMÁS llega al
 * navegador. Cuerpo JSON opcional {title, body, url, source}. docs/CONVENTIONS §11.
 *
 * Genera una notificación así:
 *   curl -X POST https://one.sytue.localhost/api/notify-self \
 *        -H 'content-type: application/json' -b <cookie de sesión> \
 *        -d '{"title":"Hola","body":"Prueba"}'
 */
import type { APIRoute } from 'astro';
import type { NewNotification } from '@sytue/api-clients';
import { getApiSession } from '../../lib/auth';
import { hubClient } from '../../lib/hub';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

function str(v: unknown, fallback: string): string {
  return typeof v === 'string' && v.trim() ? v.trim() : fallback;
}

export const POST: APIRoute = async (ctx) => {
  // Solo dev: en producción las notificaciones las emiten los servicios reales.
  if (!import.meta.env.DEV) return json({ error: 'not_found' }, 404);

  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const raw = (await ctx.request.json().catch(() => ({}))) as Record<string, unknown>;
  const notif: NewNotification = {
    source: 'drive',
    type: 'dev.test',
    title: str(raw.title, 'Notificación de prueba'),
    body: str(raw.body, 'Emitida desde el portal para probar la campana de la shell.'),
    // Sin url por defecto: al marcar leída desde el popover no navega.
    ...(typeof raw.url === 'string' && raw.url ? { url: raw.url } : {}),
  };

  try {
    const created = await hubClient(session.accessToken).createNotification(notif);
    return json({ notification: created }, 201);
  } catch {
    return json({ error: 'emit_failed' }, 502);
  }
};
