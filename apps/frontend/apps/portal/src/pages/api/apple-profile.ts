/**
 * POST /api/apple-profile — el perfil de Apple (.mobileconfig) que se instala SIN
 * escribir ninguna contraseña.
 *
 * Sytue ID crea una contraseña de aplicación para este dispositivo y la mete en las tres
 * cuentas del perfil (correo, calendario y contactos): el IMAP no acepta la contraseña
 * de la cuenta, así que el perfil sin ella instalaba cuentas que no podían entrar. La
 * contraseña solo viaja dentro del perfil y se revoca en Mi cuenta como cualquier otra.
 *
 * POST y no GET: crear una credencial no puede depender de una descarga que el
 * navegador precarga o repite (y el `checkOrigin` de Astro protege los POST). El access
 * token no llega al navegador.
 */
import type { APIRoute } from 'astro';
import { getApiSession, idBaseUrl } from '../../lib/auth';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return new Response(null, { status: 401 });

  let upstream: Response;
  try {
    upstream = await fetch(`${idBaseUrl()}/api/v1/me/apple-profile`, {
      method: 'POST',
      headers: {
        authorization: `Bearer ${session.accessToken}`,
        'content-type': 'application/json',
      },
      // Sin etiqueta: Sytue ID pone la suya por defecto a la contraseña de aplicación.
      body: '{}',
      signal: ctx.request.signal,
    });
  } catch {
    return new Response(null, { status: 502 });
  }
  if (!upstream.ok || !upstream.body) {
    return new Response(null, { status: upstream.status === 429 ? 429 : 502 });
  }
  return new Response(upstream.body, {
    status: 200,
    headers: {
      'content-type': 'application/x-apple-aspen-config',
      'content-disposition': 'attachment; filename="sytue.mobileconfig"',
      'cache-control': 'no-store',
    },
  });
};
