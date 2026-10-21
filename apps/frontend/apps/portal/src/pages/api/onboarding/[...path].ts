/**
 * BFF del alta inicial — proxy autorizado a Sytue ID (/api/v1/onboarding*).
 *
 *   GET   /api/onboarding                estado + resumen REAL (lo que hay en BD)
 *   PATCH /api/onboarding                guarda el paso en curso (y la facturación)
 *   GET   /api/onboarding/slug?slug=…    disponibilidad real del identificador
 *   POST  /api/onboarding/organization   crea (o renombra) la organización
 *   POST  /api/onboarding/domain         reclama el dominio y lo verifica por DNS
 *   POST  /api/onboarding/domain/cloudflare
 *                                        configura la zona del cliente en SU cuenta
 *                                        de Cloudflare (previsualizar / aplicar)
 *   GET   /api/onboarding/domain-availability?domain=…
 *                                        ¿está libre y cuánto cuesta? (solo consulta)
 *   POST  /api/onboarding/invitations    invita por correo (invitaciones reales)
 *   POST  /api/onboarding/roles          crea los roles recomendados elegidos
 *
 * Resuelve el access token en el servidor desde la cookie de sesión httpOnly: el
 * token JAMÁS llega al navegador (docs/CONVENTIONS.md §11). La lista blanca de
 * sub-rutas evita que este comodín se convierta en un proxy abierto a Sytue ID.
 * El status y el cuerpo del servicio se propagan tal cual: si algo falla, el
 * asistente enseña el motivo REAL en vez de fingir que funcionó.
 */
import type { APIRoute } from 'astro';
import { getApiSession, idBaseUrl } from '../../../lib/auth';

export const prerender = false;

/** Sub-rutas permitidas bajo /api/v1/onboarding (vacía = el recurso raíz). */
const ALLOWED = new Set([
  '',
  'slug',
  'organization',
  'subscription',
  'subscription/checkout',
  'domain',
  // El token de Cloudflare del cliente viaja en el CUERPO de este POST y no se
  // registra ni se devuelve: este proxy solo lo reenvía a Sytue ID.
  'domain/cloudflare',
  'domain-availability',
  'invitations',
  'roles',
]);

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

const proxy: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const sub = (ctx.params.path ?? '').replace(/^\/+|\/+$/g, '');
  if (!ALLOWED.has(sub)) return json({ error: 'not_found' }, 404);

  const url = new URL(`${idBaseUrl()}/api/v1/onboarding${sub ? `/${sub}` : '/'}`);
  url.search = new URL(ctx.request.url).search;

  const method = ctx.request.method;
  const body = method === 'GET' ? undefined : await ctx.request.text();
  try {
    const res = await fetch(url, {
      method,
      headers: {
        authorization: `Bearer ${session.accessToken}`,
        ...(body ? { 'content-type': 'application/json' } : {}),
      },
      ...(body ? { body } : {}),
    });
    const text = await res.text();
    return new Response(text || '{}', {
      status: res.status,
      headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
    });
  } catch {
    // Sytue ID no responde: se dice, no se simula éxito.
    return json({ error: 'unavailable' }, 503);
  }
};

export const GET = proxy;
export const POST = proxy;
export const PATCH = proxy;
