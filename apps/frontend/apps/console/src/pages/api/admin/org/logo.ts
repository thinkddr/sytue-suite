/*
 * BFF de Sytue Admin — logotipo de la organización.
 *
 *   PUT    /api/admin/org/logo?save=0|1   cuerpo: el fichero CRUDO
 *       save=0 analiza y devuelve el reencuadre SIN guardar (la previsualización
 *       de la consola); save=1 lo persiste. Rol admin/owner.
 *   DELETE /api/admin/org/logo            vuelve a la marca de Sytue.
 *
 * Proxy PURO: reenvía los bytes tal cual y devuelve lo que conteste Sytue ID sin
 * reescribirlo. No valida la imagen ni la toca. Quien decide qué imagen se acepta y
 * cómo se reencuadra es el backend (platform/pkg/logofit), y tiene que seguir
 * siéndolo aunque alguien llame al servicio sin pasar por aquí: un BFF que
 * reimplementa la regla acaba discrepando del servicio que la aplica de verdad.
 *
 * Existe porque el navegador nunca ve el access token. El techo de tamaño lo pone
 * Sytue ID (1 MiB), no este proxy, por lo mismo.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { adminBaseUrl } from '../../../../lib/auth.js';

export const prerender = false;

async function reenviar(ctx: Parameters<APIRoute>[0], method: 'PUT' | 'DELETE'): Promise<Response> {
  // Con `roles`: el guard comprueba aquí lo mismo que Sytue ID comprueba después.
  // No es duplicar la decisión, es no gastar un viaje al servicio para que rebote.
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) {
    return new Response(
      JSON.stringify({ message: 'No tienes permiso para cambiar el logotipo.' }),
      {
        status: guard.status === 403 ? 403 : 401,
        headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
      },
    );
  }

  // `save` se normaliza a 0/1 y NO se reenvía la query entera: es el único
  // parámetro que entiende el servicio, y así nada más puede colarse por aquí.
  const save = new URL(ctx.request.url).searchParams.get('save') === '1' ? '1' : '0';
  const url =
    method === 'PUT'
      ? `${adminBaseUrl()}/api/v1/admin/org/logo?save=${save}`
      : `${adminBaseUrl()}/api/v1/admin/org/logo`;

  try {
    const res = await fetch(url, {
      method,
      headers: {
        authorization: `Bearer ${guard.session.tokens.accessToken}`,
        ...(method === 'PUT' ? { 'content-type': 'application/octet-stream' } : {}),
      },
      ...(method === 'PUT' ? { body: await ctx.request.arrayBuffer() } : {}),
    });
    // El cuerpo se devuelve tal cual —incluido el mensaje de error, que la tarjeta
    // enseña al administrador— con el mismo status que dio el servicio.
    return new Response(res.body, {
      status: res.status,
      headers: {
        'content-type': res.headers.get('content-type') ?? 'application/json; charset=utf-8',
        'cache-control': 'no-store',
      },
    });
  } catch {
    return new Response(JSON.stringify({ message: 'No se pudo contactar con Sytue ID.' }), {
      status: 502,
      headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
    });
  }
}

export const PUT: APIRoute = (ctx) => reenviar(ctx, 'PUT');
export const DELETE: APIRoute = (ctx) => reenviar(ctx, 'DELETE');
