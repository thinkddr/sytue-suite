/*
 * BFF de ADMINISTRACIÓN DE PLATAFORMA — proxy a /api/v1/platform de Sytue ID.
 *
 *   GET|POST|DELETE /api/platform/<lo que sea>
 *
 * Es un proxy y no un endpoint por operación a propósito: la autorización real
 * (allowlist `platform_admins` + segundo factor obligatorio) vive ENTERA en
 * Sytue ID y se comprueba en cada petición. Reimplementar aquí un permiso por
 * ruta duplicaría la decisión en dos sitios que acabarían discrepando; y como el
 * navegador nunca ve el access token, sin el proxy no habría forma de llamar.
 *
 * Lo que sí impone este proxy es la allowlist de rutas: no reenvía cualquier
 * cosa bajo /api/v1, solo lo que la consola de plataforma usa.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { platformFetch } from '../../../lib/platform.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

// Rutas admitidas, ancladas: `orgs`, `orgs/<uuid>`, `orgs/<uuid>/status`,
// `orgs/<uuid>/seat-grants`, `seat-grants/<uuid>`, `merges`, `merges/<uuid>`,
// `merges/<uuid>/resume`, `admins`, `admins/<uuid>` y `usage`.
const UUID = '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}';
const RUTAS = [
  /^usage$/,
  /^orgs$/,
  new RegExp(`^orgs/${UUID}$`),
  new RegExp(`^orgs/${UUID}/status$`),
  new RegExp(`^orgs/${UUID}/education$`),
  new RegExp(`^orgs/${UUID}/seat-grants$`),
  // Propietarios y cuentas de cualquier organización (platform_usuarios.go).
  new RegExp(`^orgs/${UUID}/members$`),
  new RegExp(`^orgs/${UUID}/owners$`),
  new RegExp(`^orgs/${UUID}/owner-invitations$`),
  new RegExp(`^orgs/${UUID}/members/${UUID}/(password|block|unblock|delete)$`),
  new RegExp(`^seat-grants/${UUID}$`),
  /^merges$/,
  new RegExp(`^merges/${UUID}$`),
  new RegExp(`^merges/${UUID}/resume$`),
  /^admins$/,
  new RegExp(`^admins/${UUID}$`),
  new RegExp(`^admins/${UUID}/review$`),
  // PIM del alcance global (mig 0043). Las tres primeras van FUERA del gate en
  // Sytue ID —quien no está elevado no podría pedir elevarse—, así que este
  // proxy es la única forma de llegar a ellas desde el navegador.
  /^activation$/,
  /^activate$/,
  /^deactivate$/,
  /^activations$/,
  // Registro de lo que hace un admin global (mig 0045).
  /^audit$/,
  // Qué versión corre cada pieza de la suite.
  /^versions$/,
  // Problemas recientes agrupados + cifras de cabecera. Es la pantalla que se
  // abre cuando algo va mal: dejarla fuera de la allowlist la deja en blanco
  // justo cuando hace falta.
  /^logs$/,
  /^logs\/stats$/,
  // Segunda persona para lo irreversible (mig 0046).
  /^approvals$/,
  new RegExp(`^approvals/${UUID}/approve$`),
  new RegExp(`^approvals/${UUID}/reject$`),
];

function rutaPermitida(path: string): boolean {
  return RUTAS.some((re) => re.test(path));
}

async function proxy(ctx: Parameters<APIRoute>[0], method: string): Promise<Response> {
  // Sin exigencia de rol de organización, por lo mismo que la página: el gate
  // real es platform_admins + MFA y vive en Sytue ID, que lo comprueba en cada
  // petición. Duplicar aquí un permiso distinto solo podía divergir del de
  // verdad, y de hecho divergía.
  const guard = await requireSession(getBff(), toBffContext(ctx));
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const path = String(ctx.params.path ?? '');
  if (!rutaPermitida(path)) return json({ error: 'not_found' }, 404);

  const url = new URL(ctx.request.url);
  const query = url.searchParams.toString();
  let body: unknown;
  if (method === 'POST') {
    try {
      body = await ctx.request.json();
    } catch {
      body = {};
    }
  }
  const res = await platformFetch<unknown>(guard.session, `/${path}${query ? `?${query}` : ''}`, {
    method,
    ...(method === 'POST' ? { body } : {}),
  });
  // El `code` y el `message` del upstream se conservan: «la ventana va de 5
  // minutos a 8 horas» es la respuesta a lo que el admin acaba de escribir, y
  // aplanarla a «error» le deja probando a ciegas. Los `params`, para poder
  // decirlo traducido (p. ej. de qué organizaciones es la única propietaria).
  if (!res.ok) {
    return json(
      {
        error: 'upstream',
        status: res.status,
        code: res.code,
        message: res.message,
        params: res.params,
      },
      res.status,
    );
  }
  return json(res.data ?? {}, method === 'DELETE' ? 204 : 200);
}

export const GET: APIRoute = (ctx) => proxy(ctx, 'GET');
export const POST: APIRoute = (ctx) => proxy(ctx, 'POST');
export const DELETE: APIRoute = (ctx) => proxy(ctx, 'DELETE');
