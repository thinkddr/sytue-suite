/*
 * BFF de CONEXIONES CON TERCEROS — proxy a la superficie de operador de Sytue
 * Integrations.
 *
 *   GET    /api/platform-connections/oauth-clients
 *   PUT    /api/platform-connections/oauth-clients/<plantilla>
 *   DELETE /api/platform-connections/oauth-clients/<plantilla>
 *   GET    /api/platform-connections/templates
 *
 * QUÉ CONFIGURA. Las credenciales con las que SYTUE se presenta ante Google,
 * Microsoft y los demás proveedores. Con ellas, migrar el correo de una empresa
 * es un clic —su admin aprueba una pantalla de permisos—; sin ellas, cada
 * organización tiene que darse de alta como desarrollador ante el proveedor y
 * pegar sus propias credenciales.
 *
 * DOS AUTORIZACIONES DISTINTAS, y las dos hacen falta:
 *
 *   1. Que quien pide sea ADMIN DE PLATAFORMA. Eso lo decide Sytue ID
 *      (allowlist `platform_admins` + segundo factor) y aquí se comprueba
 *      preguntándoselo, no reimplementándolo: se hace una llamada a su API de
 *      plataforma y, si responde, es que lo es.
 *   2. Que Sytue Integrations acepte la operación. Su superficie de operador va
 *      con un token propio del despliegue, que vive SOLO en el servidor y nunca
 *      baja al navegador.
 *
 * Sin la primera, cualquiera con sesión podría registrar las credenciales de la
 * plataforma. Sin la segunda, este BFF no podría llamar. Ninguna sustituye a la
 * otra.
 */
import type { APIRoute } from 'astro';
import { requireSession, serverEnv } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { platformFetch } from '../../../lib/platform.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

// Clave de plantilla: minúsculas, dígitos y guiones. Anclada, y sin permitir
// puntos ni barras: esto se concatena a una URL, y un `..` convertiría el proxy
// en un camino para llamar a cualquier ruta de la superficie de operador.
const PLANTILLA = '[a-z0-9][a-z0-9-]{0,62}';
const RUTAS = [
  /^templates$/,
  new RegExp(`^templates/${PLANTILLA}/enabled$`),
  /^oauth-clients$/,
  new RegExp(`^oauth-clients/${PLANTILLA}$`),
];

function rutaPermitida(path: string): boolean {
  return RUTAS.some((re) => re.test(path));
}

/**
 * Base del servicio de integraciones dentro del clúster y token de operador.
 *
 * `serverEnv` y no `import.meta.env` a secas: en el SSR COMPILADO el entorno de
 * build quedó congelado —solo lleva BASE_URL, MODE, PROD y compañía— y las
 * variables del contenedor solo viven en `process.env`. Sin esto el token sale
 * vacío y cada guardado se manda con un `Bearer ` sin nada detrás, que el
 * servicio rechaza. Le pasó a la pantalla que llama a este proxy.
 */
function upstream(): { base: string; token: string } {
  const env = serverEnv(import.meta.env);
  return {
    base: String(env.SYTUE_INTEGRATIONS_INTERNAL_URL ?? 'http://sytue-one-integrations'),
    token: String(env.SYTUE_INTEGRATIONS_OPERATOR_TOKEN ?? ''),
  };
}

async function proxy(ctx: Parameters<APIRoute>[0], method: string): Promise<Response> {
  const guard = await requireSession(getBff(), toBffContext(ctx));
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const path = String(ctx.params.path ?? '');
  if (!rutaPermitida(path)) return json({ error: 'not_found' }, 404);

  // ¿Es admin de plataforma? Se le pregunta a Sytue ID, que es quien lo sabe.
  // `usage` es la lectura más barata de su API de plataforma y lleva el mismo
  // guardia que el resto, así que su respuesta ES la respuesta.
  const quien = await platformFetch<unknown>(guard.session, '/usage', { method: 'GET' });
  if (!quien.ok) {
    return json({ error: 'forbidden' }, quien.status === 401 ? 401 : 403);
  }

  const { base, token } = upstream();
  if (!token) {
    // Sin token de operador no hay superficie que llamar. Se dice, en vez de
    // devolver un 500 opaco: es un despliegue incompleto, no un error del
    // usuario, y quien lo vea tiene que saber qué falta.
    return json(
      {
        error: 'operator_token_missing',
        message:
          'Falta SYTUE_INTEGRATIONS_OPERATOR_TOKEN en el despliegue de esta superficie; ' +
          'sin él no se pueden gestionar las credenciales de los proveedores.',
      },
      503,
    );
  }

  let body: string | undefined;
  if (method === 'PUT') {
    const recibido: unknown = await ctx.request.json().catch(() => ({}));
    if (path.startsWith('oauth-clients')) {
      // Se firma quién lo hace con el correo de la sesión, no con lo que venga en
      // el cuerpo: es para auditar, y un campo que el cliente rellena no audita
      // nada.
      body = JSON.stringify({
        ...(recibido as Record<string, unknown>),
        updated_by: guard.session.user.email,
      });
    } else {
      body = JSON.stringify(recibido);
    }
  }

  // El upstream es el nombre de servicio del clúster, no una URL de la petición:
  // `path` ya está validado contra la allowlist de arriba.
  const res = await fetch(`${base}/operator/${path}`, {
    method,
    headers: {
      authorization: `Bearer ${token}`,
      ...(body ? { 'content-type': 'application/json' } : {}),
    },
    ...(body ? { body } : {}),
  });
  if (res.status === 204) return new Response(null, { status: 204 });
  const texto = await res.text();
  return new Response(texto || '{}', {
    status: res.status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = (ctx) => proxy(ctx, 'GET');
export const PUT: APIRoute = (ctx) => proxy(ctx, 'PUT');
export const DELETE: APIRoute = (ctx) => proxy(ctx, 'DELETE');
